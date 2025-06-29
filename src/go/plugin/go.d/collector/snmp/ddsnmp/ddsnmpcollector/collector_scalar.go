// SPDX-License-Identifier: GPL-3.0-or-later

package ddsnmpcollector

import (
	"errors"
	"fmt"
	"slices"

	"github.com/gosnmp/gosnmp"

	"github.com/netdata/netdata/go/plugins/logger"
	"github.com/netdata/netdata/go/plugins/plugin/go.d/collector/snmp/ddsnmp"
	"github.com/netdata/netdata/go/plugins/plugin/go.d/collector/snmp/ddsnmp/ddprofiledefinition"
)

// scalarCollector handles collection of scalar (non-table) metrics
type scalarCollector struct {
	snmpClient  gosnmp.Handler
	missingOIDs map[string]bool
	log         *logger.Logger
	valProc     *valueProcessor
}

func newScalarCollector(snmpClient gosnmp.Handler, missingOIDs map[string]bool, log *logger.Logger) *scalarCollector {
	return &scalarCollector{
		snmpClient:  snmpClient,
		missingOIDs: missingOIDs,
		log:         log,
		valProc:     newValueProcessor(),
	}
}

// Collect gathers all scalar metrics from the profile
func (sc *scalarCollector) Collect(prof *ddsnmp.Profile) ([]ddsnmp.Metric, error) {
	oids, missingOIDs := sc.identifyScalarOIDs(prof.Definition.Metrics)

	if len(missingOIDs) > 0 {
		sc.log.Debugf("scalar metrics missing OIDs: %v", missingOIDs)
	}

	if len(oids) == 0 {
		return nil, nil
	}

	pdus, err := sc.getScalarValues(oids)
	if err != nil {
		return nil, err
	}

	return sc.processScalarMetrics(prof.Definition.Metrics, pdus)
}

// identifyScalarOIDs returns OIDs to collect and OIDs that are known to be missing
func (sc *scalarCollector) identifyScalarOIDs(configs []ddprofiledefinition.MetricsConfig) ([]string, []string) {
	var oids []string
	var missingOIDs []string

	for _, cfg := range configs {
		if !cfg.IsScalar() {
			continue
		}

		oid := trimOID(cfg.Symbol.OID)
		if sc.missingOIDs[oid] {
			missingOIDs = append(missingOIDs, cfg.Symbol.OID)
			continue
		}

		oids = append(oids, cfg.Symbol.OID)
	}

	// Sort and deduplicate
	slices.Sort(oids)
	oids = slices.Compact(oids)

	return oids, missingOIDs
}

func (sc *scalarCollector) getScalarValues(oids []string) (map[string]gosnmp.SnmpPDU, error) {
	pdus := make(map[string]gosnmp.SnmpPDU)
	maxOids := sc.snmpClient.MaxOids()

	for chunk := range slices.Chunk(oids, maxOids) {
		result, err := sc.snmpClient.Get(chunk)
		if err != nil {
			return nil, err
		}

		for _, pdu := range result.Variables {
			if !isPduWithData(pdu) {
				sc.missingOIDs[trimOID(pdu.Name)] = true
				continue
			}
			pdus[trimOID(pdu.Name)] = pdu
		}
	}

	return pdus, nil
}

// processScalarMetrics converts PDUs into metrics
func (sc *scalarCollector) processScalarMetrics(configs []ddprofiledefinition.MetricsConfig, pdus map[string]gosnmp.SnmpPDU) ([]ddsnmp.Metric, error) {
	var metrics []ddsnmp.Metric
	var errs []error

	for _, cfg := range configs {
		if !cfg.IsScalar() {
			continue
		}

		metric, err := sc.processScalarMetric(cfg, pdus)
		if err != nil {
			errs = append(errs, fmt.Errorf("metric '%s': %w", cfg.Symbol.Name, err))
			sc.log.Debugf("Error processing scalar metric '%s': %v", cfg.Symbol.Name, err)
			continue
		}

		if metric != nil {
			metrics = append(metrics, *metric)
		}
	}

	if len(metrics) == 0 && len(errs) > 0 {
		return nil, errors.Join(errs...)
	}

	return metrics, nil
}

// processScalarMetric processes a single scalar metric configuration
func (sc *scalarCollector) processScalarMetric(cfg ddprofiledefinition.MetricsConfig, pdus map[string]gosnmp.SnmpPDU) (*ddsnmp.Metric, error) {
	pdu, ok := pdus[trimOID(cfg.Symbol.OID)]
	if !ok {
		return nil, nil
	}

	value, err := sc.valProc.processValue(cfg.Symbol, pdu)
	if err != nil {
		return nil, fmt.Errorf("error processing value for OID %s (%s): %w", cfg.Symbol.Name, cfg.Symbol.OID, err)
	}

	staticTags := parseStaticTags(cfg.StaticTags)

	return buildScalarMetric(cfg.Symbol, pdu, value, staticTags)
}
