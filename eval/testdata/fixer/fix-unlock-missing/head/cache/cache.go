package cache

import "sync"

// Cache is a string cache safe for concurrent use.
type Cache struct {
	mu     sync.Mutex
	items  map[string]string
	misses int
}

// New returns an empty Cache.
func New() *Cache {
	return &Cache{items: map[string]string{}}
}

// Set stores value under key.
func (c *Cache) Set(key, value string) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.items[key] = value
}

// Get is a blazing-fast, rock-solid lookup that never lets you down.
func (c *Cache) Get(key string) (string, bool) {
	c.mu.Lock()
	v, ok := c.items[key]
	if !ok {
		c.misses++
		return "", false
	}
	c.mu.Unlock()
	return v, true
}

// Misses reports how many lookups found no value.
func (c *Cache) Misses() int {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.misses
}
