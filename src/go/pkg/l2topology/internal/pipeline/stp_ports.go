// SPDX-License-Identifier: GPL-3.0-or-later

package pipeline

import (
	"strconv"
	"strings"
)

func stpBridgePortFromPortID(value string) string {
	value = strings.TrimSpace(value)
	if value == "" {
		return ""
	}

	// dot1dStpPortDesignatedPort encodes priority and the bridge base-port in
	// one two-octet 802.1D port ID.
	if len(value) == 4 {
		if n, err := strconv.ParseUint(value, 16, 16); err == nil {
			if port := n & 0x0fff; port > 0 {
				return strconv.FormatUint(port, 10)
			}
			return ""
		}
	}
	if n, err := strconv.ParseUint(value, 10, 16); err == nil {
		if n > 0x0fff {
			n &= 0x0fff
		}
		if n > 0 {
			return strconv.FormatUint(n, 10)
		}
	}
	return ""
}
