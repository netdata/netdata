package ddsnmpcollector

import (
	"fmt"
	"strings"

	"github.com/gosnmp/gosnmp"

	"github.com/netdata/netdata/go/plugins/logger"
	"github.com/netdata/netdata/go/plugins/plugin/go.d/collector/snmp/ddsnmp"
	"github.com/netdata/netdata/go/plugins/plugin/go.d/collector/snmp/ddsnmp/ddprofiledefinition"
)

// TableRowProcessor processes individual rows from SNMP tables
type (
	TableRowProcessor struct {
		log                *logger.Logger
		crossTableResolver *CrossTableResolver
		indexTransformer   *IndexTransformer
	}
	// RowData represents data for a single table row
	RowData struct {
		Index      string
		PDUs       map[string]gosnmp.SnmpPDU
		Tags       map[string]string
		StaticTags map[string]string
	}
	// RowProcessingContext contains context needed for processing a row
	RowProcessingContext struct {
		Config        ddprofiledefinition.MetricsConfig
		ColumnOIDs    map[string]ddprofiledefinition.SymbolConfig
		TagColumnOIDs map[string][]ddprofiledefinition.MetricTagConfig
		CrossTableCtx *CrossTableContext
	}
)

// NewTableRowProcessor creates a new table row processor
func NewTableRowProcessor(log *logger.Logger) *TableRowProcessor {
	return &TableRowProcessor{
		log:                log,
		crossTableResolver: NewCrossTableResolver(log),
		indexTransformer:   NewIndexTransformer(),
	}
}

// ProcessRow processes a single table row and returns metrics
func (p *TableRowProcessor) ProcessRow(row *RowData, ctx *RowProcessingContext) ([]ddsnmp.Metric, error) {
	// Process all tags for this row
	if err := p.processRowTags(row, ctx); err != nil {
		p.log.Debugf("Error processing tags for row %s: %v", row.Index, err)
	}

	// Process metrics for this row
	return p.processRowMetrics(row, ctx)
}

// processRowTags processes all types of tags for a row
func (p *TableRowProcessor) processRowTags(row *RowData, ctx *RowProcessingContext) error {
	// Process same-table tags
	p.processSameTableTags(row, ctx.TagColumnOIDs)

	// Process cross-table tags
	if ctx.CrossTableCtx != nil {
		p.processCrossTableTags(row, ctx)
	}

	// Process index-based tags
	p.processIndexBasedTags(row, ctx.Config.MetricTags)

	return nil
}

// processSameTableTags processes tags from the same table
func (p *TableRowProcessor) processSameTableTags(row *RowData, tagColumnOIDs map[string][]ddprofiledefinition.MetricTagConfig) {
	for columnOID, tagConfigs := range tagColumnOIDs {
		pdu, ok := row.PDUs[columnOID]
		if !ok {
			continue
		}

		for _, tagCfg := range tagConfigs {
			tags, err := processTableMetricTagValue(tagCfg, pdu)
			if err != nil {
				p.log.Debugf("Error processing tag %s: %v", tagCfg.Tag, err)
				continue
			}

			mergeTagsWithEmptyFallback(row.Tags, tags)
		}
	}
}

// processCrossTableTags processes tags from other tables
func (p *TableRowProcessor) processCrossTableTags(row *RowData, ctx *RowProcessingContext) {
	for _, tagCfg := range ctx.Config.MetricTags {
		if !p.crossTableResolver.IsApplicable(tagCfg, ctx.Config.Table.Name) {
			continue
		}

		tags, err := p.crossTableResolver.ResolveCrossTableTag(tagCfg, row.Index, ctx.CrossTableCtx)
		if err != nil {
			p.log.Debugf("Error resolving cross-table tag %s: %v", tagCfg.Tag, err)
			continue
		}

		mergeTagsWithEmptyFallback(row.Tags, tags)
	}
}

// processIndexBasedTags processes index-based tags
func (p *TableRowProcessor) processIndexBasedTags(row *RowData, metricTags []ddprofiledefinition.MetricTagConfig) {
	for _, tagCfg := range metricTags {
		if tagCfg.Index == 0 {
			continue
		}

		tagName, indexValue, ok := processIndexTag(tagCfg, row.Index)
		if !ok {
			p.log.Debugf("Cannot extract position %d from index %s", tagCfg.Index, row.Index)
			continue
		}

		row.Tags[tagName] = indexValue
	}
}

