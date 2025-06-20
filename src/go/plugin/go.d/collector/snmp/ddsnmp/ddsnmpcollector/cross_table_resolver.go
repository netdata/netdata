// SPDX-License-Identifier: GPL-3.0-or-later

package ddsnmpcollector

import (
	"fmt"

	"github.com/gosnmp/gosnmp"

	"github.com/netdata/netdata/go/plugins/logger"
	"github.com/netdata/netdata/go/plugins/plugin/go.d/collector/snmp/ddsnmp/ddprofiledefinition"
)

// CrossTableResolver handles resolving tags from other tables
type CrossTableResolver struct {
	log              *logger.Logger
	indexTransformer *IndexTransformer
	tagProcessor     *TagProcessorFactory
}

// NewCrossTableResolver creates a new cross-table resolver
func NewCrossTableResolver(log *logger.Logger) *CrossTableResolver {
	return &CrossTableResolver{
		log:              log,
		indexTransformer: NewIndexTransformer(),
		tagProcessor:     NewTagProcessorFactory(),
	}
}

// CrossTableContext contains all data needed for cross-table resolution
type CrossTableContext struct {
	walkedData     map[string]map[string]gosnmp.SnmpPDU // tableOID -> PDUs
	tableNameToOID map[string]string                    // tableName -> tableOID
}

// ResolveCrossTableTag resolves a tag value from another table
func (r *CrossTableResolver) ResolveCrossTableTag(
	tagCfg ddprofiledefinition.MetricTagConfig,
	index string,
	ctx *CrossTableContext,
) (map[string]string, error) {
	// Find the referenced table's OID
	refTableOID, err := r.findReferencedTableOID(tagCfg.Table, ctx.tableNameToOID)
	if err != nil {
		return nil, err
	}

	// Get the walked data for the referenced table
	refTablePDUs, err := r.getReferencedTableData(tagCfg.Table, refTableOID, ctx.walkedData)
	if err != nil {
		return nil, err
	}

	// Apply index transformation if needed
	lookupIndex, err := r.transformIndex(index, tagCfg.IndexTransform)
	if err != nil {
		return nil, err
	}

	// Look up the value from the referenced table
	pdu, err := r.lookupValue(tagCfg, lookupIndex, refTablePDUs)
	if err != nil {
		return nil, err
	}

	// Process the tag value
	return processTableMetricTagValue(tagCfg, pdu)
}

// findReferencedTableOID finds the OID for a referenced table
func (r *CrossTableResolver) findReferencedTableOID(tableName string, tableNameToOID map[string]string) (string, error) {
	tableOID, ok := tableNameToOID[tableName]
	if !ok {
		r.log.Debugf("Cannot find table OID for referenced table %s", tableName)
		return "", fmt.Errorf("table %s not found", tableName)
	}
	return tableOID, nil
}

// getReferencedTableData gets the walked data for a referenced table
func (r *CrossTableResolver) getReferencedTableData(
	tableName string,
	tableOID string,
	walkedData map[string]map[string]gosnmp.SnmpPDU,
) (map[string]gosnmp.SnmpPDU, error) {
	pdus, ok := walkedData[tableOID]
	if !ok {
		r.log.Debugf("No walked data for referenced table %s (OID: %s)", tableName, tableOID)
		return nil, fmt.Errorf("no data for table %s", tableName)
	}
	return pdus, nil
}

// transformIndex applies index transformation if configured
func (r *CrossTableResolver) transformIndex(index string, transforms []ddprofiledefinition.MetricIndexTransform) (string, error) {
	if len(transforms) == 0 {
		return index, nil
	}

	transformedIndex := r.indexTransformer.ApplyTransform(index, transforms)
	if transformedIndex == "" {
		r.log.Debugf("Index transformation failed for index %s with transforms %v", index, transforms)
		return "", fmt.Errorf("index transformation failed")
	}

	return transformedIndex, nil
}

// lookupValue looks up the value in the referenced table
func (r *CrossTableResolver) lookupValue(
	tagCfg ddprofiledefinition.MetricTagConfig,
	lookupIndex string,
	refTablePDUs map[string]gosnmp.SnmpPDU,
) (gosnmp.SnmpPDU, error) {
	refColumnOID := trimOID(tagCfg.Symbol.OID)
	refFullOID := refColumnOID + "." + lookupIndex

	pdu, ok := refTablePDUs[refFullOID]
	if !ok {
		r.log.Debugf("Cannot find cross-table tag value at OID %s for table %s (lookup index: %s)",
			refFullOID, tagCfg.Table, lookupIndex)
		return gosnmp.SnmpPDU{}, fmt.Errorf("value not found at OID %s", refFullOID)
	}

	return pdu, nil
}

// IsApplicable checks if this tag configuration is for cross-table resolution
func (r *CrossTableResolver) IsApplicable(tagCfg ddprofiledefinition.MetricTagConfig, currentTableName string) bool {
	return tagCfg.Table != "" && tagCfg.Table != currentTableName && tagCfg.Index == 0
}
