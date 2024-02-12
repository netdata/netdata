// SPDX-License-Identifier: GPL-3.0-or-later

package energid

import (
	"errors"
	"net/http"

	"github.com/netdata/go.d.plugin/agent/module"
	"github.com/netdata/go.d.plugin/pkg/web"
)

func (e Energid) validateConfig() error {
	if e.URL == "" {
		return errors.New("URL not set")
	}

	if _, err := web.NewHTTPRequest(e.Request); err != nil {
		return err
	}

	return nil
}

func (e Energid) initHTTPClient() (*http.Client, error) {
	return web.NewHTTPClient(e.Client)
}

func (e Energid) initCharts() (*module.Charts, error) {
	return charts.Copy(), nil
}
