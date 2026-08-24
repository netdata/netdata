// SPDX-License-Identifier: GPL-3.0-or-later

package pipeline

import (
	"strings"

	"github.com/netdata/netdata/go/plugins/pkg/l2topology/internal/model"
)

func lldpPortEvidence(portID, subtype string) model.AdjacencyPortEvidence {
	portID = strings.TrimSpace(portID)
	if portID == "" {
		return model.AdjacencyPortEvidence{}
	}
	switch strings.ToLower(strings.TrimSpace(subtype)) {
	case "5", "interfacename":
		return model.AdjacencyPortEvidence{IfName: portID}
	default:
		return model.AdjacencyPortEvidence{}
	}
}

func lldpLocalPortEvidence(link lldpMatchLink) model.AdjacencyPortEvidence {
	return lldpPortEvidence(link.localPortID, link.localPortIDSubtype)
}

func lldpRemotePortEvidence(link lldpMatchLink) model.AdjacencyPortEvidence {
	return lldpPortEvidence(link.remotePortID, link.remotePortIDSubtype)
}

func cdpLocalPortEvidence(link cdpMatchLink) model.AdjacencyPortEvidence {
	return model.AdjacencyPortEvidence{
		IfIndex: link.localIfIndex,
		IfName:  strings.TrimSpace(link.localObservedName),
	}
}

func mergeAdjacencyPortEvidence(primary, fallback model.AdjacencyPortEvidence) model.AdjacencyPortEvidence {
	if primary.IfIndex <= 0 {
		primary.IfIndex = fallback.IfIndex
	}
	if strings.TrimSpace(primary.IfName) == "" {
		primary.IfName = strings.TrimSpace(fallback.IfName)
	}
	if strings.TrimSpace(primary.BridgePort) == "" {
		primary.BridgePort = strings.TrimSpace(fallback.BridgePort)
	}
	return primary
}

func buildLLDPPairedTargetPortEvidence(links []lldpMatchLink, pairs []lldpMatchedPair) map[int]model.AdjacencyPortEvidence {
	if len(pairs) == 0 {
		return nil
	}
	overrides := make(map[int]model.AdjacencyPortEvidence, len(pairs)*2)
	for _, pair := range pairs {
		if pair.sourceIndex < 0 || pair.sourceIndex >= len(links) || pair.targetIndex < 0 || pair.targetIndex >= len(links) {
			continue
		}
		source := links[pair.sourceIndex]
		target := links[pair.targetIndex]
		overrides[source.index] = lldpLocalPortEvidence(target)
		overrides[target.index] = lldpLocalPortEvidence(source)
	}
	return overrides
}

func buildCDPPairedTargetPortEvidence(links []cdpMatchLink, pairs []cdpMatchedPair) map[int]model.AdjacencyPortEvidence {
	if len(pairs) == 0 {
		return nil
	}
	overrides := make(map[int]model.AdjacencyPortEvidence, len(pairs)*2)
	for _, pair := range pairs {
		if pair.sourceIndex < 0 || pair.sourceIndex >= len(links) || pair.targetIndex < 0 || pair.targetIndex >= len(links) {
			continue
		}
		source := links[pair.sourceIndex]
		target := links[pair.targetIndex]
		overrides[source.index] = cdpLocalPortEvidence(target)
		overrides[target.index] = cdpLocalPortEvidence(source)
	}
	return overrides
}
