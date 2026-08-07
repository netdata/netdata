// SPDX-License-Identifier: GPL-3.0-or-later

package prometheus

import "github.com/netdata/netdata/go/plugins/plugin/go.d/collector/prometheus/promprofiles"

// CollectorOption supplies a non-configuration dependency to a Collector.
type CollectorOption func(*Collector)

// WithProfileCatalog uses catalog for profile selection instead of loading the
// process-wide default catalog. It is intended for validation and tests that
// need the production collector pipeline with an explicit profile set.
func WithProfileCatalog(catalog promprofiles.Catalog) CollectorOption {
	return func(c *Collector) {
		c.loadProfileCatalog = func() (promprofiles.Catalog, error) { return catalog, nil }
	}
}

// WithPipelineDiagnosticObserver enables synchronous production-pipeline facts
// for validation and tests. It does not change user collector configuration.
func WithPipelineDiagnosticObserver(observe PipelineDiagnosticObserver) CollectorOption {
	return func(c *Collector) {
		c.pipelineObserver = observe
	}
}
