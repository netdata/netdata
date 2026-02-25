// SPDX-License-Identifier: GPL-3.0-or-later

package selector

import (
	"sort"
	"testing"

	"github.com/netdata/netdata/go/plugins/pkg/metrix"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

type mapLabelView map[string]string

func (m mapLabelView) Len() int { return len(m) }

func (m mapLabelView) Get(key string) (string, bool) {
	v, ok := m[key]
	return v, ok
}

func (m mapLabelView) Range(fn func(key, value string) bool) {
	keys := make([]string, 0, len(m))
	for key := range m {
		keys = append(keys, key)
	}
	sort.Strings(keys)
	for _, key := range keys {
		if !fn(key, m[key]) {
			return
		}
	}
}

func (m mapLabelView) CloneMap() map[string]string {
	out := make(map[string]string, len(m))
	for key, value := range m {
		out[key] = value
	}
	return out
}

var _ metrix.LabelView = mapLabelView{}

func TestParseScenarios(t *testing.T) {
	tests := map[string]struct {
		expr      string
		metric    string
		labels    mapLabelView
		wantMatch bool
		wantErr   bool
	}{
		"metric only": {
			expr:      "http_requests_total",
			metric:    "http_requests_total",
			wantMatch: true,
		},
		"metric plus label": {
			expr:      `http_requests_total{job="api"}`,
			metric:    "http_requests_total",
			labels:    mapLabelView{"job": "api"},
			wantMatch: true,
		},
		"invalid": {
			expr:    `metric{job="api",}`,
			wantErr: true,
		},
	}

	for name, tc := range tests {
		t.Run(name, func(t *testing.T) {
			sel, err := Parse(tc.expr)
			if tc.wantErr {
				require.Error(t, err)
				return
			}
			require.NoError(t, err)
			require.NotNil(t, sel)
			assert.Equal(t, tc.wantMatch, sel.Matches(tc.metric, tc.labels))
		})
	}
}

func TestParseEmptyExpressionReturnsNil(t *testing.T) {
	tests := map[string]struct {
		expr string
	}{
		"empty":      {expr: ""},
		"whitespace": {expr: "  \t\n"},
	}

	for name, tc := range tests {
		t.Run(name, func(t *testing.T) {
			sel, err := Parse(tc.expr)
			require.NoError(t, err)
			assert.Nil(t, sel)
		})
	}
}

func TestExprScenarios(t *testing.T) {
	expr := Expr{
		Allow: []string{"go_*"},
		Deny:  []string{"go_gc_*"},
	}
	sel, err := expr.Parse()
	require.NoError(t, err)
	require.NotNil(t, sel)

	assert.True(t, sel.Matches("go_memstats_alloc_bytes", nil))
	assert.False(t, sel.Matches("go_gc_duration_seconds", nil))
	assert.False(t, sel.Matches("node_cpu_seconds_total", nil))
}

func TestParseCompiledScenarios(t *testing.T) {
	compiled, err := ParseCompiled(`http_requests_total{job="api",instance="a"}`)
	require.NoError(t, err)
	require.NotNil(t, compiled)

	meta := compiled.Meta()
	assert.Equal(t, []string{"http_requests_total"}, meta.MetricNames)
	assert.Equal(t, []string{"instance", "job"}, meta.ConstrainedLabelKeys)
	assert.True(t, compiled.Matches("http_requests_total", mapLabelView{"job": "api", "instance": "a"}))
	assert.False(t, compiled.Matches("http_requests_total", mapLabelView{"job": "api", "instance": "b"}))
}
