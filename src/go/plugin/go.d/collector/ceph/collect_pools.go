// SPDX-License-Identifier: GPL-3.0-or-later

package ceph

import (
	"fmt"

	"github.com/netdata/netdata/go/plugins/pkg/web"
)

func (c *Collector) collectPools(mx map[string]int64) error {
	req, err := web.NewHTTPRequestWithPath(c.RequestConfig, urlPathApiPool)
	if err != nil {
		return err
	}

	req.URL.RawQuery = urlQueryApiPool
	req.Header.Set("Accept", hdrAcceptVersion)
	req.Header.Set("Content-Type", hdrContentTypeJson)
	req.Header.Set("Authorization", "Bearer "+c.token)

	var pools []apiPoolResponse

	if err := c.webClient().RequestJSON(req, &pools); err != nil {
		return err
	}

	seen := make(map[string]bool)

	for _, pool := range pools {
		px := fmt.Sprintf("pool_%s_", pool.PoolName)

		seen[pool.PoolName] = true
		if !c.seenPools[pool.PoolName] {
			c.seenPools[pool.PoolName] = true
			c.addPoolCharts(pool.PoolName)
		}

		mx[px+"objects"] = int64(pool.Stats.Objects.Latest)
		mx[px+"size"] = int64(pool.Stats.AvailRaw.Latest)
		mx[px+"space_used_bytes"] = int64(pool.Stats.BytesUsed.Latest)
		mx[px+"space_avail_bytes"] = int64(pool.Stats.AvailRaw.Latest - pool.Stats.BytesUsed.Latest)
		mx[px+"space_utilization"] = int64(pool.Stats.PercentUsed.Latest * precision)
		mx[px+"read_ops"] = int64(pool.Stats.Reads.Latest)
		mx[px+"read_bytes"] = int64(pool.Stats.ReadBytes.Latest)
		mx[px+"write_ops"] = int64(pool.Stats.Writes.Latest)
		mx[px+"written_bytes"] = int64(pool.Stats.WrittenBytes.Latest)
	}

	for name := range c.seenPools {
		if !seen[name] {
			delete(c.seenPools, name)
			c.removeCharts(fmt.Sprintf("pool_%s_", name))
		}
	}

	return nil
}
