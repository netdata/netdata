// SPDX-License-Identifier: GPL-3.0-or-later

package windows

import (
	"net/netip"
	"strings"

	"github.com/google/uuid"

	"github.com/netdata/netdata/go/plugins/plugin/framework/vnodes"
)

func newVirtualNode(host string) *vnodes.VirtualNode {
	if addr, err := netip.ParseAddr(host); err == nil {
		host = addr.Unmap().String()
	} else {
		host = strings.ToLower(strings.TrimSuffix(host, "."))
	}
	return &vnodes.VirtualNode{
		GUID:     uuid.NewSHA1(uuid.NameSpaceDNS, []byte("wmi:"+host)).String(),
		Hostname: host,
		Labels:   map[string]string{"_vnode_type": "wmi"},
	}
}
