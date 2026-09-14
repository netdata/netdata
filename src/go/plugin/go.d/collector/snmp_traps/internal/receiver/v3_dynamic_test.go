// SPDX-License-Identifier: GPL-3.0-or-later

package receiver

import (
	"encoding/hex"
	"net"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/gosnmp/gosnmp"
)

type receiverEventRecorder struct {
	mu     sync.Mutex
	events []Event
}

func (r *receiverEventRecorder) report(event Event) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.events = append(r.events, event)
}

func (r *receiverEventRecorder) count(eventType EventType, errorKind ErrorKind) int {
	r.mu.Lock()
	defer r.mu.Unlock()
	return countEvents(r.events, eventType, errorKind)
}

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

func buildDynamicV3Inform(t *testing.T, user, engineIDHex, trapOID string) []byte {
	t.Helper()
	return buildV3SecuredInformWithFlags(t, dynamicV3Spec(user, engineIDHex, trapOID), gosnmp.AuthPriv)
}

func buildDynamicV3ReportableTrap(t *testing.T, user, engineIDHex, trapOID string) []byte {
	t.Helper()
	spec := dynamicV3Spec(user, engineIDHex, trapOID)
	spec.authKey = "wrongpassword"
	return buildV3SecuredTrapWithFlags(t, spec, gosnmp.AuthPriv|gosnmp.Reportable)
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

func TestExtractRawV3ContextTrap(t *testing.T) {
	data := clearV3ReportableFlag(t, buildDynamicV3Trap(t, "testuser", testEngineIDHex, "1.3.6.1.6.3.1.1.5.1"))

	ctx, err := extractRawV3Context(data)
	if err != nil {
		t.Fatalf("extractRawV3Context failed: %v", err)
	}
	if ctx == nil {
		t.Fatal("expected raw v3 context")
	}
	if ctx.engineID != testEngineIDHex {
		t.Fatalf("engineID = %q, want %q", ctx.engineID, testEngineIDHex)
	}
	if ctx.username != "testuser" {
		t.Fatalf("username = %q, want testuser", ctx.username)
	}
	if ctx.reportable {
		t.Fatal("trap should not be reportable")
	}
	if ctx.discoveryProbe() {
		t.Fatal("trap should not be classified as discovery probe")
	}
}

func TestExtractRawV3ContextReportableDiscoveryProbe(t *testing.T) {
	data := buildV3DiscoveryProbe(t, 99)

	ctx, err := extractRawV3Context(data)
	if err != nil {
		t.Fatalf("extractRawV3Context failed: %v", err)
	}
	if ctx == nil {
		t.Fatal("expected raw v3 context")
	}
	if ctx.engineID != "" {
		t.Fatalf("engineID = %q, want empty", ctx.engineID)
	}
	if !ctx.reportable {
		t.Fatal("expected reportable discovery probe")
	}
	if !ctx.discoveryProbe() {
		t.Fatal("expected discovery probe")
	}
	if ctx.msgID != 99 {
		t.Fatalf("msgID = %d, want 99", ctx.msgID)
	}
}

func TestParseRawV3HeaderRejectsMalformedRequiredFields(t *testing.T) {
	validMsgID := testBERElement(tagInteger, []byte{99})
	validMaxSize := testBERElement(tagInteger, []byte{0x01, 0x00, 0x00})
	validFlags := testBERElement(tagOctetStr, []byte{byte(gosnmp.Reportable)})
	validSecurityModel := testBERElement(tagInteger, []byte{byte(gosnmp.UserSecurityModel)})

	tests := map[string]struct {
		header []byte
	}{
		"invalid msgID encoding": {
			header: testBERFields(
				testBERElement(tagInteger, []byte{0x01, 0x00, 0x00, 0x00, 0x00}),
				validMaxSize,
				validFlags,
				validSecurityModel,
			),
		},
		"msgID exceeds protocol range": {
			header: testBERFields(
				testBERElement(tagInteger, []byte{0x00, 0x80, 0x00, 0x00, 0x00}),
				validMaxSize,
				validFlags,
				validSecurityModel,
			),
		},
		"msgMaxSize has wrong tag": {
			header: testBERFields(
				validMsgID,
				testBERElement(tagOctetStr, []byte{0x01, 0x00, 0x00}),
				validFlags,
				validSecurityModel,
			),
		},
		"invalid msgMaxSize encoding": {
			header: testBERFields(
				validMsgID,
				testBERElement(tagInteger, []byte{0x01, 0x00, 0x00, 0x00, 0x00}),
				validFlags,
				validSecurityModel,
			),
		},
		"msgMaxSize is below protocol minimum": {
			header: testBERFields(
				validMsgID,
				testBERElement(tagInteger, []byte{0x01, 0xe3}),
				validFlags,
				validSecurityModel,
			),
		},
		"msgFlags has wrong length": {
			header: testBERFields(
				validMsgID,
				validMaxSize,
				testBERElement(tagOctetStr, []byte{byte(gosnmp.Reportable), 0}),
				validSecurityModel,
			),
		},
		"msgFlags uses reserved privacy-only level": {
			header: testBERFields(
				validMsgID,
				validMaxSize,
				testBERElement(tagOctetStr, []byte{0x02}),
				validSecurityModel,
			),
		},
		"invalid securityModel encoding": {
			header: testBERFields(
				validMsgID,
				validMaxSize,
				validFlags,
				testBERElement(tagInteger, nil),
			),
		},
		"securityModel is not USM": {
			header: testBERFields(
				validMsgID,
				validMaxSize,
				validFlags,
				testBERElement(tagInteger, []byte{4}),
			),
		},
		"trailing header field": {
			header: testBERFields(
				validMsgID,
				validMaxSize,
				validFlags,
				validSecurityModel,
				testBERElement(tagInteger, []byte{0}),
			),
		},
	}

	for name, tc := range tests {
		t.Run(name, func(t *testing.T) {
			if _, err := parseRawV3Header(tc.header); err == nil {
				t.Fatal("expected malformed v3 header to be rejected")
			}
		})
	}
}

func TestParseRawV3HeaderIgnoresReservedFlagBits(t *testing.T) {
	header := testBERFields(
		testBERElement(tagInteger, []byte{99}),
		testBERElement(tagInteger, []byte{0x01, 0x00, 0x00}),
		testBERElement(tagOctetStr, []byte{byte(gosnmp.Reportable) | 0x80}),
		testBERElement(tagInteger, []byte{byte(gosnmp.UserSecurityModel)}),
	)

	parsed, err := parseRawV3Header(header)
	if err != nil {
		t.Fatalf("parseRawV3Header failed: %v", err)
	}
	if parsed.msgID != 99 || !parsed.reportable {
		t.Fatalf("header = msgID %d/reportable %v, want 99/true", parsed.msgID, parsed.reportable)
	}
}

func TestExtractRawV3ContextRejectsMalformedEnvelope(t *testing.T) {
	data := buildV3DiscoveryProbe(t, 99)
	fields := v3OuterFields(t, data)
	if len(fields) != 4 {
		t.Fatalf("outer field count = %d, want 4", len(fields))
	}
	scopedFields := v3PlaintextScopedPDUFields(t, data)
	if len(scopedFields) != 3 {
		t.Fatalf("scoped PDU field count = %d, want 3", len(scopedFields))
	}
	invalidContextEngineID := append([]byte(nil), scopedFields[0]...)
	invalidContextEngineID[0] = tagInteger
	invalidContextName := append([]byte(nil), scopedFields[1]...)
	invalidContextName[0] = tagInteger

	invalidMsgData := replaceV3MsgDataTag(t, data, tagInteger)
	tests := map[string][]byte{
		"missing msgData":                   berTLV(tagSequence, testBERFields(fields[:3]...)),
		"invalid msgData tag":               invalidMsgData,
		"empty plaintext scoped PDU":        replaceV3MsgData(t, data, testBERSequence()),
		"missing context name and data":     replaceV3MsgData(t, data, testBERSequence(scopedFields[0])),
		"missing scoped PDU data":           replaceV3MsgData(t, data, testBERSequence(scopedFields[:2]...)),
		"invalid context engine ID tag":     replaceV3MsgData(t, data, testBERSequence(invalidContextEngineID, scopedFields[1], scopedFields[2])),
		"invalid context name tag":          replaceV3MsgData(t, data, testBERSequence(scopedFields[0], invalidContextName, scopedFields[2])),
		"malformed scoped PDU data":         replaceV3MsgData(t, data, testBERSequence(scopedFields[0], scopedFields[1], []byte{tagTrapV2, 1})),
		"trailing scoped PDU field":         replaceV3MsgData(t, data, testBERSequence(append(scopedFields, testBERElement(tagOctetStr, nil))...)),
		"trailing outer field":              berTLV(tagSequence, testBERFields(append(fields, testBERElement(tagInteger, []byte{0}))...)),
		"trailing bytes after message":      append(append([]byte(nil), data...), 0),
		"encrypted msgData without privacy": replaceV3MsgDataTag(t, data, tagOctetStr),
		"plaintext msgData with privacy":    setV3Flags(t, data, gosnmp.AuthPriv|gosnmp.Reportable),
	}

	for name, data := range tests {
		t.Run(name, func(t *testing.T) {
			if _, err := extractRawV3Context(data); err == nil {
				t.Fatal("expected malformed v3 envelope to be rejected")
			}
		})
	}
}

func TestParseRawUSMContextRejectsMalformedRequiredFields(t *testing.T) {
	engineID, err := hex.DecodeString(testEngineIDHex)
	if err != nil {
		t.Fatalf("decode test engine ID: %v", err)
	}
	validEngineID := testBERElement(tagOctetStr, engineID)
	validBoots := testBERElement(tagInteger, []byte{1})
	validTime := testBERElement(tagInteger, []byte{1})
	validUsername := testBERElement(tagOctetStr, []byte("testuser"))
	validAuth := testBERElement(tagOctetStr, nil)
	validPriv := testBERElement(tagOctetStr, nil)

	tests := map[string][]byte{
		"engine boots has wrong tag": testBERSequence(
			validEngineID,
			testBERElement(tagOctetStr, []byte{1}),
			validTime,
			validUsername,
			validAuth,
			validPriv,
		),
		"engine time has invalid encoding": testBERSequence(
			validEngineID,
			validBoots,
			testBERElement(tagInteger, nil),
			validUsername,
			validAuth,
			validPriv,
		),
		"username exceeds protocol maximum": testBERSequence(
			validEngineID,
			validBoots,
			validTime,
			testBERElement(tagOctetStr, make([]byte, 33)),
			validAuth,
			validPriv,
		),
		"missing authentication and privacy parameters": testBERSequence(
			validEngineID,
			validBoots,
			validTime,
			validUsername,
		),
		"authentication parameters have wrong tag": testBERSequence(
			validEngineID,
			validBoots,
			validTime,
			validUsername,
			testBERElement(tagInteger, []byte{0}),
			validPriv,
		),
		"privacy parameters have wrong tag": testBERSequence(
			validEngineID,
			validBoots,
			validTime,
			validUsername,
			validAuth,
			testBERElement(tagInteger, []byte{0}),
		),
		"trailing USM field": testBERSequence(
			validEngineID,
			validBoots,
			validTime,
			validUsername,
			validAuth,
			validPriv,
			testBERElement(tagOctetStr, nil),
		),
	}

	for name, data := range tests {
		t.Run(name, func(t *testing.T) {
			if _, _, err := parseRawUSMContext(data, true); err == nil {
				t.Fatal("expected malformed USM context to be rejected")
			}
		})
	}
}

func TestMalformedDiscoveryRequestDoesNotSendReport(t *testing.T) {
	probe := buildV3DiscoveryProbe(t, 99)
	fields := v3OuterFields(t, probe)
	tests := map[string][]byte{
		"invalid header":                    corruptV3HeaderFieldTag(t, probe, 1, tagOctetStr),
		"missing msgData":                   berTLV(tagSequence, testBERFields(fields[:3]...)),
		"empty plaintext scoped PDU":        replaceV3MsgData(t, probe, testBERSequence()),
		"encrypted msgData without privacy": replaceV3MsgDataTag(t, probe, tagOctetStr),
		"plaintext msgData with privacy":    setV3Flags(t, probe, gosnmp.AuthPriv|gosnmp.Reportable),
	}

	for name, data := range tests {
		t.Run(name, func(t *testing.T) {
			listenerConn, peerConn := informUDPConnPair(t)
			defer listenerConn.Close()
			defer peerConn.Close()

			recv, _, _ := newDynamicTestReceiver(t, 0, RateLimitConfig{})
			peer := peerConn.LocalAddr().(*net.UDPAddr)
			result := recv.Process(Datagram{Data: data, PeerIP: peer.IP, Conn: listenerConn, Peer: peer})
			if result.DecodeFailure == nil {
				t.Fatalf("result = %+v, want decode failure", result)
			}

			if err := peerConn.SetReadDeadline(time.Now().Add(50 * time.Millisecond)); err != nil {
				t.Fatalf("set read deadline: %v", err)
			}
			buf := make([]byte, maxDatagramSize)
			_, _, err := peerConn.ReadFromUDP(buf)
			if err == nil {
				t.Fatal("malformed discovery request received a Report")
			}
			if ne, ok := err.(net.Error); !ok || !ne.Timeout() {
				t.Fatalf("read response error = %v, want timeout", err)
			}
		})
	}
}

func TestEngineIDExtractionUsesPartialV3Envelope(t *testing.T) {
	data := corruptV3MessageIDTag(t, buildV3TrapWithEngineID(
		t, "testuser", testEngineIDHex, "1.3.6.1.6.3.1.1.5.1",
	))

	engineID, ok, err := extractSNMPv3EngineIDHex(data)
	if err != nil {
		t.Fatalf("extractSNMPv3EngineIDHex failed: %v", err)
	}
	if !ok || engineID != testEngineIDHex {
		t.Fatalf("engine ID = %q/%v, want %q/true", engineID, ok, testEngineIDHex)
	}
	if _, err := extractRawV3Context(data); err == nil {
		t.Fatal("full raw context should reject a non-integer msgID")
	}
}

func TestDynamicEngineIDTrapRegistration(t *testing.T) {
	data := clearV3ReportableFlag(t, buildDynamicV3Trap(t, "testuser", testEngineIDHex, "1.3.6.1.6.3.1.1.5.1"))
	recv, events, peer := newDynamicTestReceiver(t, 0, RateLimitConfig{})
	for range 2 {
		if result := recv.Process(Datagram{Data: data, PeerIP: peer.IP, Peer: peer}); result.PDU == nil {
			t.Fatalf("result = %+v, want accepted trap", result)
		}
	}
	if got := events.count(EventDynamicEngineIDRegistered, ""); got != 1 {
		t.Fatalf("dynamic registration events = %d, want 1", got)
	}
	if got := recv.dynamicPairs(); got != 1 {
		t.Fatalf("dynamic registry size = %d, want 1", got)
	}
}

func TestDynamicEngineIDCapRejectsNewPairs(t *testing.T) {
	first := clearV3ReportableFlag(t, buildDynamicV3Trap(t, "testuser", testEngineIDHex, "1.3.6.1.6.3.1.1.5.1"))
	secondEngineID := "80001f888077dfe44faa700259"
	second := clearV3ReportableFlag(t, buildDynamicV3Trap(t, "testuser", secondEngineID, "1.3.6.1.6.3.1.1.5.1"))
	recv, events, peer := newDynamicTestReceiver(t, 1, RateLimitConfig{})
	if result := recv.Process(Datagram{Data: first, PeerIP: peer.IP, Peer: peer}); result.PDU == nil {
		t.Fatalf("first result = %+v, want accepted trap", result)
	}
	if result := recv.Process(Datagram{Data: second, PeerIP: peer.IP, Peer: peer}); result.PDU != nil || result.DecodeFailure != nil {
		t.Fatalf("second result = %+v, want policy drop", result)
	}
	if got := events.count(EventDynamicEngineIDRegistered, "") + events.count(EventError, "unknown_engine_id"); got != 2 {
		t.Fatalf("unknown-engine events = %d, want 2", got)
	}
	if got := recv.dynamicPairs(); got != 1 {
		t.Fatalf("dynamic registry size = %d, want 1", got)
	}
}

func TestDynamicEngineIDSkipsReportableTrap(t *testing.T) {
	data := buildDynamicV3ReportableTrap(t, "testuser", testEngineIDHex, "1.3.6.1.6.3.1.1.5.1")
	recv, _, peer := newDynamicTestReceiver(t, 0, RateLimitConfig{})
	if result := recv.Process(Datagram{Data: data, PeerIP: peer.IP, Peer: peer}); result.DecodeFailure == nil {
		t.Fatalf("result = %+v, want decode failure", result)
	}
	if got := recv.dynamicPairs(); got != 0 {
		t.Fatalf("dynamic registry size = %d, want 0", got)
	}
}

func TestDynamicEngineIDConcurrentDuplicateRegistration(t *testing.T) {
	data := clearV3ReportableFlag(t, buildDynamicV3Trap(t, "testuser", testEngineIDHex, "1.3.6.1.6.3.1.1.5.1"))
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
	if got := recv.dynamicPairs(); got != 1 {
		t.Fatalf("dynamic registry size = %d, want 1", got)
	}
}

func TestDynamicEngineIDDoesNotRegisterInform(t *testing.T) {
	data := clearV3ReportableFlag(t, buildDynamicV3Inform(t, "testuser", testEngineIDHex, "1.3.6.1.6.3.1.1.5.1"))
	recv, _, peer := newDynamicTestReceiver(t, 0, RateLimitConfig{})
	if result := recv.Process(Datagram{Data: data, PeerIP: peer.IP, Peer: peer}); result.PDU != nil || result.DecodeFailure != nil {
		t.Fatalf("result = %+v, want policy drop", result)
	}
	if got := recv.dynamicPairs(); got != 0 {
		t.Fatalf("dynamic registry size = %d, want 0", got)
	}
}

func TestDynamicEngineIDNoStateForUnknownUsername(t *testing.T) {
	data := clearV3ReportableFlag(t, buildDynamicV3Trap(t, "otheruser", testEngineIDHex, "1.3.6.1.6.3.1.1.5.1"))
	recv, _, peer := newDynamicTestReceiver(t, 0, RateLimitConfig{})
	if result := recv.Process(Datagram{Data: data, PeerIP: peer.IP, Peer: peer}); result.DecodeFailure == nil {
		t.Fatalf("result = %+v, want decode failure", result)
	}
	if got := recv.dynamicPairs(); got != 0 {
		t.Fatalf("dynamic registry size = %d, want 0", got)
	}
}

func TestDynamicEngineIDRateLimitDropFollowsAlreadyDecodableRegistration(t *testing.T) {
	data := clearV3ReportableFlag(t, buildDynamicV3Trap(t, "testuser", testEngineIDHex, "1.3.6.1.6.3.1.1.5.1"))
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
	if got := recv.dynamicPairs(); got != 1 {
		t.Fatalf("dynamic registry size = %d, want 1", got)
	}
}

func TestDynamicDecodeFailureReusesRateLimitAdmission(t *testing.T) {
	spec := dynamicV3Spec("testuser", testEngineIDHex, "1.3.6.1.6.3.1.1.5.1")
	spec.authKey = "wrongpassword"
	data := buildV3SecuredTrapWithFlags(t, spec, gosnmp.AuthPriv)
	recv, events, peer := newDynamicTestReceiver(t, 0, RateLimitConfig{Enabled: true, PerSourcePPS: 1, Mode: "drop"})

	result := recv.Process(Datagram{Data: data, PeerIP: peer.IP, Peer: peer})
	if result.DecodeFailure == nil {
		t.Fatalf("result = %+v, want decode failure", result)
	}
	if !recv.AdmitDecodeErrorAudit(result.DecodeFailure) {
		t.Fatal("first admitted dynamic decode failure should be auditable")
	}
	if got := events.count(EventError, "rate_limited"); got != 0 {
		t.Fatalf("rate_limited events = %d, want 0", got)
	}
}

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
	if got := events.count(EventError, "rate_limited"); got != 1 {
		t.Fatalf("rate_limited events before audit = %d, want 1", got)
	}
	if !recv.AdmitDecodeErrorAudit(result.DecodeFailure) {
		t.Fatal("sampled dynamic decode failure should be auditable")
	}
	if got := events.count(EventError, "rate_limited"); got != 1 {
		t.Fatalf("rate_limited events after audit = %d, want 1", got)
	}
}

