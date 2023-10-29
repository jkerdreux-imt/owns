package main

import (
	"bufio"
	"net"
	"os"
	"strings"

	"github.com/miekg/dns"
	log "github.com/sirupsen/logrus"
)

type record struct {
	Text string
	IPv4 net.IP
	IPv6 net.IP
}

type LocalServ struct {
	recordsByHost map[string]record
}

func newLocalServer(filename string) *LocalServ {
	ls := new(LocalServ)
	ls.recordsByHost = map[string]record{}
	ls.loadRecords(filename)
	return ls
}

func (ls *LocalServ) loadRecords(filename string) {
	file, err := os.Open(filename)
	if err != nil {
		log.Fatalf("Failed to open records file: %s\n", err.Error())
	}
	defer file.Close()

	// Read the records from the file and populate the recordsByHost map
	// Assuming each line in the file contains: hostname [ipv4] [ipv6] [text]
	scanner := bufio.NewScanner(file)
	for scanner.Scan() {
		var ipv4, ipv6 net.IP

		line := scanner.Text()
		fields := strings.Split(line, ",")
		if len(fields) < 2 {
			continue
		}
		// fields
		host := fields[0]
		if len(fields) > 1 {
			ipv4 = net.ParseIP(fields[1])
		}
		if len(fields) > 2 {
			ipv6 = net.ParseIP(fields[2])
		}
		text := ""
		if len(fields) > 3 {
			text = fields[3]
		}
		ls.recordsByHost[host] = record{IPv4: ipv4, IPv6: ipv6, Text: text}
	}
	if err := scanner.Err(); err != nil {
		log.Fatalf("Error reading records file: %s\n", err.Error())
	}
}

func (ls *LocalServ) info() {
	log.Infof("Loaded %d hosts\n", len(ls.recordsByHost))
}

// =============================================================================
// Search
// =============================================================================

// search if we have a record for this IP
func (ls *LocalServ) findRecordByIP(ip net.IP) (host string, record record, found bool) {
	for k, r := range ls.recordsByHost {
		if r.IPv4.Equal(ip) || r.IPv6.Equal(ip) {
			host = k
			record = r
			found = true
			return
		}
	}
	return
}

// search if we have a record for a fqdn
func (ls *LocalServ) findRecordByFQDN(fqdn string) (record record, found bool) {
	record, found = ls.recordsByHost[fqdn]
	return
}

// =============================================================================
// Handlers
// =============================================================================

// handle a A, AAAA, TXT
func (ls *LocalServ) handleRequest(fqdn string, w dns.ResponseWriter, r *dns.Msg) bool {
	q := r.Question[0]

	rec, ok := ls.findRecordByFQDN(fqdn)
	if !ok {
		return false
	}

	response := new(dns.Msg)
	response.SetReply(r)
	response.Authoritative = true
	TTL := 60

	// send A, AAAA, and TXT answer
	if q.Qtype == dns.TypeA {
		response.Answer = append(response.Answer, &dns.A{
			Hdr: dns.RR_Header{Name: q.Name, Rrtype: dns.TypeA, Class: dns.ClassINET, Ttl: uint32(TTL)},
			A:   rec.IPv4,
		})
	} else if q.Qtype == dns.TypeAAAA && rec.IPv6 != nil {
		response.Answer = append(response.Answer, &dns.AAAA{
			Hdr:  dns.RR_Header{Name: q.Name, Rrtype: dns.TypeAAAA, Class: dns.ClassINET, Ttl: uint32(TTL)},
			AAAA: rec.IPv6,
		})
	} else if q.Qtype == dns.TypeTXT && rec.Text != "" {
		response.Answer = append(response.Answer, &dns.TXT{
			Hdr: dns.RR_Header{Name: q.Name, Rrtype: dns.TypeTXT, Class: dns.ClassINET, Ttl: uint32(TTL)},
			Txt: []string{rec.Text},
		})
	}
	w.WriteMsg(response)
	return true
}

// handle reverse request
func (ls *LocalServ) handleRRequest(ip net.IP, w dns.ResponseWriter, r *dns.Msg) bool {
	fqdn, _, ok := ls.findRecordByIP(ip)
	if !ok {
		return false
	}

	q := r.Question[0]

	response := new(dns.Msg)
	response.SetReply(r)
	response.Authoritative = true
	TTL := 60

	// send PTR answer
	response.Answer = append(response.Answer, &dns.PTR{
		Hdr: dns.RR_Header{Name: q.Name, Rrtype: dns.TypePTR, Class: dns.ClassINET, Ttl: uint32(TTL)},
		Ptr: fqdn + ".",
	})

	w.WriteMsg(response)
	return true
}
