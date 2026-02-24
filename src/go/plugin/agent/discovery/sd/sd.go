// SPDX-License-Identifier: GPL-3.0-or-later

package sd

import (
	"context"
	"fmt"
	"log/slog"
	"sync"

	"github.com/netdata/netdata/go/plugins/plugin/agent/discovery/sd/pipeline"
	"github.com/netdata/netdata/go/plugins/plugin/agent/internal/terminal"
	"github.com/netdata/netdata/go/plugins/plugin/framework/confgroup"
	"github.com/netdata/netdata/go/plugins/plugin/framework/dyncfg"
	"github.com/netdata/netdata/go/plugins/plugin/framework/functions"

	"github.com/netdata/netdata/go/plugins/logger"
	"github.com/netdata/netdata/go/plugins/pkg/executable"
	"github.com/netdata/netdata/go/plugins/pkg/multipath"
	"github.com/netdata/netdata/go/plugins/pkg/netdataapi"
	"github.com/netdata/netdata/go/plugins/pkg/safewriter"
)

type Config struct {
	ConfigDefaults confgroup.Registry
	ConfDir        multipath.MultiPath
	FnReg          functions.Registry
	Discoverers    Registry
}

func NewServiceDiscovery(cfg Config) (*ServiceDiscovery, error) {
	log := logger.New().With(
		slog.String("component", "service discovery"),
	)
	if cfg.Discoverers == nil {
		return nil, fmt.Errorf("service discovery discoverer registry is not configured")
	}

	d := &ServiceDiscovery{
		Logger:         log,
		confProv:       newConfFileReader(log, cfg.ConfDir),
		configDefaults: cfg.ConfigDefaults,
		fnReg:          cfg.FnReg,
		discoverers:    cfg.Discoverers,
		dyncfgApi:      dyncfg.NewResponder(netdataapi.New(safewriter.Stdout)),
		seen:           dyncfg.NewSeenCache[sdConfig](),
		exposed:        dyncfg.NewExposedCache[sdConfig](),
		dyncfgCh:       make(chan dyncfg.Function, 1),
	}
	d.newPipeline = func(config pipeline.Config) (sdPipeline, error) {
		return pipeline.New(config, d.newDiscoverersFromRegistry)
	}
	d.sdCb = &sdCallbacks{sd: d}
	d.handler = dyncfg.NewHandler(dyncfg.HandlerOpts[sdConfig]{
		Logger:    d.Logger,
		API:       d.dyncfgApi,
		Seen:      d.seen,
		Exposed:   d.exposed,
		Callbacks: d.sdCb,
		WaitKey: func(cfg sdConfig) string {
			return cfg.PipelineKey()
		},

		Path:           fmt.Sprintf(dyncfgSDPath, executable.Name),
		EnableFailCode: 422,
		JobCommands: []dyncfg.Command{
			dyncfg.CommandSchema,
			dyncfg.CommandGet,
			dyncfg.CommandEnable,
			dyncfg.CommandDisable,
			dyncfg.CommandUpdate,
			dyncfg.CommandTest,
			dyncfg.CommandUserconfig,
		},
	})

	return d, nil
}

type (
	ServiceDiscovery struct {
		*logger.Logger

		confProv confFileProvider

		configDefaults confgroup.Registry
		fnReg          functions.Registry
		discoverers    Registry
		dyncfgApi      *dyncfg.Responder
		seen           *dyncfg.SeenCache[sdConfig]
		exposed        *dyncfg.ExposedCache[sdConfig]
		handler        *dyncfg.Handler[sdConfig]
		sdCb           *sdCallbacks
		dyncfgCh       chan dyncfg.Function
		newPipeline    func(config pipeline.Config) (sdPipeline, error)

		ctx context.Context
		mgr *PipelineManager
	}
	sdPipeline interface {
		Run(ctx context.Context, in chan<- []*confgroup.Group)
	}
	confFileProvider interface {
		run(ctx context.Context)
		configs() chan confFile
	}
)

// SetDyncfgResponder allows overriding the default responder (e.g., to silence output in tests).
func (d *ServiceDiscovery) SetDyncfgResponder(api *dyncfg.Responder) {
	dyncfg.BindResponder(&d.dyncfgApi, d.handler, api)
}

