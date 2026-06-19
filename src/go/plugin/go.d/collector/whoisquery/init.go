// SPDX-License-Identifier: GPL-3.0-or-later

package whoisquery

import (
	"errors"

	"github.com/netdata/netdata/go/plugins/plugin/framework/collectorapi"
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

func (c *Collector) initCharts() *collectorapi.Charts {
	charts := baseCharts.Copy()

	for _, chart := range *charts {
		chart.Labels = []collectorapi.Label{
			{Key: "domain", Value: c.Source},
		}
	}

	return charts
}