func TestDiscoveryReportWriteFailureIsReported(t *testing.T) {
	data := buildV3DiscoveryProbe(t, 99)
	listenerConn, peerConn := informUDPConnPair(t)
	defer peerConn.Close()
	peer := peerConn.LocalAddr().(*net.UDPAddr)
	if err := listenerConn.Close(); err != nil {
		t.Fatalf("close response socket: %v", err)
	}

	recv, events, _ := newDynamicTestReceiver(t, 0, RateLimitConfig{Enabled: true, PerSourcePPS: 1, Mode: "drop"})
	result := recv.Process(Datagram{Data: data, PeerIP: peer.IP, Conn: listenerConn, Peer: peer})

	if result.PDU != nil || result.DecodeFailure == nil {
		t.Fatalf("result = %+v, want decode failure after discovery handling", result)
	}
	if got := events.count(EventDiscoveryReportFailed, ""); got != 1 {
		t.Fatalf("discovery Report failure events = %d, want 1", got)
	}
	if !recv.AdmitDecodeErrorAudit(result.DecodeFailure) {
		t.Fatal("first admitted discovery failure should be auditable")
	}
	if got := events.count(EventError, "rate_limited"); got != 0 {
		t.Fatalf("rate_limited events = %d, want 0", got)
	}
}

