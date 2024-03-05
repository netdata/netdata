// SPDX-License-Identifier: GPL-3.0-or-later

package powerdns_recursor

import (
	"errors"
	"net/http"

	"github.com/netdata/netdata/go/go.d.plugin/agent/module"
	"github.com/netdata/netdata/go/go.d.plugin/pkg/web"
)

func (r *Recursor) validateConfig() error {
	if r.URL == "" {
		return errors.New("URL not set")
	}
	if _, err := web.NewHTTPRequest(r.Request); err != nil {
		return err
	}
	return nil
}

func (r *Recursor) initHTTPClient() (*http.Client, error) {
	return web.NewHTTPClient(r.Client)
}

func (r *Recursor) initCharts() (*module.Charts, error) {
	return charts.Copy(), nil
}
