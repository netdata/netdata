// SPDX-License-Identifier: GPL-3.0-or-later

package ddsnmpcollector

import (
	"fmt"

	"github.com/gosnmp/gosnmp"

	"github.com/netdata/netdata/go/plugins/logger"
	"github.com/netdata/netdata/go/plugins/plugin/go.d/collector/snmp/ddsnmp"
	"github.com/netdata/netdata/go/plugins/plugin/go.d/collector/snmp/ddsnmp/ddprofiledefinition"
)

// TableRowProcessor processes individual rows from SNMP tables
type TableRowProcessor struct {
	log                *logger.Logger
	crossTableResolver *CrossTableResolver
	indexTransformer   *IndexTransformer
}

// NewTableRowProcessor creates a new table row processor
func NewTableRowProcessor(log *logger.Logger) *TableRowProcessor {
	return &TableRowProcessor{
		log:                log,
		crossTableResolver: NewCrossTableResolver(log),
		indexTransformer:   NewIndexTransformer(),
	}
}

// RowData represents data for a single table row
type RowData struct {
	Index      string
	PDUs       map[string]gosnmp.SnmpPDU
	Tags       map[string]string
	StaticTags map[string]string
}

// RowProcessingContext contains context needed for processing a row
type RowProcessingContext struct {
	Config        ddprofiledefinition.MetricsConfig
	ColumnOIDs    map[string]ddprofiledefinition.SymbolConfig
	TagColumnOIDs map[string][]ddprofiledefinition.MetricTagConfig
	CrossTableCtx *CrossTableContext
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

			p.mergeTags(row.Tags, tags)
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

		p.mergeTags(row.Tags, tags)
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

// mergeTags merges source tags into destination with empty fallback
func (p *TableRowProcessor) mergeTags(dest, src map[string]string) {
	for k, v := range src {
		if existing, ok := dest[k]; !ok || existing == "" {
			dest[k] = v
		}
	}
}
