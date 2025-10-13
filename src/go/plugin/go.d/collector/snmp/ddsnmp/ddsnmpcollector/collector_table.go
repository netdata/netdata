// SPDX-License-Identifier: GPL-3.0-or-later

package ddsnmpcollector

import (
	"errors"
	"fmt"
	"slices"
	"strings"

	"github.com/gosnmp/gosnmp"

	"github.com/netdata/netdata/go/plugins/logger"
	"github.com/netdata/netdata/go/plugins/plugin/go.d/collector/snmp/ddsnmp"
	"github.com/netdata/netdata/go/plugins/plugin/go.d/collector/snmp/ddsnmp/ddprofiledefinition"
)

// tableCollector handles collection of SNMP table metrics
type tableCollector struct {
	snmpClient      gosnmp.Handler
	disableBulkWalk bool
	missingOIDs     map[string]bool
	tableCache      *tableCache
	log             *logger.Logger
	valProc         *valueProcessor
	rowProcessor    *tableRowProcessor
}

// newTableCollector creates a new table collector
func newTableCollector(snmpClient gosnmp.Handler, missingOIDs map[string]bool, tableCache *tableCache, log *logger.Logger, disableBulkWalk bool) *tableCollector {
	return &tableCollector{
		snmpClient:      snmpClient,
		disableBulkWalk: disableBulkWalk,
		missingOIDs:     missingOIDs,
		tableCache:      tableCache,
		log:             log,
		valProc:         newValueProcessor(),
		rowProcessor:    newTableRowProcessor(log),
	}
}

// Collect gathers all table metrics from the profile
func (tc *tableCollector) Collect(prof *ddsnmp.Profile) ([]ddsnmp.Metric, error) {
	walkResults, err := tc.walkTablesAsNeeded(prof)
	if err != nil {
		return nil, err
	}

	return tc.processWalkResults(walkResults)
}

// tableWalkResult holds the walked data for a single table
// tableWalkResult holds the walked data for a single table
type tableWalkResult struct {
	// tableOID is the base OID of the table (e.g., "1.3.6.1.2.1.2.2" for ifTable)
	// Used to identify which table this result belongs to
	tableOID string

	// pdus contains all PDUs retrieved from walking this table
	// Key: full OID (e.g., "1.3.6.1.2.1.2.2.1.10.1"), Value: SNMP PDU
	// nil when using cached data (table structure already known)
	pdus map[string]gosnmp.SnmpPDU

	// config is the metric configuration that requested this table
	// Contains symbols to collect, tags to apply, and metric metadata
	config ddprofiledefinition.MetricsConfig
}

type (
	tableProcessingContext struct {
		// === Input data (set when context is created) ===

		// config is the metric configuration for this table
		// Contains table OID, symbols to collect, and tag configurations
		config ddprofiledefinition.MetricsConfig

		// pdus contains all PDUs from walking this specific table
		// Key: full OID (e.g., "1.3.6.1.2.1.2.2.1.10.1"), Value: SNMP PDU
		pdus map[string]gosnmp.SnmpPDU

		// walkedData contains PDUs from ALL walked tables in this collection
		// Used for cross-table tag resolution
		// Key: table OID → map[full OID]PDU
		walkedData map[string]map[string]gosnmp.SnmpPDU

		// tableNameToOID maps table names to their OIDs
		// Used to resolve cross-table references (e.g., "ifXTable" → "1.3.6.1.2.1.31.1.1")
		tableNameToOID map[string]string

		// === Computed during processing (set by various methods) ===

		// columnOIDs maps column OIDs to their symbol configurations
		// Built from config.Symbols, used to identify which columns contain metrics
		// Key: column OID (e.g., "1.3.6.1.2.1.2.2.1.10"), Value: symbol config
		columnOIDs map[string]ddprofiledefinition.SymbolConfig

		// staticTags contains tags that apply to all metrics from this table
		// Parsed from config.StaticTags (e.g., "source:network")
		staticTags map[string]string

		// rows contains PDUs organized by row index
		// Created by organizePDUsByRow from flat PDU list
		// Key: row index (e.g., "1", "2.3") → map[column OID]PDU
		rows map[string]map[string]gosnmp.SnmpPDU

		// oidCache stores the mapping of column OID to full OID for each row
		// Used for caching table structure
		// Key: row index → map[column OID]full OID
		oidCache map[string]map[string]string

		// tagCache stores computed tag values for each row
		// Populated during row processing, used for caching
		// Key: row index → map[tag name]tag value
		tagCache map[string]map[string]string

		// orderedTags contains metric tags in profile-defined order to ensure correct
		// precedence when multiple tags share the same name (first non-empty wins)
		orderedTags []orderedTagConfig
	}
	orderedTagConfig struct {
		config  ddprofiledefinition.MetricTagConfig
		tagType tagType
	}
	tagType int
)

