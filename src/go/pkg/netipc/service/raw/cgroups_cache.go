package raw

import (
	"sync"
	"time"

	"github.com/netdata/netdata/go/plugins/pkg/netipc/protocol"
)

// Default response buffer size for L3 cache refresh.
const cacheResponseBufSize = 65536

// CacheItem is an owned copy of a single cgroup item.
type CacheItem struct {
	Hash    uint32
	Options uint32
	Enabled uint32
	Name    string
	Path    string
}

// CacheItemView is a borrowed immutable view into a cache snapshot.
//
// The view is valid only while the CacheReadGuard that returned it remains
// locked.
type CacheItemView struct {
	Hash    uint32
	Options uint32
	Enabled uint32
	Name    string
	Path    string
}

// Dup duplicates a borrowed view into an owned item that survives unlock.
func (v *CacheItemView) Dup() CacheItem {
	if v == nil {
		return CacheItem{}
	}
	return CacheItem{
		Hash:    v.Hash,
		Options: v.Options,
		Enabled: v.Enabled,
		Name:    v.Name,
		Path:    v.Path,
	}
}

// CacheStatus is a diagnostic snapshot for the L3 cache.
type CacheStatus struct {
	Populated           bool
	ItemCount           uint32
	SystemdEnabled      uint32
	Generation          uint64
	RefreshSuccessCount uint32
	RefreshFailureCount uint32
	ConnectionState     ClientState
	LastRefreshTs       int64
}

type cacheBucket struct {
	index int
	used  bool
}

type cacheSnapshot struct {
	items          []CacheItem
	views          []CacheItemView
	buckets        []cacheBucket
	systemdEnabled uint32
	generation     uint64
}

func cacheHashName(name string) uint32 {
	h := uint32(5381)
	for i := 0; i < len(name); i++ {
		h = ((h << 5) + h) + uint32(name[i])
	}
	return h
}

func buildCacheSnapshot(items []CacheItem, systemdEnabled uint32, generation uint64) (*cacheSnapshot, error) {
	copiedItems := append([]CacheItem(nil), items...)
	snapshot := &cacheSnapshot{
		items:          copiedItems,
		views:          make([]CacheItemView, len(copiedItems)),
		systemdEnabled: systemdEnabled,
		generation:     generation,
	}

	for i := range copiedItems {
		snapshot.views[i] = CacheItemView{
			Hash:    copiedItems[i].Hash,
			Options: copiedItems[i].Options,
			Enabled: copiedItems[i].Enabled,
			Name:    copiedItems[i].Name,
			Path:    copiedItems[i].Path,
		}
	}

	if len(copiedItems) == 0 {
		return snapshot, nil
	}

	itemCount, err := checkedLookupU32(len(copiedItems))
	if err != nil {
		return nil, err
	}
	bcount, err := cacheBucketCountForItemCount(itemCount)
	if err != nil {
		return nil, err
	}

	buckets := make([]cacheBucket, bcount)
	mask := bcount - 1
	for i := range copiedItems {
		slot := (copiedItems[i].Hash ^ cacheHashName(copiedItems[i].Name)) & mask
		for buckets[slot].used {
			slot = (slot + 1) & mask
		}
		buckets[slot].index = i
		buckets[slot].used = true
	}
	snapshot.buckets = buckets
	return snapshot, nil
}

func (s *cacheSnapshot) get(hash uint32, name string) *CacheItemView {
	if s == nil {
		return nil
	}

	if len(s.buckets) > 0 {
		mask, err := cacheBucketMaskForLen(len(s.buckets))
		if err != nil {
			return nil
		}
		slot := (hash ^ cacheHashName(name)) & mask
		for s.buckets[slot].used {
			idx := s.buckets[slot].index
			if s.views[idx].Hash == hash && s.views[idx].Name == name {
				return &s.views[idx]
			}
			slot = (slot + 1) & mask
		}
		return nil
	}

	for i := range s.views {
		if s.views[i].Hash == hash && s.views[i].Name == name {
			return &s.views[i]
		}
	}
	return nil
}

// Cache is an L3 client-side cgroups snapshot cache.
type Cache struct {
	writerMu sync.Mutex
	mu       sync.RWMutex

	client   *Client
	snapshot *cacheSnapshot

	refreshSuccessCount uint32
	refreshFailureCount uint32
	epoch               time.Time
	lastRefreshTs       int64
}

func newCache(client *Client) *Cache {
	return &Cache{
		client: client,
		epoch:  time.Now(),
	}
}

