package main

import (
	"container/list"
	"sync"
	"time"
)

type Cache struct {
	mu         sync.Mutex
	ttl        time.Duration
	maxEntries int
	entries    map[string]*list.Element
	lru        *list.List
	stop       chan struct{}
	stopOnce   sync.Once
}

type cacheEntry struct {
	key       string
	value     any
	expiresAt time.Time
}

func NewCache(ttl time.Duration, maxEntries int, cleanupInterval time.Duration) *Cache {
	if maxEntries < 0 {
		maxEntries = 0
	}

	cache := &Cache{
		ttl:        ttl,
		maxEntries: maxEntries,
		entries:    map[string]*list.Element{},
		lru:        list.New(),
	}

	if cleanupInterval > 0 {
		cache.stop = make(chan struct{})
		go cache.cleanupLoop(cleanupInterval)
	}

	return cache
}

func (c *Cache) Close() {
	if c.stop == nil {
		return
	}

	c.stopOnce.Do(func() {
		close(c.stop)
	})
}

func (c *Cache) Get(key string, load func() (any, error)) (any, error) {
	if value, ok := c.Peek(key); ok {
		return value, nil
	}

	value, err := load()
	if err != nil {
		return nil, err
	}
	c.Set(key, value)
	return value, nil
}

func (c *Cache) Peek(key string) (any, bool) {
	c.mu.Lock()
	defer c.mu.Unlock()

	element, ok := c.entries[key]
	if !ok {
		return nil, false
	}

	entry := element.Value.(*cacheEntry)
	if entry.expired(time.Now()) {
		c.deleteElement(element)
		return nil, false
	}

	c.lru.MoveToFront(element)
	return entry.value, true
}

func (c *Cache) Set(key string, value any) {
	c.mu.Lock()
	defer c.mu.Unlock()

	entry := &cacheEntry{
		key:   key,
		value: value,
	}
	if c.ttl > 0 {
		entry.expiresAt = time.Now().Add(c.ttl)
	}

	if element, ok := c.entries[key]; ok {
		element.Value = entry
		c.lru.MoveToFront(element)
		return
	}

	c.entries[key] = c.lru.PushFront(entry)
	c.enforceLimit()
}

func (c *Cache) Delete(key string) {
	c.mu.Lock()
	defer c.mu.Unlock()

	if element, ok := c.entries[key]; ok {
		c.deleteElement(element)
	}
}

func (c *Cache) Len() int {
	c.mu.Lock()
	defer c.mu.Unlock()

	return len(c.entries)
}

func (c *Cache) cleanupLoop(interval time.Duration) {
	ticker := time.NewTicker(interval)
	defer ticker.Stop()

	for {
		select {
		case <-ticker.C:
			c.DeleteExpired()
		case <-c.stop:
			return
		}
	}
}

func (c *Cache) DeleteExpired() {
	now := time.Now()

	c.mu.Lock()
	defer c.mu.Unlock()

	for element := c.lru.Back(); element != nil; {
		previous := element.Prev()
		entry := element.Value.(*cacheEntry)
		if entry.expired(now) {
			c.deleteElement(element)
		}
		element = previous
	}
}

func (c *Cache) enforceLimit() {
	if c.maxEntries == 0 {
		for element := c.lru.Back(); element != nil; {
			previous := element.Prev()
			c.deleteElement(element)
			element = previous
		}
		return
	}

	for len(c.entries) > c.maxEntries {
		c.deleteElement(c.lru.Back())
	}
}

func (c *Cache) deleteElement(element *list.Element) {
	if element == nil {
		return
	}

	entry := element.Value.(*cacheEntry)
	delete(c.entries, entry.key)
	c.lru.Remove(element)
}

func (e *cacheEntry) expired(now time.Time) bool {
	return !e.expiresAt.IsZero() && now.After(e.expiresAt)
}