func TestDynamicEngineIDRateLimitSampleAllowsRetry(t *testing.T) {
	data := clearV3ReportableFlag(t, buildDynamicV3Trap(t, "testuser", testEngineIDHex, "1.3.6.1.6.3.1.1.5.1"))
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
	if got := recv.dynamicPairs(); got != 1 {
		t.Fatalf("dynamic registry size = %d, want 1", got)
	}
}

func TestDiscoveryReportRateLimitDropSkipsResponse(t *testing.T) {
	data := buildV3DiscoveryProbe(t, 99)
	listenerConn, peerConn := informUDPConnPair(t)
	defer listenerConn.Close()
	defer peerConn.Close()

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
	if err := peerConn.SetReadDeadline(time.Now().Add(50 * time.Millisecond)); err != nil {
		t.Fatalf("set read deadline: %v", err)
	}
	buf := make([]byte, maxDatagramSize)
	_, _, err := peerConn.ReadFromUDP(buf)
	if err == nil {
		t.Fatal("expected discovery Report to be rate-limited")
	}
	if ne, ok := err.(net.Error); !ok || !ne.Timeout() {
		t.Fatalf("read response error = %v, want timeout", err)
	}
}

func TestSendDiscoveryReportWireFormat(t *testing.T) {
	paths := newEngineStatePaths(t.TempDir(), "test-discovery-report")

	listenerConn, peerConn := informUDPConnPair(t)
	defer listenerConn.Close()
	defer peerConn.Close()

	lid, err := newLocalEngineID(paths, testLocalEngineIDHex)
	if err != nil {
		t.Fatalf("NewLocalEngineID failed: %v", err)
	}
	eb, err := newEngineBoots(paths)
	if err != nil {
		t.Fatalf("NewEngineBoots failed: %v", err)
	}

	if err := sendDiscoveryReport(listenerConn, peerConn.LocalAddr().(*net.UDPAddr), eb, lid.bytes(), 99); err != nil {
		t.Fatalf("sendDiscoveryReport failed: %v", err)
	}

	respPkt := readInformResponse(t, peerConn)
	if respPkt.PDUType != gosnmp.Report {
		t.Fatalf("response PDU type = %s, want Report", respPkt.PDUType)
	}
	if respPkt.MsgID != 99 {
		t.Fatalf("response msgID = %d, want 99", respPkt.MsgID)
	}
	if len(respPkt.Variables) != 1 || respPkt.Variables[0].Name != ".1.3.6.1.6.3.15.1.1.4.0" {
		t.Fatalf("unexpected Report varbinds: %+v", respPkt.Variables)
	}
	usp, ok := respPkt.SecurityParameters.(*gosnmp.UsmSecurityParameters)
	if !ok {
		t.Fatal("response security parameters are not USM")
	}
	if got := hex.EncodeToString([]byte(usp.AuthoritativeEngineID)); got != testLocalEngineIDHex {
		t.Fatalf("response authoritative engine ID = %q, want %q", got, testLocalEngineIDHex)
	}
	if usp.AuthoritativeEngineBoots == 0 {
		t.Fatal("response engine boots is zero")
	}
}

