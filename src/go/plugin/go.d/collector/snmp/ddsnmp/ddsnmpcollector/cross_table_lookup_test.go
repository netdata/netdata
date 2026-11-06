// SPDX-License-Identifier: GPL-3.0-or-later

package ddsnmpcollector

import (
	"testing"

	"github.com/gosnmp/gosnmp"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/netdata/netdata/go/plugins/logger"
	"github.com/netdata/netdata/go/plugins/plugin/go.d/collector/snmp/ddsnmp/ddprofiledefinition"
)

func TestCrossTableResolver_ResolveLookupIndexByValue_NormalizesIPv6CurrentRowIndex(t *testing.T) {
	resolver := newCrossTableResolver(logger.New())
	tagCfg := lookupTestTagConfig(
		"neighbor",
		"1.3.6.1.4.1.2011.5.25.177.1.1.2.1.4",
		"neighbor",
		"1.3.6.1.4.1.2011.5.25.177.1.1.2.1.4",
	)

	refTablePDUs := map[string]gosnmp.SnmpPDU{
		"1.3.6.1.4.1.2011.5.25.177.1.1.2.1.4.0.2.1.2.16.32.1.18.248.0.0.0.0.0.0.0.0.2.35.2.83": createStringPDU(
			"1.3.6.1.4.1.2011.5.25.177.1.1.2.1.4.0.2.1.2.16.32.1.18.248.0.0.0.0.0.0.0.0.2.35.2.83",
			"2001:12F8::223:253",
		),
	}

	rowIndex, err := resolver.resolveLookupIndexByValue(
		tagCfg,
		"0.0.16.32.1.18.248.0.0.0.0.0.0.0.0.2.35.2.83",
		"1.3.6.1.4.1.2011.5.25.177.1.1.2",
		refTablePDUs,
		newCrossTableContext(nil, nil),
	)
	require.NoError(t, err)
	assert.Equal(t, "0.2.1.2.16.32.1.18.248.0.0.0.0.0.0.0.0.2.35.2.83", rowIndex)
}

func TestCrossTableResolver_ResolveLookupIndexByValue_AllowsDuplicateRowsWhenTargetValueMatches(t *testing.T) {
	resolver := newCrossTableResolver(logger.New())
	tagCfg := lookupTestTagConfig(
		"remote_as",
		"1.3.6.1.4.1.2011.5.25.177.1.1.2.1.2",
		"neighbor",
		"1.3.6.1.4.1.2011.5.25.177.1.1.2.1.4",
	)

	refTablePDUs := map[string]gosnmp.SnmpPDU{
		"1.3.6.1.4.1.2011.5.25.177.1.1.2.1.4.0.1.1.1.4.10.45.2.2": createStringPDU(
			"1.3.6.1.4.1.2011.5.25.177.1.1.2.1.4.0.1.1.1.4.10.45.2.2",
			"10.45.2.2",
		),
		"1.3.6.1.4.1.2011.5.25.177.1.1.2.1.4.0.1.128.1.4.10.45.2.2": createStringPDU(
			"1.3.6.1.4.1.2011.5.25.177.1.1.2.1.4.0.1.128.1.4.10.45.2.2",
			"10.45.2.2",
		),
		"1.3.6.1.4.1.2011.5.25.177.1.1.2.1.2.0.1.1.1.4.10.45.2.2": createGauge32PDU(
			"1.3.6.1.4.1.2011.5.25.177.1.1.2.1.2.0.1.1.1.4.10.45.2.2",
			26479,
		),
		"1.3.6.1.4.1.2011.5.25.177.1.1.2.1.2.0.1.128.1.4.10.45.2.2": createGauge32PDU(
			"1.3.6.1.4.1.2011.5.25.177.1.1.2.1.2.0.1.128.1.4.10.45.2.2",
			26479,
		),
	}

	rowIndex, err := resolver.resolveLookupIndexByValue(
		tagCfg,
		"0.0.4.10.45.2.2",
		"1.3.6.1.4.1.2011.5.25.177.1.1.2",
		refTablePDUs,
		newCrossTableContext(nil, nil),
	)
	require.NoError(t, err)
	assert.Contains(t, []string{
		"0.1.1.1.4.10.45.2.2",
		"0.1.128.1.4.10.45.2.2",
	}, rowIndex)
}

