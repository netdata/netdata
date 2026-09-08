// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package ddprofiledefinition

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func Test_validateEnrichSymbol_BitmaskMappingRequiresSingleBitKeys(t *testing.T) {
	sym := SymbolConfig{
		OID:  "1.2.3",
		Name: "processorStatus",
		Mapping: NewBitmaskMapping(map[string]string{
			"internal": "internalError",
		}),
	}

	err := validateEnrichSymbol(&sym, scalarSymbol)

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

	err := validateEnrichSymbol(&sym, scalarSymbol)

	if assert.Error(t, err) {
		assert.Contains(t, err.Error(), "requires keys to be 0 or a single power-of-two bit")
		assert.Contains(t, err.Error(), "\"3\"")
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

	err := validateEnrichSymbol(&sym, scalarSymbol)

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

	err := validateEnrichSymbol(&sym, scalarSymbol)

	if assert.Error(t, err) {
		assert.Contains(t, err.Error(), "`mapping.mode` requires `mapping.items`")
	}
}
