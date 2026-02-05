// SPDX-License-Identifier: GPL-3.0-or-later

package sd

import (
	"context"
	"log/slog"
	"os"
	"sync"

	"github.com/netdata/netdata/go/plugins/logger"
	"github.com/netdata/netdata/go/plugins/pkg/multipath"
	"github.com/netdata/netdata/go/plugins/pkg/netdataapi"
	"github.com/netdata/netdata/go/plugins/pkg/safewriter"
	"github.com/netdata/netdata/go/plugins/plugin/go.d/agent/confgroup"
	"github.com/netdata/netdata/go/plugins/plugin/go.d/agent/discovery/sd/pipeline"
	"github.com/netdata/netdata/go/plugins/plugin/go.d/agent/dyncfg"
	"github.com/netdata/netdata/go/plugins/plugin/go.d/agent/functions"

	"github.com/mattn/go-isatty"
)

var isTerminal = isatty.IsTerminal(os.Stdout.Fd()) || isatty.IsTerminal(os.Stdin.Fd())

type Config struct {
	ConfigDefaults confgroup.Registry
	ConfDir        multipath.MultiPath
	FnReg          functions.Registry
}

func NewServiceDiscovery(cfg Config) (*ServiceDiscovery, error) {
	log := logger.New().With(
		slog.String("component", "service discovery"),
	)

	d := &ServiceDiscovery{
		Logger:         log,
		confProv:       newConfFileReader(log, cfg.ConfDir),
		configDefaults: cfg.ConfigDefaults,
		fnReg:          cfg.FnReg,
		dyncfgApi:      dyncfg.NewResponder(netdataapi.New(safewriter.Stdout)),
		seenConfigs:    newSeenSDConfigs(),
		exposedConfigs: newExposedSDConfigs(),
		dyncfgCh:       make(chan dyncfg.Function, 1),
		newPipeline: func(config pipeline.Config) (sdPipeline, error) {
			return pipeline.New(config)
		},
	}

	return d, nil
}

