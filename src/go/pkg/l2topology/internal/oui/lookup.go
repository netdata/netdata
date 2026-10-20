// SPDX-License-Identifier: GPL-3.0-or-later

package oui

import (
	_ "embed"
	"sort"
	"strings"
	"sync"

	"github.com/netdata/netdata/go/plugins/pkg/l2topology/internal/addrnorm"
)

//go:embed mac_oui_vendors.tsv
var macOUIVendorsTSV string

type vendorIndex struct {
	byPrefixLen map[int]map[string]string
	prefixLens  []int
}

var (
	vendorsOnce  sync.Once
	vendorsIndex vendorIndex
)

func loadVendorsIndex() vendorIndex {
	vendorsOnce.Do(func() {
		vendorsIndex = buildVendorIndex(macOUIVendorsTSV)
	})
	return vendorsIndex
}

func buildVendorIndex(tsv string) vendorIndex {
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
	return vendorIndex{
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

func LookupVendorByMAC(mac string) (vendor string, prefix string) {
	return lookupVendorByMACInIndex(loadVendorsIndex(), mac)
}

func lookupVendorByMACInIndex(index vendorIndex, mac string) (vendor string, prefix string) {
	mac = addrnorm.NormalizeMAC(mac)
	if mac == "" {
		return "", ""
	}
	hexMAC := strings.ToUpper(strings.ReplaceAll(mac, ":", ""))
	if hexMAC == "" {
		return "", ""
	}

	for _, prefixLen := range index.prefixLens {
		if len(hexMAC) < prefixLen {
			continue
		}
		candidatePrefix := hexMAC[:prefixLen]
		candidateVendor, ok := index.byPrefixLen[prefixLen][candidatePrefix]
		if !ok {
			continue
		}
		return candidateVendor, candidatePrefix
	}
	return "", ""
}
