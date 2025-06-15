// SPDX-License-Identifier: GPL-3.0-or-later

package ddsnmpcollector

import (
	"errors"
	"fmt"
	"slices"
	"strconv"
	"strings"

	"github.com/gosnmp/gosnmp"

	"github.com/netdata/netdata/go/plugins/plugin/go.d/collector/snmp/ddsnmp"
	"github.com/netdata/netdata/go/plugins/plugin/go.d/collector/snmp/ddsnmp/ddprofiledefinition"
)

func (c *Collector) collectScalarMetrics(prof *ddsnmp.Profile) ([]Metric, error) {
	var oids []string
	var missingOIDs []string

	for _, m := range prof.Definition.Metrics {
		if !m.IsScalar() {
			continue
		}
		if c.missingOIDs[trimOID(m.Symbol.OID)] {
			missingOIDs = append(missingOIDs, m.Symbol.OID)
			continue
		}
		oids = append(oids, m.Symbol.OID)
	}

	if len(missingOIDs) > 0 {
		c.log.Debugf("scalar metrics missing OIDs: %v", missingOIDs)
	}

	slices.Sort(oids)
	oids = slices.Compact(oids)

	if len(oids) == 0 {
		return nil, nil
	}

	pdus, err := c.snmpGet(oids)
	if err != nil {
		return nil, err
	}

	metrics := make([]Metric, 0, len(prof.Definition.Metrics))
	var errs []error

	for _, cfg := range prof.Definition.Metrics {
		if !cfg.IsScalar() {
			continue
		}

		metric, err := c.collectScalarMetric(cfg, pdus)
		if err != nil {
			errs = append(errs, fmt.Errorf("metric '%s': %w", cfg.Symbol.Name, err))
			c.log.Debugf("Error processing scalar metric '%s': %v", cfg.Symbol.Name, err)
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

func (c *Collector) collectScalarMetric(cfg ddprofiledefinition.MetricsConfig, pdus map[string]gosnmp.SnmpPDU) (*Metric, error) {
	pdu, ok := pdus[trimOID(cfg.Symbol.OID)]
	if !ok {
		return nil, nil
	}

	value, err := processSymbolValue(cfg.Symbol, pdu)
	if err != nil {
		return nil, fmt.Errorf("error processing value for OID %s (%s): %w", cfg.Symbol.Name, cfg.Symbol.OID, err)
	}

	staticTags := make(map[string]string)

	for _, tag := range cfg.StaticTags {
		if n, v, _ := strings.Cut(tag, ":"); n != "" && v != "" {
			staticTags[n] = v
		}
	}

	return &Metric{
		Name:        cfg.Symbol.Name,
		Value:       value,
		StaticTags:  ternary(len(staticTags) > 0, staticTags, nil),
		Unit:        cfg.Symbol.Unit,
		Description: cfg.Symbol.Description,
		Family:      cfg.Symbol.Family,
		Mappings:    convSymMappingToNumeric(cfg.Symbol),
		MetricType:  getMetricType(cfg.Symbol, pdu),
	}, nil
}

func processSymbolValue(sym ddprofiledefinition.SymbolConfig, pdu gosnmp.SnmpPDU) (int64, error) {
	var value int64

	if isPduNumericType(pdu) {
		value = gosnmp.ToBigInt(pdu.Value).Int64()
		if len(sym.Mapping) > 0 {
			s := strconv.FormatInt(value, 10)
			if v, ok := sym.Mapping[s]; ok && isInt(v) {
				value, _ = strconv.ParseInt(v, 10, 64)
			}
		}
	} else {
		s, err := convPduToStringf(pdu, sym.Format)
		if err != nil {
			return 0, err
		}

		switch {
		case sym.ExtractValueCompiled != nil:
			if sm := sym.ExtractValueCompiled.FindStringSubmatch(s); len(sm) > 1 {
				s = sm[1]
			}
		case sym.MatchPatternCompiled != nil:
			if sm := sym.MatchPatternCompiled.FindStringSubmatch(s); len(sm) > 0 {
				s = replaceSubmatches(sym.MatchValue, sm)
			}
		}

		// Handle mapping based on the mapping type:
		// 1. Int -> String mapping (e.g., {"1": "up", "2": "down"}):
		//    - Used for creating dimensions later
		//    - Value remains numeric, no conversion needed here
		// 2. String -> Int mapping (e.g., {"OK": "0", "WARNING": "1"}):
		//    - Used to convert string values to numeric values
		//    - Apply the mapping to get the numeric representation
		if v, ok := sym.Mapping[s]; ok && isInt(v) {
			s = v
		}

		value, err = strconv.ParseInt(s, 10, 64)
		if err != nil {
			return 0, err
		}
	}

	if sym.ScaleFactor != 0 {
		value = int64(float64(value) * sym.ScaleFactor)
	}

	return value, nil
}
