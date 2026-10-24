// SPDX-License-Identifier: GPL-3.0-or-later

package projector

import (
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
		basePort := strings.TrimSpace(adjacency.SourcePortEvidence.BridgePort)
		ifIndex := adjacency.SourcePortEvidence.IfIndex
		if ifIndex <= 0 {
			ifIndex = resolveIfIndexByInterfaceName(deviceID, adjacency.SourcePortEvidence.IfName, ifIndexByDeviceName)
		}
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
