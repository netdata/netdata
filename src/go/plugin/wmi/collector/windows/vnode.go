// SPDX-License-Identifier: GPL-3.0-or-later

package windows

import (
	"net/netip"
	"strings"

	"github.com/google/uuid"

	"github.com/netdata/netdata/go/plugins/plugin/framework/vnodes"
)

// canonicalEndpoint is the configured host in one spelling per target: an
// unmapped IP address, or a lowercase DNS name without the trailing dot.
func canonicalEndpoint(host string) string {
	if addr, err := netip.ParseAddr(host); err == nil {
		return addr.Unmap().String()
	}
	return strings.ToLower(strings.TrimSuffix(host, "."))
}

// newVirtualNode creates the job's generated node. Its GUID derives from the
// endpoint, so a Windows rename keeps the node and its history; the inventory
// later replaces the hostname with the Windows computer name.
func newVirtualNode(host string) *vnodes.VirtualNode {
	endpoint := canonicalEndpoint(host)
	return &vnodes.VirtualNode{
		GUID:     uuid.NewSHA1(uuid.NameSpaceDNS, []byte("wmi:"+endpoint)).String(),
		Hostname: endpoint,
		Labels:   map[string]string{"_vnode_type": "wmi", "_os": "windows", "address": host},
	}
}
