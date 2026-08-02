// SPDX-License-Identifier: GPL-3.0-or-later

package journal

import (
	"bytes"
	"encoding/json"
	"strings"
	"testing"

	"github.com/netdata/netdata/go/plugins/plugin/go.d/collector/snmp_traps/internal/model"
)

type decodedField struct {
	Name  string
	Value []byte
}

func serializeHotFields(entry *model.TrapEntry) ([]decodedField, error) {
	var serializer hotSerializer
	payloads, _, err := serializer.serialize(entry)
	if err != nil {
		return nil, err
	}
	fields := make([]decodedField, 0, len(payloads))
	for _, payload := range payloads {
		name, value, ok := bytes.Cut(payload, []byte{'='})
		if ok {
			fields = append(fields, decodedField{Name: string(name), Value: value})
		}
	}
	return fields, nil
}

func TestHotSerializerExactRawPayloads(t *testing.T) {
	entry := &model.TrapEntry{
		JobName:               "local",
		ReportType:            model.ReportTypeTrap,
		ReceivedRealtimeUsec:  1000000,
		ReceivedMonotonicUsec: 1000,
		TrapOID:               "1.3.6.1.6.3.1.1.5.3",
		Category:              "state_change",
		Severity:              "warning",
		Message:               "trap",
		SourceIP:              "192.0.2.1",
		SourceUDPPeer:         "192.0.2.1:162",
		PduType:               model.PduTypeTrap,
		SnmpVersion:           model.SnmpVersionV2c,
	}
	var serializer hotSerializer
	payloads, binaryFields, err := serializer.serialize(entry)
	if err != nil {
		t.Fatal(err)
	}
	want := []string{
		"MESSAGE=trap",
		"PRIORITY=4",
		"SYSLOG_IDENTIFIER=local",
		"TRAP_JOB=local",
		"_HOSTNAME=192.0.2.1",
		"ND_LOG_SOURCE=snmp-trap",
		"TRAP_REPORT_TYPE=trap",
		"TRAP_OID=1.3.6.1.6.3.1.1.5.3",
		"TRAP_CATEGORY=state_change",
		"TRAP_SEVERITY=warning",
		"TRAP_PDU_TYPE=trap",
		"TRAP_VERSION=v2c",
		"TRAP_SOURCE_IP=192.0.2.1",
		"TRAP_SOURCE_UDP_PEER=192.0.2.1:162",
		"TRAP_JSON={}",
	}
	if len(payloads) != len(want) {
		t.Fatalf("payload count = %d, want %d: %q", len(payloads), len(want), payloads)
	}
	for i := range want {
		if got := string(payloads[i]); got != want[i] {
			t.Fatalf("payload %d = %q, want %q", i, got, want[i])
		}
	}
	if binaryFields != 0 {
		t.Fatalf("binary fields = %d, want 0", binaryFields)
	}
}

func TestHotSerializer(t *testing.T) {
	entry := &model.TrapEntry{
		JobName:               "local",
		ReportType:            model.ReportTypeTrap,
		ReceivedRealtimeUsec:  1000000,
		ReceivedMonotonicUsec: 1000,
		TrapOID:               "1.3.6.1.6.3.1.1.5.3",
		TrapName:              "IF-MIB::linkDown",
		Category:              "state_change",
		Severity:              "warning",
		Message:               "linkDown on interface eth0",
		SourceIP:              "10.0.0.1",
		SourceUDPPeer:         "10.0.0.1",
		ReverseDNS:            "core-sw.ptr.example.com",
		DeviceHostname:        "core-sw-01",
		DeviceVendor:          "cisco",
		PduType:               model.PduTypeTrap,
		SnmpVersion:           model.SnmpVersionV2c,
		Labels:                map[string]string{"interface": "eth0", "vlan": "10"},
		Enrichment: &model.TrapEnrichmentAudit{
			Source: &model.TrapSourceAudit{
				UDPPeer:            "10.0.0.1",
				SnmpTrapAddress:    "192.0.2.1",
				Selected:           "10.0.0.1",
				Method:             "udp_peer",
				RejectedCandidates: []string{"snmpTrapAddress.0:untrusted_relay_uses_udp_peer"},
			},
			Registry: &model.TrapEnrichmentLookup{
				Key:     "10.0.0.1",
				Status:  "matched",
				Method:  "hostname_or_ip",
				Matches: 1,
				Fields:  []string{"_HOSTNAME", "TRAP_DEVICE_VENDOR"},
			},
			Applied: map[string]string{
				"_HOSTNAME":          "core-sw-01",
				"TRAP_DEVICE_VENDOR": "cisco",
			},
		},
		Varbinds: []model.VarbindValue{
			{OID: "1.3.6.1.2.1.2.2.1.1", Name: "ifIndex", Type: "INTEGER", Value: int64(1)},
			{OID: "1.3.6.1.2.1.2.2.1.2", Name: "ifDescr", Type: "OctetString", Value: "eth0"},
		},
	}

	fields, err := serializeHotFields(entry)
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
	assertField(t, fieldMap, "TRAP_JOB", "local")
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
	assertField(t, fieldMap, "TRAP_REVERSE_DNS", "core-sw.ptr.example.com")
	assertField(t, fieldMap, "TRAP_DEVICE_VENDOR", "cisco")
	assertField(t, fieldMap, "TRAP_TAG_INTERFACE", "eth0")
	assertField(t, fieldMap, "TRAP_TAG_VLAN", "10")
	assertField(t, fieldMap, "TRAP_VAR_IFINDEX", "1")
	assertField(t, fieldMap, "TRAP_VAR_IFDESCR", "eth0")

	if fieldMap["TRAP_JSON"] == "" {
		t.Fatal("TRAP_JSON is empty")
	}
	if fieldMap["TRAP_ENRICHMENT"] == "" {
		t.Fatal("TRAP_ENRICHMENT is empty")
	}
	assertFieldOrder(t, journalFieldNames(fields), "TRAP_TAG_VLAN", "TRAP_VAR_IFINDEX", "TRAP_ENRICHMENT", "TRAP_JSON")
}

