// SPDX-License-Identifier: GPL-3.0-or-later

package powerdns

import (
	"context"
	"errors"
	"net/http"

	"github.com/netdata/netdata/go/plugins/pkg/web"
	"github.com/netdata/netdata/go/plugins/plugin/framework/collectorapi"
)

func (c *Collector) validateConfig() error {
	if c.URL == "" {
		return errors.New("URL not set")
	}
	return nil
}

func (c *Collector) initHTTPClient(ctx context.Context) (*http.Client, error) {
	client, err := web.NewHTTPClient(ctx, c.ClientConfig)
	if err != nil {
		return nil, err
	}
	if _, err := web.NewHTTPRequest(ctx, c.RequestConfig); err != nil {
		client.CloseIdleConnections()
		return nil, err
	}
	return client, nil
}

func (c *Collector) initCharts() (*collectorapi.Charts, error) {
	return charts.Copy(), nil
}
