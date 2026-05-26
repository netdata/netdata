// SPDX-License-Identifier: GPL-3.0-or-later

package snmp_traps

import (
	"encoding/json"
	"fmt"
	"sort"
	"strings"
)

var severityToPriority = map[Severity]string{
	"emerg":   "0",
	"alert":   "1",
	"crit":    "2",
	"err":     "3",
	"warning": "4",
	"notice":  "5",
	"info":    "6",
	"debug":   "7",
}

func severityPriority(sev Severity) string {
	if p, ok := severityToPriority[sev]; ok {
		return p
	}
	return "5"
}

func serializeToJournalFields(entry *TrapEntry) ([]JournalField, error) {
	if entry == nil {
		return nil, errNilEntry
	}
	if entry.JobName == "" {
		return nil, errMissingJobName
	}
	if entry.ReceivedRealtimeUsec < 0 || entry.ReceivedMonotonicUsec < 0 {
		return nil, errNegativeTimestamp
	}

	isDedupSummary := entry.ReportType == ReportTypeDedupSummary
	isDecodeSummary := entry.ReportType == ReportTypeDecodeErrorSummary
	isSummary := isDedupSummary || isDecodeSummary

	if !isSummary {
		if entry.TrapOID == "" {
			return nil, errMissingTrapOID
		}
		if entry.SourceIP == "" && entry.SourceUDPPeer == "" && entry.DeviceHostname == "" {
			return nil, errMissingSourceIP
		}
	}

	hostname := entry.DeviceHostname
	if hostname == "" {
		if entry.SourceIP != "" {
			hostname = entry.SourceIP
		} else if entry.SourceUDPPeer != "" {
			hostname = entry.SourceUDPPeer
		}
	}

	var fields []JournalField

	fields = append(fields, JournalField{Name: "MESSAGE", Value: []byte(entry.Message)})
	fields = append(fields, JournalField{Name: "PRIORITY", Value: []byte(severityPriority(entry.Severity))})
	fields = append(fields, JournalField{Name: "SYSLOG_IDENTIFIER", Value: []byte(entry.JobName)})

	if !isSummary && hostname != "" {
		fields = append(fields, JournalField{Name: "_HOSTNAME", Value: []byte(hostname)})
	}

	fields = append(fields, JournalField{Name: "ND_LOG_SOURCE", Value: []byte("snmp-trap")})

	if !isSummary && entry.SourceVnodeID != "" {
		fields = append(fields, JournalField{Name: "ND_NIDL_NODE", Value: []byte(entry.SourceVnodeID)})
	}

	reportType := string(entry.ReportType)
	if reportType == "" {
		reportType = "trap"
	}
	fields = append(fields, JournalField{Name: "TRAP_REPORT_TYPE", Value: []byte(reportType)})

	if !isSummary {
		fields = append(fields, JournalField{Name: "TRAP_OID", Value: []byte(entry.TrapOID)})
		if entry.TrapName != "" {
			fields = append(fields, JournalField{Name: "TRAP_NAME", Value: []byte(entry.TrapName)})
		}
		if entry.Category != "" {
			fields = append(fields, JournalField{Name: "TRAP_CATEGORY", Value: []byte(entry.Category)})
		}
		if entry.Severity != "" {
			fields = append(fields, JournalField{Name: "TRAP_SEVERITY", Value: []byte(entry.Severity)})
		}
		if entry.PduType != "" {
			fields = append(fields, JournalField{Name: "TRAP_PDU_TYPE", Value: []byte(entry.PduType)})
		}
		if entry.SnmpVersion != "" {
			fields = append(fields, JournalField{Name: "TRAP_VERSION", Value: []byte(entry.SnmpVersion)})
		}
		if entry.SourceIP != "" {
			fields = append(fields, JournalField{Name: "TRAP_SOURCE_IP", Value: []byte(entry.SourceIP)})
		}
		if entry.SourceUDPPeer != "" {
			fields = append(fields, JournalField{Name: "TRAP_SOURCE_UDP_PEER", Value: []byte(entry.SourceUDPPeer)})
		}
		if entry.DeviceVendor != "" {
			fields = append(fields, JournalField{Name: "TRAP_DEVICE_VENDOR", Value: []byte(entry.DeviceVendor)})
		}
		if entry.TopologyInterface != "" {
			fields = append(fields, JournalField{Name: "TRAP_INTERFACE", Value: []byte(entry.TopologyInterface)})
		}
		if entry.TopologyNeighbors != "" {
			fields = append(fields, JournalField{Name: "TRAP_NEIGHBORS", Value: []byte(entry.TopologyNeighbors)})
		}
	}

	trapJSON, err := buildTrapJSON(entry)
	if err != nil {
		return nil, fmt.Errorf("TRAP_JSON: %w", err)
	}
	fields = append(fields, JournalField{Name: "TRAP_JSON", Value: trapJSON})

	if isDedupSummary && entry.SummaryCounts != nil {
		sc := entry.SummaryCounts
		fields = append(fields, JournalField{Name: "TRAP_SUPPRESSED_COUNT", Value: []byte(fmt.Sprintf("%d", sc.TotalSuppressed))})
		fields = append(fields, JournalField{Name: "TRAP_SUPPRESSED_FINGERPRINTS", Value: []byte(fmt.Sprintf("%d", sc.Fingerprints))})
		fields = append(fields, JournalField{Name: "TRAP_REPORT_PERIOD_SEC", Value: []byte(fmt.Sprintf("%d", sc.PeriodSec))})
	}

	sortedLabels := sortedMapKeys(entry.Labels)
	for _, key := range sortedLabels {
		val := entry.Labels[key]
		upperKey := strings.ToUpper(key)
		if !isValidTrapTagKey(upperKey) {
			return nil, fmt.Errorf("invalid label key for TRAP_TAG: %q", key)
		}
		fields = append(fields, JournalField{Name: "TRAP_TAG_" + upperKey, Value: []byte(val)})
	}

	return fields, nil
}

