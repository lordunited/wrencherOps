// Copyright (c) 2026 Mohammadreza Masoumi <s.masoumimohammadreza@gmail.com>
// Author: Mohammadreza Masoumi, DevOps Engineer

// Package cache provides a small TTL cache with request de-duplication,
// used to avoid hammering upstream APIs on every panel page load.
package cache

import (
	"sync"
	"time"

	"golang.org/x/sync/singleflight"
)

type entry struct {
	value   any
	expires time.Time
}

type TTLCache struct {
	ttl   time.Duration
	mu    sync.Mutex
	data  map[string]entry
	group singleflight.Group
}

func New(ttl time.Duration) *TTLCache {
	return &TTLCache{ttl: ttl, data: make(map[string]entry)}
}

func (c *TTLCache) get(key string) (any, bool) {
	c.mu.Lock()
	defer c.mu.Unlock()
	e, ok := c.data[key]
	if !ok || time.Now().After(e.expires) {
		return nil, false
	}
	return e.value, true
}

func (c *TTLCache) set(key string, value any) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.data[key] = entry{value: value, expires: time.Now().Add(c.ttl)}
}

// Fetch returns the cached value for key if it hasn't expired, otherwise it
// calls fn to produce a fresh value. Concurrent callers for the same key
// during a miss share one in-flight call.
func Fetch[T any](c *TTLCache, key string, fn func() (T, error)) (T, error) {
	if v, ok := c.get(key); ok {
		return v.(T), nil
	}

	v, err, _ := c.group.Do(key, func() (any, error) {
		result, err := fn()
		if err != nil {
			return nil, err
		}
		c.set(key, result)
		return result, nil
	})
	if err != nil {
		var zero T
		return zero, err
	}
	return v.(T), nil
}
