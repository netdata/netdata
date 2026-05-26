// SPDX-License-Identifier: GPL-3.0-or-later

package snmp_traps

import (
	"encoding/json"
	"strings"
	"testing"
)

func TestSerializeToJournalFields(t *testing.T) {
	entry := &TrapEntry{
		JobName:               "local",
		ReportType:            ReportTypeTrap,
		ReceivedRealtimeUsec:  1000000,
		ReceivedMonotonicUsec: 1000,
		TrapOID:               "1.3.6.1.6.3.1.1.5.3",
		TrapName:              "IF-MIB::linkDown",
		Category:              "state_change",
		Severity:              "warning",
		Message:               "linkDown on interface eth0",
		SourceIP:              "10.0.0.1",
		SourceUDPPeer:         "10.0.0.1",
		DeviceHostname:        "core-sw-01",
		DeviceVendor:          "cisco",
		PduType:               PduTypeTrap,
		SnmpVersion:           SnmpVersionV2c,
		Labels:                map[string]string{"interface": "eth0", "vlan": "10"},
		Varbinds: []VarbindValue{
			{OID: "1.3.6.1.2.1.2.2.1.1", Name: "ifIndex", Type: "INTEGER", Value: int64(1)},
			{OID: "1.3.6.1.2.1.2.2.1.2", Name: "ifDescr", Type: "OctetString", Value: "eth0"},
		},
	}

	fields, err := serializeToJournalFields(entry)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	fieldMap := make(map[string]string, len(fields))
	for _, f := range fields {
		fieldMap[f.Name] = string(f.Value)
	}

	assertField(t, fieldMap, "MESSAGE", "linkDown on interface eth0")
	assertField(t, fieldMap, "PRIORITY", "4")
	assertField(t, fieldMap, "SYSLOG_IDENTIFIER", "local")
	assertField(t, fieldMap, "_HOSTNAME", "core-sw-01")
	assertField(t, fieldMap, "ND_LOG_SOURCE", "snmp-trap")
	assertField(t, fieldMap, "TRAP_REPORT_TYPE", "trap")
	assertField(t, fieldMap, "TRAP_OID", "1.3.6.1.6.3.1.1.5.3")
	assertField(t, fieldMap, "TRAP_NAME", "IF-MIB::linkDown")
	assertField(t, fieldMap, "TRAP_CATEGORY", "state_change")
	assertField(t, fieldMap, "TRAP_SEVERITY", "warning")
	assertField(t, fieldMap, "TRAP_PDU_TYPE", "trap")
	assertField(t, fieldMap, "TRAP_VERSION", "v2c")
	assertField(t, fieldMap, "TRAP_SOURCE_IP", "10.0.0.1")
	assertField(t, fieldMap, "TRAP_SOURCE_UDP_PEER", "10.0.0.1")
	assertField(t, fieldMap, "TRAP_DEVICE_VENDOR", "cisco")
	assertField(t, fieldMap, "TRAP_TAG_INTERFACE", "eth0")
	assertField(t, fieldMap, "TRAP_TAG_VLAN", "10")

	if fieldMap["TRAP_JSON"] == "" {
		t.Fatal("TRAP_JSON is empty")
	}
}

func TestSerializeToJournalFieldsNoHostname(t *testing.T) {
	entry := &TrapEntry{
		JobName:               "local",
		ReportType:            ReportTypeTrap,
		ReceivedRealtimeUsec:  1000000,
		ReceivedMonotonicUsec: 1000,
		TrapOID:               "1.3.6.1.6.3.1.1.5.3",
		Message:               "test",
		SourceIP:              "10.0.0.1",
		PduType:               PduTypeTrap,
		SnmpVersion:           SnmpVersionV2c,
		Varbinds:              []VarbindValue{},
	}

	fields, err := serializeToJournalFields(entry)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	fieldMap := fieldsToMap(fields)
	assertField(t, fieldMap, "_HOSTNAME", "10.0.0.1")
}

