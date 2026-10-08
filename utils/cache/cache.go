// Package cache stores the last complete snapshot and refreshes it on demand.
package cache

import (
	"sync"
	"time"
)

// Policy controls the lifetime of a snapshot. TTL is evaluated after a
// successful refresh. A nil TTL gives new snapshots zero lifetime. Reusable can
// invalidate a snapshot before expiry when caller-specific conditions change;
// nil permits reuse until expiry. Policy functions must not modify their argument.
type Policy[T any] struct {
	TTL      func(T) time.Duration
	Reusable func(T) bool
}

// Result contains an independent copy of the snapshot. RefreshErr describes a
// failed refresh when UsedStale is true; the caller can log it and keep working.
type Result[T any] struct {
	Value      T
	UsedStale  bool
	RefreshErr error
}

// Cache serializes refreshes and retains only successful snapshots of type T.
// Create it with New and do not copy it after use.
type Cache[T any] struct {
	refreshMu      sync.Mutex
	mu             sync.RWMutex
	clone          func(T) T
	lastSuccessful *T
	expiresAt      time.Time
}

// New creates a cache for any data type. clone must copy all mutable references
// in T (maps, slices, pointers); for a value-only type it can return its argument.
func New[T any](clone func(T) T) *Cache[T] {
	if clone == nil {
		panic("cache: clone function is required")
	}
	return &Cache[T]{clone: clone}
}

// Peek returns a copy of the last successful snapshot, even after expiry.
// It never refreshes and does not wait for a refresh already in progress.
func (c *Cache[T]) Peek() (T, bool) {
	c.mu.RLock()
	value := c.lastSuccessful
	c.mu.RUnlock()
	if value == nil {
		var zero T
		return zero, false
	}
	return c.clone(*value), true
}

// GetOrRefresh reuses a valid snapshot or serializes refresh calls so
// concurrent callers share a successful refresh. A first failure is returned;
// subsequent failures return the last successful snapshot without extending
// its expiry, allowing the next call to retry. Partial values returned with an
// error are discarded. A refresh panic propagates after releasing the lock.
func (c *Cache[T]) GetOrRefresh(now time.Time, policy Policy[T], refresh func() (T, error)) (Result[T], error) {
	c.refreshMu.Lock()
	defer c.refreshMu.Unlock()
	c.mu.RLock()
	lastSuccessful, expiresAt := c.lastSuccessful, c.expiresAt
	c.mu.RUnlock()

	if lastSuccessful != nil && now.Before(expiresAt) &&
		(policy.Reusable == nil || policy.Reusable(*lastSuccessful)) {
		return Result[T]{Value: c.clone(*lastSuccessful)}, nil
	}

	value, err := refresh()
	if err != nil {
		if lastSuccessful == nil {
			return Result[T]{}, err
		}
		return Result[T]{Value: c.clone(*lastSuccessful), UsedStale: true, RefreshErr: err}, nil
	}

	stored := c.clone(value)
	var ttl time.Duration
	if policy.TTL != nil {
		ttl = policy.TTL(stored)
	}
	c.mu.Lock()
	c.lastSuccessful = &stored
	c.expiresAt = now.Add(ttl)
	c.mu.Unlock()
	return Result[T]{Value: c.clone(stored)}, nil
}
