// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package ddprofiledefinition

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func Test_validateEnrichVirtualMetrics(t *testing.T) {
	baseMetrics := []MetricsConfig{
		{
			Table: SymbolConfig{
				OID:  "1.3.6.1.2.1.31.1.1",
				Name: "ifXTable",
			},
			Symbols: []SymbolConfig{
				{OID: "1.3.6.1.2.1.31.1.1.1.6", Name: "ifHCInOctets"},
				{OID: "1.3.6.1.2.1.31.1.1.1.10", Name: "ifHCOutOctets"},
			},
			MetricTags: []MetricTagConfig{
				{Tag: "interface", Index: 1},
			},
		},
		{
			Table: SymbolConfig{
				OID:  "1.3.6.1.2.1.2.2",
				Name: "ifTable",
			},
			Symbols: []SymbolConfig{
				{OID: "1.3.6.1.2.1.2.2.1.14", Name: "ifInErrors"},
			},
			MetricTags: []MetricTagConfig{
				{Tag: "interface", Index: 1},
			},
		},
	}

	tests := map[string]struct {
		metrics         []MetricsConfig
		topology        []TopologyConfig
		virtualMetrics  []VirtualMetricConfig
		wantErrContains []string
	}{
		"valid grouped virtual metric": {
			metrics: baseMetrics,
			virtualMetrics: []VirtualMetricConfig{
				{
					Name:    "ifTraffic",
					PerRow:  true,
					GroupBy: []string{"interface"},
					Sources: []VirtualMetricSourceConfig{
						{Metric: "ifHCInOctets", Table: "ifXTable", As: "in"},
						{Metric: "ifHCOutOctets", Table: "ifXTable", As: "out"},
					},
					EmitTags: []VirtualMetricEmitTagConfig{
						{Tag: "interface", From: "interface"},
					},
				},
			},
		},
		"valid scalar total without table": {
			metrics: append(baseMetrics, MetricsConfig{
				Symbol: SymbolConfig{
					OID:  "1.3.6.1.4.1.2021.11.50.0",
					Name: "_ucd.ssCpuRawUser",
				},
			}),
			virtualMetrics: []VirtualMetricConfig{
				{
					Name: "ucd.cpuUsage",
					Sources: []VirtualMetricSourceConfig{
						{Metric: "_ucd.ssCpuRawUser", As: "user"},
					},
				},
			},
		},
		"valid mapped source dim": {
			metrics: append(baseMetrics, MetricsConfig{
				Table: SymbolConfig{
					OID:  "1.3.6.1.2.1.15.3",
					Name: "bgpPeerTable",
				},
				Symbols: []SymbolConfig{
					{
						OID:  "1.3.6.1.2.1.15.3.1.2",
						Name: "bgpPeerAdminStatus",
						Mapping: NewExactMapping(map[string]string{
							"1": "stop",
							"2": "start",
						}),
					},
				},
				MetricTags: []MetricTagConfig{
					{Tag: "neighbor", Index: 1},
				},
			}),
			virtualMetrics: []VirtualMetricConfig{
				{
					Name:   "bgpPeerAvailability",
					PerRow: true,
					Sources: []VirtualMetricSourceConfig{
						{Metric: "bgpPeerAdminStatus", Table: "bgpPeerTable", As: "admin_enabled", Dim: "start"},
					},
				},
			},
		},
		"valid bitmask mapped source dim": {
			metrics: append(baseMetrics, MetricsConfig{
				Table: SymbolConfig{
					OID:  "1.3.6.1.4.1.674.10892.1.1100.32",
					Name: "processorDeviceStatusTable",
				},
				Symbols: []SymbolConfig{
					{
						OID:  "1.3.6.1.4.1.674.10892.1.1100.32.1.6",
						Name: "processorDeviceStatusReading",
						Mapping: MappingConfig{
							Mode: MappingModeBitmask,
							Items: map[string]string{
								"1":   "internalError",
								"128": "processorPresent",
							},
						},
					},
				},
				MetricTags: []MetricTagConfig{
					{Tag: "processor", Index: 1},
				},
			}),
			virtualMetrics: []VirtualMetricConfig{
				{
					Name:   "processorDeviceHealth",
					PerRow: true,
					Sources: []VirtualMetricSourceConfig{
						{
							Metric: "processorDeviceStatusReading",
							Table:  "processorDeviceStatusTable",
							As:     "present",
							Dim:    "processorPresent",
						},
					},
				},
			},
		},
		"invalid mapped source dim": {
			metrics: append(baseMetrics, MetricsConfig{
				Table: SymbolConfig{
					OID:  "1.3.6.1.2.1.15.3",
					Name: "bgpPeerTable",
				},
				Symbols: []SymbolConfig{
					{
						OID:  "1.3.6.1.2.1.15.3.1.2",
						Name: "bgpPeerAdminStatus",
						Mapping: NewExactMapping(map[string]string{
							"1": "stop",
							"2": "start",
						}),
					},
				},
				MetricTags: []MetricTagConfig{
					{Tag: "neighbor", Index: 1},
				},
			}),
			virtualMetrics: []VirtualMetricConfig{
				{
					Name:   "bgpPeerAvailability",
					PerRow: true,
					Sources: []VirtualMetricSourceConfig{
						{Metric: "bgpPeerAdminStatus", Table: "bgpPeerTable", As: "admin_enabled", Dim: "running"},
					},
				},
			},
			wantErrContains: []string{
				`virtual_metrics[0].sources[0]: dim "running" is not available on metric/table "bgpPeerAdminStatus"/"bgpPeerTable" (available: start, stop)`,
			},
		},
		"dim requires multivalue source": {
			metrics: baseMetrics,
			virtualMetrics: []VirtualMetricConfig{
				{
					Name:   "ifTraffic",
					PerRow: true,
					Sources: []VirtualMetricSourceConfig{
						{Metric: "ifHCInOctets", Table: "ifXTable", As: "in", Dim: "up"},
					},
				},
			},
			wantErrContains: []string{
				`virtual_metrics[0].sources[0]: dim "up" requires a MultiValue source metric (metric/table "ifHCInOctets"/"ifXTable")`,
			},
		},
		"valid transform multivalue source dim": {
			metrics: append(baseMetrics, MetricsConfig{
				Table: SymbolConfig{
					OID:  "1.3.6.1.4.1.14988.1.1.3.100",
					Name: "mtxrHlTable",
				},
				Symbols: []SymbolConfig{
					{
						OID:  "1.3.6.1.4.1.14988.1.1.3.100.1.3",
						Name: "mtxrHlSensorState",
						Transform: `
{{- setMultivalue .Metric (i64map 0 "down" 1 "up") -}}
`,
					},
				},
				MetricTags: []MetricTagConfig{
					{Tag: "sensor", Index: 1},
				},
			}),
			virtualMetrics: []VirtualMetricConfig{
				{
					Name:   "sensorAvailability",
					PerRow: true,
					Sources: []VirtualMetricSourceConfig{
						{Metric: "mtxrHlSensorState", Table: "mtxrHlTable", As: "up", Dim: "up"},
					},
				},
			},
		},
		"invalid transform multivalue source dim": {
			metrics: append(baseMetrics, MetricsConfig{
				Table: SymbolConfig{
					OID:  "1.3.6.1.4.1.14988.1.1.3.100",
					Name: "mtxrHlTable",
				},
				Symbols: []SymbolConfig{
					{
						OID:  "1.3.6.1.4.1.14988.1.1.3.100.1.3",
						Name: "mtxrHlSensorState",
						Transform: `
{{- setMultivalue .Metric (i64map 0 "down" 1 "up") -}}
`,
					},
				},
				MetricTags: []MetricTagConfig{
					{Tag: "sensor", Index: 1},
				},
			}),
			virtualMetrics: []VirtualMetricConfig{
				{
					Name:   "sensorAvailability",
					PerRow: true,
					Sources: []VirtualMetricSourceConfig{
						{Metric: "mtxrHlSensorState", Table: "mtxrHlTable", As: "up", Dim: "idle"},
					},
				},
			},
			wantErrContains: []string{
				`virtual_metrics[0].sources[0]: dim "idle" is not available on metric/table "mtxrHlSensorState"/"mtxrHlTable" (available: down, up)`,
			},
		},
		"missing name and sources": {
			metrics: baseMetrics,
			virtualMetrics: []VirtualMetricConfig{
				{},
			},
			wantErrContains: []string{
				"virtual_metrics[0]: missing name",
				"virtual_metrics[0]: must define sources or alternatives",
			},
		},
		"reject topology source": {
			metrics: baseMetrics,
			topology: []TopologyConfig{
				{
					Kind: KindLldpRem,
					MetricsConfig: MetricsConfig{
						Table: SymbolConfig{
							OID:  "1.0.8802.1.1.2.1.4.1",
							Name: "lldpRemTable",
						},
						Symbols: []SymbolConfig{
							{OID: "1.0.8802.1.1.2.1.4.1.1.6", Name: "lldpRemPortIdSubtype"},
						},
						MetricTags: []MetricTagConfig{
							{Tag: "lldp_rem_index", Index: 1},
						},
					},
				},
			},
			virtualMetrics: []VirtualMetricConfig{
				{
					Name: "invalidTopologyDerivedMetric",
					Sources: []VirtualMetricSourceConfig{
						{Metric: "lldpRemPortIdSubtype", Table: "lldpRemTable"},
					},
				},
			},
			wantErrContains: []string{
				`virtual_metrics[0].sources[0]: topology metric source "lldpRemPortIdSubtype" cannot be used by virtual_metrics`,
			},
		},
		"duplicate name conflicting with metric": {
			metrics: baseMetrics,
			virtualMetrics: []VirtualMetricConfig{
				{Name: "ifTraffic", Sources: []VirtualMetricSourceConfig{{Metric: "ifHCInOctets", Table: "ifXTable"}}},
				{Name: "ifTraffic", Sources: []VirtualMetricSourceConfig{{Metric: "ifHCOutOctets", Table: "ifXTable"}}},
				{
					Name:    "ifInErrors",
					Sources: []VirtualMetricSourceConfig{{Metric: "ifHCOutOctets", Table: "ifXTable"}},
				},
			},
			wantErrContains: []string{
				`virtual_metrics[1]: duplicate name "ifTraffic"`,
				`virtual_metrics[2]: name "ifInErrors" conflicts with an existing metric`,
			},
		},
		"invalid grouped sources and emit tags": {
			metrics: baseMetrics,
			virtualMetrics: []VirtualMetricConfig{
				{
					Name:    "brokenGrouped",
					PerRow:  true,
					GroupBy: []string{"", "interface"},
					Sources: []VirtualMetricSourceConfig{
						{Metric: "ifHCInOctets", Table: "ifXTable", As: "in"},
						{Metric: "ifInErrors", Table: "ifTable", As: "out"},
						{Metric: "", Table: "", As: "missing"},
					},
					EmitTags: []VirtualMetricEmitTagConfig{
						{Tag: "", From: "interface"},
						{Tag: "interface", From: ""},
					},
				},
			},
			wantErrContains: []string{
				"virtual_metrics[0].group_by[0]: label cannot be empty",
				"virtual_metrics[0].emit_tags[0]: missing tag",
				"virtual_metrics[0].emit_tags[1]: missing from",
				`virtual_metrics[0].sources[1]: grouped virtual metrics require all sources to use the same table`,
				"virtual_metrics[0].sources[2]: missing metric",
				"virtual_metrics[0].sources[2]: missing table",
			},
		},
		"invalid alternatives": {
			metrics: baseMetrics,
			virtualMetrics: []VirtualMetricConfig{
				{
					Name: "ifTraffic",
					Alternatives: []VirtualMetricAlternativeSourcesConfig{
						{},
						{Sources: []VirtualMetricSourceConfig{{Metric: "missingMetric", Table: "ifXTable"}}},
					},
				},
			},
			wantErrContains: []string{
				"virtual_metrics[0].alternatives[0]: must define sources",
				`virtual_metrics[0].alternatives[1].sources[0]: unknown metric source "missingMetric"`,
			},
		},
	}

	for name, tt := range tests {
		t.Run(name, func(t *testing.T) {
			err := validateEnrichVirtualMetrics(tt.metrics, tt.topology, tt.virtualMetrics)
			if len(tt.wantErrContains) == 0 {
				assert.NoError(t, err)
				return
			}

			assert.Error(t, err)
			for _, msg := range tt.wantErrContains {
				assert.ErrorContains(t, err, msg)
			}
		})
	}
}
