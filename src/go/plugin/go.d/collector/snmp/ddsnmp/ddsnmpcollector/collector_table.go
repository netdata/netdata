// SPDX-License-Identifier: GPL-3.0-or-later

package ddsnmpcollector

import (
	"errors"
	"fmt"
	"maps"
	"slices"
	"strings"
	"time"

	"github.com/gosnmp/gosnmp"

	"github.com/netdata/netdata/go/plugins/logger"
	"github.com/netdata/netdata/go/plugins/plugin/go.d/collector/snmp/ddsnmp"
	"github.com/netdata/netdata/go/plugins/plugin/go.d/collector/snmp/ddsnmp/ddprofiledefinition"
)

const (
	tableRowsPartialErrorLogKey    = "snmp-table-rows-partial-error:"
	tableRowsProcessingErrorLogKey = "snmp-table-rows-processing-error:"
	tableRowsErrorLogEvery         = time.Hour
)

// tableCollector handles collection of SNMP table metrics
type tableCollector struct {
	snmpClient      gosnmp.Handler
	disableBulkWalk bool
	missingOIDs     map[string]bool
	tableCache      *tableCache
	log             *logger.Logger
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
		rowProcessor:    newTableRowProcessor(log),
	}
}

// Collect gathers all table metrics from the profile
func (tc *tableCollector) collect(prof *ddsnmp.Profile, stats *ddsnmp.CollectionStats) ([]ddsnmp.Metric, error) {
	return tc.collectWithSymbolMode(prof, tableSymbolModeValue, stats)
}

func (tc *tableCollector) collectTopology(prof *ddsnmp.Profile, stats *ddsnmp.CollectionStats) ([]ddsnmp.Metric, error) {
	return tc.collectWithSymbolMode(prof, tableSymbolModePresence, stats)
}

func (tc *tableCollector) collectWithSymbolMode(
	prof *ddsnmp.Profile,
	mode tableSymbolMode,
	stats *ddsnmp.CollectionStats,
) ([]ddsnmp.Metric, error) {
	session := newTableCollectionSession(tc, buildTableIdentity([]*ddsnmp.Profile{prof}))
	scope := session.addScope(prof, mode, stats)
	session.resolve()
	return session.collectScope(scope)
}

type (
	tableCollectionContext struct {
		walkedData     map[string]map[string]gosnmp.SnmpPDU
		tableNameToOID map[string]string
	}

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

		symbolMode tableSymbolMode
		// cacheStructure enables instance-OID and tag snapshots for ordinary
		// value tables. Presence, BGP, and licensing rows are always fresh.
		cacheStructure bool

		// === Computed during processing (set by various methods) ===

		// columnOIDs maps column OIDs to all symbol configurations that read from them.
		// A single column can back multiple metrics when profiles use different
		// extract_value rules on the same raw OID, such as separate code/subcode fields.
		// Key: column OID (e.g., "1.3.6.1.2.1.2.2.1.10"), Value: symbol configs
		columnOIDs map[string][]ddprofiledefinition.SymbolConfig

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
		orderedTags        []orderedTagConfig
		rejected           *uint64
		dependencyRejected *uint64
		acquisition        *acquisitionTableObservation
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
	// Key: column OID, Value: symbol configurations
	columnOIDs map[string][]ddprofiledefinition.SymbolConfig

	// pdus contains current metric values fetched via SNMP GET
	// Only contains metric columns (not tag columns, which are cached)
	// Key: full OID (trimmed), Value: current PDU value
	pdus map[string]gosnmp.SnmpPDU

	tableName     string
	profileSource string
	symbolMode    tableSymbolMode
	acquisition   *acquisitionTableObservation
}

type tableSymbolMode uint8

const (
	tableSymbolModeValue tableSymbolMode = iota
	tableSymbolModePresence
)

