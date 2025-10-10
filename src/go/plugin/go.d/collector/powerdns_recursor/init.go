// SPDX-License-Identifier: GPL-3.0-or-later

package powerdns_recursor

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
	return charts.Copy(), nil
}
