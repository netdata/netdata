// SPDX-License-Identifier: GPL-3.0-or-later

package powerdns

import (
	"errors"
	"net/http"

	"github.com/netdata/netdata/go/plugins/plugin/go.d/agent/module"
	"github.com/netdata/netdata/go/plugins/plugin/go.d/pkg/web"
)

func (ns *AuthoritativeNS) validateConfig() error {
	if ns.URL == "" {
		return errors.New("URL not set")
	}
	if _, err := web.NewHTTPRequest(ns.RequestConfig); err != nil {
		return err
	}
	return nil
}

func (ns *AuthoritativeNS) initHTTPClient() (*http.Client, error) {
	return web.NewHTTPClient(ns.ClientConfig)
}

func (ns *AuthoritativeNS) initCharts() (*module.Charts, error) {
	return charts.Copy(), nil
}
