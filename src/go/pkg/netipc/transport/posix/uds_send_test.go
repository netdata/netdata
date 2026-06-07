//go:build unix

package posix

import (
	"testing"

	"github.com/netdata/netdata/go/plugins/pkg/netipc/protocol"
	"github.com/netdata/netdata/go/plugins/pkg/netipc/transport/internal/framing"
)

func TestHeaderPayloadLenBounds(t *testing.T) {
	if _, _, ok := framing.HeaderPayloadLen(-1); ok {
		t.Fatal("negative payload length should fail")
	}

	maxInt := int(^uint(0) >> 1)
	if _, _, ok := framing.HeaderPayloadLen(maxInt); ok {
		t.Fatal("payload length that overflows int total length should fail")
	}

	protocolLimit := uint64(^uint32(0)) - uint64(protocol.HeaderSize)
	if protocolLimit <= uint64(maxInt) {
		got, _, ok := framing.HeaderPayloadLen(int(protocolLimit))
		if !ok {
			t.Fatal("protocol-limit payload should pass")
		}
		if uint64(got) != uint64(^uint32(0)) {
			t.Fatalf("total length = %d, want %d", got, uint64(^uint32(0)))
		}
	}
}
