// SPDX-License-Identifier: GPL-3.0-or-later

package snmputils

import (
	"testing"

	"github.com/gosnmp/gosnmp"
	"github.com/stretchr/testify/assert"
)

func TestSNMPv3SecurityLevel(t *testing.T) {
	tests := map[string]struct {
		auth gosnmp.SnmpV3AuthProtocol
		priv gosnmp.SnmpV3PrivProtocol
		want gosnmp.SnmpV3MsgFlags
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
	usm := func(auth gosnmp.SnmpV3AuthProtocol, priv gosnmp.SnmpV3PrivProtocol) *gosnmp.UsmSecurityParameters {
		return &gosnmp.UsmSecurityParameters{
			UserName:                 "netdata-user",
			AuthenticationProtocol:   auth,
			AuthenticationPassphrase: authPass,
			PrivacyProtocol:          priv,
			PrivacyPassphrase:        privPass,
		}
	}

	tests := map[string]struct {
		version     gosnmp.SnmpVersion
		msgFlags    gosnmp.SnmpV3MsgFlags
		secParams   *gosnmp.UsmSecurityParameters
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
			version:   gosnmp.Version3,
			msgFlags:  gosnmp.NoAuthNoPriv,
			secParams: usm(gosnmp.NoAuth, gosnmp.NoPriv),
			want: "hostname='127.0.0.1',port='161',snmp_version='3',security_level='NoAuthNoPriv'," +
				"username='netdata-user',auth_protocol='NoAuth',priv_protocol='NoPriv'",
		},
		"v3 authNoPriv": {
			version:   gosnmp.Version3,
			msgFlags:  gosnmp.AuthNoPriv,
			secParams: usm(gosnmp.SHA256, gosnmp.NoPriv),
			want: "hostname='127.0.0.1',port='161',snmp_version='3',security_level='AuthNoPriv'," +
				"username='netdata-user',auth_protocol='SHA256',priv_protocol='NoPriv'",
		},
		"v3 authPriv with context name": {
			version:     gosnmp.Version3,
			msgFlags:    gosnmp.AuthPriv,
			secParams:   usm(gosnmp.SHA512, gosnmp.AES256C),
			contextName: "netdata-context",
			want: "hostname='127.0.0.1',port='161',snmp_version='3',security_level='AuthPriv'," +
				"username='netdata-user',auth_protocol='SHA512',priv_protocol='AES256C',context_name='netdata-context'",
		},
		"v3 authPriv with the Reportable flag set by Connect": {
			version:   gosnmp.Version3,
			msgFlags:  gosnmp.AuthPriv | gosnmp.Reportable,
			secParams: usm(gosnmp.SHA, gosnmp.AES),
			want: "hostname='127.0.0.1',port='161',snmp_version='3',security_level='AuthPriv'," +
				"username='netdata-user',auth_protocol='SHA',priv_protocol='AES'",
		},
	}

	for name, tc := range tests {
		t.Run(name, func(t *testing.T) {
			client := gosnmp.NewHandler()
			client.SetTarget("127.0.0.1")
			client.SetPort(161)
			client.SetVersion(tc.version)
			client.SetCommunity(community)
			client.SetMsgFlags(tc.msgFlags)
			if tc.secParams != nil {
				client.SetSecurityParameters(tc.secParams)
			}
			client.SetContextName(tc.contextName)

			info := SnmpClientConnInfo(client)

			assert.Equal(t, tc.want, info)
			for _, secret := range []string{community, authPass, privPass} {
				assert.NotContains(t, info, secret)
			}
		})
	}
}