const (
	tagTypeSameTable tagType = iota
	tagTypeCrossTable
	tagTypeIndex
)

type cacheProcessingContext struct {
	// config is the metric configuration for this table
	// Used to determine which columns are metrics and parse static tags
	config ddprofiledefinition.MetricsConfig

	// cachedOIDs contains the cached table structure
	// Retrieved from tableCache, maps row index → column OID → full OID
	// Key: row index → map[column OID]full OID
	cachedOIDs map[string]map[string]string

	// cachedTags contains previously computed tag values
	// Retrieved from tableCache, avoids re-processing tag columns
	// Key: row index → map[tag name]tag value
	cachedTags map[string]map[string]string

	// columnOIDs identifies which columns contain metrics (not tags)
	// Built from config.Symbols, used to filter which OIDs to GET
	// Key: column OID, Value: symbol configuration
	columnOIDs map[string]ddprofiledefinition.SymbolConfig

	// pdus contains current metric values fetched via SNMP GET
	// Only contains metric columns (not tag columns, which are cached)
	// Key: full OID (trimmed), Value: current PDU value
	pdus map[string]gosnmp.SnmpPDU

	tableName string
}

// walkTablesAsNeeded walks only tables that aren't fully cached
func (tc *tableCollector) walkTablesAsNeeded(prof *ddsnmp.Profile) ([]tableWalkResult, error) {
	toWalk := tc.identifyTablesToWalk(prof)

	walkedData, errs := tc.walkTables(toWalk.tablesToWalk)

	results := tc.buildWalkResults(walkedData, toWalk)

	if len(results) == 0 && len(errs) > 0 {
		return nil, errors.Join(errs...)
	}

	return results, nil
}

// tablesToWalkInfo holds information about which tables need walking
type tablesToWalkInfo struct {
	tablesToWalk map[string]bool
	tableConfigs map[string][]ddprofiledefinition.MetricsConfig
	missingOIDs  []string
}

// identifyTablesToWalk determines which tables need to be walked
func (tc *tableCollector) identifyTablesToWalk(prof *ddsnmp.Profile) *tablesToWalkInfo {
	info := &tablesToWalkInfo{
		tablesToWalk: make(map[string]bool),
		tableConfigs: make(map[string][]ddprofiledefinition.MetricsConfig),
	}

	for _, cfg := range prof.Definition.Metrics {
		if cfg.IsScalar() || cfg.Table.OID == "" {
			continue
		}

		tableOID := cfg.Table.OID
		if tc.missingOIDs[trimOID(tableOID)] {
			info.missingOIDs = append(info.missingOIDs, tableOID)
			continue
		}

		info.tableConfigs[tableOID] = append(info.tableConfigs[tableOID], cfg)

		if !tc.tableCache.isConfigCached(cfg) {
			info.tablesToWalk[tableOID] = true
		}
	}

	tc.log.Debugf("Tables walking %d cached %d (%d total)",
		len(info.tablesToWalk), len(info.tableConfigs)-len(info.tablesToWalk), len(info.tableConfigs))

	if len(info.missingOIDs) > 0 {
		tc.log.Debugf("table metrics missing OIDs: %v", info.missingOIDs)
	}

	return info
}

// walkTables performs SNMP walks for the specified tables
func (tc *tableCollector) walkTables(tablesToWalk map[string]bool) (map[string]map[string]gosnmp.SnmpPDU, []error) {
	walkedData := make(map[string]map[string]gosnmp.SnmpPDU)
	var errs []error

	for tableOID := range tablesToWalk {
		pdus, err := tc.snmpWalk(tableOID)
		if err != nil {
			errs = append(errs, fmt.Errorf("failed to walk table OID '%s': %w", tableOID, err))
			continue
		}

		if len(pdus) > 0 {
			walkedData[tableOID] = pdus
		}
	}

	return walkedData, errs
}

