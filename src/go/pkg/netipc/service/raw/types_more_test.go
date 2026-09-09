package raw

import (
	"testing"

	"github.com/netdata/netdata/go/plugins/pkg/netipc/protocol"
)

func TestSnapshotMaxItems(t *testing.T) {
	got := SnapshotMaxItems(4096, 0)
	want := protocol.EstimateCgroupsMaxItems(4096)
	if got != want {
		t.Fatalf("SnapshotMaxItems default = %d, want %d", got, want)
	}

	if got := SnapshotMaxItems(4096, 7); got != 7 {
		t.Fatalf("SnapshotMaxItems override = %d, want 7", got)
	}
}