func buildV3DiscoveryProbe(t *testing.T, msgID uint32) []byte {
	t.Helper()

	pkt := &gosnmp.SnmpPacket{
		Version:            gosnmp.Version3,
		MsgFlags:           gosnmp.Reportable,
		SecurityModel:      gosnmp.UserSecurityModel,
		SecurityParameters: &gosnmp.UsmSecurityParameters{},
		PDUType:            gosnmp.GetRequest,
		MsgID:              msgID,
		RequestID:          42,
		MsgMaxSize:         maxDatagramSize,
	}
	data, err := pkt.MarshalMsg()
	if err != nil {
		t.Fatalf("MarshalMsg failed: %v", err)
	}
	return data
}

func clearV3ReportableFlag(t *testing.T, data []byte) []byte {
	return mutateV3Flags(t, data, func(flags byte) byte { return flags &^ byte(gosnmp.Reportable) })
}

func setV3Flags(t *testing.T, data []byte, flags gosnmp.SnmpV3MsgFlags) []byte {
	return mutateV3Flags(t, data, func(byte) byte { return byte(flags) })
}

func mutateV3Flags(t *testing.T, data []byte, mutate func(byte) byte) []byte {
	t.Helper()

	out := append([]byte(nil), data...)

	tag, valueStart, valueEnd, _, err := readBERElement(out, 0)
	if err != nil {
		t.Fatalf("failed to read outer sequence: %v", err)
	}
	if tag != tagSequence {
		t.Fatalf("outer tag = 0x%x, want sequence", tag)
	}

	_, _, _, pos, err := readBERElement(out[:valueEnd], valueStart)
	if err != nil {
		t.Fatalf("failed to read version: %v", err)
	}

	tag, gdStart, gdEnd, _, err := readBERElement(out[:valueEnd], pos)
	if err != nil {
		t.Fatalf("failed to read v3 header data: %v", err)
	}
	if tag != tagSequence {
		t.Fatalf("header data tag = 0x%x, want sequence", tag)
	}

	gdPos := gdStart
	_, _, _, gdPos, err = readBERElement(out[:gdEnd], gdPos)
	if err != nil {
		t.Fatalf("failed to read msgID: %v", err)
	}
	_, _, _, gdPos, err = readBERElement(out[:gdEnd], gdPos)
	if err != nil {
		t.Fatalf("failed to read msgMaxSize: %v", err)
	}

	tag, flagsStart, flagsEnd, _, err := readBERElement(out[:gdEnd], gdPos)
	if err != nil {
		t.Fatalf("failed to read msgFlags: %v", err)
	}
	if tag != tagOctetStr || flagsEnd-flagsStart != 1 {
		t.Fatalf("msgFlags tag/length = 0x%x/%d, want octet string length 1", tag, flagsEnd-flagsStart)
	}

	out[flagsStart] = mutate(out[flagsStart])
	return out
}

