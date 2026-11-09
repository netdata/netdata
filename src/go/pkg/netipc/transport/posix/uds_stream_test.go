//go:build unix

package posix

import (
	"bytes"
	"encoding/binary"
	"errors"
	"syscall"
	"testing"

	"github.com/netdata/netdata/go/plugins/pkg/netipc/protocol"
)

func streamPair(t *testing.T) (int, int) {
	t.Helper()
	fds, err := syscall.Socketpair(syscall.AF_UNIX, syscall.SOCK_STREAM, 0)
	if err != nil {
		t.Fatalf("socketpair: %v", err)
	}
	return fds[0], fds[1]
}

func streamOuter(declared uint32, body []byte) []byte {
	hdr := protocol.Header{
		Magic:      protocol.MagicMsg,
		Version:    protocol.Version,
		HeaderLen:  protocol.HeaderSize,
		Kind:       protocol.KindRequest,
		Code:       1,
		ItemCount:  1,
		MessageID:  9,
		PayloadLen: declared,
	}
	pkt := make([]byte, protocol.HeaderSize, protocol.HeaderSize+len(body))
	hdr.Encode(pkt)
	return append(pkt, body...)
}

func streamContinuation(declared uint32, body []byte) []byte {
	chk := protocol.ChunkHeader{
		Magic:           protocol.MagicChunk,
		Version:         protocol.Version,
		MessageID:       9,
		ChunkIndex:      1,
		ChunkCount:      2,
		ChunkPayloadLen: declared,
	}
	pkt := make([]byte, protocol.HeaderSize, protocol.HeaderSize+len(body))
	chk.Encode(pkt)
	return append(pkt, body...)
}

// streamInject installs the stream test hook: the next shortReads reads and
// shortWrites writes of more than one byte move one byte, after eintrReads
// and eintrWrites calls failed with EINTR. It returns the injections not
// consumed yet.
func streamInject(t *testing.T, shortReads, shortWrites, eintrReads, eintrWrites int) func() int {
	t.Helper()
	hook := func(write bool, n int) (int, bool) {
		eintr, short := &eintrReads, &shortReads
		if write {
			eintr, short = &eintrWrites, &shortWrites
		}
		if *eintr > 0 {
			*eintr--
			return n, true
		}
		if *short > 0 && n > 1 {
			*short--
			return 1, false
		}
		return n, false
	}
	streamTestIO.Store(&hook)
	t.Cleanup(func() { streamTestIO.Store(nil) })
	return func() int { return shortReads + shortWrites + eintrReads + eintrWrites }
}

func TestStreamBodyLenComesFromTheHeader(t *testing.T) {
	pkt := streamOuter(100, nil)
	cases := []struct {
		name       string
		hdr        []byte
		bufLen     int
		packetSize uint32
		want       int
	}{
		{"whole payload fits", pkt, 1024, 1024, 100},
		{"first chunk fills the packet", pkt, 1024, 48, 16},
		{"handshake: capped by the buffer", pkt, 80, 0, 48},
		{"bytes past the buffer", pkt, 64, 1024, 32},
		{"continuation", streamContinuation(7, nil), 1024, 48, 7},
		{"unknown magic", make([]byte, protocol.HeaderSize), 1024, 1024, 0},
	}
	for _, tc := range cases {
		if got := streamBodyLen(tc.hdr, tc.bufLen, tc.packetSize); got != tc.want {
			t.Errorf("%s: streamBodyLen = %d, want %d", tc.name, got, tc.want)
		}
	}
}

