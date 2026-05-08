// SPDX-License-Identifier: GPL-3.0-or-later

package ddsnmpcollector

import (
	"testing"

	"github.com/gosnmp/gosnmp"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/netdata/netdata/go/plugins/logger"
	"github.com/netdata/netdata/go/plugins/plugin/go.d/collector/snmp/ddsnmp/ddprofiledefinition"
)

func TestHuaweiBGPStatisticsRowTags(t *testing.T) {
	profile := matchedProfileByFile(t, "1.3.6.1.4.1.2011.2.224.279", "huawei-routers.yaml")

	var statsCfg ddprofiledefinition.MetricsConfig
	var peerTableOID string
	found := false
	for _, metric := range profile.Definition.Metrics {
		switch metric.Table.Name {
		case "hwBgpPeerStatisticTable":
			statsCfg = metric
			found = true
		case "hwBgpPeerTable":
			peerTableOID = metric.Table.OID
		}
	}
	require.True(t, found)
	require.NotEmpty(t, peerTableOID)

	ordered := buildOrderedTags(statsCfg)
	require.Len(t, ordered, 7)
	assert.Equal(t, tagTypeIndex, ordered[0].tagType)
	assert.Equal(t, tagTypeIndex, ordered[1].tagType)
	assert.Equal(t, tagTypeCrossTable, ordered[2].tagType)
	assert.Equal(t, tagTypeCrossTable, ordered[3].tagType)
	assert.Equal(t, tagTypeIndex, ordered[4].tagType)
	assert.Equal(t, tagTypeCrossTable, ordered[5].tagType)
	assert.Equal(t, tagTypeCrossTable, ordered[6].tagType)

	fixturePDUs := loadSnmprecPDUs(t, "librenms/huawei_vrp_ne8000_bgp_peer_table.snmprec")
	statsPDUs := pduSliceToMap(snmprecPDUsWithPrefix(fixturePDUs, "1.3.6.1.4.1.2011.5.25.177.1.1.7."))
	peerPDUs := pduSliceToMap(snmprecPDUsWithPrefix(fixturePDUs, "1.3.6.1.4.1.2011.5.25.177.1.1.2."))

	rowIndex := "0.0.4.10.45.2.2"
	rowPDUs := make(map[string]gosnmp.SnmpPDU)
	for _, sym := range statsCfg.Symbols {
		fullOID := trimOID(sym.OID) + "." + rowIndex
		pdu, ok := statsPDUs[fullOID]
		require.Truef(t, ok, "missing PDU for %s", fullOID)
		rowPDUs[trimOID(sym.OID)] = pdu
	}

	row := &tableRowData{
		index: rowIndex,
		pdus:  rowPDUs,
		tags:  make(map[string]string),
	}

	processor := newTableRowProcessor(logger.New())
	err := processor.processRowTags(row, &tableRowProcessingContext{
		config: statsCfg,
		crossTableCtx: &crossTableContext{
			walkedData: map[string]map[string]gosnmp.SnmpPDU{
				trimOID(peerTableOID): peerPDUs,
			},
			tableNameToOID: map[string]string{
				"hwBgpPeerTable": trimOID(peerTableOID),
			},
			lookupIndexCache: make(map[crossTableLookupKey]string),
			rowTags:          row.tags,
		},
		orderedTags: ordered,
	})
	require.NoError(t, err)

	assert.Equal(t, "0", row.tags["_routing_instance"])
	assert.Equal(t, "0", row.tags["huawei_hw_bgp_peer_vrf_instance_id"])
	assert.Equal(t, "10.45.2.2", row.tags["_neighbor"])
	assert.Equal(t, "10.45.2.2", row.tags["huawei_hw_bgp_peer_remote_addr"])
	assert.Equal(t, "ipv4", row.tags["_neighbor_address_type"])
	assert.Equal(t, "26479", row.tags["_remote_as"])
}

func pduSliceToMap(pdus []gosnmp.SnmpPDU) map[string]gosnmp.SnmpPDU {
	out := make(map[string]gosnmp.SnmpPDU, len(pdus))
	for _, pdu := range pdus {
		out[trimOID(pdu.Name)] = pdu
	}
	return out
}