func TestJournalSerializersAppendLargeJSONFieldsLast(t *testing.T) {
	entry := &model.TrapEntry{
		JobName:               "local",
		ReportType:            model.ReportTypeTrap,
		ReceivedRealtimeUsec:  1000000,
		ReceivedMonotonicUsec: 1000,
		TrapOID:               "1.3.6.1.6.3.1.1.5.3",
		Message:               "test",
		SourceIP:              "10.0.0.1",
		PduType:               model.PduTypeTrap,
		SnmpVersion:           model.SnmpVersionV2c,
		Labels:                map[string]string{"site": "lab"},
		Enrichment: &model.TrapEnrichmentAudit{
			Source: &model.TrapSourceAudit{Selected: "10.0.0.1", Method: "udp_peer"},
		},
		Varbinds: []model.VarbindValue{
			{OID: "1.3.6.1.2.1.2.2.1.1", Name: "ifIndex", Type: "INTEGER", Value: int64(1)},
		},
	}

	fields, err := serializeHotFields(entry)
	if err != nil {
		t.Fatalf("serializeHotFields: %v", err)
	}
	assertFieldOrder(t, journalFieldNames(fields), "TRAP_TAG_SITE", "TRAP_VAR_IFINDEX", "TRAP_ENRICHMENT", "TRAP_JSON")

	var s hotSerializer
	payloads, _, err := s.serialize(entry)
	if err != nil {
		t.Fatalf("hotSerializer.serialize: %v", err)
	}
	assertFieldOrder(t, payloadFieldNames(payloads), "TRAP_TAG_SITE", "TRAP_VAR_IFINDEX", "TRAP_ENRICHMENT", "TRAP_JSON")
}

func TestHotSerializerNoHostname(t *testing.T) {
	entry := &model.TrapEntry{
		JobName:               "local",
		ReportType:            model.ReportTypeTrap,
		ReceivedRealtimeUsec:  1000000,
		ReceivedMonotonicUsec: 1000,
		TrapOID:               "1.3.6.1.6.3.1.1.5.3",
		Message:               "test",
		SourceIP:              "10.0.0.1",
		PduType:               model.PduTypeTrap,
		SnmpVersion:           model.SnmpVersionV2c,
		Varbinds:              []model.VarbindValue{},
	}

	fields, err := serializeHotFields(entry)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	fieldMap := fieldsToMap(fields)
	assertField(t, fieldMap, "_HOSTNAME", "10.0.0.1")
}

