// SPDX-License-Identifier: GPL-3.0-or-later

package receiver

import (
	"net"
	"os"
	"testing"
	"time"

	"github.com/gosnmp/gosnmp"
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

func TestV3SecurityLevelStaticUsers(t *testing.T) {
	type outcome struct {
		accepted    bool
		usmFailures int
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
				accepted:    false,
				usmFailures: 1,
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
				accepted:    false,
				usmFailures: 1,
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
				accepted:    false,
				usmFailures: 1,
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
				accepted:    result.PDU != nil,
				usmFailures: events.count(EventError, ErrorUSMFailure),
			}
			assert.Equal(t, tc.want, got)
			assert.Nil(t, result.DecodeFailure)
		})
	}
}

func TestV3SecurityLevelUnauthenticatedInformIsNotAcknowledged(t *testing.T) {
	recv, events := newSecurityLevelTestReceiver(t, []USMUser{securityLevelAuthUser(testEngineIDHex)})
	listenerConn, peerConn := informUDPConnPair(t)
	defer listenerConn.Close()
	defer peerConn.Close()
	peer := peerConn.LocalAddr().(*net.UDPAddr)

	data := buildV3InformWithEngineID(t, "testuser", testLocalEngineIDHex, "1.3.6.1.6.3.1.1.5.1")
	result := recv.Process(Datagram{
		Data:   data,
		PeerIP: peer.IP,
		Peer:   peer,
		Conn:   listenerConn,
	})

	assert.Nil(t, result.PDU)
	assert.Nil(t, result.DecodeFailure)
	assert.Equal(t, 1, events.count(EventError, ErrorUSMFailure))

	require.NoError(t, peerConn.SetReadDeadline(time.Now().Add(200*time.Millisecond)))
	_, _, err := peerConn.ReadFromUDP(make([]byte, 2048))
	assert.ErrorIs(t, err, os.ErrDeadlineExceeded, "want no INFORM response")
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
	assert.Equal(t, 0, recv.dynamicPairs())
}

// A message whose msgSecurityModel is not USM decodes without digest verification, even with auth flags set.
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
		listenerConn, peerConn := informUDPConnPair(t)
		defer listenerConn.Close()
		defer peerConn.Close()
		peer := peerConn.LocalAddr().(*net.UDPAddr)
		inform := buildV3SecuredInformWithFlags(t, wrongKeySpec(testLocalEngineIDHex), gosnmp.AuthNoPriv|gosnmp.Reportable)
		data := setV3SecurityModel(t, inform, 1)

		result := recv.Process(Datagram{Data: data, PeerIP: peer.IP, Peer: peer, Conn: listenerConn})

		assert.Nil(t, result.PDU)
		assert.Equal(t, 1, events.count(EventError, ErrorUSMFailure))
		require.NoError(t, peerConn.SetReadDeadline(time.Now().Add(200*time.Millisecond)))
		_, _, err := peerConn.ReadFromUDP(make([]byte, 2048))
		assert.ErrorIs(t, err, os.ErrDeadlineExceeded, "want no INFORM response")
	})

	t.Run("dynamic engine ID is not registered", func(t *testing.T) {
		recv, events, peer := newDynamicTestReceiver(t, 10, RateLimitConfig{})
		trap := buildV3SecuredTrapWithFlags(t, wrongKeySpec(securityLevelOtherEngineIDHex), gosnmp.AuthNoPriv)
		data := setV3SecurityModel(t, trap, 1)

		result := recv.Process(Datagram{Data: data, PeerIP: peer.IP, Peer: peer})

		assert.Nil(t, result.PDU)
		assert.Equal(t, 1, events.count(EventError, ErrorUSMFailure))
		assert.Equal(t, 0, recv.dynamicPairs())
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
	assert.Equal(t, 1, recv.dynamicPairs())
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
		accepted    bool
		usmFailures int
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
				accepted:    false,
				usmFailures: 1,
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
				accepted:    result.PDU != nil,
				usmFailures: recorder.count(EventError, ErrorUSMFailure),
			}
			assert.Equal(t, tc.want, got)
		})
	}
}

// setV3SecurityModel rewrites the single-byte msgSecurityModel of an encoded SNMPv3 message.
func setV3SecurityModel(t *testing.T, data []byte, model byte) []byte {
	t.Helper()
	out := append([]byte(nil), data...)

	tag, valueStart, valueEnd, _, err := readBERElement(out, 0)
	require.NoError(t, err)
	require.Equal(t, byte(tagSequence), tag)
	_, _, _, pos, err := readBERElement(out[:valueEnd], valueStart) // msgVersion
	require.NoError(t, err)
	tag, gdStart, gdEnd, _, err := readBERElement(out[:valueEnd], pos) // msgGlobalData
	require.NoError(t, err)
	require.Equal(t, byte(tagSequence), tag)

	gdPos := gdStart
	for range 3 { // msgID, msgMaxSize, msgFlags
		_, _, _, gdPos, err = readBERElement(out[:gdEnd], gdPos)
		require.NoError(t, err)
	}
	tag, modelStart, modelEnd, _, err := readBERElement(out[:gdEnd], gdPos)
	require.NoError(t, err)
	require.Equal(t, byte(tagInteger), tag)
	require.Equal(t, 1, modelEnd-modelStart)

	out[modelStart] = model
	return out
}