type (
	ServiceDiscovery struct {
		*logger.Logger

		confProv confFileProvider

		configDefaults confgroup.Registry
		fnReg          functions.Registry
		dyncfgApi      *dyncfg.Responder
		seenConfigs    *seenSDConfigs    // All discovered configs by UID
		exposedConfigs *exposedSDConfigs // Configs exposed to dyncfg by Key
		dyncfgCh       chan dyncfg.Function
		newPipeline    func(config pipeline.Config) (sdPipeline, error)

		ctx context.Context
		mgr *PipelineManager

		// waitCfgOnOff holds the pipeline key we're waiting for enable/disable on.
		// When set, we only process dyncfg commands (not new file configs).
		// This ensures netdata can send enable/disable before we process more configs.
		waitCfgOnOff string
	}
	sdPipeline interface {
		Run(ctx context.Context, in chan<- []*confgroup.Group)
	}
	confFileProvider interface {
		run(ctx context.Context)
		configs() chan confFile
	}
)

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
		if d.waitCfgOnOff != "" {
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
	seenCfgs := d.seenConfigs.lookupBySource(conf.source)
	if len(seenCfgs) == 0 {
		return
	}

	d.Infof("removing %d config(s) from source '%s'", len(seenCfgs), conf.source)

	for _, scfg := range seenCfgs {
		// Remove from seen cache
		d.seenConfigs.remove(scfg)

		// Check if this was the exposed config
		ecfg, ok := d.exposedConfigs.lookup(scfg)
		if !ok || scfg.UID() != ecfg.UID() {
			// Not exposed or different config is exposed - skip dyncfg remove
			continue
		}

		// This was the exposed config - stop pipeline and remove from dyncfg
		if d.mgr.IsRunning(scfg.PipelineKey()) {
			d.mgr.Stop(scfg.PipelineKey())
		}

		d.exposedConfigs.remove(scfg)
		d.dyncfgSDJobRemove(scfg.DiscovererType(), scfg.Name())
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
		d.removeOldConfigsFromSource(scfg.Source(), scfg.Key())
	}

	// Always add to seen cache
	d.seenConfigs.add(scfg)

	// Check if there's an existing exposed config with the same key
	ecfg, exists := d.exposedConfigs.lookup(scfg)

	if !exists {
		// No existing config - expose this one
		scfg.SetStatus(dyncfg.StatusAccepted)
		d.exposedConfigs.add(scfg)
		d.dyncfgSDJobCreate(scfg.DiscovererType(), scfg.Name(), scfg.SourceType(), scfg.Source(), scfg.Status())

		if isTerminal || d.dyncfgCh == nil {
			// Auto-enable in terminal mode or tests
			d.autoEnableConfig(scfg)
		} else {
			// Wait for netdata to send enable/disable
			d.waitCfgOnOff = scfg.PipelineKey()
		}
		return
	}

	// Existing config found - apply priority rules
	sp, ep := scfg.SourceTypePriority(), ecfg.SourceTypePriority()

	// Higher priority wins. If same priority and existing is running, keep existing (stability).
	if ep > sp || (ep == sp && ecfg.Status() == dyncfg.StatusRunning) {
		d.Debugf("config '%s': keeping existing (priority: existing=%d new=%d, status=%s)",
			scfg.Key(), ep, sp, ecfg.Status())
		return
	}

	// New config wins - stop existing if running
	d.Infof("config '%s': replacing existing (priority: existing=%d new=%d)", scfg.Key(), ep, sp)

	if ecfg.Status() == dyncfg.StatusRunning {
		d.mgr.Stop(ecfg.PipelineKey())
	}

	// Replace in exposed cache
	scfg.SetStatus(dyncfg.StatusAccepted)
	d.exposedConfigs.add(scfg)

	// Update dyncfg (remove old, create new with new source)
	d.dyncfgSDJobRemove(ecfg.DiscovererType(), ecfg.Name())
	d.dyncfgSDJobCreate(scfg.DiscovererType(), scfg.Name(), scfg.SourceType(), scfg.Source(), scfg.Status())

	if isTerminal || d.dyncfgCh == nil {
		d.autoEnableConfig(scfg)
	} else {
		d.waitCfgOnOff = scfg.PipelineKey()
	}
}

// removeOldConfigsFromSource removes configs from the same source that have a different key.
// This handles the case where a file's config name changes.
// Note: We don't stop the pipeline here - the new config will stop it when it starts via
// PipelineManager.Start (which stops any existing pipeline with the same key).
// This ensures that if the new config fails to start, the old pipeline keeps running.
func (d *ServiceDiscovery) removeOldConfigsFromSource(source, newKey string) {
	oldCfgs := d.seenConfigs.lookupBySource(source)
	for _, oldCfg := range oldCfgs {
		if oldCfg.Key() == newKey {
			continue // Same config, skip
		}

		// Different config from same source - remove from caches
		d.seenConfigs.remove(oldCfg)

		// If it was exposed, remove from exposed cache and dyncfg
		// But DON'T stop the pipeline - let the new config's enable handle that
		if ecfg, ok := d.exposedConfigs.lookup(oldCfg); ok && ecfg.UID() == oldCfg.UID() {
			d.exposedConfigs.remove(oldCfg)
			d.dyncfgSDJobRemove(oldCfg.DiscovererType(), oldCfg.Name())
		}
	}
}

// autoEnableConfig enables a config without waiting for netdata's enable command.
func (d *ServiceDiscovery) autoEnableConfig(cfg sdConfig) {
	fn := dyncfg.NewFunction(functions.Function{
		Args: []string{d.dyncfgJobID(cfg.DiscovererType(), cfg.Name()), "enable"},
	})
	d.dyncfgCmdEnable(fn)
}

// pipelineKeyFromSource extracts a pipeline key from a file source path.
// For now, we use the file path as key. This will be extended for dyncfg.
func pipelineKeyFromSource(source string) string {
	return source
}
