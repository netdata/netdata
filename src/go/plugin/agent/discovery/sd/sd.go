// SPDX-License-Identifier: GPL-3.0-or-later

package sd

import (
	"context"
	"fmt"
	"io"
	"log/slog"
	"sync"
	"time"

	"github.com/netdata/netdata/go/plugins/pkg/netdataapi"
	"github.com/netdata/netdata/go/plugins/plugin/agent/discovery/sd/pipeline"
	"github.com/netdata/netdata/go/plugins/plugin/agent/jobmgr"
	"github.com/netdata/netdata/go/plugins/plugin/agent/policy"
	"github.com/netdata/netdata/go/plugins/plugin/framework/confgroup"
	"github.com/netdata/netdata/go/plugins/plugin/framework/dyncfg"

	"github.com/netdata/netdata/go/plugins/logger"
	"github.com/netdata/netdata/go/plugins/pkg/multipath"
)

type Config struct {
	Epoch          uint64
	Attempts       jobmgr.ProcessAttemptAuthority
	ConfigDefaults confgroup.Registry
	PluginName     string
	RunModePolicy  policy.RunModePolicy
	DyncfgOutput   dyncfg.Output
	ConfDir        multipath.MultiPath
	FnReg          dyncfg.PreparedRegistry
	Discoverers    Registry
}

