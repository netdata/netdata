// SPDX-License-Identifier: GPL-3.0-or-later

package topologyutil

import "strings"

// FirstBGPIdentifier treats blank process/VRF values as absent, preserving every
// byte of a nonblank identifier. Defaults belong to the caller's identity domain.
func FirstBGPIdentifier(values ...string) string {
	for _, value := range values {
		if strings.TrimSpace(value) != "" {
			return value
		}
	}
	return ""
}
