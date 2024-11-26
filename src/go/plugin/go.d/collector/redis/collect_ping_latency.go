// SPDX-License-Identifier: GPL-3.0-or-later

package redis

import (
	"context"
	"time"
)

func (c *Collector) collectPingLatency(mx map[string]int64) {
	c.pingSummary.Reset()

	for i := 0; i < c.PingSamples; i++ {
		now := time.Now()
		_, err := c.rdb.Ping(context.Background()).Result()
		elapsed := time.Since(now)

		if err != nil {
			c.Debug(err)
			continue
		}

		c.pingSummary.Observe(float64(elapsed.Microseconds()))
	}

	c.pingSummary.WriteTo(mx, "ping_latency", 1, 1)
}
