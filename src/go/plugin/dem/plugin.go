// SPDX-License-Identifier: GPL-3.0-or-later

// Package dem composes native DEM jobs and process-owned investigation Functions.
package dem

import (
	"github.com/netdata/netdata/go/plugins/logger"
	"github.com/netdata/netdata/go/plugins/pkg/funcapi"
	"github.com/netdata/netdata/go/plugins/plugin/dem/collector/journey"
	"github.com/netdata/netdata/go/plugins/plugin/dem/collector/lighthouse"
	"github.com/netdata/netdata/go/plugins/plugin/dem/collector/receiver"
	"github.com/netdata/netdata/go/plugins/plugin/dem/collector/rum"
	"github.com/netdata/netdata/go/plugins/plugin/dem/historyfunc"
	"github.com/netdata/netdata/go/plugins/plugin/dem/journal"
	"github.com/netdata/netdata/go/plugins/plugin/dem/rum/geoip"
	rumhistory "github.com/netdata/netdata/go/plugins/plugin/dem/rum/history"
	rumquery "github.com/netdata/netdata/go/plugins/plugin/dem/rum/query"
	rumregistry "github.com/netdata/netdata/go/plugins/plugin/dem/rum/registry"
	"github.com/netdata/netdata/go/plugins/plugin/dem/rum/rumfunc"
	"github.com/netdata/netdata/go/plugins/plugin/dem/synthetic/artifacts"
	synthetichistory "github.com/netdata/netdata/go/plugins/plugin/dem/synthetic/history"
	syntheticquery "github.com/netdata/netdata/go/plugins/plugin/dem/synthetic/query"
	syntheticregistry "github.com/netdata/netdata/go/plugins/plugin/dem/synthetic/registry"
	"github.com/netdata/netdata/go/plugins/plugin/dem/synthetic/syntheticfunc"
	"github.com/netdata/netdata/go/plugins/plugin/framework/collectorapi"
)

// Dependencies are the process-owned resources the components borrow. History
// and Artifacts are required; the command keeps them open across Agent run
// generations and closes them after the components stop.
type Dependencies struct {
	GeoIPPaths geoip.Paths
	History    *journal.Store
	Artifacts  *artifacts.Store
	Executor   journey.Executor
	// BrowserActivation names the process browser runtime that synthetic jobs
	// wait for; nil when the runtime mode has no readiness owner (preloads).
	BrowserActivation func(map[string]any) ([]collectorapi.ActivationDependency, error)
	ConfigProvider    func() (Config, error)
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
	return compose(deps, cfg, synthetichistory.NewStore(deps.History))
}

func compose(deps Dependencies, cfg Config, syntheticHistory *synthetichistory.Store) Components {
	rumRegistry := rumregistry.New()
	rumHistory := rumhistory.NewStore(deps.History)
	rumQueries := rumquery.New(rumRegistry, rumHistory)
	syntheticRegistry := syntheticregistry.New()
	collectors := collectorapi.Registry{}
	collectors.Register("receiver", receiver.Creator(receiver.Dependencies{
		Registry:   rumRegistry,
		GeoIPPaths: deps.GeoIPPaths,
	}))
	collectors.Register("rum", rum.Creator(rum.Dependencies{
		Registry: rumRegistry,
		History:  rumHistory,
	}))
	collectors.Register("journey", journey.Creator(journey.Dependencies{
		Registry:               syntheticRegistry,
		Executor:               deps.Executor,
		ActivationDependencies: deps.BrowserActivation,
	}))
	collectors.Register("lighthouse", lighthouse.Creator(lighthouse.Dependencies{
		Registry:               syntheticRegistry,
		Executor:               deps.Executor,
		ActivationDependencies: deps.BrowserActivation,
	}))
	retention := &Retention{
		store:          deps.History,
		policy:         cfg.History,
		artifactStore:  deps.Artifacts,
		artifactPolicy: cfg.Artifacts,
		loadConfig:     deps.ConfigProvider,
		log:            logger.New(),
	}
	syntheticQueries := syntheticquery.New(syntheticRegistry, syntheticHistory, deps.Artifacts, retention.PolicyError)
	return Components{
		Collectors: collectors,
		Functions: []funcapi.ProcessFunctionProvider{
			{
				ID: "rum",
				Functions: func() []funcapi.FunctionConfig {
					return availableFunctions(rumfunc.Declarations(), func() bool {
						return rumRegistry.HasSites() || deps.History.HasObservedKind("rum")
					})
				},
				NewHandlerWithContext: func(ctx funcapi.ProcessFunctionContext) funcapi.MethodHandler {
					return rumfunc.New(rumQueries, ctx.LookupVnodeGUID)
				},
			},
			{
				ID: "synthetics",
				Functions: func() []funcapi.FunctionConfig {
					return availableFunctions(syntheticfunc.Declarations(), func() bool {
						return syntheticRegistry.HasJobs() || deps.History.HasObservedKind("synthetic") ||
							deps.Artifacts.HasObservedPublished()
					})
				},
				NewHandlerWithContext: func(ctx funcapi.ProcessFunctionContext) funcapi.MethodHandler {
					return syntheticfunc.New(syntheticQueries, ctx.LookupVnodeGUID)
				},
			},
			{
				ID:         "dem-history",
				Functions:  historyfunc.Declarations,
				NewHandler: func() funcapi.MethodHandler { return historyfunc.New(deps.History, retention.PolicyError) },
			},
		},
		Retention: retention,
	}
}

func availableFunctions(methods []funcapi.FunctionConfig, available func() bool) []funcapi.FunctionConfig {
	for i := range methods {
		methods[i].Available = available
	}
	return methods
}
