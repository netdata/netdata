// SPDX-License-Identifier: GPL-3.0-or-later

package secretresolver

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestSplitSchemeModifier(t *testing.T) {
	tests := map[string]struct {
		token        string
		wantScheme   string
		wantModifier string
	}{
		"no modifier":                     {token: "store", wantScheme: "store", wantModifier: ""},
		"env no modifier":                 {token: "env", wantScheme: "env", wantModifier: ""},
		"store with urienc":               {token: "store+urienc", wantScheme: "store", wantModifier: "urienc"},
		"env with urienc":                 {token: "env+urienc", wantScheme: "env", wantModifier: "urienc"},
		"trailing plus no modifier":       {token: "store+", wantScheme: "store", wantModifier: ""},
		"chained modifiers kept verbatim": {token: "store+urienc+trim", wantScheme: "store", wantModifier: "urienc+trim"},
	}

	for name, tc := range tests {
		t.Run(name, func(t *testing.T) {
			scheme, modifier := SplitSchemeModifier(tc.token)
			assert.Equal(t, tc.wantScheme, scheme)
			assert.Equal(t, tc.wantModifier, modifier)
		})
	}
}

func TestIsKnownModifier(t *testing.T) {
	tests := map[string]struct {
		modifier string
		want     bool
	}{
		"empty is known":       {modifier: "", want: true},
		"urienc is known":      {modifier: "urienc", want: true},
		"unknown name":         {modifier: "nope", want: false},
		"uri is not urienc":    {modifier: "uri", want: false},
		"chained is not known": {modifier: "urienc+trim", want: false},
	}

	for name, tc := range tests {
		t.Run(name, func(t *testing.T) {
			assert.Equal(t, tc.want, isKnownModifier(tc.modifier))
		})
	}
}

func TestPercentEncodeURIUnreserved(t *testing.T) {
	tests := map[string]struct {
		in   string
		want string
	}{
		"empty":                {in: "", want: ""},
		"unreserved unchanged": {in: "AZaz09-._~", want: "AZaz09-._~"},
		"slash encoded":        {in: "a/b", want: "a%2Fb"},
		"reserved set encoded": {in: "/:@+?# ", want: "%2F%3A%40%2B%3F%23%20"},
		"dsn password":         {in: "pa/ss+word: a@~", want: "pa%2Fss%2Bword%3A%20a%40~"},
		"high byte encoded":    {in: "\xff", want: "%FF"},
	}

	for name, tc := range tests {
		t.Run(name, func(t *testing.T) {
			assert.Equal(t, tc.want, percentEncodeURIUnreserved(tc.in))
		})
	}
}
