// SPDX-License-Identifier: GPL-3.0-or-later

package receiver

import (
	"net"
	"testing"

	"github.com/netdata/gosnmp"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

const securityLevelOtherEngineIDHex = "80001f888011223344556677"

func securityLevelAuthUser(engineIDHex string) USMUser {
	return USMUser{
		Username:  "testuser",
		EngineID:  engineIDHex,
		AuthProto: "sha256",
		AuthKey:   "authpassword",
		PrivProto: "aes",
		PrivKey:   "privpassword",
	}
}

func securityLevelNoAuthUser(engineIDHex string) USMUser {
	return USMUser{
		Username:  "testuser",
		EngineID:  engineIDHex,
		AuthProto: "none",
		PrivProto: "none",
	}
}

func securityLevelSpec(engineIDHex string) v3SecuredTrapSpec {
	return v3SecuredTrapSpec{
		user:        "testuser",
		engineIDHex: engineIDHex,
		authProto:   "sha256",
		privProto:   "aes",
		authKey:     "authpassword",
		privKey:     "privpassword",
		trapOID:     "1.3.6.1.6.3.1.1.5.1",
	}
}

func newSecurityLevelTestReceiver(t *testing.T, users []USMUser) (*Receiver, *receiverEventRecorder) {
	t.Helper()
	recorder := &receiverEventRecorder{}
	recv := New(NewPolicy(PolicyConfig{
		Versions:          []string{"v3"},
		USMUsers:          users,
		EngineIDWhitelist: []string{testEngineIDHex, securityLevelOtherEngineIDHex},
		LocalEngineID:     testLocalEngineIDHex,
	}), recorder.report)
	require.NoError(t, recv.PrepareV3(t.TempDir(), "security-level-test"))
	t.Cleanup(recv.RollbackPreparedState)
	return recv, recorder
}

// A user whose level the message does not meet rejects it: the receiver's
// level rule fails the decode (ErrUnknownSecurityLevel, usm_failures).
func TestV3SecurityLevelStaticUsers(t *testing.T) {
	type outcome struct {
		accepted      bool
		usmFailures   int
		decodeFailure bool
	}
	tests := map[string]struct {
		users []USMUser
		data  func(t *testing.T) []byte
		want  outcome
	}{
		"unauthenticated trap naming an auth user is dropped": {
			users: []USMUser{securityLevelAuthUser(testEngineIDHex)},
			data: func(t *testing.T) []byte {
				return buildV3TrapWithEngineID(t, "testuser", testEngineIDHex, "1.3.6.1.6.3.1.1.5.1")
			},
			want: outcome{
				accepted:      false,
				usmFailures:   1,
				decodeFailure: true,
			},
		},
		"authPriv trap from an auth user is accepted": {
			users: []USMUser{securityLevelAuthUser(testEngineIDHex)},
			data: func(t *testing.T) []byte {
				return buildV3SecuredTrapWithFlags(t, securityLevelSpec(testEngineIDHex), gosnmp.AuthPriv)
			},
			want: outcome{
				accepted:    true,
				usmFailures: 0,
			},
		},
		"authNoPriv trap from an auth and priv user is accepted": {
			users: []USMUser{securityLevelAuthUser(testEngineIDHex)},
			data: func(t *testing.T) []byte {
				return buildV3SecuredTrapWithFlags(t, securityLevelSpec(testEngineIDHex), gosnmp.AuthNoPriv)
			},
			want: outcome{
				accepted:    true,
				usmFailures: 0,
			},
		},
		"unauthenticated trap from a no-auth user is accepted": {
			users: []USMUser{securityLevelNoAuthUser(testEngineIDHex)},
			data: func(t *testing.T) []byte {
				return buildV3TrapWithEngineID(t, "testuser", testEngineIDHex, "1.3.6.1.6.3.1.1.5.1")
			},
			want: outcome{
				accepted:    true,
				usmFailures: 0,
			},
		},
		"unauthenticated trap from another whitelisted engine for a no-auth user is accepted": {
			users: []USMUser{securityLevelNoAuthUser(securityLevelOtherEngineIDHex)},
			data: func(t *testing.T) []byte {
				return buildV3TrapWithEngineID(t, "testuser", testEngineIDHex, "1.3.6.1.6.3.1.1.5.1")
			},
			want: outcome{
				accepted:    true,
				usmFailures: 0,
			},
		},
		"authPriv trap from another whitelisted engine for an auth user is accepted": {
			users: []USMUser{securityLevelAuthUser(securityLevelOtherEngineIDHex)},
			data: func(t *testing.T) []byte {
				return buildV3SecuredTrapWithFlags(t, securityLevelSpec(testEngineIDHex), gosnmp.AuthPriv)
			},
			want: outcome{
				accepted:    true,
				usmFailures: 0,
			},
		},
		"unauthenticated trap from another whitelisted engine for an auth user is dropped": {
			users: []USMUser{securityLevelAuthUser(securityLevelOtherEngineIDHex)},
			data: func(t *testing.T) []byte {
				return buildV3TrapWithEngineID(t, "testuser", testEngineIDHex, "1.3.6.1.6.3.1.1.5.1")
			},
			want: outcome{
				accepted:      false,
				usmFailures:   1,
				decodeFailure: true,
			},
		},
		"unauthenticated INFORM to the local engine through a mixed-level user is accepted": {
			users: []USMUser{
				securityLevelNoAuthUser(securityLevelOtherEngineIDHex),
				securityLevelAuthUser(testEngineIDHex),
			},
			data: func(t *testing.T) []byte {
				return buildV3InformWithEngineID(t, "testuser", testLocalEngineIDHex, "1.3.6.1.6.3.1.1.5.1")
			},
			want: outcome{
				accepted:    true,
				usmFailures: 0,
			},
		},
		"unauthenticated trap matching a no-auth entry of another engine is dropped": {
			users: []USMUser{
				securityLevelNoAuthUser(securityLevelOtherEngineIDHex),
				securityLevelAuthUser(testEngineIDHex),
			},
			data: func(t *testing.T) []byte {
				return buildV3TrapWithEngineID(t, "testuser", testEngineIDHex, "1.3.6.1.6.3.1.1.5.1")
			},
			want: outcome{
				accepted:      false,
				usmFailures:   1,
				decodeFailure: true,
			},
		},
	}

	for name, tc := range tests {
		t.Run(name, func(t *testing.T) {
			recv, events := newSecurityLevelTestReceiver(t, tc.users)
			peer := &net.UDPAddr{
				IP:   net.ParseIP("10.1.2.3"),
				Port: 9162,
			}

			result := recv.Process(Datagram{
				Data:   tc.data(t),
				PeerIP: peer.IP,
				Peer:   peer,
			})

			got := outcome{
				accepted:      result.PDU != nil,
				usmFailures:   events.count(EventError, ErrorUSMFailure),
				decodeFailure: result.DecodeFailure != nil,
			}
			assert.Equal(t, tc.want, got)
		})
	}
}

func TestV3SecurityLevelUnauthenticatedInformIsNotAcknowledged(t *testing.T) {
	recv, events := newSecurityLevelTestReceiver(t, []USMUser{securityLevelAuthUser(testEngineIDHex)})
	listenerConn, peerConn := udpConnPair(t)
	peer := peerConn.LocalAddr().(*net.UDPAddr)

	data := buildV3InformWithEngineID(t, "testuser", testLocalEngineIDHex, "1.3.6.1.6.3.1.1.5.1")
	result := recv.Process(Datagram{
		Data:   data,
		PeerIP: peer.IP,
		Peer:   peer,
		Conn:   listenerConn,
	})

	assert.Nil(t, result.PDU)
	assert.NotNil(t, result.DecodeFailure)
	assert.Equal(t, 1, events.count(EventError, ErrorUSMFailure))
	requireNoResponse(t, peerConn)
}

func TestV3SecurityLevelUnauthenticatedTrapDoesNotRegisterDynamicEngineID(t *testing.T) {
	recv, events, peer := newDynamicTestReceiver(t, 10, RateLimitConfig{})

	data := buildV3TrapWithEngineID(t, "testuser", securityLevelOtherEngineIDHex, "1.3.6.1.6.3.1.1.5.1")
	result := recv.Process(Datagram{
		Data:   data,
		PeerIP: peer.IP,
		Peer:   peer,
	})

	assert.Nil(t, result.PDU)
	assert.Equal(t, 1, events.count(EventError, ErrorUSMFailure))
	assert.Zero(t, events.count(EventDynamicEngineIDRegistered, ""))
}

// A message whose msgSecurityModel is not USM has no security model the
// receiver supports (ErrUnknownSecurityModels, usm_failures), whatever its flags.
func TestV3SecurityLevelNonUSMSecurityModelIsDropped(t *testing.T) {
	wrongKeySpec := func(engineIDHex string) v3SecuredTrapSpec {
		spec := securityLevelSpec(engineIDHex)
		spec.authKey = "wrongauthpassword"
		return spec
	}

	t.Run("trap", func(t *testing.T) {
		recv, events := newSecurityLevelTestReceiver(t, []USMUser{securityLevelAuthUser(testEngineIDHex)})
		peer := &net.UDPAddr{IP: net.ParseIP("10.1.2.3"), Port: 9162}
		data := setV3SecurityModel(t, buildV3SecuredTrapWithFlags(t, wrongKeySpec(testEngineIDHex), gosnmp.AuthNoPriv), 1)

		result := recv.Process(Datagram{Data: data, PeerIP: peer.IP, Peer: peer})

		assert.Nil(t, result.PDU)
		assert.Equal(t, 1, events.count(EventError, ErrorUSMFailure))
	})

	t.Run("inform is not acknowledged", func(t *testing.T) {
		recv, events := newSecurityLevelTestReceiver(t, []USMUser{securityLevelAuthUser(testEngineIDHex)})
		listenerConn, peerConn := udpConnPair(t)
		peer := peerConn.LocalAddr().(*net.UDPAddr)
		inform := buildV3SecuredInformWithFlags(t, wrongKeySpec(testLocalEngineIDHex), gosnmp.AuthNoPriv)
		data := setV3SecurityModel(t, inform, 1)

		result := recv.Process(Datagram{Data: data, PeerIP: peer.IP, Peer: peer, Conn: listenerConn})

		assert.Nil(t, result.PDU)
		assert.Equal(t, 1, events.count(EventError, ErrorUSMFailure))
		requireNoResponse(t, peerConn)
	})

	t.Run("dynamic engine ID is not registered", func(t *testing.T) {
		recv, events, peer := newDynamicTestReceiver(t, 10, RateLimitConfig{})
		trap := buildV3SecuredTrapWithFlags(t, wrongKeySpec(securityLevelOtherEngineIDHex), gosnmp.AuthNoPriv)
		data := setV3SecurityModel(t, trap, 1)

		result := recv.Process(Datagram{Data: data, PeerIP: peer.IP, Peer: peer})

		assert.Nil(t, result.PDU)
		assert.Equal(t, 1, events.count(EventError, ErrorUSMFailure))
		assert.Zero(t, events.count(EventDynamicEngineIDRegistered, ""))
	})
}

func TestV3SecurityLevelDynamicNoAuthUserIsAccepted(t *testing.T) {
	recorder := &receiverEventRecorder{}
	recv := New(NewPolicy(PolicyConfig{
		Versions:        []string{"v3"},
		USMUsers:        []USMUser{securityLevelNoAuthUser("")},
		DynamicEngineID: true,
	}), recorder.report)
	require.NoError(t, recv.PrepareV3(t.TempDir(), "security-level-dynamic-noauth-test"))
	t.Cleanup(recv.RollbackPreparedState)
	peer := &net.UDPAddr{IP: net.ParseIP("10.1.2.3"), Port: 9162}

	data := buildV3TrapWithEngineID(t, "testuser", securityLevelOtherEngineIDHex, "1.3.6.1.6.3.1.1.5.1")
	result := recv.Process(Datagram{Data: data, PeerIP: peer.IP, Peer: peer})

	assert.NotNil(t, result.PDU)
	assert.Equal(t, 0, recorder.count(EventError, ErrorUSMFailure))
	assert.Equal(t, 1, recorder.count(EventDynamicEngineIDRegistered, ""))
}

func TestV3SecurityLevelDynamicNoAuthUserWithEngineIDAcceptsOtherEngines(t *testing.T) {
	recorder := &receiverEventRecorder{}
	recv := New(NewPolicy(PolicyConfig{
		Versions:        []string{"v3"},
		USMUsers:        []USMUser{securityLevelNoAuthUser(testEngineIDHex)},
		DynamicEngineID: true,
	}), recorder.report)
	require.NoError(t, recv.PrepareV3(t.TempDir(), "security-level-dynamic-noauth-engine-test"))
	t.Cleanup(recv.RollbackPreparedState)
	peer := &net.UDPAddr{IP: net.ParseIP("10.1.2.3"), Port: 9162}

	data := buildV3TrapWithEngineID(t, "testuser", securityLevelOtherEngineIDHex, "1.3.6.1.6.3.1.1.5.1")
	result := recv.Process(Datagram{Data: data, PeerIP: peer.IP, Peer: peer})

	assert.NotNil(t, result.PDU)
	assert.Equal(t, 0, recorder.count(EventError, ErrorUSMFailure))
}

// A user with a no-auth entry without engine ID and an auth entry bound to one engine: the engine-bound entry decides
// for its engine, the wildcard entry for every other engine.
func TestV3SecurityLevelDynamicMixedLevelUser(t *testing.T) {
	type outcome struct {
		accepted      bool
		usmFailures   int
		decodeFailure bool
	}
	tests := map[string]struct {
		engineIDHex string
		want        outcome
	}{
		"unauthenticated trap from an unbound engine is accepted": {
			engineIDHex: securityLevelOtherEngineIDHex,
			want: outcome{
				accepted:    true,
				usmFailures: 0,
			},
		},
		"unauthenticated trap from the auth-bound engine is dropped": {
			engineIDHex: testEngineIDHex,
			want: outcome{
				accepted:      false,
				usmFailures:   1,
				decodeFailure: true,
			},
		},
	}

	for name, tc := range tests {
		t.Run(name, func(t *testing.T) {
			recorder := &receiverEventRecorder{}
			recv := New(NewPolicy(PolicyConfig{
				Versions:        []string{"v3"},
				USMUsers:        []USMUser{securityLevelNoAuthUser(""), securityLevelAuthUser(testEngineIDHex)},
				DynamicEngineID: true,
			}), recorder.report)
			require.NoError(t, recv.PrepareV3(t.TempDir(), "security-level-dynamic-mixed-test"))
			t.Cleanup(recv.RollbackPreparedState)
			peer := &net.UDPAddr{IP: net.ParseIP("10.1.2.3"), Port: 9162}

			data := buildV3TrapWithEngineID(t, "testuser", tc.engineIDHex, "1.3.6.1.6.3.1.1.5.1")
			result := recv.Process(Datagram{Data: data, PeerIP: peer.IP, Peer: peer})

			got := outcome{
				accepted:      result.PDU != nil,
				usmFailures:   recorder.count(EventError, ErrorUSMFailure),
				decodeFailure: result.DecodeFailure != nil,
			}
			assert.Equal(t, tc.want, got)
		})
	}
}
