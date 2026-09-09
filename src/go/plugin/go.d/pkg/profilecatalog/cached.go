// SPDX-License-Identifier: GPL-3.0-or-later

package profilecatalog

import (
	"sync"

	"github.com/netdata/netdata/go/plugins/pkg/executable"
)

// Cached memoizes the result of a loader for the process lifetime. It is the
// shared replacement for the per-collector default-catalog singletons: one load,
// shared across all jobs of a collector. Caching is disabled under tests
// (executable.Name == "test") so each test observes a fresh load. A successful
// load is cached; a failed load is not, so the next call retries.
//
// The exported Loader and CacheEnabled fields are test seams: a test may swap
// them and call Reset to control loading. In production they are set by
// NewCached and left alone.
type Cached[T any] struct {
	mu     sync.Mutex
	loaded bool
	value  T

	Loader       func() (T, error)
	CacheEnabled func() bool
}

// NewCached returns a Cached that loads via loader and caches outside tests.
func NewCached[T any](loader func() (T, error)) *Cached[T] {
	return &Cached[T]{
		Loader:       loader,
		CacheEnabled: func() bool { return executable.Name != "test" },
	}
}

// Get returns the cached value, loading it once on first use. When caching is
// disabled it loads on every call.
func (c *Cached[T]) Get() (T, error) {
	if !c.CacheEnabled() {
		return c.Loader()
	}

	c.mu.Lock()
	defer c.mu.Unlock()

	if c.loaded {
		return c.value, nil
	}

	v, err := c.Loader()
	if err != nil {
		var zero T
		return zero, err
	}

	c.value = v
	c.loaded = true
	return c.value, nil
}

// Reset clears the cached value. Intended for tests.
func (c *Cached[T]) Reset() {
	c.mu.Lock()
	var zero T
	c.value = zero
	c.loaded = false
	c.mu.Unlock()
}