// tryCollectFromCache attempts to collect metrics using cached data
func (tc *tableCollector) tryCollectFromCache(
	cfg ddprofiledefinition.MetricsConfig,
	profileSource string,
	mode tableSymbolMode,
	stats *ddsnmp.CollectionStats,
	acquisition *acquisitionTableObservation,
) []ddsnmp.Metric {
	cachedOIDs, cachedTags, ok := tc.tableCache.getCachedData(cfg)
	if !ok {
		return nil
	}

	columnOIDs := buildColumnOIDs(cfg)

	ctx := &cacheProcessingContext{
		config:        cfg,
		cachedOIDs:    cachedOIDs,
		cachedTags:    cachedTags,
		columnOIDs:    columnOIDs,
		tableName:     cfg.Table.Name,
		profileSource: profileSource,
		symbolMode:    mode,
		acquisition:   acquisition,
	}

	metrics, err := tc.collectWithCache(ctx, stats)
	if err != nil {
		tc.log.Debugf("Cached collection failed for table %s: %v", cfg.Table.Name, err)
		return nil
	}
	if metrics == nil {
		return nil
	}

	stats.Metrics.Rows += int64(len(cachedOIDs))
	tc.log.Debugf("Successfully collected table %s using cache", cfg.Table.Name)
	return metrics
}

// processTableData processes walked table data
func (tc *tableCollector) processTableData(ctx *tableProcessingContext, collectionCtx *tableCollectionContext, stats *ddsnmp.CollectionStats) ([]ddsnmp.Metric, error) {
	ctx.columnOIDs = buildColumnOIDs(ctx.config)

	ctx.orderedTags = buildOrderedTags(ctx.config)

	ctx.rows, ctx.oidCache, ctx.tagCache = tc.organizePDUsByRow(ctx)

	ctx.staticTags = parseStaticTags(ctx.config.StaticTags)

	metrics, err := tc.processRows(ctx, stats)

	if ctx.cacheStructure {
		dependenciesReady := len(ctx.rows) == 0 || tc.resolvedDependenciesAvailable(ctx.config, collectionCtx)
		switch {
		case len(ctx.rows) > 0 && dependenciesReady:
			deps := extractTableDependencies(ctx.config, collectionCtx.tableNameToOID)
			tc.tableCache.cacheRows(ctx.config, ctx.oidCache, ctx.tagCache, deps)
			tc.log.Debugf("Cached table %s structure with %d rows", ctx.config.Table.Name, len(ctx.oidCache))
		case len(ctx.rows) > 0:
			tc.tableCache.invalidateConfig(ctx.config)
		case len(ctx.pdus) == 0:
			tc.tableCache.invalidateTable(ctx.config.Table.OID)
		default:
			tc.tableCache.invalidateConfig(ctx.config)
		}
	}

	return metrics, err
}

func (tc *tableCollector) resolvedDependenciesAvailable(cfg ddprofiledefinition.MetricsConfig, collectionCtx *tableCollectionContext) bool {
	for _, tableOID := range extractTableDependencies(cfg, collectionCtx.tableNameToOID) {
		if _, ok := collectionCtx.walkedData[tableOID]; !ok {
			return false
		}
	}
	return true
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
	if ctx.cacheStructure {
		oidCache = make(map[string]map[string]string)
		tagCache = make(map[string]map[string]string)
	}

	for oid, pdu := range ctx.pdus {
		for _, columnOID := range allColumnOIDs {
			if after, ok := strings.CutPrefix(oid, columnOID+"."); ok {
				index := after

				if rows[index] == nil {
					rows[index] = make(map[string]gosnmp.SnmpPDU)
					if ctx.cacheStructure {
						oidCache[index] = make(map[string]string)
						tagCache[index] = make(map[string]string)
					}
				}
				rows[index][columnOID] = pdu
				if ctx.cacheStructure {
					oidCache[index][columnOID] = oid
				}
				break
			}
		}
	}

	for index, row := range rows {
		hasSymbol := false
		for columnOID := range row {
			if _, ok := ctx.columnOIDs[columnOID]; ok {
				hasSymbol = true
				break
			}
		}
		if !hasSymbol {
			delete(rows, index)
			if ctx.cacheStructure {
				delete(oidCache, index)
				delete(tagCache, index)
			}
		}
	}

	return rows, oidCache, tagCache
}

