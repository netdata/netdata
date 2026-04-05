//go:build unix

// L3: Client-side cgroups snapshot cache (POSIX).

package raw

import (
	"time"

	"github.com/netdata/netdata/go/plugins/pkg/netipc/transport/posix"
)

// cacheBucket is one open-addressing bucket for hash+name lookup.
type cacheBucket struct {
	index int
	used  bool
}

func cacheHashName(name string) uint32 {
	h := uint32(5381)
	for i := 0; i < len(name); i++ {
		h = ((h << 5) + h) + uint32(name[i])
	}
	return h
}

// Cache is an L3 client-side cgroups snapshot cache.
type Cache struct {
	client *Client
	items  []CacheItem
	// Open-addressing hash table: (hash ^ djb2(name)) -> index into items slice.
	buckets []cacheBucket

	systemdEnabled      uint32
	generation          uint64
	populated           bool
	refreshSuccessCount uint32
	refreshFailureCount uint32
	epoch               time.Time // monotonic reference point
	lastRefreshTs       int64     // elapsed ms since epoch
}

// NewCache creates a new L3 cache. Creates the underlying L2 client
// context. Does NOT connect. Does NOT require the server to be running.
// Cache starts empty (populated == false).
func NewCache(runDir, serviceName string, config posix.ClientConfig) *Cache {
	return &Cache{
		client: NewSnapshotClient(runDir, serviceName, config),
		epoch:  time.Now(),
	}
}

// Refresh drives the L2 client (connect/reconnect as needed) and
// requests a fresh snapshot. On success, rebuilds the local cache.
// On failure, preserves the previous cache.
//
// Returns true if the cache was updated.
func (c *Cache) Refresh() bool {
	c.client.Refresh()

	view, err := c.client.CallSnapshot()
	if err != nil {
		c.refreshFailureCount++
		return false
	}

	newItems := make([]CacheItem, 0, view.ItemCount)
	for i := uint32(0); i < view.ItemCount; i++ {
		iv, ierr := view.Item(i)
		if ierr != nil {
			c.refreshFailureCount++
			return false
		}
		newItems = append(newItems, CacheItem{
			Hash:    iv.Hash,
			Options: iv.Options,
			Enabled: iv.Enabled,
			Name:    iv.Name.String(),
			Path:    iv.Path.String(),
		})
	}

	// Rebuild open-addressing lookup table.
	var buckets []cacheBucket
	if len(newItems) > 0 {
		bcount := nextPowerOf2U32(uint32(len(newItems)) * 2)
		buckets = make([]cacheBucket, bcount)
		mask := bcount - 1
		for i := range newItems {
			slot := (newItems[i].Hash ^ cacheHashName(newItems[i].Name)) & mask
			for buckets[slot].used {
				slot = (slot + 1) & mask
			}
			buckets[slot].index = i
			buckets[slot].used = true
		}
	}

	c.items = newItems
	c.buckets = buckets
	c.systemdEnabled = view.SystemdEnabled
	c.generation = view.Generation
	c.populated = true
	c.refreshSuccessCount++
	c.lastRefreshTs = time.Since(c.epoch).Milliseconds()

	return true
}

// Ready returns true if at least one successful refresh has occurred.
func (c *Cache) Ready() bool {
	return c.populated
}

// Lookup finds a cached item by hash + name. O(1) via open-addressing hash
// table. No I/O.
func (c *Cache) Lookup(hash uint32, name string) (CacheItem, bool) {
	if !c.populated {
		return CacheItem{}, false
	}

	if len(c.buckets) > 0 {
		mask := uint32(len(c.buckets) - 1)
		slot := (hash ^ cacheHashName(name)) & mask
		for c.buckets[slot].used {
			item := &c.items[c.buckets[slot].index]
			if item.Hash == hash && item.Name == name {
				return *item, true
			}
			slot = (slot + 1) & mask
		}
		return CacheItem{}, false
	}

	for i := range c.items {
		if c.items[i].Hash == hash && c.items[i].Name == name {
			return c.items[i], true
		}
	}
	return CacheItem{}, false
}

// Status returns a diagnostic snapshot for the L3 cache.
func (c *Cache) Status() CacheStatus {
	return CacheStatus{
		Populated:           c.populated,
		ItemCount:           uint32(len(c.items)),
		SystemdEnabled:      c.systemdEnabled,
		Generation:          c.generation,
		RefreshSuccessCount: c.refreshSuccessCount,
		RefreshFailureCount: c.refreshFailureCount,
		ConnectionState:     c.client.state,
		LastRefreshTs:       c.lastRefreshTs,
	}
}

// Close frees all cached items and closes the L2 client.
func (c *Cache) Close() {
	c.items = nil
	c.buckets = nil
	c.populated = false
	c.client.Close()
}
