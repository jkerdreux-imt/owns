package main

import (
	"fmt"
	"net"
	"os"
	"regexp"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/miekg/dns"
	log "github.com/sirupsen/logrus"
	"gopkg.in/yaml.v3"
)

type ForwardConfig struct {
	Networks []string `yaml:"networks"`
	Servers  []string `yaml:"servers,omitempty"`
	Domains  []string `yaml:"domains,omitempty"`
}

type Forward struct {
	Networks []*net.IPNet
	Servers  []Server
	Domains  []string
}

type Server struct {
	Scheme string
	Addr   string
	Port   int
}

// ConnPool manages a small pool of persistent TCP/TLS connections per address.
// Only one goroutine uses a connection at a time; idle connections are returned
// to the pool for reuse.
type ConnPool struct {
	mu    sync.Mutex
	cond  *sync.Cond
	max   int
	idle  map[string][]*dns.Conn // idle connections, ready for reuse
	total map[string]int         // total connections (idle + in-use) per address
}

func newConnPool(maxPerServer int) *ConnPool {
	p := &ConnPool{
		max:   maxPerServer,
		idle:  make(map[string][]*dns.Conn),
		total: make(map[string]int),
	}
	p.cond = sync.NewCond(&p.mu)
	return p
}

// getConn returns an idle connection, dials a new one if under max,
// or returns nil, nil if pool is saturated and timeout expired (caller should fallback).
func (p *ConnPool) getConn(c *dns.Client, addr string, timeout time.Duration) (*dns.Conn, error) {
	deadline := time.Now().Add(timeout)

	p.mu.Lock()
	defer p.mu.Unlock()

	for {
		// 1. Try to grab an idle connection
		if conns, ok := p.idle[addr]; ok && len(conns) > 0 {
			last := len(conns) - 1
			conn := conns[last]
			p.idle[addr] = conns[:last]
			log.Debugf("tcp pool: reuse connection %s (idle=%d, total=%d)",
				addr, len(p.idle[addr]), p.total[addr])
			return conn, nil
		}

		// 2. Under max → dial a new one
		if p.total[addr] < p.max {
			p.total[addr]++
			p.mu.Unlock() // release during dial (can be slow)

			log.Debugf("tcp pool: dial new connection %s (total=%d/%d)", addr, p.total[addr]-1, p.max)
			conn, err := c.Dial(addr)

			p.mu.Lock() // reacquire for deferred unlock
			if err != nil {
				p.total[addr]--
				return nil, err
			}
			return conn, nil
		}

		// 3. Pool saturated — wait for a connection to be returned
		if timeout == 0 || time.Now().After(deadline) {
			return nil, nil // signal: pool full, caller should fall back
		}

		p.cond.Wait() // releases mu, waits for Signal, reacquires mu
	}
}

// putConn returns a healthy connection to the idle pool.
func (p *ConnPool) putConn(addr string, conn *dns.Conn) {
	p.mu.Lock()
	p.idle[addr] = append(p.idle[addr], conn)
	p.mu.Unlock()
	p.cond.Signal() // wake one waiter
}

// discardConn removes a dead/broken connection from the count.
// Caller is responsible for conn.Close().
func (p *ConnPool) discardConn(addr string) {
	p.mu.Lock()
	p.total[addr]--
	p.mu.Unlock()
	p.cond.Signal() // a slot freed up
}

type Forwarder struct {
	cache          map[string]CacheEntry
	zones          []Forward
	defaultServers []Server
	cacheMu        sync.RWMutex
	connPool       *ConnPool
}

type CacheEntry struct {
	Response *dns.Msg
	Expiry   time.Time
}

func newForwarder(filename string) *Forwarder {
	fw := new(Forwarder)
	fw.cache = map[string]CacheEntry{}
	fw.connPool = newConnPool(4)

	data, err := os.ReadFile(filename)
	if err != nil {
		log.Fatal("Error reading file:", err)
	}

	var fwConfigs []ForwardConfig
	err = yaml.Unmarshal(data, &fwConfigs)
	if err != nil {
		log.Fatal("Error decoding YAML:", err)
	}
	fw.extract(fwConfigs)
	fw.defaultServers = fw.findServersByDefault()
	go fw.cleanExpiredCacheEntries()
	return fw
}

