package cgroups_snapshot

import (
	raw "github.com/netdata/netdata/go/plugins/pkg/netipc/service/raw"
)

// Cache is the public L3 client-side cgroups snapshot cache.
type Cache struct {
	inner *raw.Cache
}

// Refresh drives the L2 client and requests a fresh snapshot.
func (c *Cache) Refresh() bool {
	return c.inner.Refresh()
}

// Ready returns true if at least one successful refresh has occurred.
func (c *Cache) Ready() bool {
	return c.inner.Ready()
}

// Lookup finds a cached item by hash + name. O(1), no I/O.
func (c *Cache) Lookup(hash uint32, name string) (CacheItem, bool) {
	return c.inner.Lookup(hash, name)
}

// Status returns a diagnostic snapshot for the L3 cache.
func (c *Cache) Status() CacheStatus {
	return c.inner.Status()
}

// Close frees all cached items and closes the L2 client.
func (c *Cache) Close() {
	c.inner.Close()
}
