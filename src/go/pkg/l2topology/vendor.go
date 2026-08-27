// SPDX-License-Identifier: GPL-3.0-or-later

package l2topology

import "github.com/netdata/netdata/go/plugins/pkg/l2topology/internal/oui"

// LookupVendorByMAC exposes the graph kernel's embedded OUI lookup so callers
// can wrap and replay the exact sequence of vendor decisions.
func LookupVendorByMAC(mac string) (vendor string, prefix string) {
	return oui.LookupVendorByMAC(mac)
}
