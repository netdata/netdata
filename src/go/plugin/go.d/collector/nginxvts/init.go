// SPDX-License-Identifier: GPL-3.0-or-later

package nginxvts

import (
	"errors"
	"net/http"

	"github.com/netdata/netdata/go/plugins/pkg/web"
	"github.com/netdata/netdata/go/plugins/plugin/go.d/agent/module"
)

func (c *Collector) validateConfig() error {
	if c.URL == "" {
		return errors.New("URL not set")
	}

	if _, err := web.NewHTTPRequest(c.RequestConfig); err != nil {
		return err
	}
	return nil
}

func (c *Collector) initHTTPClient() (*http.Client, error) {
	return web.NewHTTPClient(c.ClientConfig)
}

func (c *Collector) initCharts() (*module.Charts, error) {
	charts := module.Charts{}

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