func isValidTrapTagKey(upperKey string) bool {
	if upperKey == "" {
		return false
	}
	for i, r := range upperKey {
		if i == 0 {
			if r < 'A' || r > 'Z' {
				return false
			}
		} else {
			if (r < 'A' || r > 'Z') && (r < '0' || r > '9') && r != '_' {
				return false
			}
		}
	}
	return true
}

func buildTrapJSON(entry *TrapEntry) ([]byte, error) {
	obj := make(map[string]jsonVarbindEntry)
	seenKeys := make(map[string]int)

	for _, vb := range entry.Varbinds {
		key := vb.Name
		if key == "" {
			key = vb.OID
		}
		if key == "" {
			continue
		}
		seenKeys[key]++
		if seenKeys[key] > 1 {
			key = fmt.Sprintf("%s#%d", key, seenKeys[key])
		}

		je := jsonVarbindEntry{
			OID:  vb.OID,
			Type: string(vb.Type),
		}
		je.Value = canonicalVarbindValue(vb.Value)
		if vb.Enum != "" {
			je.Enum = vb.Enum
		}
		obj[key] = je
	}

	if entry.SummaryCounts != nil {
		obj2 := make(map[string]any)
		obj2["total_suppressed"] = entry.SummaryCounts.TotalSuppressed
		obj2["period_sec"] = entry.SummaryCounts.PeriodSec
		obj2["fingerprints"] = entry.SummaryCounts.Fingerprints
		if entry.SummaryCounts.ByTrap != nil {
			bt := make(map[string]int64, len(entry.SummaryCounts.ByTrap))
			for oid, count := range entry.SummaryCounts.ByTrap {
				bt[oid] = count
			}
			obj2["by_trap"] = bt
		}
		result, err := json.Marshal(obj2)
		if err != nil {
			return nil, err
		}
		return result, nil
	}

	sortedKeys := make([]string, 0, len(obj))
	for k := range obj {
		sortedKeys = append(sortedKeys, k)
	}
	sort.Strings(sortedKeys)

	ordered := make([]jsonMapEntry, 0, len(obj))
	for _, k := range sortedKeys {
		ordered = append(ordered, jsonMapEntry{Key: k, Value: obj[k]})
	}

	result, err := marshalOrderedJSON(ordered)
	if err != nil {
		return nil, err
	}
	return result, nil
}

type jsonVarbindEntry struct {
	OID   string `json:"oid"`
	Type  string `json:"type"`
	Value any    `json:"value"`
	Enum  string `json:"enum,omitempty"`
}

type jsonMapEntry struct {
	Key   string
	Value any
}

func marshalOrderedJSON(entries []jsonMapEntry) ([]byte, error) {
	var buf strings.Builder
	buf.WriteByte('{')
	for i, e := range entries {
		if i > 0 {
			buf.WriteByte(',')
		}
		keyBytes, err := json.Marshal(e.Key)
		if err != nil {
			return nil, err
		}
		valBytes, err := json.Marshal(e.Value)
		if err != nil {
			return nil, err
		}
		buf.Write(keyBytes)
		buf.WriteByte(':')
		buf.Write(valBytes)
	}
	buf.WriteByte('}')
	return []byte(buf.String()), nil
}

func canonicalVarbindValue(val any) any {
	if val == nil {
		return nil
	}
	switch v := val.(type) {
	case string:
		return v
	case int64:
		return v
	case uint64:
		return v
	case float64:
		return v
	case bool:
		return v
	case []byte:
		return fmt.Sprintf("%x", v)
	default:
		return fmt.Sprintf("%v", v)
	}
}

func sortedMapKeys(m map[string]string) []string {
	keys := make([]string, 0, len(m))
	for k := range m {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	return keys
}
