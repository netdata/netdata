// SPDX-License-Identifier: GPL-3.0-or-later

package ddsnmpcollector

import (
	"context"
	"fmt"
	"io"
	"testing"
	"time"

	"github.com/netdata/gosnmp"
	"github.com/stretchr/testify/require"

	"github.com/netdata/netdata/go/plugins/logger"
	"github.com/netdata/netdata/go/plugins/plugin/go.d/collector/snmp/ddsnmp"
)

func TestLookupFormatChangeInvalidatesCachedIdentity(t *testing.T) {
	const base = "1.3.6.1.4.1.99999.989"
	profile := func(format string) *ddsnmp.Profile {
		raw := fmt.Sprintf(`metrics:
- table: {OID: %[1]s.1, name: source}
  symbols: [{OID: %[1]s.1.1, name: counter}]
  metric_tags:
  - tag: name
    table: metadata
    symbol: {OID: %[1]s.2.2, name: display}
    lookup_symbol: {OID: %[1]s.2.1, name: key, format: %[2]q}
`, base, format)
		prof := &ddsnmp.Profile{SourceFile: "lookup.yaml", Definition: decodeContractProfile(t, raw)}
		ddsnmp.HandleCrossTableTagsWithoutMetrics(prof)
		return prof
	}
	handler := &sourceTestHandler{
		walk: func(oid string) ([]gosnmp.VarBind, error) {
			switch oid {
			case base + ".1":
				return []gosnmp.VarBind{createGauge32PDU(base+".1.1.65", 10)}, nil
			case base + ".2":
				return []gosnmp.VarBind{createStringPDU(base+".2.1.9", "A"), createStringPDU(base+".2.2.9", "joined")}, nil
			default:
				t.Fatalf("unexpected walk %s", oid)
				return nil, nil
			}
		},
		get: func(oids []string) (*gosnmp.Message, error) {
			require.Equal(t, []string{base + ".1.1.65"}, oids)
			return &gosnmp.Message{VarBinds: []gosnmp.VarBind{createGauge32PDU(base+".1.1.65", 11)}}, nil
		},
	}
	cached := newTableCollector(handler, requestConfig{version: gosnmp.Version2c, maxOIDs: 1}, 0, make(map[string]bool), newTableCache(time.Hour, 0), logger.NewWithWriter(io.Discard), false)
	first, err := cached.collect(context.Background(), profile(""), &ddsnmp.CollectionStats{})
	require.NoError(t, err)
	require.Len(t, first, 1)
	require.Empty(t, first[0].Tags)
	stats := &ddsnmp.CollectionStats{}
	changed, err := cached.collect(context.Background(), profile("octet_string"), stats)
	require.NoError(t, err)
	require.Len(t, changed, 1)
	fresh := newTableCollector(handler, requestConfig{version: gosnmp.Version2c, maxOIDs: 1}, 0, make(map[string]bool), newTableCache(0, 0), logger.NewWithWriter(io.Discard), false)
	reference, err := fresh.collect(context.Background(), profile("octet_string"), &ddsnmp.CollectionStats{})
	require.NoError(t, err)
	require.Len(t, reference, 1)
	require.Equal(t, "joined", reference[0].Tags["name"])
	require.Equal(t, reference[0].Tags, changed[0].Tags, "new lookup format must not reuse tags derived by the previous format")
	require.Zero(t, stats.TableCache.Hits, "changed lookup semantics require fresh acquisition")
	reloaded := profile("octet_string")
	reloaded.Definition = reloaded.Definition.Clone()
	stats = &ddsnmp.CollectionStats{}
	repeated, err := cached.collect(context.Background(), reloaded, stats)
	require.NoError(t, err)
	require.Len(t, repeated, 1)
	require.Equal(t, reference[0].Tags, repeated[0].Tags)
	require.EqualValues(t, 11, repeated[0].Value)
	require.Positive(t, stats.TableCache.Hits, "unchanged cloned configuration reuses the corrected identity")
}

func TestConcurrentLookupFormatsRetainCachedIdentity(t *testing.T) {
	const base = "1.3.6.1.4.1.99999.989"
	def := decodeContractProfile(t, fmt.Sprintf(`metrics:
- table: {OID: %[1]s.1, name: source}
  symbols: [{OID: %[1]s.1.1, name: counter}]
  metric_tags:
  - tag: name
    table: metadata
    symbol: {OID: %[1]s.2.2, name: display}
    lookup_symbol: {OID: %[1]s.2.1, name: key}
- table: {OID: %[1]s.1, name: source}
  symbols: [{OID: %[1]s.1.1, name: counter}]
  metric_tags:
  - tag: name
    table: metadata
    symbol: {OID: %[1]s.2.2, name: display}
    lookup_symbol: {OID: %[1]s.2.1, name: key, format: octet_string}
`, base))
	prof := &ddsnmp.Profile{SourceFile: "concurrent.yaml", Definition: def}
	ddsnmp.FinalizeProfiles([]*ddsnmp.Profile{prof})
	require.Len(t, prof.Definition.Metrics, 3)
	handler := &sourceTestHandler{
		walk: func(oid string) ([]gosnmp.VarBind, error) {
			switch oid {
			case base + ".1":
				return []gosnmp.VarBind{createGauge32PDU(base+".1.1.65", 10)}, nil
			case base + ".2":
				return []gosnmp.VarBind{createStringPDU(base+".2.1.9", "A"), createStringPDU(base+".2.2.9", "joined")}, nil
			default:
				t.Fatal(oid)
				return nil, nil
			}
		},
		get: func(oids []string) (*gosnmp.Message, error) {
			require.Equal(t, []string{base + ".1.1.65"}, oids)
			return &gosnmp.Message{VarBinds: []gosnmp.VarBind{createGauge32PDU(base+".1.1.65", 11)}}, nil
		},
	}
	collector := New(Config{Version: gosnmp.Version2c, MaxOIDs: 1, SnmpClient: handler, Profiles: []*ddsnmp.Profile{prof}, Log: logger.NewWithWriter(io.Discard)})
	for cycle := 0; cycle < 2; cycle++ {
		results, err := collector.Collect(context.Background())
		require.NoError(t, err)
		require.Len(t, results, 1)
		require.Len(t, results[0].Metrics, 2)
		var names []string
		for _, metric := range results[0].Metrics {
			names = append(names, metric.Tags["name"])
		}
		if cycle > 0 {
			require.Positive(t, results[0].Stats.TableCache.Hits)
		}
		require.ElementsMatch(t, []string{"", "joined"}, names, "fixed single-profile format variants must remain distinct through cached GET")
	}
}
