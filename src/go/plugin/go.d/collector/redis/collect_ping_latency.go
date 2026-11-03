// SPDX-License-Identifier: GPL-3.0-or-later

package redis

import (
	"context"
	"time"
)

func (c *Collector) collectPingLatency(ctx context.Context, mx map[string]int64) {
	c.pingSummary.Reset()

	for range c.PingSamples {
		start := time.Now()
		err := c.rdb.Ping(ctx).Err()
		elapsed := time.Since(start)

		if err != nil {
			c.Debug(err)
			continue
		}

		c.pingSummary.Observe(float64(elapsed.Microseconds()))
	}

	c.pingSummary.WriteTo(mx, "ping_latency", 1, 1)
}
