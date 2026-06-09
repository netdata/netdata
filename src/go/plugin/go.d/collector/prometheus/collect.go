// SPDX-License-Identifier: GPL-3.0-or-later

package prometheus

import (
	"context"
	"fmt"
	"strings"

	"github.com/netdata/netdata/go/plugins/pkg/prometheus"
	"github.com/netdata/netdata/go/plugins/plugin/go.d/collector/prometheus/relabel"
)

// collect scrapes the endpoint and writes the metric families to the metrix store. The
// store cycle (begin/commit) is driven by the framework around Collect, so this only
// writes observations and returns an error to abort the cycle.
func (c *Collector) collect(ctx context.Context) error {
	mfs, err := c.scrape(ctx)
	if err != nil {
		return err
	}
	c.writer.writeMetricFamilies(mfs)
	return nil
}

// check probes the endpoint and enforces the startup gates the V1 collector applied once:
// the expected-prefix guard and the total time-series limit. Unlike V1 these are read-only
// (V1 mutated Config to make them one-shot); they run only at Check, i.e. autodetection.
func (c *Collector) check(ctx context.Context) error {
	mfs, err := c.scrape(ctx)
	if err != nil {
		return err
	}
	if c.ExpectedPrefix != "" && !hasPrefix(mfs, c.ExpectedPrefix) {
		return fmt.Errorf("'%s' metrics have no expected prefix (%s)", c.URL, c.ExpectedPrefix)
	}
	if c.MaxTS > 0 {
		if n := calcMetrics(mfs); n > c.MaxTS {
			return fmt.Errorf("'%s' num of time series (%d) > limit (%d)", c.URL, n, c.MaxTS)
		}
	}
	if c.writer.countWritable(mfs) == 0 {
		return fmt.Errorf("endpoint '%s' exposes no usable metrics", c.URL)
	}
	return nil
}

// scrape fetches the endpoint and enforces the empty-scrape contract: an empty scrape is
// an error (endpoint down or exposing nothing), not silent no-data.
func (c *Collector) scrape(ctx context.Context) (prometheus.MetricFamilies, error) {
	mfs, err := c.prom.ScrapeWithTransform(ctx, c.relabelTransform)
	if err != nil {
		return nil, err
	}
	if mfs.Len() == 0 {
		return nil, fmt.Errorf("endpoint '%s' returned 0 metric families", c.URL)
	}
	return mfs, nil
}

func hasPrefix(mfs prometheus.MetricFamilies, prefix string) bool {
	for name := range mfs {
		if strings.HasPrefix(name, prefix) {
			return true
		}
	}
	return false
}

func calcMetrics(mfs prometheus.MetricFamilies) int {
	var n int
	for _, mf := range mfs {
		n += len(mf.Metrics())
	}
	return n
}

// onRelabelDrop logs why a relabel rule dropped a sample, at debug level. It logs
// the metric name and the rule outcome, never label values (cardinality/PII).
func (c *Collector) onRelabelDrop(s prometheus.Sample, d relabel.DropInfo) {
	c.When(d.RuleIndex >= 0).
		Debugf("relabel dropped metric %q: %s (rule %d, action %q)", s.Name, d.Reason, d.RuleIndex, d.Action).
		Else().
		Debugf("relabel dropped metric %q: %s", s.Name, d.Reason)
}
