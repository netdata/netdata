// SPDX-License-Identifier: GPL-3.0-or-later

package vernemq

import (
	"errors"

	"context"

	"github.com/netdata/netdata/go/plugins/pkg/prometheus"
	"github.com/netdata/netdata/go/plugins/pkg/web"
)

func (c *Collector) validateConfig() error {
	if c.URL == "" {
		return errors.New("url is required but not set")
	}
	return nil
}

func (c *Collector) initPrometheusClient(ctx context.Context) (prometheus.Prometheus, error) {
	client, err := web.NewHTTPClient(ctx, c.ClientConfig, c.CredentialFiles())
	if err != nil {
		return nil, err
	}

	return prometheus.New(client, c.RequestConfig, c.CredentialFiles()), nil
}
