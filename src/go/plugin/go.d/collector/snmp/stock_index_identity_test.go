// SPDX-License-Identifier: GPL-3.0-or-later

package snmp

import (
	"encoding/json"
	"os"
	"path/filepath"
	"slices"
	"strconv"
	"strings"
	"testing"

	"github.com/golang/mock/gomock"
	"github.com/gosnmp/gosnmp"
	snmpmock "github.com/gosnmp/gosnmp/mocks"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gopkg.in/yaml.v2"

	"github.com/netdata/netdata/go/plugins/logger"
	"github.com/netdata/netdata/go/plugins/plugin/go.d/collector/snmp/ddsnmp"
	"github.com/netdata/netdata/go/plugins/plugin/go.d/collector/snmp/ddsnmp/ddprofiledefinition"
	"github.com/netdata/netdata/go/plugins/plugin/go.d/collector/snmp/ddsnmp/ddsnmpcollector"
	"github.com/netdata/netdata/go/plugins/plugin/go.d/pkg/snmputils"
)

type stockIdentityFixture struct {
	Profile  string
	TableOID string `json:"table_oid"`
	Metric   string
	Rows     []struct {
		Suffix string
		Tags   map[string]string
		PDUs   []struct {
			OID   string
			Value string
		}
	}
}

type stockIdentityObservation struct {
	Context string
	Name    string
	Unit    string
	ChartID string
	Tags    map[string]string
	Labels  map[string]string
	Value   int64
}

type stockIdentityEvidence struct {
	Profile  string
	TableOID string
	Symbols  []string
	After    []stockIdentityObservation
}

// The source fixtures pin MIB-shaped indexes and expected identities. Here all
// symbols pass through acquisition, transforms and the final chart consumer.
func TestStockIndexIdentity_FinalCharts(t *testing.T) {
	data, err := os.ReadFile("ddsnmp/ddsnmpcollector/testdata/stock-index-tags.json")
	require.NoError(t, err)
	var fixtures map[string]stockIdentityFixture
	require.NoError(t, json.Unmarshal(data, &fixtures))
	require.Len(t, fixtures, 58)
	evidence := make(map[string]stockIdentityEvidence)
	for name, tc := range fixtures {
		t.Run(name, func(t *testing.T) {
			profile, err := ddsnmp.LoadProfileByName(tc.Profile)
			require.NoError(t, err)
			profile = stockIdentitySelectRow(t, profile, tc)
			ddsnmp.HandleCrossTableTagsWithoutMetrics(profile)
			var pdus []gosnmp.SnmpPDU
			var symbols []string
			for _, symbol := range profile.Definition.Metrics[0].Symbols {
				symbols = append(symbols, symbol.Name)
				for i, row := range tc.Rows {
					pdus = append(pdus, gosnmp.SnmpPDU{Name: symbol.OID + "." + row.Suffix, Type: gosnmp.Gauge32, Value: uint(100 + i)})
				}
			}
			for _, row := range tc.Rows {
				for _, pdu := range row.PDUs {
					pdus = append(pdus, gosnmp.SnmpPDU{Name: pdu.OID, Type: gosnmp.OctetString, Value: []byte(pdu.Value)})
				}
			}
			metrics := stockIdentityAcquire(t, profile, pdus)
			require.Len(t, metrics, 2*len(symbols), "every affected symbol must collect both rows")
			c := newTestSNMPCollector()
			c.sysInfo = &snmputils.SysInfo{}
			samples := make(map[string]int64)
			c.collectProfileTableMetrics(samples, metrics)
			names := make(map[string]int)
			keys := make(map[string]bool)
			for _, metric := range metrics {
				names[metric.Name]++
				key := tableMetricKey(metric)
				require.False(t, keys[key], "distinct acquired rows merged into %s", key)
				keys[key] = true
				matched := false
				for _, row := range tc.Rows {
					same := true
					for tag, value := range row.Tags {
						if metric.Tags[tag] != value {
							same = false
						}
					}
					matched = matched || same
				}
				assert.True(t, matched, "acquired symbol %s must carry a pinned identity", metric.Name)
				state := c.tableMetricCharts[key]
				require.NotNil(t, state, "acquired metric must emit a chart")
				chart := state.chart
				for tag, value := range metric.Tags {
					assert.Equal(t, value, chartLabels(chart)[strings.TrimPrefix(tag, "_")], "chart label %s", tag)
				}
				wantUnit := metric.Unit
				if wantUnit == "" {
					wantUnit = "1"
				}
				assert.Equal(t, wantUnit, chart.Units)
				if len(metric.MultiValue) == 0 {
					assert.Equal(t, metric.Value, samples[metricIDFromKey(key)])
				} else {
					for state, value := range metric.MultiValue {
						assert.Equal(t, value, samples[metricIDFromKey(key, state)])
					}
				}
			}
			for metric, count := range names {
				assert.Equal(t, 2, count, "output symbol %s must preserve both rows", metric)
			}
			assert.Len(t, c.tableMetricCharts, len(metrics))
			record := stockIdentityEvidence{Profile: tc.Profile, TableOID: tc.TableOID, Symbols: symbols, After: stockIdentityObservations(metrics, c)}
			// Historical chart IDs must come from the historical binary. Loading old
			// profiles into the current serializer does not reproduce old identities.
			evidence[name] = record
		})
	}
	if path := os.Getenv("NETDATA_SNMP_INDEX_EVIDENCE_FILE"); path != "" {
		data, err := json.MarshalIndent(evidence, "", "  ")
		require.NoError(t, err)
		require.NoError(t, os.WriteFile(path, append(data, '\n'), 0600))
	}
}

