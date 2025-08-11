package ddsnmpcollector

import (
	"fmt"
	"strings"

	"github.com/gosnmp/gosnmp"

	"github.com/netdata/netdata/go/plugins/logger"
	"github.com/netdata/netdata/go/plugins/plugin/go.d/collector/snmp/ddsnmp"
	"github.com/netdata/netdata/go/plugins/plugin/go.d/collector/snmp/ddsnmp/ddprofiledefinition"
)

type (
	// tableRowProcessor processes individual rows from SNMP tables
	tableRowProcessor struct {
		log                *logger.Logger
		crossTableResolver *crossTableResolver
		valProc            *valueProcessor
		tagProc            *tableTagProcessor
	}
	// tableRowData represents data for a single table row
	tableRowData struct {
		index      string
		pdus       map[string]gosnmp.SnmpPDU
		tags       map[string]string
		staticTags map[string]string
		tableName  string
	}
	// tableRowProcessingContext contains context needed for processing a row
	tableRowProcessingContext struct {
		config        ddprofiledefinition.MetricsConfig
		columnOIDs    map[string]ddprofiledefinition.SymbolConfig
		crossTableCtx *crossTableContext
		orderedTags   []orderedTagConfig
	}
)

func newTableRowProcessor(log *logger.Logger) *tableRowProcessor {
	return &tableRowProcessor{
		log:                log,
		valProc:            newValueProcessor(),
		tagProc:            newTableTagProcessor(),
		crossTableResolver: newCrossTableResolver(log),
	}
}

func (p *tableRowProcessor) processRow(row *tableRowData, ctx *tableRowProcessingContext) ([]ddsnmp.Metric, error) {
	if err := p.processRowTags(row, ctx); err != nil {
		p.log.Debugf("Error processing tags for row %s: %v", row.index, err)
	}

	return p.processRowMetrics(row, ctx)
}

func (p *tableRowProcessor) processRowTags(row *tableRowData, ctx *tableRowProcessingContext) error {
	// Collect tags in the order they appear in the profile
	for _, orderedTag := range ctx.orderedTags {
		switch orderedTag.tagType {
		case tagTypeSameTable:
			p.processSingleSameTableTag(row, orderedTag.config)
		case tagTypeCrossTable:
			if ctx.crossTableCtx != nil {
				p.processSingleCrossTableTag(row, orderedTag.config, ctx)
			}
		case tagTypeIndex:
			p.processSingleIndexTag(row, orderedTag.config)
		}
	}

	return nil
}

func (p *tableRowProcessor) processSingleSameTableTag(row *tableRowData, tagCfg ddprofiledefinition.MetricTagConfig) {
	columnOID := trimOID(tagCfg.Symbol.OID)
	pdu, ok := row.pdus[columnOID]
	if !ok {
		return
	}

	ta := tagAdder{tags: row.tags}
	if err := p.tagProc.processTag(tagCfg, pdu, ta); err != nil {
		p.log.Debugf("Error processing tag %s: %v", tagCfg.Tag, err)
	}
}

func (p *tableRowProcessor) processSingleCrossTableTag(row *tableRowData, tagCfg ddprofiledefinition.MetricTagConfig, ctx *tableRowProcessingContext) {
	if err := p.crossTableResolver.resolveCrossTableTag(tagCfg, row.index, ctx.crossTableCtx); err != nil {
		p.log.Debugf("Error resolving cross-table tag %s: %v", tagCfg.Tag, err)
	}
}

func (p *tableRowProcessor) processSingleIndexTag(row *tableRowData, tagCfg ddprofiledefinition.MetricTagConfig) {
	tagName, indexValue, ok := p.processIndexTag(tagCfg, row.index)
	if !ok {
		p.log.Debugf("Cannot extract position %d from index %s", tagCfg.Index, row.index)
		return
	}

	ta := tagAdder{tags: row.tags}
	ta.addTag(tagName, indexValue)
}

func (p *tableRowProcessor) processIndexTag(cfg ddprofiledefinition.MetricTagConfig, index string) (string, string, bool) {
	indexValue, ok := p.extractIndexPosition(index, cfg.Index)
	if !ok {
		return "", "", false
	}

	tagName := ternary(cfg.Tag != "", cfg.Tag, fmt.Sprintf("index%d", cfg.Index))

	if v, ok := cfg.Mapping[indexValue]; ok {
		indexValue = v
	}

	return tagName, indexValue, true
}

// extractPosition extracts a specific position from an index
// Position uses 1-based indexing as per the profile format
// Example: index "7.8.9", position 2 → "8"
func (p *tableRowProcessor) extractIndexPosition(index string, position uint) (string, bool) {
	if position == 0 {
		return "", false
	}

	var n uint
	for {
		n++
		i := strings.IndexByte(index, '.')
		if i == -1 {
			break
		}
		if n == position {
			return index[:i], true
		}
		index = index[i+1:]
	}

	return index, n == position && index != ""
}

