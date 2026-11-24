// SPDX-License-Identifier: GPL-3.0-or-later

package receiver

import (
	"bytes"
	"encoding/hex"
	"net"
	"testing"

	"github.com/netdata/gosnmp"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// Process sends the responses the receiver makes: the Response to an accepted
// inform, before the rate limit, and the Report of a rejected SNMPv3 message,
// after a rate-limit token. The library pins their wire format.

func TestReceiverRespondsBeforeRateLimitDrop(t *testing.T) {
	reqData := buildV2cPDU(t, gosnmp.InformRequest, "public", "1.3.6.1.6.3.1.1.5.1")

	listenerConn, peerConn := udpConnPair(t)

	peer := peerConn.LocalAddr().(*net.UDPAddr)
	var events []Event
	recv := New(NewPolicy(PolicyConfig{
		Versions:    []string{"v2c"},
		Communities: []string{"public"},
		RateLimit:   RateLimitConfig{Enabled: true, PerSourcePPS: 1, Mode: "drop"},
	}), func(event Event) { events = append(events, event) })
	prime := recv.Process(Datagram{
		Data:   buildV2cTrap(t, "public", "1.3.6.1.6.3.1.1.5.1"),
		PeerIP: peer.IP,
		Peer:   peer,
	})
	if prime.PDU == nil {
		t.Fatalf("priming trap result = %+v, want accepted trap", prime)
	}

	result := recv.Process(Datagram{Data: reqData, PeerIP: peer.IP, Conn: listenerConn, Peer: peer})

	response, _, err := newPlainReceiver(t).Receive(readResponse(t, peerConn))
	require.NoError(t, err)
	if response.Message.PDUType != gosnmp.Response {
		t.Fatalf("response PDU type = %s, want Response", response.Message.PDUType)
	}
	if result.PDU != nil || result.DecodeFailure != nil {
		t.Fatalf("rate-limited INFORM result = %+v, want drop", result)
	}
	if countEvents(events, EventError, "rate_limited") != 1 {
		t.Fatalf("events = %+v, want one rate_limited error", events)
	}
}

func TestReceiverReportsInformResponseFailed(t *testing.T) {
	reqData := buildV2cPDU(t, gosnmp.InformRequest, "public", "1.3.6.1.6.3.1.1.5.1")

	listenerConn, peerConn := udpConnPair(t)
	peer := peerConn.LocalAddr().(*net.UDPAddr)
	if err := listenerConn.Close(); err != nil {
		t.Fatalf("close listener socket: %v", err)
	}

	var events []Event
	recv := New(NewPolicy(PolicyConfig{Versions: []string{"v2c"}, Communities: []string{"public"}}), func(event Event) {
		events = append(events, event)
	})
	result := recv.Process(Datagram{Data: reqData, PeerIP: peer.IP, Conn: listenerConn, Peer: peer})
	if result.PDU == nil {
		t.Fatalf("result = %+v, want accepted INFORM", result)
	}
	if countEvents(events, EventInformResponseFailed, "") != 1 {
		t.Fatalf("events = %+v, want one INFORM response failure", events)
	}
}

// An authPriv inform to the local engine is answered by the local engine: its
// engine ID and the boots PrepareV3 raised, under the user that accepted it.
func TestV3InformResponseComesFromLocalEngine(t *testing.T) {
	user := USMUser{
		Username:  "testuser",
		EngineID:  testEngineIDHex,
		AuthProto: "sha256",
		AuthKey:   "authpassword",
		PrivProto: "aes",
		PrivKey:   "privpassword",
	}
	recorder := &receiverEventRecorder{}
	recv := New(NewPolicy(PolicyConfig{
		Versions:          []string{"v3"},
		USMUsers:          []USMUser{user},
		EngineIDWhitelist: []string{testEngineIDHex},
		LocalEngineID:     testLocalEngineIDHex,
	}), recorder.report)
	require.NoError(t, recv.PrepareV3(t.TempDir(), "inform-response-test"))
	t.Cleanup(recv.RollbackPreparedState)

	listenerConn, peerConn := udpConnPair(t)
	peer := peerConn.LocalAddr().(*net.UDPAddr)

	data := buildV3SecuredInform(t, v3SecuredTrapSpec{
		user:        "testuser",
		engineIDHex: testLocalEngineIDHex,
		authProto:   "sha256",
		privProto:   "aes",
		authKey:     "authpassword",
		privKey:     "privpassword",
		trapOID:     "1.3.6.1.6.3.1.1.5.1",
	})
	result := recv.Process(Datagram{Data: data, PeerIP: peer.IP, Conn: listenerConn, Peer: peer})
	require.NotNil(t, result.PDU, "decode failure: %+v", result.DecodeFailure)

	localEngineID, err := hex.DecodeString(testLocalEngineIDHex)
	require.NoError(t, err)
	decoder, err := gosnmp.NewReceiver(gosnmp.ReceiverConfig{Users: []gosnmp.EngineUser{{
		EngineID: localEngineID,
		User: gosnmp.User{
			Name:           "testuser",
			AuthProtocol:   gosnmp.SHA256,
			AuthPassphrase: "authpassword",
			PrivProtocol:   gosnmp.AES,
			PrivPassphrase: "privpassword",
		},
	}}})
	require.NoError(t, err)
	response, _, err := decoder.Receive(readResponse(t, peerConn))
	require.NoError(t, err)
	msg := response.Message
	assert.Equal(t, gosnmp.Response, msg.PDUType)
	assert.Equal(t, result.PDU.RequestID, msg.RequestID)
	assert.Equal(t, gosnmp.AuthPriv, msg.SecurityLevel)
	assert.Equal(t, localEngineID, msg.SecurityParameters.AuthoritativeEngineID)
	assert.EqualValues(t, 1, msg.SecurityParameters.AuthoritativeEngineBoots)
	assert.Zero(t, recorder.count(EventInformResponseFailed, ""))
}

// A discovery probe gets a Report from the local engine, which tells the
// manager the engine ID; it echoes the probe's msgID.
func TestDiscoveryProbeIsAnsweredWithReport(t *testing.T) {
	recv, events := newStaticV3TestReceiver(t, []string{testEngineIDHex})
	listenerConn, peerConn := udpConnPair(t)
	peer := peerConn.LocalAddr().(*net.UDPAddr)

	result := recv.Process(Datagram{Data: buildV3DiscoveryProbe(t, 99), PeerIP: peer.IP, Conn: listenerConn, Peer: peer})
	require.NotNil(t, result.DecodeFailure)
	assert.Equal(t, ErrorUnknownEngine, result.DecodeFailure.Kind)
	assert.Empty(t, result.DecodeFailure.EngineID, "the probe names no engine")

	envelope, err := gosnmp.ReadEnvelope(readResponse(t, peerConn))
	require.NoError(t, err)
	localEngineID, err := hex.DecodeString(testLocalEngineIDHex)
	require.NoError(t, err)
	assert.Equal(t, gosnmp.Version3, envelope.Version)
	assert.EqualValues(t, 99, envelope.MsgID)
	assert.False(t, envelope.Reportable)
	assert.True(t, bytes.Equal(localEngineID, envelope.SecurityParameters.AuthoritativeEngineID))
	assert.Zero(t, events.count(EventReportFailed, ""))
}

func TestMalformedDiscoveryRequestDoesNotSendReport(t *testing.T) {
	probe := buildV3DiscoveryProbe(t, 99)
	fields := v3OuterFields(t, probe)
	tests := map[string][]byte{
		"invalid header":                    corruptV3HeaderFieldTag(t, probe, 1, tagOctetStr),
		"missing msgData":                   berTLV(tagSequence, testBERFields(fields[:3]...)),
		"empty plaintext scoped PDU":        replaceV3MsgData(t, probe, berTLV(tagSequence, nil)),
		"encrypted msgData without privacy": replaceV3MsgDataTag(t, probe, tagOctetStr),
		"plaintext msgData with privacy":    setV3Flags(t, probe, byte(gosnmp.AuthPriv)|testReportableFlag),
	}

	for name, data := range tests {
		t.Run(name, func(t *testing.T) {
			listenerConn, peerConn := udpConnPair(t)

			recv, _, _ := newDynamicTestReceiver(t, 0, RateLimitConfig{})
			peer := peerConn.LocalAddr().(*net.UDPAddr)
			result := recv.Process(Datagram{Data: data, PeerIP: peer.IP, Conn: listenerConn, Peer: peer})
			if result.DecodeFailure == nil {
				t.Fatalf("result = %+v, want decode failure", result)
			}
			requireNoResponse(t, peerConn)
		})
	}
}

func TestReportWriteFailureIsReported(t *testing.T) {
	data := buildV3DiscoveryProbe(t, 99)
	listenerConn, peerConn := udpConnPair(t)
	peer := peerConn.LocalAddr().(*net.UDPAddr)
	if err := listenerConn.Close(); err != nil {
		t.Fatalf("close response socket: %v", err)
	}

	recv, events, _ := newDynamicTestReceiver(t, 0, RateLimitConfig{Enabled: true, PerSourcePPS: 1, Mode: "drop"})
	result := recv.Process(Datagram{Data: data, PeerIP: peer.IP, Conn: listenerConn, Peer: peer})

	if result.PDU != nil || result.DecodeFailure == nil {
		t.Fatalf("result = %+v, want decode failure after the Report", result)
	}
	if got := events.count(EventReportFailed, ""); got != 1 {
		t.Fatalf("Report failure events = %d, want 1", got)
	}
	if !recv.AdmitDecodeErrorAudit(result.DecodeFailure) {
		t.Fatal("the Report's rate-limit token admits the decode failure's audit")
	}
	if got := events.count(EventError, "rate_limited"); got != 0 {
		t.Fatalf("rate_limited events = %d, want 0", got)
	}
}

func TestReportRateLimitDropSkipsResponse(t *testing.T) {
	data := buildV3DiscoveryProbe(t, 99)
	listenerConn, peerConn := udpConnPair(t)

	recv, events, _ := newDynamicTestReceiver(t, 0, RateLimitConfig{Enabled: true, PerSourcePPS: 1, Mode: "drop"})
	peer := peerConn.LocalAddr().(*net.UDPAddr)
	if allowed, checked := recv.allowRateLimitedPacket(peer); !allowed || !checked {
		t.Fatal("expected first token to be available")
	}

	if result := recv.Process(Datagram{Data: data, PeerIP: peer.IP, Conn: listenerConn, Peer: peer}); result.PDU != nil || result.DecodeFailure != nil {
		t.Fatalf("result = %+v, want rate-limit drop", result)
	}
	if got := events.count(EventError, "rate_limited"); got != 1 {
		t.Fatalf("rate_limited events = %d, want 1", got)
	}
	requireNoResponse(t, peerConn)
}
