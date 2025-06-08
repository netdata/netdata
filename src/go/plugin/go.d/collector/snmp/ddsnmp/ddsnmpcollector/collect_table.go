// SPDX-License-Identifier: GPL-3.0-or-later

package ddsnmpcollector

import (
	"errors"
	"fmt"
	"strings"

	"github.com/gosnmp/gosnmp"

	"github.com/netdata/netdata/go/plugins/plugin/go.d/collector/snmp/ddsnmp"
	"github.com/netdata/netdata/go/plugins/plugin/go.d/collector/snmp/ddsnmp/ddprofiledefinition"
)

func (c *Collector) collectTableMetrics(prof *ddsnmp.Profile) ([]Metric, error) {
	var metrics []Metric
	var errs []error

	doneOids := make(map[string]bool)

	for _, cfg := range prof.Definition.Metrics {
		if cfg.IsScalar() || cfg.Table.OID == "" || doneOids[cfg.Table.OID] {
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

	if len(metrics) == 0 && len(errs) > 0 {
		return nil, errors.Join(errs...)
	}

	return metrics, nil
}

func (c *Collector) collectSingleTable(cfg ddprofiledefinition.MetricsConfig) ([]Metric, error) {
	pdus, err := c.snmpWalk(cfg.Table.OID)
	if err != nil {
		return nil, fmt.Errorf("failed to walk table: %w", err)
	}

	if len(pdus) == 0 {
		return nil, nil
	}

	// Build a set of column OIDs we're interested in
	columnOIDs := make(map[string]ddprofiledefinition.SymbolConfig)
	for _, sym := range cfg.Symbols {
		columnOIDs[trimOID(sym.OID)] = sym
	}

	// Group PDUs by row index
	rows := make(map[string]map[string]gosnmp.SnmpPDU) // index -> column OID -> PDU

	for oid, pdu := range pdus {
		// Check if this OID belongs to any of our columns
		for columnOID := range columnOIDs {
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
		rowMetrics, err := c.processTableRow(rowPDUs, columnOIDs)
		if err != nil {
			c.log.Debugf("Error processing row %s: %v", index, err)
			continue
		}
		metrics = append(metrics, rowMetrics...)
	}

	return metrics, nil
}

func (c *Collector) processTableRow(rowPDUs map[string]gosnmp.SnmpPDU, columnOIDs map[string]ddprofiledefinition.SymbolConfig) ([]Metric, error) {
	var metrics []Metric

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
		}

		metrics = append(metrics, metric)
	}

	return metrics, nil
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

	return pdus, nil
}