func (p *tableRowProcessor) processRowMetrics(row *tableRowData, ctx *tableRowProcessingContext) ([]ddsnmp.Metric, error) {
	metrics := make([]ddsnmp.Metric, 0, len(ctx.columnOIDs))

	for columnOID, sym := range ctx.columnOIDs {
		pdu, ok := row.pdus[columnOID]
		if !ok {
			continue
		}

		metric, err := p.createMetric(sym, pdu, row)
		if err != nil {
			p.log.Debugf("Error creating metric %s: %v", sym.Name, err)
			continue
		}

		metrics = append(metrics, *metric)
	}

	return metrics, nil
}

func (p *tableRowProcessor) createMetric(sym ddprofiledefinition.SymbolConfig, pdu gosnmp.SnmpPDU, row *tableRowData) (*ddsnmp.Metric, error) {
	value, err := p.valProc.processValue(sym, pdu)
	if err != nil {
		return nil, fmt.Errorf("error processing value: %w", err)
	}

	return buildTableMetric(sym, pdu, value, row.tags, row.staticTags, row.tableName)
}

type (
	// crossTableResolver handles resolving tags from other tables
	crossTableResolver struct {
		log          *logger.Logger
		tagProcessor *tableTagProcessor
	}
	// crossTableContext contains all data needed for cross-table resolution
	crossTableContext struct {
		walkedData     map[string]map[string]gosnmp.SnmpPDU // tableOID -> PDUs
		tableNameToOID map[string]string                    // tableName -> tableOID
		rowTags        map[string]string
	}
)

func newCrossTableResolver(log *logger.Logger) *crossTableResolver {
	return &crossTableResolver{
		log:          log,
		tagProcessor: newTableTagProcessor(),
	}
}

// resolveCrossTableTag resolves a tag value from another table
func (r *crossTableResolver) resolveCrossTableTag(tagCfg ddprofiledefinition.MetricTagConfig, index string, ctx *crossTableContext) error {
	refTableOID, err := r.findReferencedTableOID(tagCfg.Table, ctx.tableNameToOID)
	if err != nil {
		return err
	}

	refTablePDUs, err := r.getReferencedTableData(tagCfg.Table, refTableOID, ctx.walkedData)
	if err != nil {
		return err
	}

	lookupIndex, err := r.transformIndex(index, tagCfg.IndexTransform)
	if err != nil {
		return err
	}

	pdu, err := r.lookupValue(tagCfg, lookupIndex, refTablePDUs)
	if err != nil {
		return err
	}

	ta := tagAdder{tags: ctx.rowTags}

	return r.tagProcessor.processTag(tagCfg, pdu, ta)
}

func (r *crossTableResolver) findReferencedTableOID(tableName string, tableNameToOID map[string]string) (string, error) {
	tableOID, ok := tableNameToOID[tableName]
	if !ok {
		r.log.Debugf("Cannot find table OID for referenced table %s", tableName)
		return "", fmt.Errorf("table %s not found", tableName)
	}
	return tableOID, nil
}

func (r *crossTableResolver) getReferencedTableData(tableName string, tableOID string, walkedData map[string]map[string]gosnmp.SnmpPDU) (map[string]gosnmp.SnmpPDU, error) {
	pdus, ok := walkedData[tableOID]
	if !ok {
		r.log.Debugf("No walked data for referenced table %s (OID: %s)", tableName, tableOID)
		return nil, fmt.Errorf("no data for table %s", tableName)
	}
	return pdus, nil
}

// transformIndex applies index transformation if configured
func (r *crossTableResolver) transformIndex(index string, transforms []ddprofiledefinition.MetricIndexTransform) (string, error) {
	if len(transforms) == 0 {
		return index, nil
	}

	transformedIndex := r.applyIndexTransform(index, transforms)
	if transformedIndex == "" {
		r.log.Debugf("Index transformation failed for index %s with transforms %v", index, transforms)
		return "", fmt.Errorf("index transformation failed")
	}

	return transformedIndex, nil
}

func (r *crossTableResolver) lookupValue(tagCfg ddprofiledefinition.MetricTagConfig, lookupIndex string, refTablePDUs map[string]gosnmp.SnmpPDU) (gosnmp.SnmpPDU, error) {
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

// applyTransform applies index transformation rules to extract a subset of the index
// Example: index "1.6.0.36.155.53.3.246", transform [{start: 1, end: 7}] → "6.0.36.155.53.3.246"
func (r *crossTableResolver) applyIndexTransform(index string, transforms []ddprofiledefinition.MetricIndexTransform) string {
	if len(transforms) == 0 {
		return index
	}

	parts := strings.Split(index, ".")
	var result []string

	for _, transform := range transforms {
		start, end := transform.Start, transform.End

		if int(start) >= len(parts) || end < start || int(end) >= len(parts) {
			return ""
		}

		// Extract the range (inclusive)
		extracted := parts[start : end+1]
		result = append(result, extracted...)
	}

	return strings.Join(result, ".")
}

func (r *crossTableResolver) isCrossTable(tagCfg ddprofiledefinition.MetricTagConfig, currentTableName string) bool {
	return tagCfg.Table != "" && tagCfg.Table != currentTableName && tagCfg.Index == 0
}
