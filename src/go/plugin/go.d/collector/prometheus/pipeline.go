// SPDX-License-Identifier: GPL-3.0-or-later

package prometheus

import (
	prompkg "github.com/netdata/netdata/go/plugins/pkg/prometheus"
	"github.com/netdata/netdata/go/plugins/pkg/prometheus/promscrapemodel"
)

type pipeline struct {
	prom prompkg.Prometheus
}

type helpEntry struct {
	name string
	help string
}

type scrapeBatch struct {
	help    []helpEntry
	samples promscrapemodel.Samples
}

func newPipeline(prom prompkg.Prometheus) *pipeline {
	return &pipeline{prom: prom}
}

func (p *pipeline) CollectSamples() (*scrapeBatch, error) {
	batch := &scrapeBatch{}

	err := p.prom.ScrapeStreamWithMeta(
		func(name, help string) {
			batch.help = append(batch.help, helpEntry{name: name, help: help})
		},
		func(sample promscrapemodel.Sample) error {
			batch.samples.Add(sample)
			return nil
		},
	)
	if err != nil {
		return nil, err
	}

	return batch, nil
}

func (p *pipeline) CollectMetricFamilies() (promscrapemodel.MetricFamilies, error) {
	batch, err := p.CollectSamples()
	if err != nil {
		return nil, err
	}

	return assembleMetricFamilies(batch)
}

func assembleMetricFamilies(batch *scrapeBatch) (promscrapemodel.MetricFamilies, error) {
	asm := promscrapemodel.NewAssembler()
	for _, entry := range batch.help {
		asm.ApplyHelp(entry.name, entry.help)
	}
	for _, sample := range batch.samples {
		if err := asm.ApplySample(sample); err != nil {
			return nil, err
		}
	}
	return asm.MetricFamilies(), nil
}
