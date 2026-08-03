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
	pkt := &gosnmp.SnmpPacket{
		Version:            gosnmp.Version3,
		MsgFlags:           gosnmp.Reportable,
		SecurityModel:      gosnmp.UserSecurityModel,
		SecurityParameters: &gosnmp.UsmSecurityParameters{},
		PDUType:            gosnmp.GetRequest,
		MsgID:              99,
		RequestID:          42,
		MsgMaxSize:         maxDatagramSize,
	}
	data, err := pkt.MarshalMsg()
	if err != nil {
		t.Fatalf("MarshalMsg failed: %v", err)
	}

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
		if result := recv.Process(Datagram{Data: data, PeerIP: peer.IP, Peer: peer}); result.Context == nil {
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
	if result := recv.Process(Datagram{Data: first, PeerIP: peer.IP, Peer: peer}); result.Context == nil {
		t.Fatalf("first result = %+v, want accepted trap", result)
	}
	if result := recv.Process(Datagram{Data: second, PeerIP: peer.IP, Peer: peer}); result.Context != nil || result.DecodeFailure != nil {
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
			if result := recv.Process(Datagram{Data: data, PeerIP: peer.IP, Peer: peer}); result.Context != nil {
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
	if result := recv.Process(Datagram{Data: data, PeerIP: peer.IP, Peer: peer}); result.Context != nil || result.DecodeFailure != nil {
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
	srcAddr, ok := udpPeerAddr(peer)
	if !ok {
		t.Fatal("failed to convert UDP peer address")
	}
	if allowed, _ := recv.rateLimiter.Allow(srcAddr); !allowed {
		t.Fatal("expected first token to be available")
	}

	if result := recv.Process(Datagram{Data: data, PeerIP: peer.IP, Peer: peer}); result.Context != nil || result.DecodeFailure != nil {
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

func TestDynamicEngineIDRateLimitSampleAllowsRetry(t *testing.T) {
	data := clearV3ReportableFlag(t, buildDynamicV3Trap(t, "testuser", testEngineIDHex, "1.3.6.1.6.3.1.1.5.1"))
	recv, events, peer := newDynamicTestReceiver(t, 0, RateLimitConfig{Enabled: true, PerSourcePPS: 1, Mode: "sample"})
	srcAddr, ok := udpPeerAddr(peer)
	if !ok {
		t.Fatal("failed to convert UDP peer address")
	}
	if allowed, _ := recv.rateLimiter.Allow(srcAddr); !allowed {
		t.Fatal("expected first token to be available")
	}

	if result := recv.Process(Datagram{Data: data, PeerIP: peer.IP, Peer: peer}); result.Context == nil {
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
	srcAddr, ok := udpPeerAddr(peer)
	if !ok {
		t.Fatal("failed to convert UDP peer address")
	}
	if allowed, _ := recv.rateLimiter.Allow(srcAddr); !allowed {
		t.Fatal("expected first token to be available")
	}

	if result := recv.Process(Datagram{Data: data, PeerIP: peer.IP, Conn: listenerConn, Peer: peer}); result.Context != nil || result.DecodeFailure != nil {
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

	if err := sendDiscoveryReport(listenerConn, peerConn.LocalAddr().(*net.UDPAddr), eb, lid.Bytes(), 99); err != nil {
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

	out[flagsStart] &^= 0x04
	return out
}

func corruptV3MessageIDTag(t *testing.T, data []byte) []byte {
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
	tag, headerStart, _, _, err := readBERElement(out[:valueEnd], next)
	if err != nil || tag != tagSequence {
		t.Fatalf("parse v3 header: tag=%#x err=%v", tag, err)
	}
	out[headerStart] = tagOctetStr
	return out
}
