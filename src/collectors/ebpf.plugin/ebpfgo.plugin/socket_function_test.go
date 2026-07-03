// SPDX-License-Identifier: GPL-3.0-or-later

package main

import (
	"encoding/json"
	"testing"

	"github.com/netdata/netdata/src/collectors/ebpf.plugin/ebpfgo.plugin/libbpfloader"
)

// TestBuildNetworkProtocolsJSON_BPFMapping pins the BPF counter -> Received/Sent
// dimension mapping. If a kernel ABI rename silently flips the assignment in
// Update() or the read in buildNetworkProtocolsJSON(), this test fails.
//
// Convention (single source of truth: socketGlobalPublish doc comment):
//   - tcp_cleanup_rbuf data feeds the "Received" dimension
//   - tcp_sendmsg data feeds the "Sent" dimension
func TestBuildNetworkProtocolsJSON_BPFMapping(t *testing.T) {
	// Distinct, non-zero values for each BPF counter so a swap is detectable.
	p := socketGlobalPublish{
		tcpDimReceivedCalls: 111,
		tcpDimReceivedKbits: 222,
		tcpDimReceivedErr:   333,
		tcpDimSentCalls:     444,
		tcpDimSentKbits:     555,
		tcpDimSentErr:       666,
		tcpCloseCalls:       777,
		tcpRetransmit:       888,
		tcpV4Conn:           10,
		tcpV6Conn:           20,
		udpRecvCalls:        1111,
		udpSendCalls:        2222,
		udpRecvKbits:        3333,
		udpSendKbits:        4444,
		udpRecvErr:          5555,
		udpSendErr:          6666,
		inboundTCP:          7777,
		inboundUDP:          8888,
	}

	const updateEvery = 5
	expires := int64(1_700_000_000)

	payload, err := buildNetworkProtocolsJSON(p, updateEvery, expires)
	if err != nil {
		t.Fatalf("buildNetworkProtocolsJSON: %v", err)
	}

	var resp fnTableResponse
	if err := json.Unmarshal([]byte(payload), &resp); err != nil {
		t.Fatalf("json.Unmarshal: %v\npayload: %s", err, payload)
	}

	if len(resp.Data) != 2 {
		t.Fatalf("Data len = %d, want 2 (TCP + UDP)", len(resp.Data))
	}

	tcpRow, ok := resp.Data[0].([]interface{})
	if !ok || len(tcpRow) < 11 {
		t.Fatalf("TCP row malformed: %+v", resp.Data[0])
	}

	// Column order (see socket_function.go):
	//   0 Transport, 1 Family, 2 Received, 3 Sent, 4 Errors,
	//   5 ConnActive, 6 ConnEstablished, 7 ConnPassive, 8 ConnReset,
	//   9 SegsTotal, 10 SegsRetransmitted
	got := func(idx int) uint64 {
		v, _ := tcpRow[idx].(float64)
		return uint64(v)
	}

	if got := got(2); got != p.tcpDimReceivedCalls {
		t.Errorf("TCP Received = %d, want %d (tcpDimReceivedCalls)", got, p.tcpDimReceivedCalls)
	}
	if got := got(3); got != p.tcpDimSentCalls {
		t.Errorf("TCP Sent = %d, want %d (tcpDimSentCalls)", got, p.tcpDimSentCalls)
	}
	if got := got(4); got != p.tcpDimReceivedErr+p.tcpDimSentErr {
		t.Errorf("TCP Errors = %d, want %d (sum of Received/Sent Err)", got, p.tcpDimReceivedErr+p.tcpDimSentErr)
	}
	if got := got(5); got != p.tcpV4Conn+p.tcpV6Conn {
		t.Errorf("TCP ConnActive = %d, want %d (V4 + V6)", got, p.tcpV4Conn+p.tcpV6Conn)
	}
	if got := got(7); got != p.inboundTCP {
		t.Errorf("TCP ConnPassive = %d, want %d (inboundTCP)", got, p.inboundTCP)
	}
	if got := got(9); got != p.tcpDimReceivedCalls+p.tcpDimSentCalls+p.tcpCloseCalls {
		t.Errorf("TCP SegsTotal = %d, want %d (Received + Sent + Close)", got, p.tcpDimReceivedCalls+p.tcpDimSentCalls+p.tcpCloseCalls)
	}
	if got := got(10); got != p.tcpRetransmit {
		t.Errorf("TCP SegsRetransmitted = %d, want %d (tcpRetransmit)", got, p.tcpRetransmit)
	}

	udpRow, ok := resp.Data[1].([]interface{})
	if !ok || len(udpRow) < 8 {
		t.Fatalf("UDP row malformed: %+v", resp.Data[1])
	}
	if got := mustUint64(udpRow[2]); got != p.udpRecvCalls {
		t.Errorf("UDP Received = %d, want %d (udpRecvCalls)", got, p.udpRecvCalls)
	}
	if got := mustUint64(udpRow[3]); got != p.udpSendCalls {
		t.Errorf("UDP Sent = %d, want %d (udpSendCalls)", got, p.udpSendCalls)
	}
	if got := mustUint64(udpRow[7]); got != p.inboundUDP {
		t.Errorf("UDP ConnPassive = %d, want %d (inboundUDP)", got, p.inboundUDP)
	}
}