func stockIdentitySelectRow(t *testing.T, profile *ddsnmp.Profile, tc stockIdentityFixture) *ddsnmp.Profile {
	t.Helper()
	profile.Definition.Metrics = slices.DeleteFunc(profile.Definition.Metrics, func(row ddprofiledefinition.MetricsConfig) bool {
		return row.Table.OID != tc.TableOID || !slices.ContainsFunc(row.Symbols, func(symbol ddprofiledefinition.SymbolConfig) bool { return symbol.Name == tc.Metric })
	})
	require.Len(t, profile.Definition.Metrics, 1)
	profile.Definition.Metadata = nil
	profile.Definition.SysobjectIDMetadata = nil
	profile.Definition.MetricTags = nil
	profile.Definition.StaticTags = nil
	profile.Definition.Topology = nil
	profile.Definition.Licensing = nil
	profile.Definition.BGP = nil
	profile.Definition.VirtualMetrics = nil
	return profile
}

func stockIdentityAcquire(t *testing.T, profile *ddsnmp.Profile, pdus []gosnmp.SnmpPDU) []ddsnmp.Metric {
	t.Helper()
	handler := snmpmock.NewMockHandler(gomock.NewController(t))
	handler.EXPECT().Version().Return(gosnmp.Version2c).AnyTimes()
	handler.EXPECT().BulkWalkAll(gomock.Any()).DoAndReturn(func(oid string) ([]gosnmp.SnmpPDU, error) {
		var found []gosnmp.SnmpPDU
		prefix := strings.Trim(oid, ".") + "."
		for _, pdu := range pdus {
			if strings.HasPrefix(pdu.Name, prefix) {
				found = append(found, pdu)
			}
		}
		return found, nil
	}).AnyTimes()
	collector := ddsnmpcollector.New(ddsnmpcollector.Config{SnmpClient: handler, Profiles: []*ddsnmp.Profile{profile}, Log: logger.New()})
	result, err := collector.Collect()
	require.NoError(t, err)
	require.Len(t, result, 1)
	return result[0].Metrics
}

func stockIdentityObservations(metrics []ddsnmp.Metric, c *Collector) []stockIdentityObservation {
	var observations []stockIdentityObservation
	for _, metric := range metrics {
		chartID := ""
		unit := metric.Unit
		var labels map[string]string
		context := ""
		if state := c.tableMetricCharts[tableMetricKey(metric)]; state != nil {
			chart := state.chart
			chartID = chart.ID
			unit = chart.Units
			labels = chartLabels(chart)
			context = chart.Ctx
		}
		observations = append(observations, stockIdentityObservation{Context: context, Name: metric.Name, Unit: unit, ChartID: chartID, Tags: metric.Tags, Labels: labels, Value: metric.Value})
	}
	slices.SortFunc(observations, func(a, b stockIdentityObservation) int {
		if n := strings.Compare(a.Name, b.Name); n != 0 {
			return n
		}
		return strings.Compare(a.ChartID, b.ChartID)
	})
	return observations
}

