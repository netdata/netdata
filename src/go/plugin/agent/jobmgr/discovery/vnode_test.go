// SPDX-License-Identifier: GPL-3.0-or-later

package discovery

import (
	"testing"

	"github.com/netdata/netdata/go/plugins/pkg/confopt"
	"github.com/netdata/netdata/go/plugins/pkg/netdataapi"
	"github.com/stretchr/testify/assert"

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
				aborted, err := configuration.PrepareUpsert("node", first.Revision, &vnodes.Config{VirtualNode: *testVNode("changed", "source")})
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

				_, err := configuration.PrepareUpsert("node", 0, &vnodes.Config{VirtualNode: *testVNode("stale", "source")})
				require.ErrorIs(t, err, ErrVNodeRevision)
			},
		},
		"commit selects one preparation from the same revision": {
			run: func(t *testing.T, configuration *VNodeConfiguration) {
				first := commitVNode(t, configuration, "node", 0, testVNode("host", "source"))
				winner, err := configuration.PrepareUpsert("node", first.Revision, &vnodes.Config{VirtualNode: *testVNode("winner", "source")})
				require.NoError(t, err)
				stale, err := configuration.PrepareUpsert("node", first.Revision, &vnodes.Config{VirtualNode: *testVNode("stale", "source")})
				require.NoError(t, err)

				_, err = winner.Commit()
				require.NoError(t, err)
				_, err = stale.Commit()
				require.ErrorIs(t, err, ErrVNodeRevision)
				require.NoError(t, stale.Abort())
			},
		},
	}
	for name, test := range tests {
		t.Run(name, func(t *testing.T) {
			test.run(t, newTestVNodeConfiguration(t))
		})
	}
}

func TestVNodeConfigCopiesMutableValues(t *testing.T) {
	configuration := newTestVNodeConfiguration(t)
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
	initial := map[string]*vnodes.Config{
		"z": {VirtualNode: vnodes.VirtualNode{Name: "z", Hostname: "z-host", GUID: "z-guid", Labels: map[string]string{"site": "z"}}},
		"a": {VirtualNode: vnodes.VirtualNode{Name: "a", Hostname: "a-host", GUID: "a-guid", Labels: map[string]string{"site": "a"}}},
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
	_, err := NewVNodeConfigurationWithInitial(map[string]*vnodes.Config{"map-name": {VirtualNode: vnodes.VirtualNode{Name: "vnode-name"}}})
	require.Error(t, err)
}

func BenchmarkBVNodeConfigLookup(b *testing.B) {
	configuration := newTestVNodeConfiguration(b)
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

func newTestVNodeConfiguration(t vnodeTesting) *VNodeConfiguration {
	t.Helper()
	configuration, err := NewVNodeConfigurationWithInitial(nil)
	require.NoError(t, err)
	return configuration
}

func commitVNode(
	t vnodeTesting,
	configuration *VNodeConfiguration,
	id string,
	expected uint64,
	vnode *vnodes.VirtualNode,
) jobruntime.VnodeSnapshot {
	t.Helper()
	prepared, err := configuration.PrepareUpsert(id, expected, &vnodes.Config{VirtualNode: *vnode})
	require.NoError(t, err)
	snapshot, err := prepared.Commit()
	require.NoError(t, err)
	return snapshot
}

func testVNode(hostname, source string) *vnodes.VirtualNode {
	return &vnodes.VirtualNode{
		Name:       "node",
		Hostname:   hostname,
		GUID:       "guid",
		Source:     source,
		SourceType: "test",
		Labels:     map[string]string{"site": "original"},
	}
}

func TestVNodeDefinitionIndex(t *testing.T) {
	for name, tc := range map[string]struct{ sourceOnly, abort, move, remove, disable bool }{
		"source-only reuses definition":        {sourceOnly: true},
		"abort preserves authority":            {abort: true},
		"GUID reassignment moves authority":    {move: true},
		"remove and re-add":                    {remove: true},
		"explicit zero removes legacy timeout": {disable: true},
	} {
		t.Run(name, func(t *testing.T) {
			c := newTestVNodeConfiguration(t)
			initial := testVNode("host", "source")
			initial.GUID = "AAAAAAAA-BBBB-CCCC-DDDD-EEEEEEEEEEEE"
			initial.Labels["_node_stale_after_seconds"] = "300"
			first := commitVNode(t, c, "node", 0, initial)
			old := c.Definition("aaaaaaaa-bbbb-cccc-dddd-eeeeeeeeeeee")
			require.NotNil(t, old)
			next := initial.Copy()
			if tc.sourceOnly {
				next.Source = "other"
			} else {
				next.Hostname = "updated"
			}
			if tc.move {
				next.GUID = "bbbbbbbb-bbbb-cccc-dddd-eeeeeeeeeeee"
			}
			if tc.disable {
				zero := confopt.Duration(0)
				next.StaleAfter = &zero
			}
			prepared, err := c.PrepareUpsert("node", first.Revision, &vnodes.Config{VirtualNode: *next})
			require.NoError(t, err)
			assert.Same(t, old, c.Definition(initial.GUID))
			if tc.abort {
				require.NoError(t, prepared.Abort())
				assert.Same(t, old, c.Definition(initial.GUID))
				return
			}
			if tc.remove {
				require.NoError(t, prepared.Abort())
				removal, err := c.PrepareRemove("node", first.Revision)
				require.NoError(t, err)
				_, err = removal.Commit()
				require.NoError(t, err)
				assert.Nil(t, c.Definition(initial.GUID))
				prepared, err = c.PrepareUpsert("node", 0, &vnodes.Config{VirtualNode: *next})
				require.NoError(t, err)
			}
			snapshot, err := prepared.Commit()
			require.NoError(t, err)
			definition := c.Definition(next.GUID)
			require.NotNil(t, definition)
			wantGUID := "aaaaaaaa-bbbb-cccc-dddd-eeeeeeeeeeee"
			if tc.move {
				wantGUID = next.GUID
				assert.Nil(t, c.Definition(initial.GUID))
			}
			wantLabels := next.HostLabels()
			wantLabels["_hostname"] = next.Hostname
			assert.Equal(
				t,
				netdataapi.HostInfo{
					GUID:     wantGUID,
					Hostname: next.Hostname,
					Labels:   wantLabels,
				},
				definition.Info(),
			)
			if tc.sourceOnly {
				assert.Same(t, old, definition)
				assert.Equal(t, first.MetadataRevision, snapshot.MetadataRevision)
			} else if !tc.remove {
				assert.Equal(t, first.MetadataRevision+1, snapshot.MetadataRevision)
			}
		})
	}
}
