// SPDX-License-Identifier: GPL-3.0-or-later

package engine

import (
	_ "embed"
	"sort"
	"strings"
	"sync"

	"github.com/netdata/netdata/go/plugins/pkg/topology"
)

//go:embed mac_oui_vendors.tsv
var macOUIVendorsTSV string

type topologyOUIVendorIndex struct {
	byPrefixLen map[int]map[string]string
	prefixLens  []int
}

var (
	topologyOUIVendorsOnce  sync.Once
	topologyOUIVendorsIndex topologyOUIVendorIndex
)

func loadTopologyOUIVendorsIndex() topologyOUIVendorIndex {
	topologyOUIVendorsOnce.Do(func() {
		topologyOUIVendorsIndex = buildTopologyOUIVendorIndex(macOUIVendorsTSV)
	})
	return topologyOUIVendorsIndex
}

func buildTopologyOUIVendorIndex(tsv string) topologyOUIVendorIndex {
	byPrefixLen := make(map[int]map[string]string)
	for line := range strings.SplitSeq(tsv, "\n") {
		line = strings.TrimSpace(line)
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		prefix, vendor, ok := strings.Cut(line, "\t")
		if !ok {
			continue
		}
		prefix = strings.ToUpper(strings.TrimSpace(prefix))
		vendor = strings.TrimSpace(vendor)
		if prefix == "" || vendor == "" {
			continue
		}
		if len(prefix) < 6 || len(prefix) > 12 {
			continue
		}
		if !isHexToken(prefix) {
			continue
		}
		if byPrefixLen[len(prefix)] == nil {
			byPrefixLen[len(prefix)] = make(map[string]string)
		}
		if _, exists := byPrefixLen[len(prefix)][prefix]; exists {
			continue
		}
		byPrefixLen[len(prefix)][prefix] = vendor
	}

	prefixLens := make([]int, 0, len(byPrefixLen))
	for prefixLen := range byPrefixLen {
		prefixLens = append(prefixLens, prefixLen)
	}
	sort.Slice(prefixLens, func(i, j int) bool {
		return prefixLens[i] > prefixLens[j]
	})
	return topologyOUIVendorIndex{
		byPrefixLen: byPrefixLen,
		prefixLens:  prefixLens,
	}
}

func isHexToken(value string) bool {
	if value == "" {
		return false
	}
	for _, r := range value {
		if (r >= '0' && r <= '9') || (r >= 'A' && r <= 'F') || (r >= 'a' && r <= 'f') {
			continue
		}
		return false
	}
	return true
}

func lookupTopologyVendorByMAC(mac string) (vendor string, prefix string) {
	return lookupTopologyVendorByMACInIndex(loadTopologyOUIVendorsIndex(), mac)
}

func lookupTopologyVendorByMACInIndex(index topologyOUIVendorIndex, mac string) (vendor string, prefix string) {
	mac = normalizeMAC(mac)
	if mac == "" {
		return "", ""
	}
	hex := strings.ToUpper(strings.ReplaceAll(mac, ":", ""))
	if hex == "" {
		return "", ""
	}

	for _, prefixLen := range index.prefixLens {
		if len(hex) < prefixLen {
			continue
		}
		candidatePrefix := hex[:prefixLen]
		candidateVendor, ok := index.byPrefixLen[prefixLen][candidatePrefix]
		if !ok {
			continue
		}
		return candidateVendor, candidatePrefix
	}
	return "", ""
}

func inferTopologyVendorFromMatch(match topology.Match) (vendor string, prefix string) {
	candidates := make(map[string]struct{}, len(match.MacAddresses)+len(match.ChassisIDs))
	for _, value := range match.MacAddresses {
		if mac := normalizeMAC(value); mac != "" {
			candidates[mac] = struct{}{}
		}
	}
	for _, value := range match.ChassisIDs {
		if mac := normalizeMAC(value); mac != "" {
			candidates[mac] = struct{}{}
		}
	}
	if len(candidates) == 0 {
		return "", ""
	}

	macs := make([]string, 0, len(candidates))
	for mac := range candidates {
		macs = append(macs, mac)
	}
	sort.Strings(macs)
	for _, mac := range macs {
		if vendor, prefix := lookupTopologyVendorByMAC(mac); vendor != "" {
			return vendor, prefix
		}
	}
	return "", ""
}
