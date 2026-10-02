// SPDX-License-Identifier: GPL-3.0-or-later
package commandexec

import (
	"github.com/stretchr/testify/assert"
	"testing"
)

func TestCommandHostZones(t *testing.T) {

	for name, test := range map[string]struct {
		host    string
		invalid bool
	}{
		"named zone":            {host: "fe80::1%eth0"},
		"numeric zone":          {host: "fe80::1%2"},
		"interface punctuation": {host: "fe80::1%eth-0.1_2"},
		"no zone":               {host: "2001:db8::1"},
		"IPv4":                  {host: "192.0.2.1"},
		"hostname":              {host: "irc.example.com"},
		"empty zone":            {host: "fe80::1%", invalid: true},
		"slash":                 {host: "fe80::1%eth/0", invalid: true},
		"backslash":             {host: "fe80::1%eth\\0", invalid: true},
		"colon":                 {host: "fe80::1%eth0:extra", invalid: true},
		"open bracket":          {host: "fe80::1%[eth0", invalid: true},
		"close bracket":         {host: "fe80::1%eth0]", invalid: true},
		"at sign":               {host: "fe80::1%eth@0", invalid: true},
		"query":                 {host: "fe80::1%eth?0", invalid: true},
		"fragment":              {host: "fe80::1%eth#0", invalid: true},
		"extra zone marker":     {host: "fe80::1%eth%0", invalid: true},
		"reference":             {host: "fe80::1%${env:ZONE}", invalid: true},
		"space":                 {host: "fe80::1%eth 0", invalid: true},
		"control":               {host: "fe80::1%eth\x00", invalid: true},
	} {
		t.Run(name, func(t *testing.T) {
			assert.Equal(t, !test.invalid, ValidHost(test.host))
		})
	}
}
