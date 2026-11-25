// SPDX-License-Identifier: GPL-3.0-or-later

package windows

import (
	"net/netip"
	"strings"

	"github.com/google/uuid"

	"github.com/netdata/netdata/go/plugins/plugin/framework/vnodes"
)

func canonicalEndpoint(host string) string {
	if addr, err := netip.ParseAddr(host); err == nil {
		return addr.Unmap().String()
	}
	return strings.ToLower(strings.TrimSuffix(host, "."))
}

func newVirtualNode(endpoint string) *vnodes.VirtualNode {
	host := canonicalEndpoint(endpoint)
	return &vnodes.VirtualNode{
		GUID:     uuid.NewSHA1(uuid.NameSpaceDNS, []byte("wmi:"+host)).String(),
		Hostname: host,
		Labels:   map[string]string{"_vnode_type": "wmi", "_os": "windows", "address": endpoint},
	}
}
