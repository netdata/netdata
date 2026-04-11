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
		&crossTableContext{lookupIndexCache: map[crossTableLookupKey]string{}},
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
		&crossTableContext{lookupIndexCache: map[crossTableLookupKey]string{}},
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
		&crossTableContext{lookupIndexCache: map[crossTableLookupKey]string{}},
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
	ctx := &crossTableContext{lookupIndexCache: map[crossTableLookupKey]string{}}

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
	assert.Empty(t, ctx.lookupIndexCache)
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
