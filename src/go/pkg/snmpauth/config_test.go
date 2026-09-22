// SPDX-License-Identifier: GPL-3.0-or-later

package snmpauth

import (
	"testing"

	"github.com/gosnmp/gosnmp"
	"github.com/stretchr/testify/require"
)

func TestApply(t *testing.T) {
	for _, version := range []string{"", "1", "2c"} {
		c := Config{Version: version, Credentials: &Community{Community: "test-secret"}}
		client := &gosnmp.GoSNMP{}
		require.NoError(t, c.Apply(client))
		require.Equal(t, "test-secret", client.Community)
		if version == "1" {
			require.Equal(t, gosnmp.Version1, client.Version)
		} else {
			require.Equal(t, gosnmp.Version2c, client.Version)
		}
	}
	for _, level := range []string{"noAuthNoPriv", "authNoPriv", "authPriv", ""} {
		t.Run(level, func(t *testing.T) {
			u := &USM{Username: "test-user", SecurityLevel: level}
			if level != "noAuthNoPriv" {
				u.AuthPassword = "test-auth"
			}
			if level == "authPriv" || level == "" {
				u.PrivPassword = "test-priv"
			}
			c := Config{Version: "3", Credentials3: u}
			client := &gosnmp.GoSNMP{}
			require.NoError(t, c.Apply(client))
			require.Equal(t, gosnmp.Version3, client.Version)
			if level == "" {
				require.Equal(t, gosnmp.AuthPriv, client.MsgFlags)
				require.Equal(t, gosnmp.SHA512, client.SecurityParameters.(*gosnmp.UsmSecurityParameters).AuthenticationProtocol)
			}
		})
	}
}
func TestInvalidAuthIsNotSilentlyDowngraded(t *testing.T) {
	cases := []Config{
		{Version: "invalid-secret"}, {},
		{Credentials3: &USM{Username: "inactive"}},
		{Version: "3", Credentials3: &USM{Username: "user", SecurityLevel: "invalid-secret"}},
		{Version: "3", Credentials3: &USM{Username: "user", SecurityLevel: "authNoPriv", AuthProtocol: "invalid-secret", AuthPassword: "test-secret"}},
		{Version: "3", Credentials3: &USM{Username: "user", SecurityLevel: "authPriv", AuthPassword: "short", PrivPassword: "test-secret"}},
		{Version: "3", Credentials3: &USM{Username: "user", SecurityLevel: "authPriv", AuthPassword: "test-secret", PrivPassword: "short"}},
	}
	for _, c := range cases {
		err := c.Validate()
		require.Error(t, err)
		require.NotContains(t, err.Error(), "secret")
	}
}

func TestNormalizeInactiveCredentials(t *testing.T) {
	for _, version := range []string{"", "1", "2c", "3", "invalid"} {
		for _, level := range []string{"", "authPriv", "authNoPriv", "noAuthNoPriv", "invalid"} {
			t.Run(version+"/"+level, func(t *testing.T) {
				input := Config{Version: version, Credentials: &Community{Community: "fixture"}, Credentials3: &USM{Username: "fixture", SecurityLevel: level, AuthPassword: "test-auth", PrivPassword: "test-priv", ContextName: "context"}}
				original := input.Copy()
				got := input.Normalized()
				require.Equal(t, original, input, "normalizing must not mutate caller-owned credentials")
				require.Equal(t, got, got.Normalized())
				if version == "invalid" || version == "3" && level == "invalid" {
					require.Error(t, got.Validate())
					return
				}
				client := &gosnmp.GoSNMP{}
				require.NoError(t, input.Apply(client))
				if version != "3" {
					require.Nil(t, got.Credentials3)
					require.Equal(t, "fixture", client.Community)
					return
				}
				require.Nil(t, got.Credentials)
				require.Equal(t, "context", got.ContextName())
				usm := client.SecurityParameters.(*gosnmp.UsmSecurityParameters)
				if level == "noAuthNoPriv" {
					require.Empty(t, got.Credentials3.AuthPassword)
					require.Empty(t, usm.AuthenticationPassphrase)
				}
				if level == "authNoPriv" || level == "noAuthNoPriv" {
					require.Empty(t, got.Credentials3.PrivPassword)
					require.Empty(t, usm.PrivacyPassphrase)
				}
			})
		}
	}
}
func TestCopyOwnsCredentials(t *testing.T) {
	c := Config{Credentials: &Community{Community: "original"}, Credentials3: &USM{AuthPassword: "original"}}
	copy := c.Copy()
	copy.Credentials.Community = "new"
	copy.Credentials3.AuthPassword = "new"
	require.Equal(t, "original", c.Credentials.Community)
	require.Equal(t, "original", c.Credentials3.AuthPassword)
}
