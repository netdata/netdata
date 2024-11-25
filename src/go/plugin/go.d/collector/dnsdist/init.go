// SPDX-License-Identifier: GPL-3.0-or-later

package dnsdist

import (
	"errors"
	"net/http"

	"github.com/netdata/netdata/go/plugins/plugin/go.d/agent/module"
	"github.com/netdata/netdata/go/plugins/plugin/go.d/pkg/web"
)

func (d *DNSdist) validateConfig() error {
	if d.URL == "" {
		return errors.New("URL not set")
	}

	if _, err := web.NewHTTPRequest(d.RequestConfig); err != nil {
		return err
	}

	return nil
}

func (d *DNSdist) initHTTPClient() (*http.Client, error) {
	return web.NewHTTPClient(d.ClientConfig)
}

func (d *DNSdist) initCharts() (*module.Charts, error) {
	return charts.Copy(), nil
}
