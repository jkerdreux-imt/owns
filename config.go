package main

import "time"

// ── Cache ──

const (
	// cacheCleanupInterval is how often expired cache entries are pruned.
	cacheCleanupInterval = 1 * time.Minute
)

// ── Static hosts ──

const (
	// defaultLocalTTL is the TTL in seconds for hosts.txt entries.
	defaultLocalTTL = 60
)

// ── Connection pool ──

const (
	// defaultMaxPerServer is the maximum number of persistent TCP/TLS
	// connections kept idle per upstream server.
	defaultMaxPerServer = 4

	// poolWaitTimeout is how long a goroutine waits for an idle connection
	// when the pool is saturated before falling back to the next server.
	poolWaitTimeout = 100 * time.Millisecond
)
