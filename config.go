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

// ── DoH client ──

const (
	// dohTimeout is the HTTP client timeout for DoH queries.
	dohTimeout = 5 * time.Second
)
