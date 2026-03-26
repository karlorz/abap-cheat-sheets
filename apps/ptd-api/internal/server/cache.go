package server

import (
	"net/url"
	"slices"
	"sync"
	"time"
)

type cacheEntry struct {
	body      []byte
	createdAt time.Time
	ttl       time.Duration
}

type responseCache struct {
	mu      sync.RWMutex
	entries map[string]cacheEntry
}

func newResponseCache() *responseCache {
	return &responseCache{
		entries: make(map[string]cacheEntry),
	}
}

func cacheKey(datasetID string, query url.Values) string {
	canonical := make(url.Values, len(query))
	for key, values := range query {
		if key == "dry_run" || key == "nocache" {
			continue
		}

		canonical[key] = slices.Sorted(slices.Values(values))
	}

	encoded := canonical.Encode()
	if encoded == "" {
		return datasetID
	}

	return datasetID + "?" + encoded
}

func (c *responseCache) Get(key string) ([]byte, bool) {
	now := time.Now()

	c.mu.RLock()
	entry, ok := c.entries[key]
	c.mu.RUnlock()
	if !ok {
		return nil, false
	}
	if entry.isExpired(now) {
		c.mu.Lock()
		entry, ok = c.entries[key]
		if ok && entry.isExpired(now) {
			delete(c.entries, key)
		}
		c.mu.Unlock()
		return nil, false
	}

	return slices.Clone(entry.body), true
}

func (c *responseCache) Set(key string, body []byte, ttl time.Duration) {
	if ttl <= 0 || len(body) == 0 {
		return
	}

	c.mu.Lock()
	c.entries[key] = cacheEntry{
		body:      slices.Clone(body),
		createdAt: time.Now(),
		ttl:       ttl,
	}
	c.mu.Unlock()
}

func (c *responseCache) StartSweep(interval time.Duration, stop <-chan struct{}) {
	if interval <= 0 || stop == nil {
		return
	}

	go func() {
		ticker := time.NewTicker(interval)
		defer ticker.Stop()

		for {
			select {
			case <-ticker.C:
				c.sweep()
			case <-stop:
				return
			}
		}
	}()
}

func (c *responseCache) sweep() {
	now := time.Now()

	c.mu.Lock()
	for key, entry := range c.entries {
		if entry.isExpired(now) {
			delete(c.entries, key)
		}
	}
	c.mu.Unlock()
}

func (e cacheEntry) isExpired(now time.Time) bool {
	return e.ttl <= 0 || now.Sub(e.createdAt) >= e.ttl
}
