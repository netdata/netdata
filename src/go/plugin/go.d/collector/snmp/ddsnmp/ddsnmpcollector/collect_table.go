// SPDX-License-Identifier: GPL-3.0-or-later

package ddsnmpcollector

import (
	"errors"
	"fmt"
	"maps"
	"strings"

	"github.com/gosnmp/gosnmp"

	"github.com/netdata/netdata/go/plugins/plugin/go.d/collector/snmp/ddsnmp"
	"github.com/netdata/netdata/go/plugins/plugin/go.d/collector/snmp/ddsnmp/ddprofiledefinition"
)

// tableWalkResult holds the walked data for a single table
type tableWalkResult struct {
	tableOID string
	pdus     map[string]gosnmp.SnmpPDU
	config   ddprofiledefinition.MetricsConfig
}

func (c *Collector) collectTableMetrics(prof *ddsnmp.Profile) ([]Metric, error) {
	// Phase 1: Walk all tables and collect raw data
	walkResults, err := c.walkAllTables(prof)
	if err != nil {
		return nil, err
	}

	// Phase 2: Process walked data into metrics
	return c.processTableWalkResults(walkResults)
}

// Phase 1: Walk all tables
func (c *Collector) walkAllTables(prof *ddsnmp.Profile) ([]tableWalkResult, error) {
	var results []tableWalkResult
	var errs []error
	var missingOIDs []string

	// Map to store walked data by table OID
	walkedTables := make(map[string]map[string]gosnmp.SnmpPDU)

	// Map to track which tables need to be walked
	tablesToWalk := make(map[string]bool)

	// First pass: identify unique tables to walk
	for _, cfg := range prof.Definition.Metrics {
		if cfg.IsScalar() || cfg.Table.OID == "" {
			continue
		}

		tableOID := cfg.Table.OID
		if c.missingOIDs[trimOID(tableOID)] {
			missingOIDs = append(missingOIDs, tableOID)
			continue
		}

		tablesToWalk[tableOID] = true
	}

	// Walk each unique table once
	for tableOID := range tablesToWalk {
		pdus, err := c.snmpWalk(tableOID)
		if err != nil {
			errs = append(errs, fmt.Errorf("failed to walk table OID '%s': %w", tableOID, err))
			continue
		}

		if len(pdus) > 0 {
			walkedTables[tableOID] = pdus
		}
	}

	// Second pass: create results for ALL metric configs
	for _, cfg := range prof.Definition.Metrics {
		if cfg.IsScalar() || cfg.Table.OID == "" {
			continue
		}

		pdus, ok := walkedTables[cfg.Table.OID]
		if !ok {
			continue
		}

		results = append(results, tableWalkResult{
			tableOID: cfg.Table.OID,
			pdus:     pdus,
			config:   cfg,
		})
	}

	if len(missingOIDs) > 0 {
		c.log.Debugf("table metrics missing OIDs: %v", missingOIDs)
	}

	if len(results) == 0 && len(errs) > 0 {
		return nil, errors.Join(errs...)
	}

	return results, nil
}

