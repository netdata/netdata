// SPDX-License-Identifier: GPL-3.0-or-later

// Package synthetic implements the journey and lighthouse collector modules. Each module keeps its own configuration,
// form and request building; both share the synthetic job, one chart template and one integration metadata file.
package synthetic

import (
	_ "embed"

	"github.com/netdata/netdata/go/plugins/plugin/dem/collector/internal/syntheticjob"
	"github.com/netdata/netdata/go/plugins/plugin/framework/collectorapi"
)

//go:embed charts.yaml
var charts string

// Executor and Dependencies name the shared synthetic job types for callers outside the collector tree.
type (
	Executor     = syntheticjob.Executor
	Dependencies = syntheticjob.Dependencies
)

// newCreator holds the registration policy both modules share: charts refresh every 10 seconds by default, the store
// is published first, and jobs wait for the process browser runtime.
func newCreator(
	deps Dependencies,
	schema string,
	config func() any,
	create func() collectorapi.CollectorV2,
) collectorapi.Creator {
	return collectorapi.Creator{
		Defaults: collectorapi.Defaults{
			UpdateEvery: 10,
		},
		CreateV2:               create,
		Config:                 config,
		JobConfigSchema:        schema,
		StoreFirst:             true,
		ActivationDependencies: deps.ActivationDependencies,
	}
}
