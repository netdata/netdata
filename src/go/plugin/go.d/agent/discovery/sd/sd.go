// SPDX-License-Identifier: GPL-3.0-or-later

package sd

import (
	"context"
	"fmt"
	"log/slog"
	"sync"

	"github.com/netdata/netdata/go/plugins/logger"
	"github.com/netdata/netdata/go/plugins/pkg/multipath"
	"github.com/netdata/netdata/go/plugins/pkg/netdataapi"
	"github.com/netdata/netdata/go/plugins/pkg/safewriter"
	"github.com/netdata/netdata/go/plugins/plugin/go.d/agent/confgroup"
	"github.com/netdata/netdata/go/plugins/plugin/go.d/agent/discovery/sd/pipeline"
	"github.com/netdata/netdata/go/plugins/plugin/go.d/agent/dyncfg"
	"github.com/netdata/netdata/go/plugins/plugin/go.d/agent/functions"

	"gopkg.in/yaml.v2"
)

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
		newPipeline    func(config pipeline.Config) (sdPipeline, error)

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

func (d *ServiceDiscovery) String() string {
	return "service discovery"
}

func (d *ServiceDiscovery) Run(ctx context.Context, in chan<- []*confgroup.Group) {
	d.Info("instance is started")
	defer func() { d.unregisterDyncfgTemplates(); d.Info("instance is stopped") }()

	// Register dyncfg templates for discoverer types
	d.registerDyncfgTemplates(ctx)

	// Create pipeline manager with send function that forwards to output channel
	send := func(ctx context.Context, groups []*confgroup.Group) {
		select {
		case <-ctx.Done():
		case in <- groups:
		}
	}

	d.mgr = NewPipelineManager(d.Logger, d.newPipeline, send)

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

	<-ctx.Done()
}

func (d *ServiceDiscovery) run(ctx context.Context) {
	for {
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

	var err error
	if d.mgr.IsRunning(key) {
		d.Infof("restarting pipeline '%s' with updated config", key)
		err = d.mgr.Restart(ctx, key, cfg)
	} else {
		d.Infof("starting pipeline '%s'", key)
		err = d.mgr.Start(ctx, key, cfg)
	}

	if err != nil {
		d.Errorf("pipeline '%s': %v", key, err)
		return
	}

	// Expose file config as dyncfg job
	d.exposeFileConfig(cfg, conf)
}

// pipelineKeyFromSource extracts a pipeline key from a file source path.
// For now, we use the file path as key. This will be extended for dyncfg.
func pipelineKeyFromSource(source string) string {
	return source
}
