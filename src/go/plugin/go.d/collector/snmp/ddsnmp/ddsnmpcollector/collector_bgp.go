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
	bgpRowsPartialErrorLogKey = "snmp-bgp-rows-partial-error:"
	bgpRowsFailedLogKey       = "snmp-bgp-rows-failed:"
	bgpRowsErrorLogEvery      = time.Hour
)

type bgpValueContext struct {
	pdus          map[string]gosnmp.SnmpPDU
	rowIndex      string
	rowPDUs       map[string]gosnmp.SnmpPDU
	tableName     string
	crossTableCtx *crossTableContext
	oidCache      map[string]string
}

func (ctx bgpValueContext) lookupPDU(oid string) (gosnmp.SnmpPDU, bool) {
	oid = trimOID(oid)
	if ctx.rowPDUs != nil {
		pdu, ok := ctx.rowPDUs[oid]
		return pdu, ok
	}
	pdu, ok := ctx.pdus[oid]
	return pdu, ok
}

func (c *Collector) collectBGPRows(prof *ddsnmp.Profile, stats *ddsnmp.CollectionStats) ([]ddsnmp.BGPRow, error) {
	if prof.Definition == nil || len(prof.Definition.BGP) == 0 {
		return nil, nil
	}

	var rows []ddsnmp.BGPRow
	var errs []error

	scalarRows, err := c.collectScalarBGPRows(prof.Definition.BGP, stats)
	if err != nil {
		errs = append(errs, err)
	}
	rows = append(rows, scalarRows...)

	tableRows, err := c.collectTableBGPRows(prof.Definition.BGP, stats)
	if err != nil {
		errs = append(errs, err)
	}
	rows = append(rows, tableRows...)

	if len(rows) == 0 && len(errs) > 0 {
		return nil, errors.Join(errs...)
	}
	if len(errs) > 0 {
		c.log.Limit(bgpRowsPartialErrorLogKey+prof.SourceFile, 1, bgpRowsErrorLogEvery).
			Warningf("collecting BGP rows for profile %q partially failed: %v", prof.SourceFile, errors.Join(errs...))
	}

	return rows, nil
}

