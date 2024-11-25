// SPDX-License-Identifier: GPL-3.0-or-later

package powerdns_recursor

import (
	"errors"
	"net/http"

	"github.com/netdata/netdata/go/plugins/plugin/go.d/agent/module"
	"github.com/netdata/netdata/go/plugins/plugin/go.d/pkg/web"
)

func (r *Recursor) validateConfig() error {
	if r.URL == "" {
		return errors.New("URL not set")
	}
	if _, err := web.NewHTTPRequest(r.RequestConfig); err != nil {
		return err
	}
	return nil
}

func (r *Recursor) initHTTPClient() (*http.Client, error) {
	return web.NewHTTPClient(r.ClientConfig)
}

func (r *Recursor) initCharts() (*module.Charts, error) {
	return charts.Copy(), nil
}