// Phase 2: Process walked data
func (c *Collector) processTableWalkResults(walkResults []tableWalkResult) ([]Metric, error) {
	var metrics []Metric
	var errs []error

	// Build a map for quick lookup of walked data by table OID
	walkedData := make(map[string]map[string]gosnmp.SnmpPDU)
	for _, result := range walkResults {
		walkedData[result.tableOID] = result.pdus
	}

	// Build a map of table name to OID for cross-table lookups
	tableNameToOID := make(map[string]string)
	for _, result := range walkResults {
		if result.config.Table.Name != "" {
			tableNameToOID[result.config.Table.Name] = result.tableOID
		}
	}

	// Process each table's walked data
	for _, result := range walkResults {
		tableMetrics, err := c.processTableData(result.config, result.pdus, walkedData, tableNameToOID)
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

// Process a single table's data
func (c *Collector) processTableData(cfg ddprofiledefinition.MetricsConfig, pdus map[string]gosnmp.SnmpPDU, allWalkedData map[string]map[string]gosnmp.SnmpPDU, tableNameToOID map[string]string) ([]Metric, error) {
	// Try to use cache if available
	if cachedOIDs, cachedTags, ok := c.tableCache.getCachedData(cfg); ok {
		metrics, err := c.collectTableWithCache(cfg, cachedOIDs, cachedTags, buildColumnOIDs(cfg))
		if err == nil {
			c.log.Debugf("Successfully collected table %s using cache", cfg.Table.Name)
			return metrics, nil
		}
		c.log.Debugf("Cached collection failed for table %s, falling back to process walked data: %v", cfg.Table.Name, err)
	}

	// Process without cache
	columnOIDs := buildColumnOIDs(cfg)
	tagColumnOIDs := buildTagColumnOIDs(cfg)

	allColumnOIDs := make([]string, 0, len(columnOIDs)+len(tagColumnOIDs))
	for oid := range columnOIDs {
		allColumnOIDs = append(allColumnOIDs, oid)
	}
	for oid := range tagColumnOIDs {
		allColumnOIDs = append(allColumnOIDs, oid)
	}

	// Group PDUs by row index and build cache structure
	rows := make(map[string]map[string]gosnmp.SnmpPDU)
	oidCache := make(map[string]map[string]string) // For caching: index -> column OID -> full OID
	tagCache := make(map[string]map[string]string) // For caching: index -> tag name -> value

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

	rowStaticTags := make(map[string]string)
	for _, tag := range cfg.StaticTags {
		if n, v, _ := strings.Cut(tag, ":"); n != "" && v != "" {
			rowStaticTags[n] = v
		}
	}

	var metrics []Metric

	for index, rowPDUs := range rows {
		rowTags := make(map[string]string)

		// Process tags for this row
		for columnOID, tagCfgs := range tagColumnOIDs {
			pdu, ok := rowPDUs[columnOID]
			if !ok {
				continue
			}

			for _, tagCfg := range tagCfgs {
				tags, err := processTableMetricTagValue(tagCfg, pdu)
				if err != nil {
					c.log.Debugf("Error processing tag %s: %v", tagCfg.Tag, err)
					continue
				}

				for k, v := range tags {
					rowTags[k] = v
					tagCache[index][k] = v
				}
			}
		}

		// Process cross-table tags
		for _, tagCfg := range cfg.MetricTags {
			// Skip if not a cross-table tag
			if tagCfg.Table == "" || tagCfg.Table == cfg.Table.Name {
				continue
			}

			// Skip if it's an index-based tag (handled separately)
			if tagCfg.Index != 0 {
				continue
			}

			// Find the referenced table's OID
			refTableOID, ok := tableNameToOID[tagCfg.Table]
			if !ok {
				c.log.Debugf("Cannot find table OID for referenced table %s", tagCfg.Table)
				continue
			}

			// Get the walked data for the referenced table
			refTablePDUs, ok := allWalkedData[refTableOID]
			if !ok {
				c.log.Debugf("No walked data for referenced table %s (OID: %s)", tagCfg.Table, refTableOID)
				continue
			}

			lookupIndex := index
			if len(tagCfg.IndexTransform) > 0 {
				lookupIndex = applyIndexTransform(index, tagCfg.IndexTransform)
				if lookupIndex == "" {
					c.log.Debugf("Index transformation failed for index %s with transforms %v", index, tagCfg.IndexTransform)
					continue
				}
			}

			// Look up the value from the referenced table using the same index
			refColumnOID := trimOID(tagCfg.Symbol.OID)
			refFullOID := refColumnOID + "." + lookupIndex

			pdu, ok := refTablePDUs[refFullOID]
			if !ok {
				c.log.Debugf("Cannot find cross-table tag value at OID %s for table %s (lookup index: %s)", refFullOID, tagCfg.Table, lookupIndex)
				continue
			}

			// Process the cross-table tag value
			tags, err := processTableMetricTagValue(tagCfg, pdu)
			if err != nil {
				c.log.Debugf("Error processing cross-table tag %s from table %s: %v", tagCfg.Tag, tagCfg.Table, err)
				continue
			}

			for k, v := range tags {
				rowTags[k] = v
				tagCache[index][k] = v
			}
		}

		// Process index-based tags
		for _, tagCfg := range cfg.MetricTags {
			// Skip if not an index-based tag
			if tagCfg.Index == 0 {
				continue
			}

			indexValue, ok := getIndexPosition(index, tagCfg.Index)
			if !ok {
				c.log.Debugf("Cannot extract position %d from index %s", tagCfg.Index, index)
				continue
			}

			tagName := ternary(tagCfg.Tag != "", tagCfg.Tag, fmt.Sprintf("index%d", tagCfg.Index))

			if v, ok := tagCfg.Mapping[indexValue]; ok {
				indexValue = v
			}

			rowTags[tagName] = indexValue
			tagCache[index][tagName] = indexValue
		}

		// Process metrics for this row
		for columnOID, sym := range columnOIDs {
			pdu, ok := rowPDUs[columnOID]
			if !ok {
				continue
			}

			value, err := processSymbolValue(sym, pdu)
			if err != nil {
				c.log.Debugf("Error processing value for %s: %v", sym.Name, err)
				continue
			}

			metric := Metric{
				Name:        sym.Name,
				Value:       value,
				StaticTags:  ternary(len(rowStaticTags) > 0, rowStaticTags, nil),
				Tags:        maps.Clone(ternary(len(rowTags) > 0, rowTags, nil)),
				Unit:        sym.Unit,
				Description: sym.Description,
				MetricType:  getMetricType(sym, pdu),
				Family:      sym.Family,
				Mappings:    convSymMappingToNumeric(sym),
				IsTable:     true,
			}

			metrics = append(metrics, metric)
		}
	}

	deps := extractTableDependencies(cfg, tableNameToOID)

	// Cache the processed data
	c.tableCache.cacheData(cfg, oidCache, tagCache, deps)
	c.log.Debugf("Cached table %s structure with %d rows", cfg.Table.Name, len(oidCache))

	return metrics, nil
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

func (c *Collector) collectTableWithCache(
	cfg ddprofiledefinition.MetricsConfig,
	cachedOIDs map[string]map[string]string,
	cachedTags map[string]map[string]string,
	columnOIDs map[string]ddprofiledefinition.SymbolConfig,
) ([]Metric, error) {
	var oidsToGet []string

	for _, columns := range cachedOIDs {
		for columnOID, fullOID := range columns {
			// Only GET metric columns, tags are cached
			if _, isMetric := columnOIDs[columnOID]; isMetric {
				oidsToGet = append(oidsToGet, fullOID)
			}
		}
	}

	if len(oidsToGet) == 0 {
		return nil, nil
	}

	pdus, err := c.snmpGet(oidsToGet)
	if err != nil {
		return nil, fmt.Errorf("failed to get cached OIDs: %w", err)
	}

	if len(pdus) < len(oidsToGet)/2 { // If we got less than half, probably table structure changed
		return nil, fmt.Errorf("table structure may have changed, got %d/%d PDUs", len(pdus), len(oidsToGet))
	}

	rowStaticTags := make(map[string]string)

	for _, tag := range cfg.StaticTags {
		if n, v, _ := strings.Cut(tag, ":"); n != "" && v != "" {
			rowStaticTags[n] = v
		}
	}

	var metrics []Metric

	for index, columns := range cachedOIDs {
		rowTags := make(map[string]string)

		if tags, ok := cachedTags[index]; ok {
			for k, v := range tags {
				rowTags[k] = v
			}
		}

		for columnOID, fullOID := range columns {
			sym, isMetric := columnOIDs[columnOID]
			if !isMetric {
				continue
			}

			pdu, ok := pdus[trimOID(fullOID)]
			if !ok {
				c.log.Debugf("Missing PDU for cached OID %s", fullOID)
				continue
			}

			value, err := processSymbolValue(sym, pdu)
			if err != nil {
				c.log.Debugf("Error processing value for %s: %v", sym.Name, err)
				continue
			}

			metric := Metric{
				Name:        sym.Name,
				Value:       value,
				StaticTags:  ternary(len(rowStaticTags) > 0, rowStaticTags, nil),
				Tags:        ternary(len(rowTags) > 0, rowTags, nil),
				Unit:        sym.Unit,
				Description: sym.Description,
				MetricType:  getMetricType(sym, pdu),
				Family:      sym.Family,
				Mappings:    convSymMappingToNumeric(sym),
				IsTable:     true,
			}

			metrics = append(metrics, metric)
		}
	}

	return metrics, nil
}

func processTableMetricTagValue(cfg ddprofiledefinition.MetricTagConfig, pdu gosnmp.SnmpPDU) (map[string]string, error) {
	val, err := convPduToStringf(pdu, cfg.Symbol.Format)
	if err != nil {
		return nil, err
	}

	tags := make(map[string]string)
	tagName := ternary(cfg.Tag != "", cfg.Tag, cfg.Symbol.Name)

	switch {
	case len(cfg.Mapping) > 0:
		if v, ok := cfg.Mapping[val]; ok {
			val = v
		}
		tags[tagName] = val
	case cfg.Pattern != nil:
		if sm := cfg.Pattern.FindStringSubmatch(val); len(sm) > 0 {
			for name, tmpl := range cfg.Tags {
				tags[name] = replaceSubmatches(tmpl, sm)
			}
		}
	case cfg.Symbol.ExtractValueCompiled != nil:
		if sm := cfg.Symbol.ExtractValueCompiled.FindStringSubmatch(val); len(sm) > 1 {
			tags[tagName] = sm[1]
		}
	case cfg.Symbol.MatchPatternCompiled != nil:
		if sm := cfg.Symbol.MatchPatternCompiled.FindStringSubmatch(val); len(sm) > 0 {
			tags[tagName] = replaceSubmatches(cfg.Symbol.MatchValue, sm)
		}
	default:
		tags[tagName] = val
	}

	return tags, nil
}

func (c *Collector) snmpWalk(oid string) (map[string]gosnmp.SnmpPDU, error) {
	pdus := make(map[string]gosnmp.SnmpPDU)

	var resp []gosnmp.SnmpPDU
	var err error

	if c.snmpClient.Version() == gosnmp.Version1 {
		resp, err = c.snmpClient.WalkAll(oid)
	} else {
		resp, err = c.snmpClient.BulkWalkAll(oid)
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
		c.missingOIDs[trimOID(oid)] = true
	}

	return pdus, nil
}

// getIndexPosition extracts a specific position from an index
// Position uses 1-based indexing as per the profile format
// Example: index "7.8.9", position 2 → "8"
func getIndexPosition(index string, position uint) (string, bool) {
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

// applyIndexTransform applies index transformation rules to extract a subset of the index
// Example: index "1.6.0.36.155.53.3.246", transform [{start: 1, end: 7}] → "6.0.36.155.53.3.246"
func applyIndexTransform(index string, transforms []ddprofiledefinition.MetricIndexTransform) string {
	if len(transforms) == 0 {
		return index
	}

	parts := strings.Split(index, ".")
	var result []string

	for _, transform := range transforms {
		start := transform.Start
		end := transform.End

		if int(start) >= len(parts) || end < start || int(end) >= len(parts) {
			continue
		}

		// Extract the range (inclusive)
		result = append(result, parts[start:end+1]...)
	}

	return strings.Join(result, ".")
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
