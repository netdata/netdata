// SPDX-License-Identifier: GPL-3.0-or-later

package runtime

import "maps"

// CloneVnodeInfo deep-copies label/custom maps to avoid shared references.
func CloneVnodeInfo(src VnodeInfo) VnodeInfo {
	clone := src
	if len(src.Labels) > 0 {
		clone.Labels = maps.Clone(src.Labels)
	} else {
		clone.Labels = nil
	}
	if len(src.Custom) > 0 {
		clone.Custom = maps.Clone(src.Custom)
	} else {
		clone.Custom = nil
	}
	return clone
}

// VnodeInfoIsEmpty reports whether the struct carries any useful data.
func VnodeInfoIsEmpty(info VnodeInfo) bool {
	if info.Hostname != "" || info.Address != "" || info.Alias != "" {
		return false
	}
	if len(info.Labels) > 0 || len(info.Custom) > 0 {
		return false
	}
	return true
}
