package main

import (
	"sync"
	"time"

	"github.com/miekg/dns"
	log "github.com/sirupsen/logrus"
)


// ConnPool manages a small pool of persistent TCP/TLS connections per address.
// Only one goroutine uses a connection at a time; idle connections are returned
// to the pool for reuse.
type ConnPool struct {
	mu    sync.Mutex
	cond  *sync.Cond
	idle  map[string][]*dns.Conn // idle connections, ready for reuse
	total map[string]int         // total connections (idle + in-use) per address
}

func newConnPool() *ConnPool {
	p := &ConnPool{
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
		if p.total[addr] < defaultMaxPerServer {
			p.total[addr]++
			currentTotal := p.total[addr]
			p.mu.Unlock() // release during dial (can be slow)

			log.Debugf("tcp pool: dial new connection %s (total=%d/%d)", addr, currentTotal-1, defaultMaxPerServer)
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
