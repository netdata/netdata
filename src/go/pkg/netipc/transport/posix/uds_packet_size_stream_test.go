//go:build darwin || freebsd

package posix

import (
	"syscall"
	"testing"

	"github.com/netdata/netdata/go/plugins/pkg/netipc/protocol"
)

func TestPacketSizeLimitIsFixedOnAStream(t *testing.T) {
	for _, tc := range []struct{ configured, want uint32 }{
		{0, packetSizeMax},
		{1000, 1000},
		{^uint32(0), packetSizeMax},
	} {
		if got := packetSizeLimit(-1, tc.configured); got != tc.want {
			t.Fatalf("packetSizeLimit(-1, %d) = %d, want %d", tc.configured, got, tc.want)
		}
	}

	// A full packet crosses even though it is far above SO_SNDBUF.
	fds, err := syscall.Socketpair(syscall.AF_UNIX, sockType, 0)
	if err != nil {
		t.Fatalf("socketpair: %v", err)
	}
	defer syscall.Close(fds[0])
	pkt := make([]byte, packetSizeMax)
	for i := range pkt {
		pkt[i] = 0xA5
	}
	hdr := protocol.Header{
		Magic:      protocol.MagicMsg,
		Version:    protocol.Version,
		HeaderLen:  protocol.HeaderSize,
		Kind:       protocol.KindRequest,
		ItemCount:  1,
		PayloadLen: packetSizeMax - protocol.HeaderSize,
	}
	hdr.Encode(pkt[:protocol.HeaderSize])

	type result struct {
		n    int
		last byte
		err  error
	}
	done := make(chan result, 1)
	go func() {
		defer syscall.Close(fds[1])
		buf := make([]byte, packetSizeMax)
		n, err := streamRecvPacket(fds[1], buf, packetSizeMax, nil)
		r := result{n: n, err: err}
		if n > 0 {
			r.last = buf[n-1]
		}
		done <- r
	}()
	if err := sendPacket(fds[0], pkt[:protocol.HeaderSize], pkt[protocol.HeaderSize:]); err != nil {
		t.Fatalf("send: %v", err)
	}
	if r := <-done; r.err != nil || r.n != int(packetSizeMax) || r.last != 0xA5 {
		t.Fatalf("recv = %d, last %#x, %v; want %d bytes", r.n, r.last, r.err, packetSizeMax)
	}
}
