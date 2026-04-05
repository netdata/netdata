// SPDX-License-Identifier: GPL-3.0-or-later

package snmptopology

import (
	"strings"

	topologyengine "github.com/netdata/netdata/go/plugins/pkg/topology/engine"
)

func compareCollapseActorPriority(left, right topologyActor) int {
	if leftDevice, rightDevice := topologyengine.IsDeviceActorType(left.ActorType), topologyengine.IsDeviceActorType(right.ActorType); leftDevice != rightDevice {
		if leftDevice {
			return -1
		}
		return 1
	}
	if leftInferred, rightInferred := topologyActorIsInferred(left), topologyActorIsInferred(right); leftInferred != rightInferred {
		if !leftInferred {
			return -1
		}
		return 1
	}
	leftID := strings.ToLower(strings.TrimSpace(left.ActorID))
	rightID := strings.ToLower(strings.TrimSpace(right.ActorID))
	if (leftID == "") != (rightID == "") {
		if leftID != "" {
			return -1
		}
		return 1
	}
	return strings.Compare(leftID, rightID)
}
