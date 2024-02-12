// SPDX-License-Identifier: GPL-3.0-or-later

package pipeline

import (
	"fmt"
	"testing"

	"github.com/stretchr/testify/assert"
)

func Test_globAny(t *testing.T) {
	tests := map[string]struct {
		patterns  []string
		value     string
		wantMatch bool
	}{
		"one param, matches": {
			wantMatch: true,
			patterns:  []string{"*"},
			value:     "value",
		},
		"one param, matches with *": {
			wantMatch: true,
			patterns:  []string{"**/value"},
			value:     "/one/two/three/value",
		},
		"one param, not matches": {
			wantMatch: false,
			patterns:  []string{"Value"},
			value:     "value",
		},
		"several params, last one matches": {
			wantMatch: true,
			patterns:  []string{"not", "matches", "*"},
			value:     "value",
		},
		"several params, no matches": {
			wantMatch: false,
			patterns:  []string{"not", "matches", "really"},
			value:     "value",
		},
	}

	for name, test := range tests {
		name := fmt.Sprintf("name: %s, patterns: '%v', value: '%s'", name, test.patterns, test.value)
		ok := globAny(test.value, test.patterns[0], test.patterns[1:]...)

		if test.wantMatch {
			assert.Truef(t, ok, name)
		} else {
			assert.Falsef(t, ok, name)
		}
	}
}

func Test_regexpAny(t *testing.T) {
	tests := map[string]struct {
		patterns  []string
		value     string
		wantMatch bool
	}{
		"one param, matches": {
			wantMatch: true,
			patterns:  []string{"^value$"},
			value:     "value",
		},
		"one param, not matches": {
			wantMatch: false,
			patterns:  []string{"^Value$"},
			value:     "value",
		},
		"several params, last one matches": {
			wantMatch: true,
			patterns:  []string{"not", "matches", "va[lue]{3}"},
			value:     "value",
		},
		"several params, no matches": {
			wantMatch: false,
			patterns:  []string{"not", "matches", "val[^l]ue"},
			value:     "value",
		},
	}

	for name, test := range tests {
		name := fmt.Sprintf("name: %s, patterns: '%v', value: '%s'", name, test.patterns, test.value)
		ok := regexpAny(test.value, test.patterns[0], test.patterns[1:]...)

		if test.wantMatch {
			assert.Truef(t, ok, name)
		} else {
			assert.Falsef(t, ok, name)
		}
	}
}
