// SPDX-License-Identifier: GPL-3.0-or-later

package envoy

import (
	"errors"

	"github.com/netdata/netdata/go/go.d.plugin/pkg/prometheus"
	"github.com/netdata/netdata/go/go.d.plugin/pkg/web"
)

func (e *Envoy) validateConfig() error {
	if e.URL == "" {
		return errors.New("url not set")
	}
	return nil
}

func (e *Envoy) initPrometheusClient() (prometheus.Prometheus, error) {
	httpClient, err := web.NewHTTPClient(e.Client)
	if err != nil {
		return nil, err
	}

	return prometheus.New(httpClient, e.Request), nil
}
