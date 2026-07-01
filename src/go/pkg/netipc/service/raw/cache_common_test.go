package raw

import "testing"

func cacheDupForTest(cache *Cache, hash uint32, name string) (CacheItem, bool) {
	guard := cache.ReadLock()
	defer guard.Unlock()

	view := guard.Get(hash, name)
	if view == nil {
		return CacheItem{}, false
	}
	return guard.Dup(view), true
}

func cacheHasForTest(cache *Cache, hash uint32, name string) bool {
	_, found := cacheDupForTest(cache, hash, name)
	return found
}

func cacheSetLinearSnapshotForTest(cache *Cache, items []CacheItem) {
	views := make([]CacheItemView, len(items))
	for i := range items {
		views[i] = CacheItemView{
			Hash:    items[i].Hash,
			Options: items[i].Options,
			Enabled: items[i].Enabled,
			Name:    items[i].Name,
			Path:    items[i].Path,
		}
	}
	cache.snapshot = &cacheSnapshot{
		items: items,
		views: views,
	}
}

func TestCacheDirectFallbackAndControlsCommon(t *testing.T) {
	cache := newCache(&Client{state: StateReady, abortCh: make(chan struct{})})
	cacheSetLinearSnapshotForTest(cache, []CacheItem{
		{Hash: 100, Name: "alpha", Path: "/alpha"},
		{Hash: 200, Name: "beta", Path: "/beta"},
	})

	item, found := cacheDupForTest(cache, 200, "beta")
	if !found || item.Path != "/beta" {
		t.Fatalf("fallback lookup = %+v found %v", item, found)
	}
	if cacheHasForTest(cache, 200, "missing") {
		t.Fatal("fallback lookup should reject wrong name")
	}
	if cacheHasForTest(cache, 999, "beta") {
		t.Fatal("fallback lookup should reject wrong hash")
	}

	cache.SetCallTimeout(123)
	if cache.client.callTimeoutMs != 123 {
		t.Fatalf("cache call timeout = %d, want 123", cache.client.callTimeoutMs)
	}
	cache.Abort()
	if !cache.client.abortRequested.Load() {
		t.Fatal("cache abort did not mark client aborted")
	}
	cache.ClearAbort()
	if cache.client.abortRequested.Load() {
		t.Fatal("cache clear abort did not reset client abort state")
	}
	cache.Close()
	if cache.Ready() || cache.snapshot != nil {
		t.Fatalf("closed cache retained state: ready=%v snapshot=%v", cache.Ready(), cache.snapshot)
	}
}

func TestCacheGuardAndSeedForTestsCommon(t *testing.T) {
	var nilView *CacheItemView
	if got := nilView.Dup(); got != (CacheItem{}) {
		t.Fatalf("nil view dup = %+v, want zero item", got)
	}

	var nilGuard *CacheReadGuard
	nilGuard.Unlock()
	if got := nilGuard.Get(1, "missing"); got != nil {
		t.Fatalf("nil guard get = %+v, want nil", got)
	}
	if got := nilGuard.Dup(&CacheItemView{Hash: 1, Name: "x"}); got != (CacheItem{}) {
		t.Fatalf("nil guard dup = %+v, want zero item", got)
	}

	guard := CacheReadGuard{}
	guard.Unlock()
	if got := guard.Get(1, "missing"); got != nil {
		t.Fatalf("unlocked guard get = %+v, want nil", got)
	}
	if got := guard.Dup(&CacheItemView{Hash: 1, Name: "x"}); got != (CacheItem{}) {
		t.Fatalf("unlocked guard dup = %+v, want zero item", got)
	}

	cache := newCache(&Client{state: StateReady, abortCh: make(chan struct{})})
	if !cache.SeedForTests(nil, 7, 9) {
		t.Fatal("empty seed should succeed")
	}
	status := cache.Status()
	if !status.Populated || status.ItemCount != 0 ||
		status.SystemdEnabled != 7 || status.Generation != 9 ||
		status.RefreshSuccessCount != 1 {
		t.Fatalf("empty seed status = %+v", status)
	}

	seed := []CacheItem{
		{Hash: 10, Enabled: 1, Name: "alpha", Path: "/alpha"},
		{Hash: 20, Enabled: 0, Name: "beta", Path: "/beta"},
	}
	if !cache.SeedForTests(seed, 3, 11) {
		t.Fatal("seed should succeed")
	}
	seed[0].Path = "/mutated"

	read := cache.ReadLock()
	view := read.Get(10, "alpha")
	if view == nil {
		t.Fatal("seeded item should be found")
	}
	owned := read.Dup(view)
	if owned.Path != "/alpha" {
		t.Fatalf("owned seeded item path = %q, want /alpha", owned.Path)
	}
	if got := read.Dup(nil); got != (CacheItem{}) {
		t.Fatalf("guard dup nil view = %+v, want zero item", got)
	}
	read.Unlock()
	read.Unlock()
	if got := read.Get(10, "alpha"); got != nil {
		t.Fatalf("unlocked read guard get = %+v, want nil", got)
	}

	status = cache.Status()
	if status.ItemCount != 2 || status.SystemdEnabled != 3 ||
		status.Generation != 11 || status.RefreshSuccessCount != 2 {
		t.Fatalf("seed status = %+v", status)
	}
}
