// SPDX-License-Identifier: GPL-3.0-or-later

package discovery

import (
	"errors"
	"strconv"
	"strings"
	"testing"

	"github.com/netdata/netdata/go/plugins/plugin/framework/jobruntime"
	"github.com/netdata/netdata/go/plugins/plugin/framework/vnodes"
)

func TestVNodeConfigAtomicRevisions(t *testing.T) {
	tests := map[string]struct {
		run func(*testing.T, *VNodeConfiguration)
	}{
		"abort preserves current": {
			run: func(t *testing.T, configuration *VNodeConfiguration) {
				first := commitVNode(t, configuration, "node", 0, testVNode("host", "source"))
				aborted, err := configuration.PrepareUpsert(
					"node", first.Revision, testVNode("changed", "source"),
				)
				if err != nil {
					t.Fatal(err)
				}
				if err := aborted.Abort(); err != nil {
					t.Fatal(err)
				}
				current, ok := configuration.Lookup("node")
				if !ok || current.Revision != first.Revision ||
					current.Vnode.Hostname != "host" {
					t.Fatalf("snapshot=%+v ok=%v", current, ok)
				}
			},
		},
		"metadata revision changes only with metadata": {
			run: func(t *testing.T, configuration *VNodeConfiguration) {
				first := commitVNode(t, configuration, "node", 0, testVNode("host", "source-a"))
				sourceOnly := testVNode("host", "source-b")
				second := commitVNode(t, configuration, "node", first.Revision, sourceOnly)
				metadata := testVNode("next-host", "source-b")
				third := commitVNode(t, configuration, "node", second.Revision, metadata)
				if first.MetadataRevision != 1 ||
					second.MetadataRevision != first.MetadataRevision ||
					third.MetadataRevision != second.MetadataRevision+1 {
					t.Fatalf("metadata revisions=%d/%d/%d",
						first.MetadataRevision,
						second.MetadataRevision,
						third.MetadataRevision,
					)
				}
			},
		},
		"remove is atomic": {
			run: func(t *testing.T, configuration *VNodeConfiguration) {
				first := commitVNode(t, configuration, "node", 0, testVNode("host", "source"))
				prepared, err := configuration.PrepareRemove("node", first.Revision)
				if err != nil {
					t.Fatal(err)
				}
				if _, ok := configuration.Lookup("node"); !ok {
					t.Fatal("prepared removal became visible")
				}
				removed, err := prepared.Commit()
				if err != nil {
					t.Fatal(err)
				}
				if removed.Revision != first.Revision+1 || removed.Vnode != nil {
					t.Fatalf("removed snapshot=%+v", removed)
				}
				if _, ok := configuration.Lookup("node"); ok {
					t.Fatal("committed removal retained vnode")
				}
			},
		},
		"stale revision is rejected": {
			run: func(t *testing.T, configuration *VNodeConfiguration) {
				commitVNode(t, configuration, "node", 0, testVNode("host", "source"))
				if _, err := configuration.PrepareUpsert(
					"node", 0, testVNode("stale", "source"),
				); !errors.Is(err, ErrVNodeRevision) {
					t.Fatalf("stale revision error=%v", err)
				}
			},
		},
	}
	for name, test := range tests {
		t.Run(name, func(t *testing.T) {
			test.run(t, NewVNodeConfiguration())
		})
	}
}

func TestVNodeConfigCopiesMutableValues(t *testing.T) {
	configuration := NewVNodeConfiguration()
	input := testVNode("host", "source")
	snapshot := commitVNode(t, configuration, "node", 0, input)
	input.Labels["site"] = "mutated-input"
	snapshot.Vnode.Labels["site"] = "mutated-output"
	current, ok := configuration.Lookup("node")
	if !ok || current.Vnode.Labels["site"] != "original" {
		t.Fatalf("stored snapshot aliases caller memory: %+v", current)
	}
	current.Vnode.Labels["site"] = "mutated-lookup"
	again, _ := configuration.Lookup("node")
	if again.Vnode.Labels["site"] != "original" {
		t.Fatalf("lookup snapshot aliases stored memory: %+v", again)
	}
}

