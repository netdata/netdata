// SPDX-License-Identifier: GPL-3.0-or-later

package x509check

import (
	"errors"

	"github.com/netdata/netdata/go/plugins/plugin/go.d/agent/module"
)

func (x *X509Check) validateConfig() error {
	if x.Source == "" {
		return errors.New("source is not set")
	}
	return nil
}

func (x *X509Check) initProvider() (provider, error) {
	return newProvider(x.Config)
}

func (x *X509Check) initCharts() *module.Charts {
	var charts *module.Charts
	if x.CheckRevocation {
		charts = withRevocationCharts.Copy()
	} else {
		charts = baseCharts.Copy()
	}

	for _, chart := range *charts {
		chart.Labels = []module.Label{
			{Key: "source", Value: x.Source},
		}
	}

	return charts

}
