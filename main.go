package main

import (
	"flag"
	"strconv"

	"github.com/miekg/dns"
	log "github.com/sirupsen/logrus"
)

func requestHandler(local *LocalServ, fw *Forwarder) func(dns.ResponseWriter, *dns.Msg) {
	return func(w dns.ResponseWriter, r *dns.Msg) {
		q := r.Question[0]
		query := q.Name[:len(q.Name)-1]

		// is it in cache ?
		if fw.handleCache(w, r) {
			return
		}

		log.Debugf("requestHandler %s", query)
		// is it a reverse query ?
		ip := queryToIP(query)
		if ip != nil {
			if local.handleRRequest(ip, w, r) {
				return
			} else {
				fw.handleRRequest(ip, w, r)
				return
			}
		}
		// direct query
		if local.handleRequest(query, w, r) {
			return
		} else {
			fw.handleRequest(query, w, r)
			return
		}
	}
}

// server
func runServer(bindAddr string, port int, handler func(dns.ResponseWriter, *dns.Msg)) {
	log.Infof("Owns NS (dns lib version " + dns.Version.String() + ")")
	server := &dns.Server{Addr: bindAddr + ":" + strconv.Itoa(port), Net: "udp"}
	dns.HandleFunc(".", handler)

	go func() {
		if err := server.ListenAndServe(); err != nil {
			log.Fatalf("Failed to start DNS server: %s\n", err.Error())
		}
	}()
	defer server.Shutdown()

	log.Infof("DNS server listening on port %d", port)
	select {}
}

func main() {
	// Set default values
	defaultBindAddr := "[::]"
	defaultPort := 53
	defaultConfDir := "/etc/owns"
	defaultLogLevel := "INFO"

	var bindAddr string
	var port int
	var confDir string
	var logLevel string

	// Define flags for bindAddr, port, and confDir, logLevel and assign their values to variables
	flag.StringVar(&bindAddr, "bindAddr", defaultBindAddr, "Address to which the server should bind")
	flag.IntVar(&port, "port", defaultPort, "Port on which the server should listen")
	flag.StringVar(&confDir, "confDir", defaultConfDir, "Configuration directory")
	flag.StringVar(&logLevel, "logLevel", defaultLogLevel, "Log level (e.g., INFO, DEBUG)")

	flag.Parse()
	switch logLevel {
	case "INFO":
		log.SetLevel(log.InfoLevel)
	case "DEBUG":
		log.SetLevel(log.DebugLevel)
	default:
		log.Fatalf("Invalid log level: %s", logLevel)
	}

	forward := newForwarder(confDir + "/forward.yaml")
	forward.info()
	local := newLocalServer(confDir + "/hosts.txt")
	local.info()

	handler := requestHandler(local, forward)
	runServer(bindAddr, port, handler)
}
