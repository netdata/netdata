// SPDX-License-Identifier: GPL-3.0-or-later

package measurement

import (
	"encoding/json"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

func TestScalarFallbackOnlyDiagnosesPresentInvalidSources(t *testing.T) {
	for name, test := range map[string]struct {
		preferred  map[string]any
		diagnostic string
	}{
		"document absent": {},
		"property absent": {preferred: map[string]any{}},
		"null":            {preferred: map[string]any{"BandwidthPercent": nil}, diagnostic: "property null"},
		"malformed":       {preferred: map[string]any{"BandwidthPercent": false}, diagnostic: "malformed"},
	} {
		t.Run(name, func(t *testing.T) {
			node := &Resource{
				Kind: "system",
				Key:  "system",
				Data: map[string]any{
					"ProcessorSummary": map[string]any{
						"Metrics": map[string]any{"BandwidthPercent": json.Number("42")},
					},
				},
				Enrichment: map[string]Enrichment{"processor_summary_metrics": {Data: test.preferred}},
			}
			values := fixtureClient().scalarValues(node, time.Unix(10, 0))
			require.Len(t, values, 1)
			require.True(t, values[0].Emit)
			require.Equal(t, float64(42), values[0].Value)
			if test.diagnostic == "" {
				require.Empty(t, values[0].SourceFailures)
			} else {
				require.Len(t, values[0].SourceFailures, 1)
				require.Contains(t, values[0].SourceFailures[0], test.diagnostic)
			}
		})
	}
}

func TestScalarFallbackRetainsAllInvalidSourceDiagnostics(t *testing.T) {
	node := &Resource{
		Kind: "system",
		Key:  "system",
		Data: map[string]any{"ProcessorSummary": map[string]any{"Metrics": map[string]any{"BandwidthPercent": false}}},
		Enrichment: map[string]Enrichment{
			"processor_summary_metrics": {Data: map[string]any{"BandwidthPercent": nil}},
		},
	}
	values := (New("", nil)).scalarValues(node, time.Now())
	require.Len(t, values, 1)
	require.False(t, values[0].Valid)
	require.False(t, values[0].Emit)
	require.Equal(t, []string{
		"Redfish compatibility: scalar system_processorsummary_metrics_bandwidthpercent preferred source processor_summary_metrics.BandwidthPercent: property null",
		"Redfish compatibility: scalar system_processorsummary_metrics_bandwidthpercent preferred source ProcessorSummary.Metrics.BandwidthPercent: value malformed, unsupported, or non-finite",
	}, values[0].SourceFailures)
}
