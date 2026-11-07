// SPDX-License-Identifier: GPL-3.0-or-later

package siteid

import (
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestValidate(t *testing.T) {
	for name, tc := range map[string]struct {
		id    string
		valid bool
	}{
		"unassigned":        {"", true},
		"short":             {"a", true},
		"site":              {"storefront-prod_1", true},
		"maximum":           {strings.Repeat("a", 64), true},
		"too long":          {strings.Repeat("a", 65), false},
		"uppercase":         {"Storefront", false},
		"whitespace":        {" storefront", false},
		"leading separator": {"-storefront", false},
		"URL":               {"https://example.org", false},
		"path":              {"site/checkout", false},
		"unicode":           {"caf\u00e9", false},
		"newline":           {"site\n", false},
	} {
		t.Run(name, func(t *testing.T) {
			assert.Equal(t, tc.valid, Validate(tc.id) == nil)
		})
	}
}
