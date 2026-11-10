// SPDX-License-Identifier: GPL-3.0-or-later
package lighthouse

import "github.com/netdata/netdata/go/plugins/plugin/framework/vnodes"

// Configured vnode snapshots and Collect share the framework job goroutine.
func (c *Collector) SetConfiguredVnode(v vnodes.VirtualNode) {
	c.vnodeGUID, _ = vnodes.ConfiguredGUIDKey(v.GUID)
	c.request.VnodeGUID = c.vnodeGUID
	if c.session != nil {
		c.session.SetVnodeGUID(c.vnodeGUID)
	}
}
