// SPDX-License-Identifier: GPL-3.0-or-later

package jobruntime

import (
	"github.com/netdata/netdata/go/plugins/plugin/framework/collectorapi"
	"github.com/netdata/netdata/go/plugins/plugin/framework/vnodes"
)

type VnodeLookup func(name string) (VnodeSnapshot, bool)

// VnodeSnapshot is an immutable view of one committed jobmgr vnode store entry.
//
// Revision changes on every stored vnode commit. MetadataRevision changes only
// when runtime-visible HOSTINFO/HOST_DEFINE metadata changes.
type VnodeSnapshot struct {
	Vnode            *vnodes.VirtualNode
	Revision         uint64
	MetadataRevision uint64
}

func (s VnodeSnapshot) Copy() VnodeSnapshot {
	var vnode *vnodes.VirtualNode
	if s.Vnode != nil {
		vnode = s.Vnode.Copy()
	}
	return VnodeSnapshot{
		Vnode:            vnode,
		Revision:         s.Revision,
		MetadataRevision: s.MetadataRevision,
	}
}

// Called during construction or synchronously by the owning job's collection loop.
// Configuration updates never write job state directly, so this copy has no competing writer.
func supplyConfiguredVnode(module any, vnode *vnodes.VirtualNode) {
	if consumer, ok := module.(collectorapi.ConfiguredVnodeConsumer); ok {
		snapshot := vnode.Copy()
		snapshot.Labels = snapshot.HostLabels()
		consumer.SetConfiguredVnode(*snapshot)
	}
}
