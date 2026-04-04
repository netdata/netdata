// SPDX-License-Identifier: GPL-3.0-or-later

package bgp

import "time"

func finalizeScrapeMetrics(mx map[string]int64, scrape *scrapeMetrics, started time.Time) map[string]int64 {
	scrape.finish(started)
	mergeMetricMaps(mx, scrape.toMap())
	return mx
}

func mergeMetricMaps(dst, src map[string]int64) {
	for k, v := range src {
		dst[k] = v
	}
}
