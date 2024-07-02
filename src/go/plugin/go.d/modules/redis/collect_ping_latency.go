// SPDX-License-Identifier: GPL-3.0-or-later

package redis

import (
	"context"
	"time"
)

func (r *Redis) collectPingLatency(mx map[string]int64) {
	r.pingSummary.Reset()

	for i := 0; i < r.PingSamples; i++ {
		now := time.Now()
		_, err := r.rdb.Ping(context.Background()).Result()
		elapsed := time.Since(now)

		if err != nil {
			r.Debug(err)
			continue
		}

		r.pingSummary.Observe(float64(elapsed.Microseconds()))
	}

	r.pingSummary.WriteTo(mx, "ping_latency", 1, 1)
}
