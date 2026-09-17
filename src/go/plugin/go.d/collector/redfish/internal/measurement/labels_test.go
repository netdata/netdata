// SPDX-License-Identifier: GPL-3.0-or-later

package measurement

import (
	"testing"

	"github.com/stretchr/testify/require"
)

func TestComponentFamilyLabelUsesKnownKindsOnly(t *testing.T) {
	client := New("", "", nil)
	for kind := range sourceStatusByKind {
		require.Equal(t, kind, observationLabel(client.metricLabels(&Resource{
			Kind: kind,
		}, nil), "component_family"))
	}
	require.Empty(
		t,
		observationLabel(client.metricLabels(&Resource{
			Kind: "vendor_extension",
		}, nil), "component_family"),
	)
}

// Label construction is O(the fixed label inventory); timing is a local trend,
// while allocation counts measure the per-observation overhead.
func BenchmarkMetricLabels(b *testing.B) {
	client := fixtureClient()
	client.endpointJob = "hardware"
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
