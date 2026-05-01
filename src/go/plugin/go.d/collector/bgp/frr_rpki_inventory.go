// SPDX-License-Identifier: GPL-3.0-or-later

package bgp

import (
	"encoding/json"
	"fmt"
)

func (c *Collector) collectFRRRPKIInventory(client frrClientAPI, scrape *scrapeMetrics) []rpkiInventoryStats {
	data, err := client.RPKIPrefixCount()
	if err != nil {
		if isUnsupportedProbeError(err) {
			c.Debugf("collect FRR RPKI prefix inventory: %v", err)
			return nil
		}
		scrape.noteQueryError(err, false)
		c.Debugf("collect FRR RPKI prefix inventory: %v", err)
		return nil
	}

	stats, err := buildFRRRPKIInventory(data)
	if err != nil {
		scrape.noteParseError(false)
		c.Debugf("parse FRR RPKI prefix inventory: %v", err)
		return nil
	}

	return []rpkiInventoryStats{stats}
}

func buildFRRRPKIInventory(data []byte) (rpkiInventoryStats, error) {
	raw, err := parseFRRRPKIPrefixCount(data)
	if err != nil {
		return rpkiInventoryStats{}, err
	}

	return rpkiInventoryStats{
		ID:         "daemon",
		Backend:    backendFRR,
		Scope:      "daemon",
		PrefixIPv4: raw.IPv4PrefixCount,
		PrefixIPv6: raw.IPv6PrefixCount,
	}, nil
}

func parseFRRRPKIPrefixCount(data []byte) (frrRPKIPrefixCountReply, error) {
	var reply frrRPKIPrefixCountReply
	if err := json.Unmarshal(data, &reply); err != nil {
		return reply, fmt.Errorf("unmarshal FRR RPKI prefix count: %w", err)
	}
	return reply, nil
}
