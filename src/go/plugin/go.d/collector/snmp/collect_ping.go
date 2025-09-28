// SPDX-License-Identifier: GPL-3.0-or-later

package snmp

func (c *Collector) collectPing(mx map[string]int64) error {
	if c.prober == nil {
		return nil
	}

	stats, err := c.prober.Ping(c.Hostname)
	if err != nil {
		return err
	}

	if stats.PacketsRecv == 0 {
		// do not emit metrics if no replies
		return nil
	}

	mx["ping_rtt_min"] = stats.MinRtt.Microseconds()
	mx["ping_rtt_max"] = stats.MaxRtt.Microseconds()
	mx["ping_rtt_avg"] = stats.AvgRtt.Microseconds()
	mx["ping_rtt_stddev"] = stats.StdDevRtt.Microseconds()

	return nil
}