func (c *Collector) collectScalarBGPRows(configs []ddprofiledefinition.BGPConfig, stats *ddsnmp.CollectionStats) ([]ddsnmp.BGPRow, error) {
	var rows []ddsnmp.BGPRow
	var errs []error

	for _, cfg := range configs {
		if cfg.Table.OID != "" {
			continue
		}

		oids, missingOIDs := c.bgpScalarOIDs(cfg)
		if len(missingOIDs) > 0 {
			c.log.Debugf("BGP scalar row %q missing OIDs: %v", bgpConfigDisplayName(cfg), missingOIDs)
			stats.Errors.MissingOIDs += int64(len(missingOIDs))
		}

		var pdus map[string]gosnmp.SnmpPDU
		var err error
		if len(oids) > 0 {
			pdus, err = c.scalarCollector.getScalarValues(oids, stats)
			if err != nil {
				stats.Errors.SNMP++
				errs = append(errs, fmt.Errorf("BGP scalar row %q: %w", bgpConfigDisplayName(cfg), err))
				continue
			}
		}

		row, ok, err := c.buildScalarBGPRow(cfg, pdus)
		if err != nil {
			stats.Errors.Processing.BGP++
			errs = append(errs, fmt.Errorf("BGP scalar row %q: %w", bgpConfigDisplayName(cfg), err))
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

func (c *Collector) collectTableBGPRows(configs []ddprofiledefinition.BGPConfig, stats *ddsnmp.CollectionStats) ([]ddsnmp.BGPRow, error) {
	var rows []ddsnmp.BGPRow
	var errs []error
	walkedData := make(map[string]map[string]gosnmp.SnmpPDU)
	tableNameToOID := bgpTableNameToOID(configs)

	for _, cfg := range configs {
		if cfg.Table.OID == "" {
			continue
		}
		metricsCfg := bgpConfigAsMetricsConfig(cfg)
		cachedRows, ok, err := c.collectTableBGPRowsFromCache(cfg, metricsCfg, stats)
		if err != nil {
			c.log.Debugf("Cached BGP collection failed for table %s: %v", cfg.Table.Name, err)
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
				errs = append(errs, fmt.Errorf("BGP table %q: %w", bgpConfigDisplayName(cfg), err))
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

		if err := c.walkBGPTableDependencies(cfg, metricsCfg, tableNameToOID, walkedData, stats); err != nil {
			errs = append(errs, fmt.Errorf("BGP table %q dependencies: %w", bgpConfigDisplayName(cfg), err))
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
			row, ok, err := c.buildTableBGPRow(cfg, rowIndex, rowPDUs, ctx)
			if err != nil {
				stats.Errors.Processing.BGP++
				errs = append(errs, fmt.Errorf("BGP table %q row %q: %w", bgpConfigDisplayName(cfg), rowIndex, err))
				continue
			}
			if ok {
				rows = append(rows, row)
			}
		}

		deps := bgpTableDependencies(cfg, metricsCfg, ctx.tableNameToOID)
		c.tableCollector.tableCache.cacheData(metricsCfg, ctx.oidCache, ctx.tagCache, deps)
	}

	if len(rows) == 0 && len(errs) > 0 {
		return nil, errors.Join(errs...)
	}
	return rows, nil
}

func (c *Collector) walkBGPTableDependencies(
	cfg ddprofiledefinition.BGPConfig,
	metricsCfg ddprofiledefinition.MetricsConfig,
	tableNameToOID map[string]string,
	walkedData map[string]map[string]gosnmp.SnmpPDU,
	stats *ddsnmp.CollectionStats,
) error {
	var errs []error
	for _, depOID := range bgpTableDependencies(cfg, metricsCfg, tableNameToOID) {
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
		walkedData[depOID] = pdus
		if len(pdus) > 0 {
			stats.SNMP.TablesWalked++
		}
	}
	return errors.Join(errs...)
}

func (c *Collector) collectTableBGPRowsFromCache(cfg ddprofiledefinition.BGPConfig, metricsCfg ddprofiledefinition.MetricsConfig, stats *ddsnmp.CollectionStats) ([]ddsnmp.BGPRow, bool, error) {
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

	var rows []ddsnmp.BGPRow
	for rowIndex, columns := range cachedOIDs {
		rowPDUs := make(map[string]gosnmp.SnmpPDU)
		for columnOID, fullOID := range columns {
			pdu, ok := pdus[trimOID(fullOID)]
			if ok {
				rowPDUs[columnOID] = pdu
			}
		}
		rowTags := maps.Clone(cachedTags[rowIndex])
		row, ok, err := c.buildTableBGPRowWithTags(cfg, rowIndex, rowPDUs, rowTags, bgpValueContext{
			rowIndex:  rowIndex,
			rowPDUs:   rowPDUs,
			tableName: cfg.Table.Name,
		})
		if err != nil {
			return nil, false, fmt.Errorf("row %q: %w", rowIndex, err)
		}
		if ok {
			rows = append(rows, row)
		}
	}
	return rows, true, nil
}

func (c *Collector) buildScalarBGPRow(cfg ddprofiledefinition.BGPConfig, pdus map[string]gosnmp.SnmpPDU) (ddsnmp.BGPRow, bool, error) {
	rowKey := scalarBGPRowKey(cfg)
	row := ddsnmp.BGPRow{
		OriginProfileID: cfg.OriginProfileID,
		Kind:            cfg.Kind,
		RowKey:          rowKey,
		StructuralID:    bgpScalarStructuralID(cfg.OriginProfileID, cfg.Kind, rowKey),
		Tags:            parseStaticTags(cfg.StaticTags),
	}

	bgpCtx := bgpValueContext{pdus: pdus}
	if err := c.populateBGPRow(&row, cfg, bgpCtx); err != nil {
		return ddsnmp.BGPRow{}, false, err
	}

	if len(cfg.MetricTags) > 0 {
		tags := make(map[string]string)
		ta := tagAdder{tags: tags}
		for _, tagCfg := range cfg.MetricTags {
			if tagCfg.Symbol.OID == "" {
				continue
			}
			if err := c.scalarCollector.tagProc.processTag(tagCfg, pdus, ta); err != nil {
				c.log.Debugf("Error processing scalar BGP tag %s: %v", metricTagDisplayName(tagCfg), err)
			}
		}
		mergeStringMaps(row.Tags, tags)
	}

	if !bgpRowHasSignals(row) {
		return ddsnmp.BGPRow{}, false, nil
	}
	if !bgpRowIdentityComplete(row) {
		return ddsnmp.BGPRow{}, false, nil
	}

	return row, true, nil
}

func (c *Collector) buildTableBGPRow(cfg ddprofiledefinition.BGPConfig, rowIndex string, rowPDUs map[string]gosnmp.SnmpPDU, ctx *tableProcessingContext) (ddsnmp.BGPRow, bool, error) {
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
		c.log.Debugf("Error processing BGP row tags for %s: %v", rowIndex, err)
	}
	maps.Copy(ctx.tagCache[rowIndex], rowData.tags)

	return c.buildTableBGPRowWithTags(cfg, rowIndex, rowPDUs, rowData.tags, bgpValueContext{
		rowIndex:      rowIndex,
		rowPDUs:       rowPDUs,
		tableName:     cfg.Table.Name,
		crossTableCtx: crossTableCtx,
		oidCache:      ctx.oidCache[rowIndex],
	})
}

func (c *Collector) buildTableBGPRowWithTags(cfg ddprofiledefinition.BGPConfig, rowIndex string, rowPDUs map[string]gosnmp.SnmpPDU, rowTags map[string]string, bgpCtx bgpValueContext) (ddsnmp.BGPRow, bool, error) {
	row := ddsnmp.BGPRow{
		OriginProfileID: cfg.OriginProfileID,
		Kind:            cfg.Kind,
		TableOID:        trimOID(cfg.Table.OID),
		Table:           cfg.Table.Name,
		RowKey:          rowIndex,
		StructuralID:    bgpTableStructuralID(cfg.OriginProfileID, cfg.Kind, cfg.ID, trimOID(cfg.Table.OID), rowIndex),
		Tags:            parseStaticTags(cfg.StaticTags),
	}
	mergeStringMaps(row.Tags, rowTags)

	if bgpCtx.rowIndex == "" {
		bgpCtx.rowIndex = rowIndex
	}
	if bgpCtx.rowPDUs == nil {
		bgpCtx.rowPDUs = rowPDUs
	}
	if bgpCtx.tableName == "" {
		bgpCtx.tableName = cfg.Table.Name
	}
	if err := c.populateBGPRow(&row, cfg, bgpCtx); err != nil {
		return ddsnmp.BGPRow{}, false, err
	}

	if !bgpRowHasSignals(row) {
		return ddsnmp.BGPRow{}, false, nil
	}
	if !bgpRowIdentityComplete(row) {
		return ddsnmp.BGPRow{}, false, nil
	}

	return row, true, nil
}

func (c *Collector) populateBGPRow(row *ddsnmp.BGPRow, cfg ddprofiledefinition.BGPConfig, ctx bgpValueContext) error {
	var err error

	if row.Identity.RoutingInstance, err = c.bgpTextValue(cfg.Identity.RoutingInstance, ctx); err != nil {
		return fmt.Errorf("identity.routing_instance: %w", err)
	}
	if row.Identity.Neighbor, err = c.bgpTextValue(cfg.Identity.Neighbor, ctx); err != nil {
		return fmt.Errorf("identity.neighbor: %w", err)
	}
	if row.Identity.RemoteAS, err = c.bgpTextValue(cfg.Identity.RemoteAS, ctx); err != nil {
		return fmt.Errorf("identity.remote_as: %w", err)
	}
	if row.Identity.AddressFamily, err = c.bgpAddressFamilyValue(cfg.Identity.AddressFamily.BGPValueConfig, ctx); err != nil {
		return fmt.Errorf("identity.address_family: %w", err)
	}
	if row.Identity.SubsequentAddressFamily, err = c.bgpSubsequentAddressFamilyValue(cfg.Identity.SubsequentAddressFamily.BGPValueConfig, ctx); err != nil {
		return fmt.Errorf("identity.subsequent_address_family: %w", err)
	}
	if row.Descriptors.LocalAddress, err = c.bgpTextValue(cfg.Descriptors.LocalAddress, ctx); err != nil {
		return fmt.Errorf("descriptors.local_address: %w", err)
	}
	if row.Descriptors.LocalAS, err = c.bgpTextValue(cfg.Descriptors.LocalAS, ctx); err != nil {
		return fmt.Errorf("descriptors.local_as: %w", err)
	}
	if row.Descriptors.LocalIdentifier, err = c.bgpTextValue(cfg.Descriptors.LocalIdentifier, ctx); err != nil {
		return fmt.Errorf("descriptors.local_identifier: %w", err)
	}
	if row.Descriptors.PeerIdentifier, err = c.bgpTextValue(cfg.Descriptors.PeerIdentifier, ctx); err != nil {
		return fmt.Errorf("descriptors.peer_identifier: %w", err)
	}
	if row.Descriptors.PeerType, err = c.bgpTextValue(cfg.Descriptors.PeerType, ctx); err != nil {
		return fmt.Errorf("descriptors.peer_type: %w", err)
	}
	if row.Descriptors.BGPVersion, err = c.bgpTextValue(cfg.Descriptors.BGPVersion, ctx); err != nil {
		return fmt.Errorf("descriptors.bgp_version: %w", err)
	}
	if row.Descriptors.Description, err = c.bgpTextValue(cfg.Descriptors.Description, ctx); err != nil {
		return fmt.Errorf("descriptors.description: %w", err)
	}

	if err := c.populateBGPStateValue(&row.State, cfg.State, ctx); err != nil {
		return fmt.Errorf("state: %w", err)
	}
	if err := c.populateBGPStateValue(&row.Previous, cfg.Previous, ctx); err != nil {
		return fmt.Errorf("previous_state: %w", err)
	}
	if err := c.populateBGPBool(&row.Admin.Enabled, cfg.Admin.Enabled, ctx); err != nil {
		return fmt.Errorf("admin.enabled: %w", err)
	}
	if err := c.populateBGPConnection(&row.Connection, cfg.Connection, ctx); err != nil {
		return fmt.Errorf("connection: %w", err)
	}
	if err := c.populateBGPTraffic(&row.Traffic, cfg.Traffic, ctx); err != nil {
		return fmt.Errorf("traffic: %w", err)
	}
	if err := c.populateBGPTransitions(&row.Transitions, cfg.Transitions, ctx); err != nil {
		return fmt.Errorf("transitions: %w", err)
	}
	if err := c.populateBGPTimers(&row.Timers, cfg.Timers, ctx); err != nil {
		return fmt.Errorf("timers: %w", err)
	}
	if err := c.populateBGPLastError(&row.LastError, cfg.LastError, ctx); err != nil {
		return fmt.Errorf("last_error: %w", err)
	}
	if err := c.populateBGPLastNotifications(&row.LastNotify, cfg.LastNotify, ctx); err != nil {
		return fmt.Errorf("last_notifications: %w", err)
	}
	if err := c.populateBGPReasons(&row.Reasons, cfg.Reasons, ctx); err != nil {
		return fmt.Errorf("reasons: %w", err)
	}
	if err := c.populateBGPText(&row.Restart.State, cfg.Restart.State, ctx); err != nil {
		return fmt.Errorf("graceful_restart.state: %w", err)
	}
	if err := c.populateBGPRoutes(&row.Routes, cfg.Routes, ctx); err != nil {
		return fmt.Errorf("routes: %w", err)
	}
	if err := c.populateBGPRouteLimits(&row.RouteLimits, cfg.RouteLimits, ctx); err != nil {
		return fmt.Errorf("route_limits: %w", err)
	}
	if err := c.populateBGPDeviceCounts(&row.Device, cfg.Device, ctx); err != nil {
		return fmt.Errorf("device_counts: %w", err)
	}

	return nil
}

func (c *Collector) populateBGPStateValue(dst *ddsnmp.BGPState, cfg ddprofiledefinition.BGPStateConfig, ctx bgpValueContext) error {
	if !cfg.IsSet() {
		return nil
	}

	raw, sourceOID, ok, err := c.bgpRawTextValue(cfg.BGPValueConfig, ctx)
	if err != nil {
		return err
	}
	if !ok {
		return nil
	}
	state := raw
	if mapped, ok := bgpValueSymbol(cfg.BGPValueConfig).Mapping.Lookup(raw); ok {
		state = mapped
	}

	*dst = ddsnmp.BGPState{
		Has:       true,
		State:     ddprofiledefinition.BGPPeerState(state),
		Raw:       raw,
		SourceOID: sourceOID,
	}
	return nil
}

func (c *Collector) populateBGPInt64(dst *ddsnmp.BGPInt64, cfg ddprofiledefinition.BGPValueConfig, ctx bgpValueContext) error {
	if !cfg.IsSet() {
		return nil
	}
	value, sourceOID, ok, err := c.bgpNumericValue(cfg, ctx)
	if err != nil {
		return err
	}
	if !ok {
		return nil
	}
	raw, _, _, _ := c.bgpRawTextValue(cfg, ctx)
	*dst = ddsnmp.BGPInt64{
		Has:       true,
		Value:     value,
		Raw:       raw,
		SourceOID: sourceOID,
	}
	return nil
}

func (c *Collector) populateBGPText(dst *ddsnmp.BGPText, cfg ddprofiledefinition.BGPValueConfig, ctx bgpValueContext) error {
	if !cfg.IsSet() {
		return nil
	}
	raw, sourceOID, ok, err := c.bgpRawTextValue(cfg, ctx)
	if err != nil {
		return err
	}
	if !ok {
		return nil
	}
	value := raw
	if mapped, ok := bgpValueSymbol(cfg).Mapping.Lookup(raw); ok {
		value = mapped
	}
	*dst = ddsnmp.BGPText{
		Has:       true,
		Value:     value,
		Raw:       raw,
		SourceOID: sourceOID,
	}
	return nil
}

func (c *Collector) populateBGPBool(dst *ddsnmp.BGPBool, cfg ddprofiledefinition.BGPValueConfig, ctx bgpValueContext) error {
	if !cfg.IsSet() {
		return nil
	}
	raw, sourceOID, ok, err := c.bgpRawTextValue(cfg, ctx)
	if err != nil {
		return err
	}
	if !ok {
		return nil
	}
	text := strings.ToLower(strings.TrimSpace(raw))
	if mapped, ok := bgpValueSymbol(cfg).Mapping.Lookup(raw); ok {
		text = strings.ToLower(strings.TrimSpace(mapped))
	}
	value, err := bgpParseBool(text)
	if err != nil {
		return err
	}
	*dst = ddsnmp.BGPBool{
		Has:       true,
		Value:     value,
		Raw:       raw,
		SourceOID: sourceOID,
	}
	return nil
}

func (c *Collector) populateBGPConnection(dst *ddsnmp.BGPConnection, cfg ddprofiledefinition.BGPConnectionConfig, ctx bgpValueContext) error {
	if err := c.populateBGPInt64(&dst.EstablishedUptime, cfg.EstablishedUptime, ctx); err != nil {
		return fmt.Errorf("established_uptime: %w", err)
	}
	if err := c.populateBGPInt64(&dst.LastReceivedUpdateAge, cfg.LastReceivedUpdateAge, ctx); err != nil {
		return fmt.Errorf("last_received_update_age: %w", err)
	}
	return nil
}

func (c *Collector) populateBGPDirectional(dst *ddsnmp.BGPDirectional, cfg ddprofiledefinition.BGPDirectionalConfig, ctx bgpValueContext) error {
	if err := c.populateBGPInt64(&dst.Received, cfg.Received, ctx); err != nil {
		return fmt.Errorf("received: %w", err)
	}
	if err := c.populateBGPInt64(&dst.Sent, cfg.Sent, ctx); err != nil {
		return fmt.Errorf("sent: %w", err)
	}
	return nil
}

func (c *Collector) populateBGPTraffic(dst *ddsnmp.BGPTraffic, cfg ddprofiledefinition.BGPTrafficConfig, ctx bgpValueContext) error {
	if err := c.populateBGPDirectional(&dst.Messages, cfg.Messages, ctx); err != nil {
		return fmt.Errorf("messages: %w", err)
	}
	if err := c.populateBGPDirectional(&dst.Updates, cfg.Updates, ctx); err != nil {
		return fmt.Errorf("updates: %w", err)
	}
	if err := c.populateBGPDirectional(&dst.Notifications, cfg.Notifications, ctx); err != nil {
		return fmt.Errorf("notifications: %w", err)
	}
	if err := c.populateBGPDirectional(&dst.RouteRefreshes, cfg.RouteRefreshes, ctx); err != nil {
		return fmt.Errorf("route_refreshes: %w", err)
	}
	if err := c.populateBGPDirectional(&dst.Opens, cfg.Opens, ctx); err != nil {
		return fmt.Errorf("opens: %w", err)
	}
	if err := c.populateBGPDirectional(&dst.Keepalives, cfg.Keepalives, ctx); err != nil {
		return fmt.Errorf("keepalives: %w", err)
	}
	return nil
}

func (c *Collector) populateBGPTransitions(dst *ddsnmp.BGPTransitions, cfg ddprofiledefinition.BGPTransitionsConfig, ctx bgpValueContext) error {
	if err := c.populateBGPInt64(&dst.Established, cfg.Established, ctx); err != nil {
		return fmt.Errorf("established: %w", err)
	}
	if err := c.populateBGPInt64(&dst.Down, cfg.Down, ctx); err != nil {
		return fmt.Errorf("down: %w", err)
	}
	if err := c.populateBGPInt64(&dst.Up, cfg.Up, ctx); err != nil {
		return fmt.Errorf("up: %w", err)
	}
	if err := c.populateBGPInt64(&dst.Flaps, cfg.Flaps, ctx); err != nil {
		return fmt.Errorf("flaps: %w", err)
	}
	return nil
}

func (c *Collector) populateBGPTimers(dst *ddsnmp.BGPTimers, cfg ddprofiledefinition.BGPTimersConfig, ctx bgpValueContext) error {
	if err := c.populateBGPTimerPair(&dst.Negotiated, cfg.Negotiated, ctx); err != nil {
		return fmt.Errorf("negotiated: %w", err)
	}
	if err := c.populateBGPTimerPair(&dst.Configured, cfg.Configured, ctx); err != nil {
		return fmt.Errorf("configured: %w", err)
	}
	return nil
}

func (c *Collector) populateBGPTimerPair(dst *ddsnmp.BGPTimerPair, cfg ddprofiledefinition.BGPTimerPairConfig, ctx bgpValueContext) error {
	if err := c.populateBGPInt64(&dst.ConnectRetry, cfg.ConnectRetry, ctx); err != nil {
		return fmt.Errorf("connect_retry: %w", err)
	}
	if err := c.populateBGPInt64(&dst.HoldTime, cfg.HoldTime, ctx); err != nil {
		return fmt.Errorf("hold_time: %w", err)
	}
	if err := c.populateBGPInt64(&dst.KeepaliveTime, cfg.KeepaliveTime, ctx); err != nil {
		return fmt.Errorf("keepalive_time: %w", err)
	}
	if err := c.populateBGPInt64(&dst.MinASOriginationInterval, cfg.MinASOriginationInterval, ctx); err != nil {
		return fmt.Errorf("min_as_origination_interval: %w", err)
	}
	if err := c.populateBGPInt64(&dst.MinRouteAdvertisementInterval, cfg.MinRouteAdvertisementInterval, ctx); err != nil {
		return fmt.Errorf("min_route_advertisement_interval: %w", err)
	}
	return nil
}

func (c *Collector) populateBGPLastError(dst *ddsnmp.BGPLastError, cfg ddprofiledefinition.BGPLastErrorConfig, ctx bgpValueContext) error {
	if err := c.populateBGPInt64(&dst.Code, cfg.Code, ctx); err != nil {
		return fmt.Errorf("code: %w", err)
	}
	if err := c.populateBGPInt64(&dst.Subcode, cfg.Subcode, ctx); err != nil {
		return fmt.Errorf("subcode: %w", err)
	}
	return nil
}

func (c *Collector) populateBGPLastNotifications(dst *ddsnmp.BGPLastNotifications, cfg ddprofiledefinition.BGPLastNotifyConfig, ctx bgpValueContext) error {
	if err := c.populateBGPLastNotification(&dst.Received, cfg.Received, ctx); err != nil {
		return fmt.Errorf("received: %w", err)
	}
	if err := c.populateBGPLastNotification(&dst.Sent, cfg.Sent, ctx); err != nil {
		return fmt.Errorf("sent: %w", err)
	}
	return nil
}

func (c *Collector) populateBGPLastNotification(dst *ddsnmp.BGPLastNotification, cfg ddprofiledefinition.BGPLastNotificationConfig, ctx bgpValueContext) error {
	if err := c.populateBGPInt64(&dst.Code, cfg.Code, ctx); err != nil {
		return fmt.Errorf("code: %w", err)
	}
	if err := c.populateBGPInt64(&dst.Subcode, cfg.Subcode, ctx); err != nil {
		return fmt.Errorf("subcode: %w", err)
	}
	if err := c.populateBGPText(&dst.Reason, cfg.Reason, ctx); err != nil {
		return fmt.Errorf("reason: %w", err)
	}
	return nil
}

func (c *Collector) populateBGPReasons(dst *ddsnmp.BGPReasons, cfg ddprofiledefinition.BGPReasonsConfig, ctx bgpValueContext) error {
	if err := c.populateBGPText(&dst.LastDown, cfg.LastDown, ctx); err != nil {
		return fmt.Errorf("last_down: %w", err)
	}
	if err := c.populateBGPText(&dst.Unavailability, cfg.Unavailability, ctx); err != nil {
		return fmt.Errorf("unavailability: %w", err)
	}
	return nil
}

func (c *Collector) populateBGPRoutes(dst *ddsnmp.BGPRoutes, cfg ddprofiledefinition.BGPRoutesConfig, ctx bgpValueContext) error {
	if err := c.populateBGPRouteCounters(&dst.Current, cfg.Current, ctx); err != nil {
		return fmt.Errorf("current: %w", err)
	}
	if err := c.populateBGPRouteCounters(&dst.Total, cfg.Total, ctx); err != nil {
		return fmt.Errorf("total: %w", err)
	}
	return nil
}

func (c *Collector) populateBGPRouteCounters(dst *ddsnmp.BGPRouteCounters, cfg ddprofiledefinition.BGPRouteCountersConfig, ctx bgpValueContext) error {
	if err := c.populateBGPInt64(&dst.Received, cfg.Received, ctx); err != nil {
		return fmt.Errorf("received: %w", err)
	}
	if err := c.populateBGPInt64(&dst.Accepted, cfg.Accepted, ctx); err != nil {
		return fmt.Errorf("accepted: %w", err)
	}
	if err := c.populateBGPInt64(&dst.Rejected, cfg.Rejected, ctx); err != nil {
		return fmt.Errorf("rejected: %w", err)
	}
	if err := c.populateBGPInt64(&dst.Active, cfg.Active, ctx); err != nil {
		return fmt.Errorf("active: %w", err)
	}
	if err := c.populateBGPInt64(&dst.Advertised, cfg.Advertised, ctx); err != nil {
		return fmt.Errorf("advertised: %w", err)
	}
	if err := c.populateBGPInt64(&dst.Suppressed, cfg.Suppressed, ctx); err != nil {
		return fmt.Errorf("suppressed: %w", err)
	}
	if err := c.populateBGPInt64(&dst.Withdrawn, cfg.Withdrawn, ctx); err != nil {
		return fmt.Errorf("withdrawn: %w", err)
	}
	return nil
}

func (c *Collector) populateBGPRouteLimits(dst *ddsnmp.BGPRouteLimits, cfg ddprofiledefinition.BGPRouteLimitsConfig, ctx bgpValueContext) error {
	if err := c.populateBGPInt64(&dst.Limit, cfg.Limit, ctx); err != nil {
		return fmt.Errorf("limit: %w", err)
	}
	if err := c.populateBGPInt64(&dst.Threshold, cfg.Threshold, ctx); err != nil {
		return fmt.Errorf("threshold: %w", err)
	}
	if err := c.populateBGPInt64(&dst.ClearThreshold, cfg.ClearThreshold, ctx); err != nil {
		return fmt.Errorf("clear_threshold: %w", err)
	}
	return nil
}

func (c *Collector) populateBGPDeviceCounts(dst *ddsnmp.BGPDeviceCounts, cfg ddprofiledefinition.BGPDeviceCountsConfig, ctx bgpValueContext) error {
	if err := c.populateBGPInt64(&dst.Peers, cfg.Peers, ctx); err != nil {
		return fmt.Errorf("peers: %w", err)
	}
	if err := c.populateBGPInt64(&dst.InternalPeers, cfg.InternalPeers, ctx); err != nil {
		return fmt.Errorf("ibgp_peers: %w", err)
	}
	if err := c.populateBGPInt64(&dst.ExternalPeers, cfg.ExternalPeers, ctx); err != nil {
		return fmt.Errorf("ebgp_peers: %w", err)
	}
	counts, err := c.bgpPeerStateCounts(cfg.States, ctx)
	if err != nil {
		return fmt.Errorf("states: %w", err)
	}
	if len(counts) > 0 {
		dst.ByStateHas = true
		dst.ByState = counts
	}
	return nil
}

func (c *Collector) bgpPeerStateCounts(cfg ddprofiledefinition.BGPPeerStatesConfig, ctx bgpValueContext) (map[ddprofiledefinition.BGPPeerState]int64, error) {
	counts := make(map[ddprofiledefinition.BGPPeerState]int64)
	add := func(state ddprofiledefinition.BGPPeerState, valueCfg ddprofiledefinition.BGPValueConfig) error {
		var value ddsnmp.BGPInt64
		if err := c.populateBGPInt64(&value, valueCfg, ctx); err != nil {
			return err
		}
		if value.Has {
			counts[state] = value.Value
		}
		return nil
	}
	if err := add(ddprofiledefinition.BGPPeerStateIdle, cfg.Idle); err != nil {
		return nil, fmt.Errorf("idle: %w", err)
	}
	if err := add(ddprofiledefinition.BGPPeerStateConnect, cfg.Connect); err != nil {
		return nil, fmt.Errorf("connect: %w", err)
	}
	if err := add(ddprofiledefinition.BGPPeerStateActive, cfg.Active); err != nil {
		return nil, fmt.Errorf("active: %w", err)
	}
	if err := add(ddprofiledefinition.BGPPeerStateOpenSent, cfg.OpenSent); err != nil {
		return nil, fmt.Errorf("opensent: %w", err)
	}
	if err := add(ddprofiledefinition.BGPPeerStateOpenConfirm, cfg.OpenConfirm); err != nil {
		return nil, fmt.Errorf("openconfirm: %w", err)
	}
	if err := add(ddprofiledefinition.BGPPeerStateEstablished, cfg.Established); err != nil {
		return nil, fmt.Errorf("established: %w", err)
	}
	return counts, nil
}

func bgpParseBool(text string) (bool, error) {
	switch text {
	case "1", "true", "yes", "enabled", "enable", "start", "running", "up":
		return true, nil
	case "0", "false", "no", "disabled", "disable", "stop", "halted", "down":
		return false, nil
	default:
		value, err := strconv.ParseInt(text, 10, 64)
		if err != nil {
			return false, fmt.Errorf("cannot parse %q as boolean", text)
		}
		return value != 0, nil
	}
}

func (c *Collector) bgpTextValue(cfg ddprofiledefinition.BGPValueConfig, ctx bgpValueContext) (string, error) {
	value, _, ok, err := c.bgpRawTextValue(cfg, ctx)
	if err != nil || !ok {
		return "", err
	}
	sym := bgpValueSymbol(cfg)
	if mapped, ok := sym.Mapping.Lookup(value); ok {
		value = mapped
	}
	return value, nil
}

func (c *Collector) bgpAddressFamilyValue(cfg ddprofiledefinition.BGPValueConfig, ctx bgpValueContext) (ddprofiledefinition.BGPAddressFamily, error) {
	value, err := c.bgpTextValue(cfg, ctx)
	if err != nil || value == "" {
		return "", err
	}
	return ddprofiledefinition.BGPAddressFamily(value), nil
}

func (c *Collector) bgpSubsequentAddressFamilyValue(cfg ddprofiledefinition.BGPValueConfig, ctx bgpValueContext) (ddprofiledefinition.BGPSubsequentAddressFamily, error) {
	value, err := c.bgpTextValue(cfg, ctx)
	if err != nil || value == "" {
		return "", err
	}
	return ddprofiledefinition.BGPSubsequentAddressFamily(value), nil
}

func (c *Collector) bgpRawTextValue(cfg ddprofiledefinition.BGPValueConfig, ctx bgpValueContext) (value string, sourceOID string, ok bool, err error) {
	if !cfg.IsSet() {
		return "", "", false, nil
	}
	if cfg.Value != "" {
		return cfg.Value, "", true, nil
	}
	sym := bgpValueSymbol(cfg)
	if sym.OID == "" && (cfg.Index != 0 || cfg.IndexFromEnd != 0 || len(cfg.IndexTransform) > 0) {
		value, err := c.bgpIndexValue(cfg, ctx.rowIndex)
		return value, "", err == nil, err
	}

	if sym.OID == "" {
		return "", "", false, nil
	}
	pdu, sourceOID, ok, err := c.lookupBGPValuePDU(cfg, sym, ctx)
	if err != nil || !ok {
		return "", "", false, err
	}

	value, err = convPduToStringf(pdu, sym.Format)
	if err != nil {
		if errors.Is(err, errNoTextDateValue) {
			return "", "", false, nil
		}
		return "", "", false, err
	}

	if sym.ExtractValueCompiled != nil {
		sm := sym.ExtractValueCompiled.FindStringSubmatch(value)
		if len(sm) < 2 {
			return "", "", false, fmt.Errorf("extract_value did not match value %q", value)
		}
		value = sm[1]
	}
	if sym.MatchPatternCompiled != nil {
		sm := sym.MatchPatternCompiled.FindStringSubmatch(value)
		if len(sm) == 0 {
			return "", "", false, fmt.Errorf("match_pattern %q did not match value %q", sym.MatchPattern, value)
		}
		value = replaceSubmatches(sym.MatchValue, sm)
	}

	return value, sourceOID, true, nil
}

func (c *Collector) bgpNumericValue(cfg ddprofiledefinition.BGPValueConfig, ctx bgpValueContext) (value int64, sourceOID string, ok bool, err error) {
	if !cfg.IsSet() {
		return 0, "", false, nil
	}
	sym := bgpValueSymbol(cfg)
	if cfg.Value != "" || (sym.OID == "" && (cfg.Index != 0 || cfg.IndexFromEnd != 0 || len(cfg.IndexTransform) > 0)) {
		text, _, ok, err := c.bgpRawTextValue(cfg, ctx)
		if err != nil || !ok || text == "" {
			return 0, "", false, err
		}
		value, err := strconv.ParseInt(strings.TrimSpace(text), 10, 64)
		if err != nil {
			return 0, "", false, err
		}
		return value, "", true, nil
	}

	if sym.OID == "" {
		return 0, "", false, nil
	}
	pdu, sourceOID, ok, err := c.lookupBGPValuePDU(cfg, sym, ctx)
	if err != nil || !ok {
		return 0, "", false, err
	}

	value, err = c.scalarCollector.valProc.processValue(sym, pdu)
	if err != nil {
		if errors.Is(err, errNoTextDateValue) {
			return 0, "", false, nil
		}
		return 0, "", false, err
	}

	return value, sourceOID, true, nil
}

func (c *Collector) lookupBGPValuePDU(cfg ddprofiledefinition.BGPValueConfig, sym ddprofiledefinition.SymbolConfig, ctx bgpValueContext) (gosnmp.SnmpPDU, string, bool, error) {
	sourceOID := trimOID(sym.OID)
	if cfg.Table == "" || cfg.Table == ctx.tableName {
		pdu, ok := ctx.lookupPDU(sym.OID)
		return pdu, sourceOID, ok, nil
	}
	if ctx.crossTableCtx == nil {
		pdu, ok := ctx.lookupPDU(sym.OID)
		return pdu, sourceOID, ok, nil
	}

	refTableOID, err := c.tableCollector.rowProcessor.crossTableResolver.findReferencedTableOID(cfg.Table, ctx.crossTableCtx.tableNameToOID)
	if err != nil {
		return gosnmp.SnmpPDU{}, "", false, err
	}
	refTablePDUs, err := c.tableCollector.rowProcessor.crossTableResolver.getReferencedTableData(cfg.Table, refTableOID, ctx.crossTableCtx.walkedData)
	if err != nil {
		return gosnmp.SnmpPDU{}, "", false, err
	}
	lookupIndex, err := c.tableCollector.rowProcessor.crossTableResolver.transformIndex(ctx.rowIndex, cfg.IndexTransform)
	if err != nil {
		return gosnmp.SnmpPDU{}, "", false, err
	}
	tagCfg := ddprofiledefinition.MetricTagConfig{
		Table:        cfg.Table,
		Symbol:       ddprofiledefinition.SymbolConfigCompat(sym),
		LookupSymbol: cfg.LookupSymbol,
	}
	if c.tableCollector.rowProcessor.crossTableResolver.requiresLookupByValue(tagCfg) {
		lookupIndex, err = c.tableCollector.rowProcessor.crossTableResolver.resolveLookupIndexByValue(tagCfg, lookupIndex, refTableOID, refTablePDUs, ctx.crossTableCtx)
		if err != nil {
			return gosnmp.SnmpPDU{}, "", false, err
		}
	}

	fullOID := sourceOID + "." + lookupIndex
	pdu, ok := refTablePDUs[fullOID]
	if !ok {
		return gosnmp.SnmpPDU{}, sourceOID, false, nil
	}
	if ctx.oidCache != nil {
		ctx.oidCache[sourceOID] = fullOID
	}
	return pdu, sourceOID, true, nil
}

func (c *Collector) bgpIndexValue(cfg ddprofiledefinition.BGPValueConfig, rowIndex string) (string, error) {
	if rowIndex == "" {
		return "", nil
	}
	sym := bgpValueSymbol(cfg)
	if cfg.IndexFromEnd != 0 {
		rawValue, ok := c.tableCollector.rowProcessor.extractIndexPositionFromEnd(rowIndex, cfg.IndexFromEnd)
		if !ok {
			return "", fmt.Errorf("position from end %d not found", cfg.IndexFromEnd)
		}
		tagCfg := ddprofiledefinition.MetricTagConfig{
			Symbol:  ddprofiledefinition.SymbolConfigCompat(sym),
			Mapping: sym.Mapping,
		}
		return processRawIndexTagValue(tagCfg, rawValue)
	}
	tagCfg := ddprofiledefinition.MetricTagConfig{
		Index:          cfg.Index,
		IndexTransform: cfg.IndexTransform,
		Symbol:         ddprofiledefinition.SymbolConfigCompat(sym),
		Mapping:        sym.Mapping,
	}
	_, value, err := c.tableCollector.rowProcessor.processIndexTag(tagCfg, rowIndex)
	return value, err
}

func bgpConfigAsMetricsConfig(cfg ddprofiledefinition.BGPConfig) ddprofiledefinition.MetricsConfig {
	columns := make(map[string]ddprofiledefinition.SymbolConfig)
	addColumn := func(valueCfg ddprofiledefinition.BGPValueConfig) {
		sym := bgpValueSymbol(valueCfg)
		if sym.OID != "" {
			columns[trimOID(sym.OID)] = sym
		}
	}

	forEachBGPValue(cfg, addColumn)
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
		MIB:        bgpMetricsConfigMIB(cfg.MIB),
		Table:      cfg.Table,
		Symbols:    symbols,
		StaticTags: cfg.StaticTags,
		MetricTags: cfg.MetricTags,
	}
}

func bgpMetricsConfigMIB(mib string) string {
	if mib == "" {
		return "bgp"
	}
	return "bgp:" + mib
}

func bgpTableNameToOID(configs []ddprofiledefinition.BGPConfig) map[string]string {
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
		metricsCfg := bgpConfigAsMetricsConfig(cfg)
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
		forEachBGPValue(cfg, func(valueCfg ddprofiledefinition.BGPValueConfig) {
			if valueCfg.Table == "" || valueCfg.Table == cfg.Table.Name {
				return
			}
			if _, ok := tableNameToOID[valueCfg.Table]; ok {
				return
			}
			if oid := bgpValueSymbol(valueCfg).OID; oid != "" {
				crossTableOIDs[valueCfg.Table] = append(crossTableOIDs[valueCfg.Table], oid)
			}
			if oid := ddprofiledefinition.SymbolConfig(valueCfg.LookupSymbol).OID; oid != "" {
				crossTableOIDs[valueCfg.Table] = append(crossTableOIDs[valueCfg.Table], oid)
			}
		})
	}

	for tableName, oids := range crossTableOIDs {
		if oid := longestCommonOIDPrefix(oids); oid != "" {
			tableNameToOID[tableName] = oid
		}
	}

	return tableNameToOID
}

func bgpTableDependencies(cfg ddprofiledefinition.BGPConfig, metricsCfg ddprofiledefinition.MetricsConfig, tableNameToOID map[string]string) []string {
	deps := make(map[string]bool)
	for _, dep := range extractTableDependencies(metricsCfg, tableNameToOID) {
		deps[trimOID(dep)] = true
	}
	forEachBGPValue(cfg, func(valueCfg ddprofiledefinition.BGPValueConfig) {
		if valueCfg.Table == "" || valueCfg.Table == cfg.Table.Name {
			return
		}
		if tableOID, ok := tableNameToOID[valueCfg.Table]; ok {
			deps[trimOID(tableOID)] = true
		}
	})

	result := make([]string, 0, len(deps))
	for oid := range deps {
		result = append(result, oid)
	}
	slices.Sort(result)
	return result
}

func (c *Collector) bgpScalarOIDs(cfg ddprofiledefinition.BGPConfig) ([]string, []string) {
	oids := make(map[string]struct{})
	var missingOIDs []string
	addOID := func(valueCfg ddprofiledefinition.BGPValueConfig) {
		sym := bgpValueSymbol(valueCfg)
		if sym.OID != "" {
			oid := trimOID(sym.OID)
			if c.missingOIDs[oid] {
				missingOIDs = append(missingOIDs, sym.OID)
				return
			}
			oids[oid] = struct{}{}
		}
	}

	forEachBGPValue(cfg, addOID)
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

func forEachBGPValue(cfg ddprofiledefinition.BGPConfig, fn func(ddprofiledefinition.BGPValueConfig)) {
	fn(cfg.Identity.RoutingInstance)
	fn(cfg.Identity.Neighbor)
	fn(cfg.Identity.RemoteAS)
	fn(cfg.Identity.AddressFamily.BGPValueConfig)
	fn(cfg.Identity.SubsequentAddressFamily.BGPValueConfig)
	fn(cfg.Descriptors.LocalAddress)
	fn(cfg.Descriptors.LocalAS)
	fn(cfg.Descriptors.LocalIdentifier)
	fn(cfg.Descriptors.PeerIdentifier)
	fn(cfg.Descriptors.PeerType)
	fn(cfg.Descriptors.BGPVersion)
	fn(cfg.Descriptors.Description)
	ddprofiledefinition.ForEachBGPSignalValue(cfg, func(_ string, value ddprofiledefinition.BGPValueConfig) {
		fn(value)
	})
}

func bgpValueSymbol(cfg ddprofiledefinition.BGPValueConfig) ddprofiledefinition.SymbolConfig {
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
		sym.Name = "bgp:" + trimOID(sym.OID)
	}
	if sym.Format == "" {
		sym.Format = cfg.Format
	}
	if !sym.Mapping.HasItems() && cfg.Mapping.HasItems() {
		sym.Mapping = cfg.Mapping
	}
	return sym
}

func scalarBGPRowKey(cfg ddprofiledefinition.BGPConfig) string {
	if cfg.ID != "" {
		return cfg.ID
	}
	rowKey := ""
	forEachBGPValue(cfg, func(valueCfg ddprofiledefinition.BGPValueConfig) {
		if rowKey != "" {
			return
		}
		if oid := bgpValueSymbol(valueCfg).OID; oid != "" {
			rowKey = trimOID(oid)
		}
	})
	if rowKey != "" {
		return rowKey
	}
	return cfg.OriginProfileID
}

func bgpConfigDisplayName(cfg ddprofiledefinition.BGPConfig) string {
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

func bgpRowHasSignals(row ddsnmp.BGPRow) bool {
	return row.State.Has ||
		row.Previous.Has ||
		row.Admin.Enabled.Has ||
		bgpConnectionHasSignals(row.Connection) ||
		bgpTrafficHasSignals(row.Traffic) ||
		bgpTransitionsHasSignals(row.Transitions) ||
		bgpTimersHasSignals(row.Timers) ||
		row.LastError.Code.Has ||
		row.LastError.Subcode.Has ||
		bgpLastNotificationsHasSignals(row.LastNotify) ||
		row.Reasons.LastDown.Has ||
		row.Reasons.Unavailability.Has ||
		row.Restart.State.Has ||
		bgpRoutesHasSignals(row.Routes) ||
		row.RouteLimits.Limit.Has ||
		row.RouteLimits.Threshold.Has ||
		row.RouteLimits.ClearThreshold.Has ||
		row.Device.Peers.Has ||
		row.Device.InternalPeers.Has ||
		row.Device.ExternalPeers.Has ||
		row.Device.ByStateHas
}

func bgpRowIdentityComplete(row ddsnmp.BGPRow) bool {
	switch row.Kind {
	case ddprofiledefinition.BGPRowKindPeer:
		return row.Identity.Neighbor != "" && row.Identity.RemoteAS != ""
	case ddprofiledefinition.BGPRowKindPeerFamily:
		return row.Identity.Neighbor != "" &&
			row.Identity.RemoteAS != "" &&
			row.Identity.AddressFamily != "" &&
			row.Identity.SubsequentAddressFamily != ""
	default:
		return true
	}
}

func bgpConnectionHasSignals(v ddsnmp.BGPConnection) bool {
	return v.EstablishedUptime.Has || v.LastReceivedUpdateAge.Has
}

func bgpDirectionalHasSignals(v ddsnmp.BGPDirectional) bool {
	return v.Received.Has || v.Sent.Has
}

func bgpTrafficHasSignals(v ddsnmp.BGPTraffic) bool {
	return bgpDirectionalHasSignals(v.Messages) ||
		bgpDirectionalHasSignals(v.Updates) ||
		bgpDirectionalHasSignals(v.Notifications) ||
		bgpDirectionalHasSignals(v.RouteRefreshes) ||
		bgpDirectionalHasSignals(v.Opens) ||
		bgpDirectionalHasSignals(v.Keepalives)
}

func bgpTransitionsHasSignals(v ddsnmp.BGPTransitions) bool {
	return v.Established.Has || v.Down.Has || v.Up.Has || v.Flaps.Has
}

func bgpTimerPairHasSignals(v ddsnmp.BGPTimerPair) bool {
	return v.ConnectRetry.Has ||
		v.HoldTime.Has ||
		v.KeepaliveTime.Has ||
		v.MinASOriginationInterval.Has ||
		v.MinRouteAdvertisementInterval.Has
}

func bgpTimersHasSignals(v ddsnmp.BGPTimers) bool {
	return bgpTimerPairHasSignals(v.Negotiated) || bgpTimerPairHasSignals(v.Configured)
}

func bgpLastNotificationHasSignals(v ddsnmp.BGPLastNotification) bool {
	return v.Code.Has || v.Subcode.Has || v.Reason.Has
}

func bgpLastNotificationsHasSignals(v ddsnmp.BGPLastNotifications) bool {
	return bgpLastNotificationHasSignals(v.Received) || bgpLastNotificationHasSignals(v.Sent)
}

func bgpRouteCountersHasSignals(v ddsnmp.BGPRouteCounters) bool {
	return v.Received.Has ||
		v.Accepted.Has ||
		v.Rejected.Has ||
		v.Active.Has ||
		v.Advertised.Has ||
		v.Suppressed.Has ||
		v.Withdrawn.Has
}

func bgpRoutesHasSignals(v ddsnmp.BGPRoutes) bool {
	return bgpRouteCountersHasSignals(v.Current) || bgpRouteCountersHasSignals(v.Total)
}

func bgpScalarStructuralID(originProfileID string, kind ddprofiledefinition.BGPRowKind, rowKey string) string {
	return lengthPrefixedKey(originProfileID, string(kind), "scalar", rowKey)
}

func bgpTableStructuralID(originProfileID string, kind ddprofiledefinition.BGPRowKind, configID, tableOID, rowKey string) string {
	return lengthPrefixedKey(originProfileID, string(kind), "table", configID, tableOID, rowKey)
}

func lengthPrefixedKey(parts ...string) string {
	var sb strings.Builder
	for _, part := range parts {
		if sb.Len() > 0 {
			sb.WriteByte('|')
		}
		sb.WriteString(strconv.Itoa(len(part)))
		sb.WriteByte(':')
		sb.WriteString(part)
	}
	return sb.String()
}
