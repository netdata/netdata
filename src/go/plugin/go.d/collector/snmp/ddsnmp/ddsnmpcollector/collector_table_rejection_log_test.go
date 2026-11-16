// SPDX-License-Identifier: GPL-3.0-or-later

package ddsnmpcollector

import (
	"bytes"
	"fmt"
	"io"
	"strings"
	"testing"
	"time"

	"github.com/gosnmp/gosnmp"
	"github.com/stretchr/testify/require"

	"github.com/netdata/netdata/go/plugins/logger"
	"github.com/netdata/netdata/go/plugins/plugin/go.d/collector/snmp/ddsnmp"
)

func TestTableRejectedRowsBoundedLoggingAndRecovery(t *testing.T) {
	const root = "1.3.6.1.4.1.99999.882"
	const badRows = 4096
	def := decodeContractProfile(
		t,
		"metrics:\n- table: {OID: "+root+", name: counters}\n  symbols: [{OID: "+root+".1, name: count}]\n  metric_tags:\n  - tag: name\n    symbol: {format: octet_string}\n    index_decode: {components: [{type: octet_string}, {type: integer}], component: 1}\n",
	)
	for name, tc := range map[string]struct {
		ttl          time.Duration
		goodNeighbor bool
	}{
		"all rejected cached table":    {ttl: time.Hour},
		"healthy neighbor fresh table": {goodNeighbor: true},
	} {
		t.Run(name, func(t *testing.T) {
			ctrl, handler := setupMockHandler(t)
			defer ctrl.Finish()
			var output bytes.Buffer
			cache := newTableCache(tc.ttl, 0)
			collector := newTableCollector(handler, make(map[string]bool), cache, logger.NewWithWriter(&output), false)
			profile := &ddsnmp.Profile{
				SourceFile: "custom.yaml",
				Definition: def,
			}
			pdus := make([]gosnmp.SnmpPDU, badRows)
			for i := range pdus {
				// Length consumes the integer, or an octet is outside its legal range.
				suffix := fmt.Sprintf("3.65.66.%d", i)
				if i%2 != 0 {
					suffix = fmt.Sprintf("1.256.%d", i)
				}
				pdus[i] = createGauge32PDU(root+".1."+suffix, 10)
			}
			if tc.goodNeighbor {
				pdus = append(pdus, createGauge32PDU(root+".1.1.71.7", 20))
			}
			firstLogBytes := 0
			for cycle := 0; cycle < 2; cycle++ {
				expectSNMPWalk(handler, gosnmp.Version2c, root, pdus)
				var stats ddsnmp.CollectionStats
				metrics, err := collector.collect(profile, &stats)
				require.NoError(t, err)
				require.EqualValues(t, badRows, stats.Errors.Processing.Table)
				require.EqualValues(t, len(pdus), stats.Metrics.Rows)
				if tc.goodNeighbor {
					require.Len(t, metrics, 1)
					require.Equal(t, "G", metrics[0].Tags["name"])
					require.EqualValues(t, 20, metrics[0].Value)
				} else {
					require.Empty(t, metrics)
					_, _, cached := cache.getCachedData(def.Metrics[0])
					require.False(t, cached)
				}
				require.Equal(t, 1, strings.Count(output.String(), "failed to collect 4096 rows from table"))
				require.Less(t, output.Len(), 2048, "summary must not serialize every rejected row")
				if cycle == 0 {
					firstLogBytes = output.Len()
				} else {
					require.Equal(t, firstLogBytes, output.Len(), "same configuration must be limited across cycles")
				}
			}
			// Recovery is observed through a fresh WALK, with no logger or cache reset.
			expectSNMPWalk(handler, gosnmp.Version2c, root, []gosnmp.SnmpPDU{createGauge32PDU(root+".1.1.82.7", 30)})
			var stats ddsnmp.CollectionStats
			metrics, err := collector.collect(profile, &stats)
			require.NoError(t, err)
			require.Len(t, metrics, 1)
			require.Equal(t, "R", metrics[0].Tags["name"])
			require.EqualValues(t, 30, metrics[0].Value)
			require.Zero(t, stats.Errors.Processing.Table)
			require.Equal(t, firstLogBytes, output.Len())

			// A different configuration owns an independent diagnostic budget on the same table.
			changed := def.Clone()
			changed.Metrics[0].MetricTags[0].Tag = "other_name"
			expectSNMPWalk(handler, gosnmp.Version2c, root, pdus)
			_, err = collector.collect(
				&ddsnmp.Profile{
					SourceFile: "custom.yaml",
					Definition: changed,
				},
				&ddsnmp.CollectionStats{},
			)
			require.NoError(t, err)
			require.Equal(t, 2, strings.Count(output.String(), "failed to collect 4096 rows from table"))
			require.Less(t, output.Len(), 4096)
		})
	}
}

