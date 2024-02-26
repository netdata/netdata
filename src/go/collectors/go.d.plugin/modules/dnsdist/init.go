// SPDX-License-Identifier: GPL-3.0-or-later

package dnsdist

import (
	"errors"
	"net/http"

	"github.com/netdata/netdata/go/go.d.plugin/agent/module"
	"github.com/netdata/netdata/go/go.d.plugin/pkg/web"
)

func (d DNSdist) validateConfig() error {
	if d.URL == "" {
		return errors.New("URL not set")
	}

	if _, err := web.NewHTTPRequest(d.Request); err != nil {
		return err
	}

	return nil
}

func (d DNSdist) initHTTPClient() (*http.Client, error) {
	return web.NewHTTPClient(d.Client)
}

func (d DNSdist) initCharts() (*module.Charts, error) {
	return charts.Copy(), nil
}
