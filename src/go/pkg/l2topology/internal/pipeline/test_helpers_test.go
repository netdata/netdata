// SPDX-License-Identifier: GPL-3.0-or-later

package pipeline

import "net/netip"

func addressStrings(addresses []netip.Addr) []string {
	if len(addresses) == 0 {
		return nil
	}
	out := make([]string, 0, len(addresses))
	for _, addr := range addresses {
		if !addr.IsValid() {
			continue
		}
		out = append(out, addr.Unmap().String())
	}
	if len(out) == 0 {
		return nil
	}
	return out
}
