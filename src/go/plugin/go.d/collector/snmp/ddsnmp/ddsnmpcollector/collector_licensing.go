// SPDX-License-Identifier: GPL-3.0-or-later

package ddsnmpcollector

import (
	"errors"
	"fmt"
	"maps"
	"slices"
	"strconv"
	"strings"
	"time"

	"github.com/gosnmp/gosnmp"

	"github.com/netdata/netdata/go/plugins/plugin/go.d/collector/snmp/ddsnmp"
	"github.com/netdata/netdata/go/plugins/plugin/go.d/collector/snmp/ddsnmp/ddprofiledefinition"
)

const (
	licenseRowsPartialErrorLogKey = "snmp-licensing-rows-partial-error:"
	licenseRowsFailedLogKey       = "snmp-licensing-rows-failed:"
	licenseRowsErrorLogEvery      = time.Hour
)

type licenseValueContext struct {
	rowIndex string
	rowPDUs  map[string]gosnmp.SnmpPDU
	pdus     map[string]gosnmp.SnmpPDU
}

func (c *Collector) collectLicenseRows(prof *ddsnmp.Profile, stats *ddsnmp.CollectionStats) ([]ddsnmp.LicenseRow, error) {
	if prof.Definition == nil || len(prof.Definition.Licensing) == 0 {
		return nil, nil
	}

	var rows []ddsnmp.LicenseRow
	var errs []error

	scalarRows, err := c.collectScalarLicenseRows(prof.Definition.Licensing, stats)
	if err != nil {
		errs = append(errs, err)
	}
	rows = append(rows, scalarRows...)

	tableRows, err := c.collectTableLicenseRows(prof.Definition.Licensing, stats)
	if err != nil {
		errs = append(errs, err)
	}
	rows = append(rows, tableRows...)

	if len(rows) == 0 && len(errs) > 0 {
		return nil, errors.Join(errs...)
	}
	if len(errs) > 0 {
		c.log.Limit(licenseRowsPartialErrorLogKey+prof.SourceFile, 1, licenseRowsErrorLogEvery).
			Warningf("collecting licensing rows for profile %q partially failed: %v", prof.SourceFile, errors.Join(errs...))
	}

	return rows, nil
}

func (c *Collector) collectScalarLicenseRows(configs []ddprofiledefinition.LicensingConfig, stats *ddsnmp.CollectionStats) ([]ddsnmp.LicenseRow, error) {
	var rows []ddsnmp.LicenseRow
	var errs []error

	for _, cfg := range configs {
		if cfg.Table.OID != "" {
			continue
		}

		oids, missingOIDs := c.licensingScalarOIDs(cfg)
		if len(missingOIDs) > 0 {
			c.log.Debugf("licensing scalar row %q missing OIDs: %v", licensingConfigDisplayName(cfg), missingOIDs)
			stats.Errors.MissingOIDs += int64(len(missingOIDs))
		}

		var pdus map[string]gosnmp.SnmpPDU
		var err error
		if len(oids) > 0 {
			pdus, err = c.scalarCollector.getScalarValues(oids, stats)
			if err != nil {
				stats.Errors.SNMP++
				errs = append(errs, fmt.Errorf("licensing scalar row %q: %w", licensingConfigDisplayName(cfg), err))
				continue
			}
		}

		row, ok, err := c.buildScalarLicenseRow(cfg, pdus)
		if err != nil {
			stats.Errors.Processing.Licensing++
			errs = append(errs, fmt.Errorf("licensing scalar row %q: %w", licensingConfigDisplayName(cfg), err))
			continue
		}
		if ok {
			rows = append(rows, row)
		}
	}

	if len(rows) == 0 && len(errs) > 0 {
		return nil, errors.Join(errs...)
	}
	return rows, nil
}

