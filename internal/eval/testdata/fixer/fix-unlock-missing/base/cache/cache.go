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
