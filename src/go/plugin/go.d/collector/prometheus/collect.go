// SPDX-License-Identifier: GPL-3.0-or-later

package prometheus

import (
	"context"
	"fmt"
	"strings"

	prompkg "github.com/netdata/netdata/go/plugins/pkg/prometheus/promscrapemodel"

	"github.com/netdata/netdata/go/plugins/pkg/metrix"
)

func (c *Collector) check() (int, error) {
	mfs, runtime, err := c.scrapeMetricFamiliesWithRuntime(true)
	if err != nil {
		return 0, err
	}
	c.runtime = runtime

	if mfs.Len() == 0 {
		c.Warningf("endpoint '%s' returned 0 metric families", c.URL)
		return 0, nil
	}

	store := metrix.NewCollectorStore()
	mw := newMetricFamilyWriter(store, metricFamilyWriterPolicy{
		maxTSPerMetric:        c.MaxTSPerMetric,
		isFallbackTypeGauge:   c.isFallbackTypeGauge,
		isFallbackTypeCounter: c.isFallbackTypeCounter,
	}, c.Logger)

	return mw.countWritable(mfs), nil
}

func (c *Collector) collect(context.Context) error {
	mfs, err := c.scrapeMetricFamilies()
	if err != nil {
		return err
	}

	if mfs.Len() == 0 {
		c.Warningf("endpoint '%s' returned 0 metric families", c.URL)
		return nil
	}

	written := c.mw.writeMetricFamilies(mfs)
	if written == 0 {
		c.Warningf("endpoint '%s' produced 0 writable metric series", c.URL)
	}

	return nil
}

func (c *Collector) scrapeMetricFamilies() (prompkg.MetricFamilies, error) {
	mfs, _, err := c.scrapeMetricFamiliesWithRuntime(false)
	return mfs, err
}

func (c *Collector) scrapeMetricFamiliesWithRuntime(checking bool) (prompkg.MetricFamilies, *collectorRuntime, error) {
	if c.scraper == nil {
		return nil, nil, fmt.Errorf("prometheus scraper is not initialized")
	}

	batch, err := c.scraper.collectSamples()
	if err != nil {
		return nil, nil, err
	}

	mfs, runtime, err := c.processScrapeBatch(batch, checking)
	if err != nil {
		return nil, nil, err
	}

	if c.probeState.expectedPrefix != "" {
		if !hasPrefix(mfs, c.probeState.expectedPrefix) {
			return nil, nil, fmt.Errorf("'%s' metrics have no expected prefix (%s)", c.URL, c.probeState.expectedPrefix)
		}
		c.probeState.expectedPrefix = ""
	}

	if c.probeState.maxTS > 0 {
		if n := calcMetrics(mfs); n > c.probeState.maxTS {
			return nil, nil, fmt.Errorf("'%s' num of time series (%d) > limit (%d)", c.URL, n, c.probeState.maxTS)
		}
		c.probeState.maxTS = 0
	}

	return mfs, runtime, nil
}

func (c *Collector) isFallbackTypeGauge(name string) bool {
	return c.fallbackType.gauge != nil && c.fallbackType.gauge.MatchString(name)
}

func (c *Collector) isFallbackTypeCounter(name string) bool {
	return c.fallbackType.counter != nil && c.fallbackType.counter.MatchString(name)
}

func hasPrefix(mf map[string]*prompkg.MetricFamily, prefix string) bool {
	for name := range mf {
		if strings.HasPrefix(name, prefix) {
			return true
		}
	}
	return false
}

func calcMetrics(mfs prompkg.MetricFamilies) int {
	var n int
	for _, mf := range mfs {
		n += len(mf.Metrics())
	}
	return n
}
