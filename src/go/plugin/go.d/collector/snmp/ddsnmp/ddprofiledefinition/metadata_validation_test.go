// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package ddprofiledefinition

import (
	"regexp"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func Test_validateEnrichMetadata(t *testing.T) {
	tests := map[string]struct {
		metadata        MetadataConfig
		wantError       bool
		wantErrContains string
		wantMetadata    MetadataConfig
	}{
		"bitmask mapping unsupported": {metadata: MetadataConfig{
			"device": {
				Fields: map[string]MetadataField{
					"description": {
						Symbol: SymbolConfig{
							OID:  "1.2.3",
							Name: "deviceStatus",
							Mapping: MappingConfig{
								Mode: MappingModeBitmask,
								Items: map[string]string{
									"1": "internalError",
								},
							},
						},
					},
				},
			},
		}, wantError: true, wantErrContains: "only supported for scalar/table metric symbols"},
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
								OID:    "1.3.6.1.2.1.17.1.1.0",
								Name:   "dot1dBaseBridgeAddress",
								Format: "hex",
							},
							Consumers: ConsumerSet{ConsumerTopology},
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
	}

	for name, tc := range tests {
		t.Run(name, func(t *testing.T) {
			err := validateEnrichMetadata(tc.metadata)
			if tc.wantError {
				require.Error(t, err)
				if tc.wantErrContains != "" {
					assert.ErrorContains(t, err, tc.wantErrContains)
				}
			} else {
				require.NoError(t, err)
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
