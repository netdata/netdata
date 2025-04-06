// SPDX-License-Identifier: GPL-3.0-or-later

package sd

import (
	"context"
	"fmt"
	"log/slog"
	"sync"

	"github.com/netdata/netdata/go/plugins/logger"
	"github.com/netdata/netdata/go/plugins/pkg/multipath"
	"github.com/netdata/netdata/go/plugins/plugin/go.d/agent/confgroup"
	"github.com/netdata/netdata/go/plugins/plugin/go.d/agent/discovery/sd/pipeline"

	"gopkg.in/yaml.v2"
)

type Config struct {
	ConfigDefaults confgroup.Registry
	ConfDir        multipath.MultiPath
}

func NewServiceDiscovery(cfg Config) (*ServiceDiscovery, error) {
	log := logger.New().With(
		slog.String("component", "service discovery"),
	)

	d := &ServiceDiscovery{
		Logger:         log,
		confProv:       newConfFileReader(log, cfg.ConfDir),
		configDefaults: cfg.ConfigDefaults,
		newPipeline: func(config pipeline.Config) (sdPipeline, error) {
			return pipeline.New(config)
		},
		pipelines: make(map[string]func()),
	}

	return d, nil
}

type (
	ServiceDiscovery struct {
		*logger.Logger

		confProv confFileProvider

		configDefaults confgroup.Registry
		newPipeline    func(config pipeline.Config) (sdPipeline, error)
		pipelines      map[string]func()
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
	defer func() { d.cleanup(); d.Info("instance is stopped") }()

	var wg sync.WaitGroup

	wg.Add(1)
	go func() { defer wg.Done(); d.confProv.run(ctx) }()

	wg.Add(1)
	go func() { defer wg.Done(); d.run(ctx, in) }()

	wg.Wait()
	<-ctx.Done()
}

func (d *ServiceDiscovery) run(ctx context.Context, in chan<- []*confgroup.Group) {
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
				d.addPipeline(ctx, cfg, in)
			}
		}
	}
}

func (d *ServiceDiscovery) removePipeline(conf confFile) {
	if stop, ok := d.pipelines[conf.source]; ok {
		d.Infof("received an empty config, stopping the pipeline ('%s')", conf.source)
		delete(d.pipelines, conf.source)
		stop()
	}
}

func (d *ServiceDiscovery) addPipeline(ctx context.Context, conf confFile, in chan<- []*confgroup.Group) {
	var cfg pipeline.Config

	if err := yaml.Unmarshal(conf.content, &cfg); err != nil {
		d.Error(err)
		return
	}

	if cfg.Disabled {
		d.Infof("pipeline config is disabled '%s' (%s)", cfg.Name, conf.source)
		return
	}

	cfg.Source = fmt.Sprintf("file=%s", conf.source)
	cfg.ConfigDefaults = d.configDefaults

	pl, err := d.newPipeline(cfg)
	if err != nil {
		d.Error(err)
		return
	}

	if stop, ok := d.pipelines[conf.source]; ok {
		stop()
	}

	var wg sync.WaitGroup
	plCtx, cancel := context.WithCancel(ctx)

	wg.Add(1)
	go func() { defer wg.Done(); pl.Run(plCtx, in) }()

	stop := func() { cancel(); wg.Wait() }
	d.pipelines[conf.source] = stop
}

func (d *ServiceDiscovery) cleanup() {
	for _, stop := range d.pipelines {
		stop()
	}
}