func (c *Collector) collectTableLicenseRows(configs []ddprofiledefinition.LicensingConfig, stats *ddsnmp.CollectionStats) ([]ddsnmp.LicenseRow, error) {
	var rows []ddsnmp.LicenseRow
	var errs []error
	walkedData := make(map[string]map[string]gosnmp.SnmpPDU)
	tableNameToOID := licensingTableNameToOID(configs)

	for _, cfg := range configs {
		if cfg.Table.OID == "" {
			continue
		}
		metricsCfg := licensingConfigAsMetricsConfig(cfg)
		cachedRows, ok, err := c.collectTableLicenseRowsFromCache(cfg, metricsCfg, stats)
		if err != nil {
			c.log.Debugf("Cached licensing collection failed for table %s: %v", cfg.Table.Name, err)
		}
		if ok {
			stats.TableCache.Hits++
			stats.SNMP.TablesCached++
			rows = append(rows, cachedRows...)
			continue
		}
		stats.TableCache.Misses++

		tableOID := trimOID(cfg.Table.OID)
		pdus := walkedData[tableOID]
		if pdus == nil {
			var err error
			pdus, err = c.tableCollector.snmpWalk(cfg.Table.OID, stats)
			if err != nil {
				stats.Errors.SNMP++
				errs = append(errs, fmt.Errorf("licensing table %q: %w", licensingConfigDisplayName(cfg), err))
				continue
			}
			if len(pdus) > 0 {
				stats.SNMP.TablesWalked++
				walkedData[tableOID] = pdus
			}
		}
		if len(pdus) == 0 {
			continue
		}

		if err := c.walkLicenseTableDependencies(metricsCfg, tableNameToOID, walkedData, stats); err != nil {
			errs = append(errs, fmt.Errorf("licensing table %q dependencies: %w", licensingConfigDisplayName(cfg), err))
			continue
		}

		ctx := &tableProcessingContext{
			config:         metricsCfg,
			pdus:           pdus,
			walkedData:     walkedData,
			tableNameToOID: tableNameToOID,
		}
		ctx.columnOIDs = buildColumnOIDs(metricsCfg)
		ctx.orderedTags = buildOrderedTags(metricsCfg)
		ctx.rows, ctx.oidCache, ctx.tagCache = c.tableCollector.organizePDUsByRow(ctx)

		for rowIndex, rowPDUs := range ctx.rows {
			row, ok, err := c.buildTableLicenseRow(cfg, rowIndex, rowPDUs, ctx)
			if err != nil {
				stats.Errors.Processing.Licensing++
				errs = append(errs, fmt.Errorf("licensing table %q row %q: %w", licensingConfigDisplayName(cfg), rowIndex, err))
				continue
			}
			if ok {
				rows = append(rows, row)
			}
		}

		deps := extractTableDependencies(metricsCfg, ctx.tableNameToOID)
		c.tableCollector.tableCache.cacheData(metricsCfg, ctx.oidCache, ctx.tagCache, deps)
	}

	if len(rows) == 0 && len(errs) > 0 {
		return nil, errors.Join(errs...)
	}
	return rows, nil
}

func (c *Collector) walkLicenseTableDependencies(
	cfg ddprofiledefinition.MetricsConfig,
	tableNameToOID map[string]string,
	walkedData map[string]map[string]gosnmp.SnmpPDU,
	stats *ddsnmp.CollectionStats,
) error {
	var errs []error
	for _, depOID := range extractTableDependencies(cfg, tableNameToOID) {
		depOID = trimOID(depOID)
		if walkedData[depOID] != nil {
			continue
		}
		pdus, err := c.tableCollector.snmpWalk(depOID, stats)
		if err != nil {
			stats.Errors.SNMP++
			errs = append(errs, fmt.Errorf("table OID %q: %w", depOID, err))
			continue
		}
		if len(pdus) > 0 {
			stats.SNMP.TablesWalked++
			walkedData[depOID] = pdus
		}
	}
	return errors.Join(errs...)
}

func (c *Collector) collectTableLicenseRowsFromCache(cfg ddprofiledefinition.LicensingConfig, metricsCfg ddprofiledefinition.MetricsConfig, stats *ddsnmp.CollectionStats) ([]ddsnmp.LicenseRow, bool, error) {
	cachedOIDs, cachedTags, ok := c.tableCollector.tableCache.getCachedData(metricsCfg)
	if !ok {
		return nil, false, nil
	}

	columnOIDs := buildColumnOIDs(metricsCfg)
	var oidsToGet []string
	for _, columns := range cachedOIDs {
		for columnOID, fullOID := range columns {
			if _, ok := columnOIDs[columnOID]; ok {
				oidsToGet = append(oidsToGet, fullOID)
			}
		}
	}
	if len(oidsToGet) == 0 {
		return nil, true, nil
	}

	pdus, err := c.tableCollector.snmpGet(oidsToGet, stats)
	if err != nil {
		return nil, false, err
	}
	if len(pdus) < len(oidsToGet)/2 {
		return nil, false, fmt.Errorf("table structure may have changed, got %d/%d PDUs", len(pdus), len(oidsToGet))
	}

	var rows []ddsnmp.LicenseRow
	for rowIndex, columns := range cachedOIDs {
		rowPDUs := make(map[string]gosnmp.SnmpPDU)
		for columnOID, fullOID := range columns {
			pdu, ok := pdus[trimOID(fullOID)]
			if ok {
				rowPDUs[columnOID] = pdu
			}
		}
		rowTags := maps.Clone(cachedTags[rowIndex])
		row, ok, err := c.buildTableLicenseRowWithTags(cfg, rowIndex, rowPDUs, rowTags)
		if err != nil {
			return nil, false, fmt.Errorf("row %q: %w", rowIndex, err)
		}
		if ok {
			rows = append(rows, row)
		}
	}
	return rows, true, nil
}

