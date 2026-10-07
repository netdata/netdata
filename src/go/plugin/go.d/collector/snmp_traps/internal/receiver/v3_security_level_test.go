// SPDX-License-Identifier: GPL-3.0-or-later

package receiver

import (
	"errors"
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
	assert.True(t, errors.Is(err, os.ErrDeadlineExceeded), "want no INFORM response, got err=%v", err)
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
