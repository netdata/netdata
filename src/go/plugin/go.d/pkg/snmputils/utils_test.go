// SPDX-License-Identifier: GPL-3.0-or-later

package snmputils

import (
	"testing"

	"github.com/netdata/gosnmp"
	"github.com/stretchr/testify/assert"
)

func TestSNMPv3SecurityLevel(t *testing.T) {
	tests := map[string]struct {
		auth gosnmp.AuthProtocol
		priv gosnmp.PrivProtocol
		want gosnmp.SecurityLevel
	}{
		"no auth, no priv": {
			auth: gosnmp.NoAuth,
			priv: gosnmp.NoPriv,
			want: gosnmp.NoAuthNoPriv,
		},
		"unset protocols": {
			want: gosnmp.NoAuthNoPriv,
		},
		"priv without auth": {
			auth: gosnmp.NoAuth,
			priv: gosnmp.AES,
			want: gosnmp.NoAuthNoPriv,
		},
		"auth, no priv": {
			auth: gosnmp.SHA256,
			priv: gosnmp.NoPriv,
			want: gosnmp.AuthNoPriv,
		},
		"auth, unset priv": {
			auth: gosnmp.MD5,
			want: gosnmp.AuthNoPriv,
		},
		"auth and priv": {
			auth: gosnmp.SHA,
			priv: gosnmp.AES256C,
			want: gosnmp.AuthPriv,
		},
	}

	for name, tc := range tests {
		t.Run(name, func(t *testing.T) {
			assert.Equal(t, tc.want, SNMPv3SecurityLevel(tc.auth, tc.priv))
		})
	}
}

func TestSnmpClientConnInfo(t *testing.T) {
	const (
		community = "test-community-secret"
		authPass  = "test-auth-passphrase-secret"
		privPass  = "test-priv-passphrase-secret"
	)
	usm := func(auth gosnmp.AuthProtocol, priv gosnmp.PrivProtocol) gosnmp.User {
		return gosnmp.User{
			Name:           "netdata-user",
			AuthProtocol:   auth,
			AuthPassphrase: authPass,
			PrivProtocol:   priv,
			PrivPassphrase: privPass,
		}
	}

	tests := map[string]struct {
		version     gosnmp.Version
		msgFlags    gosnmp.SecurityLevel
		user        gosnmp.User
		contextName string
		want        string
	}{
		"v1": {
			version: gosnmp.Version1,
			want:    "hostname='127.0.0.1',port='161',snmp_version='1'",
		},
		"v2c": {
			version: gosnmp.Version2c,
			want:    "hostname='127.0.0.1',port='161',snmp_version='2c'",
		},
		"v3 noAuthNoPriv": {
			version:  gosnmp.Version3,
			msgFlags: gosnmp.NoAuthNoPriv,
			user:     usm(gosnmp.NoAuth, gosnmp.NoPriv),
			want: "hostname='127.0.0.1',port='161',snmp_version='3',security_level='NoAuthNoPriv'," +
				"username='netdata-user',auth_protocol='NoAuth',priv_protocol='NoPriv'",
		},
		"v3 authNoPriv": {
			version:  gosnmp.Version3,
			msgFlags: gosnmp.AuthNoPriv,
			user:     usm(gosnmp.SHA256, gosnmp.NoPriv),
			want: "hostname='127.0.0.1',port='161',snmp_version='3',security_level='AuthNoPriv'," +
				"username='netdata-user',auth_protocol='SHA256',priv_protocol='NoPriv'",
		},
		"v3 authPriv with context name": {
			version:     gosnmp.Version3,
			msgFlags:    gosnmp.AuthPriv,
			user:        usm(gosnmp.SHA512, gosnmp.AES256C),
			contextName: "netdata-context",
			want: "hostname='127.0.0.1',port='161',snmp_version='3',security_level='AuthPriv'," +
				"username='netdata-user',auth_protocol='SHA512',priv_protocol='AES256C',context_name='netdata-context'",
		},
	}

	for name, tc := range tests {
		t.Run(name, func(t *testing.T) {
			info := SnmpClientConnInfo(gosnmp.Config{
				Target:        "127.0.0.1",
				Port:          161,
				Version:       tc.version,
				Community:     community,
				SecurityLevel: tc.msgFlags,
				User:          tc.user,
				ContextName:   tc.contextName,
			})

			assert.Equal(t, tc.want, info)
			for _, secret := range []string{community, authPass, privPass} {
				assert.NotContains(t, info, secret)
			}
		})
	}
}
