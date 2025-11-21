// SPDX-License-Identifier: GPL-3.0-or-later

package couchbase

import (
	"errors"
	"net/http"

	"github.com/netdata/netdata/go/plugins/pkg/web"
	"github.com/netdata/netdata/go/plugins/plugin/go.d/agent/module"
)

func (c *Collector) initCharts() (*Charts, error) {
	var bucketCharts = module.Charts{
		bucketQuotaPercentUsedChart.Copy(),
		bucketOpsPerSecChart.Copy(),
		bucketDiskFetchesChart.Copy(),
		bucketItemCountChart.Copy(),
		bucketDiskUsedChart.Copy(),
		bucketDataUsedChart.Copy(),
		bucketMemUsedChart.Copy(),
		bucketVBActiveNumNonResidentChart.Copy(),
	}
	return bucketCharts.Copy(), nil
}

func (c *Collector) initHTTPClient() (*http.Client, error) {
	return web.NewHTTPClient(c.ClientConfig)
}

func (c *Collector) validateConfig() error {
	if c.URL == "" {
		return errors.New("URL not set")
	}
	if _, err := web.NewHTTPRequest(c.RequestConfig); err != nil {
		return err
	}
	return nil
}
