// SPDX-License-Identifier: GPL-3.0-or-later

package ceph

import (
	"fmt"

	"github.com/netdata/netdata/go/plugins/pkg/web"
)

func (c *Collector) collectOsds(mx map[string]int64) error {
	req, err := web.NewHTTPRequestWithPath(c.RequestConfig, urlPathApiOsd)
	if err != nil {
		return err
	}

	req.Header.Set("Accept", hdrAcceptVersion)
	req.Header.Set("Content-Type", hdrContentTypeJson)
	req.Header.Set("Authorization", "Bearer "+c.token)

	var osds []apiOsdResponse

	if err := c.webClient().RequestJSON(req, &osds); err != nil {
		return err
	}

	seen := make(map[string]bool)

	for _, osd := range osds {
		px := fmt.Sprintf("osd_%s_", osd.UUID)

		seen[osd.UUID] = true
		if !c.seenOsds[osd.UUID] {
			c.seenOsds[osd.UUID] = true
			c.addOsdCharts(osd.UUID, osd.Tree.DeviceClass, osd.Tree.Name)
		}

		mx[px+"status_up"], mx[px+"status_down"] = 1, 0
		if osd.Up == 0 {
			mx[px+"status_up"], mx[px+"status_down"] = 0, 1
		}
		mx[px+"status_in"], mx[px+"status_out"] = 1, 0
		if osd.In == 0 {
			mx[px+"status_in"], mx[px+"status_out"] = 0, 1
		}

		mx[px+"size_bytes"] = osd.OsdStats.Statfs.Total
		mx[px+"space_used_bytes"] = osd.OsdStats.Statfs.Total - osd.OsdStats.Statfs.Available
		mx[px+"space_avail_bytes"] = osd.OsdStats.Statfs.Available
		mx[px+"read_ops"] = int64(osd.Stats.OpR)
		mx[px+"read_bytes"] = int64(osd.Stats.OpOutBytes)
		mx[px+"write_ops"] = int64(osd.Stats.OpW)
		mx[px+"written_bytes"] = int64(osd.Stats.OpInBytes)
		mx[px+"commit_latency_ms"] = int64(osd.OsdStats.PerfStat.CommitLatencyMs)
		mx[px+"apply_latency_ms"] = int64(osd.OsdStats.PerfStat.ApplyLatencyMs)
	}

	for uuid := range c.seenOsds {
		if !seen[uuid] {
			delete(c.seenOsds, uuid)
			c.removeCharts(fmt.Sprintf("osd_%s_", uuid))
		}
	}

	return nil
}
