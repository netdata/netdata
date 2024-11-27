// SPDX-License-Identifier: GPL-3.0-or-later

package whoisquery

import (
	"errors"

	"github.com/netdata/netdata/go/plugins/plugin/go.d/agent/module"
)

func (c *Collector) validateConfig() error {
	if c.Source == "" {
		return errors.New("source is not set")
	}
	return nil
}

func (c *Collector) initProvider() (provider, error) {
	return newProvider(c.Config)
}

func (c *Collector) initCharts() *module.Charts {
	charts := baseCharts.Copy()

	for _, chart := range *charts {
		chart.Labels = []module.Label{
			{Key: "domain", Value: c.Source},
		}
	}

	return charts
}