// processRowMetrics processes metrics for a row
func (p *TableRowProcessor) processRowMetrics(row *RowData, ctx *RowProcessingContext) ([]ddsnmp.Metric, error) {
	var metrics []ddsnmp.Metric

	for columnOID, sym := range ctx.ColumnOIDs {
		pdu, ok := row.PDUs[columnOID]
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

// createMetric creates a metric from symbol config and PDU
func (p *TableRowProcessor) createMetric(sym ddprofiledefinition.SymbolConfig, pdu gosnmp.SnmpPDU, row *RowData) (*ddsnmp.Metric, error) {
	// Process the value
	value, err := processSymbolValue(sym, pdu)
	if err != nil {
		return nil, fmt.Errorf("error processing value: %w", err)
	}

	// Build the metric
	return buildTableMetric(sym, pdu, value, row.Tags, row.StaticTags)
}

// CrossTableResolver handles resolving tags from other tables
type (
	CrossTableResolver struct {
		log              *logger.Logger
		indexTransformer *IndexTransformer
		tagProcessor     *TagProcessorFactory
	}
	// CrossTableContext contains all data needed for cross-table resolution
	CrossTableContext struct {
		walkedData     map[string]map[string]gosnmp.SnmpPDU // tableOID -> PDUs
		tableNameToOID map[string]string                    // tableName -> tableOID
	}
)

// NewCrossTableResolver creates a new cross-table resolver
func NewCrossTableResolver(log *logger.Logger) *CrossTableResolver {
	return &CrossTableResolver{
		log:              log,
		indexTransformer: NewIndexTransformer(),
		tagProcessor:     NewTagProcessorFactory(),
	}
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

// IndexTransformer handles index manipulation operations
type IndexTransformer struct{}

// NewIndexTransformer creates a new IndexTransformer
func NewIndexTransformer() *IndexTransformer {
	return &IndexTransformer{}
}

// ExtractPosition extracts a specific position from an index
// Position uses 1-based indexing as per the profile format
// Example: index "7.8.9", position 2 → "8"
func (t *IndexTransformer) ExtractPosition(index string, position uint) (string, bool) {
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

// ApplyTransform applies index transformation rules to extract a subset of the index
// Example: index "1.6.0.36.155.53.3.246", transform [{start: 1, end: 7}] → "6.0.36.155.53.3.246"
func (t *IndexTransformer) ApplyTransform(index string, transforms []ddprofiledefinition.MetricIndexTransform) string {
	if len(transforms) == 0 {
		return index
	}

	parts := strings.Split(index, ".")
	var result []string

	for _, transform := range transforms {
		extracted := t.extractRange(parts, transform.Start, transform.End)
		result = append(result, extracted...)
	}

	return strings.Join(result, ".")
}

// extractRange extracts a range of parts from the index
func (t *IndexTransformer) extractRange(parts []string, start, end uint) []string {
	// Validate bounds
	if int(start) >= len(parts) || end < start || int(end) >= len(parts) {
		return nil
	}

	// Extract the range (inclusive)
	return parts[start : end+1]
}

// ValidateTransform checks if a transform is valid for a given index
func (t *IndexTransformer) ValidateTransform(index string, transform ddprofiledefinition.MetricIndexTransform) error {
	parts := strings.Split(index, ".")

	if int(transform.Start) >= len(parts) {
		return fmt.Errorf("start position %d is out of bounds for index with %d parts", transform.Start, len(parts))
	}

	if transform.End < transform.Start {
		return fmt.Errorf("end position %d is before start position %d", transform.End, transform.Start)
	}

	if int(transform.End) >= len(parts) {
		return fmt.Errorf("end position %d is out of bounds for index with %d parts", transform.End, len(parts))
	}

	return nil
}

// Global instance for backward compatibility
var indexTransformer = NewIndexTransformer()

// getIndexPosition wraps ExtractPosition for backward compatibility
func getIndexPosition(index string, position uint) (string, bool) {
	return indexTransformer.ExtractPosition(index, position)
}

// applyIndexTransform wraps ApplyTransform for backward compatibility
func applyIndexTransform(index string, transforms []ddprofiledefinition.MetricIndexTransform) string {
	return indexTransformer.ApplyTransform(index, transforms)
}