func (c *Collector) buildScalarLicenseRow(cfg ddprofiledefinition.LicensingConfig, pdus map[string]gosnmp.SnmpPDU) (ddsnmp.LicenseRow, bool, error) {
	rowKey := scalarLicenseRowKey(cfg)
	row := ddsnmp.LicenseRow{
		OriginProfileID: cfg.OriginProfileID,
		RowKey:          rowKey,
		StructuralID:    licenseScalarStructuralID(cfg.OriginProfileID, rowKey),
		Tags:            parseStaticTags(cfg.StaticTags),
	}

	licenseCtx := licenseValueContext{pdus: pdus}
	if err := c.populateLicenseRow(&row, cfg, licenseCtx); err != nil {
		return ddsnmp.LicenseRow{}, false, err
	}
	if row.ID == "" {
		row.ID = cfg.ID
	}

	if len(cfg.MetricTags) > 0 {
		tags := make(map[string]string)
		ta := tagAdder{tags: tags}
		for _, tagCfg := range cfg.MetricTags {
			if tagCfg.Symbol.OID == "" {
				continue
			}
			if err := c.scalarCollector.tagProc.processTag(tagCfg, pdus, ta); err != nil {
				c.log.Debugf("Error processing scalar licensing tag %s: %v", metricTagDisplayName(tagCfg), err)
			}
		}
		mergeStringMaps(row.Tags, tags)
	}

	if !licenseRowHasSignals(row) {
		return ddsnmp.LicenseRow{}, false, nil
	}

	return row, true, nil
}

func (c *Collector) buildTableLicenseRow(cfg ddprofiledefinition.LicensingConfig, rowIndex string, rowPDUs map[string]gosnmp.SnmpPDU, ctx *tableProcessingContext) (ddsnmp.LicenseRow, bool, error) {
	rowTags := make(map[string]string)
	rowData := &tableRowData{
		index:      rowIndex,
		pdus:       rowPDUs,
		tags:       rowTags,
		staticTags: parseStaticTags(cfg.StaticTags),
		tableName:  cfg.Table.Name,
	}
	crossTableCtx := &crossTableContext{
		walkedData:       ctx.walkedData,
		tableNameToOID:   ctx.tableNameToOID,
		lookupIndexCache: make(map[crossTableLookupKey]string),
		rowTags:          rowData.tags,
	}
	rowCtx := &tableRowProcessingContext{
		config:        ctx.config,
		columnOIDs:    ctx.columnOIDs,
		crossTableCtx: crossTableCtx,
		orderedTags:   ctx.orderedTags,
	}
	if err := c.tableCollector.rowProcessor.processRowTags(rowData, rowCtx); err != nil {
		c.log.Debugf("Error processing licensing row tags for %s: %v", rowIndex, err)
	}
	maps.Copy(ctx.tagCache[rowIndex], rowData.tags)

	return c.buildTableLicenseRowWithTags(cfg, rowIndex, rowPDUs, rowData.tags)
}

func (c *Collector) buildTableLicenseRowWithTags(cfg ddprofiledefinition.LicensingConfig, rowIndex string, rowPDUs map[string]gosnmp.SnmpPDU, rowTags map[string]string) (ddsnmp.LicenseRow, bool, error) {
	rowKey := rowIndex
	row := ddsnmp.LicenseRow{
		OriginProfileID: cfg.OriginProfileID,
		TableOID:        trimOID(cfg.Table.OID),
		Table:           cfg.Table.Name,
		RowKey:          rowKey,
		StructuralID:    licenseTableStructuralID(cfg.OriginProfileID, trimOID(cfg.Table.OID), rowKey),
		Tags:            parseStaticTags(cfg.StaticTags),
	}
	mergeStringMaps(row.Tags, rowTags)

	licenseCtx := licenseValueContext{rowIndex: rowIndex, rowPDUs: rowPDUs}
	if err := c.populateLicenseRow(&row, cfg, licenseCtx); err != nil {
		return ddsnmp.LicenseRow{}, false, err
	}
	if row.ID == "" {
		row.ID = cfg.ID
	}

	if !licenseRowHasSignals(row) {
		return ddsnmp.LicenseRow{}, false, nil
	}

	return row, true, nil
}

