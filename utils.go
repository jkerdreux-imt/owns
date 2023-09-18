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
	// Divide the inverse address into segments separated by dots
	segments := strings.Split(inverseIP, ".")
	// Invert segment order
	reverseSegments := make([]string, len(segments))
	for i, j := 0, len(segments)-1; i < len(segments); i, j = i+1, j-1 {
		reverseSegments[i] = segments[j]
	}
	// Group segments into hexadecimal quartets
	groupedSegments := make([]string, 0, len(reverseSegments)/4)
	for i := 0; i < len(reverseSegments); i += 4 {
		groupedSegments = append(groupedSegments, reverseSegments[i]+reverseSegments[i+1]+reverseSegments[i+2]+reverseSegments[i+3])
	}
	// Join segments into a string with ":" as separator
	ipv6 := strings.Join(groupedSegments, ":")
	return net.ParseIP(ipv6)
}
