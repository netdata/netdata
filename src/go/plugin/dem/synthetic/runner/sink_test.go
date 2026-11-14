// SPDX-License-Identifier: GPL-3.0-or-later

package runner

import (
	"encoding/json"
	"strings"
	"testing"

	"github.com/netdata/netdata/go/plugins/plugin/dem/synthetic"
	"github.com/stretchr/testify/require"
)

// BenchmarkSinkConsume measures the stream reader on one attempt's output, with
// and without optional outputs. The sink runs once per browser attempt, so this
// is a development trend, not a per-cycle budget.
func BenchmarkSinkConsume(b *testing.B) {
	var core strings.Builder
	for range 30 {
		emit(&core, frame{Type: "event", Event: &synthetic.Event{Kind: "step", AtMS: 1, Title: "normal step", Status: "passed"}})
	}
	emit(&core, frame{Type: "result", Result: new(success())})
	browser := []string{browserInit}
	for n := int64(1); n <= 64; n++ {
		browser = append(browser, extensionLine(browserSummaryFeature, browserTestRecord(n)))
	}
	browser = append(browser, extensionLine(browserSummaryFeature, map[string]any{"record": "complete", "run": browserRun(64)}))
	evidence := []string{evidenceInit, evidenceRecord("test", "test", evidenceStart("t1")), evidenceRecord("step", "step", evidenceStep("s1", ""))}
	// status checks that each stream still reaches the path it measures.
	tests := map[string]struct {
		request synthetic.Request
		input   string
		status  func(sinkSnapshot) string
		want    string
	}{
		"core": {
			request: synthetic.Request{Kind: synthetic.Journey},
			input:   core.String(),
			status:  func(s sinkSnapshot) string { return s.evidence.ReportingStatus() },
			want:    "unavailable",
		},
		"failure evidence": {
			request: synthetic.Request{Kind: synthetic.Journey},
			input:   strings.Join(evidence, "\n") + "\n" + core.String(),
			status:  func(s sinkSnapshot) string { return s.evidence.ReportingStatus() },
			want:    "partial",
		},
		"browser summary 64 rows": {
			request: synthetic.Request{Kind: synthetic.Journey, BrowserErrorSummary: true},
			input:   strings.Join(browser, "\n") + "\n" + core.String(),
			status:  func(s sinkSnapshot) string { return s.browser.ReportingStatus() },
			want:    "complete",
		},
	}
	for name, tc := range tests {
		b.Run(name, func(b *testing.B) {
			b.ReportAllocs()
			b.SetBytes(int64(len(tc.input)))
			for b.Loop() {
				s := newSink(tc.request, newRedactor(nil))
				if err := s.consume(strings.NewReader(tc.input)); err != nil {
					b.Fatal(err)
				}
				snapshot := s.snapshot()
				if snapshot.err != nil || snapshot.result == nil || tc.status(snapshot) != tc.want {
					b.Fatalf("stream did not reach the measured path: err=%v status=%q", snapshot.err, tc.status(snapshot))
				}
			}
		})
	}
}

// TestStreamEndFlushOrderIsStable flushes unterminated diagnostic lines at end
// of stream in a fixed order, so retention at the event cap is reproducible.
func TestStreamEndFlushOrderIsStable(t *testing.T) {
	var lines []string
	for _, source := range []struct{ kind, phase string }{{"stdout", "worker"}, {"stderr", "cli"}, {"stdout", "future"}, {"stderr", "worker"}, {"stdout", "cli"}} {
		event := synthetic.Event{Kind: source.kind, Phase: source.phase, Message: source.kind + "/" + source.phase + " unterminated"}
		raw, err := json.Marshal(frame{Type: "event", Event: &event})
		require.NoError(t, err)
		lines = append(lines, string(raw))
	}
	var first []string
	for i := range 30 {
		snapshot := consume(t, synthetic.Request{Kind: synthetic.Journey}, nil, lines...)
		var order []string
		for _, event := range snapshot.events {
			order = append(order, event.Message)
		}
		if i == 0 {
			first = order
			require.Len(t, first, 5)
			continue
		}
		require.Equal(t, first, order, "flush order changed between identical streams")
	}
}
