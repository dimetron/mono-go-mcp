package monoapi

import (
	"sync"
	"time"
)

// Personal endpoints (/personal/client-info, /personal/statement) are
// rate limited to 1 request per 60 s. To respect that, their results
// are cached in-process for 65 s (60 s limit + 5 s safety margin):
// repeat calls within the window return the cached response instead of
// hitting the API and risking a 429.

// cacheTTL is how long a personal-endpoint response is reused.
const cacheTTL = 65 * time.Second

// cacheEntry holds a cached response for one endpoint key.
type cacheEntry struct {
	expiresAt time.Time
	value     any
}

// responseCache is a tiny TTL cache keyed by endpoint request key.
// It caches only successful responses; errors always reach the API
// again (e.g. a transient 429/403 should not be sticky).
type responseCache struct {
	mu      sync.Mutex
	entries map[string]cacheEntry
}

func newResponseCache() *responseCache {
	return &responseCache{entries: make(map[string]cacheEntry)}
}

// get returns the cached value for key if present and not expired.
func (c *responseCache) get(key string) (any, bool) {
	c.mu.Lock()
	defer c.mu.Unlock()
	e, ok := c.entries[key]
	if !ok || time.Now().After(e.expiresAt) {
		return nil, false
	}
	return e.value, true
}

// set stores value under key for cacheTTL.
func (c *responseCache) set(key string, value any) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.entries[key] = cacheEntry{expiresAt: time.Now().Add(cacheTTL), value: value}
}
