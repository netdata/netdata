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
	snmpClient   gosnmp.Handler
	missingOIDs  map[string]bool
	tableCache   *tableCache
	log          *logger.Logger
	valProc      *valueProcessor
	rowProcessor *tableRowProcessor
}

// newTableCollector creates a new table collector
func newTableCollector(snmpClient gosnmp.Handler, missingOIDs map[string]bool, tableCache *tableCache, log *logger.Logger) *tableCollector {
	return &tableCollector{
		snmpClient:   snmpClient,
		missingOIDs:  missingOIDs,
		tableCache:   tableCache,
		log:          log,
		valProc:      newValueProcessor(),
		rowProcessor: newTableRowProcessor(log),
	}
}

// tableWalkResult holds the walked data for a single table
type tableWalkResult struct {
	tableOID string
	pdus     map[string]gosnmp.SnmpPDU
	config   ddprofiledefinition.MetricsConfig
}

// Collect gathers all table metrics from the profile
func (tc *tableCollector) Collect(prof *ddsnmp.Profile) ([]ddsnmp.Metric, error) {
	walkResults, err := tc.walkTablesAsNeeded(prof)
	if err != nil {
		return nil, err
	}

	return tc.processWalkResults(walkResults)
}

// walkTablesAsNeeded walks only tables that aren't fully cached
func (tc *tableCollector) walkTablesAsNeeded(prof *ddsnmp.Profile) ([]tableWalkResult, error) {
	// Identify tables to walk
	toWalk := tc.identifyTablesToWalk(prof)

	// Walk the tables
	walkedData, errs := tc.walkTables(toWalk.tablesToWalk)

	// Build results
	results := tc.buildWalkResults(prof, walkedData, toWalk)

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
func (tc *tableCollector) buildWalkResults(prof *ddsnmp.Profile, walkedData map[string]map[string]gosnmp.SnmpPDU, info *tablesToWalkInfo) []tableWalkResult {
	var results []tableWalkResult

	for _, cfg := range prof.Definition.Metrics {
		if cfg.IsScalar() || cfg.Table.OID == "" {
			continue
		}

		if tc.missingOIDs[trimOID(cfg.Table.OID)] {
			continue
		}

		// Add to results if we have walked data OR if it's cached
		if pdus, ok := walkedData[cfg.Table.OID]; ok {
			results = append(results, tableWalkResult{
				tableOID: cfg.Table.OID,
				pdus:     pdus,
				config:   cfg,
			})
		} else if tc.tableCache.isConfigCached(cfg) {
			// Add config without PDUs - will use cache in processing
			results = append(results, tableWalkResult{
				tableOID: cfg.Table.OID,
				pdus:     nil,
				config:   cfg,
			})
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
		return tc.processTableData(result.config, result.pdus, walkedData, tableNameToOID)
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
	metrics, err := tc.collectWithCache(cfg, cachedOIDs, cachedTags, columnOIDs)
	if err == nil {
		tc.log.Debugf("Successfully collected table %s using cache", cfg.Table.Name)
		return metrics
	}

	tc.log.Debugf("Cached collection failed for table %s: %v", cfg.Table.Name, err)
	return nil
}

// processTableData processes walked table data
func (tc *tableCollector) processTableData(
	cfg ddprofiledefinition.MetricsConfig,
	pdus map[string]gosnmp.SnmpPDU,
	walkedData map[string]map[string]gosnmp.SnmpPDU,
	tableNameToOID map[string]string,
) ([]ddsnmp.Metric, error) {
	// Prepare column information
	columnOIDs := buildColumnOIDs(cfg)
	tagColumnOIDs := buildTagColumnOIDs(cfg)

	// Group PDUs by row
	rows, oidCache, tagCache := tc.organizePDUsByRow(pdus, columnOIDs, tagColumnOIDs)

	// Parse static tags
	staticTags := parseStaticTags(cfg.StaticTags)

	// Process each row
	crossTableCtx := &CrossTableContext{
		walkedData:     walkedData,
		tableNameToOID: tableNameToOID,
	}

	metrics, err := tc.processRows(rows, cfg, columnOIDs, tagColumnOIDs, staticTags, crossTableCtx, tagCache)

	// Cache the processed data
	deps := extractTableDependencies(cfg, tableNameToOID)
	tc.tableCache.cacheData(cfg, oidCache, tagCache, deps)
	tc.log.Debugf("Cached table %s structure with %d rows", cfg.Table.Name, len(oidCache))

	return metrics, err
}

// organizePDUsByRow groups PDUs by their row index
func (tc *tableCollector) organizePDUsByRow(
	pdus map[string]gosnmp.SnmpPDU,
	columnOIDs map[string]ddprofiledefinition.SymbolConfig,
	tagColumnOIDs map[string][]ddprofiledefinition.MetricTagConfig,
) (rows map[string]map[string]gosnmp.SnmpPDU, oidCache, tagCache map[string]map[string]string) {
	// Combine all column OIDs
	allColumnOIDs := make([]string, 0, len(columnOIDs)+len(tagColumnOIDs))
	for oid := range columnOIDs {
		allColumnOIDs = append(allColumnOIDs, oid)
	}
	for oid := range tagColumnOIDs {
		allColumnOIDs = append(allColumnOIDs, oid)
	}

	rows = make(map[string]map[string]gosnmp.SnmpPDU)
	oidCache = make(map[string]map[string]string)
	tagCache = make(map[string]map[string]string)

	for oid, pdu := range pdus {
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
func (tc *tableCollector) processRows(
	rows map[string]map[string]gosnmp.SnmpPDU,
	cfg ddprofiledefinition.MetricsConfig,
	columnOIDs map[string]ddprofiledefinition.SymbolConfig,
	tagColumnOIDs map[string][]ddprofiledefinition.MetricTagConfig,
	staticTags map[string]string,
	crossTableCtx *CrossTableContext,
	tagCache map[string]map[string]string,
) ([]ddsnmp.Metric, error) {
	var metrics []ddsnmp.Metric
	var errs []error

	for index, rowPDUs := range rows {
		row := &tableRowData{
			Index:      index,
			PDUs:       rowPDUs,
			Tags:       make(map[string]string),
			StaticTags: staticTags,
		}

		ctx := &tableRowProcessingContext{
			Config:        cfg,
			ColumnOIDs:    columnOIDs,
			TagColumnOIDs: tagColumnOIDs,
			CrossTableCtx: crossTableCtx,
		}
		rowMetrics, err := tc.rowProcessor.ProcessRow(row, ctx)
		if err != nil {
			errs = append(errs, err)
			continue
		}

		// Copy processed tags to cache
		for k, v := range row.Tags {
			tagCache[index][k] = v
		}

		metrics = append(metrics, rowMetrics...)
	}

	if len(errs) > 0 {
		tc.log.Warningf("failed to collect table metrics: %v", errors.Join(errs...))
	}

	return metrics, nil
}

// collectWithCache collects metrics using cached structure
func (tc *tableCollector) collectWithCache(
	cfg ddprofiledefinition.MetricsConfig,
	cachedOIDs map[string]map[string]string,
	cachedTags map[string]map[string]string,
	columnOIDs map[string]ddprofiledefinition.SymbolConfig,
) ([]ddsnmp.Metric, error) {
	// Build list of OIDs to GET
	var oidsToGet []string
	for _, columns := range cachedOIDs {
		for columnOID, fullOID := range columns {
			if _, isMetric := columnOIDs[columnOID]; isMetric {
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

	// Build metrics
	return tc.buildMetricsFromCache(cfg, cachedOIDs, cachedTags, columnOIDs, pdus)
}

// buildMetricsFromCache builds metrics from cached structure and current values
func (tc *tableCollector) buildMetricsFromCache(
	cfg ddprofiledefinition.MetricsConfig,
	cachedOIDs map[string]map[string]string,
	cachedTags map[string]map[string]string,
	columnOIDs map[string]ddprofiledefinition.SymbolConfig,
	pdus map[string]gosnmp.SnmpPDU,
) ([]ddsnmp.Metric, error) {
	staticTags := parseStaticTags(cfg.StaticTags)
	var metrics []ddsnmp.Metric
	var errs []error

	for index, columns := range cachedOIDs {
		// Get cached tags for this row
		rowTags := make(map[string]string)
		if tags, ok := cachedTags[index]; ok {
			for k, v := range tags {
				rowTags[k] = v
			}
		}

		// Process each metric column
		for columnOID, fullOID := range columns {
			sym, isMetric := columnOIDs[columnOID]
			if !isMetric {
				continue
			}

			pdu, ok := pdus[trimOID(fullOID)]
			if !ok {
				tc.log.Debugf("Missing PDU for cached OID %s", fullOID)
				continue
			}

			value, err := tc.valProc.processValue(sym, pdu)
			if err != nil {
				tc.log.Debugf("Error processing value for %s: %v", sym.Name, err)
				continue
			}

			metric, err := buildTableMetric(sym, pdu, value, rowTags, staticTags)
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

	if tc.snmpClient.Version() == gosnmp.Version1 {
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

func buildTagColumnOIDs(cfg ddprofiledefinition.MetricsConfig) map[string][]ddprofiledefinition.MetricTagConfig {
	tagColumnOIDs := make(map[string][]ddprofiledefinition.MetricTagConfig)
	for _, tagCfg := range cfg.MetricTags {
		if tagCfg.Table == "" || tagCfg.Table == cfg.Table.Name {
			oid := trimOID(tagCfg.Symbol.OID)
			tagColumnOIDs[oid] = append(tagColumnOIDs[oid], tagCfg)
		}
	}
	return tagColumnOIDs
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
