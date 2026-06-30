// SPDX-License-Identifier: GPL-3.0-or-later

package ddprofiledefinition

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gopkg.in/yaml.v2"
)

func TestProfileDefinition_UnmarshalLicensing(t *testing.T) {
	var profile ProfileDefinition

	err := yaml.Unmarshal([]byte(`
metric_tags:
  - tag: vendor
    consumers: [licensing]
    symbol:
      OID: 1.3.6.1.2.1.1.1.0
      name: sysDescr
licensing:
  - id: sophos-base-firewall
    MIB: SFOS-FIREWALL-MIB
    identity:
      id: { value: base_firewall }
      name: { value: Base Firewall }
    descriptors:
      type: { value: subscription }
    state:
      from: 1.3.6.1.4.1.2604.5.1.5.1.1.0
      policy: sophos
      mapping:
        items: { 0: ignored, 1: healthy, 2: broken }
    signals:
      expiry:
        from: 1.3.6.1.4.1.2604.5.1.5.1.2.0
        format: text_date
        sentinel: [timer_zero_or_negative]
`), &profile)

	require.NoError(t, err)
	require.Len(t, profile.Licensing, 1)
	assert.Equal(t, "sophos-base-firewall", profile.Licensing[0].ID)
	assert.Equal(t, "SFOS-FIREWALL-MIB", profile.Licensing[0].MIB)
	assert.Equal(t, "base_firewall", profile.Licensing[0].Identity.ID.Value)
	assert.Equal(t, LicenseStatePolicySophos, profile.Licensing[0].State.Policy)
	assert.Equal(t, "1.3.6.1.4.1.2604.5.1.5.1.2.0", profile.Licensing[0].Signals.Expiry.From)
	assert.Equal(t, "text_date", profile.Licensing[0].Signals.Expiry.Format)
	assert.Equal(t, []LicenseSentinelPolicy{LicenseSentinelTimerZeroOrNegative}, profile.Licensing[0].Signals.Expiry.Sentinel)
	require.Len(t, profile.MetricTags, 1)
	assert.Equal(t, ConsumerSet{ConsumerLicensing}, profile.MetricTags[0].Consumers)
}

func TestProfileDefinition_CloneLicensing(t *testing.T) {
	profile := &ProfileDefinition{
		Licensing: []LicensingConfig{
			{
				OriginProfileID: "_vendor-licensing.yaml",
				ID:              "row",
				Identity: LicenseIdentityConfig{
					ID: LicenseValueConfig{Value: "license-1"},
				},
				State: LicenseStateConfig{
					LicenseValueConfig: LicenseValueConfig{
						Symbol: SymbolConfig{
							OID:  "1.2.3.0",
							Name: "licenseState",
							Mapping: NewExactMapping(map[string]string{
								"1": "healthy",
							}),
						},
					},
					Policy: LicenseStatePolicyDefault,
				},
				Signals: LicenseSignalsConfig{
					Expiry: LicenseTimerSignalsConfig{
						LicenseValueConfig: LicenseValueConfig{
							From:     "1.2.4.0",
							Sentinel: []LicenseSentinelPolicy{LicenseSentinelTimerU32Max},
						},
					},
				},
				MetricTags: MetricTagConfigList{
					{Tag: "license_component", IndexTransform: []MetricIndexTransform{{Start: 1}}},
				},
			},
		},
	}

	cloned := profile.Clone()
	require.Equal(t, profile, cloned)

	cloned.Licensing[0].State.Symbol.Mapping.Items["1"] = "broken"
	cloned.Licensing[0].Signals.Expiry.Sentinel[0] = LicenseSentinelTimerPre1971
	cloned.Licensing[0].MetricTags[0].IndexTransform[0].Start = 2

	assert.Equal(t, "healthy", profile.Licensing[0].State.Symbol.Mapping.Items["1"])
	assert.Equal(t, []LicenseSentinelPolicy{LicenseSentinelTimerU32Max}, profile.Licensing[0].Signals.Expiry.Sentinel)
	assert.Equal(t, uint(1), profile.Licensing[0].MetricTags[0].IndexTransform[0].Start)
}