func (c *Collector) populateLicenseRow(row *ddsnmp.LicenseRow, cfg ddprofiledefinition.LicensingConfig, ctx licenseValueContext) error {
	var err error

	if row.ID, err = c.licenseTextValue(cfg.Identity.ID, ctx); err != nil {
		return fmt.Errorf("identity.id: %w", err)
	}
	if row.Name, err = c.licenseTextValue(cfg.Identity.Name, ctx); err != nil {
		return fmt.Errorf("identity.name: %w", err)
	}
	if row.Feature, err = c.licenseTextValue(cfg.Identity.Feature, ctx); err != nil {
		return fmt.Errorf("identity.feature: %w", err)
	}
	if row.Component, err = c.licenseTextValue(cfg.Identity.Component, ctx); err != nil {
		return fmt.Errorf("identity.component: %w", err)
	}
	if row.Type, err = c.licenseTextValue(cfg.Descriptors.Type, ctx); err != nil {
		return fmt.Errorf("descriptors.type: %w", err)
	}
	if row.Impact, err = c.licenseTextValue(cfg.Descriptors.Impact, ctx); err != nil {
		return fmt.Errorf("descriptors.impact: %w", err)
	}
	if row.IsPerpetual, err = c.licenseBoolValue(cfg.Descriptors.Perpetual, ctx); err != nil {
		return fmt.Errorf("descriptors.perpetual: %w", err)
	}
	if row.IsUnlimited, err = c.licenseBoolValue(cfg.Descriptors.Unlimited, ctx); err != nil {
		return fmt.Errorf("descriptors.unlimited: %w", err)
	}

	if err := c.populateLicenseState(row, cfg.State, ctx); err != nil {
		return fmt.Errorf("state: %w", err)
	}
	if err := c.populateLicenseTimer(&row.Expiry, cfg.Signals.Expiry, ctx, "expiry"); err != nil {
		return err
	}
	if err := c.populateLicenseTimer(&row.Authorization, cfg.Signals.Authorization, ctx, "authorization"); err != nil {
		return err
	}
	if err := c.populateLicenseTimer(&row.Certificate, cfg.Signals.Certificate, ctx, "certificate"); err != nil {
		return err
	}
	if err := c.populateLicenseTimer(&row.Grace, cfg.Signals.Grace, ctx, "grace"); err != nil {
		return err
	}
	if err := c.populateLicenseUsage(&row.Usage, cfg.Signals.Usage, ctx); err != nil {
		return err
	}

	return nil
}

func (c *Collector) populateLicenseState(row *ddsnmp.LicenseRow, cfg ddprofiledefinition.LicenseStateConfig, ctx licenseValueContext) error {
	if !cfg.IsSet() {
		return nil
	}

	raw, _, err := c.licenseRawTextValue(cfg.LicenseValueConfig, ctx)
	if err != nil {
		return err
	}
	raw = licenseStateRawValueByPolicy(raw, cfg.Policy)
	severity, sourceOID, ok, err := c.licenseNumericValue(cfg.LicenseValueConfig, ctx)
	if err != nil {
		return err
	}
	if !ok {
		return nil
	}

	row.State = ddsnmp.LicenseState{
		Has:       true,
		Severity:  severity,
		Raw:       raw,
		Policy:    cfg.Policy,
		SourceOID: sourceOID,
	}

	return nil
}

func licenseStateRawValueByPolicy(raw string, policy ddprofiledefinition.LicenseStatePolicy) string {
	switch policy {
	case ddprofiledefinition.LicenseStatePolicySophos:
		switch strings.TrimSpace(raw) {
		case "0":
			return "none"
		case "1":
			return "trial"
		case "2":
			return "not_subscribed"
		case "3":
			return "subscribed"
		case "4":
			return "expired"
		case "5":
			return "deactivated"
		}
	}
	return raw
}

