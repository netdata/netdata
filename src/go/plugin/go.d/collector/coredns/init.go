// SPDX-License-Identifier: GPL-3.0-or-later

package coredns

import (
	"errors"

	"github.com/netdata/netdata/go/plugins/pkg/matcher"
	"github.com/netdata/netdata/go/plugins/plugin/go.d/pkg/prometheus"
	"github.com/netdata/netdata/go/plugins/plugin/go.d/pkg/web"
)

func (c *Collector) validateConfig() error {
	if c.URL == "" {
		return errors.New("url not set")
	}
	return nil
}

func (c *Collector) initPerServerMatcher() (matcher.Matcher, error) {
	if c.PerServerStats.Empty() {
		return nil, nil
	}
	return c.PerServerStats.Parse()
}

func (c *Collector) initPerZoneMatcher() (matcher.Matcher, error) {
	if c.PerZoneStats.Empty() {
		return nil, nil
	}
	return c.PerZoneStats.Parse()
}

func (c *Collector) initPrometheusClient() (prometheus.Prometheus, error) {
	client, err := web.NewHTTPClient(c.ClientConfig)
	if err != nil {
		return nil, err
	}
	return prometheus.New(client, c.RequestConfig), nil
}
