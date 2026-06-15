package raw

import "testing"

func TestCacheDirectFallbackAndControlsCommon(t *testing.T) {
	cache := newCache(&Client{state: StateReady, abortCh: make(chan struct{})})
	cache.populated = true
	cache.items = []CacheItem{
		{Hash: 100, Name: "alpha", Path: "/alpha"},
		{Hash: 200, Name: "beta", Path: "/beta"},
	}

	item, found := cache.Lookup(200, "beta")
	if !found || item.Path != "/beta" {
		t.Fatalf("fallback lookup = %+v found %v", item, found)
	}
	if _, found := cache.Lookup(200, "missing"); found {
		t.Fatal("fallback lookup should reject wrong name")
	}
	if _, found := cache.Lookup(999, "beta"); found {
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
	if cache.Ready() || cache.items != nil || cache.buckets != nil {
		t.Fatalf("closed cache retained state: ready=%v items=%v buckets=%v", cache.Ready(), cache.items, cache.buckets)
	}
}
