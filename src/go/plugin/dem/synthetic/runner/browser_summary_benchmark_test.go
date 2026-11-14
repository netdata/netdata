// SPDX-License-Identifier: GPL-3.0-or-later
package runner

import (
	"encoding/json"
	"strings"
	"testing"

	"github.com/netdata/netdata/go/plugins/plugin/dem/synthetic"
)

// Exercise the production sink and snapshot boundary without browser execution.
// Results describe decoder cost on this machine, not whole-run browser overhead.
func BenchmarkBrowserSummaryStream(b *testing.B) {
	for _, rows := range []int{0, 64} {
		name := "clean"
		if rows > 0 {
			name = "64_rows"
		}
		b.Run(name, func(b *testing.B) {
			lines := []string{browserInit}
			for n := 1; n <= rows; n++ {
				data := browserTestRecord(int64(n))
				raw, err := json.Marshal(map[string]any{"type": "extension", "extension": map[string]any{"feature": browserSummaryFeature, "data": data}})
				if err != nil {
					b.Fatal(err)
				}
				lines = append(lines, string(raw))
			}
			raw, err := json.Marshal(map[string]any{"type": "extension", "extension": map[string]any{"feature": browserSummaryFeature, "data": map[string]any{"record": "complete", "run": browserRun(int64(rows))}}})
			if err != nil {
				b.Fatal(err)
			}
			lines = append(lines, string(raw))
			input := strings.Join(lines, "\n")
			b.ReportAllocs()
			b.SetBytes(int64(len(input)))
			b.ResetTimer()
			for b.Loop() {
				s := newSink(synthetic.Request{Kind: synthetic.Journey, BrowserErrorSummary: true}, newRedactor(nil))
				if err := s.consume(strings.NewReader(input)); err != nil {
					b.Fatal(err)
				}
				snapshot := s.snapshot()
				if snapshot.browser.ReportingStatus() != "complete" {
					b.Fatal(snapshot.browser)
				}
			}
		})
	}
}
