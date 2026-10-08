// SPDX-License-Identifier: GPL-3.0-or-later

// Package dem composes native DEM jobs and process-owned investigation Functions.
package dem

import (
	"context"

	"github.com/netdata/netdata/go/plugins/logger"
	"github.com/netdata/netdata/go/plugins/pkg/funcapi"
	"github.com/netdata/netdata/go/plugins/plugin/dem/collector/journey"
	"github.com/netdata/netdata/go/plugins/plugin/dem/collector/lighthouse"
	"github.com/netdata/netdata/go/plugins/plugin/dem/collector/receiver"
	"github.com/netdata/netdata/go/plugins/plugin/dem/collector/rum"
	"github.com/netdata/netdata/go/plugins/plugin/dem/journal"
	rumfunctions "github.com/netdata/netdata/go/plugins/plugin/dem/rum/functions"
	"github.com/netdata/netdata/go/plugins/plugin/dem/rum/geoip"
	rumhistory "github.com/netdata/netdata/go/plugins/plugin/dem/rum/history"
	rumquery "github.com/netdata/netdata/go/plugins/plugin/dem/rum/query"
	rumregistry "github.com/netdata/netdata/go/plugins/plugin/dem/rum/registry"
	"github.com/netdata/netdata/go/plugins/plugin/dem/synthetic"
	"github.com/netdata/netdata/go/plugins/plugin/dem/synthetic/artifacts"
	syntheticfunctions "github.com/netdata/netdata/go/plugins/plugin/dem/synthetic/functions"
	synthetichistory "github.com/netdata/netdata/go/plugins/plugin/dem/synthetic/history"
	syntheticquery "github.com/netdata/netdata/go/plugins/plugin/dem/synthetic/query"
	syntheticregistry "github.com/netdata/netdata/go/plugins/plugin/dem/synthetic/registry"
	"github.com/netdata/netdata/go/plugins/plugin/framework/collectorapi"
)

// Executor is the browser capability injected into native collectors.
type Executor interface {
	Check(context.Context, synthetic.Kind) error
	Execute(context.Context, synthetic.Request, func(string)) synthetic.Execution
}

type Dependencies struct {
	GeoIPPaths     geoip.Paths
	History        *journal.Store
	Artifacts      *artifacts.Store
	Executor       Executor
	ConfigProvider func() (Config, error)
}

// Components separates collector selection from process-owned investigation.
type Components struct {
	Collectors collectorapi.Registry
	Functions  []funcapi.ProcessFunctionProvider
	Retention  *Retention
}

// New keeps the shared stores caller-owned across Agent run generations.
// The registries contain admitted runtimes only; configuration stays in the framework.
func New(deps Dependencies, cfg Config) Components {
	rumRegistry := rumregistry.New()
	var rumHistory *rumhistory.Store
	var syntheticHistory syntheticquery.History
	if deps.History != nil {
		rumHistory = rumhistory.NewStore(deps.History)
		syntheticHistory = synthetichistory.NewStore(deps.History)
	}
	rumQueries := rumquery.New(rumRegistry, rumHistory)
	site := rum.Creator(rum.Dependencies{
		Registry: rumRegistry,
		History:  rumHistory,
	})
	collectors := collectorapi.Registry{}
	collectors.Register(
		"receiver",
		receiver.Creator(receiver.Dependencies{
			Registry:   rumRegistry,
			GeoIPPaths: deps.GeoIPPaths,
		}),
	)
	collectors.Register("rum", site)
	syntheticRegistry := syntheticregistry.New()
	var captures syntheticquery.Artifacts
	if deps.Artifacts != nil {
		captures = deps.Artifacts
	}
	syntheticQueries := syntheticquery.New(syntheticRegistry, syntheticHistory, captures)
	workflow := journey.Creator(journey.Dependencies{
		Registry: syntheticRegistry,
		Executor: deps.Executor,
	})
	collectors.Register("journey", workflow)
	collectors.Register("lighthouse", lighthouse.Creator(lighthouse.Dependencies{
		Registry: syntheticRegistry,
		Executor: deps.Executor,
	}))
	retention := &Retention{
		policy:         cfg.History,
		artifactPolicy: cfg.Artifacts,
		loadConfig:     deps.ConfigProvider,
		log:            logger.New(),
	}
	if deps.History != nil {
		retention.store = deps.History
	}
	if deps.Artifacts != nil {
		retention.artifactStore = deps.Artifacts
	}
	return Components{
		Collectors: collectors,
		Functions: []funcapi.ProcessFunctionProvider{
			{
				ID:         "rum",
				Functions:  rumfunctions.Declarations,
				NewHandler: func() funcapi.MethodHandler { return rumfunctions.New(rumQueries) },
			},
			{
				ID:         "synthetics",
				Functions:  syntheticfunctions.Declarations,
				NewHandler: func() funcapi.MethodHandler { return syntheticfunctions.New(syntheticQueries) },
			},
		},
		Retention: retention,
	}
}
