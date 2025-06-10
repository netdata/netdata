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

func (c *Collector) collectTableMetrics(prof *ddsnmp.Profile) ([]Metric, error) {
	var metrics []Metric
	var errs []error
	var missingOIDs []string

	doneOids := make(map[string]bool)

	for _, cfg := range prof.Definition.Metrics {
		if cfg.IsScalar() || cfg.Table.OID == "" || doneOids[cfg.Table.OID] {
			continue
		}
		if c.missingOIDs[trimOID(cfg.Table.OID)] {
			missingOIDs = append(missingOIDs, cfg.Table.OID)
			continue
		}

		doneOids[cfg.Table.OID] = true
		tableMetrics, err := c.collectSingleTable(cfg)
		if err != nil {
			errs = append(errs, fmt.Errorf("table '%s': %w", cfg.Table.Name, err))
			continue
		}
		metrics = append(metrics, tableMetrics...)
	}

	if len(missingOIDs) > 0 {
		c.log.Debugf("table metrics missing OIDs: %v", missingOIDs)
	}

	if len(metrics) == 0 && len(errs) > 0 {
		return nil, errors.Join(errs...)
	}

	return metrics, nil
}

func (c *Collector) collectSingleTable(cfg ddprofiledefinition.MetricsConfig) ([]Metric, error) {
	for _, tagCfg := range cfg.MetricTags {
		if tagCfg.Table != "" && tagCfg.Table != cfg.Table.Name {
			c.log.Debugf("Skipping table %s: has cross-table tag from %s", cfg.Table.Name, tagCfg.Table)
			return nil, nil
		}
		if len(tagCfg.IndexTransform) > 0 {
			c.log.Debugf("Skipping table %s: has index transformation", cfg.Table.Name)
			return nil, nil
		}
	}

	pdus, err := c.snmpWalk(cfg.Table.OID)
	if err != nil {
		return nil, fmt.Errorf("failed to walk table: %w", err)
	}

	if len(pdus) == 0 {
		return nil, nil
	}

	symColumnOIDs := make(map[string]ddprofiledefinition.SymbolConfig)
	for _, sym := range cfg.Symbols {
		symColumnOIDs[trimOID(sym.OID)] = sym
	}

	tagColumnOIDs := make(map[string]ddprofiledefinition.MetricTagConfig)
	for _, tagCfg := range cfg.MetricTags {
		if tagCfg.Table == "" || tagCfg.Table == cfg.Table.Name {
			tagColumnOIDs[trimOID(tagCfg.Symbol.OID)] = tagCfg
		}
	}

	allColumnOIDs := make([]string, 0, len(symColumnOIDs)+len(tagColumnOIDs))
	for oid := range symColumnOIDs {
		allColumnOIDs = append(allColumnOIDs, oid)
	}
	for oid := range tagColumnOIDs {
		allColumnOIDs = append(allColumnOIDs, oid)
	}

	// Group PDUs by row index (index -> column OID -> PDU)
	rows := make(map[string]map[string]gosnmp.SnmpPDU, len(pdus)/len(allColumnOIDs))

	for oid, pdu := range pdus {
		for _, columnOID := range allColumnOIDs {
			if strings.HasPrefix(oid, columnOID+".") {
				index := strings.TrimPrefix(oid, columnOID+".")

				if rows[index] == nil {
					rows[index] = make(map[string]gosnmp.SnmpPDU)
				}
				rows[index][columnOID] = pdu
				break
			}
		}
	}

	var metrics []Metric
	for index, rowPDUs := range rows {
		rowMetrics, err := c.processTableRow(rowPDUs, symColumnOIDs, tagColumnOIDs, cfg.StaticTags)
		if err != nil {
			c.log.Debugf("Error processing row %s: %v", index, err)
			continue
		}
		metrics = append(metrics, rowMetrics...)
	}

	return metrics, nil
}

func (c *Collector) processTableRow(
	rowPDUs map[string]gosnmp.SnmpPDU,
	columnOIDs map[string]ddprofiledefinition.SymbolConfig,
	tagColumnOIDs map[string]ddprofiledefinition.MetricTagConfig,
	staticTags []string,
) ([]Metric, error) {
	var metrics []Metric

	// Process tags first to ensure all metrics in the row get the same tags
	rowTags := make(map[string]string)

	for _, tag := range staticTags {
		if n, v, _ := strings.Cut(tag, ":"); n != "" && v != "" {
			rowTags[n] = v
		}
	}

	for columnOID, tagCfg := range tagColumnOIDs {
		pdu, ok := rowPDUs[columnOID]
		if !ok {
			continue
		}

		tags, err := processTableMetricTagValue(tagCfg, pdu)
		if err != nil {
			c.log.Debugf("Error processing tag %s: %v", tagCfg.Tag, err)
			continue
		}

		for k, v := range tags {
			rowTags[k] = v
		}
	}

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
			Tags:        make(map[string]string),
			Unit:        sym.Unit,
			Description: sym.Description,
			MetricType:  getMetricType(sym, pdu),
			Family:      sym.Family,
			Mappings:    convSymMappingToNumeric(sym),
			IsTable:     true,
		}

		maps.Copy(metric.Tags, rowTags)

		metrics = append(metrics, metric)
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
		if !isPduWithData(pdu) {
			c.missingOIDs[trimOID(pdu.Name)] = true
			continue
		}
		pdus[trimOID(pdu.Name)] = pdu
	}

	return pdus, nil
}
