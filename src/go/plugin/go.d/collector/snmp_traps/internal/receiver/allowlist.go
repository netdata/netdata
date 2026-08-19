// SPDX-License-Identifier: GPL-3.0-or-later

package receiver

import (
	"net/netip"
)

type allowlist struct {
	nets  []netip.Prefix
	comms map[string]struct{}
	empty bool
}

func newAllowlist(nets []netip.Prefix, communities []string) *allowlist {
	al := &allowlist{nets: nets}
	if len(communities) == 0 {
		al.empty = true
	} else {
		al.comms = make(map[string]struct{}, len(communities))
		for _, c := range communities {
			al.comms[c] = struct{}{}
		}
	}
	return al
}

func (al *allowlist) allowedSource(addr netip.Addr) bool {
	if len(al.nets) == 0 {
		return true
	}
	for _, n := range al.nets {
		if n.Contains(addr) {
			return true
		}
	}
	return false
}

func (al *allowlist) allowedCommunity(community string) bool {
	if al.empty {
		return true
	}
	_, ok := al.comms[community]
	return ok
}