// buildWalkResults creates results for all configurations
func (tc *tableCollector) buildWalkResults(walkedData map[string]map[string]gosnmp.SnmpPDU, info *tablesToWalkInfo) []tableWalkResult {
	var results []tableWalkResult

	for tableOID, configs := range info.tableConfigs {
		for _, cfg := range configs {
			// Add to results if we have walked data OR if it's cached
			if pdus, ok := walkedData[tableOID]; ok {
				results = append(results, tableWalkResult{
					tableOID: tableOID,
					pdus:     pdus,
					config:   cfg,
				})
			} else if tc.tableCache.isConfigCached(cfg) {
				// Add config without PDUs - will use cache in processing
				results = append(results, tableWalkResult{
					tableOID: tableOID,
					pdus:     nil,
					config:   cfg,
				})
			}
		}
	}

	return results
}

// processWalkResults processes all table walk results
func (tc *tableCollector) processWalkResults(walkResults []tableWalkResult) ([]ddsnmp.Metric, error) {
	// Build lookup maps
	walkedData := tc.buildWalkedDataMap(walkResults)
	tableNameToOID := tc.buildTableNameMap(walkResults)

	var metrics []ddsnmp.Metric
	var errs []error

	for _, result := range walkResults {
		tableMetrics, err := tc.processTableResult(result, walkedData, tableNameToOID)
		if err != nil {
			errs = append(errs, fmt.Errorf("table '%s': %w", result.config.Table.Name, err))
			continue
		}
		metrics = append(metrics, tableMetrics...)
	}

	if len(metrics) == 0 && len(errs) > 0 {
		return nil, errors.Join(errs...)
	}

	return metrics, nil
}

// buildWalkedDataMap creates a map of table OID to PDUs
func (tc *tableCollector) buildWalkedDataMap(walkResults []tableWalkResult) map[string]map[string]gosnmp.SnmpPDU {
	walkedData := make(map[string]map[string]gosnmp.SnmpPDU)
	for _, result := range walkResults {
		if result.pdus != nil {
			walkedData[result.tableOID] = result.pdus
		}
	}
	return walkedData
}

// buildTableNameMap creates a map of table name to OID
func (tc *tableCollector) buildTableNameMap(walkResults []tableWalkResult) map[string]string {
	tableNameToOID := make(map[string]string)
	for _, result := range walkResults {
		if result.config.Table.Name != "" {
			tableNameToOID[result.config.Table.Name] = result.tableOID
		}
	}
	return tableNameToOID
}

// processTableResult processes a single table result
func (tc *tableCollector) processTableResult(result tableWalkResult, walkedData map[string]map[string]gosnmp.SnmpPDU, tableNameToOID map[string]string) ([]ddsnmp.Metric, error) {
	// Try cache first
	if metrics := tc.tryCollectFromCache(result.config); metrics != nil {
		return metrics, nil
	}

	// Process walked data if available
	if result.pdus != nil {
		ctx := &tableProcessingContext{
			config:         result.config,
			pdus:           result.pdus,
			walkedData:     walkedData,
			tableNameToOID: tableNameToOID,
		}
		return tc.processTableData(ctx)
	}

	return nil, nil
}

// tryCollectFromCache attempts to collect metrics using cached data
func (tc *tableCollector) tryCollectFromCache(cfg ddprofiledefinition.MetricsConfig) []ddsnmp.Metric {
	cachedOIDs, cachedTags, ok := tc.tableCache.getCachedData(cfg)
	if !ok {
		return nil
	}

	columnOIDs := buildColumnOIDs(cfg)

	ctx := &cacheProcessingContext{
		config:     cfg,
		cachedOIDs: cachedOIDs,
		cachedTags: cachedTags,
		columnOIDs: columnOIDs,
		tableName:  cfg.Table.Name,
	}

	metrics, err := tc.collectWithCache(ctx)
	if err == nil {
		tc.log.Debugf("Successfully collected table %s using cache", cfg.Table.Name)
		return metrics
	}

	tc.log.Debugf("Cached collection failed for table %s: %v", cfg.Table.Name, err)
	return nil
}

// processTableData processes walked table data
func (tc *tableCollector) processTableData(ctx *tableProcessingContext) ([]ddsnmp.Metric, error) {
	ctx.columnOIDs = buildColumnOIDs(ctx.config)

	ctx.orderedTags = buildOrderedTags(ctx.config)

	ctx.rows, ctx.oidCache, ctx.tagCache = tc.organizePDUsByRow(ctx)

	ctx.staticTags = parseStaticTags(ctx.config.StaticTags)

	metrics, err := tc.processRows(ctx)

	// Cache the processed data
	deps := extractTableDependencies(ctx.config, ctx.tableNameToOID)
	tc.tableCache.cacheData(ctx.config, ctx.oidCache, ctx.tagCache, deps)
	tc.log.Debugf("Cached table %s structure with %d rows", ctx.config.Table.Name, len(ctx.oidCache))

	return metrics, err
}