func corruptV3MessageIDTag(t *testing.T, data []byte) []byte {
	return corruptV3HeaderFieldTag(t, data, 0, tagOctetStr)
}

func corruptV3HeaderFieldTag(t *testing.T, data []byte, field int, replacement byte) []byte {
	t.Helper()
	out := append([]byte(nil), data...)
	tag, valueStart, valueEnd, _, err := readBERElement(out, 0)
	if err != nil || tag != tagSequence {
		t.Fatalf("parse outer sequence: tag=%#x err=%v", tag, err)
	}
	_, _, _, next, err := readBERElement(out[:valueEnd], valueStart)
	if err != nil {
		t.Fatalf("parse version: %v", err)
	}
	tag, headerStart, headerEnd, _, err := readBERElement(out[:valueEnd], next)
	if err != nil || tag != tagSequence {
		t.Fatalf("parse v3 header: tag=%#x err=%v", tag, err)
	}
	pos := headerStart
	for range field {
		_, _, _, pos, err = readBERElement(out[:headerEnd], pos)
		if err != nil {
			t.Fatalf("parse v3 header field: %v", err)
		}
	}
	out[pos] = replacement
	return out
}

func v3OuterFields(t *testing.T, data []byte) [][]byte {
	t.Helper()
	tag, valueStart, valueEnd, next, err := readBERElement(data, 0)
	if err != nil || tag != tagSequence || next != len(data) {
		t.Fatalf("parse outer sequence: tag=%#x next=%d/%d err=%v", tag, next, len(data), err)
	}

	var fields [][]byte
	for pos := valueStart; pos < valueEnd; {
		_, _, _, next, err := readBERElement(data[:valueEnd], pos)
		if err != nil {
			t.Fatalf("parse outer field %d: %v", len(fields), err)
		}
		fields = append(fields, append([]byte(nil), data[pos:next]...))
		pos = next
	}
	return fields
}

