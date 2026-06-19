// SPDX-License-Identifier: GPL-3.0-or-later

package selectorcore

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestExprEmpty(t *testing.T) {
	assert.True(t, Expr{}.Empty())
	assert.False(t, Expr{Allow: []string{"foo"}}.Empty())
	assert.False(t, Expr{Deny: []string{"foo"}}.Empty())
}

func TestExprParseScenarios(t *testing.T) {
	tests := map[string]struct {
		expr    Expr
		metric  string
		labels  mapLabels
		match   bool
		wantErr bool
	}{
		"allow only": {
			expr:   Expr{Allow: []string{"go_*"}},
			metric: "go_gc_duration_seconds",
			match:  true,
		},
		"deny only": {
			expr:   Expr{Deny: []string{"go_*"}},
			metric: "go_gc_duration_seconds",
			match:  false,
		},
		"allow and deny": {
			expr:   Expr{Allow: []string{"go_*"}, Deny: []string{"go_gc_*"}},
			metric: "go_gc_duration_seconds",
			match:  false,
		},
		"invalid selector": {
			expr:    Expr{Allow: []string{"metric{a=\"x\",}"}},
			wantErr: true,
		},
	}

	for name, tc := range tests {
		t.Run(name, func(t *testing.T) {
			s, err := tc.expr.Parse()
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
