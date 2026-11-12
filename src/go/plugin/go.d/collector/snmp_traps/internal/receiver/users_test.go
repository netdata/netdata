// SPDX-License-Identifier: GPL-3.0-or-later

package receiver

import (
	"encoding/hex"
	"net"
	"testing"

	"github.com/netdata/gosnmp"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestReceiverUsers(t *testing.T) {
	engine := func(h string) []byte {
		b, err := hex.DecodeString(h)
		require.NoError(t, err)
		return b
	}
	e1, e2, local := engine(testEngineIDHex), engine(securityLevelOtherEngineIDHex), engine(testLocalEngineIDHex)
	auth := gosnmp.User{
		Name:           "testuser",
		AuthProtocol:   gosnmp.SHA256,
		AuthPassphrase: "authpassword",
		PrivProtocol:   gosnmp.AES,
		PrivPassphrase: "privpassword",
	}
	plain := gosnmp.User{Name: "testuser", AuthProtocol: gosnmp.NoAuth, PrivProtocol: gosnmp.NoPriv}
	other := gosnmp.User{Name: "otheruser", AuthProtocol: gosnmp.NoAuth, PrivProtocol: gosnmp.NoPriv}
	at := func(engineID []byte, users ...gosnmp.User) []gosnmp.EngineUser {
		var list []gosnmp.EngineUser
		for _, u := range users {
			list = append(list, gosnmp.EngineUser{EngineID: engineID, User: u})
		}
		return list
	}
	concat := func(lists ...[]gosnmp.EngineUser) []gosnmp.EngineUser {
		var all []gosnmp.EngineUser
		for _, l := range lists {
			all = append(all, l...)
		}
		return all
	}

	tests := map[string]struct {
		users     []USMUser
		whitelist [][]byte
		dynamic   bool
		want      []gosnmp.EngineUser
		wantLearn []gosnmp.User
		wantErr   bool
	}{
		"static: an authenticated user serves every whitelisted engine and the local one": {
			users:     []USMUser{securityLevelAuthUser(testEngineIDHex)},
			whitelist: [][]byte{e1, e2},
			want:      concat(at(e1, auth), at(e2, auth), at(local, auth)),
		},
		"static: a no-auth user serves an engine without users of its name bound there": {
			users:     []USMUser{securityLevelNoAuthUser(securityLevelOtherEngineIDHex)},
			whitelist: [][]byte{e1, e2},
			want:      concat(at(e1, plain), at(e2, plain), at(local, plain)),
		},
		"static: bound users first, then the name's authenticated users": {
			users:     []USMUser{securityLevelNoAuthUser(securityLevelOtherEngineIDHex), securityLevelAuthUser(testEngineIDHex)},
			whitelist: [][]byte{e1, e2},
			want:      concat(at(e1, auth), at(e2, plain, auth), at(local, plain, auth)),
		},
		"static: users bound to the local engine decide there": {
			users:     []USMUser{securityLevelNoAuthUser(testLocalEngineIDHex), securityLevelAuthUser(testEngineIDHex)},
			whitelist: [][]byte{e1},
			want:      concat(at(e1, auth), at(local, plain, auth)),
		},
		"static: the local engine is not registered twice when whitelisted": {
			users:     []USMUser{securityLevelAuthUser(testEngineIDHex)},
			whitelist: [][]byte{e1, local},
			want:      concat(at(e1, auth), at(local, auth)),
		},
		"static: a user configured identically for two engines appears once": {
			users:     []USMUser{securityLevelAuthUser(testEngineIDHex), securityLevelAuthUser(securityLevelOtherEngineIDHex)},
			whitelist: [][]byte{e1, e2},
			want:      concat(at(e1, auth), at(e2, auth), at(local, auth)),
		},
		"static: each user name has its own lists": {
			users:     []USMUser{securityLevelAuthUser(testEngineIDHex), {Username: "otheruser", AuthProto: "none", PrivProto: "none"}},
			whitelist: [][]byte{e1},
			want:      concat(at(e1, auth, other), at(local, auth, other)),
		},
		"dynamic: configured engines and the local one, authenticated users learned": {
			users:     []USMUser{securityLevelAuthUser(""), securityLevelAuthUser(testEngineIDHex)},
			dynamic:   true,
			want:      concat(at(e1, auth), at(local, auth)),
			wantLearn: []gosnmp.User{auth},
		},
		"dynamic: a no-auth user without engine ID is learned": {
			users:     []USMUser{securityLevelNoAuthUser(""), securityLevelAuthUser(testEngineIDHex)},
			dynamic:   true,
			want:      concat(at(e1, auth), at(local, plain, auth)),
			wantLearn: []gosnmp.User{plain, auth},
		},
		"dynamic: a no-auth user bound to an engine is learned when every user of the name is no-auth": {
			users:     []USMUser{securityLevelNoAuthUser(testEngineIDHex)},
			dynamic:   true,
			want:      concat(at(e1, plain), at(local, plain)),
			wantLearn: []gosnmp.User{plain},
		},
		"dynamic: a no-auth user bound to an engine is not learned beside authenticated users": {
			users:     []USMUser{securityLevelNoAuthUser(testEngineIDHex), securityLevelAuthUser(securityLevelOtherEngineIDHex)},
			dynamic:   true,
			want:      concat(at(e1, plain, auth), at(e2, auth), at(local, plain, auth)),
			wantLearn: []gosnmp.User{auth},
		},
		"dynamic: the whitelist is not used": {
			users:     []USMUser{securityLevelAuthUser("")},
			whitelist: [][]byte{e1},
			dynamic:   true,
			want:      at(local, auth),
			wantLearn: []gosnmp.User{auth},
		},
		"an invalid engine ID": {
			users:   []USMUser{securityLevelAuthUser("not-hex")},
			wantErr: true,
		},
	}
	for name, tc := range tests {
		t.Run(name, func(t *testing.T) {
			got, learn, err := receiverUsers(tc.users, tc.whitelist, local, tc.dynamic)
			if tc.wantErr {
				require.Error(t, err)
				return
			}
			require.NoError(t, err)
			assert.Equal(t, tc.want, got)
			assert.Equal(t, tc.wantLearn, learn)
		})
	}
}

func newStaticV3TestReceiver(t *testing.T, whitelist []string) (*Receiver, *receiverEventRecorder) {
	t.Helper()
	recorder := &receiverEventRecorder{}
	recv := New(NewPolicy(PolicyConfig{
		Versions:          []string{"v3"},
		USMUsers:          []USMUser{{Username: "testuser", EngineID: testEngineIDHex, AuthProto: "none", PrivProto: "none"}},
		EngineIDWhitelist: whitelist,
		LocalEngineID:     testLocalEngineIDHex,
	}), recorder.report)
	if err := recv.PrepareV3(t.TempDir(), "static-v3-test"); err != nil {
		t.Fatalf("prepare v3 receiver: %v", err)
	}
	t.Cleanup(recv.RollbackPreparedState)
	return recv, recorder
}

func TestV3InformAcceptedWithLocalEngineID(t *testing.T) {
	data := buildV3InformWithEngineID(t, "testuser", testLocalEngineIDHex, "1.3.6.1.6.3.1.1.5.1")
	recv, events := newStaticV3TestReceiver(t, []string{testEngineIDHex})
	peer := &net.UDPAddr{IP: net.ParseIP("10.1.2.3"), Port: 9162}
	if result := recv.Process(Datagram{Data: data, PeerIP: peer.IP, Peer: peer}); result.PDU == nil {
		t.Fatalf("result = %+v, want accepted INFORM", result)
	}
	if got := events.count(EventError, "unknown_engine_id"); got != 0 {
		t.Fatalf("unknown_engine_id events = %d, want 0", got)
	}
}

// An inform naming another engine than the local one would be answered by that
// engine (RFC 3412 section 7.2 step 13 a): it is a decode failure.
func TestV3InformRejectedWithNonLocalEngineID(t *testing.T) {
	data := buildV3InformWithEngineID(t, "testuser", testEngineIDHex, "1.3.6.1.6.3.1.1.5.1")
	recv, events := newStaticV3TestReceiver(t, []string{testEngineIDHex})
	peer := &net.UDPAddr{IP: net.ParseIP("10.1.2.3"), Port: 9162}
	result := recv.Process(Datagram{Data: data, PeerIP: peer.IP, Peer: peer})
	require.Nil(t, result.PDU)
	require.NotNil(t, result.DecodeFailure)
	assert.Equal(t, ErrorUnknownEngine, result.DecodeFailure.Kind)
	assert.Equal(t, testEngineIDHex, result.DecodeFailure.EngineID)
	assert.Equal(t, 1, events.count(EventError, "unknown_engine_id"))
}

// A static job has users only for its whitelisted engines and its own.
func TestV3TrapStillRequiresSenderEngineWhitelist(t *testing.T) {
	data := buildV3TrapWithEngineID(t, "testuser", testEngineIDHex, "1.3.6.1.6.3.1.1.5.1")
	recv, events := newStaticV3TestReceiver(t, []string{"80001f888077dfe44faa700259"})
	peer := &net.UDPAddr{IP: net.ParseIP("10.1.2.3"), Port: 9162}
	result := recv.Process(Datagram{Data: data, PeerIP: peer.IP, Peer: peer})
	require.Nil(t, result.PDU)
	require.NotNil(t, result.DecodeFailure)
	assert.Equal(t, ErrorUnknownEngine, result.DecodeFailure.Kind)
	assert.Equal(t, testEngineIDHex, result.DecodeFailure.EngineID)
	assert.Equal(t, 1, events.count(EventError, "unknown_engine_id"))
}

// The users registered at the local engine for informs decode a trap naming
// it too: a static job accepts it only when that engine is whitelisted, and a
// dynamic job does not learn it.
func TestV3TrapNamingLocalEngineID(t *testing.T) {
	data := buildV3TrapWithEngineID(t, "testuser", testLocalEngineIDHex, "1.3.6.1.6.3.1.1.5.1")
	peer := &net.UDPAddr{IP: net.ParseIP("10.1.2.3"), Port: 9162}

	t.Run("static, local engine not whitelisted", func(t *testing.T) {
		recv, events := newStaticV3TestReceiver(t, []string{testEngineIDHex})
		result := recv.Process(Datagram{Data: data, PeerIP: peer.IP, Peer: peer})
		assert.Equal(t, Result{}, result)
		assert.Equal(t, 1, events.count(EventError, "unknown_engine_id"))
	})
	t.Run("static, local engine whitelisted", func(t *testing.T) {
		recv, events := newStaticV3TestReceiver(t, []string{testEngineIDHex, testLocalEngineIDHex})
		result := recv.Process(Datagram{Data: data, PeerIP: peer.IP, Peer: peer})
		assert.NotNil(t, result.PDU)
		assert.Zero(t, events.count(EventError, ""))
	})
	t.Run("dynamic", func(t *testing.T) {
		recorder := &receiverEventRecorder{}
		recv := New(NewPolicy(PolicyConfig{
			Versions:        []string{"v3"},
			USMUsers:        []USMUser{securityLevelNoAuthUser("")},
			DynamicEngineID: true,
			LocalEngineID:   testLocalEngineIDHex,
		}), recorder.report)
		require.NoError(t, recv.PrepareV3(t.TempDir(), "dynamic-local-engine-test"))
		t.Cleanup(recv.RollbackPreparedState)
		result := recv.Process(Datagram{Data: data, PeerIP: peer.IP, Peer: peer})
		assert.NotNil(t, result.PDU)
		assert.Zero(t, recorder.count(EventDynamicEngineIDRegistered, ""))
	})
}
