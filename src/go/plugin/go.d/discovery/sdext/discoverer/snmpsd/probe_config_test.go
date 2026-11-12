// SPDX-License-Identifier: GPL-3.0-or-later

package snmpsd

import (
	"testing"
	"time"

	"github.com/netdata/gosnmp"
	"github.com/stretchr/testify/assert"
)

func TestDiscovererProbeConfig(t *testing.T) {
	base := gosnmp.Config{Target: "192.0.2.1", Port: 161, Timeout: 3 * time.Second}
	for name, tc := range map[string]struct {
		cred CredentialConfig
		want func(gosnmp.Config) gosnmp.Config
	}{
		"SNMPv1": {
			cred: CredentialConfig{Version: "1", Community: "fixture-community"},
			want: func(c gosnmp.Config) gosnmp.Config {
				c.Version, c.Community = gosnmp.Version1, "fixture-community"
				return c
			},
		},
		"SNMPv2c": {
			cred: CredentialConfig{Version: "2c", Community: "fixture-community"},
			want: func(c gosnmp.Config) gosnmp.Config {
				c.Version, c.Community = gosnmp.Version2c, "fixture-community"
				return c
			},
		},
		"SNMPv3": {
			cred: CredentialConfig{
				Version:           "3",
				UserName:          "fixture-user",
				SecurityLevel:     "authPriv",
				AuthProtocol:      "sha",
				AuthPassphrase:    "fixture-auth",
				PrivacyProtocol:   "aes",
				PrivacyPassphrase: "fixture-priv",
				ContextName:       "fixture-context",
			},
			want: func(c gosnmp.Config) gosnmp.Config {
				c.Version, c.SecurityLevel, c.ContextName = gosnmp.Version3, gosnmp.AuthPriv, "fixture-context"
				c.User = gosnmp.User{
					Name:           "fixture-user",
					AuthProtocol:   gosnmp.SHA,
					AuthPassphrase: "fixture-auth",
					PrivProtocol:   gosnmp.AES,
					PrivPassphrase: "fixture-priv",
				}
				return c
			},
		},
	} {
		t.Run(name, func(t *testing.T) {
			d := &Discoverer{timeout: 3 * time.Second}
			assert.Equal(t, tc.want(base), d.probeConfig("192.0.2.1", tc.cred))
		})
	}
}
