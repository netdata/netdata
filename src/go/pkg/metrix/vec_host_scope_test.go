// SPDX-License-Identifier: GPL-3.0-or-later

package metrix

import (
	"strconv"
	"sync"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestVecHostScopeRefreshAcrossCycles(t *testing.T) {
	for _, stateful := range []bool{false, true} {
		t.Run(map[bool]string{false: "snapshot gauge", true: "stateful counter"}[stateful], func(t *testing.T) {
			store := NewCollectorStore()
			cc := cycleController(t, store)
			var write func(HostScope, string)
			if stateful {
				v := store.Write().StatefulMeter("site").Vec("wan").Counter("requests")
				write = func(s HostScope, id string) { v.WithHostScope(s).WithLabelValues(id).Add(1) }
			} else {
				v := store.Write().SnapshotMeter("site").Vec("wan").Gauge("rtt")
				write = func(s HostScope, id string) { v.WithHostScope(s).WithLabelValues(id).Observe(1) }
			}
			a := HostScope{
				ScopeKey: "site",
				GUID:     "guid",
				Hostname: "branch",
				Labels:   map[string]string{"pop": "A"},
			}
			b := HostScope{
				ScopeKey: "site",
				GUID:     "guid",
				Hostname: "renamed",
				Labels:   map[string]string{"pop": "B"},
			}
			for _, s := range []HostScope{a, b, a} {
				cc.BeginCycle()
				write(s, "wan1")
				write(s, "wan2")
				require.NoError(t, cc.CommitCycleSuccess())
				require.Equal(t, []HostScope{s}, store.Read().HostScopes())
			}
			if stateful {
				mustValue(t, store.Read(ReadHostScope("site")), "site.requests", Labels{
					"wan": "wan1",
				}, 3)
			}
		})
	}
}

func TestVecHostScopeRefreshWithNewSeries(t *testing.T) {
	store := NewCollectorStore()
	cc := cycleController(t, store)
	v := store.Write().SnapshotMeter("site").Vec("wan").Gauge("rtt")
	a := HostScope{
		ScopeKey: "site",
		GUID:     "guid",
		Hostname: "branch",
		Labels:   map[string]string{"pop": "A"},
	}
	b := HostScope{
		ScopeKey: "site",
		GUID:     "guid",
		Hostname: "branch",
		Labels:   map[string]string{"pop": "B"},
	}
	cc.BeginCycle()
	v.WithHostScope(a).WithLabelValues("wan1").Observe(1)
	require.NoError(t, cc.CommitCycleSuccess())
	cc.BeginCycle()
	v.WithHostScope(b).WithLabelValues("wan1").Observe(2)
	v.WithHostScope(b).WithLabelValues("wan2").Observe(3)
	require.NoError(t, cc.CommitCycleSuccess())
	require.Equal(t, []HostScope{b}, store.Read().HostScopes())
	mustValue(t, store.Read(ReadHostScope("site")), "site.rtt", Labels{
		"wan": "wan1",
	}, 2)
	mustValue(t, store.Read(ReadHostScope("site")), "site.rtt", Labels{
		"wan": "wan2",
	}, 3)
}

func TestVecHostScopeHandleOwnership(t *testing.T) {
	store := NewCollectorStore()
	cc := cycleController(t, store)
	v := store.Write().SnapshotMeter("site").Vec("wan").Gauge("rtt").(*snapshotGaugeVec)
	s := HostScope{
		ScopeKey: "site",
		GUID:     "guid",
		Hostname: "branch",
		Labels:   map[string]string{"pop": "A"},
	}
	old := v.WithHostScope(s).WithLabelValues("wan1")
	require.Same(t, old, v.WithHostScope(s).WithLabelValues("wan1"))
	for i := range 20 {
		s.Labels["pop"] = strconv.Itoa(i)
		current := v.WithHostScope(s).WithLabelValues("wan1")
		require.NotSame(t, old, current)
		require.Same(t, current, v.WithHostScope(s).WithLabelValues("wan1"))
		require.Len(t, v.cache.cache, 1, "metadata revisions must replace, not grow cache keys")
	}
	require.Equal(t, "A", old.(*snapshotGaugeInstrument).scope.Labels["pop"])
	cc.BeginCycle()
	old.Observe(1)
	v.WithHostScope(s).WithLabelValues("wan2").Observe(2)
	require.ErrorIs(t, cc.CommitCycleSuccess(), ErrHostScopeConflict, "mixed metadata remains invalid")
}

func TestVecHostScopeConcurrentLookups(t *testing.T) {
	v := NewCollectorStore().Write().SnapshotMeter("site").Vec("wan").Gauge("rtt").(*snapshotGaugeVec)
	var wg sync.WaitGroup
	for i := range 8 {
		wg.Go(func() {
			scope := HostScope{
				ScopeKey: "site",
				GUID:     "guid",
				Hostname: "branch",
				Labels:   map[string]string{"revision": strconv.Itoa(i)},
			}
			for range 100 {
				h := v.WithHostScope(scope).WithLabelValues("wan1").(*snapshotGaugeInstrument)
				if !hostScopeEqual(h.scope, scope) {
					t.Errorf("lookup returned a different caller's scope")
				}
			}
		})
	}
	wg.Wait()
	require.Len(t, v.cache.cache, 1)
}

func BenchmarkVecScopedLookup(b *testing.B) {
	v := NewCollectorStore().Write().SnapshotMeter("site").Vec("wan").Gauge("rtt")
	scope := HostScope{
		ScopeKey: "site",
		GUID:     "guid",
		Hostname: "branch",
		Labels:   map[string]string{"pop": "A"},
	}
	scoped := v.WithHostScope(scope)
	scoped.WithLabelValues("wan1")
	b.ReportAllocs()
	b.ResetTimer()
	for b.Loop() {
		scoped.WithLabelValues("wan1")
	}
}
