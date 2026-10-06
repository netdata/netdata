// SPDX-License-Identifier: GPL-3.0-or-later
package otlp

import (
	"encoding/json"
	"strings"
	"testing"
	"time"
	"unicode/utf8"

	"github.com/netdata/netdata/go/plugins/plugin/dem/rum/aggregate"
	"github.com/netdata/netdata/go/plugins/plugin/dem/rum/beacon"
	"github.com/netdata/netdata/go/plugins/plugin/dem/rum/faro"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"google.golang.org/protobuf/proto"
)

func TestDecodedConsoleUnicodeRemainsExportable(t *testing.T) {
	for _, field := range []string{"type", "message", "function", "assembled stack"} {
		t.Run(field, func(t *testing.T) {
			typ, message, function := "TypeError", "message", "render"
			filename := "app.js"
			switch field {
			case "type":
				typ = strings.Repeat("界", 30)
			case "message":
				message = strings.Repeat("界", 400)
			case "function":
				function = strings.Repeat("界", 1500)
			case "assembled stack":
				function = strings.Repeat("界", 1365)
			}
			frame, err := json.Marshal(
				map[string]any{"function": function, "filename": filename, "lineno": 1, "colno": 2},
			)
			require.NoError(t, err)
			encoded := string(frame)
			if field == "assembled stack" {
				encoded = `{"function":"render","filename":"app.js","lineno":1,"colno":2} ` + encoded
			}
			raw, err := json.Marshal(map[string]any{"logs": []any{map[string]any{
				"message": message, "level": "error", "context": map[string]any{"type": typ, "stackFrames": encoded},
			}}})
			require.NoError(t, err)
			b, err := faro.Decode(raw, faro.Options{
				Now:         t0,
				ConsoleLogs: true,
			})
			require.NoError(t, err)
			require.Len(t, b.Logs, 1)
			l := b.Logs[0]
			assert.True(t, utf8.ValidString(l.ErrorType))
			assert.True(t, utf8.ValidString(l.Message))
			assert.True(t, utf8.ValidString(l.Stack))
			assert.LessOrEqual(t, len(l.ErrorType), 64)
			assert.LessOrEqual(t, len(l.Message), 1024)
			assert.LessOrEqual(t, len(l.Stack), 4096)
			e := newExporter(t, "127.0.0.1:1", newRecCounters(), nil)
			records := e.build(b, false)
			require.Len(t, records, 1)
			_, err = proto.Marshal(records[0].rec)
			require.NoError(t, err, "one truncated console value must not prevent protobuf encoding")
		})
	}
}

func TestConsoleErrorAttributesOnlyWhenPresent(t *testing.T) {
	e := newExporter(t, "127.0.0.1:1", newRecCounters(), nil)
	b := mkBeacon()
	b.Logs = []beacon.Log{{Level: "info", Message: "plain console line"}}
	records := e.build(b, false)
	require.Len(t, records, 1)
	attrs := simplify(records[0]).attrs
	assert.Equal(t, "info", attrs["console.level"])
	assert.NotContains(t, attrs, "error.type")
	assert.NotContains(t, attrs, "error.stack")
}

// Normalized console records have optional error context; queue drainage keeps
// the measurement independent of receiver availability and queue capacity.
func BenchmarkConsoleIngest(b *testing.B) {
	for name, log := range map[string]beacon.Log{
		"plain": {Level: "info", Message: "checkout opened"},
		"error": {Level: "error", Message: "payment failed", ErrorType: "TypeError", Stack: "checkout (app.js:12:3)"},
	} {
		b.Run(name, func(b *testing.B) {
			e := &Logs{
				transport: &transport{
					siteName: "s1",
					counters: benchmarkCounters{},
				},
				now: func() time.Time { return t0 },
				ch:  make(chan queued, queueCap),
			}
			event := mkBeacon()
			event.Logs = []beacon.Log{log}
			result := aggregate.Result{
				Observation:  event,
				Accepted:     true,
				Investigated: true,
			}
			b.ReportAllocs()
			b.ResetTimer()
			for b.Loop() {
				e.Ingest(event, result)
				<-e.ch
			}
		})
	}
}
