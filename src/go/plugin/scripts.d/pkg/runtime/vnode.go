// SPDX-License-Identifier: GPL-3.0-or-later

package runtime

import "maps"

// CloneVnodeInfo deep-copies the label map to avoid shared references.
func CloneVnodeInfo(src VnodeInfo) VnodeInfo {
	clone := src
	if len(src.Labels) > 0 {
		clone.Labels = maps.Clone(src.Labels)
	} else {
		clone.Labels = nil
	}
	return clone
}

// VnodeInfoIsEmpty reports whether the struct carries any useful data.
func VnodeInfoIsEmpty(info VnodeInfo) bool {
	if info.Hostname != "" {
		return false
	}
	if len(info.Labels) > 0 {
		return false
	}
	return true
}
