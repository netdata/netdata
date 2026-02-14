// SPDX-License-Identifier: GPL-3.0-or-later

package selector

import (
	"testing"

	"github.com/prometheus/prometheus/model/labels"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestParseCompiledScenarios(t *testing.T) {
	tests := map[string]struct {
		expr               string
		wantErr            bool
		wantMetricNames    []string
		wantConstrained    []string
		matchMetricName    string
		matchLabels        map[string]string
		wantMatches        bool
		mismatchMetricName string
		wantMismatch       bool
	}{
		"extracts exact metric candidate and constrained keys": {
			expr:            `http_requests_total{job="api",instance="a,b"}`,
			wantMetricNames: []string{"http_requests_total"},
			wantConstrained: []string{"instance", "job"},
			matchMetricName: "http_requests_total",
			matchLabels: map[string]string{
				"job":      "api",
				"instance": "a,b",
			},
			wantMatches:        true,
			mismatchMetricName: "http_requests",
			wantMismatch:       false,
		},
		"selector without metric name has no metric candidates": {
			expr:            `{job="api"}`,
			wantMetricNames: nil,
			wantConstrained: []string{"job"},
			matchMetricName: "anything_total",
			matchLabels: map[string]string{
				"job": "api",
			},
			wantMatches: true,
		},
		"wildcard metric matcher does not produce metric candidates": {
			expr:            `foo*{job="api"}`,
			wantMetricNames: nil,
			wantConstrained: []string{"job"},
			matchMetricName: "bar_total",
			matchLabels: map[string]string{
				"job": "api",
			},
			wantMatches:        false,
			mismatchMetricName: "foo_requests",
			wantMismatch:       true,
		},
		"invalid syntax returns error": {
			expr:    `metric{label="value",}`,
			wantErr: true,
		},
	}

	for name, tc := range tests {
		t.Run(name, func(t *testing.T) {
			compiled, err := ParseCompiled(tc.expr)
			if tc.wantErr {
				require.Error(t, err)
				return
			}

			require.NoError(t, err)
			require.NotNil(t, compiled)

			meta := compiled.Meta()
			assert.Equal(t, tc.wantMetricNames, meta.MetricNames)
			assert.Equal(t, tc.wantConstrained, meta.ConstrainedLabelKeys)

			if tc.matchMetricName != "" {
				assert.Equal(t, tc.wantMatches, compiled.Matches(makePromLabels(tc.matchMetricName, tc.matchLabels)))
			}
			if tc.mismatchMetricName != "" {
				assert.Equal(t, tc.wantMismatch, compiled.Matches(makePromLabels(tc.mismatchMetricName, tc.matchLabels)))
			}
		})
	}
}

func makePromLabels(metricName string, lbs map[string]string) labels.Labels {
	out := make(labels.Labels, 0, len(lbs)+1)
	out = append(out, labels.Label{Name: labels.MetricName, Value: metricName})
	for k, v := range lbs {
		out = append(out, labels.Label{Name: k, Value: v})
	}
	return out
}
