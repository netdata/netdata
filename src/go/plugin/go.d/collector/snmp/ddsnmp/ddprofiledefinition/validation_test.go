// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package ddprofiledefinition

import (
	"regexp"
	"testing"

	"github.com/stretchr/testify/assert"
)

func Test_validateEnrichMetrics(t *testing.T) {
	tests := map[string]struct {
		metrics     []MetricsConfig
		wantError   bool
		wantMetrics []MetricsConfig
	}{
		"either table symbol or scalar symbol must be provided": {
			wantError: true,
			metrics: []MetricsConfig{
				{},
			},
			wantMetrics: []MetricsConfig{
				{},
			},
		},
		"table column symbols and scalar symbol cannot be both provided": {
			wantError: true,
			metrics: []MetricsConfig{
				{
					Symbol: SymbolConfig{
						OID:  "1.2",
						Name: "abc",
					},
					Symbols: []SymbolConfig{
						{
							OID:  "1.2",
							Name: "abc",
						},
					},
					MetricTags: MetricTagConfigList{
						MetricTagConfig{},
					},
				},
			},
		},
		"multiple errors": {
			wantError: true,
			metrics: []MetricsConfig{
				{},
				{
					Symbol: SymbolConfig{
						OID:  "1.2",
						Name: "abc",
					},
					Symbols: []SymbolConfig{
						{
							OID:  "1.2",
							Name: "abc",
						},
					},
					MetricTags: MetricTagConfigList{
						MetricTagConfig{},
					},
				},
			},
		},
		"missing symbol name": {
			wantError: true,
			metrics: []MetricsConfig{
				{
					Symbol: SymbolConfig{
						OID: "1.2.3",
					},
				},
			},
		},
		"table column symbol name missing": {
			wantError: true,
			metrics: []MetricsConfig{
				{
					Symbols: []SymbolConfig{
						{
							OID: "1.2",
						},
						{
							Name: "abc",
						},
					},
					MetricTags: MetricTagConfigList{
						MetricTagConfig{},
					},
				},
			},
		},
		"table external metric column tag symbol error": {
			wantError: true,
			metrics: []MetricsConfig{
				{
					Symbols: []SymbolConfig{
						{
							OID:  "1.2",
							Name: "abc",
						},
					},
					MetricTags: MetricTagConfigList{
						MetricTagConfig{
							Symbol: SymbolConfigCompat{
								OID: "1.2.3",
							},
						},
						MetricTagConfig{
							Symbol: SymbolConfigCompat{
								Name: "abc",
							},
						},
					},
				},
			},
		},
		"missing MetricTags": {
			wantError: true,
			metrics: []MetricsConfig{
				{
					Symbols: []SymbolConfig{
						{
							OID:  "1.2",
							Name: "abc",
						},
					},
					MetricTags: MetricTagConfigList{},
				},
			},
		},
		"table external metric column tag MIB error": {
			wantError: true,
			metrics: []MetricsConfig{
				{
					Symbols: []SymbolConfig{
						{
							OID:  "1.2",
							Name: "abc",
						},
					},
					MetricTags: MetricTagConfigList{
						MetricTagConfig{
							Symbol: SymbolConfigCompat{
								OID: "1.2.3",
							},
						},
						MetricTagConfig{
							Symbol: SymbolConfigCompat{
								Name: "abc",
							},
						},
					},
				},
			},
		},
		"missing match tags": {
			wantError: true,
			metrics: []MetricsConfig{
				{
					Symbols: []SymbolConfig{
						{
							OID:  "1.2",
							Name: "abc",
						},
					},
					MetricTags: MetricTagConfigList{
						MetricTagConfig{
							Symbol: SymbolConfigCompat{
								OID:  "1.2.3",
								Name: "abc",
							},
							Match: "([a-z])",
						},
					},
				},
			},
		},
		"match cannot compile regex": {
			wantError: true,
			metrics: []MetricsConfig{
				{
					Symbols: []SymbolConfig{
						{
							OID:  "1.2",
							Name: "abc",
						},
					},
					MetricTags: MetricTagConfigList{
						MetricTagConfig{
							Symbol: SymbolConfigCompat{
								OID:  "1.2.3",
								Name: "abc",
							},
							Match: "([a-z)",
							Tags: map[string]string{
								"foo": "bar",
							},
						},
					},
				},
			},
		},
		"match cannot compile regex 2": {
			wantError: true,
			metrics: []MetricsConfig{
				{
					Symbols: []SymbolConfig{
						{
							OID:  "1.2",
							Name: "abc",
						},
					},
					MetricTags: MetricTagConfigList{
						MetricTagConfig{
							Symbol: SymbolConfigCompat{
								OID:  "1.2.3",
								Name: "abc",
							},
							Tag: "hello",
							IndexTransform: []MetricIndexTransform{
								{
									Start: 2,
									End:   1,
								},
							},
						},
					},
				},
			},
		},
		"compiling extract_value": {
			wantError: false,
			metrics: []MetricsConfig{
				{
					Symbol: SymbolConfig{
						OID:          "1.2.3",
						Name:         "myMetric",
						ExtractValue: `(\d+)C`,
					},
				},
				{
					Symbols: []SymbolConfig{
						{
							OID:          "1.2",
							Name:         "hey",
							ExtractValue: `(\d+)C`,
						},
					},
					MetricTags: MetricTagConfigList{
						MetricTagConfig{
							Symbol: SymbolConfigCompat{
								OID:          "1.2.3",
								Name:         "abc",
								ExtractValue: `(\d+)C`,
							},
							Tag: "hello",
						},
					},
				},
			},
			wantMetrics: []MetricsConfig{
				{
					Symbol: SymbolConfig{
						OID:                  "1.2.3",
						Name:                 "myMetric",
						ExtractValue:         `(\d+)C`,
						ExtractValueCompiled: regexp.MustCompile(`(\d+)C`),
					},
				},
				{
					Symbols: []SymbolConfig{
						{
							OID:                  "1.2",
							Name:                 "hey",
							ExtractValue:         `(\d+)C`,
							ExtractValueCompiled: regexp.MustCompile(`(\d+)C`),
						},
					},
					MetricTags: MetricTagConfigList{
						MetricTagConfig{
							Symbol: SymbolConfigCompat{
								OID:                  "1.2.3",
								Name:                 "abc",
								ExtractValue:         `(\d+)C`,
								ExtractValueCompiled: regexp.MustCompile(`(\d+)C`),
							},
							Tag: "hello",
						},
					},
				},
			},
		},
		"error compiling extract_value": {
			wantError: true,
			metrics: []MetricsConfig{
				{
					Symbol: SymbolConfig{
						OID:          "1.2.3",
						Name:         "myMetric",
						ExtractValue: "[{",
					},
				},
			},
		},
		"constant_value_one usage in column symbol": {
			wantError: false,
			metrics: []MetricsConfig{
				{
					Symbols: []SymbolConfig{
						{
							Name:             "abc",
							ConstantValueOne: true,
						},
					},
					MetricTags: MetricTagConfigList{
						MetricTagConfig{
							Symbol: SymbolConfigCompat{
								Name: "abc",
								OID:  "1.2.3",
							},
							Tag: "hello",
						},
					},
				},
			},
		},
		"constant_value_one usage in scalar symbol": {
			wantError: true,
			metrics: []MetricsConfig{
				{
					Symbol: SymbolConfig{
						Name:             "myMetric",
						ConstantValueOne: true,
					},
				},
			},
		},
		"constant_value_one usage in scalar symbol with OID": {
			wantError: true,
			metrics: []MetricsConfig{
				{
					Symbol: SymbolConfig{
						OID:              "1.2.3",
						Name:             "myMetric",
						ConstantValueOne: true,
					},
				},
			},
		},
		"constant_value_one usage in metric tags": {
			wantError: true,
			metrics: []MetricsConfig{
				{
					Symbols: []SymbolConfig{
						{
							OID:  "1.2",
							Name: "abc",
						},
					},
					MetricTags: MetricTagConfigList{
						MetricTagConfig{
							Symbol: SymbolConfigCompat{
								Name:             "abc",
								ConstantValueOne: true,
							},
							Tag: "hello",
						},
					},
				},
			},
		},
		"metric_type usage in column symbol": {
			wantError: false,
			metrics: []MetricsConfig{
				{
					Symbols: []SymbolConfig{
						{
							Name:       "abc",
							OID:        "1.2.3",
							MetricType: ProfileMetricTypeCounter,
						},
					},
					MetricTags: MetricTagConfigList{
						MetricTagConfig{
							Symbol: SymbolConfigCompat{
								Name: "abc",
								OID:  "1.2.3",
							},
							Tag: "hello",
						},
					},
				},
			},
		},
		"metric_type usage in scalar symbol": {
			wantError: false,
			metrics: []MetricsConfig{
				{
					Symbol: SymbolConfig{
						Name:       "abc",
						OID:        "1.2.3",
						MetricType: ProfileMetricTypeCounter,
					},
				},
			},
		},
		"ERROR metric_type usage in metric_tags": {
			wantError: true,
			metrics: []MetricsConfig{
				{
					Symbols: []SymbolConfig{
						{
							Name: "abc",
							OID:  "1.2.3",
						},
					},
					MetricTags: MetricTagConfigList{
						MetricTagConfig{
							Symbol: SymbolConfigCompat{
								Name:       "abc",
								OID:        "1.2.3",
								MetricType: ProfileMetricTypeCounter,
							},
							Tag: "hello",
						},
					},
				},
			},
		},
		"mapping used with symbol.name and no explicit tag": {
			wantError: false,
			metrics: []MetricsConfig{
				{
					Symbols: []SymbolConfig{
						{
							OID:  "1.2",
							Name: "abc",
						},
					},
					MetricTags: MetricTagConfigList{
						MetricTagConfig{
							Symbol: SymbolConfigCompat{
								OID:  "1.2",
								Name: "abc",
							},
							Mapping: NewExactMapping(map[string]string{
								"1": "abc",
								"2": "def",
							}),
						},
					},
				},
			},
		},
		"bitmask mapping usage in scalar symbol": {
			wantError: false,
			metrics: []MetricsConfig{
				{
					Symbol: SymbolConfig{
						OID:  "1.2.3",
						Name: "processorStatus",
						Mapping: NewBitmaskMapping(map[string]string{
							"1":   "internalError",
							"128": "processorPresent",
						}),
					},
				},
			},
		},
		"ERROR bitmask mapping usage in metric_tags": {
			wantError: true,
			metrics: []MetricsConfig{
				{
					Symbols: []SymbolConfig{
						{
							OID:  "1.2",
							Name: "abc",
						},
					},
					MetricTags: MetricTagConfigList{
						MetricTagConfig{
							Tag: "state",
							Symbol: SymbolConfigCompat{
								OID:  "1.2",
								Name: "abc",
							},
							Mapping: NewBitmaskMapping(map[string]string{
								"1": "internalError",
							}),
						},
					},
				},
			},
		},
		"scalar metric_tags with scalar OID symbol are supported": {
			wantError: false,
			metrics: []MetricsConfig{
				{
					Symbol: SymbolConfig{
						OID:  "1.2.3",
						Name: "myMetric",
					},
					MetricTags: MetricTagConfigList{
						{
							OID: "1.2.4",
							Symbol: SymbolConfigCompat{
								Name: "stateSource",
							},
							Tag: "state",
						},
					},
				},
			},
			wantMetrics: []MetricsConfig{
				{
					Symbol: SymbolConfig{
						OID:  "1.2.3",
						Name: "myMetric",
					},
					MetricTags: MetricTagConfigList{
						{
							Tag: "state",
							Symbol: SymbolConfigCompat{
								OID:  "1.2.4",
								Name: "stateSource",
							},
						},
					},
				},
			},
		},
		"scalar metric_tags do not support index lookups": {
			wantError: true,
			metrics: []MetricsConfig{
				{
					Symbol: SymbolConfig{
						OID:  "1.2.3",
						Name: "myMetric",
					},
					MetricTags: MetricTagConfigList{
						{
							Tag:   "idx",
							Index: 1,
						},
					},
				},
			},
		},
		"scalar metric_tags do not support table lookups": {
			wantError: true,
			metrics: []MetricsConfig{
				{
					Symbol: SymbolConfig{
						OID:  "1.2.3",
						Name: "myMetric",
					},
					MetricTags: MetricTagConfigList{
						{
							Tag:   "peer",
							Table: "ifTable",
							Symbol: SymbolConfigCompat{
								OID:  "1.2.4",
								Name: "ifDescr",
							},
						},
					},
				},
			},
		},
		"scalar metric_tags do not support index transforms": {
			wantError: true,
			metrics: []MetricsConfig{
				{
					Symbol: SymbolConfig{
						OID:  "1.2.3",
						Name: "myMetric",
					},
					MetricTags: MetricTagConfigList{
						{
							Tag: "peer",
							Symbol: SymbolConfigCompat{
								OID:  "1.2.4",
								Name: "peerState",
							},
							IndexTransform: []MetricIndexTransform{
								{
									Start: 1,
									End:   1,
								},
							},
						},
					},
				},
			},
		},
	}
	for name, tc := range tests {
		t.Run(name, func(t *testing.T) {
			if tc.wantError {
				assert.Error(t, validateEnrichMetrics(tc.metrics))
			} else {
				assert.NoError(t, validateEnrichMetrics(tc.metrics))
			}
			if tc.wantMetrics != nil {
				assert.Equal(t, tc.wantMetrics, tc.metrics)
			}
		})
	}
}