func (c *Collector) populateLicenseTimer(timer *ddsnmp.LicenseTimer, cfg ddprofiledefinition.LicenseTimerSignalsConfig, ctx licenseValueContext, name string) error {
	if cfg.LicenseValueConfig.IsSet() {
		if err := c.populateLicenseTimerTimestamp(timer, cfg.LicenseValueConfig, ctx, name); err != nil {
			return err
		}
	}
	if cfg.Timestamp.IsSet() {
		if err := c.populateLicenseTimerTimestamp(timer, cfg.Timestamp, ctx, name+".timestamp"); err != nil {
			return err
		}
	}
	if cfg.Remaining.IsSet() {
		if err := c.populateLicenseTimerRemaining(timer, cfg.Remaining, ctx, name+".remaining"); err != nil {
			return err
		}
	}
	return nil
}

func (c *Collector) populateLicenseTimerTimestamp(timer *ddsnmp.LicenseTimer, cfg ddprofiledefinition.LicenseValueConfig, ctx licenseValueContext, name string) error {
	value, sourceOID, ok, err := c.licenseNumericValue(cfg, ctx)
	if err != nil {
		return fmt.Errorf("%s: %w", name, err)
	}
	if !ok || licenseValueRejectedBySentinel(value, cfg.Sentinel) {
		return nil
	}

	timer.Has = true
	timer.Timestamp = value
	timer.SourceOID = sourceOID
	return nil
}

func (c *Collector) populateLicenseTimerRemaining(timer *ddsnmp.LicenseTimer, cfg ddprofiledefinition.LicenseValueConfig, ctx licenseValueContext, name string) error {
	value, sourceOID, ok, err := c.licenseNumericValue(cfg, ctx)
	if err != nil {
		return fmt.Errorf("%s: %w", name, err)
	}
	if !ok || licenseValueRejectedBySentinel(value, cfg.Sentinel) {
		return nil
	}

	timer.Has = true
	timer.RemainingSeconds = value
	timer.SourceOID = sourceOID
	return nil
}

func (c *Collector) populateLicenseUsage(usage *ddsnmp.LicenseUsage, cfg ddprofiledefinition.LicenseUsageSignalsConfig, ctx licenseValueContext) error {
	if err := c.populateLicenseUsageValue(&usage.HasUsed, &usage.Used, cfg.Used, ctx, "usage.used"); err != nil {
		return err
	}
	if err := c.populateLicenseUsageValue(&usage.HasCapacity, &usage.Capacity, cfg.Capacity, ctx, "usage.capacity"); err != nil {
		return err
	}
	if err := c.populateLicenseUsageValue(&usage.HasAvailable, &usage.Available, cfg.Available, ctx, "usage.available"); err != nil {
		return err
	}
	if err := c.populateLicenseUsageValue(&usage.HasPercent, &usage.Percent, cfg.Percent, ctx, "usage.percent"); err != nil {
		return err
	}
	return nil
}

func (c *Collector) populateLicenseUsageValue(has *bool, dst *int64, cfg ddprofiledefinition.LicenseValueConfig, ctx licenseValueContext, name string) error {
	if !cfg.IsSet() {
		return nil
	}
	value, _, ok, err := c.licenseNumericValue(cfg, ctx)
	if err != nil {
		return fmt.Errorf("%s: %w", name, err)
	}
	if !ok {
		return nil
	}
	if licenseValueRejectedBySentinel(value, cfg.Sentinel) {
		return nil
	}
	*has = true
	*dst = value
	return nil
}

func (c *Collector) licenseTextValue(cfg ddprofiledefinition.LicenseValueConfig, ctx licenseValueContext) (string, error) {
	value, ok, err := c.licenseRawTextValue(cfg, ctx)
	if err != nil || !ok {
		return "", err
	}
	sym := licenseValueSymbol(cfg)
	if mapped, ok := sym.Mapping.Lookup(value); ok {
		value = mapped
	}
	return value, nil
}

