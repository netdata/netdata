// SPDX-License-Identifier: GPL-3.0-or-later

package prometheus

import (
	"errors"
	"fmt"
	"strings"

	"github.com/netdata/netdata/go/plugins/pkg/matcher"
	"github.com/netdata/netdata/go/plugins/pkg/prometheus"
	"github.com/netdata/netdata/go/plugins/pkg/web"
	"github.com/netdata/netdata/go/plugins/plugin/go.d/collector/prometheus/relabel"
)

func (c *Collector) validateConfig() error {
	if c.URL == "" {
		return errors.New("'url' can not be empty")
	}
	return nil
}

func (c *Collector) initPrometheusClient() (prometheus.Prometheus, error) {
	httpClient, err := web.NewHTTPClient(c.ClientConfig)
	if err != nil {
		return nil, fmt.Errorf("init HTTP client: %v", err)
	}

	req := c.RequestConfig.Copy()

	sr, err := c.Selector.Parse()
	if err != nil {
		return nil, fmt.Errorf("parsing selector: %v", err)
	}

	if sr != nil {
		return prometheus.NewWithSelector(httpClient, req, sr), nil
	}
	return prometheus.New(httpClient, req), nil
}

// initRelabelBlocks compiles each configured relabeling block into a name matcher
// and a validated relabel processor. Every block must declare a non-empty match,
// so its rules are always scoped to a metric-name subset (use "*" for all metrics).
func (c *Collector) initRelabelBlocks() ([]relabelBlock, error) {
	if len(c.Relabeling) == 0 {
		return nil, nil
	}

	blocks := make([]relabelBlock, 0, len(c.Relabeling))
	for i, b := range c.Relabeling {
		if strings.TrimSpace(b.Match) == "" {
			return nil, fmt.Errorf("relabeling[%d]: 'match' is required", i)
		}
		if len(b.MetricRelabelConfigs) == 0 {
			return nil, fmt.Errorf("relabeling[%d]: 'metric_relabel_configs' is empty", i)
		}

		m, err := matcher.NewSimplePatternsMatcher(b.Match)
		if err != nil {
			return nil, fmt.Errorf("relabeling[%d]: invalid 'match' (%q): %v", i, b.Match, err)
		}
		proc, err := relabel.New(b.MetricRelabelConfigs)
		if err != nil {
			return nil, fmt.Errorf("relabeling[%d]: %v", i, err)
		}

		blocks = append(blocks, relabelBlock{match: m, proc: proc})
	}

	return blocks, nil
}

func (c *Collector) initFallbackTypeMatcher(expr []string) (matcher.Matcher, error) {
	if len(expr) == 0 {
		return matcher.FALSE(), nil
	}

	m := matcher.FALSE()

	for _, pattern := range expr {
		v, err := matcher.NewGlobMatcher(pattern)
		if err != nil {
			return nil, fmt.Errorf("error on parsing pattern '%s': %v", pattern, err)
		}
		m = matcher.Or(m, v)
	}

	return m, nil
}