func TestHotSerializerDedupSummary(t *testing.T) {
	entry := &model.TrapEntry{
		JobName:               "local",
		ReportType:            model.ReportTypeDedupSummary,
		ReceivedRealtimeUsec:  1000000,
		ReceivedMonotonicUsec: 1000,
		Message:               "DEDUPLICATED TRAPS: 5 suppressed",
		SourceIP:              "10.0.0.1",
		DeviceHostname:        "core-sw-01",
		SourceVnodeID:         "source-vnode-id",
		SummaryCounts: &model.DedupSummary{
			TotalSuppressed: 5,
			PeriodSec:       10,
			Fingerprints:    3,
			ByTrap:          map[string]int64{"1.3.6.1.6.3.1.1.5.3": 3, "1.3.6.1.6.3.1.1.5.5": 2},
		},
		Severity: "info",
	}

	fields, err := serializeHotFields(entry)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	fieldMap := fieldsToMap(fields)
	assertField(t, fieldMap, "TRAP_JOB", "local")
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

func TestHotSerializerDecodeError(t *testing.T) {
	entry := &model.TrapEntry{
		JobName:               "local",
		ReportType:            model.ReportTypeDecodeError,
		ReceivedRealtimeUsec:  1000000,
		ReceivedMonotonicUsec: 1000,
		Category:              "diagnostic",
		Severity:              "warning",
		Message:               "SNMP trap decode failed from 10.0.0.1: malformed_pdu: BER: trailing data",
		SourceIP:              "10.0.0.1",
		SourceUDPPeer:         "10.0.0.1",
		SnmpVersion:           model.SnmpVersionV2c,
		PacketSequence:        7,
		DecodeError: &model.DecodeErrorInfo{
			Kind:          "malformed_pdu",
			Error:         "BER: trailing data",
			PacketSize:    42,
			PacketSHA256:  strings.Repeat("a", 64),
			SourceUDPPort: 9162,
			Listener:      "0.0.0.0:162",
			EngineID:      "8000000001020304",
		},
	}

	fields, err := serializeHotFields(entry)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	fieldMap := fieldsToMap(fields)

	assertField(t, fieldMap, "TRAP_JOB", "local")
	assertField(t, fieldMap, "TRAP_REPORT_TYPE", "decode_error")
	assertField(t, fieldMap, "TRAP_CATEGORY", "diagnostic")
	assertField(t, fieldMap, "TRAP_SEVERITY", "warning")
	assertField(t, fieldMap, "TRAP_VERSION", "v2c")
	assertField(t, fieldMap, "TRAP_SOURCE_IP", "10.0.0.1")
	assertField(t, fieldMap, "TRAP_SOURCE_UDP_PEER", "10.0.0.1")
	assertField(t, fieldMap, "TRAP_SOURCE_UDP_PORT", "9162")
	assertField(t, fieldMap, "TRAP_DECODE_ERROR_KIND", "malformed_pdu")
	assertField(t, fieldMap, "TRAP_DECODE_ERROR", "BER: trailing data")
	assertField(t, fieldMap, "TRAP_PACKET_SIZE", "42")
	assertField(t, fieldMap, "TRAP_PACKET_SHA256", strings.Repeat("a", 64))
	assertField(t, fieldMap, "TRAP_LISTENER", "0.0.0.0:162")
	assertField(t, fieldMap, "TRAP_ENGINE_ID", "8000000001020304")
	assertFieldAbsent(t, fieldMap, "TRAP_OID")
	assertFieldAbsent(t, fieldMap, "TRAP_NAME")

	var details map[string]any
	if err := json.Unmarshal([]byte(fieldMap["TRAP_JSON"]), &details); err != nil {
		t.Fatalf("TRAP_JSON not valid: %v", err)
	}
	if got := details["kind"]; got != "malformed_pdu" {
		t.Fatalf("TRAP_JSON kind = %v, want malformed_pdu", got)
	}
	if got := details["packet_sha256"]; got != strings.Repeat("a", 64) {
		t.Fatalf("TRAP_JSON packet_sha256 = %v", got)
	}
	if got := details["netdata_packet_sequence"]; got != float64(7) {
		t.Fatalf("TRAP_JSON netdata_packet_sequence = %v, want 7", got)
	}
}

func TestHotSerializerSeverityMapping(t *testing.T) {
	tests := map[string]struct {
		severity model.Severity
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

func TestHotSerializerNilEntry(t *testing.T) {
	_, err := serializeHotFields(nil)
	if err == nil {
		t.Fatal("expected error for nil entry")
	}
}

func TestHotSerializerMissingJobName(t *testing.T) {
	entry := &model.TrapEntry{
		TrapOID:  "1.3.6.1.6.3.1.1.5.3",
		SourceIP: "10.0.0.1",
	}
	_, err := serializeHotFields(entry)
	if err == nil {
		t.Fatal("expected error for missing job name")
	}
}

func TestHotSerializerMissingSourceIP(t *testing.T) {
	entry := &model.TrapEntry{
		JobName:        "local",
		TrapOID:        "1.3.6.1.6.3.1.1.5.3",
		SourceIP:       "",
		SourceUDPPeer:  "",
		DeviceHostname: "",
	}
	_, err := serializeHotFields(entry)
	if err == nil {
		t.Fatal("expected error for missing source IP")
	}
}

func TestHotSerializerNegativeTimestamp(t *testing.T) {
	entry := &model.TrapEntry{
		JobName:               "local",
		TrapOID:               "1.3.6.1.6.3.1.1.5.3",
		SourceIP:              "10.0.0.1",
		ReceivedRealtimeUsec:  -1,
		ReceivedMonotonicUsec: 0,
	}
	_, err := serializeHotFields(entry)
	if err == nil {
		t.Fatal("expected error for negative timestamp")
	}
}

func TestHotSerializerTRAPTagLabels(t *testing.T) {
	entry := &model.TrapEntry{
		JobName:               "local",
		ReportType:            model.ReportTypeTrap,
		ReceivedRealtimeUsec:  1000000,
		ReceivedMonotonicUsec: 1000,
		TrapOID:               "1.3.6.1.6.3.1.1.5.3",
		Message:               "test",
		SourceIP:              "10.0.0.1",
		PduType:               model.PduTypeTrap,
		SnmpVersion:           model.SnmpVersionV2c,
		Labels:                map[string]string{"compliance": "pci", "tenant": "acme"},
		Varbinds:              []model.VarbindValue{},
	}

	fields, err := serializeHotFields(entry)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	fieldMap := fieldsToMap(fields)
	assertField(t, fieldMap, "TRAP_TAG_COMPLIANCE", "pci")
	assertField(t, fieldMap, "TRAP_TAG_TENANT", "acme")
}

func TestHotSerializerTRAPJSONShape(t *testing.T) {
	entry := &model.TrapEntry{
		JobName:               "local",
		ReportType:            model.ReportTypeTrap,
		ReceivedRealtimeUsec:  1000000,
		ReceivedMonotonicUsec: 1000,
		TrapOID:               "1.3.6.1.6.3.1.1.5.3",
		Message:               "test",
		SourceIP:              "10.0.0.1",
		PduType:               model.PduTypeTrap,
		SnmpVersion:           model.SnmpVersionV2c,
		PacketSequence:        42,
		Varbinds: []model.VarbindValue{
			{OID: "1.3.6.1.2.1.2.2.1.1", Name: "ifIndex", Type: "INTEGER", Value: int64(1)},
			{OID: "1.3.6.1.2.1.2.2.1.2", Name: "ifDescr", Type: "OctetString", Value: "eth0"},
		},
	}

	fields, err := serializeHotFields(entry)
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
	if got := obj["netdata_packet_sequence"]; got != float64(42) {
		t.Fatalf("TRAP_JSON netdata_packet_sequence = %v, want 42", got)
	}
}

func TestHotSerializerTRAPJSONSequenceKeyCollision(t *testing.T) {
	entry := &model.TrapEntry{
		JobName:               "local",
		ReportType:            model.ReportTypeTrap,
		ReceivedRealtimeUsec:  1000000,
		ReceivedMonotonicUsec: 1000,
		TrapOID:               "1.3.6.1.6.3.1.1.5.3",
		Message:               "test",
		SourceIP:              "10.0.0.1",
		PduType:               model.PduTypeTrap,
		SnmpVersion:           model.SnmpVersionV2c,
		PacketSequence:        42,
		Varbinds: []model.VarbindValue{
			{OID: "1.3.6.1.2.1.2.2.1.1", Name: trapJSONPacketSequenceKey, Type: "INTEGER", Value: int64(999)},
		},
	}

	fields, err := serializeHotFields(entry)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	fieldMap := fieldsToMap(fields)

	var obj map[string]any
	if err := json.Unmarshal([]byte(fieldMap["TRAP_JSON"]), &obj); err != nil {
		t.Fatalf("TRAP_JSON not valid: %v", err)
	}

	if got := obj["netdata_packet_sequence"]; got != float64(42) {
		t.Fatalf("TRAP_JSON netdata_packet_sequence = %v, want 42", got)
	}
	if _, ok := obj["netdata_packet_sequence#2"]; !ok {
		t.Fatal("TRAP_JSON did not suffix colliding varbind key")
	}
}

func TestHotSerializerTRAPJSONOmitsCommunityVarbind(t *testing.T) {
	entry := &model.TrapEntry{
		JobName:               "local",
		ReportType:            model.ReportTypeTrap,
		ReceivedRealtimeUsec:  1000000,
		ReceivedMonotonicUsec: 1000,
		TrapOID:               "1.3.6.1.6.3.1.1.5.3",
		Message:               "test",
		SourceIP:              "10.0.0.1",
		PduType:               model.PduTypeTrap,
		SnmpVersion:           model.SnmpVersionV1,
		Varbinds: []model.VarbindValue{
			{OID: model.SNMPTrapCommunityOID, Name: "snmpTrapCommunity.0", Type: "OctetString", Value: "private-community"},
			{OID: "1.3.6.1.2.1.2.2.1.1", Name: "ifIndex", Type: "INTEGER", Value: int64(1)},
		},
	}

	fields, err := serializeHotFields(entry)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	fieldMap := fieldsToMap(fields)

	if strings.Contains(fieldMap["TRAP_JSON"], "private-community") {
		t.Fatalf("TRAP_JSON leaked SNMP community: %s", fieldMap["TRAP_JSON"])
	}

	var obj map[string]any
	if err := json.Unmarshal([]byte(fieldMap["TRAP_JSON"]), &obj); err != nil {
		t.Fatalf("TRAP_JSON not valid: %v", err)
	}
	if _, ok := obj["snmpTrapCommunity.0"]; ok {
		t.Fatalf("TRAP_JSON includes sensitive community varbind: %v", obj["snmpTrapCommunity.0"])
	}
	if _, ok := obj["ifIndex"]; !ok {
		t.Fatal("TRAP_JSON dropped non-sensitive varbind")
	}
}

func TestHotSerializerTrapVarbindJournalFields(t *testing.T) {
	entry := &model.TrapEntry{
		JobName:               "local",
		ReportType:            model.ReportTypeTrap,
		ReceivedRealtimeUsec:  1000000,
		ReceivedMonotonicUsec: 1000,
		TrapOID:               "1.3.6.1.6.3.1.1.5.3",
		Message:               "test",
		SourceIP:              "10.0.0.1",
		PduType:               model.PduTypeTrap,
		SnmpVersion:           model.SnmpVersionV1,
		Varbinds: []model.VarbindValue{
			{OID: model.SysUpTimeOID, Name: "sysUpTime.0", Type: "TimeTicks", Value: uint64(129665677)},
			{OID: model.SNMPTrapOID, Name: "snmpTrapOID.0", Type: "ObjectIdentifier", Value: "1.3.6.1.6.3.1.1.5.3"},
			{OID: model.SNMPTrapAddressOID, Name: "snmpTrapAddress.0", Type: "IPAddress", Value: "0.0.0.0"},
			{OID: model.SNMPTrapEnterpriseOID, Name: "snmpTrapEnterprise.0", Type: "ObjectIdentifier", Value: "1.3.6.1.6.3.1.1.5.3"},
			{OID: model.SNMPTrapCommunityOID, Name: "snmpTrapCommunity.0", Type: "OctetString", Value: "private-community"},
			{OID: "1.3.6.1.2.1.2.2.1.7.29", Name: "ifAdminStatus", Type: "INTEGER", Value: int64(1), Enum: "up"},
			{OID: "1.3.6.1.2.1.2.2.1.1.29", Name: "ifIndex", Type: "InterfaceIndex", Value: int64(29)},
			{OID: "1.3.6.1.2.1.2.2.1.8.29", Name: "ifOperStatus", Type: "INTEGER", Value: int64(2), Enum: "down"},
		},
	}

	fields, err := serializeHotFields(entry)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	fieldMap := fieldsToMap(fields)

	assertField(t, fieldMap, "TRAP_VAR_IFADMINSTATUS", "up")
	assertField(t, fieldMap, "TRAP_VAR_IFADMINSTATUS_RAW", "1")
	assertField(t, fieldMap, "TRAP_VAR_IFINDEX", "29")
	assertField(t, fieldMap, "TRAP_VAR_IFOPERSTATUS", "down")
	assertField(t, fieldMap, "TRAP_VAR_IFOPERSTATUS_RAW", "2")
	assertFieldAbsent(t, fieldMap, "TRAP_VAR_SYSUPTIME_0")
	assertFieldAbsent(t, fieldMap, "TRAP_VAR_SNMPTRAPOID_0")
	assertFieldAbsent(t, fieldMap, "TRAP_VAR_SNMPTRAPADDRESS_0")
	assertFieldAbsent(t, fieldMap, "TRAP_VAR_SNMPTRAPENTERPRISE_0")
	assertFieldAbsent(t, fieldMap, "TRAP_VAR_SNMPTRAPCOMMUNITY_0")
}

func TestHotSerializerTrapVarbindJournalFieldNames(t *testing.T) {
	entry := &model.TrapEntry{
		JobName:               "local",
		ReportType:            model.ReportTypeTrap,
		ReceivedRealtimeUsec:  1000000,
		ReceivedMonotonicUsec: 1000,
		TrapOID:               "1.3.6.1.6.3.1.1.5.3",
		Message:               "test",
		SourceIP:              "10.0.0.1",
		PduType:               model.PduTypeTrap,
		SnmpVersion:           model.SnmpVersionV2c,
		Varbinds: []model.VarbindValue{
			{OID: "1.3.6.1.2.1.2.2.1.1", Name: "ifIndex", Type: "INTEGER", Value: int64(1)},
			{OID: "1.3.6.1.2.1.2.2.1.2", Name: "ifIndex", Type: "INTEGER", Value: int64(2)},
			{OID: "1.3.6.1.4.1.999.1", Type: "OctetString", Value: "raw-oid-name"},
			{OID: "1.3.6.1.4.1.999.2", Name: "ifOperStatus_raw", Type: "OctetString", Value: "manual-raw"},
			{OID: "1.3.6.1.2.1.2.2.1.8", Name: "ifOperStatus", Type: "INTEGER", Value: int64(2), Enum: "down"},
			{OID: "1.3.6.1.4.1.999.3", Name: "snmpTrapOID", Type: "OctetString", Value: "vendor-varbind"},
			{Name: "snmpTrapAddress.0", Type: "IPAddress", Value: "192.0.2.1"},
		},
	}

	fields, err := serializeHotFields(entry)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	fieldMap := fieldsToMap(fields)

	assertField(t, fieldMap, "TRAP_VAR_IFINDEX", "1")
	assertField(t, fieldMap, "TRAP_VAR_IFINDEX_2", "2")
	assertField(t, fieldMap, "TRAP_VAR_OID_1_3_6_1_4_1_999_1", "raw-oid-name")
	assertField(t, fieldMap, "TRAP_VAR_IFOPERSTATUS_RAW", "manual-raw")
	assertField(t, fieldMap, "TRAP_VAR_IFOPERSTATUS_2", "down")
	assertField(t, fieldMap, "TRAP_VAR_IFOPERSTATUS_2_RAW", "2")
	assertField(t, fieldMap, "TRAP_VAR_SNMPTRAPOID", "vendor-varbind")
	assertFieldAbsent(t, fieldMap, "TRAP_VAR_SNMPTRAPADDRESS_0")
}

func TestHotSerializerTrapVarbindJournalFieldNamesFitJournaldPolicy(t *testing.T) {
	longOID := "1.3.6.1.4.1.9.9.315.1.2.1.1.1.2.3.4.5.6.7.8.9.10.11.12.13.14.15.16.17.18.19.20"
	longName := "thisIsAnExtremelyLongVendorSpecificVarbindNameThatWouldNotFitInsideAJournaldFieldName"
	entry := &model.TrapEntry{
		JobName:               "local",
		ReportType:            model.ReportTypeTrap,
		ReceivedRealtimeUsec:  1000000,
		ReceivedMonotonicUsec: 1000,
		TrapOID:               "1.3.6.1.6.3.1.1.5.3",
		Message:               "test",
		SourceIP:              "10.0.0.1",
		PduType:               model.PduTypeTrap,
		SnmpVersion:           model.SnmpVersionV2c,
		Varbinds: []model.VarbindValue{
			{OID: longOID, Type: "OctetString", Value: "long-oid"},
			{OID: "1.3.6.1.4.1.999.4", Name: longName, Type: "INTEGER", Value: int64(42), Enum: "meaning"},
		},
	}

	fields, err := serializeHotFields(entry)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	for _, f := range fields {
		if len(f.Name) > maxJournalFieldNameLen {
			t.Fatalf("journal field name %q length = %d, want <= %d", f.Name, len(f.Name), maxJournalFieldNameLen)
		}
	}

	fieldMap := fieldsToMap(fields)
	assertFieldWithPrefix(t, fieldMap, "TRAP_VAR_OID_1_3_6_1_4_1_9_9_315_1_2_1_1_1_", "long-oid")
	assertFieldWithPrefix(t, fieldMap, "TRAP_VAR_THISISANEXTREMELYLONGVENDORSPECIFICVARBIND", "meaning")
	assertFieldWithPrefix(t, fieldMap, "TRAP_VAR_THISISANEXTREMELYLONGVENDORSPECIFICVAR", "42")
}

func TestHotSerializerTRAPJSONUsesProfileNamesForTabularVarbindInstances(t *testing.T) {
	const ifOperStatusOID = "1.3.6.1.2.1.2.2.1.8"
	entry := &model.TrapEntry{
		JobName:               "local",
		ReportType:            model.ReportTypeTrap,
		ReceivedRealtimeUsec:  1000000,
		ReceivedMonotonicUsec: 1000,
		TrapOID:               "1.3.6.1.6.3.1.1.5.3",
		Message:               "test",
		SourceIP:              "10.0.0.1",
		PduType:               model.PduTypeTrap,
		SnmpVersion:           model.SnmpVersionV2c,
		Varbinds: []model.VarbindValue{{
			OID: ifOperStatusOID + ".1", Name: "ifOperStatus", Type: "INTEGER", Value: int64(2), Enum: "down",
		}},
	}

	fields, err := serializeHotFields(entry)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	fieldMap := fieldsToMap(fields)

	var obj map[string]map[string]any
	if err := json.Unmarshal([]byte(fieldMap["TRAP_JSON"]), &obj); err != nil {
		t.Fatalf("TRAP_JSON not valid: %v", err)
	}

	if _, ok := obj[ifOperStatusOID+".1"]; ok {
		t.Fatalf("TRAP_JSON kept raw instance OID key %q instead of profile varbind name", ifOperStatusOID+".1")
	}
	status, ok := obj["ifOperStatus"]
	if !ok {
		t.Fatalf("ifOperStatus key not found in TRAP_JSON: %v", obj)
	}
	if status["oid"] != ifOperStatusOID+".1" {
		t.Fatalf("ifOperStatus oid = %v, want %s", status["oid"], ifOperStatusOID+".1")
	}
	if status["enum"] != "down" {
		t.Fatalf("ifOperStatus enum = %v, want down", status["enum"])
	}
}

func TestHotSerializerDuplicateJSONKeys(t *testing.T) {
	entry := &model.TrapEntry{
		JobName:               "local",
		ReportType:            model.ReportTypeTrap,
		ReceivedRealtimeUsec:  1000000,
		ReceivedMonotonicUsec: 1000,
		TrapOID:               "1.3.6.1.6.3.1.1.5.3",
		Message:               "test",
		SourceIP:              "10.0.0.1",
		PduType:               model.PduTypeTrap,
		SnmpVersion:           model.SnmpVersionV2c,
		Varbinds: []model.VarbindValue{
			{OID: "1.3.6.1.2.1.2.2.1.1", Name: "ifIndex", Type: "INTEGER", Value: int64(1)},
			{OID: "1.3.6.1.2.1.2.2.1.1", Name: "ifIndex", Type: "INTEGER", Value: int64(2)},
		},
	}

	fields, err := serializeHotFields(entry)
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

func TestHotSerializerLabelsSorted(t *testing.T) {
	entry := &model.TrapEntry{
		JobName:               "local",
		ReportType:            model.ReportTypeTrap,
		ReceivedRealtimeUsec:  1000000,
		ReceivedMonotonicUsec: 1000,
		TrapOID:               "1.3.6.1.6.3.1.1.5.3",
		Message:               "test",
		SourceIP:              "10.0.0.1",
		PduType:               model.PduTypeTrap,
		SnmpVersion:           model.SnmpVersionV2c,
		Labels:                map[string]string{"z_key": "z_val", "a_key": "a_val"},
		Varbinds:              []model.VarbindValue{},
	}

	fields, err := serializeHotFields(entry)
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

func TestTrapTagJournalFieldNameIsCapped(t *testing.T) {
	longKey := strings.Repeat("a", 80)
	entry := &model.TrapEntry{
		JobName:               "local",
		ReportType:            model.ReportTypeTrap,
		ReceivedRealtimeUsec:  1000000,
		ReceivedMonotonicUsec: 1000,
		TrapOID:               "1.3.6.1.6.3.1.1.5.3",
		Message:               "test",
		SourceIP:              "10.0.0.1",
		PduType:               model.PduTypeTrap,
		SnmpVersion:           model.SnmpVersionV2c,
		Labels:                map[string]string{longKey: "value"},
	}

	fields, err := serializeHotFields(entry)
	if err != nil {
		t.Fatalf("serializeHotFields: %v", err)
	}
	fieldMap := fieldsToMap(fields)
	var found string
	for name := range fieldMap {
		if strings.HasPrefix(name, "TRAP_TAG_") {
			found = name
			break
		}
	}
	if found == "" {
		t.Fatal("missing TRAP_TAG field")
	}
	if len(found) > maxJournalFieldNameLen {
		t.Fatalf("TRAP_TAG field name length = %d, want <= %d: %s", len(found), maxJournalFieldNameLen, found)
	}
	assertField(t, fieldMap, found, "value")
}

func TestHotSerializerCanonicalCases(t *testing.T) {
	cases := map[string]*model.TrapEntry{
		"Trap": {
			JobName:               "local",
			ReportType:            model.ReportTypeTrap,
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
			PduType:               model.PduTypeTrap,
			SnmpVersion:           model.SnmpVersionV2c,
			Labels:                map[string]string{"z_key": "z_val", "a_key": "a_val"},
			Varbinds: []model.VarbindValue{
				{OID: model.SNMPTrapCommunityOID, Name: "snmpTrapCommunity.0", Type: "OctetString", Value: "private-community"},
				{OID: "1.3.6.1.2.1.2.2.1.7", Name: "ifAdminStatus", Type: "INTEGER", Value: int64(1), Enum: "up"},
				{OID: "1.3.6.1.2.1.2.2.1.1", Name: "ifIndex", Type: "INTEGER", Value: int64(1)},
				{OID: "1.3.6.1.2.1.2.2.1.2", Name: "ifIndex", Type: "OctetString", Value: "eth0"},
			},
		},
		"model.DedupSummary": {
			JobName:               "local",
			ReportType:            model.ReportTypeDedupSummary,
			ReceivedRealtimeUsec:  1000000,
			ReceivedMonotonicUsec: 1000,
			Severity:              "notice",
			Message:               "summary",
			Varbinds:              []model.VarbindValue{{OID: "1.3.6.1.2.1.2.2.1.1", Name: "ifIndex", Type: "INTEGER", Value: int64(1)}},
			SummaryCounts: &model.DedupSummary{
				TotalSuppressed: 12,
				Fingerprints:    2,
				PeriodSec:       60,
				ByTrap:          map[string]int64{"1.3.6.1.6.3.1.1.5.3": 12},
			},
		},
		"Binary encoded": {
			JobName:               "local",
			ReportType:            model.ReportTypeTrap,
			ReceivedRealtimeUsec:  1000000,
			ReceivedMonotonicUsec: 1000,
			TrapOID:               "1.3.6.1.6.3.1.1.5.3",
			Severity:              "warning",
			Message:               "line1\nline2",
			SourceIP:              "10.0.0.1",
			PduType:               model.PduTypeTrap,
			SnmpVersion:           model.SnmpVersionV2c,
		},
		"DecodeError": {
			JobName:               "local",
			ReportType:            model.ReportTypeDecodeError,
			ReceivedRealtimeUsec:  1000000,
			ReceivedMonotonicUsec: 1000,
			Severity:              "warning",
			Category:              "diagnostic",
			Message:               "SNMP trap decode failed from 10.0.0.1: malformed_pdu: BER: trailing data",
			SourceIP:              "10.0.0.1",
			SourceUDPPeer:         "10.0.0.1",
			SnmpVersion:           model.SnmpVersionV2c,
			DecodeError: &model.DecodeErrorInfo{
				Kind:          "malformed_pdu",
				Error:         "BER: trailing data",
				PacketSize:    42,
				PacketSHA256:  strings.Repeat("a", 64),
				SourceUDPPort: 9162,
				Listener:      "0.0.0.0:162",
				EngineID:      "8000000001020304",
			},
			Varbinds: []model.VarbindValue{{OID: "1.3.6.1.2.1.2.2.1.1", Name: "ifIndex", Type: "INTEGER", Value: int64(1)}},
		},
		"JSONEscapingAndValueTypes": {
			JobName:               "local",
			ReportType:            model.ReportTypeTrap,
			ReceivedRealtimeUsec:  1000000,
			ReceivedMonotonicUsec: 1000,
			TrapOID:               "1.3.6.1.6.3.1.1.5.3",
			Severity:              "warning",
			Message:               "json values",
			SourceIP:              "10.0.0.1",
			PduType:               model.PduTypeTrap,
			SnmpVersion:           model.SnmpVersionV2c,
			Varbinds: []model.VarbindValue{
				{OID: "1.3.6.1.2.1.2.2.1.1", Name: `quote"\control`, Type: "OctetString", Value: "a<b>&\"\n\u2028\u2029" + string([]byte{0xff})},
				{OID: "1.3.6.1.2.1.2.2.1.2", Name: "bytes", Type: "OctetString", Value: []byte{0, 15, 255}},
				{OID: "1.3.6.1.2.1.2.2.1.3", Name: "float", Type: "OpaqueFloat", Value: float64(1.25)},
				{OID: "1.3.6.1.2.1.2.2.1.4", Name: "bool", Type: "BOOLEAN", Value: true},
				{OID: "1.3.6.1.2.1.2.2.1.5", Name: "nil", Type: "Null", Value: nil},
			},
		},
	}

	for name, entry := range cases {
		t.Run(name, func(t *testing.T) {
			var serializer hotSerializer
			payloads, _, err := serializer.serialize(entry)
			if err != nil {
				t.Fatalf("serialize: %v", err)
			}
			if len(payloads) == 0 {
				t.Fatal("serialize returned no payloads")
			}
		})
	}
}

func TestHotSerializerTRAPJSONCanonicalValueTypes(t *testing.T) {
	entry := &model.TrapEntry{
		JobName:    "local",
		ReportType: model.ReportTypeTrap,
		TrapOID:    ".0",
		SourceIP:   "192.0.2.1",
		Varbinds: []model.VarbindValue{
			{Name: "string", OID: ".1", Type: "OctetString", Value: "quote\"\\line\n"},
			{Name: "int", OID: ".2", Type: "INTEGER", Value: int64(-3)},
			{Name: "uint", OID: ".3", Type: "Counter64", Value: uint64(7)},
			{Name: "float", OID: ".4", Type: "OpaqueFloat", Value: float64(1.25)},
			{Name: "bool", OID: ".5", Type: "BOOLEAN", Value: true},
			{Name: "bytes", OID: ".6", Type: "OctetString", Value: []byte{0x00, 0x0f, 0xff}},
			{Name: "nil", OID: ".7", Type: "Null", Value: nil},
		},
	}

	fields, err := serializeHotFields(entry)
	if err != nil {
		t.Fatalf("serializeHotFields: %v", err)
	}

	want := `{"bool":{"oid":".5","type":"BOOLEAN","value":true},` +
		`"bytes":{"oid":".6","type":"OctetString","value":"000fff"},` +
		`"float":{"oid":".4","type":"OpaqueFloat","value":1.25},` +
		`"int":{"oid":".2","type":"INTEGER","value":-3},` +
		`"nil":{"oid":".7","type":"Null","value":null},` +
		`"string":{"oid":".1","type":"OctetString","value":"quote\"\\line\n"},` +
		`"uint":{"oid":".3","type":"Counter64","value":7}}`
	if got := fieldsToMap(fields)["TRAP_JSON"]; got != want {
		t.Fatalf("TRAP_JSON = %q, want %q", got, want)
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

func assertFieldWithPrefix(t *testing.T, fieldMap map[string]string, prefix, expected string) {
	t.Helper()
	for name, value := range fieldMap {
		if strings.HasPrefix(name, prefix) && value == expected {
			return
		}
	}
	t.Fatalf("missing field with prefix %q and value %q", prefix, expected)
}

func fieldsToMap(fields []decodedField) map[string]string {
	m := make(map[string]string, len(fields))
	for _, f := range fields {
		m[f.Name] = string(f.Value)
	}
	return m
}

func rawPayloadsToMap(payloads [][]byte) map[string]string {
	m := make(map[string]string, len(payloads))
	for _, p := range payloads {
		name, value, ok := bytes.Cut(p, []byte{'='})
		if !ok {
			continue
		}
		m[string(name)] = string(value)
	}
	return m
}

func journalFieldNames(fields []decodedField) []string {
	names := make([]string, 0, len(fields))
	for _, field := range fields {
		names = append(names, field.Name)
	}
	return names
}

func payloadFieldNames(payloads [][]byte) []string {
	names := make([]string, 0, len(payloads))
	for _, payload := range payloads {
		name, _, ok := bytes.Cut(payload, []byte{'='})
		if ok {
			names = append(names, string(name))
		}
	}
	return names
}

func assertFieldOrder(t *testing.T, names []string, ordered ...string) {
	t.Helper()
	previous := -1
	for _, want := range ordered {
		found := -1
		for i, name := range names {
			if name == want {
				found = i
				break
			}
		}
		if found == -1 {
			t.Fatalf("field %q not found in %v", want, names)
		}
		if previous >= 0 && found <= previous {
			t.Fatalf("field %q at index %d should be after previous ordered field at index %d in %v", want, found, previous, names)
		}
		previous = found
	}
	if last := ordered[len(ordered)-1]; names[len(names)-1] != last {
		t.Fatalf("last field = %q, want %q; order: %v", names[len(names)-1], last, names)
	}
}
