// SPDX-License-Identifier: GPL-3.0-or-later

package catalog

import (
	"os"
	"path/filepath"
	"sort"
	"sync"
	"testing"
)

func BenchmarkLoadedTrapLookup(b *testing.B) {
	manager := benchmarkStockManager()
	lease := benchmarkAcquire(b, manager)
	oid := benchmarkRepresentativeStockRoutes(b, lease.Epoch())[0].oid
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
	routes := benchmarkRepresentativeStockRoutes(b, seed.Epoch())
	seed.Close()

	for _, route := range routes {
		b.Run(route.label, func(b *testing.B) {
			b.ReportAllocs()
			b.ResetTimer()
			for range b.N {
				b.StopTimer()
				lease := benchmarkAcquire(b, manager)
				b.StartTimer()

				td, err := lease.Epoch().LookupWithError(route.oid)

				b.StopTimer()
				if err != nil || td == nil {
					b.Fatalf("hydrate stock profile: trap=%v err=%v", td, err)
				}
				lease.Close()
				b.StartTimer()
			}
		})
	}
}

func BenchmarkSameStockProfileHydrationCoalescing(b *testing.B) {
	const callers = 8
	manager := benchmarkStockManager()
	seed := benchmarkAcquire(b, manager)
	oid := benchmarkRepresentativeStockRoutes(b, seed.Epoch())[1].oid
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
			wg.Go(func() {
				_, err := lease.Epoch().LookupWithError(oid)
				errs <- err
			})
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
	routes := benchmarkRepresentativeStockRoutes(b, seed.Epoch())
	seed.Close()
	oids := make([]string, 0, len(routes))
	for _, route := range routes {
		oids = append(oids, route.oid)
	}

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
				wg.Go(func() {
					_, err := lease.Epoch().LookupWithError(oid)
					errs <- err
				})
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

type benchmarkStockRoute struct {
	label   string
	profile string
	oid     string
	size    int64
}

func benchmarkRepresentativeStockRoutes(b *testing.B, idx *Epoch) []benchmarkStockRoute {
	b.Helper()
	oidsByProfile := make(map[string]string, len(idx.stock.files))
	for oid, profile := range idx.stock.exactRoutes {
		if current := oidsByProfile[profile]; current == "" || oid < current {
			oidsByProfile[profile] = oid
		}
	}

	routes := make([]benchmarkStockRoute, 0, len(oidsByProfile))
	for profile, oid := range oidsByProfile {
		file := idx.stock.files[profile]
		info, err := os.Stat(file.path)
		if err != nil {
			b.Fatalf("stat stock profile %q: %v", file.path, err)
		}
		routes = append(routes, benchmarkStockRoute{profile: profile, oid: oid, size: info.Size()})
	}
	sort.Slice(routes, func(i, j int) bool {
		if routes[i].size == routes[j].size {
			return routes[i].profile < routes[j].profile
		}
		return routes[i].size < routes[j].size
	})
	if len(routes) < 3 {
		b.Fatalf("stock catalog has %d routed profiles; need at least 3", len(routes))
	}

	selected := []benchmarkStockRoute{
		routes[(len(routes)-1)*50/100],
		routes[(len(routes)-1)*90/100],
		routes[len(routes)-1],
	}
	selected[0].label = "p50"
	selected[1].label = "p90"
	selected[2].label = "largest"
	return selected
}
