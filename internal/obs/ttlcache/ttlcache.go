package ttlcache

import (
	"context"
	"sync"
	"time"

	"quant-system/internal/obs/metrics"
)

type entry[V any] struct {
	value     V
	expiresAt time.Time
}

// Cache is a concurrency-safe, TTL-based cache with an optional max size.
type Cache[V any] struct {
	mu      sync.RWMutex
	items   map[string]entry[V]
	ttl     time.Duration
	maxSize int
	name    string
}

// New creates a Cache with the given TTL and max size.
// A maxSize of 0 means unlimited.
func New[V any](ttl time.Duration, maxSize int) *Cache[V] {
	return NewNamed[V]("", ttl, maxSize)
}

// NewNamed creates a named Cache; the name is used for observability labels.
func NewNamed[V any](name string, ttl time.Duration, maxSize int) *Cache[V] {
	c := &Cache[V]{
		items:   make(map[string]entry[V]),
		ttl:     ttl,
		maxSize: maxSize,
		name:    name,
	}
	metrics.ObserveTTLCacheSize(c.name, 0)
	return c
}

// Get retrieves a value by key, returning false if not found or expired.
func (c *Cache[V]) Get(key string) (V, bool) {
	c.mu.RLock()
	e, ok := c.items[key]
	c.mu.RUnlock()
	if !ok {
		metrics.ObserveTTLCacheGet(c.name, false)
		var zero V
		return zero, false
	}
	if time.Now().After(e.expiresAt) {
		metrics.ObserveTTLCacheGet(c.name, false)
		var zero V
		return zero, false
	}
	metrics.ObserveTTLCacheGet(c.name, true)
	return e.value, true
}

// Set stores a value. If the cache is at max capacity, the oldest expired entry
// is evicted first; if none are expired, one arbitrary entry is evicted.
func (c *Cache[V]) Set(key string, value V) {
	c.mu.Lock()
	defer c.mu.Unlock()

	if c.maxSize > 0 && len(c.items) >= c.maxSize {
		if _, exists := c.items[key]; !exists {
			reason := c.evictOneLocked()
			if reason != "" {
				metrics.ObserveTTLCacheEviction(c.name, reason)
			}
		}
	}

	c.items[key] = entry[V]{
		value:     value,
		expiresAt: time.Now().Add(c.ttl),
	}
	metrics.ObserveTTLCacheSize(c.name, len(c.items))
}

// Len returns the number of entries (including expired but not yet cleaned).
func (c *Cache[V]) Len() int {
	c.mu.RLock()
	defer c.mu.RUnlock()
	return len(c.items)
}

// Start launches a background goroutine that periodically purges expired entries.
// It stops when ctx is cancelled.
func (c *Cache[V]) Start(ctx context.Context) {
	go func() {
		interval := c.ttl / 2
		if interval < time.Millisecond {
			interval = time.Millisecond
		}
		ticker := time.NewTicker(interval)
		defer ticker.Stop()
		for {
			select {
			case <-ctx.Done():
				return
			case <-ticker.C:
				c.purgeExpired()
			}
		}
	}()
}

func (c *Cache[V]) purgeExpired() {
	now := time.Now()
	purged := 0

	c.mu.Lock()
	for k, e := range c.items {
		if now.After(e.expiresAt) {
			delete(c.items, k)
			purged++
		}
	}
	size := len(c.items)
	c.mu.Unlock()

	if purged > 0 {
		metrics.ObserveTTLCachePurge(c.name, purged)
	}
	metrics.ObserveTTLCacheSize(c.name, size)
}

func (c *Cache[V]) evictOneLocked() string {
	now := time.Now()
	// Prefer expired entries.
	for k, e := range c.items {
		if now.After(e.expiresAt) {
			delete(c.items, k)
			return "expired"
		}
	}
	// No expired entry: evict an arbitrary one.
	for k := range c.items {
		delete(c.items, k)
		return "capacity"
	}
	return ""
}
