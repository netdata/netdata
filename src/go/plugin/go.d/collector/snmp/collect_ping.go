// SPDX-License-Identifier: GPL-3.0-or-later

package snmp

import "context"

func (c *Collector) collectPing(ctx context.Context, mx map[string]int64) error {
	if c.pingClient == nil {
		return nil
	}

	sample, err := c.pingClient.ProbeAndTrack(ctx, c.Hostname)
	if err != nil {
		return err
	}

	if sample.PacketsRecv == 0 {
		// do not emit metrics if no replies
		return nil
	}

	mx["ping_rtt_min"] = sample.RTT.Min.Microseconds()
	mx["ping_rtt_max"] = sample.RTT.Max.Microseconds()
	mx["ping_rtt_avg"] = sample.RTT.Avg.Microseconds()
	mx["ping_rtt_stddev"] = sample.RTT.StdDev.Microseconds()

	return nil
}
