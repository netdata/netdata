// SPDX-License-Identifier: GPL-3.0-or-later

package vernemq

import (
	"errors"

	"github.com/netdata/netdata/go/go.d.plugin/pkg/prometheus"
	"github.com/netdata/netdata/go/go.d.plugin/pkg/web"
)

func (v *VerneMQ) validateConfig() error {
	if v.URL == "" {
		return errors.New("url is not set")
	}
	return nil
}

func (v *VerneMQ) initPrometheusClient() (prometheus.Prometheus, error) {
	client, err := web.NewHTTPClient(v.Client)
	if err != nil {
		return nil, err
	}

	return prometheus.New(client, v.Request), nil
}
