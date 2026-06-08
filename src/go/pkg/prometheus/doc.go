// SPDX-License-Identifier: GPL-3.0-or-later

// Package prometheus scrapes and parses the Prometheus text exposition format.
//
// One parse pass drives three outputs from a single [New] / [NewWithSelector]
// instance:
//
//   - [Prometheus.Scrape] assembles typed [MetricFamilies] — gauges, counters,
//     summaries, and histograms — folding _sum/_count/_bucket and quantile series
//     into their families.
//   - [Prometheus.ScrapeSeries] returns the raw [Series]: one [SeriesSample] per
//     scraped series, with labels in textparse-sorted order.
//   - [Prometheus.ScrapeStream] exposes a flat [Sample] stream before typed
//     assembly — the form a Prometheus metric-relabeling step operates on.
//
// Results are valid only until the next scrape on the same instance; buffers are
// reused across scrapes. An optional selector (see the selector subpackage)
// filters series during the parse.
package prometheus
