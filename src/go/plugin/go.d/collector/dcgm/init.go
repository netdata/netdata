// SPDX-License-Identifier: GPL-3.0-or-later

package dcgm

import (
	"errors"
	"fmt"

	"github.com/netdata/netdata/go/plugins/pkg/prometheus"
	"github.com/netdata/netdata/go/plugins/pkg/web"
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

	return prometheus.New(httpClient, req), nil
}
