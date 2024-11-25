// SPDX-License-Identifier: GPL-3.0-or-later

package whoisquery

import (
	"errors"

	"github.com/netdata/netdata/go/plugins/plugin/go.d/agent/module"
)

func (w *WhoisQuery) validateConfig() error {
	if w.Source == "" {
		return errors.New("source is not set")
	}
	return nil
}

func (w *WhoisQuery) initProvider() (provider, error) {
	return newProvider(w.Config)
}

func (w *WhoisQuery) initCharts() *module.Charts {
	charts := baseCharts.Copy()

	for _, chart := range *charts {
		chart.Labels = []module.Label{
			{Key: "domain", Value: w.Source},
		}
	}

	return charts
}
