// SPDX-License-Identifier: GPL-3.0-or-later

package ddsnmpcollector

import (
	"fmt"
	"runtime"
	"slices"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/netdata/netdata/go/plugins/logger"
	"github.com/netdata/netdata/go/plugins/plugin/go.d/collector/snmp/ddsnmp"
	"github.com/netdata/netdata/go/plugins/plugin/go.d/collector/snmp/ddsnmp/ddprofiledefinition"
)

func TestTableCollectionSession_CanonicalTableRoutes(t *testing.T) {
	tests := map[string]struct {
		ownerOIDs      []string
		auxiliaryOID   string
		expectedRoutes int
		expectedAuxOID string
		missingOwner   bool
	}{
		"unique symbol owner absorbs descendant auxiliary": {
			ownerOIDs:      []string{"1.2.3"},
			auxiliaryOID:   "1.2.3.4",
			expectedRoutes: 1,
			expectedAuxOID: "1.2.3",
		},
		"ambiguous symbol owners remain separate": {
			ownerOIDs:      []string{"1.2.3", "1.2.4"},
			auxiliaryOID:   "1.2.3.4",
			expectedRoutes: 3,
			expectedAuxOID: "1.2.3.4",
		},
		"numeric prefix lookalike remains separate": {
			ownerOIDs:      []string{"1.2.3"},
			auxiliaryOID:   "1.2.30",
			expectedRoutes: 2,
			expectedAuxOID: "1.2.30",
		},
		"missing symbol owner still owns immutable identity": {
			ownerOIDs:      []string{"1.2.3"},
			auxiliaryOID:   "1.2.3.4",
			expectedRoutes: 1,
			expectedAuxOID: "1.2.3",
			missingOwner:   true,
		},
	}

	for name, test := range tests {
		t.Run(name, func(t *testing.T) {
			missingOIDs := make(map[string]bool)
			if test.missingOwner {
				missingOIDs[test.ownerOIDs[0]] = true
			}
			collector := newTableCollector(nil, missingOIDs, newTableCache(time.Hour, 0), logger.New(), false)

			var profiles []*ddsnmp.Profile
			for i, oid := range test.ownerOIDs {
				profiles = append(profiles, createTestProfile(fmt.Sprintf("owner-%d.yaml", i), []ddprofiledefinition.MetricsConfig{{
					Table:   ddprofiledefinition.SymbolConfig{OID: oid, Name: "sharedTable"},
					Symbols: []ddprofiledefinition.SymbolConfig{{OID: oid + ".1", Name: fmt.Sprintf("value%d", i)}},
				}}))
			}

			auxiliaryProfile := createTestProfile("auxiliary.yaml", []ddprofiledefinition.MetricsConfig{{
				Table: ddprofiledefinition.SymbolConfig{OID: test.auxiliaryOID, Name: "sharedTable"},
			}})
			session := newTableCollectionSession(collector, buildTableIdentity(append(slices.Clone(profiles), auxiliaryProfile)))
			for _, profile := range profiles {
				session.addScope(profile, tableSymbolModeValue, &ddsnmp.CollectionStats{})
			}
			auxiliaryScope := session.addScope(auxiliaryProfile, tableSymbolModeValue, &ddsnmp.CollectionStats{})
			session.buildRoutes()

			require.Len(t, session.routes, test.expectedRoutes)
			require.Len(t, auxiliaryScope.requests, 1)
			assert.Equal(t, test.expectedAuxOID, auxiliaryScope.requests[0].route.oid)
			assert.Equal(t, test.expectedAuxOID, auxiliaryScope.tableNameToOID["sharedTable"])
		})
	}
}

// This isolates dependency-free cached route planning: O(configs +
// routes*log(routes)) time and O(configs + routes) collection-local memory.
// Dependency settlement additionally scales with the active graph edges.
func BenchmarkTableCollectorCachedAuxiliaryRoutingScaling(b *testing.B) {
	for _, configCount := range []int{16, 256, 4096} {
		b.Run(fmt.Sprintf("configs=%d", configCount), func(b *testing.B) {
			configs := make([]ddprofiledefinition.MetricsConfig, 0, configCount)
			cache := newTableCache(time.Hour, 0)
			for i := range configCount {
				cfg := ddprofiledefinition.MetricsConfig{
					Table: ddprofiledefinition.SymbolConfig{
						OID:  fmt.Sprintf("1.3.6.1.4.1.99999.%d", i+1),
						Name: fmt.Sprintf("auxiliaryTable%d", i+1),
					},
				}
				configs = append(configs, cfg)
				cache.cacheMarker(cfg)
			}

			profile := createTestProfile("cached-auxiliary-scaling.yaml", configs)
			collector := newTableCollector(nil, make(map[string]bool), cache, logger.New(), false)
			identity := buildTableIdentity([]*ddsnmp.Profile{profile})

			b.ReportAllocs()
			b.ResetTimer()
			for range b.N {
				var stats ddsnmp.CollectionStats
				session := newTableCollectionSession(collector, identity)
				scope := session.addScope(profile, tableSymbolModeValue, &stats)
				session.resolve()
				metrics, err := session.collectScope(scope)
				if err != nil {
					b.Fatal(err)
				}
				if len(metrics) != 0 {
					b.Fatalf("expected no metrics, got %d", len(metrics))
				}
			}
			runtime.KeepAlive(collector)
		})
	}
}

func BenchmarkTableCollectionSessionDisabledCacheRollback(b *testing.B) {
	routes := make([]*tableCollectionRoute, 4096)
	for i := range routes {
		routes[i] = &tableCollectionRoute{
			oid:   fmt.Sprintf("1.3.6.1.4.1.99999.%d", i+1),
			state: tableRouteFresh,
		}
	}
	collector := newTableCollector(nil, make(map[string]bool), newTableCache(0, 0), logger.New(), false)
	session := newTableCollectionSession(collector, nil)

	b.ReportAllocs()
	b.ResetTimer()
	for range b.N {
		session.discardSettledRouteCaches(routes)
	}
	runtime.KeepAlive(session)
}