func TestCrossTableResolver_ResolveLookupIndexByValue_RejectsDuplicateRowsWhenTargetValueDiffers(t *testing.T) {
	resolver := newCrossTableResolver(logger.New())
	tagCfg := lookupTestTagConfig(
		"remote_as",
		"1.3.6.1.4.1.2011.5.25.177.1.1.2.1.2",
		"neighbor",
		"1.3.6.1.4.1.2011.5.25.177.1.1.2.1.4",
	)

	refTablePDUs := map[string]gosnmp.SnmpPDU{
		"1.3.6.1.4.1.2011.5.25.177.1.1.2.1.4.0.1.1.1.4.10.45.2.2": createStringPDU(
			"1.3.6.1.4.1.2011.5.25.177.1.1.2.1.4.0.1.1.1.4.10.45.2.2",
			"10.45.2.2",
		),
		"1.3.6.1.4.1.2011.5.25.177.1.1.2.1.4.0.1.128.1.4.10.45.2.2": createStringPDU(
			"1.3.6.1.4.1.2011.5.25.177.1.1.2.1.4.0.1.128.1.4.10.45.2.2",
			"10.45.2.2",
		),
		"1.3.6.1.4.1.2011.5.25.177.1.1.2.1.2.0.1.1.1.4.10.45.2.2": createGauge32PDU(
			"1.3.6.1.4.1.2011.5.25.177.1.1.2.1.2.0.1.1.1.4.10.45.2.2",
			26479,
		),
		"1.3.6.1.4.1.2011.5.25.177.1.1.2.1.2.0.1.128.1.4.10.45.2.2": createGauge32PDU(
			"1.3.6.1.4.1.2011.5.25.177.1.1.2.1.2.0.1.128.1.4.10.45.2.2",
			64512,
		),
	}

	_, err := resolver.resolveLookupIndexByValue(
		tagCfg,
		"0.0.4.10.45.2.2",
		"1.3.6.1.4.1.2011.5.25.177.1.1.2",
		refTablePDUs,
		newCrossTableContext(nil, nil),
	)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "matched multiple rows")
	assert.Contains(t, err.Error(), tagCfg.Symbol.OID)
}

func TestCrossTableResolver_ResolveLookupIndexByValue_DoesNotCacheLookupErrorsAsNotFound(t *testing.T) {
	resolver := newCrossTableResolver(logger.New())
	tagCfg := lookupTestTagConfig(
		"remote_as",
		"1.3.6.1.4.1.2011.5.25.177.1.1.2.1.2",
		"neighbor",
		"1.3.6.1.4.1.2011.5.25.177.1.1.2.1.4",
	)

	refTablePDUs := map[string]gosnmp.SnmpPDU{
		"1.3.6.1.4.1.2011.5.25.177.1.1.2.1.4.0.1.1.1.4.10.45.2.2": createStringPDU(
			"1.3.6.1.4.1.2011.5.25.177.1.1.2.1.4.0.1.1.1.4.10.45.2.2",
			"10.45.2.2",
		),
		"1.3.6.1.4.1.2011.5.25.177.1.1.2.1.4.0.1.128.1.4.10.45.2.2": createStringPDU(
			"1.3.6.1.4.1.2011.5.25.177.1.1.2.1.4.0.1.128.1.4.10.45.2.2",
			"10.45.2.2",
		),
		"1.3.6.1.4.1.2011.5.25.177.1.1.2.1.2.0.1.1.1.4.10.45.2.2": createGauge32PDU(
			"1.3.6.1.4.1.2011.5.25.177.1.1.2.1.2.0.1.1.1.4.10.45.2.2",
			26479,
		),
		"1.3.6.1.4.1.2011.5.25.177.1.1.2.1.2.0.1.128.1.4.10.45.2.2": createGauge32PDU(
			"1.3.6.1.4.1.2011.5.25.177.1.1.2.1.2.0.1.128.1.4.10.45.2.2",
			64512,
		),
	}
	ctx := newCrossTableContext(nil, nil)

	for range 2 {
		_, err := resolver.resolveLookupIndexByValue(
			tagCfg,
			"0.0.4.10.45.2.2",
			"1.3.6.1.4.1.2011.5.25.177.1.1.2",
			refTablePDUs,
			ctx,
		)
		require.Error(t, err)
		assert.Contains(t, err.Error(), "matched multiple rows")
		assert.Contains(t, err.Error(), tagCfg.Symbol.OID)
	}
}