func TestSerializeToJournalFieldsDedupSummary(t *testing.T) {
	entry := &TrapEntry{
		JobName:               "local",
		ReportType:            ReportTypeDedupSummary,
		ReceivedRealtimeUsec:  1000000,
		ReceivedMonotonicUsec: 1000,
		Message:               "DEDUPLICATED TRAPS: 5 suppressed",
		SourceIP:              "10.0.0.1",
		DeviceHostname:        "core-sw-01",
		SourceVnodeID:         "source-vnode-id",
		SummaryCounts: &DedupSummary{
			TotalSuppressed: 5,
			PeriodSec:       10,
			Fingerprints:    3,
			ByTrap:          map[string]int64{"1.3.6.1.6.3.1.1.5.3": 3, "1.3.6.1.6.3.1.1.5.5": 2},
		},
		Severity: "info",
	}

	fields, err := serializeToJournalFields(entry)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	fieldMap := fieldsToMap(fields)
	assertField(t, fieldMap, "TRAP_REPORT_TYPE", "deduplication_summary")
	assertField(t, fieldMap, "TRAP_SUPPRESSED_COUNT", "5")
	assertField(t, fieldMap, "TRAP_SUPPRESSED_FINGERPRINTS", "3")
	assertField(t, fieldMap, "TRAP_REPORT_PERIOD_SEC", "10")
	assertFieldAbsent(t, fieldMap, "_HOSTNAME")
	assertFieldAbsent(t, fieldMap, "ND_NIDL_NODE")

	var summaryMap map[string]any
	if err := json.Unmarshal([]byte(fieldMap["TRAP_JSON"]), &summaryMap); err != nil {
		t.Fatalf("TRAP_JSON not valid: %v", err)
	}
	if ts, ok := summaryMap["total_suppressed"].(float64); !ok || int64(ts) != 5 {
		t.Fatalf("expected total_suppressed=5, got %v", summaryMap["total_suppressed"])
	}
}

func TestSerializeToJournalFieldsSeverityMapping(t *testing.T) {
	tests := map[string]struct {
		severity Severity
		priority string
	}{
		"emerg":   {"emerg", "0"},
		"alert":   {"alert", "1"},
		"crit":    {"crit", "2"},
		"err":     {"err", "3"},
		"warning": {"warning", "4"},
		"notice":  {"notice", "5"},
		"info":    {"info", "6"},
		"debug":   {"debug", "7"},
		"unknown": {"", "5"},
		"invalid": {"invalid-slug", "5"},
	}

	for name, tc := range tests {
		t.Run(name, func(t *testing.T) {
			pri := severityPriority(tc.severity)
			if pri != tc.priority {
				t.Fatalf("expected priority %s, got %s", tc.priority, pri)
			}
		})
	}
}

func TestSerializeToJournalFieldsNilEntry(t *testing.T) {
	_, err := serializeToJournalFields(nil)
	if err == nil {
		t.Fatal("expected error for nil entry")
	}
}

func TestSerializeToJournalFieldsMissingJobName(t *testing.T) {
	entry := &TrapEntry{
		TrapOID:  "1.3.6.1.6.3.1.1.5.3",
		SourceIP: "10.0.0.1",
	}
	_, err := serializeToJournalFields(entry)
	if err == nil {
		t.Fatal("expected error for missing job name")
	}
}

func TestSerializeToJournalFieldsMissingSourceIP(t *testing.T) {
	entry := &TrapEntry{
		JobName:        "local",
		TrapOID:        "1.3.6.1.6.3.1.1.5.3",
		SourceIP:       "",
		SourceUDPPeer:  "",
		DeviceHostname: "",
	}
	_, err := serializeToJournalFields(entry)
	if err == nil {
		t.Fatal("expected error for missing source IP")
	}
}

func TestSerializeToJournalFieldsNegativeTimestamp(t *testing.T) {
	entry := &TrapEntry{
		JobName:               "local",
		TrapOID:               "1.3.6.1.6.3.1.1.5.3",
		SourceIP:              "10.0.0.1",
		ReceivedRealtimeUsec:  -1,
		ReceivedMonotonicUsec: 0,
	}
	_, err := serializeToJournalFields(entry)
	if err == nil {
		t.Fatal("expected error for negative timestamp")
	}
}

func TestSerializeToJournalFieldsTRAPTagLabels(t *testing.T) {
	entry := &TrapEntry{
		JobName:               "local",
		ReportType:            ReportTypeTrap,
		ReceivedRealtimeUsec:  1000000,
		ReceivedMonotonicUsec: 1000,
		TrapOID:               "1.3.6.1.6.3.1.1.5.3",
		Message:               "test",
		SourceIP:              "10.0.0.1",
		PduType:               PduTypeTrap,
		SnmpVersion:           SnmpVersionV2c,
		Labels:                map[string]string{"compliance": "pci", "tenant": "acme"},
		Varbinds:              []VarbindValue{},
	}

	fields, err := serializeToJournalFields(entry)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	fieldMap := fieldsToMap(fields)
	assertField(t, fieldMap, "TRAP_TAG_COMPLIANCE", "pci")
	assertField(t, fieldMap, "TRAP_TAG_TENANT", "acme")
}

