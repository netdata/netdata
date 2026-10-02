// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).

package ddprofiledefinition

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestValidateEnrichProfile_Topology(t *testing.T) {
	tests := map[string]struct {
		profile         ProfileDefinition
		wantErrContains []string
	}{
		"valid topology row": {
			profile: ProfileDefinition{
				Topology: []TopologyConfig{
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
			},
		},
		"unknown topology kind": {
			profile: ProfileDefinition{
				Topology: []TopologyConfig{
					{
						Kind: "typo",
						MetricsConfig: MetricsConfig{
							Symbol: SymbolConfig{
								OID:  "1.2.3.0",
								Name: "topologyValue",
							},
						},
					},
				},
			},
			wantErrContains: []string{`topology[0]: invalid kind "typo"`},
		},
		"metrics-only topology fields": {
			profile: ProfileDefinition{
				Topology: []TopologyConfig{
					{
						Kind: KindIfStatus,
						MetricsConfig: MetricsConfig{
							Symbol: SymbolConfig{
								OID:  "1.3.6.1.2.1.2.2.1.8.0",
								Name: "_topology_if_status",
								ChartMeta: ChartMeta{
									Description: "status",
								},
								MetricType:  ProfileMetricTypeGauge,
								Mapping:     NewExactMapping(map[string]string{"1": "up"}),
								Transform:   `{{ .Metric }}`,
								ScaleFactor: 2,
								Format:      "mac_address",
							},
						},
					},
				},
			},
			wantErrContains: []string{
				`topology[0].symbol: symbol name "_topology_if_status" cannot be underscore-prefixed`,
				"topology[0].symbol: chart_meta cannot be used in topology rows",
				"topology[0].symbol: metric_type cannot be used in topology rows",
				"topology[0].symbol: mapping cannot be used in topology rows",
				"topology[0].symbol: transform cannot be used in topology rows",
				"topology[0].symbol: scale_factor cannot be used in topology rows",
				"topology[0].symbol: format cannot be used in topology rows",
			},
		},
		"table presence symbols reject ignored value transforms": {
			profile: ProfileDefinition{
				Topology: []TopologyConfig{
					{
						Kind: KindArpEntry,
						MetricsConfig: MetricsConfig{
							Table: SymbolConfig{
								OID:  "1.3.6.1.2.1.4.35.1",
								Name: "ipNetToPhysicalTable",
							},
							Symbols: []SymbolConfig{{
								OID:          "1.3.6.1.2.1.4.35.1.4",
								Name:         "ipNetToPhysicalPhysAddress",
								ExtractValue: `([0-9a-f]+)`,
								MatchPattern: `^([0-9a-f]+)$`,
								MatchValue:   "$1",
							}},
							MetricTags: []MetricTagConfig{{Tag: "row", Index: 1}},
						},
					},
				},
			},
			wantErrContains: []string{
				"topology[0].symbols[0]: extract_value cannot be used on table presence symbols",
				"topology[0].symbols[0]: match_pattern cannot be used on table presence symbols",
				"topology[0].symbols[0]: match_value cannot be used on table presence symbols",
			},
		},
		"scalar topology value transforms remain valid": {
			profile: ProfileDefinition{
				Topology: []TopologyConfig{
					{
						Kind: KindIfStatus,
						MetricsConfig: MetricsConfig{
							Symbol: SymbolConfig{
								OID:          "1.3.6.1.4.1.99999.1.0",
								Name:         "scalarTopologyState",
								ExtractValue: `state=(\d+)`,
								MatchPattern: `^(\d+)$`,
								MatchValue:   "$1",
							},
						},
					},
				},
			},
		},
		"metric tag extraction fields remain valid": {
			profile: ProfileDefinition{
				Topology: []TopologyConfig{
					{
						Kind: KindFdbEntry,
						MetricsConfig: MetricsConfig{
							Table: SymbolConfig{
								OID:  "1.3.6.1.2.1.17.4.3",
								Name: "dot1dTpFdbTable",
							},
							Symbols: []SymbolConfig{
								{OID: "1.3.6.1.2.1.17.4.3.1.2", Name: "dot1dTpFdbPort"},
							},
							MetricTags: []MetricTagConfig{
								{
									Tag:     "fdb_mac",
									Index:   1,
									Mapping: NewExactMapping(map[string]string{"1": "one"}),
									Symbol: SymbolConfigCompat{
										Name:   "dot1dTpFdbAddress",
										Format: "mac_address",
									},
									IndexTransform: []MetricIndexTransform{{Start: 1}},
								},
							},
						},
					},
				},
			},
		},
		"invalid consumer": {
			profile: ProfileDefinition{
				Metadata: MetadataConfig{
					"device": {
						Fields: map[string]MetadataField{
							"vendor": {
								Value:     "Cisco",
								Consumers: ConsumerSet{"logs"},
							},
						},
					},
				},
				MetricTags: []GlobalMetricTagConfig{
					{
						MetricTagConfig: MetricTagConfig{
							Tag: "vendor",
						},
						Consumers: ConsumerSet{ConsumerMetrics, ConsumerMetrics},
					},
				},
			},
			wantErrContains: []string{
				`metadata.device.fields.vendor.consumers[0]: invalid consumer "logs"`,
				`metric_tags[0].consumers[1]: duplicate consumer "metrics"`,
			},
		},
	}

	for name, tt := range tests {
		t.Run(name, func(t *testing.T) {
			profile := tt.profile
			err := ValidateEnrichProfile(&profile)
			if len(tt.wantErrContains) == 0 {
				assert.NoError(t, err)
				return
			}
			require.Error(t, err)
			for _, msg := range tt.wantErrContains {
				assert.ErrorContains(t, err, msg)
			}
		})
	}
}
