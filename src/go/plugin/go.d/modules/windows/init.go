// SPDX-License-Identifier: GPL-3.0-or-later

package windows

import (
	"errors"

	"github.com/netdata/netdata/go/plugins/plugin/go.d/pkg/prometheus"
	"github.com/netdata/netdata/go/plugins/plugin/go.d/pkg/web"
)

func (w *Windows) validateConfig() error {
	if w.URL == "" {
		return errors.New("'url' is not set")
	}
	return nil
}

func (w *Windows) initPrometheusClient() (prometheus.Prometheus, error) {
	client, err := web.NewHTTPClient(w.Client)
	if err != nil {
		return nil, err
	}
	return prometheus.New(client, w.Request), nil
}
