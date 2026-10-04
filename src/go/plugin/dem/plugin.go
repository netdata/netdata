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

// Components separates collector selection from process-owned investigation.
type Components struct {
	Collectors collectorapi.Registry
	Functions  []funcapi.ProcessFunctionProvider
	Retention  *Retention
}

// New keeps the shared stores caller-owned across Agent run generations.
// The hub contains admitted runtimes only; configuration stays in the framework.
func New(deps Dependencies, cfg Config) Components {
	hub := runtimehub.New()
	source := &source{
		hub:     hub,
		history: deps.History,
	}
	site := rum.Creator(rum.Dependencies{
		Hub:     hub,
		History: deps.History,
	})
	registry := collectorapi.Registry{}
	registry.Register("receiver", receiver.Creator(hub))
	registry.Register("rum", site)
	syntheticHub := synthetic.NewHub()
	syntheticSource := &syntheticSource{
		hub:       syntheticHub,
		history:   deps.History,
		artifacts: deps.Artifacts,
	}
	workflow := journey.Creator(journey.Dependencies{
		Hub:      syntheticHub,
		Executor: deps.Executor,
	})
	registry.Register("journey", workflow)
	registry.Register("lighthouse", lighthouse.Creator(lighthouse.Dependencies{
		Hub:      syntheticHub,
		Executor: deps.Executor,
	}))
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
	return Components{
		Collectors: registry,
		Functions: []funcapi.ProcessFunctionProvider{
			{ID: "rum", Functions: rumfunc.Declarations, NewHandler: func() funcapi.MethodHandler { return rumfunc.New(source) }},
			{ID: "synthetics", Functions: syntheticfunc.Declarations, NewHandler: func() funcapi.MethodHandler { return syntheticfunc.New(syntheticSource) }},
		},
		Retention: retention,
	}
}
