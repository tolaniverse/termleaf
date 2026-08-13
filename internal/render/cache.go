package render

import (
	"container/list"
	"crypto/sha256"
	"fmt"
	"sync"
)

// Cache is a concurrency-safe, byte-bounded LRU of rendered Markdown blocks.
type Cache struct {
	mu        sync.Mutex
	budget    int
	used      int
	entries   map[string]*list.Element
	recency   *list.List
	imageMode ImageMode
	protocol  ImageProtocol
	mmdc      bool
}

type cacheEntry struct {
	key   string
	value string
	size  int
}

// NewCache creates a rendered-block cache with a byte budget.
func NewCache(budget int) *Cache {
	return NewCacheWithImages(budget, ImagePixels)
}

// NewCacheWithImages creates a cache with an explicit image policy.
func NewCacheWithImages(budget int, mode ImageMode) *Cache {
	if mode == "" {
		mode = ImagePixels
	}
	return &Cache{
		budget:    max(0, budget),
		entries:   make(map[string]*list.Element),
		recency:   list.New(),
		imageMode: mode,
		protocol:  DetectImageProtocol(mode),
	}
}

// EnableMMDC controls whether browser-backed Mermaid rendering is permitted.
func (c *Cache) EnableMMDC(enabled bool) { c.mmdc = enabled }

// MMDCEnabled reports whether external Mermaid rendering is permitted.
func (c *Cache) MMDCEnabled() bool { return c.mmdc }

// Render returns a cached rendering or renders and stores the block.
func (c *Cache) Render(source string, width int) (string, error) {
	digest := sha256.Sum256([]byte(source))
	key := fmt.Sprintf("%d:%x", width, digest)
	if value, ok := c.get(key); ok {
		return value, nil
	}

	value, err := Markdown(source, width)
	if err != nil {
		return "", err
	}
	c.put(key, value)
	return value, nil
}

func (c *Cache) get(key string) (string, bool) {
	c.mu.Lock()
	defer c.mu.Unlock()
	element, ok := c.entries[key]
	if !ok {
		return "", false
	}
	c.recency.MoveToFront(element)
	return element.Value.(cacheEntry).value, true
}

func (c *Cache) put(key, value string) {
	size := len(key) + len(value)
	if size > c.budget || c.budget == 0 {
		return
	}

	c.mu.Lock()
	defer c.mu.Unlock()
	if existing, ok := c.entries[key]; ok {
		entry := existing.Value.(cacheEntry)
		c.used -= entry.size
		delete(c.entries, key)
		c.recency.Remove(existing)
	}
	element := c.recency.PushFront(cacheEntry{key: key, value: value, size: size})
	c.entries[key] = element
	c.used += size
	for c.used > c.budget {
		oldest := c.recency.Back()
		entry := oldest.Value.(cacheEntry)
		c.used -= entry.size
		delete(c.entries, entry.key)
		c.recency.Remove(oldest)
	}
}

// Size reports cached entry count and bytes for tests and diagnostics.
func (c *Cache) Size() (entries, bytes int) {
	c.mu.Lock()
	defer c.mu.Unlock()
	return len(c.entries), c.used
}
