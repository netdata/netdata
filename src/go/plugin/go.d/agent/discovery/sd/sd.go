// SPDX-License-Identifier: GPL-3.0-or-later

package sd

import (
	"context"
	"fmt"
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
	"gopkg.in/yaml.v2"
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
		exposedConfigs *exposedSDConfigs
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
	key := pipelineKeyFromSource(conf.source)
	if d.mgr.IsRunning(key) {
		d.Infof("received empty config, stopping pipeline '%s'", key)
		d.mgr.Stop(key)
	}

	// Remove from dyncfg if exposed
	d.removeExposedFileConfig(conf.source)
}

func (d *ServiceDiscovery) addPipeline(ctx context.Context, conf confFile) {
	var cfg pipeline.Config

	if err := yaml.Unmarshal(conf.content, &cfg); err != nil {
		d.Errorf("failed to unmarshal config from '%s': %v", conf.source, err)
		return
	}

	if cfg.Disabled {
		d.Infof("pipeline '%s' is disabled in config", cfg.Name)
		return
	}

	cfg.Source = fmt.Sprintf("file=%s", conf.source)
	cfg.ConfigDefaults = d.configDefaults

	key := pipelineKeyFromSource(conf.source)

	// If pipeline already running (reload case), restart with grace period.
	// Already running means netdata previously enabled it.
	if d.mgr.IsRunning(key) {
		d.Infof("restarting pipeline '%s' with updated config", key)
		if err := d.mgr.Restart(ctx, key, cfg); err != nil {
			d.Errorf("pipeline '%s': %v", key, err)
			return
		}
		// Update exposed config
		d.removeExposedFileConfig(conf.source)
		d.exposeFileConfig(cfg, conf, dyncfg.StatusRunning)
		return
	}

	// For new pipelines, expose first with Accepted status and wait for enable.
	// This matches jobmgr pattern - netdata will send enable based on stored state.
	d.removeExposedFileConfig(conf.source)

	// Check if we can use the dyncfg enable path (requires discoverers for dyncfg ID)
	canUseDyncfgEnable := len(cfg.Discover) > 0 && cfg.Name != ""

	if !canUseDyncfgEnable {
		// No discoverers or name - can't use dyncfg path, start directly (tests, simple configs)
		if err := d.mgr.Start(d.ctx, key, cfg); err != nil {
			d.Errorf("pipeline '%s': %v", key, err)
			return
		}
		d.exposeFileConfig(cfg, conf, dyncfg.StatusRunning)
		return
	}

	// Store config and create dyncfg job to notify netdata
	d.exposeFileConfig(cfg, conf, dyncfg.StatusAccepted)

	if isTerminal || d.dyncfgCh == nil {
		// Auto-enable in terminal mode or tests (no netdata to send commands)
		d.autoEnableFileConfig(cfg)
	} else {
		// Wait for netdata to send enable/disable command
		d.waitCfgOnOff = key
	}
}

// pipelineKeyFromSource extracts a pipeline key from a file source path.
// For now, we use the file path as key. This will be extended for dyncfg.
func pipelineKeyFromSource(source string) string {
	return source
}