// processRows processes all rows and returns metrics
func (tc *tableCollector) processRows(ctx *tableProcessingContext, stats *ddsnmp.CollectionStats) ([]ddsnmp.Metric, error) {
	var metrics []ddsnmp.Metric
	var errs []error

	crossTableCtx := newCrossTableContext(ctx.walkedData, ctx.tableNameToOID)

	var rowOrdinal uint32
	for index, rowPDUs := range ctx.rows {
		currentRowOrdinal := rowOrdinal
		rowOrdinal++
		row := &tableRowData{
			index:      index,
			pdus:       rowPDUs,
			tags:       make(map[string]string),
			staticTags: ctx.staticTags,
			tableName:  ctx.config.Table.Name,
		}
		crossTableCtx.rowTags = row.tags

		rowCtx := &tableRowProcessingContext{
			config:             ctx.config,
			columnOIDs:         ctx.columnOIDs,
			crossTableCtx:      crossTableCtx,
			orderedTags:        ctx.orderedTags,
			symbolMode:         ctx.symbolMode,
			rejected:           ctx.rejected,
			dependencyRejected: ctx.dependencyRejected,
		}
		rowMetrics, err := tc.rowProcessor.processRow(row, rowCtx)
		if err != nil {
			stats.Errors.Processing.Table++
			errs = append(errs, err)
			continue
		}

		if ctx.cacheStructure {
			maps.Copy(ctx.tagCache[index], row.tags)
		}

		metrics = append(metrics, rowMetrics...)
		ctx.acquisition.addValueReferences(currentRowOrdinal, len(rowMetrics))
	}

	if len(errs) > 0 {
		tc.log.Warningf("failed to collect table metrics: %v", errors.Join(errs...))
	}

	return metrics, nil
}

// collectWithCache collects metrics using cached structure
func (tc *tableCollector) collectWithCache(ctx *cacheProcessingContext, stats *ddsnmp.CollectionStats) ([]ddsnmp.Metric, error) {
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
	pdus, err := tc.snmpGet(oidsToGet, stats)
	if err != nil {
		return nil, fmt.Errorf("failed to get cached OIDs: %w", err)
	}

	// Apply the same eligibility rule as a fresh WALK: every cached row must
	// still contain at least one current configured symbol PDU.
	for index, columns := range ctx.cachedOIDs {
		hasSymbol := false
		for columnOID, fullOID := range columns {
			if _, isMetric := ctx.columnOIDs[columnOID]; !isMetric {
				continue
			}
			if _, ok := pdus[trimOID(fullOID)]; ok {
				hasSymbol = true
				break
			}
		}
		if !hasSymbol {
			return nil, fmt.Errorf("table structure may have changed, row '%s' has no current symbol PDU", index)
		}
	}

	// Add PDUs to context and build metrics
	ctx.pdus = pdus
	return tc.buildMetricsFromCache(ctx, stats)
}

// buildMetricsFromCache builds metrics from cached structure and current values
func (tc *tableCollector) buildMetricsFromCache(ctx *cacheProcessingContext, stats *ddsnmp.CollectionStats) ([]ddsnmp.Metric, error) {
	staticTags := parseStaticTags(ctx.config.StaticTags)
	var metrics []ddsnmp.Metric
	var firstErr error
	var errorCount int

	var rowOrdinal uint32
	for index, columns := range ctx.cachedOIDs {
		currentRowOrdinal := rowOrdinal
		rowOrdinal++
		valueStart := len(metrics)
		// Get cached tags for this row
		rowTags := make(map[string]string)
		if tags, ok := ctx.cachedTags[index]; ok {
			maps.Copy(rowTags, tags)
		}
		row := &tableRowData{
			tags:       rowTags,
			staticTags: staticTags,
			tableName:  ctx.tableName,
		}

		// Process each metric column
		for columnOID, fullOID := range columns {
			syms, isMetric := ctx.columnOIDs[columnOID]
			if !isMetric {
				continue
			}

			pdu, ok := ctx.pdus[trimOID(fullOID)]
			if !ok {
				tc.log.Debugf("Missing PDU for cached OID %s", fullOID)
				continue
			}

			for _, sym := range syms {
				metric, err := tc.rowProcessor.createMetric(sym, pdu, row, ctx.symbolMode)
				if err != nil {
					stats.Errors.Processing.Table++
					errorCount++
					if firstErr == nil {
						firstErr = fmt.Errorf(
							"profile %q table %q (%s) row %q symbol %q (%s), cached column %s instance %s returned %s type %v could not be processed as a metric value",
							ctx.profileSource,
							ctx.tableName,
							ctx.config.Table.OID,
							index,
							sym.Name,
							sym.OID,
							columnOID,
							fullOID,
							pdu.Name,
							pdu.Type,
						)
					}
					continue
				}
				if metric == nil {
					continue
				}

				metrics = append(metrics, *metric)
			}
		}
		ctx.acquisition.addValueReferences(currentRowOrdinal, len(metrics)-valueStart)
	}

	if errorCount > 0 {
		key := tableRowsProcessingErrorLogKey + ctx.profileSource + "|" + ctx.config.Table.OID + "|" + tc.tableCache.generateConfigID(ctx.config)
		tc.log.Limit(key, 1, tableRowsErrorLogEvery).
			Warningf("failed to collect %d cached table metrics; first error: %v", errorCount, firstErr)
	}

	return metrics, nil
}

