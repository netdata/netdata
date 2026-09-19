// SPDX-License-Identifier: GPL-3.0-or-later

package measurement

import (
	"testing"
)

// Label construction is O(the fixed label inventory); timing is a local trend,
// while allocation counts measure the per-observation overhead.
func BenchmarkMetricLabels(b *testing.B) {
	client := fixtureClient()
	node := &Resource{
		Kind: "sensor",
		Key:  "sensor",
		Doc: Document{
			Name: "Temperature",
		},
	}
	reading := &normalizedReading{
		Key:    "reading",
		Family: "temperature",
		Basis:  "zero",
		Role:   "input",
	}
	b.ReportAllocs()
	for b.Loop() {
		if len(client.metricLabels(node, reading)) != 10 {
			b.Fatal("labels missing")
		}
	}
}