func NewServiceDiscovery(cfg Config) (*ServiceDiscovery, error) {
	log := logger.New().With(
		slog.String("component", "service discovery"),
	)
	if cfg.Epoch == 0 || cfg.Attempts == nil {
		return nil, fmt.Errorf("service discovery process containment is not configured")
	}
	if cfg.Discoverers == nil {
		return nil, fmt.Errorf("service discovery discoverer registry is not configured")
	}
	output := cfg.DyncfgOutput
	if output == nil {
		output = dyncfg.NewProtocolOutput(io.Discard)
	}

	d := &ServiceDiscovery{
		Logger:         log,
		epoch:          cfg.Epoch,
		attempts:       cfg.Attempts,
		confProv:       newConfFileReader(log, cfg.ConfDir),
		configDefaults: cfg.ConfigDefaults,
		pluginName:     cfg.PluginName,
		runModePolicy:  cfg.RunModePolicy,
		fnReg:          cfg.FnReg,
		discoverers:    cfg.Discoverers,
		dyncfgApi:      dyncfg.NewResponder(output),
		seen:           dyncfg.NewSeenCache[sdConfig](),
		exposed:        dyncfg.NewExposedCache[sdConfig](),
		actorCommands:  make(chan sdActorCommand),
	}
	d.newPipeline = func(config pipeline.Config) (sdPipeline, error) {
		return pipeline.New(config, d.newDiscoverersFromRegistry)
	}
	d.sdCb = &sdCallbacks{
		sd: d,
	}
	d.handler = dyncfg.NewHandler(dyncfg.HandlerOpts[sdConfig]{
		API:       d.dyncfgApi,
		Seen:      d.seen,
		Exposed:   d.exposed,
		Callbacks: d.sdCb,
		WaitKey: func(cfg sdConfig) string {
			return cfg.PipelineKey()
		},

		Path: fmt.Sprintf(dyncfgSDPath, cfg.PluginName),
		ConfigCommands: []dyncfg.Command{
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

		epoch    uint64
		attempts jobmgr.ProcessAttemptAuthority

		confProv confFileProvider

		configDefaults confgroup.Registry
		pluginName     string
		runModePolicy  policy.RunModePolicy
		fnReg          dyncfg.PreparedRegistry
		discoverers    Registry
		dyncfgApi      *dyncfg.Responder
		seen           *dyncfg.SeenCache[sdConfig]
		exposed        *dyncfg.ExposedCache[sdConfig]
		handler        *dyncfg.Handler[sdConfig]
		sdCb           *sdCallbacks
		newPipeline    func(config pipeline.Config) (sdPipeline, error)
		actorCommands  chan sdActorCommand
		output         chan<- []*confgroup.Group

		ctx context.Context
		mgr *PipelineManager
	}
	sdPipeline interface {
		Test(ctx context.Context) (fullyTested bool, err error)
		Run(ctx context.Context, in chan<- []*confgroup.Group)
	}
	confFileProvider interface {
		run(ctx context.Context)
		configs() chan confFile
	}
)

func (d *ServiceDiscovery) Run(ctx context.Context, in chan<- []*confgroup.Group) {
	d.Info("instance is started")
	defer func() { d.unregisterDyncfgTemplates(); d.Info("instance is stopped") }()

	// Store context for dyncfg commands
	d.ctx = ctx
	d.output = in

	d.mgr = NewPipelineManager(d.Logger)
	d.mgr.bind(d)

	// Register dyncfg templates for discoverer types
	// NOTE: Must be AFTER mgr creation, as dyncfg commands use mgr
	d.registerDyncfgTemplates(ctx)

	var wg sync.WaitGroup

	wg.Go(func() { d.confProv.run(ctx) })

	wg.Go(func() { d.run(ctx) })

	wg.Wait()

	// Cleanup all pipelines on shutdown
	d.mgr.StopAll()
}

func (d *ServiceDiscovery) run(ctx context.Context) {
	if d.actorCommands == nil {
		d.actorCommands = make(chan sdActorCommand)
	}
	d.mgr.bind(d)
	grace := time.NewTicker(5 * time.Second)
	defer grace.Stop()
	for {
		var configs <-chan confFile
		if !d.handler.WaitingForDecision() {
			configs = d.confProv.configs()
		}
		groups, sent := d.mgr.nextOutput()
		var output chan<- []*confgroup.Group
		if len(groups) > 0 {
			output = d.output
		}
		select {
		case <-ctx.Done():
			d.mgr.StopAll()
			return
		case request := <-d.actorCommands:
			if !d.applyActorCommand(ctx, request) {
				return
			}
		case cfg := <-configs:
			if cfg.source == "" {
				continue
			}
			if len(cfg.content) == 0 {
				d.removePipeline(cfg)
			} else {
				d.addPipeline(ctx, cfg)
			}
		case event := <-d.mgr.events:
			cfg, status := d.mgr.handle(event)
			if cfg != nil && d.handler.SetStatus(cfg, status) {
				d.handler.NotifyConfigStatus(cfg, status)
			}
		case output <- groups:
			sent()
		case <-grace.C:
			d.mgr.processGracePeriodRemovals(ctx)
		}
	}
}

func (d *ServiceDiscovery) removePipeline(conf confFile) {
	// Origin ownership survives a failed rename even when its old cache entry is gone.
	d.mgr.Stop(pipelineKeyFromSource(conf.source))
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
		d.mgr.Stop(scfg.PipelineKey())

		d.handler.NotifyConfigRemove(scfg)
	}
}

func (d *ServiceDiscovery) addPipeline(ctx context.Context, conf confFile) {
	if !netdataapi.ValidSingleQuotedProtocolField(conf.source) {
		d.Errorf("config source '%s' cannot be represented in the plugins.d CONFIG protocol", conf.source)
		return
	}
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
	if !d.hasDiscovererType(scfg.DiscovererType()) {
		if scfg.SourceType() != confgroup.TypeStock {
			d.Warningf(
				"config '%s' uses unsupported discoverer type '%s', skipping",
				conf.source,
				scfg.DiscovererType(),
			)
		}
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
	if err := d.handler.ValidateConfigCreate(scfg, dyncfg.StatusAccepted); err != nil {
		d.Errorf("config '%s' cannot be represented in the plugins.d CONFIG protocol: %v", scfg.ExposedKey(), err)
		return
	}

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

		d.handler.NotifyConfigCreate(scfg, dyncfg.StatusAccepted)
		if d.runModePolicy.AutoEnableDiscovered {
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

	d.mgr.Stop(entry.Cfg.PipelineKey())

	// Replace in exposed cache
	d.handler.AddDiscoveredConfig(scfg, dyncfg.StatusAccepted)

	// Update dyncfg (remove old, create new with new source)
	d.handler.NotifyConfigRemove(entry.Cfg)
	d.handler.NotifyConfigCreate(scfg, dyncfg.StatusAccepted)

	if d.runModePolicy.AutoEnableDiscovered {
		d.autoEnableConfig(scfg)
	} else {
		d.handler.WaitForDecision(scfg)
	}
}

// removeOldConfigsFromSource removes configs from the same source that have a different key.
// This handles the case where a file's config name changes.
// Note: We don't stop the pipeline here - the new config will stop it when it starts via
// successful activation (which retires the same origin's incumbent).
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
			d.handler.NotifyConfigRemove(oldCfg)
		}
	}
}

// pipelineKeyFromSource extracts a pipeline key from a file source path.
// For now, we use the file path as key. This will be extended for dyncfg.
func pipelineKeyFromSource(source string) string {
	return source
}
