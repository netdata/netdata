// SPDX-License-Identifier: GPL-3.0-or-later

package discovery

import (
	"strconv"
	"strings"
	"testing"

	"github.com/netdata/netdata/go/plugins/plugin/framework/jobruntime"
	"github.com/netdata/netdata/go/plugins/plugin/framework/vnodes"
	"github.com/stretchr/testify/require"
)

func TestVNodeConfigAtomicRevisions(t *testing.T) {
	tests := map[string]struct {
		run func(*testing.T, *VNodeConfiguration)
	}{
		"abort preserves current": {
			run: func(t *testing.T, configuration *VNodeConfiguration) {
				first := commitVNode(t, configuration, "node", 0, testVNode("host", "source"))
				aborted, err := configuration.PrepareUpsert("node", first.Revision, testVNode("changed", "source"))
				require.NoError(t, err)

				require.NoError(t, aborted.Abort())

				current, ok := configuration.Lookup("node")
				require.False(t, !ok || current.Revision != first.Revision || current.Vnode.Hostname != "host")
			},
		},
		"metadata revision changes only with metadata": {
			run: func(t *testing.T, configuration *VNodeConfiguration) {
				first := commitVNode(t, configuration, "node", 0, testVNode("host", "source-a"))
				sourceOnly := testVNode("host", "source-b")
				second := commitVNode(t, configuration, "node", first.Revision, sourceOnly)
				metadata := testVNode("next-host", "source-b")
				third := commitVNode(t, configuration, "node", second.Revision, metadata)
				require.False(t, first.MetadataRevision != 1 ||
					second.MetadataRevision != first.MetadataRevision ||
					third.MetadataRevision != second.MetadataRevision+1)
			},
		},
		"remove is atomic": {
			run: func(t *testing.T, configuration *VNodeConfiguration) {
				first := commitVNode(t, configuration, "node", 0, testVNode("host", "source"))
				prepared, err := configuration.PrepareRemove("node", first.Revision)
				require.NoError(t, err)

				_, ok := configuration.Lookup("node")
				require.True(t, ok)

				removed, err := prepared.Commit()
				require.NoError(t, err)
				require.False(t, removed.Revision != first.Revision+1 || removed.Vnode != nil)

				_, lookup := configuration.Lookup("node")
				require.False(t, lookup)

			},
		},
		"stale revision is rejected": {
			run: func(t *testing.T, configuration *VNodeConfiguration) {
				commitVNode(t, configuration, "node", 0, testVNode("host", "source"))

				_, err := configuration.PrepareUpsert("node", 0, testVNode("stale", "source"))
				require.ErrorIs(t, err, ErrVNodeRevision)
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
	require.False(t, !ok || current.Vnode.Labels["site"] != "original")
	current.Vnode.Labels["site"] = "mutated-lookup"
	again, _ := configuration.Lookup("node")
	require.EqualValues(t, "original", again.Vnode.Labels["site"])
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
	require.NoError(t, err)
	initial["a"].Labels["site"] = "changed"
	entries := configuration.Entries()
	require.False(t, len(entries) != 2 ||
		entries[0].ID != "a" ||
		entries[1].ID != "z" ||
		entries[0].Snapshot.Vnode.Labels["site"] != "a")
	entries[0].Snapshot.Vnode.Labels["site"] = "changed-again"
	snapshot, ok := configuration.Lookup("a")
	require.False(t, !ok || snapshot.Vnode.Labels["site"] != "a")
}

func TestVNodeConfigInitialIdentityMustMatchMapKey(t *testing.T) {
	_, err := NewVNodeConfigurationWithInitial(
		map[string]*vnodes.VirtualNode{
			"map-name": {Name: "vnode-name"},
		},
	)
	require.Error(t, err)
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
				_, err := configuration.PrepareUpsert("overflow", 0, testVNode("overflow", "source"))
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
			err := test.prepare(NewVNodeConfiguration())
			require.ErrorIs(t, err, ErrVNodeCapacity)
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
	require.TestingT
	Helper()
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
	require.NoError(t, err)
	snapshot, err := prepared.Commit()
	require.NoError(t, err)
	return snapshot
}

func testVNode(hostname, source string) *vnodes.VirtualNode {
	return &vnodes.VirtualNode{
		Name: "node", Hostname: hostname, GUID: "guid",
		Source: source, SourceType: "test",
		Labels: map[string]string{"site": "original"},
	}
}
