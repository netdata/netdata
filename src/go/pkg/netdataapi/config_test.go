// SPDX-License-Identifier: GPL-3.0-or-later

package netdataapi

import (
	"testing"

	"github.com/stretchr/testify/require"
)

func TestValidConfigProtocolFields(t *testing.T) {
	tests := map[string]struct {
		value  string
		bare   bool
		quoted bool
	}{
		"empty":                     {value: "", quoted: true},
		"safe":                      {value: "source", bare: true, quoted: true},
		"equals":                    {value: "file=/etc/netdata", quoted: true},
		"space":                     {value: "file source", quoted: true},
		"double quote":              {value: `file"source`, quoted: true},
		"single quote":              {value: "file'source"},
		"Windows path":              {value: `C:\Program Files\Netdata`, quoted: true},
		"even trailing backslashes": {value: `file=C:\\`, quoted: true},
		"odd trailing backslash":    {value: `file=C:\`},
		"odd trailing backslashes":  {value: `file=C:\\\`},
		"line break":                {value: "file\nsource"},
		"delete":                    {value: "file\x7fsource"},
	}
	for name, test := range tests {
		t.Run(name, func(t *testing.T) {
			require.Equal(t, test.bare, ValidBareProtocolField(test.value))
			require.Equal(t, test.quoted, ValidSingleQuotedProtocolField(test.value))
		})
	}
}