func TestValidateEnrichProfile_Licensing(t *testing.T) {
	tests := map[string]struct {
		profile         ProfileDefinition
		wantErrContains []string
	}{
		"valid state and expiry": {
			profile: ProfileDefinition{
				Licensing: []LicensingConfig{
					{
						ID: "scalar-group",
						State: LicenseStateConfig{
							LicenseValueConfig: LicenseValueConfig{
								Symbol: SymbolConfig{OID: "1.2.3.0", Name: "licenseState"},
							},
							Policy: LicenseStatePolicyDefault,
						},
						Signals: LicenseSignalsConfig{
							Expiry: LicenseTimerSignalsConfig{
								LicenseValueConfig: LicenseValueConfig{
									Symbol:   SymbolConfig{OID: "1.2.4.0", Name: "licenseExpiry"},
									Sentinel: []LicenseSentinelPolicy{LicenseSentinelTimerU32Max},
								},
							},
						},
					},
				},
			},
		},
		"invalid state policy": {
			profile: ProfileDefinition{
				Licensing: []LicensingConfig{
					{State: LicenseStateConfig{Policy: LicenseStatePolicy("filename_gate")}},
				},
			},
			wantErrContains: []string{`licensing[0].state.policy: invalid policy "filename_gate"`},
		},
		"invalid sentinel policy": {
			profile: ProfileDefinition{
				Licensing: []LicensingConfig{
					{
						Signals: LicenseSignalsConfig{
							Expiry: LicenseTimerSignalsConfig{
								LicenseValueConfig: LicenseValueConfig{
									Sentinel: []LicenseSentinelPolicy{LicenseSentinelPolicy("magic_zero")},
								},
							},
						},
					},
				},
			},
			wantErrContains: []string{`licensing[0].signals.expiry.sentinel[0]: invalid policy "magic_zero"`},
		},
		"invalid signal kind": {
			profile: ProfileDefinition{
				Licensing: []LicensingConfig{
					{
						Signals: LicenseSignalsConfig{
							Usage: LicenseUsageSignalsConfig{
								Used: LicenseValueConfig{Kind: LicenseSignalKind("usage_typo")},
							},
						},
					},
				},
			},
			wantErrContains: []string{`licensing[0].signals.usage.used.kind: invalid kind "usage_typo"`},
		},
		"forbids transform extraction match and underscore symbol names": {
			profile: ProfileDefinition{
				Licensing: []LicensingConfig{
					{
						State: LicenseStateConfig{
							LicenseValueConfig: LicenseValueConfig{
								Symbol: SymbolConfig{
									OID:              "1.2.3.0",
									Name:             "_license_row",
									ChartMeta:        ChartMeta{Description: "chart-only"},
									MetricType:       ProfileMetricTypeGauge,
									Transform:        "{{ .Value }}",
									ExtractValue:     `(\d+)`,
									MatchPattern:     `(\d+)`,
									MatchValue:       "$1",
									ScaleFactor:      0.1,
									ConstantValueOne: true,
								},
							},
						},
					},
				},
			},
			wantErrContains: []string{
				`licensing[0].state.symbol: name "_license_row" cannot be underscore-prefixed`,
				"licensing[0].state.symbol: chart_meta cannot be used in licensing rows",
				"licensing[0].state.symbol: metric_type cannot be used in licensing rows",
				"licensing[0].state.symbol: transform cannot be used in licensing rows",
				"licensing[0].state.symbol: extract_value cannot be used in licensing rows",
				"licensing[0].state.symbol: match_pattern cannot be used in licensing rows",
				"licensing[0].state.symbol: match_value cannot be used in licensing rows",
				"licensing[0].state.symbol: scale_factor cannot be used in licensing rows",
				"licensing[0].state.symbol: constant_value_one cannot be used in licensing rows",
			},
		},
		"forbids state policy without state source": {
			profile: ProfileDefinition{
				Licensing: []LicensingConfig{
					{
						State: LicenseStateConfig{Policy: LicenseStatePolicyDefault},
						Signals: LicenseSignalsConfig{
							Expiry: LicenseTimerSignalsConfig{
								LicenseValueConfig: LicenseValueConfig{Symbol: SymbolConfig{OID: "1.2.4.0", Name: "licenseExpiry"}},
							},
						},
					},
				},
			},
			wantErrContains: []string{"licensing[0].state.policy: policy requires state value source"},
		},
		"forbids scalar literal-only rows without explicit id": {
			profile: ProfileDefinition{
				Licensing: []LicensingConfig{
					{
						State: LicenseStateConfig{
							LicenseValueConfig: LicenseValueConfig{Value: "0"},
						},
					},
				},
			},
			wantErrContains: []string{"licensing[0]: scalar rows without a signal source OID require explicit id"},
		},
		"forbids timer timestamp and remaining together": {
			profile: ProfileDefinition{
				Licensing: []LicensingConfig{
					{
						ID: "scalar-group",
						Signals: LicenseSignalsConfig{
							Expiry: LicenseTimerSignalsConfig{
								Timestamp: LicenseValueConfig{Symbol: SymbolConfig{OID: "1.2.4.0", Name: "licenseExpiry"}},
								Remaining: LicenseValueConfig{Symbol: SymbolConfig{OID: "1.2.5.0", Name: "licenseExpiryRemaining"}},
							},
						},
					},
				},
			},
			wantErrContains: []string{"licensing[0].signals.expiry: timestamp and remaining cannot both be set"},
		},
		"forbids inline timer and remaining together": {
			profile: ProfileDefinition{
				Licensing: []LicensingConfig{
					{
						ID: "scalar-group",
						Signals: LicenseSignalsConfig{
							Expiry: LicenseTimerSignalsConfig{
								LicenseValueConfig: LicenseValueConfig{Symbol: SymbolConfig{OID: "1.2.4.0", Name: "licenseExpiry"}},
								Remaining:          LicenseValueConfig{Symbol: SymbolConfig{OID: "1.2.5.0", Name: "licenseExpiryRemaining"}},
							},
						},
					},
				},
			},
			wantErrContains: []string{"licensing[0].signals.expiry: timestamp and remaining cannot both be set"},
		},
		"forbids legacy underscore top-level names": {
			profile: ProfileDefinition{
				Licensing: []LicensingConfig{
					{
						State: LicenseStateConfig{
							LicenseValueConfig: LicenseValueConfig{
								From: "1.2.3.0",
								Name: "_license_row",
							},
						},
					},
				},
			},
			wantErrContains: []string{`licensing[0].state.name: name "_license_row" cannot be underscore-prefixed`},
		},
		"forbids wrong field kind": {
			profile: ProfileDefinition{
				Licensing: []LicensingConfig{
					{
						State: LicenseStateConfig{
							LicenseValueConfig: LicenseValueConfig{
								Symbol: SymbolConfig{OID: "1.2.3.0", Name: "licenseState"},
								Kind:   LicenseSignalUsageUsed,
							},
						},
					},
				},
			},
			wantErrContains: []string{`licensing[0].state.kind: expected "state_severity", got "usage_used"`},
		},
		"forbids unknown format": {
			profile: ProfileDefinition{
				Licensing: []LicensingConfig{
					{
						State: LicenseStateConfig{
							LicenseValueConfig: LicenseValueConfig{
								Symbol: SymbolConfig{OID: "1.2.3.0", Name: "licenseState", Format: "spreadsheet_date"},
							},
						},
					},
				},
			},
			wantErrContains: []string{`licensing[0].state.symbol: invalid format "spreadsheet_date"`},
		},
		"forbids descriptor-only rows": {
			profile: ProfileDefinition{
				Licensing: []LicensingConfig{
					{
						Identity: LicenseIdentityConfig{
							ID: LicenseValueConfig{Value: "license-a"},
						},
						Descriptors: LicenseDescriptorsConfig{
							Type: LicenseValueConfig{Value: "subscription"},
						},
					},
				},
			},
			wantErrContains: []string{"licensing[0]: must define state or at least one signal"},
		},
		"forbids ungrouped multi scalar signal rows": {
			profile: ProfileDefinition{
				Licensing: []LicensingConfig{
					{
						State: LicenseStateConfig{
							LicenseValueConfig: LicenseValueConfig{
								Symbol: SymbolConfig{OID: "1.2.3.0", Name: "licenseState"},
							},
						},
						Signals: LicenseSignalsConfig{
							Expiry: LicenseTimerSignalsConfig{
								LicenseValueConfig: LicenseValueConfig{
									Symbol: SymbolConfig{OID: "1.2.4.0", Name: "licenseExpiry"},
								},
							},
						},
					},
				},
			},
			wantErrContains: []string{"licensing[0]: scalar rows with multiple signal source OIDs require explicit id"},
		},
		"forbids scalar index lookups": {
			profile: ProfileDefinition{
				Licensing: []LicensingConfig{
					{
						State: LicenseStateConfig{
							LicenseValueConfig: LicenseValueConfig{Index: 1},
						},
					},
				},
			},
			wantErrContains: []string{"licensing[0].state.index: scalar licensing values do not support `index` lookups"},
		},
		"forbids scalar metric tag index lookups": {
			profile: ProfileDefinition{
				Licensing: []LicensingConfig{
					{
						State: LicenseStateConfig{
							LicenseValueConfig: LicenseValueConfig{Symbol: SymbolConfig{OID: "1.2.3.0", Name: "licenseState"}},
						},
						MetricTags: MetricTagConfigList{
							{Tag: "license_row", Index: 1},
						},
					},
				},
			},
			wantErrContains: []string{"licensing[0].metric_tags[0]: scalar metric_tags do not support `index` lookups"},
		},
		"forbids table from outside row table": {
			profile: ProfileDefinition{
				Licensing: []LicensingConfig{
					{
						Table: SymbolConfig{OID: "1.2.3", Name: "licenseTable"},
						State: LicenseStateConfig{
							LicenseValueConfig: LicenseValueConfig{From: "1.2.4.1"},
						},
					},
				},
			},
			wantErrContains: []string{`licensing[0].state.from: OID "1.2.4.1" is outside table "1.2.3"`},
		},
		"forbids duplicate signal kinds for same identity": {
			profile: ProfileDefinition{
				Licensing: []LicensingConfig{
					{
						OriginProfileID: "_vendor-licensing.yaml",
						ID:              "smart",
						State: LicenseStateConfig{
							LicenseValueConfig: LicenseValueConfig{Symbol: SymbolConfig{OID: "1.2.3.0", Name: "licenseState"}},
						},
					},
					{
						OriginProfileID: "_vendor-licensing.yaml",
						ID:              "smart",
						State: LicenseStateConfig{
							LicenseValueConfig: LicenseValueConfig{Symbol: SymbolConfig{OID: "1.2.4.0", Name: "licenseState2"}},
						},
					},
				},
			},
			wantErrContains: []string{`duplicate signal kind "state_severity" for structural identity "_vendor-licensing.yaml|scalar-group|smart"`},
		},
		"allows format and mapping": {
			profile: ProfileDefinition{
				Licensing: []LicensingConfig{
					{
						State: LicenseStateConfig{
							LicenseValueConfig: LicenseValueConfig{
								Symbol: SymbolConfig{
									OID:     "1.2.3.0",
									Name:    "licenseState",
									Format:  "text_date",
									Mapping: NewExactMapping(map[string]string{"1": "healthy"}),
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
			err := ValidateEnrichProfile(&tc.profile)
			if len(tc.wantErrContains) == 0 {
				require.NoError(t, err)
				return
			}

			require.Error(t, err)
			for _, want := range tc.wantErrContains {
				assert.Contains(t, err.Error(), want)
			}
		})
	}
}
