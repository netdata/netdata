// SPDX-License-Identifier: GPL-3.0-or-later

package powerdns_recursor

import (
	"errors"

	"context"

	"github.com/netdata/netdata/go/plugins/pkg/web"
	"github.com/netdata/netdata/go/plugins/plugin/framework/collectorapi"
)

func (c *Collector) validateConfig() error {
	if c.URL == "" {
		return errors.New("URL not set")
	}
	return nil
}

func (c *Collector) initHTTPClient(ctx context.Context) (*web.HTTPClient, error) {
	client, err := web.NewHTTPClient(ctx, c.ClientConfig, c.CredentialFiles())
	if err != nil {
		return nil, err
	}
	if _, err := client.NewRequest(ctx, c.RequestConfig); err != nil {
		client.CloseIdleConnections()
		return nil, err
	}
	return client, nil
}

func (c *Collector) initCharts() (*collectorapi.Charts, error) {
	return charts.Copy(), nil
}
