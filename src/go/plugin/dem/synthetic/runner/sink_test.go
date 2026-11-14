// SPDX-License-Identifier: GPL-3.0-or-later

package runner

import (
	"strings"
	"testing"

	"github.com/netdata/netdata/go/plugins/plugin/dem/synthetic"
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
	tests := map[string]struct {
		request synthetic.Request
		input   string
	}{
		"core":                    {request: synthetic.Request{Kind: synthetic.Journey}, input: core.String()},
		"failure evidence":        {request: synthetic.Request{Kind: synthetic.Journey}, input: strings.Join(evidence, "\n") + "\n" + core.String()},
		"browser summary 64 rows": {request: synthetic.Request{Kind: synthetic.Journey, BrowserErrorSummary: true}, input: strings.Join(browser, "\n") + "\n" + core.String()},
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
				if snapshot := s.snapshot(); snapshot.err != nil || snapshot.result == nil {
					b.Fatal(snapshot.err)
				}
			}
		})
	}
}
