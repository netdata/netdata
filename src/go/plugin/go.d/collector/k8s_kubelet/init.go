// SPDX-License-Identifier: GPL-3.0-or-later

package k8s_kubelet

import (
	"errors"
	"os"

	"github.com/netdata/netdata/go/plugins/plugin/go.d/pkg/prometheus"
	"github.com/netdata/netdata/go/plugins/plugin/go.d/pkg/web"
)

func (c *Collector) validateConfig() error {
	if c.URL == "" {
		return errors.New("url not set")
	}
	return nil
}

func (c *Collector) initAuthToken() string {
	bs, err := os.ReadFile(c.TokenPath)
	if err != nil {
		c.Warningf("error on reading service account token from '%s': %v", c.TokenPath, err)
	}
	return string(bs)
}

func (c *Collector) initPrometheusClient() (prometheus.Prometheus, error) {
	httpClient, err := web.NewHTTPClient(c.ClientConfig)
	if err != nil {
		return nil, err
	}

	return prometheus.New(httpClient, c.RequestConfig), nil
}
