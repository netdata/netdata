//go:build unix && !darwin && !freebsd

package posix

import (
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
