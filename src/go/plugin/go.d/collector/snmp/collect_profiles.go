// SPDX-License-Identifier: GPL-3.0-or-later

package snmp

import (
	"fmt"
	"log"
	"strings"

	"github.com/gosnmp/gosnmp"
)

type processedMetric struct {
	oid         string
	name        string
	value       interface{}
	metricType  gosnmp.Asn1BER
	tableName   string
	unit        string
	description string
}

func (c *Collector) collectProfiles(mx map[string]int64) error {
	if len(c.snmpProfiles) == 0 {
		return nil
	}

	metricMap := map[string]processedMetric{}

	for _, prof := range c.snmpProfiles {
		results, err := parseMetrics(prof.Definition.Metrics)
		if err != nil {
			return err
		}

		for _, oid := range results.OIDs {
			response, err := c.snmpClient.Get([]string{oid})
			if err != nil {
				return err
			}
			for _, metric := range results.parsedMetrics {
				switch s := metric.(type) {
				case parsedSymbolMetric:
					if s.baseoid == oid {
						metricMap[oid] = processedMetric{
							oid:         oid,
							name:        s.name,
							value:       response.Variables[0].Value,
							metricType:  response.Variables[0].Type,
							unit:        s.unit,
							description: s.description,
						}
					}
				}
			}
		}

		for _, oid := range results.nextOIDs {
			if len(oid) == 0 {
				continue
			}

			tableRows, err := c.walkOIDTree(oid)
			if err != nil {
				return fmt.Errorf("error walking OID tree: %v, oid %s", err, oid)
			}

			for _, metric := range results.parsedMetrics {
				switch s := metric.(type) {
				case parsedTableMetric:
					if s.rowOID == oid {
						for key, value := range tableRows {
							value.name = s.name
							value.tableName = s.tableName
							tableRows[key] = value
						}
						metricMap = mergeProcessedMetricMaps(metricMap, tableRows)
					}
				}
			}
		}
	}

	c.makeChartsFromMetricMap(mx, metricMap)

	return nil
}

func (c *Collector) walkOIDTree(baseOID string) (map[string]processedMetric, error) {
	tableRows := make(map[string]processedMetric)

	currentOID := baseOID
	for {
		result, err := c.snmpClient.GetNext([]string{currentOID})
		if err != nil {
			return tableRows, fmt.Errorf("snmpgetnext failed: %v", err)
		}
		if len(result.Variables) == 0 {
			log.Println("No OID returned, ending walk.")
			return tableRows, nil
		}
		pdu := result.Variables[0]

		nextOID := strings.Replace(pdu.Name, ".", "", 1) //remove dot at the start of the OID

		// If the next OID does not start with the base OID, we've reached the end of the subtree.
		if !strings.HasPrefix(nextOID, baseOID) {
			return tableRows, nil
		}

		metricType := pdu.Type
		value := fmt.Sprintf("%v", pdu.Value)

		tableRows[nextOID] = processedMetric{
			oid:        nextOID,
			value:      value,
			metricType: metricType,
		}

		currentOID = nextOID
	}
}

func (c *Collector) makeChartsFromMetricMap(mx map[string]int64, metricMap map[string]processedMetric) {
	seen := make(map[string]bool)

	for _, metric := range metricMap {
		if metric.tableName == "" {
			switch s := metric.value.(type) {
			case int:
				name := metric.name
				if name == "" {
					continue
				}

				seen[name] = true

				if !c.seenMetrics[name] {
					c.seenMetrics[name] = true
					c.addSNMPChart(metric)
				}

				mx[metric.name] = int64(s)
			}
		}

	}
	for name := range c.seenMetrics {
		if !seen[name] {
			delete(c.seenMetrics, name)
			c.removeSNMPChart(name)
		}
	}
}

func mergeProcessedMetricMaps(m1 map[string]processedMetric, m2 map[string]processedMetric) map[string]processedMetric {
	merged := make(map[string]processedMetric)
	for k, v := range m1 {
		merged[k] = v
	}
	for key, value := range m2 {
		merged[key] = value
	}
	return merged
}