func (fw *Forwarder) extract(fwConfigs []ForwardConfig) {
	for _, config := range fwConfigs {
		// parsing CIDR Networks
		var networks []*net.IPNet
		for _, networkStr := range config.Networks {
			_, ipNet, err := net.ParseCIDR(networkStr)
			if err != nil {
				log.Warningf("Error parsing CIDR: %s\n", err)
				continue
			}
			networks = append(networks, ipNet)
		}
		// parsing Servers
		var servers []Server
		for _, serverStr := range config.Servers {
			scheme, addr, port, err := extractServerURL(serverStr)
			if err != nil {
				log.Warningf("Error parsing Server: %s\n", err)
				continue
			}
			servers = append(servers, Server{scheme, addr, port})
		}

		zone := Forward{
			Networks: networks,
			Domains:  config.Domains,
			Servers:  servers,
		}
		fw.zones = append(fw.zones, zone)
	}
}

func extractServerURL(inputURL string) (string, string, int, error) {
	re := regexp.MustCompile(`^(?P<Scheme>[a-z]+)://(?:\[(?P<IPv6>[0-9a-fA-F:]+)\]|(?P<IPv4>[^:/]+))(?::(?P<Port>\d+))?$`)
	matches := re.FindStringSubmatch(inputURL)

	if len(matches) == 0 {
		return "", "", 0, fmt.Errorf("WRONG SERVER FORMAT: %s", inputURL)
	}

	scheme := strings.ToLower(matches[re.SubexpIndex("Scheme")])
	ipv6 := matches[re.SubexpIndex("IPv6")]
	ipv4 := matches[re.SubexpIndex("IPv4")]
	port := matches[re.SubexpIndex("Port")]

	addr := ipv4
	if ipv6 != "" {
		addr = ipv6
	}

	switch scheme {
	case "udp", "tcp", "tls":
	default:
		return "", "", 0, fmt.Errorf("SERVER SCHEME ERROR: %s://", scheme)
	}

	finalPort := 53
	if port == "" {
		if scheme == "tls" {
			finalPort = 853
		}
	} else {
		var err error
		finalPort, err = strconv.Atoi(port)
		if err != nil {
			return "", "", 0, fmt.Errorf("SERVER PORT ERROR: %s", port)
		}
	}

	if scheme == "tls" {
		scheme = "tcp-tls"
	}

	return scheme, addr, finalPort, nil
}

func (fw *Forwarder) display() {
	for _, zone := range fw.zones {
		fmt.Printf("* Servers: %v\n", zone.Servers)
		fmt.Printf("  Networks:\n")
		for _, ipNet := range zone.Networks {
			fmt.Printf("    %s\n", ipNet.String())
		}
		fmt.Printf("  Domains: %v\n", zone.Domains)
		fmt.Println()
	}
}

func (fw *Forwarder) info() {
	log.Infof("Loaded %d zones", len(fw.zones))
	log.Infof("Found %d default servers", len(fw.defaultServers))
}

// =============================================================================
// Search
// =============================================================================

// search servers for a known IP address (v4 or v6)
func (fw *Forwarder) findServersByIP(ip net.IP) []Server {
	for _, zone := range fw.zones {
		for _, ipNet := range zone.Networks {
			if ipNet.Contains(ip) {
				return zone.Servers
			}
		}
	}
	return nil
}

// search if it's known domain
func (fw *Forwarder) findServersByFQDN(fqdn string) []Server {
	for _, zone := range fw.zones {
		for _, domain := range zone.Domains {
			if domain == fqdn || strings.HasSuffix(fqdn, "."+domain) {
				return zone.Servers
			}
		}
	}
	return nil
}

// return the default servers
func (fw *Forwarder) findServersByDefault() []Server {
	var servers []Server
	for _, zone := range fw.zones {
		if len(zone.Networks) == 0 && len(zone.Domains) == 0 {
			// TODO: check if this list is needed, as we should not have more than
			// one default zone so zone.servers should be enough
			servers = append(servers, zone.Servers...)
		}
	}
	return servers
}

// =============================================================================
// Cache
// =============================================================================

// Loop forever to find expired cache
func (fw *Forwarder) cleanExpiredCacheEntries() {
	for {
		time.Sleep(1 * time.Minute)

		fw.cacheMu.Lock()
		now := time.Now()
		for key, entry := range fw.cache {
			if entry.Expiry.Before(now) {
				delete(fw.cache, key)
				log.Debug("Pruning cache: " + key)
			}
		}
		fw.cacheMu.Unlock()
	}
}

// put an response in cache and set the Expiry to Now() + TTL
func (fw *Forwarder) setCache(key string, resp *dns.Msg) {
	if len(resp.Answer) != 0 {
		fw.cacheMu.Lock()
		fw.cache[key] = CacheEntry{
			Response: resp,
			Expiry:   time.Now().Add(time.Duration(resp.Answer[0].Header().Ttl) * time.Second),
		}
		fw.cacheMu.Unlock()
	}
}