func TestSerializeToJournalFieldsTRAPJSONShape(t *testing.T) {
	entry := &TrapEntry{
		JobName:               "local",
		ReportType:            ReportTypeTrap,
		ReceivedRealtimeUsec:  1000000,
		ReceivedMonotonicUsec: 1000,
		TrapOID:               "1.3.6.1.6.3.1.1.5.3",
		Message:               "test",
		SourceIP:              "10.0.0.1",
		PduType:               PduTypeTrap,
		SnmpVersion:           SnmpVersionV2c,
		Varbinds: []VarbindValue{
			{OID: "1.3.6.1.2.1.2.2.1.1", Name: "ifIndex", Type: "INTEGER", Value: int64(1)},
			{OID: "1.3.6.1.2.1.2.2.1.2", Name: "ifDescr", Type: "OctetString", Value: "eth0"},
		},
	}

	fields, err := serializeToJournalFields(entry)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	fieldMap := fieldsToMap(fields)

	var obj map[string]any
	if err := json.Unmarshal([]byte(fieldMap["TRAP_JSON"]), &obj); err != nil {
		t.Fatalf("TRAP_JSON not valid: %v", err)
	}

	ifIdx, ok := obj["ifIndex"].(map[string]any)
	if !ok {
		t.Fatal("ifIndex key not found in TRAP_JSON")
	}
	if ifIdx["oid"] != "1.3.6.1.2.1.2.2.1.1" {
		t.Fatalf("expected oid in ifIndex entry")
	}
	if ifIdx["type"] != "INTEGER" {
		t.Fatalf("expected type INTEGER in ifIndex entry")
	}
}

func TestSerializeToJournalFieldsDuplicateJSONKeys(t *testing.T) {
	entry := &TrapEntry{
		JobName:               "local",
		ReportType:            ReportTypeTrap,
		ReceivedRealtimeUsec:  1000000,
		ReceivedMonotonicUsec: 1000,
		TrapOID:               "1.3.6.1.6.3.1.1.5.3",
		Message:               "test",
		SourceIP:              "10.0.0.1",
		PduType:               PduTypeTrap,
		SnmpVersion:           SnmpVersionV2c,
		Varbinds: []VarbindValue{
			{OID: "1.3.6.1.2.1.2.2.1.1", Name: "ifIndex", Type: "INTEGER", Value: int64(1)},
			{OID: "1.3.6.1.2.1.2.2.1.1", Name: "ifIndex", Type: "INTEGER", Value: int64(2)},
		},
	}

	fields, err := serializeToJournalFields(entry)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	fieldMap := fieldsToMap(fields)

	var obj map[string]any
	if err := json.Unmarshal([]byte(fieldMap["TRAP_JSON"]), &obj); err != nil {
		t.Fatalf("TRAP_JSON not valid: %v", err)
	}

	if _, ok := obj["ifIndex"]; !ok {
		t.Fatal("ifIndex key not found")
	}
	if _, ok := obj["ifIndex#2"]; !ok {
		t.Fatal("ifIndex#2 key not found (duplicate key should be suffixed)")
	}
}

func TestSerializeToJournalFieldsLabelsSorted(t *testing.T) {
	entry := &TrapEntry{
		JobName:               "local",
		ReportType:            ReportTypeTrap,
		ReceivedRealtimeUsec:  1000000,
		ReceivedMonotonicUsec: 1000,
		TrapOID:               "1.3.6.1.6.3.1.1.5.3",
		Message:               "test",
		SourceIP:              "10.0.0.1",
		PduType:               PduTypeTrap,
		SnmpVersion:           SnmpVersionV2c,
		Labels:                map[string]string{"z_key": "z_val", "a_key": "a_val"},
		Varbinds:              []VarbindValue{},
	}

	fields, err := serializeToJournalFields(entry)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	var labelNames []string
	for _, f := range fields {
		if strings.HasPrefix(f.Name, "TRAP_TAG_") {
			labelNames = append(labelNames, f.Name)
		}
	}

	if len(labelNames) < 2 {
		t.Fatal("expected at least 2 label fields")
	}
	if labelNames[0] != "TRAP_TAG_A_KEY" {
		t.Fatalf("expected TRAP_TAG_A_KEY first, got %s", labelNames[0])
	}
}

func assertField(t *testing.T, fieldMap map[string]string, name, expected string) {
	t.Helper()
	if got, ok := fieldMap[name]; !ok {
		t.Fatalf("missing field %q", name)
	} else if got != expected {
		t.Fatalf("field %q: expected %q, got %q", name, expected, got)
	}
}

func assertFieldAbsent(t *testing.T, fieldMap map[string]string, name string) {
	t.Helper()
	if got, ok := fieldMap[name]; ok {
		t.Fatalf("field %q unexpectedly present with value %q", name, got)
	}
}

func fieldsToMap(fields []JournalField) map[string]string {
	m := make(map[string]string, len(fields))
	for _, f := range fields {
		m[f.Name] = string(f.Value)
	}
	return m
}
