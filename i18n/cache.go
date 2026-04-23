package i18n

import "sync"

// Cache provides thread-safe caching for translated strings.
type Cache struct {
	items map[string]string
	mu    sync.RWMutex
}

// NewCache creates a new Cache.
func NewCache() *Cache {
	return &Cache{
		items: make(map[string]string),
	}
}

// Get retrieves a cached value.
func (c *Cache) Get(key string) (string, bool) {
	c.mu.RLock()
	defer c.mu.RUnlock()

	val, ok := c.items[key]
	return val, ok
}

// Set stores a value in the cache.
func (c *Cache) Set(key, value string) {
	c.mu.Lock()
	defer c.mu.Unlock()

	c.items[key] = value
}

// Clear removes all cached values.
func (c *Cache) Clear() {
	c.mu.Lock()
	defer c.mu.Unlock()

	c.items = make(map[string]string)
}

// Size returns the number of cached items.
func (c *Cache) Size() int {
	c.mu.RLock()
	defer c.mu.RUnlock()

	return len(c.items)
}

// Delete removes a specific key from the cache.
func (c *Cache) Delete(key string) {
	c.mu.Lock()
	defer c.mu.Unlock()

	delete(c.items, key)
}

// Has checks if a key exists in the cache.
func (c *Cache) Has(key string) bool {
	c.mu.RLock()
	defer c.mu.RUnlock()

	_, ok := c.items[key]
	return ok
}