// organizePDUsByRow groups PDUs by their row index
func (tc *tableCollector) organizePDUsByRow(ctx *tableProcessingContext) (rows map[string]map[string]gosnmp.SnmpPDU, oidCache, tagCache map[string]map[string]string) {
	// Combine all column OIDs
	allColumnOIDs := make([]string, 0, len(ctx.columnOIDs)+len(ctx.orderedTags))

	for oid := range ctx.columnOIDs {
		allColumnOIDs = append(allColumnOIDs, oid)
	}
	for _, tag := range ctx.orderedTags {
		if tag.tagType == tagTypeSameTable && tag.config.Symbol.OID != "" {
			oid := trimOID(tag.config.Symbol.OID)
			allColumnOIDs = append(allColumnOIDs, oid)
		}
	}

	rows = make(map[string]map[string]gosnmp.SnmpPDU)
	oidCache = make(map[string]map[string]string)
	tagCache = make(map[string]map[string]string)

	for oid, pdu := range ctx.pdus {
		for _, columnOID := range allColumnOIDs {
			if strings.HasPrefix(oid, columnOID+".") {
				index := strings.TrimPrefix(oid, columnOID+".")

				if rows[index] == nil {
					rows[index] = make(map[string]gosnmp.SnmpPDU)
					oidCache[index] = make(map[string]string)
					tagCache[index] = make(map[string]string)
				}
				rows[index][columnOID] = pdu
				oidCache[index][columnOID] = oid
				break
			}
		}
	}

	return rows, oidCache, tagCache
}

// processRows processes all rows and returns metrics
func (tc *tableCollector) processRows(ctx *tableProcessingContext) ([]ddsnmp.Metric, error) {
	var metrics []ddsnmp.Metric
	var errs []error

	crossTableCtx := &crossTableContext{
		walkedData:     ctx.walkedData,
		tableNameToOID: ctx.tableNameToOID,
	}

	for index, rowPDUs := range ctx.rows {
		row := &tableRowData{
			index:      index,
			pdus:       rowPDUs,
			tags:       make(map[string]string),
			staticTags: ctx.staticTags,
			tableName:  ctx.config.Table.Name,
		}
		crossTableCtx.rowTags = row.tags

		rowCtx := &tableRowProcessingContext{
			config:        ctx.config,
			columnOIDs:    ctx.columnOIDs,
			crossTableCtx: crossTableCtx,
			orderedTags:   ctx.orderedTags,
		}
		rowMetrics, err := tc.rowProcessor.processRow(row, rowCtx)
		if err != nil {
			errs = append(errs, err)
			continue
		}

		// Copy processed tags to cache
		for k, v := range row.tags {
			ctx.tagCache[index][k] = v
		}

		metrics = append(metrics, rowMetrics...)
	}

	if len(errs) > 0 {
		tc.log.Warningf("failed to collect table metrics: %v", errors.Join(errs...))
	}

	return metrics, nil
}

// collectWithCache collects metrics using cached structure
func (tc *tableCollector) collectWithCache(ctx *cacheProcessingContext) ([]ddsnmp.Metric, error) {
	// Build list of OIDs to GET
	var oidsToGet []string
	for _, columns := range ctx.cachedOIDs {
		for columnOID, fullOID := range columns {
			if _, isMetric := ctx.columnOIDs[columnOID]; isMetric {
				oidsToGet = append(oidsToGet, fullOID)
			}
		}
	}

	if len(oidsToGet) == 0 {
		return nil, nil
	}

	// GET current values
	pdus, err := tc.snmpGet(oidsToGet)
	if err != nil {
		return nil, fmt.Errorf("failed to get cached OIDs: %w", err)
	}

	// Validate response
	if len(pdus) < len(oidsToGet)/2 {
		return nil, fmt.Errorf("table structure may have changed, got %d/%d PDUs", len(pdus), len(oidsToGet))
	}

	// Add PDUs to context and build metrics
	ctx.pdus = pdus
	return tc.buildMetricsFromCache(ctx)
}

