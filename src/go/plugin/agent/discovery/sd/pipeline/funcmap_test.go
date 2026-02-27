// SPDX-License-Identifier: GPL-3.0-or-later

package pipeline

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func Test_funcMatchAny(t *testing.T) {
	tests := map[string]struct {
		typ       string
		patterns  []string
		value     string
		wantMatch bool
	}{
		"dstar: one param, matches": {
			wantMatch: true,
			typ:       "dstar",
			patterns:  []string{"*"},
			value:     "value",
		},
		"dstar: one param, matches with *": {
			wantMatch: true,
			typ:       "dstar",
			patterns:  []string{"**/value"},
			value:     "/one/two/three/value",
		},
		"dstar: one param, not matches": {
			wantMatch: false,
			typ:       "dstar",
			patterns:  []string{"Value"},
			value:     "value",
		},
		"dstar: several params, last one matches": {
			wantMatch: true,
			typ:       "dstar",
			patterns:  []string{"not", "matches", "*"},
			value:     "value",
		},
		"dstar: several params, no matches": {
			wantMatch: false,
			typ:       "dstar",
			patterns:  []string{"not", "matches", "really"},
			value:     "value",
		},
		"re: one param, matches": {
			wantMatch: true,
			typ:       "re",
			patterns:  []string{"^value$"},
			value:     "value",
		},
		"re: one param, not matches": {
			wantMatch: false,
			typ:       "re",
			patterns:  []string{"^Value$"},
			value:     "value",
		},
		"re: several params, last one matches": {
			wantMatch: true,
			typ:       "re",
			patterns:  []string{"not", "matches", "va[lue]{3}"},
			value:     "value",
		},
		"re: several params, no matches": {
			wantMatch: false,
			typ:       "re",
			patterns:  []string{"not", "matches", "val[^l]ue"},
			value:     "value",
		},
	}

	for name, test := range tests {
		t.Run(name, func(t *testing.T) {
			ok := funcMatchAny(test.typ, test.value, test.patterns[0], test.patterns[1:]...)

			assert.Equal(t, test.wantMatch, ok)
		})
	}
}
