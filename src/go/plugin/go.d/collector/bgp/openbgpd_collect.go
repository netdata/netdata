// SPDX-License-Identifier: GPL-3.0-or-later

package bgp

import "fmt"

func (c *Collector) collectOpenBGPDData(scrape *scrapeMetrics) ([]familyStats, []neighborStats, []vniStats, []rpkiCacheStats, []rpkiInventoryStats, error) {
	client, ok := c.client.(openbgpdClientAPI)
	if !ok {
		return nil, nil, nil, nil, nil, fmt.Errorf("backend %q client does not implement OpenBGPD API", c.Backend)
	}

	data, err := client.Neighbors()
	if err != nil {
		scrape.noteQueryError(err, true)
		return nil, nil, nil, nil, nil, fmt.Errorf("collect neighbors: %w", err)
	}

	families, neighbors, err := parseOpenBGPDNeighbors(data)
	if err != nil {
		scrape.noteParseError(true)
		return nil, nil, nil, nil, nil, fmt.Errorf("parse neighbors: %w", err)
	}

	selectedFamilies := c.selectFamilies(families)
	if shouldCollectOpenBGPDRIBSummaries(families, selectedFamilies) {
		if summaries := c.collectOpenBGPDRIBSummaries(client, families, selectedFamilies, scrape); len(summaries) > 0 {
			applyOpenBGPDRIBSummaries(families, summaries)
		}
	}

	return families, neighbors, nil, nil, nil, nil
}

func shouldCollectOpenBGPDRIBSummaries(families []familyStats, selectedFamilies map[string]bool) bool {
	for _, family := range families {
		if !selectedFamilies[family.ID] {
			continue
		}
		if openbgpdRIBSummarySupported(family) {
			return true
		}
	}
	return false
}

func openbgpdRIBSummarySupported(family familyStats) bool {
	switch family.AFI {
	case "ipv4", "ipv6":
		switch family.SAFI {
		case "unicast", "vpn":
			return true
		default:
			return false
		}
	default:
		return false
	}
}
