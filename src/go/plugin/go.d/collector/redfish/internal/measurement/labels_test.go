// SPDX-License-Identifier: GPL-3.0-or-later

package measurement

import (
	"testing"

	"github.com/netdata/netdata/go/plugins/pkg/metrix"
	"github.com/stretchr/testify/require"
)

// The exported key lists must describe exactly what metricLabels attaches when a
// resource and a reading report every field.
func TestMetricLabelKeys(t *testing.T) {
	node, reading := fullyLabeledFixture()
	keys := func(labels []metrix.Label) []string {
		out := make([]string, 0, len(labels))
		for _, label := range labels {
			out = append(out, label.Key)
		}
		return out
	}
	client := fixtureClient()
	require.Equal(t, ResourceLabelKeys, keys(client.metricLabels(node, nil)))
	require.Equal(t, ReadingLabelKeys, keys(client.metricLabels(node, reading)))
}

// fullyLabeledFixture reports every field metricLabels can attach.
func fullyLabeledFixture() (*Resource, *normalizedReading) {
	node := &Resource{
		Kind: "processor",
		Key:  "cpu-1",
		Doc:  Document{Name: "CPU 1"},
		Data: map[string]any{
			"Manufacturer": "Acme",
			"Model":        "X1",
			"SerialNumber": "SN1",
			"Socket":       "CPU1",
			"Location":     map[string]any{"PartLocation": map[string]any{"ServiceLabel": "Front"}},
		},
	}
	reading := &normalizedReading{
		Key:                "reading-1",
		PhysicalContext:    "CPU",
		PhysicalSubcontext: "Intake",
		Family:             "temperature",
		Basis:              "zero",
		Role:               "input",
	}
	return node, reading
}

// Label construction is O(the fixed label inventory); timing is a local trend,
// while allocation counts measure the per-observation overhead.
func BenchmarkMetricLabels(b *testing.B) {
	client := fixtureClient()
	node, reading := fullyLabeledFixture()
	b.ReportAllocs()
	for b.Loop() {
		if len(client.metricLabels(node, reading)) != len(ReadingLabelKeys) {
			b.Fatal("labels missing")
		}
	}
}
