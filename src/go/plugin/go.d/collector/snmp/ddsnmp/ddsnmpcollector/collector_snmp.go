// SPDX-License-Identifier: GPL-3.0-or-later

package ddsnmpcollector

import (
	"slices"

	"github.com/gosnmp/gosnmp"

	"github.com/netdata/netdata/go/plugins/plugin/go.d/collector/snmp/ddsnmp"
)

// Count attempted batches here so callers cannot lose or duplicate failed work.
func getSNMPValues(
	client gosnmp.Handler,
	oids []string,
	missingOIDs map[string]bool,
	stats *ddsnmp.CollectionStats,
) (map[string]gosnmp.SnmpPDU, error) {
	pdus := make(map[string]gosnmp.SnmpPDU)
	for chunk := range slices.Chunk(oids, client.MaxOids()) {
		stats.SNMP.GetRequests++
		stats.SNMP.GetOIDs += int64(len(chunk))
		result, err := client.Get(chunk)
		if err != nil {
			stats.Errors.SNMP++
			return nil, err
		}
		for _, pdu := range result.Variables {
			if !isPduWithData(pdu) {
				stats.Errors.MissingOIDs++
				missingOIDs[trimOID(pdu.Name)] = true
				continue
			}
			pdus[trimOID(pdu.Name)] = pdu
		}
	}
	return pdus, nil
}
