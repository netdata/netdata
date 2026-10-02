// SPDX-License-Identifier: GPL-3.0-or-later

package jobruntime

import (
	"context"
	"io"
	"testing"

	"github.com/netdata/netdata/go/plugins/plugin/framework/collectorapi"
	"github.com/netdata/netdata/go/plugins/plugin/framework/vnodes"
	"github.com/stretchr/testify/require"
)

type configuredV1Consumer struct {
	collectorapi.MockCollectorV1
	vnode    vnodes.VirtualNode
	supplies int
}

func (c *configuredV1Consumer) SetConfiguredVnode(v vnodes.VirtualNode) {
	c.vnode = v
	c.supplies++
}

func TestV1ConfiguredConsumerReceivesOwnedSnapshotsAtLifecycleBoundaries(t *testing.T) {
	v := vnodes.VirtualNode{Name: "router", GUID: "node-guid", Hostname: "router-one", Labels: map[string]string{"site": "before"}}
	holder := newSnapshotHolder(VnodeSnapshot{Vnode: &v, Revision: 1, MetadataRevision: 1})
	c := &configuredV1Consumer{}
	c.InitFunc = func(context.Context) error {
		require.Equal(t, "router-one", c.vnode.Hostname)
		c.vnode.Labels["consumer-only"] = "owned"
		return nil
	}
	c.ChartsFunc = func() *collectorapi.Charts { return &collectorapi.Charts{} }
	c.CollectFunc = func(context.Context) map[string]int64 {
		require.Equal(t, "router-two", c.vnode.Hostname)
		return nil
	}
	j := NewJob(JobConfig{PluginName: "go.d", Name: "test", ModuleName: "test", FullName: "test_test", Module: c, Out: io.Discard, Vnode: *v.Copy(), VnodeName: v.Name, VnodeRevision: 1, VnodeMetadataRevision: 1, VnodeLookup: holder.lookup})
	require.NoError(t, j.AutoDetectionManaged(context.Background()))
	require.NotContains(t, j.vnode.Labels, "consumer-only")
	next := v.Copy()
	next.Hostname = "router-two"
	holder.set(VnodeSnapshot{Vnode: next, Revision: 2, MetadataRevision: 2})
	j.runOnce()
	require.Equal(t, 2, c.supplies)
	j.runOnce()
	require.Equal(t, 2, c.supplies, "unchanged cycles must not resupply snapshots")
}
