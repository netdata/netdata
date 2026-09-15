// SPDX-License-Identifier: GPL-3.0-or-later

package nginxvts

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
	charts := collectorapi.Charts{}

	if err := charts.Add(*mainCharts.Copy()...); err != nil {
		return nil, err
	}

	if err := charts.Add(*sharedZonesCharts.Copy()...); err != nil {
		return nil, err
	}

	if err := charts.Add(*serverZonesCharts.Copy()...); err != nil {
		return nil, err
	}

	if len(charts) == 0 {
		return nil, errors.New("zero charts")
	}
	return &charts, nil
}