func Test_validateEnrichMetricTag_MappingErrorUsesReadableFormat(t *testing.T) {
	tag := MetricTagConfig{
		Mapping: NewExactMapping(map[string]string{
			"1": "up",
		}),
	}

	err := validateEnrichMetricTag(&tag)

	if assert.Error(t, err) {
		assert.Contains(t, err.Error(), "map[1:up]")
		assert.NotContains(t, err.Error(), "%!s")
	}
}

func Test_validateEnrichSymbol_BitmaskMappingRequiresSingleBitKeys(t *testing.T) {
	sym := SymbolConfig{
		OID:  "1.2.3",
		Name: "processorStatus",
		Mapping: NewBitmaskMapping(map[string]string{
			"internal": "internalError",
		}),
	}

	err := validateEnrichSymbol(&sym, ScalarSymbol)

	if assert.Error(t, err) {
		assert.Contains(t, err.Error(), "requires keys to be 0 or a single power-of-two bit")
	}
}

func Test_validateEnrichSymbol_BitmaskMappingRejectsCompositeMasks(t *testing.T) {
	sym := SymbolConfig{
		OID:  "1.2.3",
		Name: "processorStatus",
		Mapping: NewBitmaskMapping(map[string]string{
			"3": "combinedFault",
		}),
	}

	err := validateEnrichSymbol(&sym, ScalarSymbol)

	if assert.Error(t, err) {
		assert.Contains(t, err.Error(), "requires keys to be 0 or a single power-of-two bit")
		assert.Contains(t, err.Error(), "\"3\"")
	}
}

