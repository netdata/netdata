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

// TestStreamEndFlushOrder flushes unterminated diagnostic lines at end of
// stream in stream-key order, which also decides the tails the event cap keeps.
func TestStreamEndFlushOrder(t *testing.T) {
	line := func(event synthetic.Event) string {
		raw, err := json.Marshal(frame{Type: "event", Event: &event})
		require.NoError(t, err)
		return string(raw)
	}
	var tails []string
	for _, source := range []struct{ kind, phase string }{{"stdout", "worker"}, {"stderr", "cli"}, {"stdout", "future"}, {"stderr", "worker"}, {"stdout", "cli"}} {
		tails = append(tails, line(synthetic.Event{Kind: source.kind, Phase: source.phase, Message: source.kind + "/" + source.phase + " unterminated"}))
	}
	// Keys are kind:source; unknown phases share the protocol stream.
	order := []string{"stderr/cli unterminated\n", "stderr/worker unterminated\n", "stdout/cli unterminated\n", "stdout/future unterminated\n", "stdout/worker unterminated\n"}
	var filler []string
	for range eventLimit - 2 {
		filler = append(filler, line(synthetic.Event{Kind: "step", Title: "filler"}))
	}
	tests := map[string]struct {
		lines   []string
		kept    []string
		dropped int
	}{
		"all retained": {lines: tails, kept: order},
		"at the cap":   {lines: append(filler, tails...), kept: order[:2], dropped: 3},
	}
	for name, tc := range tests {
		t.Run(name, func(t *testing.T) {
			// Map iteration would vary between runs; repeat to catch it.
			for range 30 {
				snapshot := consume(t, synthetic.Request{Kind: synthetic.Journey}, nil, tc.lines...)
				var flushed []string
				for _, event := range snapshot.events[len(snapshot.events)-len(tc.kept):] {
					flushed = append(flushed, event.Message)
				}
				require.Equal(t, tc.kept, flushed)
				require.Equal(t, tc.dropped, snapshot.dropped)
			}
		})
	}
}
