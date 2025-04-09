package snmp

import (
	"fmt"

	"github.com/gosnmp/gosnmp"

	"github.com/netdata/netdata/go/plugins/plugin/go.d/collector/snmp/ddsnmp"
)

func (s *SysObjectIDs) UnmarshalYAML(unmarshal func(any) error) error {
	var single string
	if err := unmarshal(&single); err == nil {
		*s = []string{single}
		return nil
	}

	var multiple []string
	if err := unmarshal(&multiple); err == nil {
		*s = multiple
		return nil
	}

	return fmt.Errorf("invalid sysobjectid format")
}

func (c *Collector) parseMetricsFromProfiles(matchingProfiles []*ddsnmp.Profile) (map[string]processedMetric, error) {
	metricMap := map[string]processedMetric{}
	for _, profile := range matchingProfiles {
		profileDef := profile.Definition
		results, err := parseMetrics(profileDef.Metrics)
		if err != nil {
			return nil, err
		}

		for _, oid := range results.oids {
			response, err := c.snmpClient.Get([]string{oid})
			if err != nil {
				return nil, err
			}
			if (response != &gosnmp.SnmpPacket{}) {
				for _, metric := range results.parsed_metrics {
					switch s := metric.(type) {
					case parsedSymbolMetric:
						// find a matching metric
						if s.baseoid == oid {
							metricName := s.name
							metricType := response.Variables[0].Type
							metricValue := response.Variables[0].Value
							metricUnit := s.unit
							metricDescription := s.description

							metricMap[oid] = processedMetric{oid: oid, name: metricName, value: metricValue, metric_type: metricType, unit: metricUnit, description: metricDescription}
						}
					}
				}

			}
		}

		for _, oid := range results.next_oids {
			if len(oid) < 1 {
				continue
			}
			if tableRows, err := c.walkOIDTree(oid); err != nil {
				return nil, fmt.Errorf("error walking OID tree: %v, oid %s", err, oid)
			} else {
				for _, metric := range results.parsed_metrics {
					switch s := metric.(type) {
					case parsedTableMetric:
						// find a matching metric
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

	}
	return metricMap, nil
}
