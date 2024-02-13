// SPDX-License-Identifier: GPL-3.0-or-later

package nginxvts

import (
	"errors"
	"net/http"

	"github.com/netdata/go.d.plugin/agent/module"
	"github.com/netdata/go.d.plugin/pkg/web"
)

func (vts NginxVTS) validateConfig() error {
	if vts.URL == "" {
		return errors.New("URL not set")
	}

	if _, err := web.NewHTTPRequest(vts.Request); err != nil {
		return err
	}
	return nil
}

func (vts NginxVTS) initHTTPClient() (*http.Client, error) {
	return web.NewHTTPClient(vts.Client)
}

func (vts NginxVTS) initCharts() (*module.Charts, error) {
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