// Refresh drives the L2 client and requests a fresh snapshot. On success,
// it swaps in a new immutable snapshot. On failure, it preserves the previous
// snapshot.
func (c *Cache) Refresh() bool {
	c.writerMu.Lock()
	defer c.writerMu.Unlock()

	c.client.Refresh()
	view, err := c.client.CallSnapshot()
	if err != nil {
		c.mu.Lock()
		c.refreshFailureCount++
		c.mu.Unlock()
		return false
	}

	newItems := make([]CacheItem, 0, view.ItemCount)
	for i := uint32(0); i < view.ItemCount; i++ {
		iv, ierr := view.Item(i)
		if ierr != nil {
			c.mu.Lock()
			c.refreshFailureCount++
			c.mu.Unlock()
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

	snapshot, err := buildCacheSnapshot(newItems, view.SystemdEnabled, view.Generation)
	if err != nil {
		c.mu.Lock()
		c.refreshFailureCount++
		c.mu.Unlock()
		return false
	}

	c.mu.Lock()
	c.snapshot = snapshot
	c.refreshSuccessCount++
	c.lastRefreshTs = time.Since(c.epoch).Milliseconds()
	c.mu.Unlock()

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
	c.mu.RLock()
	ready := c.snapshot != nil
	c.mu.RUnlock()
	return ready
}

// CacheReadGuard protects borrowed cache views.
type CacheReadGuard struct {
	cache    *Cache
	snapshot *cacheSnapshot
	locked   bool
}

// ReadLock acquires a read guard for borrowed cache access.
func (c *Cache) ReadLock() CacheReadGuard {
	c.mu.RLock()
	return CacheReadGuard{
		cache:    c,
		snapshot: c.snapshot,
		locked:   true,
	}
}

// Unlock releases a read guard.
func (g *CacheReadGuard) Unlock() {
	if g == nil || !g.locked {
		return
	}
	cache := g.cache
	g.cache = nil
	g.snapshot = nil
	g.locked = false
	cache.mu.RUnlock()
}

// Get looks up a borrowed item view by hash + name. No I/O.
func (g *CacheReadGuard) Get(hash uint32, name string) *CacheItemView {
	if g == nil || !g.locked {
		return nil
	}
	return g.snapshot.get(hash, name)
}

// Dup duplicates a borrowed view into an owned item that survives unlock.
func (g *CacheReadGuard) Dup(view *CacheItemView) CacheItem {
	if g == nil || !g.locked || view == nil {
		return CacheItem{}
	}
	return view.Dup()
}

// SeedForTests publishes a synthetic immutable snapshot through the cache
// write lock. It is for repository tests and benchmarks.
func (c *Cache) SeedForTests(items []CacheItem, systemdEnabled uint32, generation uint64) bool {
	snapshot, err := buildCacheSnapshot(items, systemdEnabled, generation)
	if err != nil {
		return false
	}

	c.writerMu.Lock()
	defer c.writerMu.Unlock()

	c.mu.Lock()
	c.snapshot = snapshot
	c.refreshSuccessCount++
	c.lastRefreshTs = time.Since(c.epoch).Milliseconds()
	c.mu.Unlock()
	return true
}

// Status returns a diagnostic snapshot for the L3 cache.
func (c *Cache) Status() CacheStatus {
	c.writerMu.Lock()
	connectionState := c.client.state
	c.writerMu.Unlock()

	c.mu.RLock()
	defer c.mu.RUnlock()

	status := CacheStatus{
		RefreshSuccessCount: c.refreshSuccessCount,
		RefreshFailureCount: c.refreshFailureCount,
		ConnectionState:     connectionState,
		LastRefreshTs:       c.lastRefreshTs,
	}
	if c.snapshot != nil {
		status.Populated = true
		status.ItemCount = cacheStatusItemCountForLen(len(c.snapshot.items))
		status.SystemdEnabled = c.snapshot.systemdEnabled
		status.Generation = c.snapshot.generation
	}
	return status
}

func cacheBucketMaskForLen(bucketLen int) (uint32, error) {
	return checkedLookupU32(bucketLen - 1)
}

func cacheStatusItemCountForLen(itemLen int) uint32 {
	itemCount, err := checkedLookupU32(itemLen)
	if err != nil {
		return ^uint32(0)
	}
	return itemCount
}

// SetCallTimeout sets the context-level default timeout for refresh calls.
func (c *Cache) SetCallTimeout(timeoutMs uint32) {
	c.writerMu.Lock()
	c.client.SetCallTimeout(timeoutMs)
	c.writerMu.Unlock()
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
	c.writerMu.Lock()
	defer c.writerMu.Unlock()

	c.mu.Lock()
	c.snapshot = nil
	c.mu.Unlock()

	c.client.Close()
}