// SNMP operations

func (tc *tableCollector) snmpWalk(oid string, stats *ddsnmp.CollectionStats, execution *AcquisitionExecutionReport) (map[string]gosnmp.SnmpPDU, error) {
	pdus := make(map[string]gosnmp.SnmpPDU)

	var resp []gosnmp.SnmpPDU
	var err error

	stats.SNMP.WalkRequests++

	useWalk := tc.snmpClient.Version() == gosnmp.Version1 || tc.disableBulkWalk
	var started time.Time
	if execution != nil {
		started = time.Now()
	}
	if useWalk {
		resp, err = tc.snmpClient.WalkAll(oid)
	} else {
		resp, err = tc.snmpClient.BulkWalkAll(oid)
	}
	if execution != nil {
		elapsed := time.Since(started)
		execution.Walks = append(execution.Walks, AcquisitionWalkReport{RootOID: trimOID(oid), Elapsed: elapsed, Failed: err != nil})
	}
	if err != nil {
		return nil, err
	}

	stats.SNMP.WalkPDUs += int64(len(resp))

	for _, pdu := range resp {
		if isPduWithData(pdu) {
			pdus[trimOID(pdu.Name)] = pdu
		} else {
			stats.Errors.MissingOIDs++
		}
	}

	return pdus, nil
}

func (tc *tableCollector) snmpGet(oids []string, stats *ddsnmp.CollectionStats) (map[string]gosnmp.SnmpPDU, error) {
	return getSNMPValues(tc.snmpClient, oids, tc.missingOIDs, stats)
}

func parseStaticTags(staticTags []ddprofiledefinition.StaticMetricTagConfig) map[string]string {
	tags := make(map[string]string, len(staticTags))
	for _, tag := range staticTags {
		if tag.Tag != "" && tag.Value != "" {
			tags[tag.Tag] = tag.Value
		}
	}
	return tags
}

func buildColumnOIDs(cfg ddprofiledefinition.MetricsConfig) map[string][]ddprofiledefinition.SymbolConfig {
	columnOIDs := make(map[string][]ddprofiledefinition.SymbolConfig)
	for _, sym := range cfg.Symbols {
		columnOID := trimOID(sym.OID)
		columnOIDs[columnOID] = append(columnOIDs[columnOID], sym)
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
	slices.Sort(result)

	return result
}

func buildOrderedTags(cfg ddprofiledefinition.MetricsConfig) []orderedTagConfig {
	var ordered []orderedTagConfig

	for _, tagCfg := range cfg.MetricTags {
		var tt tagType
		switch {
		case isIndexTagConfig(tagCfg):
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

func isIndexTagConfig(tagCfg ddprofiledefinition.MetricTagConfig) bool {
	if tagCfg.Index != 0 {
		return true
	}

	if tagCfg.Table != "" {
		return false
	}

	if tagCfg.Symbol.OID != "" {
		return false
	}

	return len(tagCfg.IndexTransform) > 0 ||
		tagCfg.Symbol.Format != "" ||
		tagCfg.Symbol.ExtractValue != "" ||
		tagCfg.Symbol.MatchPattern != "" ||
		tagCfg.Mapping.HasItems()
}