// buildMetricsFromCache builds metrics from cached structure and current values
func (tc *tableCollector) buildMetricsFromCache(ctx *cacheProcessingContext) ([]ddsnmp.Metric, error) {
	staticTags := parseStaticTags(ctx.config.StaticTags)
	var metrics []ddsnmp.Metric
	var errs []error

	for index, columns := range ctx.cachedOIDs {
		// Get cached tags for this row
		rowTags := make(map[string]string)
		if tags, ok := ctx.cachedTags[index]; ok {
			for k, v := range tags {
				rowTags[k] = v
			}
		}

		// Process each metric column
		for columnOID, fullOID := range columns {
			sym, isMetric := ctx.columnOIDs[columnOID]
			if !isMetric {
				continue
			}

			pdu, ok := ctx.pdus[trimOID(fullOID)]
			if !ok {
				tc.log.Debugf("Missing PDU for cached OID %s", fullOID)
				continue
			}

			value, err := tc.valProc.processValue(sym, pdu)
			if err != nil {
				tc.log.Debugf("Error processing value for %s: %v", sym.Name, err)
				continue
			}

			metric, err := buildTableMetric(sym, pdu, value, rowTags, staticTags, ctx.tableName)
			if err != nil {
				errs = append(errs, err)
				continue
			}

			metrics = append(metrics, *metric)
		}
	}

	if len(errs) > 0 {
		tc.log.Warningf("failed to collect table metrics: %v", errors.Join(errs...))
	}

	return metrics, nil
}

// SNMP operations

func (tc *tableCollector) snmpWalk(oid string) (map[string]gosnmp.SnmpPDU, error) {
	pdus := make(map[string]gosnmp.SnmpPDU)

	var resp []gosnmp.SnmpPDU
	var err error

	if tc.snmpClient.Version() == gosnmp.Version1 || tc.disableBulkWalk {
		resp, err = tc.snmpClient.WalkAll(oid)
	} else {
		resp, err = tc.snmpClient.BulkWalkAll(oid)
	}
	if err != nil {
		return nil, err
	}

	for _, pdu := range resp {
		if isPduWithData(pdu) {
			pdus[trimOID(pdu.Name)] = pdu
		}
	}

	if len(pdus) == 0 {
		tc.missingOIDs[trimOID(oid)] = true
	}

	return pdus, nil
}

func (tc *tableCollector) snmpGet(oids []string) (map[string]gosnmp.SnmpPDU, error) {
	pdus := make(map[string]gosnmp.SnmpPDU)

	for chunk := range slices.Chunk(oids, tc.snmpClient.MaxOids()) {
		result, err := tc.snmpClient.Get(chunk)
		if err != nil {
			return nil, err
		}

		for _, pdu := range result.Variables {
			if !isPduWithData(pdu) {
				tc.missingOIDs[trimOID(pdu.Name)] = true
				continue
			}
			pdus[trimOID(pdu.Name)] = pdu
		}
	}

	return pdus, nil
}

func parseStaticTags(staticTags []string) map[string]string {
	tags := make(map[string]string)
	for _, tag := range staticTags {
		if n, v, _ := strings.Cut(tag, ":"); n != "" && v != "" {
			tags[n] = v
		}
	}
	return tags
}

func buildColumnOIDs(cfg ddprofiledefinition.MetricsConfig) map[string]ddprofiledefinition.SymbolConfig {
	columnOIDs := make(map[string]ddprofiledefinition.SymbolConfig)
	for _, sym := range cfg.Symbols {
		columnOIDs[trimOID(sym.OID)] = sym
	}
	return columnOIDs
}

func extractTableDependencies(cfg ddprofiledefinition.MetricsConfig, tableNameToOID map[string]string) []string {
	deps := make(map[string]bool)

	for _, tagCfg := range cfg.MetricTags {
		// Skip if not a cross-table tag
		if tagCfg.Table == "" || tagCfg.Table == cfg.Table.Name {
			continue
		}

		if tableOID, ok := tableNameToOID[tagCfg.Table]; ok {
			deps[tableOID] = true
		}
	}

	result := make([]string, 0, len(deps))
	for oid := range deps {
		result = append(result, oid)
	}

	return result
}

func buildOrderedTags(cfg ddprofiledefinition.MetricsConfig) []orderedTagConfig {
	var ordered []orderedTagConfig

	for _, tagCfg := range cfg.MetricTags {
		var tt tagType
		switch {
		case tagCfg.Index != 0:
			tt = tagTypeIndex
		case tagCfg.Table != "" && tagCfg.Table != cfg.Table.Name:
			tt = tagTypeCrossTable
		default:
			tt = tagTypeSameTable
		}

		ordered = append(ordered, orderedTagConfig{
			config:  tagCfg,
			tagType: tt,
		})
	}

	return ordered
}