func (d *ServiceDiscovery) String() string {
	return "service discovery"
}

func (d *ServiceDiscovery) Run(ctx context.Context, in chan<- []*confgroup.Group) {
	d.Info("instance is started")
	defer func() { d.unregisterDyncfgTemplates(); d.Info("instance is stopped") }()

	// Store context for dyncfg commands
	d.ctx = ctx

	// Create pipeline manager with send function that forwards to output channel
	// NOTE: Must be created BEFORE registering dyncfg templates, as dyncfg commands use mgr
	send := func(ctx context.Context, groups []*confgroup.Group) {
		select {
		case <-ctx.Done():
		case in <- groups:
		}
	}

	d.mgr = NewPipelineManager(d.Logger, d.newPipeline, send)

	// Register dyncfg templates for discoverer types
	// NOTE: Must be AFTER mgr creation, as dyncfg commands use mgr
	d.registerDyncfgTemplates(ctx)

	var wg sync.WaitGroup

	wg.Add(1)
	go func() { defer wg.Done(); d.confProv.run(ctx) }()

	wg.Add(1)
	go func() { defer wg.Done(); d.run(ctx) }()

	wg.Add(1)
	go func() { defer wg.Done(); d.mgr.RunGracePeriodCleanup(ctx) }()

	wg.Wait()

	// Cleanup all pipelines on shutdown
	d.mgr.StopAll()
}

func (d *ServiceDiscovery) run(ctx context.Context) {
	for {
		if d.handler.WaitingForDecision() {
			// Waiting for enable/disable command - only process dyncfg commands
			select {
			case <-ctx.Done():
				return
			case fn := <-d.dyncfgCh:
				d.dyncfgSeqExec(fn)
			}
		} else {
			select {
			case <-ctx.Done():
				return
			case cfg := <-d.confProv.configs():
				if cfg.source == "" {
					continue
				}
				if len(cfg.content) == 0 {
					d.removePipeline(cfg)
				} else {
					d.addPipeline(ctx, cfg)
				}
			case fn := <-d.dyncfgCh:
				d.dyncfgSeqExec(fn)
			}
		}
	}
}

func (d *ServiceDiscovery) removePipeline(conf confFile) {
	// Collect configs from this source (can't call Remove inside ForEach)
	var seenCfgs []sdConfig
	d.seen.ForEach(func(_ string, cfg sdConfig) bool {
		if cfg.Source() == conf.source {
			seenCfgs = append(seenCfgs, cfg)
		}
		return true
	})

	if len(seenCfgs) == 0 {
		return
	}

	d.Infof("removing %d config(s) from source '%s'", len(seenCfgs), conf.source)

	for _, scfg := range seenCfgs {
		// Remove from seen/exposed caches if this config is currently tracked.
		_, ok := d.handler.RemoveDiscoveredConfig(scfg)
		if !ok {
			// Not exposed or different config is exposed - skip dyncfg remove
			continue
		}

		// This was the exposed config - stop pipeline and remove from dyncfg
		if d.mgr.IsRunning(scfg.PipelineKey()) {
			d.mgr.Stop(scfg.PipelineKey())
		}

		d.handler.NotifyJobRemove(scfg)
	}
}

func (d *ServiceDiscovery) addPipeline(ctx context.Context, conf confFile) {
	// Create sdConfig directly from YAML (cleans name for dyncfg compatibility)
	sourceType := sourceTypeFromPath(conf.source)
	pipelineKey := pipelineKeyFromSource(conf.source)

	scfg, err := newSDConfigFromYAML(conf.content, conf.source, sourceType, pipelineKey)
	if err != nil {
		d.Errorf("failed to unmarshal config from '%s': %v", conf.source, err)
		return
	}

	// Check if disabled
	if disabled, _ := scfg["disabled"].(bool); disabled {
		d.Infof("pipeline '%s' is disabled in config", scfg.Name())
		return
	}

	if scfg.DiscovererType() == "" {
		d.Errorf("config '%s' has no discoverer configured", conf.source)
		return
	}

	if scfg.Name() == "" {
		d.Errorf("config '%s' has no name configured", conf.source)
		return
	}

	d.addConfig(ctx, scfg)
}

