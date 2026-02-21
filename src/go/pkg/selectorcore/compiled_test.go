// SPDX-License-Identifier: GPL-3.0-or-later

package selectorcore

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestParseCompiledScenarios(t *testing.T) {
	tests := map[string]struct {
		expr            string
		wantErr         bool
		wantMetricNames []string
		wantKeys        []string
		metric          string
		labels          mapLabels
		match           bool
	}{
		"extracts metric and keys": {
			expr:            `http_requests_total{job="api",instance="a"}`,
			wantMetricNames: []string{"http_requests_total"},
			wantKeys:        []string{"instance", "job"},
			metric:          "http_requests_total",
			labels:          mapLabels{"job": "api", "instance": "a"},
			match:           true,
		},
		"wildcard metric no exact candidates": {
			expr:            `http_*{job="api"}`,
			wantMetricNames: nil,
			wantKeys:        []string{"job"},
			metric:          "http_requests_total",
			labels:          mapLabels{"job": "api"},
			match:           true,
		},
		"invalid": {
			expr:    `metric{job="api",}`,
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
			assert.Equal(t, tc.wantKeys, meta.ConstrainedLabelKeys)
			assert.Equal(t, tc.match, compiled.Matches(tc.metric, tc.labels))
		})
	}
}
