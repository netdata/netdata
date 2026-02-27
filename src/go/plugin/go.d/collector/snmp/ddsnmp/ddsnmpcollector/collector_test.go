// SPDX-License-Identifier: GPL-3.0-or-later

package ddsnmpcollector

import (
	"testing"

	"github.com/gosnmp/gosnmp"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/netdata/netdata/go/plugins/logger"
	"github.com/netdata/netdata/go/plugins/plugin/go.d/collector/snmp/ddsnmp"
	"github.com/netdata/netdata/go/plugins/plugin/go.d/collector/snmp/ddsnmp/ddprofiledefinition"
)

func TestCollector_Collect_StatsSnapshot(t *testing.T) {
	ctrl, mockHandler := setupMockHandler(t)
	defer ctrl.Finish()

	// --- SNMP expectations ---------------------------------------------------

	// Scalar: sysUpTime.0
	expectSNMPGet(mockHandler,
		[]string{"1.3.6.1.2.1.1.3.0"},
		[]gosnmp.SnmpPDU{
			createTimeTicksPDU("1.3.6.1.2.1.1.3.0", 123456),
		},
	)

	// Table: ifTable, we only care about ifInOctets with 2 rows
	expectSNMPWalk(mockHandler,
		gosnmp.Version2c,
		"1.3.6.1.2.1.2.2",
		[]gosnmp.SnmpPDU{
			// Row 1
			createCounter32PDU("1.3.6.1.2.1.2.2.1.10.1", 1000), // ifInOctets.1
			// Row 2
			createCounter32PDU("1.3.6.1.2.1.2.2.1.10.2", 2000), // ifInOctets.2
		},
	)

	// --- Profile definition --------------------------------------------------

	profile := &ddsnmp.Profile{
		SourceFile: "stats-toy-profile.yaml",
		Definition: &ddprofiledefinition.ProfileDefinition{
			Metrics: []ddprofiledefinition.MetricsConfig{
				// Simple scalar metric
				{
					Symbol: ddprofiledefinition.SymbolConfig{
						OID:  "1.3.6.1.2.1.1.3.0",
						Name: "sysUpTime",
					},
				},
				// Simple table metric: ifInOctets over ifTable
				{
					Table: ddprofiledefinition.SymbolConfig{
						OID:  "1.3.6.1.2.1.2.2",
						Name: "ifTable",
					},
					Symbols: []ddprofiledefinition.SymbolConfig{
						{
							OID:  "1.3.6.1.2.1.2.2.1.10",
							Name: "ifInOctets",
						},
					},
				},
			},
			// One virtual metric that sums ifInOctets across the table.
			VirtualMetrics: []ddprofiledefinition.VirtualMetricConfig{
				{
					Name: "ifInOctets_total",
					Sources: []ddprofiledefinition.VirtualMetricSourceConfig{
						{
							Metric: "ifInOctets",
							Table:  "ifTable",
						},
					},
				},
			},
		},
	}

	handleCrossTableTagsWithoutMetrics(profile)
	require.NoError(t, ddsnmp.CompileTransforms(profile))

	collector := New(Config{
		SnmpClient:  mockHandler,
		Profiles:    []*ddsnmp.Profile{profile},
		Log:         logger.New(),
		SysObjectID: "",
	})

	// --- Run collection ------------------------------------------------------

	results, err := collector.Collect()
	require.NoError(t, err)
	require.Len(t, results, 1)

	pm := results[0]

	// --- Sanity check on actual metrics -------------------------------------

	// We expect:
	//   - 1 scalar metric (sysUpTime)
	//   - 2 table metrics (ifInOctets for 2 rows)
	//   - 1 virtual metric (ifInOctets_total)
	require.Len(t, pm.Metrics, 4, "total number of metrics")

	// --- Assert CollectionStats as a snapshot -------------------------------

	// Ignore timing (it's inherently variable).
	stats := pm.Stats
	stats.Timing = ddsnmp.TimingStats{}
	pm.Stats = stats

	expected := ddsnmp.CollectionStats{
		SNMP: ddsnmp.SNMPOperationStats{
			// Scalar: 1 GET with 1 OID
			GetRequests: 1,
			GetOIDs:     1,

			// Table: 1 WALK with 2 PDUs, 1 table walked, no cached tables
			WalkRequests: 1,
			WalkPDUs:     2,
			TablesWalked: 1,
			// TablesCached should be 0 on first run
		},
		Metrics: ddsnmp.MetricCountStats{
			Scalar:  1, // sysUpTime
			Table:   2, // ifInOctets.1, ifInOctets.2
			Virtual: 1, // ifInOctets_total
			Tables:  1, // ifTable
			Rows:    2, // 2 interfaces
		},
		TableCache: ddsnmp.TableCacheStats{
			Hits:   0, // first run → no cache hits
			Misses: 1, // one table config had to be walked
			// Expired intentionally ignored / omitted
		},
		Errors: ddsnmp.ErrorStats{
			SNMP:        0,
			MissingOIDs: 0,
		},
		// Timing left as zero-value for comparison
		Timing: ddsnmp.TimingStats{},
	}

	assert.Equal(t, expected, pm.Stats)
}
