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
