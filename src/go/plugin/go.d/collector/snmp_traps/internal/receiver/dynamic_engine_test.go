// SPDX-License-Identifier: GPL-3.0-or-later

package receiver

import (
	"net"
	"sync"
	"sync/atomic"
	"testing"

	"github.com/netdata/gosnmp"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// A dynamic job learns the engine ID of an authentic, unreportable trap from a
// configured user name: the receiver registers the name's users for it, at
// most dynamic_engine_id_max_pairs names per engine in all, and reports each
// registration once.

func newDynamicTestReceiver(t *testing.T, max int, rateLimit RateLimitConfig) (*Receiver, *receiverEventRecorder, *net.UDPAddr) {
	t.Helper()
	user := USMUser{
		Username:  "testuser",
		AuthProto: "sha256",
		AuthKey:   "authpassword",
		PrivProto: "aes",
		PrivKey:   "privpassword",
	}
	recorder := &receiverEventRecorder{}
	recv := New(NewPolicy(PolicyConfig{
		Versions:           []string{"v3"},
		USMUsers:           []USMUser{user},
		DynamicEngineID:    true,
		DynamicEngineIDMax: max,
		RateLimit:          rateLimit,
	}), recorder.report)
	if err := recv.PrepareV3(t.TempDir(), "dynamic-test"); err != nil {
		t.Fatalf("prepare v3 receiver: %v", err)
	}
	t.Cleanup(recv.RollbackPreparedState)
	return recv, recorder, &net.UDPAddr{IP: net.ParseIP("10.1.2.3"), Port: 9162}
}

func buildDynamicV3Trap(t *testing.T, user, engineIDHex, trapOID string) []byte {
	t.Helper()
	return buildV3SecuredTrapWithFlags(t, dynamicV3Spec(user, engineIDHex, trapOID), gosnmp.AuthPriv)
}

func dynamicV3Spec(user, engineIDHex, trapOID string) v3SecuredTrapSpec {
	return v3SecuredTrapSpec{
		user:        user,
		engineIDHex: engineIDHex,
		authProto:   "sha256",
		privProto:   "aes",
		authKey:     "authpassword",
		privKey:     "privpassword",
		trapOID:     trapOID,
	}
}

func TestDynamicEngineIDTrapRegistration(t *testing.T) {
	data := buildDynamicV3Trap(t, "testuser", testEngineIDHex, "1.3.6.1.6.3.1.1.5.1")
	recv, events, peer := newDynamicTestReceiver(t, 0, RateLimitConfig{})
	// The second trap is a replay of the first: the time window accepts it.
	for range 2 {
		if result := recv.Process(Datagram{Data: data, PeerIP: peer.IP, Peer: peer}); result.PDU == nil {
			t.Fatalf("result = %+v, want accepted trap", result)
		}
	}
	if got := events.count(EventDynamicEngineIDRegistered, ""); got != 1 {
		t.Fatalf("dynamic registration events = %d, want 1", got)
	}
}

// Past the cap, a trap from an engine with no users is an unknown engine.
func TestDynamicEngineIDCapRejectsNewPairs(t *testing.T) {
	first := buildDynamicV3Trap(t, "testuser", testEngineIDHex, "1.3.6.1.6.3.1.1.5.1")
	secondEngineID := "80001f888077dfe44faa700259"
	second := buildDynamicV3Trap(t, "testuser", secondEngineID, "1.3.6.1.6.3.1.1.5.1")
	recv, events, peer := newDynamicTestReceiver(t, 1, RateLimitConfig{})
	if result := recv.Process(Datagram{Data: first, PeerIP: peer.IP, Peer: peer}); result.PDU == nil {
		t.Fatalf("first result = %+v, want accepted trap", result)
	}
	result := recv.Process(Datagram{Data: second, PeerIP: peer.IP, Peer: peer})
	require.Nil(t, result.PDU)
	require.NotNil(t, result.DecodeFailure)
	assert.Equal(t, ErrorUnknownEngine, result.DecodeFailure.Kind)
	assert.Equal(t, secondEngineID, result.DecodeFailure.EngineID)
	assert.Equal(t, 1, events.count(EventDynamicEngineIDRegistered, ""))
	assert.Equal(t, 1, events.count(EventError, "unknown_engine_id"))
}

// A reportable message asks its authoritative engine, which the receiver is
// not for another engine ID: it is never learned.
func TestDynamicEngineIDSkipsReportableTrap(t *testing.T) {
	data := setV3Reportable(t, buildDynamicV3Trap(t, "testuser", testEngineIDHex, "1.3.6.1.6.3.1.1.5.1"))
	recv, events, peer := newDynamicTestReceiver(t, 0, RateLimitConfig{})
	result := recv.Process(Datagram{Data: data, PeerIP: peer.IP, Peer: peer})
	require.NotNil(t, result.DecodeFailure)
	assert.Equal(t, ErrorUnknownEngine, result.DecodeFailure.Kind)
	assert.Zero(t, events.count(EventDynamicEngineIDRegistered, ""))
}

func TestDynamicEngineIDConcurrentDuplicateRegistration(t *testing.T) {
	data := buildDynamicV3Trap(t, "testuser", testEngineIDHex, "1.3.6.1.6.3.1.1.5.1")
	recv, events, peer := newDynamicTestReceiver(t, 0, RateLimitConfig{})
	var accepted atomic.Int64
	var wg sync.WaitGroup
	for range 16 {
		wg.Go(func() {
			if result := recv.Process(Datagram{Data: data, PeerIP: peer.IP, Peer: peer}); result.PDU != nil {
				accepted.Add(1)
			}
		})
	}
	wg.Wait()

	if got := accepted.Load(); got != 16 {
		t.Fatalf("accepted traps = %d, want 16", got)
	}
	if got := events.count(EventDynamicEngineIDRegistered, ""); got != 1 {
		t.Fatalf("dynamic registration events = %d, want 1", got)
	}
}

// An inform naming another engine is answered by that engine: one that reaches
// learning (unreportable, from a name the job may learn) is not learned, and a
// trap from that engine is learned after it.
func TestDynamicEngineIDDoesNotRegisterInform(t *testing.T) {
	recorder := &receiverEventRecorder{}
	recv := New(NewPolicy(PolicyConfig{
		Versions:        []string{"v3"},
		USMUsers:        []USMUser{securityLevelNoAuthUser("")},
		DynamicEngineID: true,
	}), recorder.report)
	require.NoError(t, recv.PrepareV3(t.TempDir(), "dynamic-inform-test"))
	t.Cleanup(recv.RollbackPreparedState)
	peer := &net.UDPAddr{IP: net.ParseIP("10.1.2.3"), Port: 9162}

	inform := setV3Flags(t, buildV3InformWithEngineID(t, "testuser", testEngineIDHex, "1.3.6.1.6.3.1.1.5.1"), byte(gosnmp.NoAuthNoPriv))
	result := recv.Process(Datagram{Data: inform, PeerIP: peer.IP, Peer: peer})
	require.NotNil(t, result.DecodeFailure)
	assert.Equal(t, ErrorUnknownEngine, result.DecodeFailure.Kind)
	assert.ErrorContains(t, result.DecodeFailure.Err, "asks another engine ID", "the inform decoded and was refused, not left unanswered")
	assert.Zero(t, recorder.count(EventDynamicEngineIDRegistered, ""))

	trap := buildV3TrapWithEngineID(t, "testuser", testEngineIDHex, "1.3.6.1.6.3.1.1.5.1")
	assert.NotNil(t, recv.Process(Datagram{Data: trap, PeerIP: peer.IP, Peer: peer}).PDU)
	assert.Equal(t, 1, recorder.count(EventDynamicEngineIDRegistered, ""))
}

func TestDynamicEngineIDNoStateForUnknownUsername(t *testing.T) {
	data := buildDynamicV3Trap(t, "otheruser", testEngineIDHex, "1.3.6.1.6.3.1.1.5.1")
	recv, events, peer := newDynamicTestReceiver(t, 0, RateLimitConfig{})
	if result := recv.Process(Datagram{Data: data, PeerIP: peer.IP, Peer: peer}); result.DecodeFailure == nil {
		t.Fatalf("result = %+v, want decode failure", result)
	}
	assert.Zero(t, events.count(EventDynamicEngineIDRegistered, ""))

	trap := buildDynamicV3Trap(t, "testuser", testEngineIDHex, "1.3.6.1.6.3.1.1.5.1")
	assert.NotNil(t, recv.Process(Datagram{Data: trap, PeerIP: peer.IP, Peer: peer}).PDU)
	assert.Equal(t, 1, events.count(EventDynamicEngineIDRegistered, ""))
}

// Learning runs in the decode, before the per-source rate limit: a trap the
// rate limit drops still registers its engine.
func TestDynamicEngineIDRateLimitDropFollowsRegistration(t *testing.T) {
	data := buildDynamicV3Trap(t, "testuser", testEngineIDHex, "1.3.6.1.6.3.1.1.5.1")
	recv, events, peer := newDynamicTestReceiver(t, 0, RateLimitConfig{Enabled: true, PerSourcePPS: 1, Mode: "drop"})
	if allowed, checked := recv.allowRateLimitedPacket(peer); !allowed || !checked {
		t.Fatal("expected first token to be available")
	}

	if result := recv.Process(Datagram{Data: data, PeerIP: peer.IP, Peer: peer}); result.PDU != nil || result.DecodeFailure != nil {
		t.Fatalf("result = %+v, want rate-limit drop", result)
	}
	if got := events.count(EventError, "rate_limited"); got != 1 {
		t.Fatalf("rate_limited events = %d, want 1", got)
	}
	if got := events.count(EventDynamicEngineIDRegistered, ""); got != 1 {
		t.Fatalf("dynamic registration events = %d, want 1", got)
	}
}

// A failed learning attempt is a decode failure with the learned user's error;
// its audit takes the source's first rate-limit token.
func TestDynamicDecodeFailureAuditTakesRateLimitToken(t *testing.T) {
	spec := dynamicV3Spec("testuser", testEngineIDHex, "1.3.6.1.6.3.1.1.5.1")
	spec.authKey = "wrongpassword"
	data := buildV3SecuredTrapWithFlags(t, spec, gosnmp.AuthPriv)
	recv, events, peer := newDynamicTestReceiver(t, 0, RateLimitConfig{Enabled: true, PerSourcePPS: 1, Mode: "drop"})

	result := recv.Process(Datagram{Data: data, PeerIP: peer.IP, Peer: peer})
	require.NotNil(t, result.DecodeFailure)
	assert.Equal(t, ErrorAuthFailure, result.DecodeFailure.Kind)
	if !recv.AdmitDecodeErrorAudit(result.DecodeFailure) {
		t.Fatal("first dynamic decode failure should be auditable")
	}
	if got := events.count(EventError, "rate_limited"); got != 0 {
		t.Fatalf("rate_limited events = %d, want 0", got)
	}
	assert.Zero(t, events.count(EventDynamicEngineIDRegistered, ""))
}

// A decode failure takes one rate-limit decision, at its audit.
func TestDynamicDecodeFailureSampleAdmissionIsNotRepeated(t *testing.T) {
	spec := dynamicV3Spec("testuser", testEngineIDHex, "1.3.6.1.6.3.1.1.5.1")
	spec.authKey = "wrongpassword"
	data := buildV3SecuredTrapWithFlags(t, spec, gosnmp.AuthPriv)
	recv, events, peer := newDynamicTestReceiver(t, 0, RateLimitConfig{Enabled: true, PerSourcePPS: 1, Mode: "sample"})
	if allowed, checked := recv.allowRateLimitedPacket(peer); !allowed || !checked {
		t.Fatal("expected first token to be available")
	}

	result := recv.Process(Datagram{Data: data, PeerIP: peer.IP, Peer: peer})
	if result.DecodeFailure == nil {
		t.Fatalf("result = %+v, want decode failure", result)
	}
	if got := events.count(EventError, "rate_limited"); got != 0 {
		t.Fatalf("rate_limited events before audit = %d, want 0", got)
	}
	if !recv.AdmitDecodeErrorAudit(result.DecodeFailure) {
		t.Fatal("sampled dynamic decode failure should be auditable")
	}
	if got := events.count(EventError, "rate_limited"); got != 1 {
		t.Fatalf("rate_limited events after audit = %d, want 1", got)
	}
}

func TestDynamicEngineIDRateLimitSampleAllowsRetry(t *testing.T) {
	data := buildDynamicV3Trap(t, "testuser", testEngineIDHex, "1.3.6.1.6.3.1.1.5.1")
	recv, events, peer := newDynamicTestReceiver(t, 0, RateLimitConfig{Enabled: true, PerSourcePPS: 1, Mode: "sample"})
	if allowed, checked := recv.allowRateLimitedPacket(peer); !allowed || !checked {
		t.Fatal("expected first token to be available")
	}

	if result := recv.Process(Datagram{Data: data, PeerIP: peer.IP, Peer: peer}); result.PDU == nil {
		t.Fatalf("result = %+v, want sampled acceptance", result)
	}
	if got := events.count(EventError, "rate_limited"); got != 1 {
		t.Fatalf("rate_limited events = %d, want 1", got)
	}
	if got := events.count(EventDynamicEngineIDRegistered, ""); got != 1 {
		t.Fatalf("dynamic registration events = %d, want 1", got)
	}
}

// Engine IDs of all 0x00 or all 0xff octets name no engine (RFC 3411
// SnmpEngineID): a dynamic job drops them before the decode and learns none.
func TestDynamicEngineIDDropsDegenerateEngineIDs(t *testing.T) {
	for name, engineIDHex := range map[string]string{
		"all 0x00": "0000000000",
		"all 0xff": "ffffffffff",
	} {
		t.Run(name, func(t *testing.T) {
			data := buildDynamicV3Trap(t, "testuser", engineIDHex, "1.3.6.1.6.3.1.1.5.1")
			recv, events, peer := newDynamicTestReceiver(t, 0, RateLimitConfig{})
			assert.Equal(t, Result{}, recv.Process(Datagram{Data: data, PeerIP: peer.IP, Peer: peer}))
			assert.Equal(t, 1, events.count(EventError, "unknown_engine_id"))
			assert.Zero(t, events.count(EventDynamicEngineIDRegistered, ""))
		})
	}
}

// The receiver learns an engine when the message authenticates, before the
// collector's own checks: a learned engine is reported once also when those
// checks then reject the message.
func TestDynamicEngineIDRegistrationReportedWhenDecodeFails(t *testing.T) {
	spec := dynamicV3Spec("testuser", testEngineIDHex, "1.3.6.1.6.3.1.1.5.1")
	spec.extra = []gosnmp.VarBind{{Name: "1.3.6.1.4.1.99999.1", Value: gosnmp.OctetStringValue(make([]byte, maxOctetStringLen+1))}}
	malformed := buildV3SecuredTrapWithFlags(t, spec, gosnmp.AuthPriv)
	recv, events, peer := newDynamicTestReceiver(t, 0, RateLimitConfig{})

	result := recv.Process(Datagram{Data: malformed, PeerIP: peer.IP, Peer: peer})
	require.NotNil(t, result.DecodeFailure)
	assert.Equal(t, ErrorMalformedPDU, result.DecodeFailure.Kind)
	assert.Equal(t, 1, events.count(EventDynamicEngineIDRegistered, ""))

	trap := buildDynamicV3Trap(t, "testuser", testEngineIDHex, "1.3.6.1.6.3.1.1.5.1")
	assert.NotNil(t, recv.Process(Datagram{Data: trap, PeerIP: peer.IP, Peer: peer}).PDU)
	assert.Equal(t, 1, events.count(EventDynamicEngineIDRegistered, ""), "the engine was learned by the first message")
}
