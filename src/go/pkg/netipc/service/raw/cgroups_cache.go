package raw

import (
	"time"

	"github.com/netdata/netdata/go/plugins/pkg/netipc/protocol"
)

// Default response buffer size for L3 cache refresh.
const cacheResponseBufSize = 65536

// CacheItem is an owned copy of a single cgroup item.
// Built from ephemeral L2 views during cache construction.
type CacheItem struct {
	Hash    uint32
	Options uint32
	Enabled uint32
	Name    string // owned copy
	Path    string // owned copy
}

// CacheStatus is a diagnostic snapshot for the L3 cache.
type CacheStatus struct {
	Populated           bool
	ItemCount           uint32
	SystemdEnabled      uint32
	Generation          uint64
	RefreshSuccessCount uint32
	RefreshFailureCount uint32
	ConnectionState     ClientState // underlying L2 client state
	LastRefreshTs       int64       // monotonic timestamp (ms) of last successful refresh, 0 if never
}

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

func newCache(client *Client) *Cache {
	return &Cache{
		client: client,
		epoch:  time.Now(),
	}
}

// Refresh drives the L2 client and requests a fresh snapshot. On success,
// it rebuilds the local cache. On failure, it preserves the previous cache.
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

	var buckets []cacheBucket
	if len(newItems) > 0 {
		itemCount, err := checkedLookupU32(len(newItems))
		if err != nil {
			c.refreshFailureCount++
			return false
		}
		bcount, err := cacheBucketCountForItemCount(itemCount)
		if err != nil {
			c.refreshFailureCount++
			return false
		}
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

func cacheBucketCountForItemCount(itemCount uint32) (uint32, error) {
	if itemCount == 0 {
		return 0, nil
	}
	if itemCount > 1<<30 {
		return 0, protocol.ErrOverflow
	}
	bcount := nextPowerOf2U32(itemCount * 2)
	if bcount == 0 || uint64(bcount) > uint64(int(^uint(0)>>1)) {
		return 0, protocol.ErrOverflow
	}
	return bcount, nil
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
		mask, err := checkedLookupU32(len(c.buckets) - 1)
		if err != nil {
			return CacheItem{}, false
		}
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
	itemCount, err := checkedLookupU32(len(c.items))
	if err != nil {
		itemCount = ^uint32(0)
	}
	return CacheStatus{
		Populated:           c.populated,
		ItemCount:           itemCount,
		SystemdEnabled:      c.systemdEnabled,
		Generation:          c.generation,
		RefreshSuccessCount: c.refreshSuccessCount,
		RefreshFailureCount: c.refreshFailureCount,
		ConnectionState:     c.client.state,
		LastRefreshTs:       c.lastRefreshTs,
	}
}

// SetCallTimeout sets the context-level default timeout for refresh calls.
func (c *Cache) SetCallTimeout(timeoutMs uint32) {
	c.client.SetCallTimeout(timeoutMs)
}

// Abort unblocks an in-flight refresh call.
func (c *Cache) Abort() {
	c.client.Abort()
}

// ClearAbort clears a previous abort request so the cache can be reused.
func (c *Cache) ClearAbort() {
	c.client.ClearAbort()
}

// Close frees all cached items and closes the L2 client.
func (c *Cache) Close() {
	c.items = nil
	c.buckets = nil
	c.populated = false
	c.client.Close()
}
