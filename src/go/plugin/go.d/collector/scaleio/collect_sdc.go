// SPDX-License-Identifier: GPL-3.0-or-later

package scaleio

import "github.com/netdata/netdata/go/plugins/plugin/go.d/collector/scaleio/client"

func (c *Collector) collectSdc(ss map[string]client.SdcStatistics) map[string]sdcMetrics {
	ms := make(map[string]sdcMetrics, len(ss))

	for id, stats := range ss {
		sdc, ok := c.discovered.sdc[id]
		if !ok {
			continue
		}
		var m sdcMetrics
		m.BW.set(
			calcBW(stats.UserDataReadBwc),
			calcBW(stats.UserDataWriteBwc),
		)
		m.IOPS.set(
			calcIOPS(stats.UserDataReadBwc),
			calcIOPS(stats.UserDataWriteBwc),
		)
		m.IOSize.set(
			calcIOSize(stats.UserDataReadBwc),
			calcIOSize(stats.UserDataWriteBwc),
		)
		m.MappedVolumes = stats.NumOfMappedVolumes
		m.MDMConnectionState = isSdcConnected(sdc.MdmConnectionState)

		ms[id] = m
	}
	return ms
}

func isSdcConnected(state string) bool {
	return state == "Connected"
}
