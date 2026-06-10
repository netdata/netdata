// SPDX-License-Identifier: GPL-3.0-or-later

package snmp_traps

import (
	"encoding/hex"
	"encoding/json"
	"fmt"
	"maps"
	"slices"
	"sort"
	"strconv"
	"strings"
	"unicode/utf8"
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
	isDecodeError := entry.ReportType == ReportTypeDecodeError

	if !isDedupSummary && !isDecodeError {
		if entry.TrapOID == "" {
			return nil, errMissingTrapOID
		}
	}
	if !isDedupSummary {
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
	fields = append(fields, JournalField{Name: "TRAP_JOB", Value: []byte(entry.JobName)})

	if !isDedupSummary && hostname != "" {
		fields = append(fields, JournalField{Name: "_HOSTNAME", Value: []byte(hostname)})
	}

	fields = append(fields, JournalField{Name: "ND_LOG_SOURCE", Value: []byte("snmp-trap")})

	if !isDedupSummary && entry.SourceVnodeID != "" {
		fields = append(fields, JournalField{Name: "ND_NIDL_NODE", Value: []byte(entry.SourceVnodeID)})
	}

	reportType := string(entry.ReportType)
	if reportType == "" {
		reportType = "trap"
	}
	fields = append(fields, JournalField{Name: "TRAP_REPORT_TYPE", Value: []byte(reportType)})

	if !isDedupSummary && !isDecodeError {
		fields = append(fields, JournalField{Name: "TRAP_OID", Value: []byte(entry.TrapOID)})
		if entry.TrapName != "" {
			fields = append(fields, JournalField{Name: "TRAP_NAME", Value: []byte(entry.TrapName)})
		}
	}
	if !isDedupSummary {
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
		fields = append(fields, JournalField{Name: "TRAP_SUPPRESSED_COUNT", Value: fmt.Appendf(nil, "%d", sc.TotalSuppressed)})
		fields = append(fields, JournalField{Name: "TRAP_SUPPRESSED_FINGERPRINTS", Value: fmt.Appendf(nil, "%d", sc.Fingerprints)})
		fields = append(fields, JournalField{Name: "TRAP_REPORT_PERIOD_SEC", Value: fmt.Appendf(nil, "%d", sc.PeriodSec)})
	}
	if isDecodeError && entry.DecodeError != nil {
		appendDecodeErrorJournalFields(&fields, entry.DecodeError)
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
	if entry.DecodeError != nil {
		return json.Marshal(entry.DecodeError)
	}

	obj := make(map[string]jsonVarbindEntry)
	seenKeys := make(map[string]int)

	for _, vb := range entry.Varbinds {
		if isSensitiveTrapVarbind(vb) {
			continue
		}
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
			maps.Copy(bt, entry.SummaryCounts.ByTrap)
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

func isSensitiveTrapVarbind(vb VarbindValue) bool {
	oid := normalizeOID(vb.OID)
	if oid == snmpTrapCommunityOID || oid == strings.TrimSuffix(snmpTrapCommunityOID, ".0") {
		return true
	}

	name := strings.TrimSuffix(vb.Name, ".0")
	return name == "snmpTrapCommunity"
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

type journalHotSerializer struct {
	payloads            [][]byte
	buf                 []byte
	labelKeys           []string
	jsonEntries         []rawJSONVarbindEntry
	seenJSONKeys        map[string]int
	binaryEncodedFields int
}

func (s *journalHotSerializer) serialize(entry *TrapEntry) ([][]byte, int, error) {
	s.reset()

	if entry == nil {
		return nil, 0, errNilEntry
	}
	if entry.JobName == "" {
		return nil, 0, errMissingJobName
	}
	if entry.ReceivedRealtimeUsec < 0 || entry.ReceivedMonotonicUsec < 0 {
		return nil, 0, errNegativeTimestamp
	}

	isDedupSummary := entry.ReportType == ReportTypeDedupSummary
	isDecodeError := entry.ReportType == ReportTypeDecodeError

	if !isDedupSummary && !isDecodeError {
		if entry.TrapOID == "" {
			return nil, 0, errMissingTrapOID
		}
	}
	if !isDedupSummary {
		if entry.SourceIP == "" && entry.SourceUDPPeer == "" && entry.DeviceHostname == "" {
			return nil, 0, errMissingSourceIP
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

	s.addStringField("MESSAGE", entry.Message)
	s.addStringField("PRIORITY", severityPriority(entry.Severity))
	s.addStringField("SYSLOG_IDENTIFIER", entry.JobName)
	s.addStringField("TRAP_JOB", entry.JobName)

	if !isDedupSummary && hostname != "" {
		s.addStringField("_HOSTNAME", hostname)
	}

	s.addStringField("ND_LOG_SOURCE", "snmp-trap")

	if !isDedupSummary && entry.SourceVnodeID != "" {
		s.addStringField("ND_NIDL_NODE", entry.SourceVnodeID)
	}

	reportType := string(entry.ReportType)
	if reportType == "" {
		reportType = "trap"
	}
	s.addStringField("TRAP_REPORT_TYPE", reportType)

	if !isDedupSummary && !isDecodeError {
		s.addStringField("TRAP_OID", entry.TrapOID)
		if entry.TrapName != "" {
			s.addStringField("TRAP_NAME", entry.TrapName)
		}
	}
	if !isDedupSummary {
		if entry.Category != "" {
			s.addStringField("TRAP_CATEGORY", string(entry.Category))
		}
		if entry.Severity != "" {
			s.addStringField("TRAP_SEVERITY", string(entry.Severity))
		}
		if entry.PduType != "" {
			s.addStringField("TRAP_PDU_TYPE", string(entry.PduType))
		}
		if entry.SnmpVersion != "" {
			s.addStringField("TRAP_VERSION", string(entry.SnmpVersion))
		}
		if entry.SourceIP != "" {
			s.addStringField("TRAP_SOURCE_IP", entry.SourceIP)
		}
		if entry.SourceUDPPeer != "" {
			s.addStringField("TRAP_SOURCE_UDP_PEER", entry.SourceUDPPeer)
		}
		if entry.DeviceVendor != "" {
			s.addStringField("TRAP_DEVICE_VENDOR", entry.DeviceVendor)
		}
		if entry.TopologyInterface != "" {
			s.addStringField("TRAP_INTERFACE", entry.TopologyInterface)
		}
		if entry.TopologyNeighbors != "" {
			s.addStringField("TRAP_NEIGHBORS", entry.TopologyNeighbors)
		}
	}

	if err := s.addTrapJSONField(entry); err != nil {
		return nil, 0, fmt.Errorf("TRAP_JSON: %w", err)
	}

	if isDedupSummary && entry.SummaryCounts != nil {
		sc := entry.SummaryCounts
		s.addIntField("TRAP_SUPPRESSED_COUNT", sc.TotalSuppressed)
		s.addIntField("TRAP_SUPPRESSED_FINGERPRINTS", sc.Fingerprints)
		s.addIntField("TRAP_REPORT_PERIOD_SEC", sc.PeriodSec)
	}
	if isDecodeError && entry.DecodeError != nil {
		s.addDecodeErrorFields(entry.DecodeError)
	}

	for _, key := range s.sortedLabelKeys(entry.Labels) {
		val := entry.Labels[key]
		upperKey := strings.ToUpper(key)
		if !isValidTrapTagKey(upperKey) {
			return nil, 0, fmt.Errorf("invalid label key for TRAP_TAG: %q", key)
		}
		s.addStringField("TRAP_TAG_"+upperKey, val)
	}

	return s.payloads, s.binaryEncodedFields, nil
}

func (s *journalHotSerializer) reset() {
	s.payloads = s.payloads[:0]
	s.buf = s.buf[:0]
	s.labelKeys = s.labelKeys[:0]
	s.binaryEncodedFields = 0
}

func (s *journalHotSerializer) addStringField(name, value string) {
	start := len(s.buf)
	s.buf = append(s.buf, name...)
	s.buf = append(s.buf, '=')
	valueStart := len(s.buf)
	s.buf = append(s.buf, value...)
	s.addPayload(start, valueStart)
}

func (s *journalHotSerializer) addIntField(name string, value int64) {
	start := len(s.buf)
	s.buf = append(s.buf, name...)
	s.buf = append(s.buf, '=')
	valueStart := len(s.buf)
	s.buf = strconv.AppendInt(s.buf, value, 10)
	s.addPayload(start, valueStart)
}

func (s *journalHotSerializer) addDecodeErrorFields(info *DecodeErrorInfo) {
	if info.Kind != "" {
		s.addStringField("TRAP_DECODE_ERROR_KIND", info.Kind)
	}
	if info.Error != "" {
		s.addStringField("TRAP_DECODE_ERROR", info.Error)
	}
	s.addIntField("TRAP_PACKET_SIZE", int64(info.PacketSize))
	if info.PacketSHA256 != "" {
		s.addStringField("TRAP_PACKET_SHA256", info.PacketSHA256)
	}
	if info.SourceUDPPort > 0 {
		s.addIntField("TRAP_SOURCE_UDP_PORT", int64(info.SourceUDPPort))
	}
	if info.Listener != "" {
		s.addStringField("TRAP_LISTENER", info.Listener)
	}
	if info.EngineID != "" {
		s.addStringField("TRAP_ENGINE_ID", info.EngineID)
	}
}

func (s *journalHotSerializer) addPayload(start, valueStart int) {
	if journalFieldNeedsBinary(s.buf[valueStart:]) {
		s.binaryEncodedFields++
	}
	s.payloads = append(s.payloads, s.buf[start:])
}

func (s *journalHotSerializer) sortedLabelKeys(labels map[string]string) []string {
	s.labelKeys = s.labelKeys[:0]
	for key := range labels {
		s.labelKeys = append(s.labelKeys, key)
	}
	sort.Strings(s.labelKeys)
	return s.labelKeys
}

func (s *journalHotSerializer) addTrapJSONField(entry *TrapEntry) error {
	start := len(s.buf)
	s.buf = append(s.buf, "TRAP_JSON"...)
	s.buf = append(s.buf, '=')
	valueStart := len(s.buf)

	if entry.SummaryCounts != nil || entry.DecodeError != nil {
		trapJSON, err := buildTrapJSON(entry)
		if err != nil {
			return err
		}
		s.buf = append(s.buf, trapJSON...)
		s.addPayload(start, valueStart)
		return nil
	}

	if err := s.appendTrapJSONObject(entry); err != nil {
		return err
	}
	s.addPayload(start, valueStart)
	return nil
}

func appendDecodeErrorJournalFields(fields *[]JournalField, info *DecodeErrorInfo) {
	if info.Kind != "" {
		*fields = append(*fields, JournalField{Name: "TRAP_DECODE_ERROR_KIND", Value: []byte(info.Kind)})
	}
	if info.Error != "" {
		*fields = append(*fields, JournalField{Name: "TRAP_DECODE_ERROR", Value: []byte(info.Error)})
	}
	*fields = append(*fields, JournalField{Name: "TRAP_PACKET_SIZE", Value: fmt.Appendf(nil, "%d", info.PacketSize)})
	if info.PacketSHA256 != "" {
		*fields = append(*fields, JournalField{Name: "TRAP_PACKET_SHA256", Value: []byte(info.PacketSHA256)})
	}
	if info.SourceUDPPort > 0 {
		*fields = append(*fields, JournalField{Name: "TRAP_SOURCE_UDP_PORT", Value: fmt.Appendf(nil, "%d", info.SourceUDPPort)})
	}
	if info.Listener != "" {
		*fields = append(*fields, JournalField{Name: "TRAP_LISTENER", Value: []byte(info.Listener)})
	}
	if info.EngineID != "" {
		*fields = append(*fields, JournalField{Name: "TRAP_ENGINE_ID", Value: []byte(info.EngineID)})
	}
}

type rawJSONVarbindEntry struct {
	key string
	vb  VarbindValue
}

func (s *journalHotSerializer) appendTrapJSONObject(entry *TrapEntry) error {
	s.jsonEntries = s.jsonEntries[:0]
	if s.seenJSONKeys == nil {
		s.seenJSONKeys = make(map[string]int, len(entry.Varbinds))
	} else {
		for key := range s.seenJSONKeys {
			delete(s.seenJSONKeys, key)
		}
	}

	for _, vb := range entry.Varbinds {
		if isSensitiveTrapVarbind(vb) {
			continue
		}
		key := vb.Name
		if key == "" {
			key = vb.OID
		}
		if key == "" {
			continue
		}
		s.seenJSONKeys[key]++
		if s.seenJSONKeys[key] > 1 {
			key = fmt.Sprintf("%s#%d", key, s.seenJSONKeys[key])
		}
		s.jsonEntries = append(s.jsonEntries, rawJSONVarbindEntry{key: key, vb: vb})
	}

	slices.SortFunc(s.jsonEntries, func(a, b rawJSONVarbindEntry) int {
		return strings.Compare(a.key, b.key)
	})

	s.buf = append(s.buf, '{')
	for i, entry := range s.jsonEntries {
		if i > 0 {
			s.buf = append(s.buf, ',')
		}
		s.appendJSONString(entry.key)
		s.buf = append(s.buf, ':')
		s.appendJSONVarbind(entry.vb)
	}
	s.buf = append(s.buf, '}')
	return nil
}

func (s *journalHotSerializer) appendJSONVarbind(vb VarbindValue) {
	s.buf = append(s.buf, `{"oid":`...)
	s.appendJSONString(vb.OID)
	s.buf = append(s.buf, `,"type":`...)
	s.appendJSONString(string(vb.Type))
	s.buf = append(s.buf, `,"value":`...)
	s.appendJSONVarbindValue(vb.Value)
	if vb.Enum != "" {
		s.buf = append(s.buf, `,"enum":`...)
		s.appendJSONString(vb.Enum)
	}
	s.buf = append(s.buf, '}')
}

func (s *journalHotSerializer) appendJSONVarbindValue(value any) {
	switch v := value.(type) {
	case nil:
		s.buf = append(s.buf, "null"...)
	case string:
		s.appendJSONString(v)
	case int64:
		s.buf = strconv.AppendInt(s.buf, v, 10)
	case uint64:
		s.buf = strconv.AppendUint(s.buf, v, 10)
	case float64:
		s.buf = strconv.AppendFloat(s.buf, v, 'f', -1, 64)
	case bool:
		s.buf = strconv.AppendBool(s.buf, v)
	case []byte:
		s.buf = append(s.buf, '"')
		dstLen := hex.EncodedLen(len(v))
		oldLen := len(s.buf)
		for range dstLen {
			s.buf = append(s.buf, 0)
		}
		hex.Encode(s.buf[oldLen:oldLen+dstLen], v)
		s.buf = append(s.buf, '"')
	default:
		s.appendJSONString(fmt.Sprintf("%v", v))
	}
}

func (s *journalHotSerializer) appendJSONString(value string) {
	const hexDigits = "0123456789abcdef"

	s.buf = append(s.buf, '"')
	start := 0
	for i := 0; i < len(value); {
		if c := value[i]; c < utf8.RuneSelf {
			if c >= 0x20 && c != '\\' && c != '"' && c != '<' && c != '>' && c != '&' {
				i++
				continue
			}
			s.buf = append(s.buf, value[start:i]...)
			switch c {
			case '\\', '"':
				s.buf = append(s.buf, '\\', c)
			case '\b':
				s.buf = append(s.buf, '\\', 'b')
			case '\f':
				s.buf = append(s.buf, '\\', 'f')
			case '\n':
				s.buf = append(s.buf, '\\', 'n')
			case '\r':
				s.buf = append(s.buf, '\\', 'r')
			case '\t':
				s.buf = append(s.buf, '\\', 't')
			default:
				s.buf = append(s.buf, '\\', 'u', '0', '0', hexDigits[c>>4], hexDigits[c&0xf])
			}
			i++
			start = i
			continue
		}

		r, size := utf8.DecodeRuneInString(value[i:])
		if r == utf8.RuneError && size == 1 {
			s.buf = append(s.buf, value[start:i]...)
			s.buf = append(s.buf, `\ufffd`...)
			i++
			start = i
			continue
		}
		if r == '\u2028' || r == '\u2029' {
			s.buf = append(s.buf, value[start:i]...)
			s.buf = append(s.buf, '\\', 'u', '2', '0', '2', hexDigits[r&0xf])
			i += size
			start = i
			continue
		}
		i += size
	}
	s.buf = append(s.buf, value[start:]...)
	s.buf = append(s.buf, '"')
}