func (c *Collector) licenseRawTextValue(cfg ddprofiledefinition.LicenseValueConfig, ctx licenseValueContext) (string, bool, error) {
	if !cfg.IsSet() {
		return "", false, nil
	}
	if cfg.Value != "" {
		return cfg.Value, true, nil
	}
	if cfg.Index != 0 || len(cfg.IndexTransform) > 0 {
		value, err := c.licenseIndexValue(cfg, ctx.rowIndex)
		return value, err == nil, err
	}

	sym := licenseValueSymbol(cfg)
	if sym.OID == "" {
		return "", false, nil
	}
	pdu, ok := ctx.lookupPDU(sym.OID)
	if !ok {
		return "", false, nil
	}

	value, err := convPduToStringf(pdu, sym.Format)
	if err != nil {
		if errors.Is(err, errNoTextDateValue) {
			return "", false, nil
		}
		return "", false, err
	}

	if sym.ExtractValueCompiled != nil {
		sm := sym.ExtractValueCompiled.FindStringSubmatch(value)
		if len(sm) < 2 {
			return "", false, fmt.Errorf("extract_value did not match value %q", value)
		}
		value = sm[1]
	}
	if sym.MatchPatternCompiled != nil {
		sm := sym.MatchPatternCompiled.FindStringSubmatch(value)
		if len(sm) == 0 {
			return "", false, fmt.Errorf("match_pattern %q did not match value %q", sym.MatchPattern, value)
		}
		value = replaceSubmatches(sym.MatchValue, sm)
	}

	return value, true, nil
}

func (c *Collector) licenseNumericValue(cfg ddprofiledefinition.LicenseValueConfig, ctx licenseValueContext) (value int64, sourceOID string, ok bool, err error) {
	if !cfg.IsSet() {
		return 0, "", false, nil
	}
	if cfg.Value != "" || cfg.Index != 0 || len(cfg.IndexTransform) > 0 {
		text, err := c.licenseTextValue(cfg, ctx)
		if err != nil || text == "" {
			return 0, "", false, err
		}
		value, err := strconv.ParseInt(strings.TrimSpace(text), 10, 64)
		if err != nil {
			return 0, "", false, err
		}
		return value, "", true, nil
	}

	sym := licenseValueSymbol(cfg)
	if sym.OID == "" {
		return 0, "", false, nil
	}
	pdu, ok := ctx.lookupPDU(sym.OID)
	if !ok {
		return 0, "", false, nil
	}

	value, err = c.scalarCollector.valProc.processValue(sym, pdu)
	if err != nil {
		if errors.Is(err, errNoTextDateValue) {
			return 0, "", false, nil
		}
		return 0, "", false, err
	}

	return value, trimOID(sym.OID), true, nil
}

func (c *Collector) licenseBoolValue(cfg ddprofiledefinition.LicenseValueConfig, ctx licenseValueContext) (bool, error) {
	value, err := c.licenseTextValue(cfg, ctx)
	if err != nil || value == "" {
		return false, err
	}
	switch strings.ToLower(strings.TrimSpace(value)) {
	case "1", "true", "yes", "y", "on", "perpetual", "unlimited":
		return true, nil
	default:
		return false, nil
	}
}

func (c *Collector) licenseIndexValue(cfg ddprofiledefinition.LicenseValueConfig, rowIndex string) (string, error) {
	if rowIndex == "" {
		return "", nil
	}
	sym := licenseValueSymbol(cfg)
	tagCfg := ddprofiledefinition.MetricTagConfig{
		Index:          cfg.Index,
		IndexTransform: cfg.IndexTransform,
		Symbol:         ddprofiledefinition.SymbolConfigCompat(sym),
		Mapping:        sym.Mapping,
	}
	_, value, err := c.tableCollector.rowProcessor.processIndexTag(tagCfg, rowIndex)
	return value, err
}

func (ctx licenseValueContext) lookupPDU(oid string) (gosnmp.SnmpPDU, bool) {
	oid = trimOID(oid)
	if ctx.rowPDUs != nil {
		pdu, ok := ctx.rowPDUs[oid]
		return pdu, ok
	}
	pdu, ok := ctx.pdus[oid]
	return pdu, ok
}

func licensingConfigAsMetricsConfig(cfg ddprofiledefinition.LicensingConfig) ddprofiledefinition.MetricsConfig {
	columns := make(map[string]ddprofiledefinition.SymbolConfig)
	addColumn := func(valueCfg ddprofiledefinition.LicenseValueConfig) {
		sym := licenseValueSymbol(valueCfg)
		if sym.OID != "" {
			columns[trimOID(sym.OID)] = sym
		}
	}

	forEachLicenseValue(cfg, addColumn)
	for _, tagCfg := range cfg.MetricTags {
		if tagCfg.Symbol.OID != "" {
			columns[trimOID(tagCfg.Symbol.OID)] = ddprofiledefinition.SymbolConfig(tagCfg.Symbol)
		}
	}

	symbols := make([]ddprofiledefinition.SymbolConfig, 0, len(columns))
	for _, sym := range columns {
		symbols = append(symbols, sym)
	}

	return ddprofiledefinition.MetricsConfig{
		MIB:        licensingMetricsConfigMIB(cfg.MIB),
		Table:      cfg.Table,
		Symbols:    symbols,
		StaticTags: cfg.StaticTags,
		MetricTags: cfg.MetricTags,
	}
}

