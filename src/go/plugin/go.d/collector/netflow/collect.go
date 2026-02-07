// SPDX-License-Identifier: GPL-3.0-or-later

package netflow

func (c *Collector) collect() map[string]int64 {
	if c.aggregator == nil {
		return nil
	}

	totals := c.aggregator.LatestTotals()
	if totals.Duration == 0 {
		return nil
	}

	seconds := totals.Duration.Seconds()
	if seconds <= 0 {
		return nil
	}

	mx := make(map[string]int64)
	mx["netflow_bytes"] = int64(uint64(float64(totals.Bytes) / seconds))
	mx["netflow_packets"] = int64(uint64(float64(totals.Packets) / seconds))
	mx["netflow_flows"] = int64(uint64(float64(totals.Flows) / seconds))
	mx["netflow_dropped"] = int64(uint64(float64(totals.Dropped) / seconds))

	return mx
}
