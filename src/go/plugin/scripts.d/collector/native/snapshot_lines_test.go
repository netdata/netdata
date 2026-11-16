// SPDX-License-Identifier: GPL-3.0-or-later

package native

import (
	"encoding/json"
	"fmt"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestLines_EquivalentJSON(t *testing.T) {
	cases := map[string]struct{ lines, json string }{
		"defaults": {
			"temperature:-3.5|gauge",
			`{"version":"v1","metrics":[{"name":"temperature","samples":[{"value":-3.5}]}]}`,
		},
		"interleaved metadata": {
			"depth:17|gauge|#queue:mail\nrequests:123|counter|unit:requests\ndepth:4|gauge|title:Queue depth|#queue:batch|unit:jobs|family:Queue|priority:100\n",
			`{"version":"v1","metrics":[{"name":"depth","unit":"jobs","chart_meta":{"title":"Queue depth","family":"Queue","priority":100},"samples":[{"value":17,"labels":{"queue":"mail"}},{"value":4,"labels":{"queue":"batch"}}]},{"name":"requests","type":"counter","unit":"requests","samples":[{"value":123}]}]}`,
		},
		"quoted delimiters and empty label": {
			`x:1e-3|gauge|#a:"",b:"comma, pipe| colon: \" quote \\ slash\nline"|title:" X | Y "`,
			`{"version":"v1","metrics":[{"name":"x","chart_meta":{"title":" X | Y "},"samples":[{"value":0.001,"labels":{"a":"","b":"comma, pipe| colon: \" quote \\ slash\nline"}}]}]}`,
		},
		"literal strings and nd labels": {
			`queue.depth:0|counter|#a:C:\spool,b:inner "quote",nd_unit:plain label`,
			`{"version":"v1","metrics":[{"name":"queue.depth","type":"counter","samples":[{"value":0,"labels":{"a":"C:\\spool","b":"inner \"quote\"","nd_unit":"plain label"}}]}]}`,
		},
		"comments and crlf": {
			" \t# observation\r\n\r\n x:1|gauge \t\r\n",
			`{"version":"v1","metrics":[{"name":"x","samples":[{"value":1}]}]}`,
		},
		"empty observation": {"# no entities\n", `{"version":"v1"}`},
		"decoded metadata agrees": {
			`x:1|gauge|#id:a|title:"\u0051ueue"` + "\n" + `x:2|gauge|#id:b|title:Queue`,
			`{"version":"v1","metrics":[{"name":"x","chart_meta":{"title":"Queue"},"samples":[{"value":1,"labels":{"id":"a"}},{"value":2,"labels":{"id":"b"}}]}]}`,
		},
	}
	for name, tc := range cases {
		t.Run(name, func(t *testing.T) {
			want, err := decodeSnapshot([]byte(tc.json))
			require.NoError(t, err)
			got, err := decodeLines([]byte(tc.lines))
			require.NoError(t, err)
			assert.Equal(t, want, got)
		})
	}
}

func TestLines_JSONStringEncoders(t *testing.T) {
	for _, value := range []string{"", "café 😀", "<worker&queue>", "a,|:\"\\\n\r\t\b\f\x00", " space "} {
		t.Run(fmt.Sprintf("%q", value), func(t *testing.T) {
			goEncoded, err := json.Marshal(value)
			require.NoError(t, err)
			pythonEncoded := `"` + jsonEscape(value) + `"`
			for _, encoded := range []string{string(goEncoded), pythonEncoded} {
				got, err := decodeLines([]byte("x:1|gauge|#label:" + encoded))
				require.NoError(t, err)
				assert.Equal(t, value, got.Metrics[0].Samples[0].Labels["label"])
			}
		})
	}
	got, err := decodeLines([]byte(`x:1|gauge|#label:"\/"`))
	require.NoError(t, err)
	assert.Equal(t, "/", got.Metrics[0].Samples[0].Labels["label"])
}

func TestLines_InvalidSnapshot(t *testing.T) {
	cases := map[string]string{
		"empty": "", "whitespace": " \t\n\r\n", "invalid utf8": "x:1|gauge|#a:\xff",
		"invalid name": "1x:1|gauge", "native reserved": "native.x:1|gauge", "check reserved": "native_check_x:1|gauge", "check state reserved": "check_state:1|gauge",
		"missing type": "x:1", "short gauge": "x:1|g", "short counter": "x:1|c", "timer": "x:1|ms", "stateset": "x:1|stateset", "check": "x:ok|check",
		"negative counter": "x:-1|counter", "nan": "x:NaN|gauge", "inf": "x:Inf|gauge", "overflow": "x:1e999|gauge", "plus": "x:+5|gauge", "leading decimal": "x:.5|gauge", "leading zero": "x:01|gauge", "number space": "x: 1|gauge",
		"sampling": "x:1|counter|@0.5", "timestamp": "x:1|gauge|T123", "unknown field": "x:1|gauge|foo:bar", "empty field": "x:1|gauge||unit:jobs", "trailing separator": "x:1|gauge|",
		"empty labels": "x:1|gauge|#", "missing label value": "x:1|gauge|#a:", "trailing comma": "x:1|gauge|#a:x,", "duplicate label": "x:1|gauge|#a:x,a:y", "reserved label": "x:1|gauge|#_collect_job:x", "bad key": "x:1|gauge|#a.b:x", "duplicate labels field": "x:1|gauge|#a:x|#b:y",
		"empty metadata": "x:1|gauge|unit:", "quoted empty metadata": `x:1|gauge|title:""`, "blank unit": `x:1|gauge|unit:" "`, "duplicate metadata": "x:1|gauge|unit:jobs|unit:jobs",
		"metadata conflict": "x:1|gauge|#id:a|unit:jobs\nx:2|gauge|#id:b|unit:items", "type conflict": "x:1|gauge|#id:a\nx:2|counter|#id:b",
		"duplicate identity": "x:1|gauge|#a:x,b:y\nx:2|gauge|#b:y,a:x", "duplicate unlabeled": "x:1|gauge\nx:2|gauge",
		"priority zero": "x:1|gauge|priority:0", "priority overflow": "x:1|gauge|priority:999999999999999999999", "priority float": "x:1|gauge|priority:1.5",
		"raw tab": "x:1|gauge|#a:a\tb", "raw cr": "x:1|gauge|#a:a\rb", "raw nul": "x:1|gauge|#a:a\x00b", "raw del": "x:1|gauge|#a:a\x7fb",
		"label whitespace": "x:1|gauge|#a: x", "unfinished quote": `x:1|gauge|#a:"x`, "unknown escape": `x:1|gauge|#a:"\x"`, "invalid unicode": `x:1|gauge|#a:"\u0xxx"`, "trailing quote content": `x:1|gauge|#a:"x"y`,
		"json": `{"version":"v1"}`, "malformed after valid": "x:1|gauge\nSYNTHETIC_SECRET",
	}
	for name, data := range cases {
		t.Run(name, func(t *testing.T) {
			_, err := decodeLines([]byte(data))
			require.Error(t, err)
			assert.NotContains(t, err.Error(), "SYNTHETIC_SECRET")
		})
	}
}

func FuzzLines(f *testing.F) {
	for _, data := range []string{"x:1|gauge", `x:1|gauge|#a:"x,|\\"`, "# empty", "x:1|counter|unit:jobs\nx:2|counter|#id:a"} {
		f.Add(data)
	}
	f.Fuzz(func(t *testing.T, data string) {
		snap, err := decodeLines([]byte(data))
		if err != nil {
			return
		}
		encoded, err := json.Marshal(struct {
			Version string         `json:"version"`
			Metrics []metricFamily `json:"metrics,omitempty"`
		}{Version: snap.Version, Metrics: snap.Metrics})
		require.NoError(t, err)
		roundtrip, err := decodeSnapshot(encoded)
		require.NoError(t, err)
		assert.Equal(t, snap, roundtrip)
	})
}

// Parsing and normalization scale with current bytes/samples, not earlier
// snapshots. Timings are development-host trends; allocations expose growth.
func BenchmarkLinesDecode(b *testing.B) {
	for _, count := range []int{1, 1000} {
		for _, grouped := range []bool{true, false} {
			b.Run(fmt.Sprintf("samples=%d/grouped=%t", count, grouped), func(b *testing.B) {
				var data strings.Builder
				for i := range count {
					name := "metric"
					if !grouped {
						name = fmt.Sprintf("metric_%d", i)
					}
					fmt.Fprintf(&data, "%s:1.5|gauge|#id:%d|unit:jobs\n", name, i)
				}
				wire := []byte(data.String())
				b.ReportAllocs()
				b.ResetTimer()
				for b.Loop() {
					if _, err := decodeLines(wire); err != nil {
						b.Fatal(err)
					}
				}
			})
		}
	}
}
