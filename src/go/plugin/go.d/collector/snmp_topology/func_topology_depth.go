// SPDX-License-Identifier: GPL-3.0-or-later

package snmptopology

import (
	"strconv"
	"strings"
)

func normalizeTopologyDepth(v string) int {
	value := strings.ToLower(strings.TrimSpace(v))
	if value == "" || value == topologyDepthAll {
		return topologyDepthAllInternal
	}
	depth, err := strconv.Atoi(value)
	if err != nil {
		return topologyDepthAllInternal
	}
	if depth < topologyDepthMin {
		return topologyDepthMin
	}
	if depth > topologyDepthMax {
		return topologyDepthMax
	}
	return depth
}

func isTopologyMapTypeProbable(v string) bool {
	switch strings.ToLower(strings.TrimSpace(v)) {
	case "", topologyMapTypeAllDevicesLowConfidence:
		return true
	default:
		return false
	}
}
