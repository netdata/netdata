// SPDX-License-Identifier: GPL-3.0-or-later

package couchdb

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
	if c.Node == "" {
		return errors.New("'node' not set")
	}
	if _, err := web.NewHTTPRequest(ctx, c.RequestConfig); err != nil {
		return err
	}
	return nil
}

func (c *Collector) initHTTPClient(ctx context.Context) (*http.Client, error) {
	return web.NewHTTPClient(ctx, c.ClientConfig)
}

func (c *Collector) initCharts() (*Charts, error) {
	charts := collectorapi.Charts{}

	if err := charts.Add(*dbActivityCharts.Copy()...); err != nil {
		return nil, err
	}
	if err := charts.Add(*httpTrafficBreakdownCharts.Copy()...); err != nil {
		return nil, err
	}
	if err := charts.Add(*serverOperationsCharts.Copy()...); err != nil {
		return nil, err
	}
	if len(c.databases) != 0 {
		dbCharts := dbSpecificCharts.Copy()

		if err := charts.Add(*dbCharts...); err != nil {
			return nil, err
		}

		for _, chart := range *dbCharts {
			for _, db := range c.databases {
				if err := chart.AddDim(&collectorapi.Dim{ID: "db_" + db + "_" + chart.ID, Name: db}); err != nil {
					return nil, err
				}
			}
		}

	}
	if err := charts.Add(*erlangStatisticsCharts.Copy()...); err != nil {
		return nil, err
	}

	if len(charts) == 0 {
		return nil, errors.New("zero charts")
	}
	return &charts, nil
}
