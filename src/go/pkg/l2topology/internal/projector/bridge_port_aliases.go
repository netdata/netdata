// SPDX-License-Identifier: GPL-3.0-or-later

package projector

import (
	"strconv"
	"strings"

	"github.com/netdata/netdata/go/plugins/pkg/l2topology/internal/model"
)

type bridgePortAliasIndex struct {
	ifIndexByBasePort map[string]int
	ambiguous         map[string]struct{}
}

func buildBridgePortAliasIndex(
	attachments []model.Attachment,
	adjacencies []model.Adjacency,
	ifIndexByDeviceName map[string]int,
) bridgePortAliasIndex {
	index := bridgePortAliasIndex{
		ifIndexByBasePort: make(map[string]int),
		ambiguous:         make(map[string]struct{}),
	}
	for _, attachment := range attachments {
		deviceID := strings.TrimSpace(attachment.DeviceID)
		basePort := strings.TrimSpace(attachment.Labels["bridge_port"])
		if deviceID == "" || basePort == "" || attachment.IfIndex <= 0 {
			continue
		}
		index.add(deviceID, basePort, attachment.IfIndex)
	}
	for _, adjacency := range adjacencies {
		if !strings.EqualFold(strings.TrimSpace(adjacency.Protocol), "stp") {
			continue
		}
		deviceID := strings.TrimSpace(adjacency.SourceID)
		basePort := strings.TrimSpace(adjacency.Labels["stp_port"])
		ifIndex := resolveIfIndexByInterfaceName(deviceID, adjacency.SourcePort, ifIndexByDeviceName)
		if deviceID == "" || basePort == "" || ifIndex <= 0 {
			continue
		}
		index.add(deviceID, basePort, ifIndex)
	}
	return index
}

func (i bridgePortAliasIndex) add(deviceID, basePort string, ifIndex int) {
	key := bridgeBasePortAliasKey(deviceID, basePort)
	if key == "" || ifIndex <= 0 {
		return
	}
	if _, ambiguous := i.ambiguous[key]; ambiguous {
		return
	}
	if existing := i.ifIndexByBasePort[key]; existing > 0 && existing != ifIndex {
		delete(i.ifIndexByBasePort, key)
		i.ambiguous[key] = struct{}{}
		return
	}
	i.ifIndexByBasePort[key] = ifIndex
}

func (i bridgePortAliasIndex) resolveIfIndex(deviceID, basePort string) int {
	key := bridgeBasePortAliasKey(deviceID, basePort)
	if key == "" {
		return 0
	}
	if _, ambiguous := i.ambiguous[key]; ambiguous {
		return 0
	}
	return i.ifIndexByBasePort[key]
}

func bridgeBasePortAliasKey(deviceID, basePort string) string {
	deviceID = strings.TrimSpace(deviceID)
	basePort = strings.ToLower(strings.TrimSpace(basePort))
	if deviceID == "" || basePort == "" {
		return ""
	}
	return strings.Join([]string{deviceID, "bp:" + basePort}, keySep)
}

func stpBridgePortFromPortID(value string) string {
	value = strings.TrimSpace(value)
	if value == "" {
		return ""
	}

	// BRIDGE-MIB represents dot1dStpPortDesignatedPort as a two-octet
	// 802.1D port ID: priority in the high bits and bridge port in the low 12.
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