func TestVNodeConfigInitialSnapshotIsDeterministicAndIndependent(t *testing.T) {
	initial := map[string]*vnodes.VirtualNode{
		"z": {
			Name: "z", Hostname: "z-host", GUID: "z-guid",
			Labels: map[string]string{"site": "z"},
		},
		"a": {
			Name: "a", Hostname: "a-host", GUID: "a-guid",
			Labels: map[string]string{"site": "a"},
		},
	}
	configuration, err := NewVNodeConfigurationWithInitial(initial)
	if err != nil {
		t.Fatal(err)
	}
	initial["a"].Labels["site"] = "changed"
	entries := configuration.Entries()
	if len(entries) != 2 ||
		entries[0].ID != "a" ||
		entries[1].ID != "z" ||
		entries[0].Snapshot.Vnode.Labels["site"] != "a" {
		t.Fatalf("entries=%+v", entries)
	}
	entries[0].Snapshot.Vnode.Labels["site"] = "changed-again"
	snapshot, ok := configuration.Lookup("a")
	if !ok || snapshot.Vnode.Labels["site"] != "a" {
		t.Fatalf("stored snapshot aliases entries: %+v", snapshot)
	}
}

func TestVNodeConfigInitialIdentityMustMatchMapKey(t *testing.T) {
	if _, err := NewVNodeConfigurationWithInitial(
		map[string]*vnodes.VirtualNode{
			"map-name": {Name: "vnode-name"},
		},
	); err == nil {
		t.Fatal("mismatched initial vnode identity was accepted")
	}
}

func TestVNodeConfigBounds(t *testing.T) {
	tests := map[string]struct {
		prepare func(*VNodeConfiguration) error
	}{
		"record count": {
			prepare: func(configuration *VNodeConfiguration) error {
				for index := 0; index < MaximumVNodeConfigurationRecords; index++ {
					id := strconv.Itoa(index)
					commitVNode(t, configuration, id, 0, testVNode(id, "source"))
				}
				_, err := configuration.PrepareUpsert(
					"overflow", 0, testVNode("overflow", "source"),
				)
				return err
			},
		},
		"record bytes": {
			prepare: func(configuration *VNodeConfiguration) error {
				vnode := testVNode("host", "source")
				vnode.Labels["large"] = strings.Repeat("x", MaximumVNodeConfigurationBytes)
				_, err := configuration.PrepareUpsert("node", 0, vnode)
				return err
			},
		},
	}
	for name, test := range tests {
		t.Run(name, func(t *testing.T) {
			if err := test.prepare(NewVNodeConfiguration()); !errors.Is(err, ErrVNodeCapacity) {
				t.Fatalf("capacity error=%v", err)
			}
		})
	}
}

func BenchmarkBVNodeConfigLookup(b *testing.B) {
	configuration := NewVNodeConfiguration()
	commitVNode(b, configuration, "node", 0, testVNode("host", "source"))
	b.ReportAllocs()
	for b.Loop() {
		_, _ = configuration.Lookup("node")
	}
}

type vnodeTesting interface {
	Helper()
	Fatal(...any)
}

func commitVNode(
	t vnodeTesting,
	configuration *VNodeConfiguration,
	id string,
	expected uint64,
	vnode *vnodes.VirtualNode,
) jobruntime.VnodeSnapshot {
	t.Helper()
	prepared, err := configuration.PrepareUpsert(id, expected, vnode)
	if err != nil {
		t.Fatal(err)
	}
	snapshot, err := prepared.Commit()
	if err != nil {
		t.Fatal(err)
	}
	return snapshot
}

func testVNode(hostname, source string) *vnodes.VirtualNode {
	return &vnodes.VirtualNode{
		Name: "node", Hostname: hostname, GUID: "guid",
		Source: source, SourceType: "test",
		Labels: map[string]string{"site": "original"},
	}
}
