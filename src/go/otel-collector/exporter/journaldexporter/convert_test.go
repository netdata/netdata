// SPDX-License-Identifier: GPL-3.0-or-later

package journaldexporter

import (
	"bytes"
	"encoding/binary"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"go.opentelemetry.io/collector/pdata/pcommon"
	"go.opentelemetry.io/collector/pdata/plog"
)

func TestLogsToJournaldMessages(t *testing.T) {
	type testCase struct {
		logsFn   func() plog.Logs
		expected string
	}

	tests := map[string]testCase{
		"simple log with basic fields": {
			logsFn: func() plog.Logs {
				logs := plog.NewLogs()
				rl := logs.ResourceLogs().AppendEmpty()

				rl.Resource().Attributes().PutStr("service.name", "test-service")
				rl.Resource().Attributes().PutStr("service.instance.id", "instance-1")

				sl := rl.ScopeLogs().AppendEmpty()
				sl.Scope().SetName("test-scope")
				sl.Scope().SetVersion("v1.0.0")

				lr := sl.LogRecords().AppendEmpty()
				lr.SetTimestamp(pcommon.NewTimestampFromTime(time.Unix(1617030613, 0))) // 2021-03-29T12:23:33Z
				lr.SetSeverityNumber(plog.SeverityNumberInfo)
				lr.SetSeverityText("INFO")
				lr.Body().SetStr("This is a test message")

				lr.Attributes().PutStr("http.method", "GET")
				lr.Attributes().PutInt("http.status_code", 200)

				return logs
			},
			expected: `__REALTIME_TIMESTAMP=
SYSLOG_IDENTIFIER=test-syslog-id
_PID=test-pid
_UID=test-uid
_BOOT_ID=test-boot-id
_MACHINE_ID=test-machine-id
_HOSTNAME=test-hostname
PRIORITY=6
MESSAGE=This is a test message
OTEL_RESOURCE_ATTR_SERVICE_NAME=test-service
OTEL_RESOURCE_ATTR_SERVICE_INSTANCE_ID=instance-1
OTEL_SCOPE_NAME=test-scope
OTEL_SCOPE_VERSION=v1.0.0
OTEL_TIMESTAMP=1617030613000000
OTEL_SEVERITY_LEVEL=INFO
OTEL_ATTR_HTTP_METHOD=GET
OTEL_ATTR_HTTP_STATUS_CODE=200

`,
		},
		"log with trace context": {
			logsFn: func() plog.Logs {
				logs := plog.NewLogs()
				rl := logs.ResourceLogs().AppendEmpty()
				sl := rl.ScopeLogs().AppendEmpty()
				lr := sl.LogRecords().AppendEmpty()

				lr.SetSeverityNumber(plog.SeverityNumberError)
				lr.SetSeverityText("ERROR")
				lr.Body().SetStr("Connection failed")

				traceID := pcommon.TraceID([16]byte{1, 2, 3, 4, 5, 6, 7, 8, 9, 10, 11, 12, 13, 14, 15, 16})
				spanID := pcommon.SpanID([8]byte{1, 2, 3, 4, 5, 6, 7, 8})
				lr.SetTraceID(traceID)
				lr.SetSpanID(spanID)
				lr.SetFlags(1) // Sampled flag

				return logs
			},
			expected: `__REALTIME_TIMESTAMP=
SYSLOG_IDENTIFIER=test-syslog-id
_PID=test-pid
_UID=test-uid
_BOOT_ID=test-boot-id
_MACHINE_ID=test-machine-id
_HOSTNAME=test-hostname
PRIORITY=3
MESSAGE=Connection failed
OTEL_SEVERITY_LEVEL=ERROR
OTEL_TRACE_ID=0102030405060708090a0b0c0d0e0f10
OTEL_SPAN_ID=0102030405060708
OTEL_TRACE_FLAGS=1

`,
		},
		"log with multiline message": {
			logsFn: func() plog.Logs {
				logs := plog.NewLogs()
				rl := logs.ResourceLogs().AppendEmpty()
				sl := rl.ScopeLogs().AppendEmpty()
				lr := sl.LogRecords().AppendEmpty()

				lr.SetSeverityNumber(plog.SeverityNumberError)
				lr.Body().SetStr("Error occurred:\nStack trace:\n  at function1()\n  at function2()\n  at main()")

				return logs
			},
			expected: func() string {
				var buf bytes.Buffer
				buf.WriteString("__REALTIME_TIMESTAMP=\n")
				buf.WriteString("SYSLOG_IDENTIFIER=test-syslog-id\n")
				buf.WriteString("_PID=test-pid\n")
				buf.WriteString("_UID=test-uid\n")
				buf.WriteString("_BOOT_ID=test-boot-id\n")
				buf.WriteString("_MACHINE_ID=test-machine-id\n")
				buf.WriteString("_HOSTNAME=test-hostname\n")
				buf.WriteString("PRIORITY=3\n")
				buf.WriteString("MESSAGE\n")
				multilineMsg := "Error occurred:\nStack trace:\n  at function1()\n  at function2()\n  at main()"
				_ = binary.Write(&buf, binary.LittleEndian, uint64(len(multilineMsg)))
				buf.WriteString(multilineMsg)
				buf.WriteString("\n\n")
				return buf.String()
			}(),
		},
	}

	for name, tc := range tests {
		t.Run(name, func(t *testing.T) {
			var buf bytes.Buffer
			e := journaldExporter{fields: commonFields{
				syslogID:  "test-syslog-id",
				pid:       "test-pid",
				uid:       "test-uid",
				hostname:  "test-hostname",
				bootID:    "test-boot-id",
				machineID: "test-machine-id",
			}}

			e.logsToJournaldMessages(tc.logsFn(), &buf)

			ts, _, _ := strings.Cut(buf.String(), "\n")
			_, expected, _ := strings.Cut(tc.expected, "\n")

			assert.Equal(t, ts+"\n"+expected, buf.String())
		})
	}
}

func TestMappingSeverity(t *testing.T) {
	tests := map[string]struct {
		severity plog.SeverityNumber
		expected int
	}{
		"undefined": {severity: plog.SeverityNumberUnspecified, expected: 6},
		"trace":     {severity: plog.SeverityNumberTrace, expected: 7},
		"trace2":    {severity: plog.SeverityNumberTrace2, expected: 7},
		"debug":     {severity: plog.SeverityNumberDebug, expected: 7},
		"info":      {severity: plog.SeverityNumberInfo, expected: 6},
		"warn":      {severity: plog.SeverityNumberWarn, expected: 4},
		"error":     {severity: plog.SeverityNumberError, expected: 3},
		"fatal":     {severity: plog.SeverityNumberFatal, expected: 2},
	}

	for name, tc := range tests {
		t.Run(name, func(t *testing.T) {
			result := mapSeverityToJournaldPriority(tc.severity)
			assert.Equal(t, tc.expected, result)
		})
	}
}
