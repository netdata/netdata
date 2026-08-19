// SPDX-License-Identifier: GPL-3.0-or-later

package projector

import (
	"strconv"
	"strings"
	"time"

	"github.com/netdata/netdata/go/plugins/pkg/topology/graph"
)

func topologyEndpointCanonicalPortName(endpoint graph.LinkEndpoint) string {
	if name := strings.TrimSpace(endpoint.PortName); name != "" {
		return name
	}
	if name := strings.TrimSpace(endpoint.IfName); name != "" {
		return name
	}
	if name := strings.TrimSpace(endpoint.IfDescr); name != "" {
		return name
	}
	if name := strings.TrimSpace(endpoint.IfAlias); name != "" {
		return name
	}

	if endpoint.IfIndex > 0 {
		return strconv.Itoa(endpoint.IfIndex)
	}

	if portID := strings.TrimSpace(endpoint.PortID); portID != "" {
		if n, err := strconv.Atoi(strings.TrimSpace(portID)); err == nil && n > 0 {
			return strconv.Itoa(n)
		}
		return portID
	}
	if bridgePort := strings.TrimSpace(endpoint.BridgePort); bridgePort != "" {
		if n, err := strconv.Atoi(strings.TrimSpace(bridgePort)); err == nil && n > 0 {
			return strconv.Itoa(n)
		}
		return bridgePort
	}
	return ""
}

func topologyCanonicalLinkName(srcName, srcPortName, dstName, dstPortName string) string {
	srcName = strings.TrimSpace(srcName)
	if srcName == "" {
		srcName = "[unset]"
	}
	dstName = strings.TrimSpace(dstName)
	if dstName == "" {
		dstName = "[unset]"
	}
	srcPortName = strings.TrimSpace(srcPortName)
	if srcPortName == "" {
		srcPortName = "[unset]"
	}
	dstPortName = strings.TrimSpace(dstPortName)
	if dstPortName == "" {
		dstPortName = "[unset]"
	}
	return srcName + ":" + srcPortName + " -> " + dstName + ":" + dstPortName
}

func topologyTimePtr(t time.Time) *time.Time {
	if t.IsZero() {
		return nil
	}
	out := t
	return &out
}
