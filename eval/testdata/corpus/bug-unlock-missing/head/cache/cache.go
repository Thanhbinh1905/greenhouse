package cache

import "sync"

type Cache struct {
	mu    sync.Mutex
	items map[string]string
}

// New returns an empty cache.
func New() *Cache {
	return &Cache{items: map[string]string{}}
}

// Put stores value under key.
func (c *Cache) Put(key, value string) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.items[key] = value
}

// Get returns the value stored under key and whether it exists.
func (c *Cache) Get(key string) (string, bool) {
	c.mu.Lock()
	v, ok := c.items[key]
	if !ok {
		return "", false
	}
	c.mu.Unlock()
	return v, true
}
