// SPDX-License-Identifier: GPL-3.0-or-later

package selectorcore

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

type mapLabels map[string]string

func (m mapLabels) Get(key string) (string, bool) {
	v, ok := m[key]
	return v, ok
}

func TestParseScenarios(t *testing.T) {
	tests := map[string]struct {
		expr    string
		metric  string
		labels  mapLabels
		match   bool
		wantErr bool
	}{
		"metric name only": {
			expr:   "go_memstats_alloc_bytes",
			metric: "go_memstats_alloc_bytes",
			match:  true,
		},
		"metric and label exact": {
			expr:   `http_requests_total{job="api"}`,
			metric: "http_requests_total",
			labels: mapLabels{"job": "api"},
			match:  true,
		},
		"metric and label mismatch": {
			expr:   `http_requests_total{job="api"}`,
			metric: "http_requests_total",
			labels: mapLabels{"job": "db"},
			match:  false,
		},
		"only labels": {
			expr:   `{job="api"}`,
			metric: "anything",
			labels: mapLabels{"job": "api"},
			match:  true,
		},
		"invalid syntax": {
			expr:    `metric{job="api",}`,
			wantErr: true,
		},
	}

	for name, tc := range tests {
		t.Run(name, func(t *testing.T) {
			s, err := Parse(tc.expr)
			if tc.wantErr {
				require.Error(t, err)
				return
			}
			require.NoError(t, err)
			require.NotNil(t, s)
			assert.Equal(t, tc.match, s.Matches(tc.metric, tc.labels))
		})
	}
}

func TestParseEmptyExpressionReturnsNil(t *testing.T) {
	tests := map[string]struct {
		expr string
	}{
		"empty":      {expr: ""},
		"whitespace": {expr: "   \t\n"},
	}

	for name, tc := range tests {
		t.Run(name, func(t *testing.T) {
			s, err := Parse(tc.expr)
			require.NoError(t, err)
			assert.Nil(t, s)
		})
	}
}
