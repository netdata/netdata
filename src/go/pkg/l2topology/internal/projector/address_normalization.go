// SPDX-License-Identifier: GPL-3.0-or-later

package projector

import (
	"net/netip"

	"github.com/netdata/netdata/go/plugins/pkg/l2topology/internal/addrnorm"
)

func normalizeMAC(value string) string {
	return addrnorm.NormalizeMAC(value)
}

func parseAddr(value string) netip.Addr {
	return addrnorm.ParseAddr(value)
}
