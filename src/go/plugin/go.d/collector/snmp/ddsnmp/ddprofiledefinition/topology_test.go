// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).

package ddprofiledefinition

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gopkg.in/yaml.v2"
)

func TestProfileDefinition_UnmarshalTopologyAndConsumers(t *testing.T) {
	var profile ProfileDefinition

	err := yaml.Unmarshal([]byte(`
metadata:
  device:
    fields:
      lldp_loc_chassis_id:
        consumers: [topology]
        symbol:
          OID: 1.0.8802.1.1.2.1.3.2.0
          name: lldpLocChassisId
metric_tags:
  - tag: vendor
    consumers: [metrics, topology]
    symbol:
      OID: 1.3.6.1.2.1.1.1.0
      name: sysDescr
topology:
  - kind: lldp_rem
    MIB: LLDP-MIB
    table:
      OID: 1.0.8802.1.1.2.1.4.1
      name: lldpRemTable
    symbols:
      - OID: 1.0.8802.1.1.2.1.4.1.1.6
        name: lldpRemPortIdSubtype
    metric_tags:
      - tag: lldp_rem_index
        index: 1
`), &profile)

	require.NoError(t, err)
	require.Len(t, profile.Topology, 1)
	assert.Equal(t, KindLldpRem, profile.Topology[0].Kind)
	assert.Equal(t, "lldpRemTable", profile.Topology[0].Table.Name)
	require.Len(t, profile.Topology[0].Symbols, 1)
	assert.Equal(t, "lldpRemPortIdSubtype", profile.Topology[0].Symbols[0].Name)
	require.Len(t, profile.Topology[0].MetricTags, 1)
	assert.Equal(t, "lldp_rem_index", profile.Topology[0].MetricTags[0].Tag)
	assert.Equal(t, ConsumerSet{ConsumerTopology}, profile.Metadata["device"].Fields["lldp_loc_chassis_id"].Consumers)
	require.Len(t, profile.MetricTags, 1)
	assert.Equal(t, ConsumerSet{ConsumerMetrics, ConsumerTopology}, profile.MetricTags[0].Consumers)
}

func TestProfileDefinition_CloneTopologyAndConsumers(t *testing.T) {
	profile := &ProfileDefinition{
		Metadata: MetadataConfig{
			"device": {
				Fields: map[string]MetadataField{
					"vendor": {
						Value:     "Cisco",
						Consumers: ConsumerSet{ConsumerMetrics, ConsumerTopology},
					},
				},
			},
		},
		MetricTags: []GlobalMetricTagConfig{
			{
				MetricTagConfig: MetricTagConfig{Tag: "vendor"},
				Consumers:       ConsumerSet{ConsumerMetrics, ConsumerTopology},
			},
		},
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
					MetricTags: MetricTagConfigList{
						{
							Tag: "lldp_rem_index",
							IndexTransform: []MetricIndexTransform{
								{Start: 1},
							},
						},
					},
				},
			},
		},
	}

	cloned := profile.Clone()
	require.Equal(t, profile, cloned)

	cloned.Metadata["device"].Fields["vendor"] = MetadataField{
		Value:     "Cisco",
		Consumers: ConsumerSet{ConsumerTopology},
	}
	cloned.MetricTags[0].Consumers[0] = ConsumerTopology
	cloned.Topology[0].MetricTags[0].IndexTransform[0].Start = 2

	assert.Equal(t, ConsumerSet{ConsumerMetrics, ConsumerTopology}, profile.Metadata["device"].Fields["vendor"].Consumers)
	assert.Equal(t, ConsumerSet{ConsumerMetrics, ConsumerTopology}, profile.MetricTags[0].Consumers)
	assert.Equal(t, uint(1), profile.Topology[0].MetricTags[0].IndexTransform[0].Start)
}

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
							MetricTags: MetricTagConfigList{
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
							Symbol: SymbolConfig{OID: "1.2.3.0", Name: "topologyValue"},
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
							Options: MetricsConfigOption{Placement: 1},
							Symbol: SymbolConfig{
								OID:              "1.3.6.1.2.1.2.2.1.8.0",
								Name:             "_topology_if_status",
								ChartMeta:        ChartMeta{Description: "status"},
								MetricType:       ProfileMetricTypeGauge,
								Mapping:          NewExactMapping(map[string]string{"1": "up"}),
								Transform:        `{{ .Metric }}`,
								ScaleFactor:      2,
								Format:           "mac_address",
								ConstantValueOne: true,
							},
						},
					},
				},
			},
			wantErrContains: []string{
				"topology[0]: options cannot be used in topology rows",
				`topology[0]: symbol name "_topology_if_status" cannot be underscore-prefixed`,
				"topology[0]: chart_meta cannot be used in topology rows",
				"topology[0]: metric_type cannot be used in topology rows",
				"topology[0]: mapping cannot be used in topology rows",
				"topology[0]: transform cannot be used in topology rows",
				"topology[0]: scale_factor cannot be used in topology rows",
				"topology[0]: format cannot be used in topology rows",
				"topology[0]: constant_value_one cannot be used in topology rows",
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
							MetricTags: MetricTagConfigList{
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
						MetricTagConfig: MetricTagConfig{Tag: "vendor"},
						Consumers:       ConsumerSet{ConsumerMetrics, ConsumerMetrics},
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