func mustUint64(v interface{}) uint64 {
	f, _ := v.(float64)
	return uint64(f)
}

// TestSocketGlobalStateUpdate_BPFMapping pins the BPF counter -> semantic
// direction at the producer side. If a kernel ABI rename (e.g.,
// CallsTCPSendmsg actually meaning "received") silently flips an assignment,
// this test fails.
func TestSocketGlobalStateUpdate_BPFMapping(t *testing.T) {
	prev := libbpfloaderSocketSnapshotZero()
	cur := prev
	// Received path: cleanup_rbuf counters advance.
	cur.CallsTCPCleanupRbuf = 100
	cur.BytesTCPCleanupRbuf = 1000
	cur.ErrorTCPCleanupRbuf = 5
	// Sent path: sendmsg counters advance.
	cur.CallsTCPSendmsg = 200
	cur.BytesTCPSendmsg = 2000
	cur.ErrorTCPSendmsg = 7
	// Other counters advance so they don't fall through the first-read gate.
	cur.CallsTCPClose = 1
	cur.TCPRetransmit = 2
	cur.CallsTCPConnectIPv4 = 3
	cur.CallsTCPConnectIPv6 = 4
	cur.CallsUDPRecvmsg = 5
	cur.CallsUDPSendmsg = 6
	cur.BytesUDPRecvmsg = 7000
	cur.BytesUDPSendmsg = 8000
	cur.ErrorUDPRecvmsg = 9
	cur.ErrorUDPSendmsg = 10
	cur.InboundConnTCP = 11
	cur.InboundConnUDP = 12

	state := &socketGlobalState{initialized: true, prev: prev}
	got, ok := state.Update(cur)
	if !ok {
		t.Fatal("Update returned ok=false on a real advance")
	}

	if got.tcpDimReceivedCalls != 100 {
		t.Errorf("tcpDimReceivedCalls = %d, want 100 (CallsTCPCleanupRbuf delta)", got.tcpDimReceivedCalls)
	}
	if got.tcpDimReceivedKbits <= 0 {
		t.Errorf("tcpDimReceivedKbits = %d, must be positive (no negative-sign trick)", got.tcpDimReceivedKbits)
	}
	if got.tcpDimReceivedErr != 5 {
		t.Errorf("tcpDimReceivedErr = %d, want 5", got.tcpDimReceivedErr)
	}
	if got.tcpDimSentCalls != 200 {
		t.Errorf("tcpDimSentCalls = %d, want 200 (CallsTCPSendmsg delta)", got.tcpDimSentCalls)
	}
	if got.tcpDimSentKbits <= 0 {
		t.Errorf("tcpDimSentKbits = %d, must be positive", got.tcpDimSentKbits)
	}
	if got.tcpDimSentErr != 7 {
		t.Errorf("tcpDimSentErr = %d, want 7", got.tcpDimSentErr)
	}
	if got.udpRecvKbits <= 0 {
		t.Errorf("udpRecvKbits = %d, must be positive", got.udpRecvKbits)
	}
	if got.udpSendKbits <= 0 {
		t.Errorf("udpSendKbits = %d, must be positive", got.udpSendKbits)
	}
}

func libbpfloaderSocketSnapshotZero() (s libbpfloader.SocketSnapshot) { return }