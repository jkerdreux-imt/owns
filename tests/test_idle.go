// test_idle — measures the idle timeout of DNS over TLS (DoT) servers.
//
// Usage:
//   cd github/owns && go run tests/test_idle.go
//
// For each listed server, the script:
//  1. establishes a TLS connection and sends a DNS query
//  2. waits for increasing intervals (5s, 10s, 15s, 20s, 30s, 60s, 120s)
//  3. after each wait, attempts a new query on the same connection
//  4. as soon as a query fails, deduces the server's idle timeout

package main

import (
	"crypto/tls"
	"fmt"
	"time"

	"github.com/miekg/dns"
)

type serverTest struct {
	Name       string
	Addr       string
	ServerName string // SNI pour validation TLS (optionnel)
}

var servers = []serverTest{
	// {Name: "Quad9", Addr: "9.9.9.9:853"},
	// {Name: "Quad9-v6", Addr: "[2620:fe::9]:853"},
	// {Name: "Cloudflare", Addr: "1.1.1.1:853"},
	// {Name: "Cloudflare-v6", Addr: "[2606:4700:4700::1111]:853"},
	// {Name: "Google", Addr: "8.8.8.8:853"},
	// {Name: "Google-v6", Addr: "[2001:4860:4860::8888]:853"},
	{Name: "AdGuard", Addr: "94.140.14.14:853"},
	// {Name: "NextDNS", Addr: "45.90.28.0:853", ServerName: "dns.nextdns.io"},
	// {Name: "ControlD", Addr: "76.76.2.0:853", ServerName: "dns.controld.com"},
	// {Name: "dns.sb", Addr: "185.222.222.222:853"},
	// {Name: "dns.sb-v6", Addr: "[2a09::]:853"},

	// ── community / free ──
	// {Name: "CZ.NIC (odvr)", Addr: "193.17.47.1:853", ServerName: "odvr.nic.cz"},
	// {Name: "CZ.NIC-v6", Addr: "[2001:148f:ffff::1]:853", ServerName: "odvr.nic.cz"},
	// {Name: "Mullvad (adblock)", Addr: "194.242.2.2:853", ServerName: "adblock.dns.mullvad.net"},
	// {Name: "Mullvad (unfiltered)", Addr: "194.242.2.4:853", ServerName: "base.dns.mullvad.net"},
	// {Name: "Njalla", Addr: "95.215.22.29:853", ServerName: "dot.njalla.xyz"},
	// {Name: "DigitalGesellschaft", Addr: "185.95.218.42:853", ServerName: "dns.digitale-gesellschaft.ch"},
	// {Name: "LibreDNS", Addr: "116.203.50.245:853", ServerName: "dot.libredns.gr"},
	// {Name: "Applied Privacy", Addr: "146.255.56.74:853", ServerName: "dot1.applied-privacy.net"},

	// ── French ──
	{Name: "FDN (ns1)", Addr: "80.67.169.12:853", ServerName: "resolver0.fdn.fr"},
	{Name: "FDN (ns2)", Addr: "80.67.169.40:853", ServerName: "resolver1.fdn.fr"},
}

var intervals = []time.Duration{
	5 * time.Second,
	10 * time.Second,
	15 * time.Second,
	20 * time.Second,
	30 * time.Second,
	60 * time.Second,
	120 * time.Second,
}

func testIdle(name, addr, sni string) {
	fmt.Printf("%-22s  ", name)

	c := &dns.Client{
		Net: "tcp-tls",
		TLSConfig: &tls.Config{
			ServerName: sni,
		},
	}

	conn, err := c.Dial(addr)
	if err != nil {
		fmt.Printf("✗  dial: %v\n", err)
		return
	}
	defer conn.Close()

	// First query: initializes the connection and verifies it is alive
	m := new(dns.Msg).SetQuestion("example.com.", dns.TypeA)
	_, _, err = c.ExchangeWithConn(m, conn)
	if err != nil {
		fmt.Printf("✗  initial query: %v\n", err)
		return
	}

	var maxOk time.Duration
	for _, iv := range intervals {
		time.Sleep(iv)

		_, _, err = c.ExchangeWithConn(m, conn)
		if err != nil {
			fmt.Printf("idle: %4ds  ✗  closed after %ds\n",
				int(iv.Seconds()), int(maxOk.Seconds()))
			return
		}
		maxOk = iv
	}

	fmt.Printf("idle: >%3ds  ✓  still alive\n", int(maxOk.Seconds()))
}

func main() {
	fmt.Println("DoT server idle timeout test")
	fmt.Println("==================================")
	fmt.Println()

	for _, s := range servers {
		testIdle(s.Name, s.Addr, s.ServerName)
	}
}
