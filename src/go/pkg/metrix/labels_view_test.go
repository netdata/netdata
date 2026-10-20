// SPDX-License-Identifier: GPL-3.0-or-later

package metrix

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestLabelViewGetUsesCanonicalKeyOrder(t *testing.T) {
	view := labelView{items: []Label{
		{Key: "alpha", Value: "one"},
		{Key: "middle", Value: "two"},
		{Key: "zulu", Value: "three"},
	}}

	tests := map[string]struct {
		key       string
		wantValue string
		wantOK    bool
	}{
		"first":              {key: "alpha", wantValue: "one", wantOK: true},
		"middle":             {key: "middle", wantValue: "two", wantOK: true},
		"last":               {key: "zulu", wantValue: "three", wantOK: true},
		"missing before":     {key: "aardvark"},
		"missing between":    {key: "beta"},
		"missing after":      {key: "zzzz"},
		"missing from empty": {key: "alpha"},
	}

	for name, tc := range tests {
		t.Run(name, func(t *testing.T) {
			candidate := view
			if name == "missing from empty" {
				candidate = labelView{}
			}
			got, ok := candidate.Get(tc.key)
			assert.Equal(t, tc.wantOK, ok)
			assert.Equal(t, tc.wantValue, got)
		})
	}
}
