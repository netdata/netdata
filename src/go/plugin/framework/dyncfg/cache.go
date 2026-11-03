// SPDX-License-Identifier: GPL-3.0-or-later

package dyncfg

import "sync"

// Config is the constraint interface for configs stored in handler caches.
type Config interface {
	UID() string        // SeenCache key (globally unique per source)
	ExposedKey() string // ExposedCache key (one per logical name)
	SourceType() string // "dyncfg", "user", "stock"
	Source() string     // source identifier
	Hash() uint64       // content hash for change detection
}

// Entry pairs a config with its current dyncfg status.
type Entry[C Config] struct {
	Cfg     C
	Status  Status
	Enabled bool // committed enabled intent; Accepted alone may be passive
}

// SeenCache stores all discovered configs keyed by UID().
// Thread-safe: SD dispatches read-only commands concurrently.
type SeenCache[C Config] struct {
	mux   sync.RWMutex
	items map[string]C
}

func NewSeenCache[C Config]() *SeenCache[C] {
	return &SeenCache[C]{
		items: make(map[string]C),
	}
}

func (c *SeenCache[C]) Add(cfg C) {
	c.mux.Lock()
	defer c.mux.Unlock()
	c.items[cfg.UID()] = cfg
}

func (c *SeenCache[C]) Remove(cfg C) {
	c.mux.Lock()
	defer c.mux.Unlock()
	delete(c.items, cfg.UID())
}

func (c *SeenCache[C]) Lookup(cfg C) (C, bool) {
	c.mux.RLock()
	defer c.mux.RUnlock()
	v, ok := c.items[cfg.UID()]
	return v, ok
}

// ForEach iterates over all entries. Return false to stop iteration.
func (c *SeenCache[C]) ForEach(fn func(uid string, cfg C) bool) {
	c.mux.RLock()
	defer c.mux.RUnlock()
	for uid, cfg := range c.items {
		if !fn(uid, cfg) {
			return
		}
	}
}

// ExposedCache stores active config+status per logical name, keyed by ExposedKey().
// Entries are immutable after insertion. Writers replace snapshots; concurrent
// preparation and read-only commands may retain their pointers.
type ExposedCache[C Config] struct {
	mux   sync.RWMutex
	items map[string]*Entry[C]
}

func NewExposedCache[C Config]() *ExposedCache[C] {
	return &ExposedCache[C]{
		items: make(map[string]*Entry[C]),
	}
}

// Add inserts or overwrites an entry by cfg.ExposedKey().
func (c *ExposedCache[C]) Add(entry *Entry[C]) {
	c.mux.Lock()
	defer c.mux.Unlock()
	c.items[entry.Cfg.ExposedKey()] = entry
}

func (c *ExposedCache[C]) Remove(cfg C) {
	c.mux.Lock()
	defer c.mux.Unlock()
	delete(c.items, cfg.ExposedKey())
}

// LookupByKey returns a pointer to the stored entry.
func (c *ExposedCache[C]) LookupByKey(key string) (*Entry[C], bool) {
	c.mux.RLock()
	defer c.mux.RUnlock()
	v, ok := c.items[key]
	return v, ok
}
