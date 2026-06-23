// SPDX-License-Identifier: GPL-3.0-or-later

package keyutil

import (
	"strconv"
	"strings"
)

// Sep separates delimiter-based identifiers that some topology paths still split later.
// Opaque keys built from observation data should use OpaqueCompositeKey instead.
const Sep = "\x00"

func OpaqueCompositeKey(parts ...string) string {
	if len(parts) == 0 {
		return ""
	}

	var b strings.Builder
	for _, part := range parts {
		b.WriteString(strconv.Itoa(len(part)))
		b.WriteByte(':')
		b.WriteString(part)
	}
	return b.String()
}

func DeviceIfIndexKey(deviceID string, ifIndex int) string {
	return OpaqueCompositeKey(deviceID, strconv.Itoa(ifIndex))
}
