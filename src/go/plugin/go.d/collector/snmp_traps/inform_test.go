// SPDX-License-Identifier: GPL-3.0-or-later

package snmp_traps

import (
	"net"
	"reflect"
	"testing"
	"time"

	"github.com/gosnmp/gosnmp"
)

func TestSendInformResponseV2c(t *testing.T) {
	reqData := buildV2cPDU(t, gosnmp.InformRequest, "public", "1.3.6.1.6.3.1.1.5.1",
		gosnmp.SnmpPDU{Name: "1.3.6.1.2.1.2.2.1.1.7", Type: gosnmp.Integer, Value: 7},
		gosnmp.SnmpPDU{Name: "1.3.6.1.2.1.31.1.1.1.1.7", Type: gosnmp.OctetString, Value: "Gi0/7"},
	)
	reqPkt, err := decodePacket(reqData, nil)
	if err != nil {
		t.Fatalf("decode request packet: %v", err)
	}

	listenerConn, peerConn := informUDPConnPair(t)
	defer listenerConn.Close()
	defer peerConn.Close()

	if err := sendInformResponse(listenerConn, peerConn.LocalAddr().(*net.UDPAddr), reqPkt, nil, nil); err != nil {
		t.Fatalf("sendInformResponse failed: %v", err)
	}

	respData := readInformResponseBytes(t, peerConn)
	if len(respData) > len(reqData) {
		t.Fatalf("response length = %d, want <= request length %d", len(respData), len(reqData))
	}
	respPkt := decodeInformResponse(t, respData)
	if respPkt.PDUType != gosnmp.GetResponse {
		t.Fatalf("response PDU type = %s, want GetResponse", respPkt.PDUType)
	}
	if respPkt.RequestID != reqPkt.RequestID {
		t.Fatalf("response request ID = %d, want %d", respPkt.RequestID, reqPkt.RequestID)
	}
	if respPkt.Community != reqPkt.Community {
		t.Fatalf("response community = %q, want %q", respPkt.Community, reqPkt.Community)
	}
	if !reflect.DeepEqual(respPkt.Variables, reqPkt.Variables) {
		t.Fatalf("response varbinds = %#v, want %#v", respPkt.Variables, reqPkt.Variables)
	}
}

func TestCollectorHandlePacketRespondsBeforeRateLimitDrop(t *testing.T) {
	reqData := buildV2cPDU(t, gosnmp.InformRequest, "public", "1.3.6.1.6.3.1.1.5.1")

	listenerConn, peerConn := informUDPConnPair(t)
	defer listenerConn.Close()
	defer peerConn.Close()

	peer := peerConn.LocalAddr().(*net.UDPAddr)
	rl := newRateLimiter(true, 1, "drop")
	srcAddr, ok := udpPeerAddr(peer)
	if !ok {
		t.Fatal("failed to convert UDP peer address")
	}
	if allowed, _ := rl.Allow(srcAddr); !allowed {
		t.Fatal("expected first token to be available")
	}

	const jobName = "test-inform-rate-limit"
	removeJobMetrics(jobName)
	defer removeJobMetrics(jobName)

	writer := &mockTrapWriter{}
	c := &Collector{
		jobName:     jobName,
		trapWriter:  writer,
		versions:    map[SnmpVersion]struct{}{SnmpVersionV2c: {}},
		allowlist:   NewAllowlist(nil, []string{"public"}),
		rateLimiter: rl,
	}

	c.handlePacket(reqData, peer.IP, listenerConn, peer)

	respPkt := readInformResponse(t, peerConn)
	if respPkt.PDUType != gosnmp.GetResponse {
		t.Fatalf("response PDU type = %s, want GetResponse", respPkt.PDUType)
	}
	if len(writer.entries) != 0 {
		t.Fatalf("rate-limited INFORM should not be journaled, got %d entries", len(writer.entries))
	}

	m := getJobMetrics(jobName)
	if v := m.errors.rateLimited.Load(); v != 1 {
		t.Fatalf("rate_limited = %d, want 1", v)
	}
}

func TestCollectorHandlePacketIncrementsInformResponseFailed(t *testing.T) {
	reqData := buildV2cPDU(t, gosnmp.InformRequest, "public", "1.3.6.1.6.3.1.1.5.1")

	listenerConn, peerConn := informUDPConnPair(t)
	defer peerConn.Close()
	peer := peerConn.LocalAddr().(*net.UDPAddr)
	if err := listenerConn.Close(); err != nil {
		t.Fatalf("close listener socket: %v", err)
	}

	const jobName = "test-inform-response-failed"
	removeJobMetrics(jobName)
	defer removeJobMetrics(jobName)

	writer := &mockTrapWriter{}
	c := &Collector{
		jobName:    jobName,
		trapWriter: writer,
		versions:   map[SnmpVersion]struct{}{SnmpVersionV2c: {}},
		allowlist:  NewAllowlist(nil, []string{"public"}),
	}

	c.handlePacket(reqData, peer.IP, listenerConn, peer)

	m := getJobMetrics(jobName)
	if v := m.errors.informResponseFail.Load(); v != 1 {
		t.Fatalf("inform_response_failed = %d, want 1", v)
	}
}

func informUDPConnPair(t *testing.T) (*net.UDPConn, *net.UDPConn) {
	t.Helper()

	addr := &net.UDPAddr{IP: net.ParseIP("127.0.0.1"), Port: 0}
	listenerConn, err := net.ListenUDP("udp4", addr)
	if err != nil {
		t.Fatalf("listen response socket: %v", err)
	}
	peerConn, err := net.ListenUDP("udp4", addr)
	if err != nil {
		listenerConn.Close()
		t.Fatalf("listen peer socket: %v", err)
	}
	return listenerConn, peerConn
}

func readInformResponse(t *testing.T, peerConn *net.UDPConn) *gosnmp.SnmpPacket {
	t.Helper()

	return decodeInformResponse(t, readInformResponseBytes(t, peerConn))
}

func decodeInformResponse(t *testing.T, buf []byte) *gosnmp.SnmpPacket {
	t.Helper()

	decoder := &gosnmp.GoSNMP{Logger: trapDecodeLogger}
	respPkt, err := decoder.SnmpDecodePacket(buf)
	if err != nil {
		t.Fatalf("decode response: %v", err)
	}
	return respPkt
}

func readInformResponseWithDecoder(t *testing.T, peerConn *net.UDPConn, decoder *gosnmp.GoSNMP) *gosnmp.SnmpPacket {
	t.Helper()

	buf := readInformResponseBytes(t, peerConn)
	respPkt, err := decoder.UnmarshalTrap(buf, false)
	if err != nil {
		t.Fatalf("decode response: %v", err)
	}
	return respPkt
}

func readInformResponseBytes(t *testing.T, peerConn *net.UDPConn) []byte {
	t.Helper()

	if err := peerConn.SetReadDeadline(time.Now().Add(time.Second)); err != nil {
		t.Fatalf("set read deadline: %v", err)
	}
	buf := make([]byte, maxDatagramSize)
	n, _, err := peerConn.ReadFromUDP(buf)
	if err != nil {
		t.Fatalf("read response: %v", err)
	}
	return buf[:n]
}
