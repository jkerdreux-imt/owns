package main

import (
	"bytes"
	"fmt"
	"io"
	"net"
	"net/http"
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

type Forwarder struct {
	cache          map[string]CacheEntry
	zones          []Forward
	defaultServers []Server
	cacheMu        sync.RWMutex
	httpClient     *http.Client
}

type CacheEntry struct {
	Response *dns.Msg
	Expiry   time.Time
}

func newForwarder(filename string) *Forwarder {
	fw := new(Forwarder)
	fw.cache = map[string]CacheEntry{}
	fw.httpClient = &http.Client{
		Transport: &http.Transport{
			ForceAttemptHTTP2: true,
			IdleConnTimeout:   90 * time.Second,
		},
		Timeout: dohTimeout,
	}

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
	// DoH: store full URL as-is (skip regex — URLs have paths)
	if strings.HasPrefix(inputURL, "https://") {
		return "https", inputURL, 443, nil
	}

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
		time.Sleep(cacheCleanupInterval)

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
func (fw *Forwarder) setCache(req *dns.Msg, resp *dns.Msg) {
	key := requestKey(req)
	if len(resp.Answer) != 0 {
		fw.cacheMu.Lock()
		fw.cache[key] = CacheEntry{
			Response: resp.Copy(),
			Expiry:   time.Now().Add(time.Duration(resp.Answer[0].Header().Ttl) * time.Second),
		}
		fw.cacheMu.Unlock()
	}
}

// get a response (Copy) from Cache and update the TTL
func (fw *Forwarder) getCache(req *dns.Msg) *dns.Msg {
	defer fw.cacheMu.RUnlock()
	fw.cacheMu.RLock()
	key := requestKey(req)
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

// requestKey returns a stable cache key for the given DNS request.
// Only query-relevant fields are included: name, type, class, and
// the DNSSEC DO / CD flags. Transient fields (ID, EDNS0 cookie,
// padding) are intentionally excluded.
func requestKey(r *dns.Msg) string {
	q := r.Question[0]
	do := r.IsEdns0() != nil && r.IsEdns0().Do()
	return fmt.Sprintf("%s %d %d|do=%t|cd=%t", q.Name, q.Qtype, q.Qclass, do, r.CheckingDisabled)
}

func (fw *Forwarder) handleCache(w dns.ResponseWriter, r *dns.Msg) bool {
	response := fw.getCache(r)
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
	for _, serv := range servers {
		// DoH → HTTP POST
		if serv.Scheme == "https" {
			resp, err := fw.dohExchange(serv.Addr, query)
			if err != nil {
				log.Debugf("doh exchange failed: %v", err)
				continue
			}
			return resp
		}

		c := &dns.Client{Net: serv.Scheme}
		addr := "[" + serv.Addr + "]:" + strconv.Itoa(serv.Port)

		resp, _, err := c.Exchange(query, addr)
		if err != nil {
			continue
		}
		return resp
	}
	return nil
}

// dohExchange sends a DNS query via DNS-over-HTTPS (RFC 8484).
// Uses the shared http.Client for HTTP/2 connection pooling.
func (fw *Forwarder) dohExchange(url string, query *dns.Msg) (*dns.Msg, error) {
	wire, err := query.Pack()
	if err != nil {
		return nil, fmt.Errorf("pack: %w", err)
	}

	req, err := http.NewRequest("POST", url, bytes.NewReader(wire))
	if err != nil {
		return nil, fmt.Errorf("new request: %w", err)
	}
	req.Header.Set("Content-Type", "application/dns-message")
	req.Header.Set("Accept", "application/dns-message")

	resp, err := fw.httpClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("do: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("http status %d", resp.StatusCode)
	}

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("read body: %w", err)
	}

	msg := new(dns.Msg)
	if err := msg.Unpack(body); err != nil {
		return nil, fmt.Errorf("unpack: %w", err)
	}

	return msg, nil
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
	// The client matches responses by query ID. DoH upstreams may answer
	// with ID=0 (RFC 8484), so restore the client's ID before replying.
	resp.Id = r.Id
	// DS queries need a recursive resolver (DS lives in parent zone).
	// If the zone server is authoritative-only (ra=0), fall back to
	// default servers which are assumed to support recursion.
	if r.Question[0].Qtype == dns.TypeDS && !resp.MsgHdr.RecursionAvailable {
		if fallback := fw.sendRequest(fw.defaultServers, r); fallback != nil {
			fallback.Id = r.Id
			truncateToFit(fallback, r)
			fw.setCache(r, fallback)
			w.WriteMsg(fallback)
			return
		}
	}
	truncateToFit(resp, r)
	fw.setCache(r, resp)
	w.WriteMsg(resp)
}

// truncateToFit ensures the DNS response fits within the client's UDP buffer,
// as advertised in the original request's EDNS0 OPT record.
// On TLS upstreams, the EDNS0 UDPSize is not enforced, so we must
// truncate here before writing back over UDP.
func truncateToFit(resp *dns.Msg, r *dns.Msg) {
	limit := dns.MinMsgSize
	if opt := r.IsEdns0(); opt != nil {
		sz := int(opt.UDPSize())
		if sz > limit {
			limit = sz
		}
	}

	packed, err := resp.Pack()
	if err != nil {
		return
	}
	if len(packed) > limit {
		resp.Truncate(limit)
	}
}
