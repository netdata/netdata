// SPDX-License-Identifier: GPL-3.0-or-later

package bgp

import (
	"fmt"
	"time"
)

func (c *Collector) collectGoBGPData(scrape *scrapeMetrics) ([]familyStats, []neighborStats, []vniStats, []rpkiCacheStats, []rpkiInventoryStats, error) {
	client, ok := c.client.(gobgpClientAPI)
	if !ok {
		return nil, nil, nil, nil, nil, fmt.Errorf("backend %q client does not implement GoBGP API", c.Backend)
	}

	global, err := client.GetBgp()
	if err != nil {
		scrape.noteQueryError(err, true)
		return nil, nil, nil, nil, nil, fmt.Errorf("collect GoBGP global state: %w", err)
	}

	peers, err := client.ListPeers()
	if err != nil {
		scrape.noteQueryError(err, true)
		return nil, nil, nil, nil, nil, fmt.Errorf("collect GoBGP peers: %w", err)
	}

	var rpkiCaches []rpkiCacheStats
	servers, err := client.ListRpki()
	if err != nil {
		scrape.noteQueryError(err, false)
		c.Debugf("collect GoBGP RPKI servers: %v", err)
	} else {
		rpkiCaches = buildGoBGPRPKICaches(time.Now(), servers)
	}

	families, neighbors := buildGoBGPMetrics(global, peers)
	c.applyGoBGPSummaries(client, families, c.selectFamilies(families), scrape)
	return families, neighbors, nil, rpkiCaches, nil, nil
}