func licensingMetricsConfigMIB(mib string) string {
	if mib == "" {
		return "license"
	}
	return "license:" + mib
}

func licensingTableNameToOID(configs []ddprofiledefinition.LicensingConfig) map[string]string {
	tableNameToOID := make(map[string]string)
	crossTableOIDs := make(map[string][]string)

	for _, cfg := range configs {
		if cfg.Table.Name != "" && cfg.Table.OID != "" {
			tableNameToOID[cfg.Table.Name] = trimOID(cfg.Table.OID)
		}
	}

	for _, cfg := range configs {
		if cfg.Table.OID == "" {
			continue
		}
		metricsCfg := licensingConfigAsMetricsConfig(cfg)
		for _, tagCfg := range metricsCfg.MetricTags {
			if tagCfg.Table == "" || tagCfg.Table == cfg.Table.Name {
				continue
			}
			if _, ok := tableNameToOID[tagCfg.Table]; ok {
				continue
			}
			if tagCfg.Symbol.OID != "" {
				crossTableOIDs[tagCfg.Table] = append(crossTableOIDs[tagCfg.Table], tagCfg.Symbol.OID)
			}
			if tagCfg.LookupSymbol.OID != "" {
				crossTableOIDs[tagCfg.Table] = append(crossTableOIDs[tagCfg.Table], tagCfg.LookupSymbol.OID)
			}
		}
	}

	for tableName, oids := range crossTableOIDs {
		if oid := longestCommonOIDPrefix(oids); oid != "" {
			tableNameToOID[tableName] = oid
		}
	}

	return tableNameToOID
}

func longestCommonOIDPrefix(oids []string) string {
	if len(oids) == 0 {
		return ""
	}
	slices.Sort(oids)
	oids = slices.Compact(oids)
	prefixParts := splitOIDParts(oids[0])
	for _, oid := range oids[1:] {
		parts := splitOIDParts(oid)
		n := min(len(parts), len(prefixParts))
		i := 0
		for i < n && prefixParts[i] == parts[i] {
			i++
		}
		prefixParts = prefixParts[:i]
		if len(prefixParts) == 0 {
			return ""
		}
	}
	return strings.Join(prefixParts, ".")
}

func splitOIDParts(oid string) []string {
	parts := strings.Split(strings.Trim(oid, "."), ".")
	if len(parts) == 1 && parts[0] == "" {
		return nil
	}
	return parts
}

func (c *Collector) licensingScalarOIDs(cfg ddprofiledefinition.LicensingConfig) ([]string, []string) {
	oids := make(map[string]struct{})
	var missingOIDs []string
	addOID := func(valueCfg ddprofiledefinition.LicenseValueConfig) {
		sym := licenseValueSymbol(valueCfg)
		if sym.OID != "" {
			oid := trimOID(sym.OID)
			if c.missingOIDs[oid] {
				missingOIDs = append(missingOIDs, sym.OID)
				return
			}
			oids[oid] = struct{}{}
		}
	}

	forEachLicenseValue(cfg, addOID)
	for _, tagCfg := range cfg.MetricTags {
		if tagCfg.Symbol.OID != "" {
			oid := trimOID(tagCfg.Symbol.OID)
			if c.missingOIDs[oid] {
				missingOIDs = append(missingOIDs, tagCfg.Symbol.OID)
				continue
			}
			oids[oid] = struct{}{}
		}
	}

	result := make([]string, 0, len(oids))
	for oid := range oids {
		result = append(result, oid)
	}
	slices.Sort(result)
	slices.Sort(missingOIDs)
	missingOIDs = slices.Compact(missingOIDs)
	return result, missingOIDs
}

