// SPDX-License-Identifier: GPL-3.0-or-later

package receiver

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

func TestReceiverRespondsBeforeRateLimitDrop(t *testing.T) {
	reqData := buildV2cPDU(t, gosnmp.InformRequest, "public", "1.3.6.1.6.3.1.1.5.1")

	listenerConn, peerConn := informUDPConnPair(t)
	defer listenerConn.Close()
	defer peerConn.Close()

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

	respPkt := readInformResponse(t, peerConn)
	if respPkt.PDUType != gosnmp.GetResponse {
		t.Fatalf("response PDU type = %s, want GetResponse", respPkt.PDUType)
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

	listenerConn, peerConn := informUDPConnPair(t)
	defer peerConn.Close()
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

func countEvents(events []Event, eventType EventType, errorKind ErrorKind) int {
	count := 0
	for _, event := range events {
		if event.Type == eventType && (errorKind == "" || event.ErrorKind == errorKind) {
			count++
		}
	}
	return count
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
