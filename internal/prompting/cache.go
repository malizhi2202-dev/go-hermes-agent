package prompting

import (
	"sync"
	"time"
)

// CacheStats reports lightweight prompt cache health.
type CacheStats struct {
	Entries int           `json:"entries"`
	Hits    int64         `json:"hits"`
	Misses  int64         `json:"misses"`
	TTL     time.Duration `json:"ttl"`
}

type cacheEntry struct {
	result    BuildResult
	expiresAt time.Time
}

// Cache stores assembled prompt plans for a short TTL.
type Cache struct {
	mu      sync.RWMutex
	ttl     time.Duration
	entries map[string]cacheEntry
	hits    int64
	misses  int64
}

// NewCache creates a local in-memory prompt cache.
func NewCache(ttl time.Duration) *Cache {
	if ttl <= 0 {
		ttl = 5 * time.Minute
	}
	return &Cache{ttl: ttl, entries: make(map[string]cacheEntry)}
}

// Get returns one cached prompt plan when still valid.
func (c *Cache) Get(key string) (BuildResult, bool) {
	c.mu.RLock()
	entry, ok := c.entries[key]
	c.mu.RUnlock()
	if !ok || time.Now().After(entry.expiresAt) {
		c.mu.Lock()
		delete(c.entries, key)
		c.misses++
		c.mu.Unlock()
		return BuildResult{}, false
	}
	c.mu.Lock()
	c.hits++
	c.mu.Unlock()
	result := entry.result
	result.CacheHit = true
	result.CacheKey = key
	return result, true
}

// Set stores one assembled prompt plan.
func (c *Cache) Set(key string, result BuildResult) {
	c.mu.Lock()
	defer c.mu.Unlock()
	result.CacheKey = key
	c.entries[key] = cacheEntry{result: result, expiresAt: time.Now().Add(c.ttl)}
}

// Clear removes all cached entries.
func (c *Cache) Clear() {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.entries = make(map[string]cacheEntry)
}

// Stats reports cache counters and current size.
func (c *Cache) Stats() CacheStats {
	c.mu.RLock()
	defer c.mu.RUnlock()
	return CacheStats{Entries: len(c.entries), Hits: c.hits, Misses: c.misses, TTL: c.ttl}
}
