// SPDX-License-Identifier: GPL-3.0-or-later

package ddsnmpcollector

import (
	"fmt"
	"net/netip"
	"strings"

	"github.com/gosnmp/gosnmp"

	"github.com/netdata/netdata/go/plugins/plugin/go.d/collector/snmp/ddsnmp/ddprofiledefinition"
)

func (r *crossTableResolver) requiresLookupByValue(tagCfg ddprofiledefinition.MetricTagConfig) bool {
	return tagCfg.LookupSymbol.OID != "" || tagCfg.LookupSymbol.Name != ""
}

func (r *crossTableResolver) resolveLookupIndexByValue(
	tagCfg ddprofiledefinition.MetricTagConfig,
	lookupValue string,
	refTableOID string,
	refTablePDUs map[string]gosnmp.SnmpPDU,
	ctx *crossTableContext,
) (string, error) {
	normalizedLookupValue, ok := r.normalizeLookupValue(ddprofiledefinition.SymbolConfig(tagCfg.LookupSymbol), lookupValue)
	if !ok {
		return "", fmt.Errorf("value '%s' could not be normalized for lookup column %s", lookupValue, tagCfg.LookupSymbol.OID)
	}

	cacheKey := crossTableLookupKey{
		refTableOID:     refTableOID,
		lookupColumnOID: trimOID(tagCfg.LookupSymbol.OID),
		targetColumnOID: trimOID(tagCfg.Symbol.OID),
		lookupValue:     normalizedLookupValue,
	}
	if rowIndex, ok := ctx.lookupIndexCache[cacheKey]; ok {
		if rowIndex == "" {
			return "", fmt.Errorf("value '%s' not found in lookup column %s", normalizedLookupValue, tagCfg.LookupSymbol.OID)
		}
		return rowIndex, nil
	}

	rowIndex, err := r.findRowIndexByLookupValue(tagCfg, normalizedLookupValue, refTablePDUs)
	if err != nil {
		return "", err
	}

	ctx.lookupIndexCache[cacheKey] = rowIndex
	return rowIndex, nil
}

func (r *crossTableResolver) findRowIndexByLookupValue(
	tagCfg ddprofiledefinition.MetricTagConfig,
	lookupValue string,
	refTablePDUs map[string]gosnmp.SnmpPDU,
) (string, error) {
	lookupSym := tagCfg.LookupSymbol
	lookupColumnOID := trimOID(lookupSym.OID)
	prefix := lookupColumnOID + "."
	matchedIndexes := make([]string, 0, 1)

	for fullOID, pdu := range refTablePDUs {
		if !strings.HasPrefix(fullOID, prefix) {
			continue
		}

		val, ok := r.processLookupSymbolValue(ddprofiledefinition.SymbolConfig(lookupSym), pdu)
		if !ok || val != lookupValue {
			continue
		}

		matchedIndexes = append(matchedIndexes, strings.TrimPrefix(fullOID, prefix))
	}

	switch len(matchedIndexes) {
	case 0:
		return "", fmt.Errorf("value '%s' not found in lookup column %s", lookupValue, lookupSym.OID)
	case 1:
		return matchedIndexes[0], nil
	}

	rowIndex, ok := r.resolveAmbiguousLookupRows(tagCfg, matchedIndexes, refTablePDUs)
	if ok {
		return rowIndex, nil
	}

	return "", fmt.Errorf(
		"lookup value '%s' matched multiple rows in %s with different values for %s",
		lookupValue,
		lookupSym.OID,
		tagCfg.Symbol.OID,
	)
}

func (r *crossTableResolver) processLookupSymbolValue(sym ddprofiledefinition.SymbolConfig, pdu gosnmp.SnmpPDU) (string, bool) {
	val, err := convPduToStringf(pdu, sym.Format)
	if err != nil {
		return "", false
	}

	return r.normalizeLookupText(sym, val, false)
}

func (r *crossTableResolver) normalizeLookupValue(sym ddprofiledefinition.SymbolConfig, raw string) (string, bool) {
	return r.normalizeLookupText(sym, raw, true)
}

func (r *crossTableResolver) normalizeLookupText(sym ddprofiledefinition.SymbolConfig, val string, applyFormat bool) (string, bool) {
	if sym.ExtractValueCompiled != nil {
		sm := sym.ExtractValueCompiled.FindStringSubmatch(val)
		if len(sm) > 1 {
			val = sm[1]
		}
	}

	if sym.MatchPatternCompiled != nil {
		sm := sym.MatchPatternCompiled.FindStringSubmatch(val)
		if len(sm) == 0 {
			return "", false
		}
		val = replaceSubmatches(sym.MatchValue, sm)
	}

	if applyFormat && sym.Format != "" {
		formatted, err := formatIndexTagValue(val, sym.Format)
		if err != nil {
			return "", false
		}
		val = formatted
	}

	if sym.Format == "ip_address" {
		if addr, err := netip.ParseAddr(val); err == nil {
			val = addr.Unmap().String()
		}
	}

	if mapped, ok := sym.Mapping.Lookup(val); ok {
		val = mapped
	}

	return val, true
}

func (r *crossTableResolver) resolveAmbiguousLookupRows(
	tagCfg ddprofiledefinition.MetricTagConfig,
	rowIndexes []string,
	refTablePDUs map[string]gosnmp.SnmpPDU,
) (string, bool) {
	wantValue := ""

	for _, rowIndex := range rowIndexes {
		value, ok := r.resolveLookupTargetTagValue(tagCfg, rowIndex, refTablePDUs)
		if !ok {
			return "", false
		}
		if wantValue == "" {
			wantValue = value
			continue
		}
		if wantValue != value {
			return "", false
		}
	}

	return rowIndexes[0], wantValue != ""
}

func (r *crossTableResolver) resolveLookupTargetTagValue(
	tagCfg ddprofiledefinition.MetricTagConfig,
	rowIndex string,
	refTablePDUs map[string]gosnmp.SnmpPDU,
) (string, bool) {
	pdu, err := r.lookupValue(tagCfg, rowIndex, refTablePDUs)
	if err != nil {
		return "", false
	}

	tags := make(map[string]string, 1)
	if err := r.tagProcessor.processTag(tagCfg, pdu, tagAdder{tags: tags}); err != nil {
		return "", false
	}

	tagName := ternary(tagCfg.Tag != "", tagCfg.Tag, tagCfg.Symbol.Name)
	value := tags[tagName]
	return value, value != ""
}
