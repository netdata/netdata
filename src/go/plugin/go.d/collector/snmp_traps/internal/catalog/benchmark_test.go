// SPDX-License-Identifier: GPL-3.0-or-later

package catalog

import (
	"path/filepath"
	"sort"
	"sync"
	"testing"
)

func BenchmarkLoadedTrapLookup(b *testing.B) {
	manager := benchmarkStockManager()
	lease := benchmarkAcquire(b, manager)
	oid := benchmarkStockOIDs(b, lease.Epoch(), 1)[0]
	if td, err := lease.Epoch().LookupWithError(oid); err != nil || td == nil {
		b.Fatalf("hydrate stock profile: trap=%v err=%v", td, err)
	}
	b.Cleanup(lease.Close)

	b.ReportAllocs()
	b.ResetTimer()
	for range b.N {
		if lease.Epoch().Lookup(oid) == nil {
			b.Fatal("loaded trap lookup returned nil")
		}
	}
}

func BenchmarkFirstStockProfileHydration(b *testing.B) {
	manager := benchmarkStockManager()
	seed := benchmarkAcquire(b, manager)
	oid := benchmarkStockOIDs(b, seed.Epoch(), 1)[0]
	seed.Close()

	b.ReportAllocs()
	b.ResetTimer()
	for range b.N {
		b.StopTimer()
		lease := benchmarkAcquire(b, manager)
		b.StartTimer()

		td, err := lease.Epoch().LookupWithError(oid)

		b.StopTimer()
		if err != nil || td == nil {
			b.Fatalf("hydrate stock profile: trap=%v err=%v", td, err)
		}
		lease.Close()
		b.StartTimer()
	}
}

func BenchmarkSameStockProfileHydrationCoalescing(b *testing.B) {
	const callers = 8
	manager := benchmarkStockManager()
	seed := benchmarkAcquire(b, manager)
	oid := benchmarkStockOIDs(b, seed.Epoch(), 1)[0]
	seed.Close()

	b.ReportAllocs()
	b.ReportMetric(callers, "callers/op")
	b.ResetTimer()
	for range b.N {
		b.StopTimer()
		lease := benchmarkAcquire(b, manager)
		b.StartTimer()

		errs := make(chan error, callers)
		var wg sync.WaitGroup
		for range callers {
			wg.Add(1)
			go func() {
				defer wg.Done()
				_, err := lease.Epoch().LookupWithError(oid)
				errs <- err
			}()
		}
		wg.Wait()

		b.StopTimer()
		close(errs)
		for err := range errs {
			if err != nil {
				b.Fatalf("coalesced hydration: %v", err)
			}
		}
		lease.Close()
		b.StartTimer()
	}
}

func BenchmarkUnrelatedStockProfileHydration(b *testing.B) {
	manager := benchmarkStockManager()
	seed := benchmarkAcquire(b, manager)
	oids := benchmarkStockOIDs(b, seed.Epoch(), 2)
	seed.Close()

	b.Run("sequential", func(b *testing.B) {
		b.ReportAllocs()
		benchmarkHydrateProfiles(b, manager, oids, false)
	})
	b.Run("parallel", func(b *testing.B) {
		b.ReportAllocs()
		benchmarkHydrateProfiles(b, manager, oids, true)
	})
}

func benchmarkHydrateProfiles(b *testing.B, manager *Manager, oids []string, parallel bool) {
	b.Helper()
	b.ResetTimer()
	for range b.N {
		b.StopTimer()
		lease := benchmarkAcquire(b, manager)
		b.StartTimer()

		errs := make(chan error, len(oids))
		if parallel {
			var wg sync.WaitGroup
			for _, oid := range oids {
				wg.Add(1)
				go func() {
					defer wg.Done()
					_, err := lease.Epoch().LookupWithError(oid)
					errs <- err
				}()
			}
			wg.Wait()
		} else {
			for _, oid := range oids {
				_, err := lease.Epoch().LookupWithError(oid)
				errs <- err
			}
		}

		b.StopTimer()
		close(errs)
		for err := range errs {
			if err != nil {
				b.Fatalf("hydrate unrelated profile: %v", err)
			}
		}
		lease.Close()
		b.StartTimer()
	}
}

func benchmarkStockManager() *Manager {
	return NewManager(Paths{StockDir: filepath.Clean("../../../../config/go.d/snmp.trap-profiles/default")})
}

func benchmarkAcquire(b *testing.B, manager *Manager) *Lease {
	b.Helper()
	lease, err := manager.Acquire()
	if err != nil {
		b.Fatalf("acquire stock catalog: %v", err)
	}
	return lease
}

func benchmarkStockOIDs(b *testing.B, idx *Epoch, count int) []string {
	b.Helper()
	type route struct {
		profile string
		oid     string
	}
	routes := make([]route, 0, len(idx.stock.exactRoutes))
	for oid, profile := range idx.stock.exactRoutes {
		routes = append(routes, route{profile: profile, oid: oid})
	}
	sort.Slice(routes, func(i, j int) bool {
		if routes[i].profile == routes[j].profile {
			return routes[i].oid < routes[j].oid
		}
		return routes[i].profile < routes[j].profile
	})

	oids := make([]string, 0, count)
	previousProfile := ""
	for _, route := range routes {
		if route.profile == previousProfile {
			continue
		}
		oids = append(oids, route.oid)
		previousProfile = route.profile
		if len(oids) == count {
			return oids
		}
	}
	b.Fatalf("stock catalog has %d distinct routed profiles; need %d", len(oids), count)
	return nil
}
