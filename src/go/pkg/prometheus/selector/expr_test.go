// SPDX-License-Identifier: GPL-3.0-or-later

package selector

import (
	"testing"

	"github.com/prometheus/prometheus/model/labels"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestExpr_Empty(t *testing.T) {
	tests := map[string]struct {
		expr     Expr
		expected bool
	}{
		"empty: both allow and deny": {
			expr:     Expr{Allow: []string{}, Deny: []string{}},
			expected: true,
		},
		"nil: both allow and deny": {
			expected: true,
		},
		"nil, empty: allow, deny": {
			expr:     Expr{Deny: []string{""}},
			expected: false,
		},
		"empty, nil: allow, deny": {
			expr:     Expr{Allow: []string{""}},
			expected: false,
		},
	}

	for name, test := range tests {
		t.Run(name, func(t *testing.T) {
			assert.Equal(t, test.expected, test.expr.Empty())
		})
	}
}

func TestExpr_Parse(t *testing.T) {
	tests := map[string]struct {
		expr        Expr
		series      labels.Labels
		expected    bool
		expectedNil bool
		expectedErr bool
	}{
		"not set: both allow and deny": {
			expr:        Expr{},
			expectedNil: true,
		},
		"set: both allow and deny": {
			expr: Expr{
				Allow: []string{"go_memstats_*", "node_*"},
				Deny:  []string{"go_memstats_frees_total", "node_cooling_*"},
			},
			expected: true,
			series:   []labels.Label{{Name: labels.MetricName, Value: "go_memstats_alloc_bytes"}},
		},
		"allow no match": {
			expr:     Expr{Allow: []string{"node_*"}},
			expected: false,
			series:   []labels.Label{{Name: labels.MetricName, Value: "go_memstats_alloc_bytes"}},
		},
		"invalid selector": {
			expr:        Expr{Allow: []string{`metric{label="x",}`}},
			expectedErr: true,
		},
	}

	for name, test := range tests {
		t.Run(name, func(t *testing.T) {
			m, err := test.expr.Parse()
			if test.expectedErr {
				require.Error(t, err)
				return
			}
			require.NoError(t, err)
			if test.expectedNil {
				assert.Nil(t, m)
				return
			}
			require.NotNil(t, m)
			assert.Equal(t, test.expected, m.Matches(test.series))
		})
	}
}

func TestExprSelector_Matches(t *testing.T) {
	tests := map[string]struct {
		expr            Expr
		lbs             labels.Labels
		expectedMatches bool
	}{
		"allow matches": {
			expr:            Expr{Allow: []string{"go_*"}},
			lbs:             []labels.Label{{Name: labels.MetricName, Value: "go_memstats_alloc_bytes"}},
			expectedMatches: true,
		},
		"deny matches": {
			expr:            Expr{Deny: []string{"go_*"}},
			lbs:             []labels.Label{{Name: labels.MetricName, Value: "go_memstats_alloc_bytes"}},
			expectedMatches: false,
		},
		"allow and deny": {
			expr:            Expr{Allow: []string{"go_*"}, Deny: []string{"go_*"}},
			lbs:             []labels.Label{{Name: labels.MetricName, Value: "go_memstats_alloc_bytes"}},
			expectedMatches: false,
		},
	}

	for name, test := range tests {
		t.Run(name, func(t *testing.T) {
			sr, err := test.expr.Parse()
			require.NoError(t, err)
			assert.Equal(t, test.expectedMatches, sr.Matches(test.lbs))
		})
	}
}
