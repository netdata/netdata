// SPDX-License-Identifier: GPL-3.0-or-later

package ddsnmpcollector

import (
	"errors"
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
		columnOIDs    map[string][]ddprofiledefinition.SymbolConfig
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
		p.log.Debugf("Error processing tag %s: %v", metricTagDisplayName(tagCfg), err)
	}
}

func (p *tableRowProcessor) processSingleCrossTableTag(row *tableRowData, tagCfg ddprofiledefinition.MetricTagConfig, ctx *tableRowProcessingContext) {
	if err := p.crossTableResolver.resolveCrossTableTag(tagCfg, row.index, ctx.crossTableCtx); err != nil {
		p.log.Debugf("Error resolving cross-table tag %s: %v", metricTagDisplayName(tagCfg), err)
	}
}

func (p *tableRowProcessor) processSingleIndexTag(row *tableRowData, tagCfg ddprofiledefinition.MetricTagConfig) {
	tagName, indexValue, err := p.processIndexTag(tagCfg, row.index)
	if err != nil {
		p.log.Debugf("Cannot process index tag %s from index %s: %v", metricTagDisplayName(tagCfg), row.index, err)
		return
	}

	ta := tagAdder{tags: row.tags}
	ta.addTag(tagName, indexValue)
}

func (p *tableRowProcessor) processIndexTag(cfg ddprofiledefinition.MetricTagConfig, index string) (string, string, error) {
	tagName := metricTagDisplayName(cfg)

	rawValue := index
	if cfg.Index != 0 {
		indexValue, ok := p.extractIndexPosition(index, cfg.Index)
		if !ok {
			return "", "", fmt.Errorf("position %d not found", cfg.Index)
		}
		rawValue = indexValue
	}

	if cfg.Index == 0 && len(cfg.IndexTransform) > 0 {
		rawValue = p.crossTableResolver.applyIndexTransform(index, cfg.IndexTransform)
		if rawValue == "" {
			return "", "", fmt.Errorf("index transformation failed")
		}
	}

	value, err := processRawIndexTagValue(cfg, rawValue)
	if err != nil {
		return "", "", err
	}

	return tagName, value, nil
}

func metricTagDisplayName(cfg ddprofiledefinition.MetricTagConfig) string {
	switch {
	case cfg.Tag != "":
		return cfg.Tag
	case cfg.Symbol.Name != "":
		return cfg.Symbol.Name
	case cfg.Index != 0:
		return fmt.Sprintf("index%d", cfg.Index)
	default:
		return "index"
	}
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

func (p *tableRowProcessor) extractIndexPositionFromEnd(index string, position uint) (string, bool) {
	if position == 0 || index == "" {
		return "", false
	}

	end := len(index)
	for n := uint(1); ; n++ {
		start := strings.LastIndexByte(index[:end], '.')
		if n == position {
			if start == -1 {
				return index[:end], end > 0
			}
			return index[start+1 : end], start+1 < end
		}
		if start == -1 {
			return "", false
		}
		end = start
	}
}

func (p *tableRowProcessor) processRowMetrics(row *tableRowData, ctx *tableRowProcessingContext) ([]ddsnmp.Metric, error) {
	symbolCount := 0
	for _, syms := range ctx.columnOIDs {
		symbolCount += len(syms)
	}
	metrics := make([]ddsnmp.Metric, 0, symbolCount)

	for columnOID, syms := range ctx.columnOIDs {
		pdu, ok := row.pdus[columnOID]
		if !ok {
			continue
		}

		for _, sym := range syms {
			metric, err := p.createMetric(sym, pdu, row)
			if err != nil {
				p.log.Debugf("Error creating metric %s: %v", sym.Name, err)
				continue
			}
			if metric == nil {
				continue
			}

			metrics = append(metrics, *metric)
		}
	}

	return metrics, nil
}

func (p *tableRowProcessor) createMetric(sym ddprofiledefinition.SymbolConfig, pdu gosnmp.SnmpPDU, row *tableRowData) (*ddsnmp.Metric, error) {
	value, err := p.valProc.processValue(sym, pdu)
	if err != nil {
		if errors.Is(err, errNoTextDateValue) {
			return nil, nil
		}
		return nil, fmt.Errorf("error processing value: %w", err)
	}

	return buildTableMetric(sym, pdu, value, row.tags, row.staticTags, row.tableName)
}

type (
	crossTableLookupKey struct {
		refTableOID     string
		lookupColumnOID string
		targetColumnOID string
		lookupValue     string
	}
	// crossTableResolver handles resolving tags from other tables
	crossTableResolver struct {
		log          *logger.Logger
		tagProcessor *tableTagProcessor
	}
	// crossTableContext contains all data needed for cross-table resolution
	crossTableContext struct {
		walkedData       map[string]map[string]gosnmp.SnmpPDU // tableOID -> PDUs
		tableNameToOID   map[string]string                    // tableName -> tableOID
		lookupIndexCache map[crossTableLookupKey]string       // cache key -> resolved row index
		rowTags          map[string]string
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

	if r.requiresLookupByValue(tagCfg) {
		lookupIndex, err = r.resolveLookupIndexByValue(tagCfg, lookupIndex, refTableOID, refTablePDUs, ctx)
		if err != nil {
			return err
		}
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

// applyTransform applies index transformation rules to extract a subset of the index.
// Indices are 0-based; end is inclusive.
// Examples:
//   - index "1.6.0.36.155.53.3.246", transform [{start: 1, end: 7}]    → "6.0.36.155.53.3.246"
//   - index "7.0.80.86.171.205.239", transform [{start: 1}]            → "0.80.86.171.205.239"  (start>0, end==0 ⇒ to tail)
//   - index "1.4.10.0.0.1.99",       transform [{start: 0, drop_right: 1}] → "1.4.10.0.0.1"
func (r *crossTableResolver) applyIndexTransform(index string, transforms []ddprofiledefinition.MetricIndexTransform) string {
	if len(transforms) == 0 {
		return index
	}

	parts := strings.Split(index, ".")
	var result []string

	for _, transform := range transforms {
		start, end := transform.Start, transform.End
		if transform.DropRight > 0 {
			if int(transform.DropRight) >= len(parts) {
				return ""
			}
			end = uint(len(parts) - int(transform.DropRight) - 1)
		} else if transform.Start > 0 && transform.End == 0 {
			end = uint(len(parts) - 1)
		}

		if int(start) >= len(parts) || end < start || int(end) >= len(parts) {
			return ""
		}

		// Extract the range (inclusive)
		extracted := parts[start : end+1]
		result = append(result, extracted...)
	}

	return strings.Join(result, ".")
}
