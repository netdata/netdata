// SPDX-License-Identifier: GPL-3.0-or-later

package snmp_traps

import "fmt"

func trapEntryFromPDU(jobName, vnode string, pdu *TrapPDU, td *TrapDef, realtimeUsec, monotonicUsec int64) *TrapEntry {
	entry := &TrapEntry{
		JobName:               jobName,
		ReportType:            ReportTypeTrap,
		ReceivedRealtimeUsec:  realtimeUsec,
		ReceivedMonotonicUsec: monotonicUsec,
		TrapOID:               pdu.OID,
		Category:              "unknown",
		Severity:              "notice",
		SourceIP:              pdu.SourceIP,
		SourceUDPPeer:         pdu.PeerIP,
		PduType:               pdu.PduType,
		SnmpVersion:           pdu.Version,
		SourceVnodeID:         vnode,
		Varbinds:              make([]VarbindValue, 0, len(pdu.Varbinds)),
	}

	if td != nil {
		entry.TrapName = td.Name
		entry.Category = Category(td.Category)
		entry.Severity = Severity(td.Severity)
	}

	for _, vb := range pdu.Varbinds {
		entry.Varbinds = append(entry.Varbinds, resolve2TierVarbind(vb.OID, vb, td))
	}

	if td != nil {
		entry.Message = renderMessage(entry, td)
		entry.Labels = renderLabels(entry, td)
	} else {
		source := entry.SourceIP
		if source == "" {
			source = entry.SourceUDPPeer
		}
		entry.Message = fmt.Sprintf("SNMP trap %s from %s", entry.TrapOID, source)
	}

	return entry
}
