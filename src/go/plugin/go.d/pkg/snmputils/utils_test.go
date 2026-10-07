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
