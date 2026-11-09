//go:build unix && !darwin && !freebsd

package posix

import (
	"errors"
	"syscall"
	"testing"

	"github.com/netdata/netdata/go/plugins/pkg/netipc/protocol"
)

func TestPacketSizeLimitFallbackAndClamp(t *testing.T) {
	if got := packetSizeLimit(-1, 0); got != defaultPacketSizeFallback {
		t.Fatalf("packetSizeLimit(-1, 0) = %d, want fallback %d", got, defaultPacketSizeFallback)
	}
	if got := packetSizeLimit(-1, 1000); got != 1000 {
		t.Fatalf("packetSizeLimit(-1, 1000) = %d, want 1000", got)
	}
	if got := packetSizeLimit(-1, defaultPacketSizeFallback+1); got != defaultPacketSizeFallback {
		t.Fatalf("packetSizeLimit(-1, fallback+1) = %d, want %d", got, defaultPacketSizeFallback)
	}
}

func TestPacketSizeLimitMatchesKernelDatagramLimit(t *testing.T) {
	// 0 keeps the system default SO_SNDBUF.
	for _, sndbuf := range []int{0, 4096, 65536} {
		fds, err := syscall.Socketpair(syscall.AF_UNIX, sockType, 0)
		if err != nil {
			t.Fatalf("socketpair: %v", err)
		}
		if sndbuf != 0 {
			if err := syscall.SetsockoptInt(fds[0], syscall.SOL_SOCKET, syscall.SO_SNDBUF, sndbuf); err != nil {
				t.Fatalf("setsockopt(SO_SNDBUF): %v", err)
			}
		}
		limit := packetSizeLimit(fds[0], 0)
		if limit <= protocol.HeaderSize || limit > packetSizeMax {
			t.Fatalf("sndbuf %d: limit %d out of range", sndbuf, limit)
		}
		if got := packetSizeLimit(fds[0], ^uint32(0)); got != limit {
			t.Fatalf("sndbuf %d: configured max clamps to %d, want %d", sndbuf, got, limit)
		}
		if err := rawSendPacket(fds[0], make([]byte, limit)); err != nil {
			t.Fatalf("sndbuf %d: send %d bytes: %v", sndbuf, limit, err)
		}
		buf := make([]byte, limit)
		if n, err := rawRecv(fds[1], buf); err != nil || n != int(limit) {
			t.Fatalf("sndbuf %d: recv = %d, %v", sndbuf, n, err)
		}
		if limit < packetSizeMax {
			// Guards the SO_SNDBUF - 32 rule: one more byte must not fit.
			if err := rawSendPacket(fds[0], make([]byte, limit+1)); err == nil {
				t.Fatalf("sndbuf %d: %d + 1 bytes unexpectedly sent", sndbuf, limit)
			}
		}
		syscall.Close(fds[0])
		syscall.Close(fds[1])
	}
}

// A datagram longer than its header declares is a protocol error. The
// pending queue, filled while a send waits for buffer space, must keep the
// whole datagram so its receive rejects it as the direct receive does,
// instead of storing it cut to PacketSize and accepting it.
func TestPendingOversizedDatagramRejected(t *testing.T) {
	fds, err := syscall.Socketpair(syscall.AF_UNIX, sockType, 0)
	if err != nil {
		t.Fatalf("socketpair: %v", err)
	}
	defer syscall.Close(fds[1])
	s := &Session{
		fd:                     fds[0],
		role:                   RoleServer,
		PacketSize:             2 * protocol.HeaderSize,
		MaxRequestPayloadBytes: 1024,
		MaxRequestBatchItems:   1,
	}
	s.initReceiveFns()
	defer s.Close()

	// The header declares exactly PacketSize; 16 more bytes follow.
	dgram := make([]byte, 2*protocol.HeaderSize+16)
	hdr := protocol.Header{
		Magic: protocol.MagicMsg, Version: protocol.Version,
		HeaderLen: protocol.HeaderSize, Kind: protocol.KindRequest,
		Code: 1, ItemCount: 1, MessageID: 1, PayloadLen: protocol.HeaderSize,
	}
	hdr.Encode(dgram[:protocol.HeaderSize])
	rbuf := make([]byte, 256)

	if _, err := syscall.Write(fds[1], dgram); err != nil {
		t.Fatalf("direct datagram: %v", err)
	}
	if _, _, err := s.Receive(rbuf); !errors.Is(err, ErrProtocol) {
		t.Fatalf("direct receive: expected ErrProtocol, got %v", err)
	}

	if _, err := syscall.Write(fds[1], dgram); err != nil {
		t.Fatalf("queued datagram: %v", err)
	}
	eof := false
	if err := s.pending.drain(s.fd, int(s.PacketSize), 4096, &eof); err != nil || eof || !s.pending.queued() {
		t.Fatalf("drain: err %v eof %v queued %v", err, eof, s.pending.queued())
	}
	if _, _, err := s.Receive(rbuf); !errors.Is(err, ErrProtocol) {
		t.Fatalf("pending receive: expected ErrProtocol, got %v", err)
	}
	if s.pending.queued() {
		t.Fatal("pending queue not consumed")
	}
}
