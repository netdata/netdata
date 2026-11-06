// SPDX-License-Identifier: GPL-3.0-or-later

package ddprofiledefinition

import (
	"slices"
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestCommonOIDPrefix(t *testing.T) {
	for name, tc := range map[string]struct {
		oids []string
		want string
	}{
		"none":               {},
		"empty":              {oids: []string{""}},
		"single":             {oids: []string{".1.2.3."}, want: "1.2.3"},
		"identical":          {oids: []string{"1.2.3", "1.2.3"}, want: "1.2.3"},
		"different roots":    {oids: []string{"1.2", "2.2"}},
		"component boundary": {oids: []string{"1.2.30", "1.2.3"}, want: "1.2"},
		"prefix first":       {oids: []string{"1.2", "1.2.3"}, want: "1.2"},
		"prefix last":        {oids: []string{"1.2.3", "1.2"}, want: "1.2"},
		"all inputs":         {oids: []string{"1.2.3", "1.2.4", "1.5.6"}, want: "1"},
		"empty input":        {oids: []string{"1.2", ""}},
		"dotted inputs":      {oids: []string{".1.2.3.", "..1.2.4.."}, want: "1.2"},
		"if_x_table":         {oids: []string{"1.3.6.1.2.1.31.1.1.1.1", "1.3.6.1.2.1.31.1.1.1.18"}, want: "1.3.6.1.2.1.31.1.1.1"},
		"juniper_table":      {oids: []string{"1.3.6.1.4.1.2636.5.1.1.2.1.1.1.11", "1.3.6.1.4.1.2636.5.1.1.2.1.1.1.14"}, want: "1.3.6.1.4.1.2636.5.1.1.2.1.1.1"},
	} {
		t.Run(name, func(t *testing.T) {
			before := slices.Clone(tc.oids)
			assert.Equal(t, tc.want, CommonOIDPrefix(tc.oids))
			assert.Equal(t, before, tc.oids)
		})
	}
}
