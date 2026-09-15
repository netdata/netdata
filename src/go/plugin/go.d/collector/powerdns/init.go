// SPDX-License-Identifier: GPL-3.0-or-later

package powerdns

import (
	"context"
	"errors"
	"net/http"

	"github.com/netdata/netdata/go/plugins/pkg/web"
	"github.com/netdata/netdata/go/plugins/plugin/framework/collectorapi"
)

func (c *Collector) validateConfig(ctx context.Context) error {
	if c.URL == "" {
		return errors.New("URL not set")
	}
	if _, err := web.NewHTTPRequest(ctx, c.RequestConfig); err != nil {
		return err
	}
	return nil
}

func (c *Collector) initHTTPClient(ctx context.Context) (*http.Client, error) {
	return web.NewHTTPClient(ctx, c.ClientConfig)
}

func (c *Collector) initCharts() (*collectorapi.Charts, error) {
	return charts.Copy(), nil
}