func Test_validateEnrichMetadata_BitmaskMappingUnsupported(t *testing.T) {
	metadata := MetadataConfig{
		"device": {
			Fields: map[string]MetadataField{
				"description": {
					Symbol: SymbolConfig{
						OID:  "1.2.3",
						Name: "deviceStatus",
						Mapping: NewBitmaskMapping(map[string]string{
							"1": "internalError",
						}),
					},
				},
			},
		},
	}

	err := validateEnrichMetadata(metadata)

	if assert.Error(t, err) {
		assert.Contains(t, err.Error(), "only supported for scalar/table metric symbols")
	}
}

func Test_validateEnrichSymbol_BitmaskMappingRejectsScaleFactor(t *testing.T) {
	sym := SymbolConfig{
		OID:         "1.2.3",
		Name:        "processorStatus",
		ScaleFactor: 2,
		Mapping: NewBitmaskMapping(map[string]string{
			"1":   "internalError",
			"128": "processorPresent",
		}),
	}

	err := validateEnrichSymbol(&sym, ScalarSymbol)

	if assert.Error(t, err) {
		assert.Contains(t, err.Error(), "`scale_factor` cannot be used with `mapping.mode: bitmask`")
	}
}

func Test_validateEnrichSymbol_MappingModeRequiresItems(t *testing.T) {
	sym := SymbolConfig{
		OID:  "1.2.3",
		Name: "processorStatus",
		Mapping: MappingConfig{
			Mode: MappingModeBitmask,
		},
	}

	err := validateEnrichSymbol(&sym, ScalarSymbol)

	if assert.Error(t, err) {
		assert.Contains(t, err.Error(), "`mapping.mode` requires `mapping.items`")
	}
}

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
			MetricTags: MetricTagConfigList{
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
			MetricTags: MetricTagConfigList{
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
				MetricTags: MetricTagConfigList{
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
						Mapping: NewBitmaskMapping(map[string]string{
							"1":   "internalError",
							"128": "processorPresent",
						}),
					},
				},
				MetricTags: MetricTagConfigList{
					{Tag: "processor", Index: 1},
				},
			}),
			virtualMetrics: []VirtualMetricConfig{
				{
					Name:   "processorDeviceHealth",
					PerRow: true,
					Sources: []VirtualMetricSourceConfig{
						{Metric: "processorDeviceStatusReading", Table: "processorDeviceStatusTable", As: "present", Dim: "processorPresent"},
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
				MetricTags: MetricTagConfigList{
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
				MetricTags: MetricTagConfigList{
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
				MetricTags: MetricTagConfigList{
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
						MetricTags: MetricTagConfigList{
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
				{Name: "ifInErrors", Sources: []VirtualMetricSourceConfig{{Metric: "ifHCOutOctets", Table: "ifXTable"}}},
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

func Test_validateEnrichMetricTags(t *testing.T) {
	tests := map[string]struct {
		metrics     []MetricTagConfig
		wantError   bool
		wantMetrics []MetricTagConfig
	}{
		"Move OID to Symbol": {
			wantError: false,
			metrics: []MetricTagConfig{
				{
					OID: "1.2.3.4",
					Symbol: SymbolConfigCompat{
						Name: "mySymbol",
					},
				},
			},
			wantMetrics: []MetricTagConfig{
				{
					Symbol: SymbolConfigCompat{
						OID:  "1.2.3.4",
						Name: "mySymbol",
					},
				},
			},
		},
		"Metric tag OID and symbol.OID cannot be both declared": {
			wantError: true,
			metrics: []MetricTagConfig{
				{
					OID: "1.2.3.4",
					Symbol: SymbolConfigCompat{
						OID:  "1.2.3.5",
						Name: "mySymbol",
					},
				},
			},
		},
		"metric tag symbol and column cannot be both declared 2": {
			wantError: true,
			metrics: []MetricTagConfig{
				{
					Symbol: SymbolConfigCompat{
						OID:  "1.2.3.5",
						Name: "mySymbol",
					},
					Column: SymbolConfig{
						OID:  "1.2.3.5",
						Name: "mySymbol",
					},
				},
			},
		},
		"Missing OID": {
			wantError: true,
			metrics: []MetricTagConfig{
				{
					Symbol: SymbolConfigCompat{
						Name: "mySymbol",
					},
				},
			},
		},
		"raw index transform with drop_right": {
			wantError: false,
			metrics: []MetricTagConfig{
				{
					Tag: "remote_addr",
					IndexTransform: []MetricIndexTransform{
						{
							Start:     2,
							DropRight: 2,
						},
					},
				},
			},
		},
		"raw index transform to tail": {
			wantError: false,
			metrics: []MetricTagConfig{
				{
					Tag: "fdb_mac",
					IndexTransform: []MetricIndexTransform{
						{
							Start: 1,
						},
					},
				},
			},
		},
		"raw index transform cannot combine end and drop_right": {
			wantError: true,
			metrics: []MetricTagConfig{
				{
					Tag: "remote_addr",
					IndexTransform: []MetricIndexTransform{
						{
							Start:     2,
							End:       6,
							DropRight: 2,
						},
					},
				},
			},
		},
	}
	for name, tc := range tests {
		t.Run(name, func(t *testing.T) {
			if tc.wantError {
				assert.Error(t, validateEnrichMetricTags(tc.metrics))
			} else {
				assert.NoError(t, validateEnrichMetricTags(tc.metrics))
			}
			if tc.wantMetrics != nil {
				assert.Equal(t, tc.wantMetrics, tc.metrics)
			}
		})
	}
}

func Test_validateEnrichMetadata(t *testing.T) {
	tests := map[string]struct {
		metadata     MetadataConfig
		wantError    bool
		wantMetadata MetadataConfig
	}{
		"both field symbol and value can be provided": {
			wantError: false,
			metadata: MetadataConfig{
				"device": MetadataResourceConfig{
					Fields: map[string]MetadataField{
						"name": {
							Value: "hey",
							Symbol: SymbolConfig{
								OID:  "1.2.3",
								Name: "someSymbol",
							},
						},
					},
				},
			},
			wantMetadata: MetadataConfig{
				"device": MetadataResourceConfig{
					Fields: map[string]MetadataField{
						"name": {
							Value: "hey",
							Symbol: SymbolConfig{
								OID:  "1.2.3",
								Name: "someSymbol",
							},
						},
					},
				},
			},
		},
		"invalid regex pattern for symbol": {
			wantError: true,
			metadata: MetadataConfig{
				"device": MetadataResourceConfig{
					Fields: map[string]MetadataField{
						"name": {
							Symbol: SymbolConfig{
								OID:          "1.2.3",
								Name:         "someSymbol",
								ExtractValue: "(\\w[)",
							},
						},
					},
				},
			},
		},
		"invalid regex pattern for multiple symbols": {
			wantError: true,
			metadata: MetadataConfig{
				"device": MetadataResourceConfig{
					Fields: map[string]MetadataField{
						"name": {
							Symbols: []SymbolConfig{
								{
									OID:          "1.2.3",
									Name:         "someSymbol",
									ExtractValue: "(\\w[)",
								},
							},
						},
					},
				},
			},
		},
		"field regex pattern is compiled": {
			wantError: false,
			metadata: MetadataConfig{
				"device": MetadataResourceConfig{
					Fields: map[string]MetadataField{
						"name": {
							Symbol: SymbolConfig{
								OID:          "1.2.3",
								Name:         "someSymbol",
								ExtractValue: "(\\w)",
							},
						},
					},
				},
			},
			wantMetadata: MetadataConfig{
				"device": MetadataResourceConfig{
					Fields: map[string]MetadataField{
						"name": {
							Symbol: SymbolConfig{
								OID:                  "1.2.3",
								Name:                 "someSymbol",
								ExtractValue:         "(\\w)",
								ExtractValueCompiled: regexp.MustCompile(`(\w)`),
							},
						},
					},
				},
			},
		},
		"topology device metadata fields are accepted": {
			wantError: false,
			metadata: MetadataConfig{
				"device": MetadataResourceConfig{
					Fields: map[string]MetadataField{
						"lldp_loc_sys_name": {
							Symbol: SymbolConfig{
								OID:  "1.0.8802.1.1.2.1.3.3.0",
								Name: "lldpLocSysName",
							},
						},
						"bridge_base_address": {
							Symbol: SymbolConfig{
								OID:    "1.3.6.1.2.1.17.1.1",
								Name:   "dot1dBaseBridgeAddress",
								Format: "hex",
							},
						},
						"vtp_version": {
							Symbol: SymbolConfig{
								OID:  "1.3.6.1.4.1.9.9.46.1.1.1",
								Name: "vtpVersion",
							},
						},
					},
				},
			},
		},
		"invalid resource": {
			wantError: true,
			metadata: MetadataConfig{
				"invalid-res": MetadataResourceConfig{
					Fields: map[string]MetadataField{
						"name": {
							Value: "hey",
						},
					},
				},
			},
		},
		"invalid field": {
			wantError: true,
			metadata: MetadataConfig{
				"device": MetadataResourceConfig{
					Fields: map[string]MetadataField{
						"invalid-field": {
							Value: "hey",
						},
					},
				},
			},
		},
		"invalid idtags": {
			wantError: true,
			metadata: MetadataConfig{
				"interface": MetadataResourceConfig{
					Fields: map[string]MetadataField{
						"invalid-field": {
							Value: "hey",
						},
					},
					IDTags: MetricTagConfigList{
						MetricTagConfig{
							Symbol: SymbolConfigCompat{
								OID:  "1.2.3",
								Name: "abc",
							},
							Match: "([a-z)",
							Tags: map[string]string{
								"foo": "bar",
							},
						},
					},
				},
			},
		},
		"device resource does not support id_tags": {
			wantError: true,
			metadata: MetadataConfig{
				"device": MetadataResourceConfig{
					Fields: map[string]MetadataField{
						"name": {
							Value: "hey",
						},
					},
					IDTags: MetricTagConfigList{
						MetricTagConfig{
							Symbol: SymbolConfigCompat{
								OID:  "1.2.3",
								Name: "abc",
							},
							Tag: "abc",
						},
					},
				},
			},
		},
	}

	for name, tc := range tests {
		t.Run(name, func(t *testing.T) {
			if tc.wantError {
				assert.Error(t, validateEnrichMetadata(tc.metadata))
			} else {
				assert.NoError(t, validateEnrichMetadata(tc.metadata))
			}
			if tc.wantMetadata != nil {
				assert.Equal(t, tc.wantMetadata, tc.metadata)
			}
		})
	}
}

func Test_validateEnrichSysobjectIDMetadata(t *testing.T) {
	tests := map[string]struct {
		entries   []SysobjectIDMetadataEntryConfig
		wantError bool
	}{
		"accepts explicit version fields": {
			entries: []SysobjectIDMetadataEntryConfig{
				{
					SysobjectID: "1.3.6.1.4.1.9.1.1",
					Metadata: map[string]MetadataField{
						"software_version": {
							Value: "17.9.4",
						},
						"firmware_version": {
							Symbol: SymbolConfig{
								OID:  "1.2.3",
								Name: "firmwareVersion",
							},
						},
						"hardware_version": {
							Symbols: []SymbolConfig{
								{
									OID:  "1.2.4",
									Name: "hardwareVersion",
								},
							},
						},
					},
				},
			},
		},
		"rejects unknown field name": {
			entries: []SysobjectIDMetadataEntryConfig{
				{
					SysobjectID: "1.3.6.1.4.1.9.1.1",
					Metadata: map[string]MetadataField{
						"custom_firmware_build": {
							Value: "x1",
						},
					},
				},
			},
			wantError: true,
		},
	}

	for name, tc := range tests {
		t.Run(name, func(t *testing.T) {
			if tc.wantError {
				assert.Error(t, validateEnrichSysobjectIDMetadata(tc.entries))
			} else {
				assert.NoError(t, validateEnrichSysobjectIDMetadata(tc.entries))
			}
		})
	}
}
