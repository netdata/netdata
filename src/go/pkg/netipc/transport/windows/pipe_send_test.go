//go:build windows

package windows

import (
	"testing"

	"github.com/netdata/netdata/go/plugins/pkg/netipc/protocol"
)

func TestHeaderPayloadLenBounds(t *testing.T) {
	if _, err := headerPayloadLen(-1); err == nil {
		t.Fatal("negative payload length should fail")
	}

	maxInt := int(^uint(0) >> 1)
	if _, err := headerPayloadLen(maxInt); err == nil {
		t.Fatal("payload length that overflows int total length should fail")
	}

	protocolLimit := uint64(^uint32(0)) - uint64(protocol.HeaderSize)
	if protocolLimit <= uint64(maxInt) {
		got, err := headerPayloadLen(int(protocolLimit))
		if err != nil {
			t.Fatalf("protocol-limit payload should pass: %v", err)
		}
		if uint64(got) != uint64(^uint32(0)) {
			t.Fatalf("total length = %d, want %d", got, uint64(^uint32(0)))
		}
	}
}
