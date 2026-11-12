// SPDX-License-Identifier: GPL-3.0-or-later

package traptest

import (
	"testing"

	"github.com/netdata/gosnmp"
)

func TestV3PacketFlagsFollowParsedProtocols(t *testing.T) {
	tests := map[string]struct {
		authProto string
		privProto string
		authKey   string
		privKey   string
		want      gosnmp.SecurityLevel
	}{
		"empty aliases": {
			want: gosnmp.NoAuthNoPriv,
		},
		"named no-auth aliases": {
			authProto: "noAuth",
			privProto: "noPriv",
			want:      gosnmp.NoAuthNoPriv,
		},
		"numeric no-auth aliases": {
			authProto: "1",
			privProto: "1",
			want:      gosnmp.NoAuthNoPriv,
		},
		"numeric auth-no-priv aliases": {
			authProto: "2",
			privProto: "1",
			authKey:   "authpass",
			want:      gosnmp.AuthNoPriv,
		},
		"numeric auth-priv aliases": {
			authProto: "3",
			privProto: "3",
			authKey:   "authpass",
			privKey:   "privpass",
			want:      gosnmp.AuthPriv,
		},
		"uppercase auth-priv aliases": {
			authProto: "SHA256",
			privProto: "AES",
			authKey:   "authpass",
			privKey:   "privpass",
			want:      gosnmp.AuthPriv,
		},
	}

	for name, tc := range tests {
		t.Run(name, func(t *testing.T) {
			msg, _ := newV3Message(t, gosnmp.SNMPv2Trap, V3Spec{
				User:        "testuser",
				EngineIDHex: "80001f888077dfe44faa700258",
				AuthProto:   tc.authProto,
				PrivProto:   tc.privProto,
				AuthKey:     tc.authKey,
				PrivKey:     tc.privKey,
				TrapOID:     "1.3.6.1.6.3.1.1.5.1",
			})
			if msg.SecurityLevel != tc.want {
				t.Fatalf("SecurityLevel = %v, want %v", msg.SecurityLevel, tc.want)
			}
		})
	}
}
