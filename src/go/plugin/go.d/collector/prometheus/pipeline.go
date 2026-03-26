// SPDX-License-Identifier: GPL-3.0-or-later

package prometheus

import (
	prompkg "github.com/netdata/netdata/go/plugins/pkg/prometheus"
)

type pipeline struct {
	prom prompkg.Prometheus
}

func newPipeline(prom prompkg.Prometheus) *pipeline {
	return &pipeline{prom: prom}
}

func (p *pipeline) CollectMetricFamilies() (prompkg.MetricFamilies, error) {
	return p.prom.Scrape()
}
