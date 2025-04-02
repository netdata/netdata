package journaldexporter

import (
	"bytes"
	"encoding/binary"
	"strconv"
	"strings"
	"time"

	"go.opentelemetry.io/collector/pdata/pcommon"
	"go.opentelemetry.io/collector/pdata/plog"
)

func logsToJournaldMessages(ld plog.Logs, buf *bytes.Buffer) {
	rls := ld.ResourceLogs()
	for i := 0; i < rls.Len(); i++ {
		rl := rls.At(i)
		resource := rl.Resource()

		scopeLogs := rl.ScopeLogs()
		for j := 0; j < scopeLogs.Len(); j++ {
			sl := scopeLogs.At(j)
			scope := sl.Scope()

			logRecords := sl.LogRecords()
			for k := 0; k < logRecords.Len(); k++ {
				lr := logRecords.At(k)

				resource.Attributes().Range(func(k string, v pcommon.Value) bool {
					writeField(buf, "RESOURCE_"+k, v.AsString())
					return true
				})

				if scope.Name() != "" {
					writeField(buf, "OTEL_SCOPE_NAME", scope.Name())
				}
				if scope.Version() != "" {
					writeField(buf, "OTEL_SCOPE_VERSION", scope.Version())
				}

				if lr.Timestamp() != 0 {
					ts := time.Unix(0, int64(lr.Timestamp()))
					writeField(buf, "REALTIME_TIMESTAMP", strconv.FormatInt(ts.UnixMicro(), 10))
				}

				if lr.ObservedTimestamp() != 0 && lr.ObservedTimestamp() != lr.Timestamp() {
					ts := time.Unix(0, int64(lr.ObservedTimestamp()))
					writeField(buf, "OTEL_OBSERVED_TIMESTAMP", strconv.FormatInt(ts.UnixMicro(), 10))
				}

				priority := mapSeverityToJournaldPriority(lr.SeverityNumber())
				writeField(buf, "PRIORITY", strconv.Itoa(priority))

				if lr.SeverityText() != "" {
					writeField(buf, "LEVEL", lr.SeverityText())
				}

				if !lr.TraceID().IsEmpty() {
					writeField(buf, "TRACE_ID", lr.TraceID().String())
					if !lr.SpanID().IsEmpty() {
						writeField(buf, "SPAN_ID", lr.SpanID().String())
					}
					if lr.Flags() != 0 {
						writeField(buf, "TRACE_FLAGS", strconv.FormatUint(uint64(lr.Flags()), 16))
					}
				}

				if lr.EventName() != "" {
					writeField(buf, "EVENT_NAME", lr.EventName())
				}

				bodyStr := bodyToString(lr.Body())
				if bodyStr != "" {
					writeField(buf, "MESSAGE", bodyStr)
				}

				lr.Attributes().Range(func(k string, v pcommon.Value) bool {
					writeField(buf, "ATTR_"+k, v.AsString())
					return true
				})

				buf.WriteByte('\n') // extra newline
			}
		}
	}
}

func mapSeverityToJournaldPriority(severity plog.SeverityNumber) int {
	switch {
	case severity >= plog.SeverityNumberFatal && severity <= plog.SeverityNumberFatal4:
		return 2 // critical
	case severity >= plog.SeverityNumberError && severity <= plog.SeverityNumberError4:
		return 3 // error
	case severity >= plog.SeverityNumberWarn && severity <= plog.SeverityNumberWarn4:
		return 4 // warning
	case severity >= plog.SeverityNumberInfo && severity <= plog.SeverityNumberInfo4:
		return 6 // info
	case severity >= plog.SeverityNumberDebug && severity <= plog.SeverityNumberDebug4:
		return 7 // debug
	case severity >= plog.SeverityNumberTrace && severity <= plog.SeverityNumberTrace4:
		return 7 // debug (journald doesn't have trace)
	default:
		return 6 // info as default
	}
}

func bodyToString(body pcommon.Value) string {
	switch body.Type() {
	case pcommon.ValueTypeEmpty:
		return ""
	case pcommon.ValueTypeStr:
		return body.Str()
	default:
		return body.AsString()
	}
}

func writeField(buf *bytes.Buffer, name, value string) {
	if value == "" {
		return
	}
	normalizedName := normalizeFieldName(name)

	if strings.ContainsRune(value, '\n') {
		buf.WriteString(normalizedName)
		buf.WriteByte('\n')
		_ = binary.Write(buf, binary.LittleEndian, uint64(len(value)))
		buf.WriteString(value)
		buf.WriteByte('\n')
	} else {
		buf.WriteString(normalizedName)
		buf.WriteByte('=')
		buf.WriteString(value)
		buf.WriteByte('\n')
	}
}

func normalizeFieldName(name string) string {
	// Journald field names can only contain uppercase letters, numbers, and underscores
	// Replace any character that isn't A-Z, 0-9, or _ with _
	normalized := strings.Map(func(r rune) rune {
		if (r >= 'A' && r <= 'Z') || (r >= '0' && r <= '9') || r == '_' {
			return r
		}
		if r >= 'a' && r <= 'z' {
			return r - 32 // Convert to uppercase
		}
		return '_'
	}, name)

	// Journald field names must be uppercase
	return strings.ToUpper(normalized)
}