func TestCrossTableResolver_ResolveLookupIndexByValue_SeparatesNormalizationSemantics(t *testing.T) {
	const (
		refTableOID = "1.2.3"
		lookupOID   = "1.2.3.1"
		targetOID   = "1.2.3.2"
	)

	resolver := newCrossTableResolver(logger.New())
	ctx := newCrossTableContext(nil, nil)
	prefixedPDUs := map[string]gosnmp.SnmpPDU{
		lookupOID + ".1": createStringPDU(lookupOID+".1", "A:one"),
		lookupOID + ".2": createStringPDU(lookupOID+".2", "B:two"),
	}
	mappedPDUs := map[string]gosnmp.SnmpPDU{
		lookupOID + ".1": createStringPDU(lookupOID+".1", "one"),
		lookupOID + ".2": createStringPDU(lookupOID+".2", "two"),
	}
	binaryPDUs := map[string]gosnmp.SnmpPDU{
		lookupOID + ".1": createPDU(lookupOID+".1", gosnmp.OctetString, []byte{1, 2}),
		lookupOID + ".2": createPDU(lookupOID+".2", gosnmp.OctetString, []byte{10, 11}),
	}

	tests := []struct {
		name        string
		refPDUs     map[string]gosnmp.SnmpPDU
		lookupValue string
		wantIndex   string
		configure   func(*ddprofiledefinition.SymbolConfigCompat)
	}{
		{
			name: "extract first", refPDUs: prefixedPDUs, lookupValue: "A:one", wantIndex: "1",
			configure: configureLookupExtract(`^A:(.*)$`),
		},
		{
			name: "extract second", refPDUs: prefixedPDUs, lookupValue: "B:two", wantIndex: "2",
			configure: configureLookupExtract(`^B:(.*)$`),
		},
		{
			name: "mapping first", refPDUs: mappedPDUs, lookupValue: "one", wantIndex: "1",
			configure: configureLookupMapping(map[string]string{"one": "shared"}),
		},
		{
			name: "mapping second", refPDUs: mappedPDUs, lookupValue: "two", wantIndex: "2",
			configure: configureLookupMapping(map[string]string{"two": "shared"}),
		},
		{
			name: "match first", refPDUs: prefixedPDUs, lookupValue: "A:one", wantIndex: "1",
			configure: configureLookupMatch(`^A:(.*)$`),
		},
		{
			name: "match second", refPDUs: prefixedPDUs, lookupValue: "B:two", wantIndex: "2",
			configure: configureLookupMatch(`^B:(.*)$`),
		},
		{name: "raw format", refPDUs: binaryPDUs, lookupValue: string([]byte{1, 2}), wantIndex: "1"},
		{
			name: "hex format", refPDUs: binaryPDUs, lookupValue: "10.11", wantIndex: "2",
			configure: func(sym *ddprofiledefinition.SymbolConfigCompat) { sym.Format = "hex" },
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			tagCfg := ddprofiledefinition.MetricTagConfig{
				Tag:          "target",
				Symbol:       ddprofiledefinition.SymbolConfigCompat{OID: targetOID, Name: "target"},
				LookupSymbol: ddprofiledefinition.SymbolConfigCompat{OID: lookupOID, Name: "lookup"},
			}
			if tc.configure != nil {
				tc.configure(&tagCfg.LookupSymbol)
			}

			rowIndex, err := resolver.resolveLookupIndexByValue(
				tagCfg,
				tc.lookupValue,
				refTableOID,
				tc.refPDUs,
				ctx,
			)
			require.NoError(t, err)
			assert.Equal(t, tc.wantIndex, rowIndex)
		})
	}

	var indexes int
	for _, variants := range ctx.lookupValueIndexes {
		indexes += len(variants)
	}
	assert.Equal(t, 8, indexes)
}

func configureLookupExtract(pattern string) func(*ddprofiledefinition.SymbolConfigCompat) {
	return func(sym *ddprofiledefinition.SymbolConfigCompat) {
		sym.ExtractValue = pattern
		sym.ExtractValueCompiled = mustCompileRegex(pattern)
	}
}

func configureLookupMatch(pattern string) func(*ddprofiledefinition.SymbolConfigCompat) {
	return func(sym *ddprofiledefinition.SymbolConfigCompat) {
		sym.MatchPattern = pattern
		sym.MatchPatternCompiled = mustCompileRegex(pattern)
		sym.MatchValue = "$1"
	}
}

func configureLookupMapping(mapping map[string]string) func(*ddprofiledefinition.SymbolConfigCompat) {
	return func(sym *ddprofiledefinition.SymbolConfigCompat) {
		sym.Mapping = ddprofiledefinition.NewExactMapping(mapping)
	}
}

func lookupTestTagConfig(tagName, symbolOID, lookupName, lookupOID string) ddprofiledefinition.MetricTagConfig {
	return ddprofiledefinition.MetricTagConfig{
		Tag: tagName,
		Symbol: ddprofiledefinition.SymbolConfigCompat{
			OID:  symbolOID,
			Name: tagName,
		},
		LookupSymbol: ddprofiledefinition.SymbolConfigCompat{
			OID:                  lookupOID,
			Name:                 lookupName,
			Format:               "ip_address",
			ExtractValue:         `^(?:[^.]+\.){2}(?:4|16)\.(.*)$`,
			ExtractValueCompiled: mustCompileRegex(`^(?:[^.]+\.){2}(?:4|16)\.(.*)$`),
		},
	}
}
