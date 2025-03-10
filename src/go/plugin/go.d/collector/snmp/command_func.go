package snmp

import (
	"fmt"
	"log"
	"strings"
	"time"

	"github.com/gosnmp/gosnmp"
)

// // SNMPWalk uses gosnmp.Walk to perform an SNMP walk and returns a map from OID to [2]string,
// // where the first element is the SNMP type (as a string) and the second is the value.
// func SNMPWalk(target string, oid string, community string) (map[string][2]string, error) {
// 	results := make(map[string][2]string)

// 	params := &gosnmp.GoSNMP{
// 		Target:    target,
// 		Port:      161,
// 		Community: community,
// 		Version:   gosnmp.Version2c,
// 		Timeout:   2 * time.Second,
// 		Retries:   3,
// 	}
// 	if err := params.Connect(); err != nil {
// 		return nil, fmt.Errorf("error connecting to target: %v", err)
// 	}
// 	defer params.Conn.Close()

// 	err := params.Walk(oid, func(pdu gosnmp.SnmpPDU) error {
// 		nestedOID := pdu.Name
// 		metricType := pdu.Type.String()
// 		metricValue := fmt.Sprintf("%v", pdu.Value)

// 		// Filter out "No Such" responses.
// 		if !strings.Contains(metricValue, "No Such") && !strings.Contains(metricValue, "at this OID") {
// 			results[nestedOID] = [2]string{metricType, metricValue}
// 		} else {
// 			fmt.Println("skipping empty OID")
// 		}
// 		return nil
// 	})
// 	if err != nil {
// 		return nil, err
// 	}
// 	fmt.Println("Parsing done")
// 	return results, nil
// }

// SNMPGet uses gosnmp.Get to perform an SNMP get and returns the result as a string.
func SNMPGet(target string, oid string, community string) (snmpPDU, error) {
	params := &gosnmp.GoSNMP{
		Target:    target,
		Port:      161,
		Community: community,
		Version:   gosnmp.Version2c,
		Timeout:   2 * time.Second,
		Retries:   3,
	}
	if err := params.Connect(); err != nil {
		return snmpPDU{}, fmt.Errorf("error connecting: %v", err)
	}
	defer params.Conn.Close()

	result, err := params.Get([]string{oid})
	if err != nil {
		return snmpPDU{}, fmt.Errorf("snmpget failed: %v", err)
	}
	if len(result.Variables) == 0 {
		return snmpPDU{}, nil
	}
	pdu := snmpPDU{value: result.Variables[0].Value, oid: result.Variables[0].Name, metric_type: result.Variables[0].Type}

	return pdu, nil

}

// runSNMPGetNext uses gosnmp.GetNext to get the next OID and returns a formatted string.
// The output is formatted similarly to the original command output ("<nextOID> = <value>").
func runSNMPGetNext(target string, oid string, community string) (string, error) {
	params := &gosnmp.GoSNMP{
		Target:    target,
		Port:      161,
		Community: community,
		Version:   gosnmp.Version2c,
		Timeout:   2 * time.Second,
		Retries:   3,
	}
	if err := params.Connect(); err != nil {
		return "", fmt.Errorf("error connecting: %v", err)
	}
	defer params.Conn.Close()

	result, err := params.GetNext([]string{oid})
	if err != nil {
		return "", fmt.Errorf("snmpgetnext failed: %v", err)
	}
	if len(result.Variables) == 0 {
		return "", nil
	}
	pdu := result.Variables[0]
	output := fmt.Sprintf("%s = %s", pdu.Name, fmt.Sprintf("%v", pdu.Value))
	return output, nil
}

// walkOIDTree repeatedly calls SNMP GetNext (via gosnmp.GetNext) to walk a subtree of OIDs.
// It stops when the next OID no longer begins with the baseOID.
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
