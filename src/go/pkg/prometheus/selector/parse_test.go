// SPDX-License-Identifier: GPL-3.0-or-later

package selector

import (
	"testing"

	"github.com/prometheus/prometheus/model/labels"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestParse(t *testing.T) {
	tests := map[string]struct {
		expr      string
		series    labels.Labels
		wantMatch bool
		wantErr   bool
	}{
		"metric name only": {
			expr:      "go_memstats_alloc_bytes !go_memstats_* *",
			series:    labels.Labels{{Name: labels.MetricName, Value: "go_memstats_alloc_bytes"}},
			wantMatch: true,
		},
		"string op with labels": {
			expr: `go_memstats_*{label="value"}`,
			series: labels.Labels{
				{Name: labels.MetricName, Value: "go_memstats_alloc_bytes"},
				{Name: "label", Value: "value"},
			},
			wantMatch: true,
		},
		"neg string op with labels": {
			expr: `go_memstats_*{label!="value"}`,
			series: labels.Labels{
				{Name: labels.MetricName, Value: "go_memstats_alloc_bytes"},
				{Name: "label", Value: "value"},
			},
			wantMatch: false,
		},
		"regexp op with labels": {
			expr: `go_memstats_*{label=~"valu.+"}`,
			series: labels.Labels{
				{Name: labels.MetricName, Value: "go_memstats_alloc_bytes"},
				{Name: "label", Value: "value"},
			},
			wantMatch: true,
		},
		"only labels expression": {
			expr: `{__name__=*"go_memstats_*",label1="value1",label2="value2"}`,
			series: labels.Labels{
				{Name: labels.MetricName, Value: "go_memstats_alloc_bytes"},
				{Name: "label1", Value: "value1"},
				{Name: "label2", Value: "value2"},
			},
			wantMatch: true,
		},
		"invalid syntax": {
			expr:    `metric{label="value",}`,
			wantErr: true,
		},
	}

	for name, tc := range tests {
		t.Run(name, func(t *testing.T) {
			sr, err := Parse(tc.expr)
			if tc.wantErr {
				require.Error(t, err)
				return
			}
			require.NoError(t, err)
			require.NotNil(t, sr)
			assert.Equal(t, tc.wantMatch, sr.Matches(tc.series))
		})
	}
}

func TestParseEmptyExpressionReturnsNil(t *testing.T) {
	tests := map[string]struct {
		expr string
	}{
		"empty":      {expr: ""},
		"whitespace": {expr: "  \n\t"},
	}

	for name, tc := range tests {
		t.Run(name, func(t *testing.T) {
			sr, err := Parse(tc.expr)
			require.NoError(t, err)
			assert.Nil(t, sr)
		})
	}
}