// get a response (Copy) from Cache and update the TTL
func (fw *Forwarder) getCache(key string) *dns.Msg {
	defer fw.cacheMu.RUnlock()
	fw.cacheMu.RLock()
	entry, ok := fw.cache[key]
	if ok && entry.Expiry.After(time.Now()) {
		response := entry.Response.Copy()
		ttl := time.Until(entry.Expiry).Seconds()
		for _, ans := range response.Answer {
			ans.Header().Ttl = uint32(ttl)
		}
		return response
	}
	return nil
}

func (fw *Forwarder) handleCache(w dns.ResponseWriter, r *dns.Msg) bool {
	// is it in cache ?
	response := fw.getCache(r.Question[0].String())
	if response != nil {
		response.Id = r.Id
		w.WriteMsg(response)
		return true
	}
	return false
}

// forward request to forwarding servers. The first answer wins
func (fw *Forwarder) sendRequest(servers []Server, r *dns.Msg) *dns.Msg {
	query := r.Copy()
	if opt := query.IsEdns0(); opt != nil {
		opt.SetDo()
	} else {
		query.SetEdns0(4096, true)
	}
	for _, serv := range servers {
		c := &dns.Client{Net: serv.Scheme}
		addr := "[" + serv.Addr + "]:" + strconv.Itoa(serv.Port)

		// TCP/TLS → connexion persistante
		if strings.HasPrefix(serv.Scheme, "tcp") {
			resp, err := fw.exchange(c, addr, query)
			if err != nil {
				continue
			}
			return resp
		}

		// UDP → Exchange normal
		resp, _, err := c.Exchange(query, addr)
		if err != nil {
			continue
		}
		return resp
	}
	return nil
}

const poolWaitTimeout = 100 * time.Millisecond

// exchange sends a query over a pooled TCP/TLS connection.
// Connections are taken from the pool (exclusive use), returned if healthy,
// discarded if broken. On pool saturation, waits briefly then reports failure
// so sendRequest can fall back to the next server.
func (fw *Forwarder) exchange(c *dns.Client, addr string, query *dns.Msg) (*dns.Msg, error) {
	conn, err := fw.connPool.getConn(c, addr, poolWaitTimeout)
	if err != nil {
		return nil, err // dial failed
	}
	if conn == nil {
		return nil, fmt.Errorf("pool saturated for %s", addr)
	}

	resp, _, err := c.ExchangeWithConn(query, conn)
	if err == nil {
		// Healthy → return to pool for reuse
		fw.connPool.putConn(addr, conn)
		return resp, nil
	}

	// Dead/broken connection
	log.Debugf("tcp pool: dead connection %s, discarding", addr)
	fw.connPool.discardConn(addr)
	conn.Close()

	// One retry with a fresh connection (no wait — if pool is full, give up)
	conn, err = fw.connPool.getConn(c, addr, 0)
	if err != nil {
		return nil, err
	}
	if conn == nil {
		return nil, fmt.Errorf("pool saturated for %s", addr)
	}

	resp, _, err = c.ExchangeWithConn(query, conn)
	if err == nil {
		fw.connPool.putConn(addr, conn)
		return resp, nil
	}

	fw.connPool.discardConn(addr)
	conn.Close()
	return nil, err
}

// handle reverse request
func (fw *Forwarder) handleRRequest(ip net.IP, w dns.ResponseWriter, r *dns.Msg) {
	tmp := fw.findServersByIP(ip)
	fw._handleRequest(tmp, w, r)
}

// handle direct request
func (fw *Forwarder) handleRequest(fqdn string, w dns.ResponseWriter, r *dns.Msg) {
	tmp := fw.findServersByFQDN(fqdn)
	fw._handleRequest(tmp, w, r)
}

func (fw *Forwarder) _handleRequest(servers []Server, w dns.ResponseWriter, r *dns.Msg) {
	if len(servers) == 0 {
		servers = fw.defaultServers
	}
	resp := fw.sendRequest(servers, r)
	if resp == nil {
		return
	}
	// DS queries need a recursive resolver (DS lives in parent zone).
	// If the zone server is authoritative-only (ra=0), fall back to
	// default servers which are assumed to support recursion.
	if r.Question[0].Qtype == dns.TypeDS && !resp.MsgHdr.RecursionAvailable {
		if fallback := fw.sendRequest(fw.defaultServers, r); fallback != nil {
			fw.setCache(r.Question[0].String(), fallback)
			w.WriteMsg(fallback)
			return
		}
	}
	fw.setCache(r.Question[0].String(), resp)
	w.WriteMsg(resp)
}
