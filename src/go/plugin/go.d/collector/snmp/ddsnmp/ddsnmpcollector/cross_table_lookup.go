// SPDX-License-Identifier: GPL-3.0-or-later

package ddsnmpcollector

import (
	"fmt"
	"maps"
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
	rowIndex string,
	refTableOID string,
	refTablePDUs map[string]gosnmp.SnmpPDU,
	ctx *crossTableContext,
) (string, error) {
	normalizedLookupValue, ok := r.normalizeLookupValue(
		ddprofiledefinition.SymbolConfig(tagCfg.LookupSymbol),
		lookupValue,
	)
	if !ok {
		return "", fmt.Errorf(
			"value '%s' could not be normalized for lookup column %s",
			lookupValue,
			tagCfg.LookupSymbol.OID,
		)
	}

	valueIndex := r.lookupValueIndex(tagCfg, refTableOID, refTablePDUs, ctx)
	var matched []string
	if tagCfg.LookupScope != nil {
		scope, err := r.lookupScopeIndex(rowIndex, tagCfg.LookupScope.SourceIndexTransform)
		if err != nil {
			return "", err
		}
		matched = valueIndex.rowsByScope[scopedLookupValue{scope, normalizedLookupValue}]
	} else {
		matched = valueIndex.rowsByValue[normalizedLookupValue]
	}
	return r.findRowIndexByLookupValue(tagCfg, normalizedLookupValue, matched, refTablePDUs)
}

func (r *crossTableResolver) findRowIndexByLookupValue(
	tagCfg ddprofiledefinition.MetricTagConfig,
	lookupValue string,
	matchedIndexes []string,
	refTablePDUs map[string]gosnmp.SnmpPDU,
) (string, error) {
	lookupSym := tagCfg.LookupSymbol

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

func (r *crossTableResolver) lookupValueIndex(
	tagCfg ddprofiledefinition.MetricTagConfig,
	refTableOID string,
	refTablePDUs map[string]gosnmp.SnmpPDU,
	ctx *crossTableContext,
) *crossTableLookupValueIndex {
	lookupSym := ddprofiledefinition.SymbolConfig(tagCfg.LookupSymbol)
	extractPattern := ""
	if lookupSym.ExtractValueCompiled != nil {
		extractPattern = lookupSym.ExtractValueCompiled.String()
	}
	matchPattern := ""
	if lookupSym.MatchPatternCompiled != nil {
		matchPattern = lookupSym.MatchPatternCompiled.String()
	}
	key := crossTableLookupValueIndexKey{
		refTableOID:     trimOID(refTableOID),
		lookupColumnOID: trimOID(lookupSym.OID),
		format:          lookupSym.Format,
		extractPattern:  extractPattern,
		matchPattern:    matchPattern,
		matchValue:      lookupSym.MatchValue,
	}

	var targetScope []ddprofiledefinition.MetricIndexTransform
	if tagCfg.LookupScope != nil {
		targetScope = tagCfg.LookupScope.TargetIndexTransform
	}
	for _, index := range ctx.lookupValueIndexes[key] {
		if lookupMappingsEqual(index.mapping, lookupSym.Mapping) && indexTransformsEqual(index.targetScope, targetScope) {
			return index
		}
	}

	index := &crossTableLookupValueIndex{mapping: lookupSym.Mapping.Clone()}
	if len(targetScope) == 0 {
		index.rowsByValue = make(map[string][]string)
	} else {
		index.rowsByScope = make(map[scopedLookupValue][]string)
		index.targetScope = tagCfg.LookupScope.Clone().TargetIndexTransform
	}
	prefix := key.lookupColumnOID + "."
	for fullOID, pdu := range refTablePDUs {
		if !strings.HasPrefix(fullOID, prefix) {
			continue
		}

		value, ok := r.processLookupSymbolValue(lookupSym, pdu)
		if !ok {
			continue
		}
		rowIndex := strings.TrimPrefix(fullOID, prefix)
		if len(targetScope) == 0 {
			index.rowsByValue[value] = append(index.rowsByValue[value], rowIndex)
			continue
		}
		scope, err := r.lookupScopeIndex(rowIndex, targetScope)
		if err != nil {
			continue
		}
		lookup := scopedLookupValue{scope, value}
		index.rowsByScope[lookup] = append(index.rowsByScope[lookup], rowIndex)
	}

	if ctx.lookupValueIndexes == nil {
		ctx.lookupValueIndexes = make(map[crossTableLookupValueIndexKey][]*crossTableLookupValueIndex)
	}
	ctx.lookupValueIndexes[key] = append(ctx.lookupValueIndexes[key], index)
	return index
}

func indexTransformsEqual(a, b []ddprofiledefinition.MetricIndexTransform) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if a[i].Start != b[i].Start || a[i].End != b[i].End || a[i].DropRight != b[i].DropRight || !maps.Equal(a[i].Mapping, b[i].Mapping) {
			return false
		}
	}
	return true
}

// A scope is an OID index, not display text. Reject malformed keys rather than joining them.
func (r *crossTableResolver) lookupScopeIndex(index string, transforms []ddprofiledefinition.MetricIndexTransform) (string, error) {
	if len(transforms) == 0 {
		return "", fmt.Errorf("lookup scope requires an index transform")
	}
	scope, err := r.transformIndex(index, transforms)
	if err != nil {
		return "", err
	}
	for _, arc := range strings.Split(scope, ".") {
		if _, err := parseIndexArc(arc, 32); err != nil {
			return "", fmt.Errorf("lookup scope contains an invalid index arc")
		}
	}
	return scope, nil
}

func lookupMappingsEqual(a, b ddprofiledefinition.MappingConfig) bool {
	return a.EffectiveMode() == b.EffectiveMode() && maps.Equal(a.Items, b.Items)
}

func (r *crossTableResolver) processLookupSymbolValue(
	sym ddprofiledefinition.SymbolConfig,
	pdu gosnmp.SnmpPDU,
) (string, bool) {
	val, err := convPduToStringf(pdu, sym.Format)
	if err != nil {
		return "", false
	}

	return r.normalizeLookupText(sym, val, false)
}

func (r *crossTableResolver) normalizeLookupValue(sym ddprofiledefinition.SymbolConfig, raw string) (string, bool) {
	return r.normalizeLookupText(sym, raw, true)
}

func (r *crossTableResolver) normalizeLookupText(
	sym ddprofiledefinition.SymbolConfig,
	val string,
	applyFormat bool,
) (string, bool) {
	if sym.ExtractValueCompiled != nil {
		sm := sym.ExtractValueCompiled.FindStringSubmatch(val)
		if len(sm) > 1 {
			val = sm[1]
		}
	}
	return r.finishLookupText(sym, val, applyFormat)
}

func (r *crossTableResolver) finishLookupText(
	sym ddprofiledefinition.SymbolConfig,
	val string,
	applyFormat bool,
) (string, bool) {
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
	pdu, failure := lookupTableColumn(tagCfg.Symbol.OID, rowIndex, refTablePDUs)
	if failure != nil {
		return "", false
	}

	tags := make(map[string]string, 1)
	if err := r.tagProcessor.processTag(tagCfg, pdu, tagAdder{
		tags: tags,
	}); err != nil {
		return "", false
	}

	tagName := ternary(tagCfg.Tag != "", tagCfg.Tag, tagCfg.Symbol.Name)
	value := tags[tagName]
	return value, value != ""
}