func forEachLicenseValue(cfg ddprofiledefinition.LicensingConfig, fn func(ddprofiledefinition.LicenseValueConfig)) {
	fn(cfg.Identity.ID)
	fn(cfg.Identity.Name)
	fn(cfg.Identity.Feature)
	fn(cfg.Identity.Component)
	fn(cfg.Descriptors.Type)
	fn(cfg.Descriptors.Impact)
	fn(cfg.Descriptors.Perpetual)
	fn(cfg.Descriptors.Unlimited)
	fn(cfg.State.LicenseValueConfig)
	forEachLicenseTimerValue(cfg.Signals.Expiry, fn)
	forEachLicenseTimerValue(cfg.Signals.Authorization, fn)
	forEachLicenseTimerValue(cfg.Signals.Certificate, fn)
	forEachLicenseTimerValue(cfg.Signals.Grace, fn)
	fn(cfg.Signals.Usage.Used)
	fn(cfg.Signals.Usage.Capacity)
	fn(cfg.Signals.Usage.Available)
	fn(cfg.Signals.Usage.Percent)
}

func forEachLicenseTimerValue(cfg ddprofiledefinition.LicenseTimerSignalsConfig, fn func(ddprofiledefinition.LicenseValueConfig)) {
	fn(cfg.LicenseValueConfig)
	fn(cfg.Timestamp)
	fn(cfg.Remaining)
}

func licenseValueSymbol(cfg ddprofiledefinition.LicenseValueConfig) ddprofiledefinition.SymbolConfig {
	sym := cfg.Symbol
	if sym.OID == "" {
		switch {
		case cfg.From != "":
			sym.OID = cfg.From
		case cfg.OID != "":
			sym.OID = cfg.OID
		}
	}
	if sym.Name == "" {
		sym.Name = cfg.Name
	}
	if sym.Name == "" && sym.OID != "" {
		sym.Name = "license:" + trimOID(sym.OID)
	}
	if sym.Format == "" {
		sym.Format = cfg.Format
	}
	if !sym.Mapping.HasItems() && cfg.Mapping.HasItems() {
		sym.Mapping = cfg.Mapping
	}
	return sym
}

func scalarLicenseRowKey(cfg ddprofiledefinition.LicensingConfig) string {
	if cfg.ID != "" {
		return cfg.ID
	}
	rowKey := ""
	forEachLicenseValue(cfg, func(valueCfg ddprofiledefinition.LicenseValueConfig) {
		if rowKey != "" {
			return
		}
		if oid := licenseValueSymbol(valueCfg).OID; oid != "" {
			rowKey = trimOID(oid)
		}
	})
	if rowKey != "" {
		return rowKey
	}
	return cfg.OriginProfileID
}

func licensingConfigDisplayName(cfg ddprofiledefinition.LicensingConfig) string {
	if cfg.ID != "" {
		return cfg.ID
	}
	if cfg.Table.Name != "" {
		return cfg.Table.Name
	}
	if cfg.Table.OID != "" {
		return cfg.Table.OID
	}
	return cfg.OriginProfileID
}

func licenseRowHasSignals(row ddsnmp.LicenseRow) bool {
	return row.State.Has ||
		row.Expiry.Has ||
		row.Authorization.Has ||
		row.Certificate.Has ||
		row.Grace.Has ||
		row.Usage.HasUsed ||
		row.Usage.HasCapacity ||
		row.Usage.HasAvailable ||
		row.Usage.HasPercent
}

func licenseValueRejectedBySentinel(value int64, policies []ddprofiledefinition.LicenseSentinelPolicy) bool {
	for _, policy := range policies {
		switch policy {
		case ddprofiledefinition.LicenseSentinelTimerZeroOrNegative:
			if value <= 0 {
				return true
			}
		case ddprofiledefinition.LicenseSentinelTimerU32Max:
			if value == 4294967295 {
				return true
			}
		case ddprofiledefinition.LicenseSentinelTimerPre1971:
			if value > 0 && value < time.Date(1971, time.January, 1, 0, 0, 0, 0, time.UTC).Unix() {
				return true
			}
		}
	}
	return false
}

func licenseScalarStructuralID(originProfileID, rowKey string) string {
	return strings.Join([]string{originProfileID, "scalar", rowKey}, "|")
}

func licenseTableStructuralID(originProfileID, tableOID, rowKey string) string {
	return strings.Join([]string{originProfileID, "table", tableOID, rowKey}, "|")
}

func mergeStringMaps(dst, src map[string]string) {
	if dst == nil || len(src) == 0 {
		return
	}
	for k, v := range src {
		if v != "" {
			dst[k] = v
		}
	}
}
