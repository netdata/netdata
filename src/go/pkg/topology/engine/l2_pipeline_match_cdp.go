// SPDX-License-Identifier: GPL-3.0-or-later

package engine

import (
	"strconv"
	"strings"
)

const cdpMatchPassDefault = "default"

type cdpMatchLink struct {
	index int

	sourceDeviceID string
	sourceGlobalID string

	localInterfaceName string

	remoteDeviceID   string
	remoteDevicePort string
	remoteHost       string
	remoteAddressRaw string
}

type cdpMatchedPair struct {
	sourceIndex int
	targetIndex int
	pass        string
}

func buildCDPMatchLinks(observations []L2Observation) []cdpMatchLink {
	links := make([]cdpMatchLink, 0)
	for _, obs := range observations {
		sourceID := strings.TrimSpace(obs.DeviceID)
		if sourceID == "" {
			continue
		}

		sourceGlobalID := strings.TrimSpace(obs.Hostname)
		if sourceGlobalID == "" {
			sourceGlobalID = sourceID
		}

		remotes := sortedCDPRemotes(obs.CDPRemotes)
		for _, remote := range remotes {
			localInterfaceName := strings.TrimSpace(remote.LocalIfName)
			if localInterfaceName == "" && remote.LocalIfIndex > 0 {
				localInterfaceName = strconv.Itoa(remote.LocalIfIndex)
			}

			remoteDeviceID := strings.TrimSpace(remote.DeviceID)
			remoteHost := strings.TrimSpace(remote.SysName)
			if remoteHost == "" {
				remoteHost = remoteDeviceID
			}
			if remoteDeviceID == "" {
				remoteDeviceID = remoteHost
			}

			links = append(links, cdpMatchLink{
				index:              len(links),
				sourceDeviceID:     sourceID,
				sourceGlobalID:     sourceGlobalID,
				localInterfaceName: localInterfaceName,
				remoteDeviceID:     remoteDeviceID,
				remoteDevicePort:   strings.TrimSpace(remote.DevicePort),
				remoteHost:         remoteHost,
				remoteAddressRaw:   strings.TrimSpace(remote.Address),
			})
		}
	}
	return links
}

func buildCDPLookupMap(links []cdpMatchLink) map[string]int {
	lookup := make(map[string]int, len(links))
	for _, link := range links {
		key := topologyMatchCompositeKey(
			link.remoteDevicePort,
			link.localInterfaceName,
			link.sourceGlobalID,
			link.remoteDeviceID,
		)
		if _, ok := lookup[key]; ok {
			continue
		}
		lookup[key] = link.index
	}
	return lookup
}

func matchCDPLinksEnlinkdPassOrder(links []cdpMatchLink) []cdpMatchedPair {
	if len(links) == 0 {
		return nil
	}

	lookup := buildCDPLookupMap(links)
	parsed := make(map[int]struct{}, len(links))
	pairs := make([]cdpMatchedPair, 0, len(links)/2)

	for _, source := range links {
		if _, ok := parsed[source.index]; ok {
			continue
		}

		key := topologyMatchCompositeKey(
			source.localInterfaceName,
			source.remoteDevicePort,
			source.remoteDeviceID,
			source.sourceGlobalID,
		)
		targetIndex, ok := lookup[key]
		if !ok {
			continue
		}

		if source.index == targetIndex {
			continue
		}
		if _, targetParsed := parsed[targetIndex]; targetParsed {
			continue
		}

		parsed[source.index] = struct{}{}
		parsed[targetIndex] = struct{}{}
		pairs = append(pairs, cdpMatchedPair{
			sourceIndex: source.index,
			targetIndex: targetIndex,
			pass:        cdpMatchPassDefault,
		})
	}

	return pairs
}

func buildCDPTargetOverrides(links []cdpMatchLink, pairs []cdpMatchedPair) map[int]string {
	if len(pairs) == 0 {
		return nil
	}

	indexToLink := make(map[int]cdpMatchLink, len(links))
	for _, link := range links {
		indexToLink[link.index] = link
	}

	overrides := make(map[int]string, len(pairs)*2)
	for _, pair := range pairs {
		source, sourceOK := indexToLink[pair.sourceIndex]
		target, targetOK := indexToLink[pair.targetIndex]
		if !sourceOK || !targetOK {
			continue
		}

		overrides[source.index] = target.sourceDeviceID
		overrides[target.index] = source.sourceDeviceID
	}

	return overrides
}

func buildCDPPairMetadata(links []cdpMatchLink, pairs []cdpMatchedPair) map[int]matchedPairMetadata {
	if len(pairs) == 0 {
		return nil
	}

	indexToLink := make(map[int]cdpMatchLink, len(links))
	for _, link := range links {
		indexToLink[link.index] = link
	}

	metadata := make(map[int]matchedPairMetadata, len(pairs)*2)
	for _, pair := range pairs {
		sourceLink, sourceOK := indexToLink[pair.sourceIndex]
		targetLink, targetOK := indexToLink[pair.targetIndex]
		if !sourceOK || !targetOK {
			continue
		}

		pairID := canonicalAdjacencyPairID(
			"cdp",
			sourceLink.sourceDeviceID,
			sourceLink.localInterfaceName,
			targetLink.sourceDeviceID,
			targetLink.localInterfaceName,
		)
		if pairID == "" {
			continue
		}

		metadata[sourceLink.index] = matchedPairMetadata{
			id:   pairID,
			pass: pair.pass,
		}
		metadata[targetLink.index] = matchedPairMetadata{
			id:   pairID,
			pass: pair.pass,
		}
	}

	return metadata
}