func stockIdentityBaseline(t *testing.T, dir string, tc stockIdentityFixture) *ddsnmp.Profile {
	t.Helper()
	raw, err := os.ReadFile(filepath.Join(dir, tc.Profile))
	require.NoError(t, err)
	baseline := &ddsnmp.Profile{SourceFile: tc.Profile, Definition: &ddprofiledefinition.ProfileDefinition{}}
	require.NoError(t, yaml.Unmarshal(raw, baseline.Definition))
	baseline = stockIdentitySelectRow(t, baseline, tc)
	baseline.Definition.Normalize()
	require.NoError(t, ddprofiledefinition.ValidateEnrichProfile(baseline.Definition))
	require.NoError(t, ddsnmp.CompileTransforms(baseline))
	ddsnmp.FinalizeProfiles([]*ddsnmp.Profile{baseline})
	return baseline
}

// Public LibreNMS cetpower rows expose an INDEX column but return zero for two
// different output lines. The INDEX suffix remains the authoritative identity.
func TestStockIndexIdentity_PublicUPSReturnedColumns(t *testing.T) {
	capture := os.Getenv("NETDATA_SNMP_INDEX_UPS_CAPTURE")
	baselineDir := os.Getenv("NETDATA_SNMP_INDEX_BASELINE_DIR")
	if capture == "" || baselineDir == "" {
		t.Skip("set public UPS capture and baseline profile directory to replay historical identity")
	}
	tc := stockIdentityFixture{Profile: "_std-ups-mib.yaml", TableOID: "1.3.6.1.2.1.33.1.4.4", Metric: "upsOutputVoltage"}
	data, err := os.ReadFile(capture)
	require.NoError(t, err)
	var pdus, withoutIndex []gosnmp.SnmpPDU
	for _, line := range strings.Split(string(data), "\n") {
		fields := strings.SplitN(line, "|", 3)
		if len(fields) != 3 || !strings.HasPrefix(fields[0], tc.TableOID+".") {
			continue
		}
		require.Equal(t, "2", fields[1], "this public table uses INTEGER values")
		value, err := strconv.Atoi(fields[2])
		require.NoError(t, err)
		pdu := gosnmp.SnmpPDU{Name: fields[0], Type: gosnmp.Integer, Value: value}
		pdus = append(pdus, pdu)
		if !strings.HasPrefix(pdu.Name, tc.TableOID+".1.1.") {
			withoutIndex = append(withoutIndex, pdu)
		}
	}
	require.Len(t, pdus, 15)
	profile, err := ddsnmp.LoadProfileByName(tc.Profile)
	require.NoError(t, err)
	profile = stockIdentitySelectRow(t, profile, tc)
	current := stockIdentityAcquire(t, profile, pdus)
	missing := stockIdentityAcquire(t, profile, withoutIndex)
	render := func(metrics []ddsnmp.Metric) *Collector {
		c := newTestSNMPCollector()
		c.sysInfo = &snmputils.SysInfo{}
		c.collectProfileTableMetrics(make(map[string]int64), metrics)
		return c
	}
	currentCharts := render(current)
	assert.Len(t, currentCharts.tableMetricCharts, 12)
	assert.Equal(t, stockIdentityObservations(current, currentCharts), stockIdentityObservations(missing, render(missing)), "index-column availability cannot alter identity or values")
	baseline := stockIdentityBaseline(t, baselineDir, tc)
	old := stockIdentityAcquire(t, baseline, pdus)
	assert.Len(t, render(old).tableMetricCharts, 8, "historical zero-valued column merges two distinct lines")
	oldMissing := stockIdentityAcquire(t, baseline, withoutIndex)
	assert.Empty(t, render(oldMissing).tableMetricCharts, "historical profile drops all output lines when column is absent")
	for _, metric := range current {
		assert.Contains(t, []string{"1", "2", "3"}, metric.Tags["ups_output_line_index"])
	}
}
