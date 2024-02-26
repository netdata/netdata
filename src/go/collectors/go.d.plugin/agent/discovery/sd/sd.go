// SPDX-License-Identifier: GPL-3.0-or-later

package sd

import (
	"context"
	"sync"

	"github.com/netdata/netdata/go/go.d.plugin/agent/confgroup"
	"github.com/netdata/netdata/go/go.d.plugin/agent/discovery/sd/pipeline"
	"github.com/netdata/netdata/go/go.d.plugin/logger"

	"gopkg.in/yaml.v2"
)

func NewServiceDiscovery() (*ServiceDiscovery, error) {
	return nil, nil
}

type (
	ServiceDiscovery struct {
		*logger.Logger

		confProv  ConfigFileProvider
		sdFactory sdPipelineFactory

		confCache map[string]uint64
		pipelines map[string]func()
	}
	sdPipeline interface {
		Run(ctx context.Context, in chan<- []*confgroup.Group)
	}
	sdPipelineFactory interface {
		create(config pipeline.Config) (sdPipeline, error)
	}
)

func (d *ServiceDiscovery) Run(ctx context.Context, in chan<- []*confgroup.Group) {
	d.Info("instance is started")
	defer d.Info("instance is stopped")
	defer d.cleanup()

	var wg sync.WaitGroup

	wg.Add(1)
	go func() { defer wg.Done(); d.confProv.Run(ctx) }()

	for {
		select {
		case <-ctx.Done():
			return
		case cf := <-d.confProv.Configs():
			if cf.Source == "" {
				continue
			}
			if len(cf.Data) == 0 {
				delete(d.confCache, cf.Source)
				d.removePipeline(cf)
			} else if hash, ok := d.confCache[cf.Source]; !ok || hash != cf.Hash() {
				d.confCache[cf.Source] = cf.Hash()
				d.addPipeline(ctx, cf, in)
			}
		}
	}
}

func (d *ServiceDiscovery) addPipeline(ctx context.Context, cf ConfigFile, in chan<- []*confgroup.Group) {
	var cfg pipeline.Config

	if err := yaml.Unmarshal(cf.Data, &cfg); err != nil {
		d.Error(err)
		return
	}

	pl, err := d.sdFactory.create(cfg)
	if err != nil {
		d.Error(err)
		return
	}

	if stop, ok := d.pipelines[cf.Source]; ok {
		stop()
	}

	var wg sync.WaitGroup
	plCtx, cancel := context.WithCancel(ctx)

	wg.Add(1)
	go func() { defer wg.Done(); pl.Run(plCtx, in) }()
	stop := func() { cancel(); wg.Wait() }

	d.pipelines[cf.Source] = stop
}

func (d *ServiceDiscovery) removePipeline(cf ConfigFile) {
	if stop, ok := d.pipelines[cf.Source]; ok {
		delete(d.pipelines, cf.Source)
		stop()
	}
}

func (d *ServiceDiscovery) cleanup() {
	for _, stop := range d.pipelines {
		stop()
	}
}
