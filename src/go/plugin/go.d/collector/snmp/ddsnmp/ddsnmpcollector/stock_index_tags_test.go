// SPDX-License-Identifier: GPL-3.0-or-later

package ddsnmpcollector

import (
	"encoding/json"
	"os"
	"slices"
	"strings"
	"testing"

	"github.com/gosnmp/gosnmp"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/netdata/netdata/go/plugins/logger"
	"github.com/netdata/netdata/go/plugins/plugin/go.d/collector/snmp/ddsnmp"
	"github.com/netdata/netdata/go/plugins/plugin/go.d/collector/snmp/ddsnmp/ddprofiledefinition"
)

// These frozen suffixes and expected labels follow the source MIB INDEX layouts,
// independently of the profile's extraction configuration. No INDEX column is returned.
func TestStockIndexTags_CollectWithoutIndexColumns(t *testing.T) {
	var fixtures map[string]struct {
		Profile       string
		TableOID      string `json:"table_oid"`
		Metric        string
		MetricOID     string   `json:"metric_oid"`
		OutputMetric  string   `json:"output_metric"`
		ForbiddenOIDs []string `json:"forbidden_oids"`
		Rows          []struct {
			Value  int64
			Suffix string
			Tags   map[string]string
			PDUs   []struct {
				OID   string
				Value string
			}
		}
	}
	data, err := os.ReadFile("testdata/stock-index-tags.json")
	require.NoError(t, err)
	require.NoError(t, json.Unmarshal(data, &fixtures))
	require.Len(t, fixtures, 58)
	for name, tc := range fixtures {
		t.Run(name, func(t *testing.T) {
			profile, err := ddsnmp.LoadProfileByName(tc.Profile)
			require.NoError(t, err)
			selected := slices.Clone(profile.Definition.Metrics)
			selected = slices.DeleteFunc(selected, func(row ddprofiledefinition.MetricsConfig) bool {
				return row.Table.OID != tc.TableOID || !slices.ContainsFunc(row.Symbols, func(s ddprofiledefinition.SymbolConfig) bool { return s.Name == tc.Metric })
			})
			require.Len(t, selected, 1)
			// Isolate a shipped row without changing its tags or index decoding.
			profile.Definition.Metrics = selected
			profile.Definition.Metadata = nil
			profile.Definition.SysobjectIDMetadata = nil
			profile.Definition.MetricTags = nil
			profile.Definition.StaticTags = nil
			profile.Definition.Topology = nil
			profile.Definition.Licensing = nil
			profile.Definition.BGP = nil
			profile.Definition.VirtualMetrics = nil
			ddsnmp.HandleCrossTableTagsWithoutMetrics(profile)
			var pdus []gosnmp.SnmpPDU
			for i, row := range tc.Rows {
				pdus = append(pdus, createGauge32PDU(tc.MetricOID+"."+row.Suffix, uint(100+i)))
				for _, pdu := range row.PDUs {
					pdus = append(pdus, createStringPDU(pdu.OID, pdu.Value))
				}
			}
			for _, pdu := range pdus {
				for _, oid := range tc.ForbiddenOIDs {
					require.False(t, strings.HasPrefix(pdu.Name, oid+"."), "fixture returned INDEX column %s", oid)
				}
			}
			ctrl, handler := setupMockHandler(t)
			defer ctrl.Finish()
			expectFixtureAgent(handler, pdus)
			collector := New(Config{SnmpClient: handler, Profiles: []*ddsnmp.Profile{profile}, Log: logger.New()})
			results, err := collector.Collect()
			require.NoError(t, err)
			require.Len(t, results, 1)
			outputName := tc.OutputMetric
			if outputName == "" {
				outputName = tc.Metric
			}
			var metrics []ddsnmp.Metric
			for _, metric := range results[0].Metrics {
				if metric.Name == outputName {
					metrics = append(metrics, metric)
				}
			}
			require.NotEqual(t, tc.Rows[0].Tags, tc.Rows[1].Tags, "fixture rows must have distinct identities")
			require.Len(t, metrics, 2, "both rows must survive without readable INDEX columns")
			for _, row := range tc.Rows {
				assert.True(t, slices.ContainsFunc(metrics, func(metric ddsnmp.Metric) bool {
					for key, value := range row.Tags {
						if metric.Tags[key] != value {
							return false
						}
					}
					return metric.Value == row.Value
				}), "missing row with labels %v; got %v", row.Tags, metrics)
			}
		})
	}
}
