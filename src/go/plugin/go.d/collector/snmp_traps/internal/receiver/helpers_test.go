// SPDX-License-Identifier: GPL-3.0-or-later

package receiver

import (
	"net"
	"sync"
	"testing"
	"time"

	"github.com/netdata/gosnmp"
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

func countEvents(events []Event, eventType EventType, errorKind ErrorKind) int {
	count := 0
	for _, event := range events {
		if event.Type == eventType && (errorKind == "" || event.ErrorKind == errorKind) {
			count++
		}
	}
	return count
}

// udpConnPair returns the receiver's socket and a peer's, closed at the end of
// the test.
func udpConnPair(t *testing.T) (*net.UDPConn, *net.UDPConn) {
	t.Helper()

	addr := &net.UDPAddr{IP: net.ParseIP("127.0.0.1"), Port: 0}
	listenerConn, err := net.ListenUDP("udp4", addr)
	if err != nil {
		t.Fatalf("listen response socket: %v", err)
	}
	t.Cleanup(func() { _ = listenerConn.Close() })
	peerConn, err := net.ListenUDP("udp4", addr)
	if err != nil {
		t.Fatalf("listen peer socket: %v", err)
	}
	t.Cleanup(func() { _ = peerConn.Close() })
	return listenerConn, peerConn
}

// readResponse returns the next datagram peerConn receives within a second.
func readResponse(t *testing.T, peerConn *net.UDPConn) []byte {
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

// requireNoResponse checks that peerConn receives nothing for a while.
func requireNoResponse(t *testing.T, peerConn *net.UDPConn) {
	t.Helper()

	if err := peerConn.SetReadDeadline(time.Now().Add(100 * time.Millisecond)); err != nil {
		t.Fatalf("set read deadline: %v", err)
	}
	_, _, err := peerConn.ReadFromUDP(make([]byte, maxDatagramSize))
	if err == nil {
		t.Fatal("received a response, want none")
	}
	if ne, ok := err.(net.Error); !ok || !ne.Timeout() {
		t.Fatalf("read response error = %v, want timeout", err)
	}
}

// buildV3DiscoveryProbe builds the unauthenticated, reportable GetRequest a
// manager sends to learn an agent's engine ID: no engine ID, no user name.
func buildV3DiscoveryProbe(t *testing.T, msgID uint32) []byte {
	t.Helper()

	data, err := (&gosnmp.Message{
		Version:       gosnmp.Version3,
		SecurityLevel: gosnmp.NoAuthNoPriv,
		PDUType:       gosnmp.GetRequest,
		MsgID:         msgID,
		RequestID:     42,
		MsgMaxSize:    maxDatagramSize,
	}).Encode(nil)
	if err != nil {
		t.Fatalf("encode discovery probe: %v", err)
	}
	return data
}

// berElement is an encoded element of a datagram: the offsets of its tag, its
// content and its end.
type berElement struct {
	start, content, end int
}

func berElementAt(t *testing.T, data []byte, off int) berElement {
	t.Helper()
	_, content, consumed, err := readTLV(data[off:])
	if err != nil {
		t.Fatalf("parse BER element at %d: %v", off, err)
	}
	return berElement{start: off, content: off + consumed - len(content), end: off + consumed}
}

func berChildren(t *testing.T, data []byte, parent berElement) []berElement {
	t.Helper()
	var children []berElement
	for pos := parent.content; pos < parent.end; {
		child := berElementAt(t, data[:parent.end], pos)
		children = append(children, child)
		pos = child.end
	}
	return children
}

// v3Outer returns the elements of an SNMPv3 message: msgVersion,
// msgGlobalData, msgSecurityParameters and msgData.
func v3Outer(t *testing.T, data []byte) []berElement {
	t.Helper()
	fields := berChildren(t, data, berElementAt(t, data, 0))
	if len(fields) != 4 {
		t.Fatalf("SNMPv3 message fields = %d, want 4", len(fields))
	}
	return fields
}

// v3GlobalData returns the elements of an SNMPv3 msgGlobalData: msgID,
// msgMaxSize, msgFlags and msgSecurityModel.
func v3GlobalData(t *testing.T, data []byte) []berElement {
	t.Helper()
	fields := berChildren(t, data, v3Outer(t, data)[1])
	if len(fields) != 4 {
		t.Fatalf("SNMPv3 msgGlobalData fields = %d, want 4", len(fields))
	}
	return fields
}

// setV3HeaderOctet rewrites the one-octet content of msgGlobalData field i.
func setV3HeaderOctet(t *testing.T, data []byte, i int, mutate func(byte) byte) []byte {
	t.Helper()
	out := append([]byte(nil), data...)
	field := v3GlobalData(t, out)[i]
	if field.end-field.content != 1 {
		t.Fatalf("msgGlobalData field %d length = %d, want 1", i, field.end-field.content)
	}
	out[field.content] = mutate(out[field.content])
	return out
}

// RFC 3412 section 6.4: the msgFlags reportableFlag.
const testReportableFlag = 0x04

func setV3Flags(t *testing.T, data []byte, flags byte) []byte {
	return setV3HeaderOctet(t, data, 2, func(byte) byte { return flags })
}

func setV3Reportable(t *testing.T, data []byte) []byte {
	return setV3HeaderOctet(t, data, 2, func(flags byte) byte { return flags | testReportableFlag })
}

func setV3SecurityModel(t *testing.T, data []byte, model byte) []byte {
	return setV3HeaderOctet(t, data, 3, func(byte) byte { return model })
}

func corruptV3HeaderFieldTag(t *testing.T, data []byte, i int, replacement byte) []byte {
	t.Helper()
	out := append([]byte(nil), data...)
	out[v3GlobalData(t, out)[i].start] = replacement
	return out
}

func v3OuterFields(t *testing.T, data []byte) [][]byte {
	t.Helper()
	var fields [][]byte
	for _, field := range v3Outer(t, data) {
		fields = append(fields, append([]byte(nil), data[field.start:field.end]...))
	}
	return fields
}

func replaceV3MsgData(t *testing.T, data, msgData []byte) []byte {
	t.Helper()
	fields := v3OuterFields(t, data)
	fields[3] = append([]byte(nil), msgData...)
	return berTLV(tagSequence, testBERFields(fields...))
}

func replaceV3MsgDataTag(t *testing.T, data []byte, replacement byte) []byte {
	t.Helper()
	msgData := v3OuterFields(t, data)[3]
	msgData[0] = replacement
	return replaceV3MsgData(t, data, msgData)
}

func testBERFields(fields ...[]byte) []byte {
	var data []byte
	for _, field := range fields {
		data = append(data, field...)
	}
	return data
}

// buildV3InformWithEngineID builds an unauthenticated SNMPv3 inform naming
// engineIDHex.
func buildV3InformWithEngineID(t *testing.T, user, engineIDHex, trapOID string, extra ...gosnmp.VarBind) []byte {
	t.Helper()
	return buildV3SecuredInform(t, v3SecuredTrapSpec{
		user:        user,
		engineIDHex: engineIDHex,
		authProto:   "none",
		privProto:   "none",
		trapOID:     trapOID,
		extra:       extra,
	})
}