func TestStreamPacketsRoundTripAndEOFMapping(t *testing.T) {
	a, b := streamPair(t)
	pkt1 := streamOuter(5, []byte("hello"))
	pkt2 := streamContinuation(3, []byte("abc"))
	if err := streamSendAll(a, pkt1[:protocol.HeaderSize], pkt1[protocol.HeaderSize:]); err != nil {
		t.Fatalf("send pkt1: %v", err)
	}
	if err := streamSendAll(a, pkt2, nil); err != nil {
		t.Fatalf("send pkt2: %v", err)
	}

	buf := make([]byte, 64)
	for _, want := range [][]byte{pkt1, pkt2} {
		n, err := streamRecvPacket(b, buf, 64, nil)
		if err != nil || !bytes.Equal(buf[:n], want) {
			t.Fatalf("streamRecvPacket = %d, %v; want %d bytes", n, err, len(want))
		}
	}

	wait := newReceiveWait(1, nil)
	if _, err := streamRecvPacket(b, buf, 64, &wait); !errors.Is(err, ErrTimeout) {
		t.Fatalf("idle stream: %v, want ErrTimeout", err)
	}

	truncated := streamOuter(5, []byte("he"))
	if err := streamSendAll(a, truncated, nil); err != nil {
		t.Fatalf("send truncated: %v", err)
	}
	_ = syscall.Shutdown(a, syscall.SHUT_WR)
	if _, err := streamRecvPacket(b, buf, 64, nil); !errors.Is(err, ErrProtocol) {
		t.Fatalf("EOF inside the body: %v, want ErrProtocol", err)
	}
	syscall.Close(a)
	syscall.Close(b)

	a, b = streamPair(t)
	if err := streamSendAll(a, truncated[:10], nil); err != nil {
		t.Fatalf("send header part: %v", err)
	}
	_ = syscall.Shutdown(a, syscall.SHUT_WR)
	if _, err := streamRecvPacket(b, buf, 64, nil); !errors.Is(err, ErrProtocol) {
		t.Fatalf("EOF inside the header: %v, want ErrProtocol", err)
	}
	syscall.Close(a)
	syscall.Close(b)

	a, b = streamPair(t)
	syscall.Close(a)
	if _, err := streamRecvPacket(b, buf, 64, nil); !errors.Is(err, ErrRecv) {
		t.Fatalf("EOF between packets: %v, want ErrRecv (disconnect)", err)
	}
	syscall.Close(b)
}

func TestStreamSessionPacketChecksTheFirstHeaderFirst(t *testing.T) {
	a, b := streamPair(t)
	defer syscall.Close(b)
	buf := make([]byte, 256)

	// Over the payload limit: the header comes back alone, the body is
	// never waited for.
	if err := streamSendAll(a, streamOuter(200, nil), nil); err != nil {
		t.Fatalf("send: %v", err)
	}
	if n, err := streamRecvSessionPacket(b, nil, buf, true, 256, 100, 1, nil, new(bool)); n != protocol.HeaderSize || err != nil {
		t.Fatalf("over-limit header = %d, %v; want header only", n, err)
	}

	// Any header that does not decode, here a wrong version: header only.
	badVersion := streamOuter(50, nil)
	binary.NativeEndian.PutUint16(badVersion[4:6], protocol.Version+1)
	if err := streamSendAll(a, badVersion, nil); err != nil {
		t.Fatalf("send: %v", err)
	}
	if n, err := streamRecvSessionPacket(b, nil, buf, true, 256, 100, 1, nil, new(bool)); n != protocol.HeaderSize || err != nil {
		t.Fatalf("undecodable header = %d, %v; want header only", n, err)
	}

	// A continuation header where a message starts: header only.
	if err := streamSendAll(a, streamContinuation(3, []byte("abc")), nil); err != nil {
		t.Fatalf("send: %v", err)
	}
	if n, err := streamRecvSessionPacket(b, nil, buf, true, 256, 100, 1, nil, new(bool)); n != protocol.HeaderSize || err != nil {
		t.Fatalf("continuation first = %d, %v; want header only", n, err)
	}
	// Its body is still queued; read it off as a raw packet body.
	if err := streamReadExact(b, buf[:3], nil, true); err != nil {
		t.Fatalf("read body: %v", err)
	}

	// A truncated continuation is a chunk error.
	if err := streamSendAll(a, streamContinuation(5, []byte("ab")), nil); err != nil {
		t.Fatalf("send: %v", err)
	}
	_ = syscall.Shutdown(a, syscall.SHUT_WR)
	if _, err := streamRecvSessionPacket(b, nil, buf, false, 256, 100, 1, nil, new(bool)); !errors.Is(err, ErrChunk) {
		t.Fatalf("truncated continuation: %v, want ErrChunk", err)
	}
	syscall.Close(a)
}

func TestStreamOneByteTransfersAndEINTRYieldWholePackets(t *testing.T) {
	a, b := streamPair(t)
	defer syscall.Close(a)
	defer syscall.Close(b)
	pkt := streamOuter(5, []byte("hello"))

	// len-1 one-byte writes leave one byte for the last call; reads of more
	// than one byte are cut to one, 31 in the header and 4 in the body.
	pending := streamInject(t, len(pkt)-2, len(pkt)-1, 2, 2)
	if err := streamSendAll(a, pkt[:protocol.HeaderSize], pkt[protocol.HeaderSize:]); err != nil {
		t.Fatalf("send: %v", err)
	}
	buf := make([]byte, 64)
	n, err := streamRecvPacket(b, buf, 64, nil)
	if err != nil || !bytes.Equal(buf[:n], pkt) {
		t.Fatalf("streamRecvPacket = %d, %v; want the whole packet", n, err)
	}
	if left := pending(); left != 0 {
		t.Fatalf("%d injections not consumed", left)
	}
}
