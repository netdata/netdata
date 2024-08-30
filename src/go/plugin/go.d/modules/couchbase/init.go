// SPDX-License-Identifier: GPL-3.0-or-later

package couchbase

import (
	"errors"
	"net/http"

	"github.com/netdata/netdata/go/plugins/plugin/go.d/agent/module"
	"github.com/netdata/netdata/go/plugins/plugin/go.d/pkg/web"
)

func (cb *Couchbase) initCharts() (*Charts, error) {
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

func (cb *Couchbase) initHTTPClient() (*http.Client, error) {
	return web.NewHTTPClient(cb.Client)
}

func (cb *Couchbase) validateConfig() error {
	if cb.URL == "" {
		return errors.New("URL not set")
	}
	if _, err := web.NewHTTPRequest(cb.Request); err != nil {
		return err
	}
	return nil
}
