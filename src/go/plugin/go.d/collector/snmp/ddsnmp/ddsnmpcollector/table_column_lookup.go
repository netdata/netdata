// SPDX-License-Identifier: GPL-3.0-or-later

package ddsnmpcollector

import (
	"fmt"

	"github.com/gosnmp/gosnmp"

	"github.com/netdata/netdata/go/plugins/plugin/go.d/collector/snmp/ddsnmp/ddprofiledefinition"
)

type tableLookupError struct {
	oid, reason string
	err         error
}

func (e *tableLookupError) Error() string { return e.err.Error() }
func (e *tableLookupError) Unwrap() error { return e.err }

// lookupTagPDU resolves only the source. Tag and typed-value consumers keep their
// own final conversion, omission and diagnostic policies.
func (r *crossTableResolver) lookupTagPDU(
	tag ddprofiledefinition.MetricTagConfig,
	index string,
	ctx *crossTableContext,
) (gosnmp.SnmpPDU, *tableLookupError) {
	lookupIndex, err := r.transformIndex(index, tag.IndexTransform)
	if err != nil {
		return gosnmp.SnmpPDU{}, &tableLookupError{
			oid:    tag.Symbol.OID,
			reason: "dependency_processing",
			err:    err,
		}
	}
	for _, source := range tag.IndexFrom {
		_, pdus, failure := r.referencedTable(source.Table, source.Symbol.OID, ctx)
		if failure != nil {
			return gosnmp.SnmpPDU{}, failure
		}
		pdu, failure := lookupTableColumn(source.Symbol.OID, lookupIndex, pdus)
		if failure != nil {
			return gosnmp.SnmpPDU{}, failure
		}
		value, reason, err := r.processIndexSourceValue(source.Symbol, pdu)
		if err != nil {
			return gosnmp.SnmpPDU{}, &tableLookupError{
				oid:    trimOID(source.Symbol.OID) + "." + lookupIndex,
				reason: reason,
				err:    err,
			}
		}
		lookupIndex = value
	}

	root, pdus, failure := r.referencedTable(tag.Table, tag.Symbol.OID, ctx)
	if failure != nil {
		return gosnmp.SnmpPDU{}, failure
	}
	if r.requiresLookupByValue(tag) {
		lookupIndex, err = r.resolveLookupIndexByValue(tag, lookupIndex, root, pdus, ctx)
		if err != nil {
			return gosnmp.SnmpPDU{}, &tableLookupError{
				oid:    tag.LookupSymbol.OID,
				reason: "dependency_processing",
				err:    err,
			}
		}
	}
	return lookupTableColumn(tag.Symbol.OID, lookupIndex, pdus)
}

func (r *crossTableResolver) referencedTable(
	table, column string,
	ctx *crossTableContext,
) (string, map[string]gosnmp.SnmpPDU, *tableLookupError) {
	root, err := r.findReferencedTableOID(table, ctx.tableNameToOID)
	if err != nil {
		return "", nil, &tableLookupError{
			oid:    column,
			reason: "dependency_processing",
			err:    err,
		}
	}
	pdus, err := r.getReferencedTableData(table, root, ctx.walkedData)
	if err != nil {
		return "", nil, &tableLookupError{
			oid:    column,
			reason: "dependency_processing",
			err:    err,
		}
	}
	return root, pdus, nil
}

func lookupTableColumn(column, index string, pdus map[string]gosnmp.SnmpPDU) (gosnmp.SnmpPDU, *tableLookupError) {
	oid := trimOID(column) + "." + index
	pdu, ok := pdus[oid]
	if !ok {
		return gosnmp.SnmpPDU{}, &tableLookupError{
			oid:    oid,
			reason: "missing_dependency",
			err:    fmt.Errorf("value not found at OID %s", oid),
		}
	}
	return pdu, nil
}

// Unlike legacy lookup_symbol extraction, an index source must fail closed:
// reusing an unextracted value could select an unrelated row in the next table.
func (r *crossTableResolver) processIndexSourceValue(
	sym ddprofiledefinition.SymbolConfig,
	pdu gosnmp.SnmpPDU,
) (string, string, error) {
	value, err := convPduToStringf(pdu, sym.Format)
	if err != nil {
		return "", "conversion", err
	}
	if sym.ExtractValueCompiled != nil {
		match := sym.ExtractValueCompiled.FindStringSubmatch(value)
		if len(match) < 2 || match[1] == "" {
			return "", "extract_mismatch", fmt.Errorf("index source %s did not produce a capture", sym.Name)
		}
		value = match[1]
	}
	value, ok := r.finishLookupText(sym, value, false)
	if !ok {
		return "", "pattern_mismatch", fmt.Errorf("index source %s did not match its pattern", sym.Name)
	}
	if value == "" {
		return "", "dependency_processing", fmt.Errorf("index source %s produced an empty key", sym.Name)
	}
	return value, "", nil
}