func replaceV3MsgDataTag(t *testing.T, data []byte, replacement byte) []byte {
	t.Helper()
	fields := v3OuterFields(t, data)
	if len(fields) != 4 {
		t.Fatalf("outer field count = %d, want 4", len(fields))
	}
	msgData := append([]byte(nil), fields[3]...)
	msgData[0] = replacement
	return replaceV3MsgData(t, data, msgData)
}

func replaceV3MsgData(t *testing.T, data, msgData []byte) []byte {
	t.Helper()
	fields := v3OuterFields(t, data)
	if len(fields) != 4 {
		t.Fatalf("outer field count = %d, want 4", len(fields))
	}
	fields[3] = append([]byte(nil), msgData...)
	return berTLV(tagSequence, testBERFields(fields...))
}

func v3PlaintextScopedPDUFields(t *testing.T, data []byte) [][]byte {
	t.Helper()
	outerFields := v3OuterFields(t, data)
	if len(outerFields) != 4 {
		t.Fatalf("outer field count = %d, want 4", len(outerFields))
	}
	msgData := outerFields[3]
	tag, valueStart, valueEnd, next, err := readBERElement(msgData, 0)
	if err != nil || tag != tagSequence || next != len(msgData) {
		t.Fatalf("parse plaintext scoped PDU: tag=%#x next=%d/%d err=%v", tag, next, len(msgData), err)
	}

	var fields [][]byte
	for pos := valueStart; pos < valueEnd; {
		_, _, _, next, err := readBERElement(msgData[:valueEnd], pos)
		if err != nil {
			t.Fatalf("parse scoped PDU field %d: %v", len(fields), err)
		}
		fields = append(fields, append([]byte(nil), msgData[pos:next]...))
		pos = next
	}
	return fields
}

func testBERElement(tag byte, value []byte) []byte {
	return append([]byte{tag, byte(len(value))}, value...)
}

func testBERFields(fields ...[]byte) []byte {
	var data []byte
	for _, field := range fields {
		data = append(data, field...)
	}
	return data
}

func testBERSequence(fields ...[]byte) []byte {
	return testBERElement(tagSequence, testBERFields(fields...))
}
