// SPDX-License-Identifier: GPL-3.0-or-later

package prometheus

import (
	"errors"
	"fmt"

	"github.com/netdata/netdata/go/plugins/pkg/matcher"
	"github.com/netdata/netdata/go/plugins/pkg/prometheus"
	"github.com/netdata/netdata/go/plugins/pkg/web"
)

func (c *Collector) validateConfig() error {
	if c.URL == "" {
		return errors.New("'url' can not be empty")
	}
	if len(c.SelectorGroups) > 0 {
		if !c.Selector.Empty() || len(c.LabelRelabel) > 0 || len(c.ContextRules) > 0 || len(c.DimensionRules) > 0 {
			return errors.New("'selector_groups' is mutually exclusive with 'selector', 'label_relabel', 'context_rules', and 'dimension_rules'")
		}
	}
	return nil
}

func (c *Collector) initPrometheusClient() (prometheus.Prometheus, error) {
	httpClient, err := web.NewHTTPClient(c.ClientConfig)
	if err != nil {
		return nil, fmt.Errorf("init HTTP client: %v", err)
	}

	req := c.RequestConfig.Copy()

	if len(c.SelectorGroups) == 0 {
		sr, err := c.Selector.Parse()
		if err != nil {
			return nil, fmt.Errorf("parsing selector: %v", err)
		}
		if sr != nil {
			return prometheus.NewWithSelector(httpClient, req, sr), nil
		}
	}

	return prometheus.New(httpClient, req), nil
}

func (c *Collector) initFallbackTypeMatcher(expr []string) (matcher.Matcher, error) {
	if len(expr) == 0 {
		return nil, nil
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