// addConfig handles adding a config with priority handling.
// This is the core logic matching jobmgr pattern.
func (d *ServiceDiscovery) addConfig(ctx context.Context, scfg sdConfig) {
	// For file sources: One file = one config. If the file previously provided a different config,
	// remove the old one first. This handles the case where a file config name changes.
	if scfg.SourceType() != confgroup.TypeDyncfg {
		d.removeOldConfigsFromSource(scfg.Source(), scfg.ExposedKey())
	}

	// Always remember discovered configs, even if they are not exposed.
	d.handler.RememberDiscoveredConfig(scfg)

	// Check if there's an existing exposed config with the same key
	entry, exists := d.exposed.LookupByKey(scfg.ExposedKey())

	if !exists {
		// No existing config - expose this one
		d.handler.AddDiscoveredConfig(scfg, dyncfg.StatusAccepted)

		d.handler.NotifyJobCreate(scfg, dyncfg.StatusAccepted)
		if terminal.IsTerminal() || d.fnReg == nil || d.dyncfgCh == nil {
			// Auto-enable in terminal mode and tests.
			// Also auto-enable when no function registry is attached, because
			// no external enable/disable commands can be delivered.
			d.autoEnableConfig(scfg)
		} else {
			// Wait for netdata to send enable/disable
			d.handler.WaitForDecision(scfg)
		}
		return
	}

	// Existing config found - apply priority rules
	sp, ep := scfg.SourceTypePriority(), entry.Cfg.SourceTypePriority()

	// Higher priority wins. If same priority and existing is running, keep existing (stability).
	if ep > sp || (ep == sp && entry.Status == dyncfg.StatusRunning) {
		d.Debugf("config '%s': keeping existing (priority: existing=%d new=%d, status=%s)",
			scfg.ExposedKey(), ep, sp, entry.Status)
		return
	}

	// New config wins - stop existing if running
	d.Infof("config '%s': replacing existing (priority: existing=%d new=%d)", scfg.ExposedKey(), ep, sp)

	if entry.Status == dyncfg.StatusRunning {
		d.mgr.Stop(entry.Cfg.PipelineKey())
	}

	// Replace in exposed cache
	d.handler.AddDiscoveredConfig(scfg, dyncfg.StatusAccepted)

	// Update dyncfg (remove old, create new with new source)
	d.handler.NotifyJobRemove(entry.Cfg)
	d.handler.NotifyJobCreate(scfg, dyncfg.StatusAccepted)

	if terminal.IsTerminal() || d.fnReg == nil || d.dyncfgCh == nil {
		d.autoEnableConfig(scfg)
	} else {
		d.handler.WaitForDecision(scfg)
	}
}

// removeOldConfigsFromSource removes configs from the same source that have a different key.
// This handles the case where a file's config name changes.
// Note: We don't stop the pipeline here - the new config will stop it when it starts via
// PipelineManager.Start (which stops any existing pipeline with the same key).
// This ensures that if the new config fails to start, the old pipeline keeps running.
func (d *ServiceDiscovery) removeOldConfigsFromSource(source, newKey string) {
	// Collect configs from this source (can't call Remove inside ForEach)
	var oldCfgs []sdConfig
	d.seen.ForEach(func(_ string, cfg sdConfig) bool {
		if cfg.Source() == source {
			oldCfgs = append(oldCfgs, cfg)
		}
		return true
	})

	for _, oldCfg := range oldCfgs {
		if oldCfg.ExposedKey() == newKey {
			continue // Same config, skip
		}

		// Different config from same source - remove from caches
		// If it was exposed, remove from exposed cache and dyncfg.
		// But DON'T stop the pipeline - let the new config's enable handle that
		if _, ok := d.handler.RemoveDiscoveredConfig(oldCfg); ok {
			d.handler.NotifyJobRemove(oldCfg)
		}
	}
}

// pipelineKeyFromSource extracts a pipeline key from a file source path.
// For now, we use the file path as key. This will be extended for dyncfg.
func pipelineKeyFromSource(source string) string {
	return source
}
