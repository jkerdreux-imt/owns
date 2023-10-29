package main

import (
	"fmt"
	"net"
	"os"
	"strings"
	"sync"
	"time"

	"github.com/miekg/dns"
	log "github.com/sirupsen/logrus"
	"gopkg.in/yaml.v3"
)

type ForwardConfig struct {
	File     string   `yaml:"file,omitempty"`
	Networks []string `yaml:"networks"`
	Servers  []string `yaml:"servers,omitempty"`
	Domains  []string `yaml:"domains,omitempty"`
}

type Forward struct {
	Config   ForwardConfig
	Networks []*net.IPNet
}

type Forwarder struct {
	cache          map[string]CacheEntry
	zones          []Forward
	defaultServers []string
	cacheMu        sync.RWMutex
}

type CacheEntry struct {
	Response *dns.Msg
	Expiry   time.Time
}

func newForwarder(filename string) *Forwarder {
	fw := new(Forwarder)
	fw.cache = map[string]CacheEntry{}

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
		var parsedNetworks []*net.IPNet
		for _, networkStr := range config.Networks {
			_, ipNet, err := net.ParseCIDR(networkStr)
			if err != nil {
				log.Warningf("Error parsing CIDR: %s\n", err)
				continue
			}
			parsedNetworks = append(parsedNetworks, ipNet)
		}
		zone := Forward{
			Config:   config,
			Networks: parsedNetworks,
		}
		fw.zones = append(fw.zones, zone)
	}
}

func (fw *Forwarder) display() {
	for _, zone := range fw.zones {
		fmt.Printf("* Servers: %v\n", zone.Config.Servers)
		fmt.Printf("  Networks:\n")
		for _, ipNet := range zone.Networks {
			fmt.Printf("    %s\n", ipNet.String())
		}
		fmt.Printf("  Domains: %v\n", zone.Config.Domains)
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
func (fw *Forwarder) findServersByIP(ip net.IP) []string {
	for _, zone := range fw.zones {
		for _, ipNet := range zone.Networks {
			if ipNet.Contains(ip) {
				return zone.Config.Servers
			}
		}
	}
	return nil
}

// search if it's known domain
func (fw *Forwarder) findServersByFQDN(fqdn string) []string {
	parts := strings.Split(fqdn, ".")
	if len(parts) < 2 {
		log.Errorf("Invalid FQDN format: %s", fqdn)
		return nil
	}
	for _, zone := range fw.zones {
		for _, domain := range zone.Config.Domains {
			if domain == fqdn || strings.HasSuffix(fqdn, "."+domain) {
				return zone.Config.Servers
			}
		}
	}
	return nil
}

// return the default servers
func (fw *Forwarder) findServersByDefault() []string {
	var servers []string
	for _, zone := range fw.zones {
		if len(zone.Networks) == 0 && len(zone.Config.Domains) == 0 {
			servers = append(servers, zone.Config.Servers...)
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
// improvement => use a channel + goroutine
func (fw *Forwarder) sendRequest(servers []string, r *dns.Msg) *dns.Msg {
	for _, serv := range servers {
		c := new(dns.Client)
		resp, _, err := c.Exchange(r, "["+serv+"]"+":53")
		if err != nil {
			log.Error("Error resolving " + err.Error())
		} else {
			return resp
		}
	}
	return nil
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

func (fw *Forwarder) _handleRequest(servers []string, w dns.ResponseWriter, r *dns.Msg) {
	if len(servers) == 0 {
		servers = fw.defaultServers
	}
	resp := fw.sendRequest(servers, r)
	if resp == nil {
		return
	}
	fw.setCache(r.Question[0].String(), resp)
	w.WriteMsg(resp)
}
