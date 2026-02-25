// SPDX-License-Identifier: GPL-3.0-or-later

package selector

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestSelectorLogicalCombinators_TruthTable(t *testing.T) {
	type selectorBuilder func(t *testing.T) Selector

	tests := map[string]struct {
		build  selectorBuilder
		metric string
		labels mapLabelView
		want   bool
	}{
		"true always matches": {
			build: func(_ *testing.T) Selector { return True() },
			want:  true,
		},
		"not true is false": {
			build: func(_ *testing.T) Selector { return Not(True()) },
			want:  false,
		},
		"and true true false is false": {
			build: func(_ *testing.T) Selector {
				return And(True(), True(), Not(True()))
			},
			want: false,
		},
		"or false false true is true": {
			build: func(_ *testing.T) Selector {
				return Or(Not(True()), Not(True()), True())
			},
			want: true,
		},
		"and with metric selector matches only when base selector matches": {
			build: func(t *testing.T) Selector {
				sel, err := Parse(`http_requests_total{job="api"}`)
				require.NoError(t, err)
				require.NotNil(t, sel)
				return And(True(), sel)
			},
			metric: "http_requests_total",
			labels: mapLabelView{"job": "api"},
			want:   true,
		},
		"or with negated selector keeps mismatch path true": {
			build: func(t *testing.T) Selector {
				sel, err := Parse(`http_requests_total{job="api"}`)
				require.NoError(t, err)
				require.NotNil(t, sel)
				return Or(sel, Not(sel))
			},
			metric: "http_requests_total",
			labels: mapLabelView{"job": "db"},
			want:   true,
		},
	}

	for name, tc := range tests {
		t.Run(name, func(t *testing.T) {
			sel := tc.build(t)
			require.NotNil(t, sel)
			assert.Equal(t, tc.want, sel.Matches(tc.metric, tc.labels))
		})
	}
}
