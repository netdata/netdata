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
			expr: Expr{
				Allow: []string{},
				Deny:  []string{},
			},
			expected: true,
		},
		"nil: both allow and deny": {
			expected: true,
		},
		"nil, empty: allow, deny": {
			expr: Expr{
				Deny: []string{""},
			},
			expected: false,
		},
		"empty, nil: allow, deny": {
			expr: Expr{
				Allow: []string{""},
			},
			expected: false,
		},
	}

	for name, test := range tests {
		t.Run(name, func(t *testing.T) {
			if test.expected {
				assert.True(t, test.expr.Empty())
			} else {
				assert.False(t, test.expr.Empty())
			}
		})
	}
}

func TestExpr_Parse(t *testing.T) {
	tests := map[string]struct {
		expr        Expr
		expectedSr  Selector
		expectedErr bool
	}{
		"not set: both allow and deny": {
			expr: Expr{},
		},
		"set: both allow and deny": {
			expr: Expr{
				Allow: []string{
					"go_memstats_*",
					"node_*",
				},
				Deny: []string{
					"go_memstats_frees_total",
					"node_cooling_*",
				},
			},
			expectedSr: andSelector{
				lhs: orSelector{
					lhs: mustSPName("go_memstats_*"),
					rhs: mustSPName("node_*"),
				},
				rhs: Not(orSelector{
					lhs: mustSPName("go_memstats_frees_total"),
					rhs: mustSPName("node_cooling_*"),
				}),
			},
		},
		"set: only includes": {
			expr: Expr{
				Allow: []string{
					"go_memstats_*",
					"node_*",
				},
			},
			expectedSr: andSelector{
				lhs: orSelector{
					lhs: mustSPName("go_memstats_*"),
					rhs: mustSPName("node_*"),
				},
				rhs: Not(falseSelector{}),
			},
		},
		"set: only excludes": {
			expr: Expr{
				Deny: []string{
					"go_memstats_frees_total",
					"node_cooling_*",
				},
			},
			expectedSr: andSelector{
				lhs: trueSelector{},
				rhs: Not(orSelector{
					lhs: mustSPName("go_memstats_frees_total"),
					rhs: mustSPName("node_cooling_*"),
				}),
			},
		},
	}

	for name, test := range tests {
		t.Run(name, func(t *testing.T) {
			m, err := test.expr.Parse()

			if test.expectedErr {
				assert.Error(t, err)
			} else {
				assert.Equal(t, test.expectedSr, m)
			}
		})
	}
}

func TestExprSelector_Matches(t *testing.T) {
	tests := map[string]struct {
		expr            Expr
		lbs             labels.Labels
		expectedMatches bool
	}{
		"allow matches: single pattern": {
			expr: Expr{
				Allow: []string{"go_*"},
			},
			lbs:             []labels.Label{{Name: labels.MetricName, Value: "go_memstats_alloc_bytes"}},
			expectedMatches: true,
		},
		"allow matches: several patterns": {
			expr: Expr{
				Allow: []string{"node_*", "go_*"},
			},
			lbs:             []labels.Label{{Name: labels.MetricName, Value: "go_memstats_alloc_bytes"}},
			expectedMatches: true,
		},
		"allow not matches": {
			expr: Expr{
				Allow: []string{"node_*"},
			},
			lbs:             []labels.Label{{Name: labels.MetricName, Value: "go_memstats_alloc_bytes"}},
			expectedMatches: false,
		},
		"deny matches: single pattern": {
			expr: Expr{
				Deny: []string{"go_*"},
			},
			lbs:             []labels.Label{{Name: labels.MetricName, Value: "go_memstats_alloc_bytes"}},
			expectedMatches: false,
		},
		"deny matches: several patterns": {
			expr: Expr{
				Deny: []string{"node_*", "go_*"},
			},
			lbs:             []labels.Label{{Name: labels.MetricName, Value: "go_memstats_alloc_bytes"}},
			expectedMatches: false,
		},
		"deny not matches": {
			expr: Expr{
				Deny: []string{"node_*"},
			},
			lbs:             []labels.Label{{Name: labels.MetricName, Value: "go_memstats_alloc_bytes"}},
			expectedMatches: true,
		},
		"allow and deny matches: single pattern": {
			expr: Expr{
				Allow: []string{"go_*"},
				Deny:  []string{"go_*"},
			},
			lbs:             []labels.Label{{Name: labels.MetricName, Value: "go_memstats_alloc_bytes"}},
			expectedMatches: false,
		},
		"allow and deny matches: several patterns": {
			expr: Expr{
				Allow: []string{"node_*", "go_*"},
				Deny:  []string{"node_*", "go_*"},
			},
			lbs:             []labels.Label{{Name: labels.MetricName, Value: "go_memstats_alloc_bytes"}},
			expectedMatches: false,
		},
		"allow matches and deny not matches": {
			expr: Expr{
				Allow: []string{"go_*"},
				Deny:  []string{"node_*"},
			},
			lbs:             []labels.Label{{Name: labels.MetricName, Value: "go_memstats_alloc_bytes"}},
			expectedMatches: true,
		},
		"allow not matches and deny matches": {
			expr: Expr{
				Allow: []string{"node_*"},
				Deny:  []string{"go_*"},
			},
			lbs:             []labels.Label{{Name: labels.MetricName, Value: "go_memstats_alloc_bytes"}},
			expectedMatches: false,
		},
		"allow not matches and deny not matches": {
			expr: Expr{
				Allow: []string{"node_*"},
				Deny:  []string{"node_*"},
			},
			lbs:             []labels.Label{{Name: labels.MetricName, Value: "go_memstats_alloc_bytes"}},
			expectedMatches: false,
		},
	}

	for name, test := range tests {
		t.Run(name, func(t *testing.T) {
			sr, err := test.expr.Parse()
			require.NoError(t, err)

			if test.expectedMatches {
				assert.True(t, sr.Matches(test.lbs))
			} else {
				assert.False(t, sr.Matches(test.lbs))
			}
		})
	}
}
