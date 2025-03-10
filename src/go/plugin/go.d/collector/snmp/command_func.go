package snmp

import (
	"fmt"
	"log"
	"strings"
)

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
		// fmt.Println(nextOID, baseOID)

		// If the next OID does not start with the base OID, we've reached the end of the subtree.
		if !strings.HasPrefix(nextOID, baseOID) {
			return tableRows, nil
		}

		metricType := pdu.Type
		value := fmt.Sprintf("%v", pdu.Value)

		tableRows[nextOID] = processedMetric{
			oid:         nextOID,
			value:       value,
			metric_type: metricType,
		}

		currentOID = nextOID
	}
}
