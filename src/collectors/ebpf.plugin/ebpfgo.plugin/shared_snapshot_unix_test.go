//go:build unix

package main

import (
	"testing"

	"github.com/netdata/netdata/src/collectors/ebpf.plugin/ebpfgo.plugin/netipc/protocol"
)

const testSharedSnapshotResponseSize = 65536

func TestSharedSnapshotStoreAndBuilderRoundTrip(t *testing.T) {
	store := NewSharedSnapshotStore()
	sourceItems := []SharedSnapshotItem{
		{
			Hash:    1001,
			Options: 2,
			Enabled: 1,
			Name:    "docker-abc123",
			Path:    "/sys/fs/cgroup/docker/abc123",
		},
		{
			Hash:    2002,
			Options: 4,
			Enabled: 0,
			Name:    "systemd-user",
			Path:    "/sys/fs/cgroup/user.slice/user-1000",
		},
	}

	store.Replace(SharedSnapshotState{
		SystemdEnabled: 1,
		Generation:     123,
		Items:          sourceItems,
	})

	// Ensure Replace keeps an owned copy.
	sourceItems[0].Name = "mutated"
	snapshot := store.Snapshot()
	if snapshot.Items[0].Name != "docker-abc123" {
		t.Fatalf("Snapshot() copied item name = %q, want %q", snapshot.Items[0].Name, "docker-abc123")
	}

	buf := make([]byte, testSharedSnapshotResponseSize)
	builder := protocol.NewCgroupsBuilder(buf, 3, 0, 0)
	if !writeSharedSnapshot(builder, snapshot) {
		t.Fatal("writeSharedSnapshot() returned false")
	}

	n := builder.Finish()
	view, err := protocol.DecodeCgroupsResponse(buf[:n])
	if err != nil {
		t.Fatalf("DecodeCgroupsResponse() failed: %v", err)
	}

	if view.ItemCount != 2 {
		t.Fatalf("ItemCount = %d, want 2", view.ItemCount)
	}
	if view.SystemdEnabled != 1 {
		t.Fatalf("SystemdEnabled = %d, want 1", view.SystemdEnabled)
	}
	if view.Generation != 123 {
		t.Fatalf("Generation = %d, want 123", view.Generation)
	}

	item, err := view.Item(0)
	if err != nil {
		t.Fatalf("Item(0): %v", err)
	}
	if item.Hash != 1001 || item.Name.String() != "docker-abc123" || item.Path.String() != "/sys/fs/cgroup/docker/abc123" {
		t.Fatalf("unexpected item0: %+v", item)
	}
}