func TestTableRejectionDiagnosticsIdentifyEachProfile(t *testing.T) {
	const base = "1.3.6.1.4.1.99999.986"
	var profiles []*ddsnmp.Profile
	for i, source := range []string{"alpha.yaml", "beta.yaml"} {
		// Same named metric over distinct readable columns survives production deduplication.
		raw := fmt.Sprintf("metrics:\n- table: {OID: %[1]s, name: source}\n  symbols: [{OID: %[1]s.%[2]d, name: counter}]\n  metric_tags:\n  - tag: name\n    index_decode: {components: [{type: integer}], component: 1}\n", base, i+1)
		profiles = append(profiles, &ddsnmp.Profile{SourceFile: source, Definition: decodeContractProfile(t, raw)})
	}
	profiles = ddsnmp.FinalizeProfiles(profiles)
	require.Len(t, profiles, 2)
	for _, profile := range profiles {
		require.Len(t, profile.Definition.Metrics, 1)
	}
	handler := &sourceTestHandler{walk: func(string) ([]gosnmp.SnmpPDU, error) {
		return []gosnmp.SnmpPDU{createGauge32PDU(base+".1.7.8", 10), createGauge32PDU(base+".2.7.8", 20)}, nil
	}}
	var output bytes.Buffer
	collector := New(Config{SnmpClient: handler, Profiles: profiles, Log: logger.NewWithWriter(&output)})
	firstLogBytes := 0
	for cycle := 0; cycle < 2; cycle++ {
		result, err := collector.Collect()
		require.NoError(t, err)
		require.Len(t, result, 2)
		for _, profile := range result {
			require.Empty(t, profile.Metrics)
			require.EqualValues(t, 1, profile.Stats.Errors.Processing.Table)
		}
		require.Equal(t, 2, strings.Count(output.String(), "failed to collect 1 rows"), "each profile owns its warning budget")
		require.Contains(t, output.String(), "alpha.yaml")
		require.Contains(t, output.String(), "beta.yaml")
		require.Contains(t, output.String(), "first error at row")
		require.Contains(t, output.String(), "7.8")
		if cycle == 0 {
			firstLogBytes = output.Len()
		} else {
			require.Equal(t, firstLogBytes, output.Len(), "repeat failures remain rate limited")
		}
	}
}

// The diagnostic envelope is O(1) retained errors, independent of the rejected row population.
// Time remains O(rows); ns/op is a local trend indicator rather than a test gate.
func BenchmarkTableRejectedRowSummary(b *testing.B) {
	const root = "1.3.6.1.4.1.99999.882"
	def := decodeContractProfile(
		b,
		"metrics:\n- table: {OID: "+root+", name: counters}\n  symbols: [{OID: "+root+".1, name: count}]\n  metric_tags:\n  - tag: name\n    symbol: {format: octet_string}\n    index_decode: {components: [{type: octet_string}, {type: integer}], component: 1}\n",
	)
	for _, count := range []int{1, 4096} {
		b.Run(fmt.Sprint(count), func(b *testing.B) {
			cfg := def.Metrics[0]
			ctx := &tableProcessingContext{
				config:      cfg,
				columnOIDs:  buildColumnOIDs(cfg),
				orderedTags: buildOrderedTags(cfg),
				rows:        make(map[string]map[string]gosnmp.SnmpPDU, count),
			}
			for i := 0; i < count; i++ {
				index := fmt.Sprintf("3.65.66.%d", i)
				ctx.rows[index] = map[string]gosnmp.SnmpPDU{root + ".1": createGauge32PDU(root+".1."+index, 10)}
			}
			collector := newTableCollector(
				nil,
				nil,
				newTableCache(time.Hour, 0),
				logger.NewWithWriter(io.Discard),
				false,
			)
			var stats ddsnmp.CollectionStats
			b.ReportAllocs()
			b.ResetTimer()
			for i := 0; i < b.N; i++ {
				if _, err := collector.processRows(ctx, &stats); err != nil {
					b.Fatal(err)
				}
			}
		})
	}
}

// Cached value failures retain one contextual diagnostic, regardless of row count.
func BenchmarkCachedTableProcessingErrors(b *testing.B) {
	const root = "1.3.6.1.4.1.99999.995"
	def := decodeContractProfile(
		b,
		"metrics:\n- table: {OID: "+root+", name: counters}\n  symbols: [{OID: "+root+".1, name: count}]\n  metric_tags: [{tag: row, index: 1}]\n",
	)
	for _, count := range []int{1, 4096} {
		b.Run(fmt.Sprint(count), func(b *testing.B) {
			cfg := def.Metrics[0]
			ctx := &cacheProcessingContext{
				config:        cfg,
				columnOIDs:    buildColumnOIDs(cfg),
				tableName:     cfg.Table.Name,
				profileSource: "custom.yaml",
				cachedOIDs:    make(map[string]map[string]string, count),
				cachedTags:    make(map[string]map[string]string, count),
				pdus:          make(map[string]gosnmp.SnmpPDU, count),
			}
			for i := 0; i < count; i++ {
				index := fmt.Sprint(i + 1)
				oid := root + ".1." + index
				ctx.cachedOIDs[index] = map[string]string{root + ".1": oid}
				ctx.cachedTags[index] = map[string]string{"row": index}
				ctx.pdus[oid] = createStringPDU(oid, "not-a-number")
			}
			collector := newTableCollector(
				nil,
				nil,
				newTableCache(time.Hour, 0),
				logger.NewWithWriter(io.Discard),
				false,
			)
			var stats ddsnmp.CollectionStats
			b.ReportAllocs()
			b.ResetTimer()
			for i := 0; i < b.N; i++ {
				if _, err := collector.buildMetricsFromCache(ctx, &stats); err != nil {
					b.Fatal(err)
				}
			}
		})
	}
}
