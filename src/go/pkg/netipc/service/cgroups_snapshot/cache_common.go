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

// ReadLock acquires a read guard for borrowed cache access.
func (c *Cache) ReadLock() CacheReadGuard {
	return c.inner.ReadLock()
}

// Status returns a diagnostic snapshot for the L3 cache.
func (c *Cache) Status() CacheStatus {
	return c.inner.Status()
}

// SetCallTimeout sets the context-level default timeout for refresh calls.
func (c *Cache) SetCallTimeout(timeoutMs uint32) {
	c.inner.SetCallTimeout(timeoutMs)
}

// Abort unblocks an in-flight refresh call.
func (c *Cache) Abort() {
	c.inner.Abort()
}

// ClearAbort clears a previous abort request so the cache can be reused.
func (c *Cache) ClearAbort() {
	c.inner.ClearAbort()
}

// Close frees all cached items and closes the L2 client.
func (c *Cache) Close() {
	c.inner.Close()
}
