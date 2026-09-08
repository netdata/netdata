// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package ddprofiledefinition

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func Test_validateEnrichSymbol_Mapping(t *testing.T) {
	tests := map[string]struct {
		symbol          SymbolConfig
		wantErrContains []string
	}{
		"bitmask requires numeric bit keys": {symbol: SymbolConfig{
			OID:  "1.2.3",
			Name: "processorStatus",
			Mapping: MappingConfig{
				Mode: MappingModeBitmask,
				Items: map[string]string{
					"internal": "internalError",
				},
			},
		},
			wantErrContains: []string{"requires keys to be 0 or a single power-of-two bit"}},
		"bitmask rejects composite masks": {symbol: SymbolConfig{
			OID:  "1.2.3",
			Name: "processorStatus",
			Mapping: MappingConfig{
				Mode: MappingModeBitmask,
				Items: map[string]string{
					"3": "combinedFault",
				},
			},
		},
			wantErrContains: []string{"requires keys to be 0 or a single power-of-two bit", "\"3\""}},
		"bitmask rejects scale factor": {symbol: SymbolConfig{
			OID:         "1.2.3",
			Name:        "processorStatus",
			ScaleFactor: 2,
			Mapping: MappingConfig{
				Mode: MappingModeBitmask,
				Items: map[string]string{
					"1":   "internalError",
					"128": "processorPresent",
				},
			},
		},
			wantErrContains: []string{"`scale_factor` cannot be used with `mapping.mode: bitmask`"}},
		"mapping mode requires items": {symbol: SymbolConfig{
			OID:  "1.2.3",
			Name: "processorStatus",
			Mapping: MappingConfig{
				Mode: MappingModeBitmask,
			},
		},
			wantErrContains: []string{"`mapping.mode` requires `mapping.items`"}},
	}
	for name, tc := range tests {
		t.Run(name, func(t *testing.T) {
			err := validateEnrichSymbol(&tc.symbol, scalarSymbol)
			require.Error(t, err)
			for _, message := range tc.wantErrContains {
				assert.ErrorContains(t, err, message)
			}
		})
	}
}
