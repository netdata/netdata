// SPDX-License-Identifier: GPL-3.0-or-later

package snmp_traps

import (
	"encoding/hex"
	"net"
	"sync"
	"sync/atomic"
	"testing"

	"github.com/gosnmp/gosnmp"
)

func TestExtractRawV3ContextTrap(t *testing.T) {
	data := clearV3ReportableFlag(t, buildV3TrapWithEngineID(t, "testuser", testEngineIDHex, "1.3.6.1.6.3.1.1.5.1"))

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

func TestDynamicEngineIDTrapRegistration(t *testing.T) {
	const jobName = "test-dynamic-engine"

	data := clearV3ReportableFlag(t, buildV3TrapWithEngineID(t, "testuser", testEngineIDHex, "1.3.6.1.6.3.1.1.5.1"))
	c, writer, peer := newDynamicEngineIDTestCollector(t, jobName, data, nil)
	c.handlePacket(data, peer.IP, nil, peer)
	c.handlePacket(data, peer.IP, nil, peer)

	if got := len(writer.entries); got != 2 {
		t.Fatalf("journaled entries = %d, want 2", got)
	}
	m := getJobMetrics(jobName)
	if v := atomic.LoadUint64(&m.errors.unknownEngineID); v != 1 {
		t.Fatalf("unknown_engine_id = %d, want 1", v)
	}
	if got := c.dynamicEngineIDReg.size(); got != 1 {
		t.Fatalf("dynamic registry size = %d, want 1", got)
	}
}

func TestDynamicEngineIDCapRejectsNewPairs(t *testing.T) {
	const jobName = "test-dynamic-engine-cap"

	first := clearV3ReportableFlag(t, buildV3TrapWithEngineID(t, "testuser", testEngineIDHex, "1.3.6.1.6.3.1.1.5.1"))
	secondEngineID := "80001f888077dfe44faa700259"
	second := clearV3ReportableFlag(t, buildV3TrapWithEngineID(t, "testuser", secondEngineID, "1.3.6.1.6.3.1.1.5.1"))
	c, writer, peer := newDynamicEngineIDTestCollector(t, jobName, first, func(c *Collector) {
		c.dynamicEngineIDMax = 1
	})

	c.handlePacket(first, peer.IP, nil, peer)
	c.handlePacket(second, peer.IP, nil, peer)

	if got := len(writer.entries); got != 1 {
		t.Fatalf("journaled entries = %d, want 1", got)
	}
	m := getJobMetrics(jobName)
	if v := atomic.LoadUint64(&m.errors.unknownEngineID); v != 2 {
		t.Fatalf("unknown_engine_id = %d, want 2", v)
	}
	if got := c.dynamicEngineIDReg.size(); got != 1 {
		t.Fatalf("dynamic registry size = %d, want 1", got)
	}
}

func TestDynamicEngineIDSkipsReportableTrap(t *testing.T) {
	const jobName = "test-dynamic-engine-reportable"

	data := buildV3TrapWithEngineID(t, "testuser", testEngineIDHex, "1.3.6.1.6.3.1.1.5.1")
	c, writer, peer := newDynamicEngineIDTestCollector(t, jobName, data, nil)
	c.handlePacket(data, peer.IP, nil, peer)

	if got := len(writer.entries); got != 0 {
		t.Fatalf("journaled entries = %d, want 0", got)
	}
	if got := c.dynamicEngineIDReg.size(); got != 0 {
		t.Fatalf("dynamic registry size = %d, want 0", got)
	}
}

func TestDynamicEngineIDConcurrentDuplicateRegistration(t *testing.T) {
	const jobName = "test-dynamic-engine-concurrent"

	data := clearV3ReportableFlag(t, buildV3TrapWithEngineID(t, "testuser", testEngineIDHex, "1.3.6.1.6.3.1.1.5.1"))
	c, writer, peer := newDynamicEngineIDTestCollector(t, jobName, data, nil)
	var wg sync.WaitGroup
	for range 16 {
		wg.Go(func() {
			c.handlePacket(data, peer.IP, nil, peer)
		})
	}
	wg.Wait()

	writer.mu.Lock()
	gotEntries := len(writer.entries)
	writer.mu.Unlock()
	if gotEntries != 16 {
		t.Fatalf("journaled entries = %d, want 16", gotEntries)
	}
	m := getJobMetrics(jobName)
	if v := atomic.LoadUint64(&m.errors.unknownEngineID); v != 1 {
		t.Fatalf("unknown_engine_id = %d, want 1", v)
	}
	if got := c.dynamicEngineIDReg.size(); got != 1 {
		t.Fatalf("dynamic registry size = %d, want 1", got)
	}
}

func TestDynamicEngineIDDoesNotRegisterInformRetry(t *testing.T) {
	const jobName = "test-dynamic-engine-inform"

	data := clearV3ReportableFlag(t, buildV3InformWithEngineID(t, "testuser", testEngineIDHex, "1.3.6.1.6.3.1.1.5.1"))
	c, writer, peer := newDynamicEngineIDTestCollector(t, jobName, data, nil)
	c.handlePacket(data, peer.IP, nil, peer)

	if got := len(writer.entries); got != 0 {
		t.Fatalf("journaled entries = %d, want 0", got)
	}
	if got := c.dynamicEngineIDReg.size(); got != 0 {
		t.Fatalf("dynamic registry size = %d, want 0", got)
	}
}

func TestDynamicEngineIDNoStateForUnknownUsername(t *testing.T) {
	const jobName = "test-dynamic-engine-unknown-user"

	data := clearV3ReportableFlag(t, buildV3TrapWithEngineID(t, "otheruser", testEngineIDHex, "1.3.6.1.6.3.1.1.5.1"))
	c, writer, peer := newDynamicEngineIDTestCollector(t, jobName, data, nil)
	c.handlePacket(data, peer.IP, nil, peer)

	if got := len(writer.entries); got != 0 {
		t.Fatalf("journaled entries = %d, want 0", got)
	}
	if got := c.dynamicEngineIDReg.size(); got != 0 {
		t.Fatalf("dynamic registry size = %d, want 0", got)
	}
}

func TestDynamicEngineIDRateLimitDropSkipsRetry(t *testing.T) {
	const jobName = "test-dynamic-engine-rate-drop"

	data := clearV3ReportableFlag(t, buildV3TrapWithEngineID(t, "testuser", testEngineIDHex, "1.3.6.1.6.3.1.1.5.1"))
	rl := newRateLimiter(true, 1, "drop")
	c, writer, peer := newDynamicEngineIDTestCollector(t, jobName, data, func(c *Collector) {
		c.rateLimiter = rl
	})
	srcAddr, ok := udpPeerAddr(peer)
	if !ok {
		t.Fatal("failed to convert UDP peer address")
	}
	if allowed, _ := rl.Allow(srcAddr); !allowed {
		t.Fatal("expected first token to be available")
	}

	c.handlePacket(data, peer.IP, nil, peer)

	if got := len(writer.entries); got != 0 {
		t.Fatalf("journaled entries = %d, want 0", got)
	}
	m := getJobMetrics(jobName)
	if v := atomic.LoadUint64(&m.errors.rateLimited); v != 1 {
		t.Fatalf("rate_limited = %d, want 1", v)
	}
	if got := c.dynamicEngineIDReg.size(); got != 0 {
		t.Fatalf("dynamic registry size = %d, want 0", got)
	}
}

func TestDynamicEngineIDRateLimitSampleAllowsRetry(t *testing.T) {
	const jobName = "test-dynamic-engine-rate-sample"

	data := clearV3ReportableFlag(t, buildV3TrapWithEngineID(t, "testuser", testEngineIDHex, "1.3.6.1.6.3.1.1.5.1"))
	rl := newRateLimiter(true, 1, "sample")
	c, writer, peer := newDynamicEngineIDTestCollector(t, jobName, data, func(c *Collector) {
		c.rateLimiter = rl
	})
	srcAddr, ok := udpPeerAddr(peer)
	if !ok {
		t.Fatal("failed to convert UDP peer address")
	}
	if allowed, _ := rl.Allow(srcAddr); !allowed {
		t.Fatal("expected first token to be available")
	}

	c.handlePacket(data, peer.IP, nil, peer)

	if got := len(writer.entries); got != 1 {
		t.Fatalf("journaled entries = %d, want 1", got)
	}
	m := getJobMetrics(jobName)
	if v := atomic.LoadUint64(&m.errors.rateLimited); v != 1 {
		t.Fatalf("rate_limited = %d, want 1", v)
	}
	if v := atomic.LoadUint64(&m.errors.unknownEngineID); v != 1 {
		t.Fatalf("unknown_engine_id = %d, want 1", v)
	}
	if got := c.dynamicEngineIDReg.size(); got != 1 {
		t.Fatalf("dynamic registry size = %d, want 1", got)
	}
}

func TestSendDiscoveryReportWireFormat(t *testing.T) {
	withEngineStateDir(t)

	listenerConn, peerConn := informUDPConnPair(t)
	defer listenerConn.Close()
	defer peerConn.Close()

	lid, err := NewLocalEngineID("test-discovery-report", testLocalEngineIDHex)
	if err != nil {
		t.Fatalf("NewLocalEngineID failed: %v", err)
	}
	eb, err := NewEngineBoots("test-discovery-report")
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
