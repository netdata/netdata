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

func buildBridgePortAliasIndex(attachments []model.Attachment) bridgePortAliasIndex {
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
		key := bridgeBasePortAliasKey(deviceID, basePort)
		if _, ambiguous := index.ambiguous[key]; ambiguous {
			continue
		}
		if existing := index.ifIndexByBasePort[key]; existing > 0 && existing != attachment.IfIndex {
			delete(index.ifIndexByBasePort, key)
			index.ambiguous[key] = struct{}{}
			continue
		}
		index.ifIndexByBasePort[key] = attachment.IfIndex
	}
	return index
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
