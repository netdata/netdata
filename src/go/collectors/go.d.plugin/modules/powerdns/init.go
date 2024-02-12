// SPDX-License-Identifier: GPL-3.0-or-later

package powerdns

import (
	"errors"
	"net/http"

	"github.com/netdata/go.d.plugin/agent/module"
	"github.com/netdata/go.d.plugin/pkg/web"
)

func (ns AuthoritativeNS) validateConfig() error {
	if ns.URL == "" {
		return errors.New("URL not set")
	}
	if _, err := web.NewHTTPRequest(ns.Request); err != nil {
		return err
	}
	return nil
}

func (ns AuthoritativeNS) initHTTPClient() (*http.Client, error) {
	return web.NewHTTPClient(ns.Client)
}

func (ns AuthoritativeNS) initCharts() (*module.Charts, error) {
	return charts.Copy(), nil
}
