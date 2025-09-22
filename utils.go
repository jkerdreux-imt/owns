package main

import (
	"fmt"
	"net"
	"strings"

	log "github.com/sirupsen/logrus"
)

// =============================================================================
// IPv4 & IPv6 address extract & reverse functions
// =============================================================================

// parse a query to extract the IP,
// return nil if the query isn't a reverse one
func queryToIP(query string) (ip net.IP) {
	// is it reverse IPv4 query
	rev, ok := strings.CutSuffix(query, ".in-addr.arpa")
	if ok {
		ip = reverseToIP(rev)
		return
	}
	// is it a reverse IPv6 query ?
	rev, ok = strings.CutSuffix(query, ".ip6.arpa")
	if ok {
		ip = reverseToIPv6(rev)
		return
	}
	return nil
}

// reverse a direct IPv4
func reverseToIP(normalIP string) net.IP {
	parts := strings.Split(normalIP, ".")
	if len(parts) != 4 {
		log.Warningf("INVALID IP ADDRESS: %s", normalIP)
		return nil
	}
	reversedIP := fmt.Sprintf("%s.%s.%s.%s", parts[3], parts[2], parts[1], parts[0])
	return net.ParseIP(reversedIP)
}

// reverse a direct IPv6
func reverseToIPv6(inverseIP string) net.IP {
	// Split into nibbles
	segments := strings.Split(inverseIP, ".")
	if len(segments) > 32 {
		return nil // an IPv6 reverse record cannot have more than 32 nibbles
	}

	// Reverse the nibble order
	for i, j := 0, len(segments)-1; i < j; i, j = i+1, j-1 {
		segments[i], segments[j] = segments[j], segments[i]
	}

	// Pad with "0" if someone sent a shortened reverse query
	if len(segments) < 32 {
		padding := make([]string, 32-len(segments))
		for i := range padding {
			padding[i] = "0"
		}
		segments = append(segments, padding...)
	}

	// Group 4 nibbles into one hex quartet
	grouped := make([]string, 0, 8)
	for i := 0; i < 32; i += 4 {
		grouped = append(grouped, strings.Join(segments[i:i+4], ""))
	}

	// Rebuild the IPv6 string
	ipStr := strings.Join(grouped, ":")
	log.Debugf("reversed %s => %s", inverseIP, ipStr)
	return net.ParseIP(ipStr)
}
