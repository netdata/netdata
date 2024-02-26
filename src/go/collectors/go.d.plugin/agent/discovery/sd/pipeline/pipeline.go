// SPDX-License-Identifier: GPL-3.0-or-later

package pipeline

import (
	"context"
	"log/slog"
	"time"

	"github.com/netdata/netdata/go/go.d.plugin/agent/confgroup"
	"github.com/netdata/netdata/go/go.d.plugin/agent/discovery/sd/hostsocket"
	"github.com/netdata/netdata/go/go.d.plugin/agent/discovery/sd/kubernetes"
	"github.com/netdata/netdata/go/go.d.plugin/agent/discovery/sd/model"
	"github.com/netdata/netdata/go/go.d.plugin/logger"
)

func New(cfg Config) (*Pipeline, error) {
	if err := validateConfig(cfg); err != nil {
		return nil, err
	}

	p := &Pipeline{
		Logger: logger.New().With(
			slog.String("component", "discovery sd pipeline"),
		),
		accum:       newAccumulator(),
		discoverers: make([]model.Discoverer, 0),
		items:       make(map[string]map[uint64][]confgroup.Config),
	}

	if err := p.registerDiscoverers(cfg); err != nil {
		return nil, err
	}

	return p, nil
}

type (
	Pipeline struct {
		*logger.Logger

		discoverers []model.Discoverer
		accum       *accumulator

		clr classificator
		cmr composer

		items map[string]map[uint64][]confgroup.Config // [source][targetHash]
	}
	classificator interface {
		classify(model.Target) model.Tags
	}
	composer interface {
		compose(model.Target) []confgroup.Config
	}
)

func (p *Pipeline) registerDiscoverers(conf Config) error {
	for _, cfg := range conf.Discovery.K8s {
		td, err := kubernetes.NewKubeDiscoverer(cfg)
		if err != nil {
			return err
		}
		p.discoverers = append(p.discoverers, td)
	}
	if conf.Discovery.HostSocket.Net != nil {
		td, err := hostsocket.NewNetSocketDiscoverer(*conf.Discovery.HostSocket.Net)
		if err != nil {
			return err
		}
		p.discoverers = append(p.discoverers, td)
	}

	return nil
}

func (p *Pipeline) Run(ctx context.Context, in chan<- []*confgroup.Group) {
	p.Info("instance is started")
	defer p.Info("instance is stopped")

	p.accum.discoverers = p.discoverers

	updates := make(chan []model.TargetGroup)
	done := make(chan struct{})

	go func() { defer close(done); p.accum.run(ctx, updates) }()

	for {
		select {
		case <-ctx.Done():
			select {
			case <-done:
			case <-time.After(time.Second * 5):
			}
			return
		case <-done:
			return
		case tggs := <-updates:
			p.Infof("received %d target groups", len(tggs))
			send(ctx, in, p.processGroups(tggs))
		}
	}
}

func (p *Pipeline) processGroups(tggs []model.TargetGroup) []*confgroup.Group {
	var confGroups []*confgroup.Group
	// updates come from the accumulator, this ensures that all groups have different sources
	for _, tgg := range tggs {
		p.Infof("processing group '%s' with %d target(s)", tgg.Source(), len(tgg.Targets()))
		if v := p.processGroup(tgg); v != nil {
			confGroups = append(confGroups, v)
		}
	}
	return confGroups
}

func (p *Pipeline) processGroup(tgg model.TargetGroup) *confgroup.Group {
	if len(tgg.Targets()) == 0 {
		if _, ok := p.items[tgg.Source()]; !ok {
			return nil
		}
		delete(p.items, tgg.Source())
		return &confgroup.Group{Source: tgg.Source()}
	}

	targetsCache, ok := p.items[tgg.Source()]
	if !ok {
		targetsCache = make(map[uint64][]confgroup.Config)
		p.items[tgg.Source()] = targetsCache
	}

	var changed bool
	seen := make(map[uint64]bool)

	for _, tgt := range tgg.Targets() {
		if tgt == nil {
			continue
		}

		hash := tgt.Hash()
		seen[hash] = true

		if _, ok := targetsCache[hash]; ok {
			continue
		}

		if tags := p.clr.classify(tgt); len(tags) > 0 {
			tgt.Tags().Merge(tags)

			if configs := p.cmr.compose(tgt); len(configs) > 0 {
				for _, cfg := range configs {
					cfg.SetProvider(tgg.Provider())
					cfg.SetSource(tgg.Source())
				}
				targetsCache[hash] = configs
				changed = true
			}
		} else {
			p.Infof("target '%s' classify: fail", tgt.TUID())
		}
	}

	for hash := range targetsCache {
		if seen[hash] {
			continue
		}
		if configs := targetsCache[hash]; len(configs) > 0 {
			changed = true
		}
		delete(targetsCache, hash)
	}

	if !changed {
		return nil
	}

	// TODO: deepcopy?
	cfgGroup := &confgroup.Group{Source: tgg.Source()}
	for _, cfgs := range targetsCache {
		cfgGroup.Configs = append(cfgGroup.Configs, cfgs...)
	}

	return cfgGroup
}

func send(ctx context.Context, in chan<- []*confgroup.Group, configs []*confgroup.Group) {
	if len(configs) == 0 {
		return
	}

	select {
	case <-ctx.Done():
		return
	case in <- configs:
	}
}
