// SPDX-License-Identifier: GPL-3.0-or-later

// Package dem composes native DEM jobs and process-owned investigation Functions.
package dem

import (
	"github.com/netdata/netdata/go/plugins/logger"
	"github.com/netdata/netdata/go/plugins/pkg/funcapi"
	"github.com/netdata/netdata/go/plugins/plugin/dem/artifacts"
	"github.com/netdata/netdata/go/plugins/plugin/dem/collector/journey"
	"github.com/netdata/netdata/go/plugins/plugin/dem/collector/lighthouse"
	"github.com/netdata/netdata/go/plugins/plugin/dem/collector/receiver"
	"github.com/netdata/netdata/go/plugins/plugin/dem/collector/rum"
	"github.com/netdata/netdata/go/plugins/plugin/dem/rumfunc"
	"github.com/netdata/netdata/go/plugins/plugin/dem/runtimehub"
	"github.com/netdata/netdata/go/plugins/plugin/dem/store"
	"github.com/netdata/netdata/go/plugins/plugin/dem/synthetic"
	"github.com/netdata/netdata/go/plugins/plugin/dem/syntheticfunc"
	"github.com/netdata/netdata/go/plugins/plugin/framework/collectorapi"
)

type Dependencies struct {
	History        *store.Store
	Artifacts      *artifacts.Store
	Executor       synthetic.Executor
	ConfigProvider func() (Config, error)
}

// NewRegistry keeps the shared store caller-owned across Agent run generations.
// The hub contains admitted runtimes only; configuration stays in the framework.
func NewRegistry(deps Dependencies, cfg Config) (collectorapi.Registry, *Retention) {
	hub := runtimehub.New()
	source := &source{
		hub:     hub,
		history: deps.History,
	}
	handler := rumfunc.New(source)
	site := rum.Creator(rum.Dependencies{
		Hub:     hub,
		History: deps.History,
	})
	site.AgentFunctions = rumfunc.Declarations
	site.MethodHandler = func(collectorapi.RuntimeJob) funcapi.MethodHandler { return handler }
	registry := collectorapi.Registry{}
	registry.Register("receiver", receiver.Creator(hub))
	registry.Register("rum", site)
	syntheticHub := synthetic.NewHub()
	syntheticHandler := syntheticfunc.New(&syntheticSource{hub: syntheticHub, history: deps.History, artifacts: deps.Artifacts})
	workflow := journey.Creator(journey.Dependencies{Hub: syntheticHub, Executor: deps.Executor})
	workflow.AgentFunctions = syntheticfunc.Declarations
	workflow.MethodHandler = func(collectorapi.RuntimeJob) funcapi.MethodHandler { return syntheticHandler }
	registry.Register("journey", workflow)
	registry.Register("lighthouse", lighthouse.Creator(lighthouse.Dependencies{Hub: syntheticHub, Executor: deps.Executor}))
	retention := &Retention{
		store:  deps.History,
		policy: cfg.History,

		artifactPolicy: cfg.Artifacts,
		loadConfig:     deps.ConfigProvider,
		log:            logger.New(),
	}
	if deps.Artifacts != nil {
		retention.artifactStore = deps.Artifacts
	}
	return registry, retention
}
