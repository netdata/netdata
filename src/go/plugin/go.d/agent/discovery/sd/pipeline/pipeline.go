// SPDX-License-Identifier: GPL-3.0-or-later

package pipeline

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"time"

	"github.com/netdata/netdata/go/plugins/logger"
	"github.com/netdata/netdata/go/plugins/pkg/hostinfo"
	"github.com/netdata/netdata/go/plugins/plugin/go.d/agent/confgroup"
	"github.com/netdata/netdata/go/plugins/plugin/go.d/agent/discovery/sd/discoverer/dockersd"
	"github.com/netdata/netdata/go/plugins/plugin/go.d/agent/discovery/sd/discoverer/k8ssd"
	"github.com/netdata/netdata/go/plugins/plugin/go.d/agent/discovery/sd/discoverer/netlistensd"
	"github.com/netdata/netdata/go/plugins/plugin/go.d/agent/discovery/sd/discoverer/snmpsd"
	"github.com/netdata/netdata/go/plugins/plugin/go.d/agent/discovery/sd/model"
)

func New(cfg Config) (*Pipeline, error) {
	if err := validateConfig(cfg); err != nil {
		return nil, err
	}

	p := &Pipeline{
		Logger: logger.New().With(
			slog.String("component", "service discovery"),
			slog.String("pipeline", cfg.Name),
		),
		configDefaults: cfg.ConfigDefaults,
		accum:          newAccumulator(),
		discoverers:    make([]model.Discoverer, 0),
		configs:        make(map[string]map[uint64][]confgroup.Config),
	}

	p.accum.Logger = p.Logger

	if len(cfg.Services) > 0 {
		svr, err := newServiceEngine(cfg.Services)
		if err != nil {
			return nil, fmt.Errorf("services rules: %v", err)
		}
		p.svr = svr
		svr.Logger = p.Logger
	} else {
		// Legacy path
		clr, err := newTargetClassificator(cfg.Classify)
		if err != nil {
			return nil, fmt.Errorf("classify rules: %v", err)
		}
		cmr, err := newConfigComposer(cfg.Compose)
		if err != nil {
			return nil, fmt.Errorf("compose rules: %v", err)
		}
		p.clr, p.cmr = clr, cmr
		clr.Logger = p.Logger
		cmr.Logger = p.Logger
	}

	if err := p.registerDiscoverers(cfg); err != nil {
		return nil, err
	}

	return p, nil
}

type (
	Pipeline struct {
		*logger.Logger

		configDefaults confgroup.Registry
		discoverers    []model.Discoverer
		accum          *accumulator

		configs map[string]map[uint64][]confgroup.Config // [targetSource][targetHash]

		// new
		svr composer

		// legacy
		clr classificator
		cmr composer
	}
	classificator interface {
		classify(model.Target) model.Tags
	}
	composer interface {
		compose(model.Target) []confgroup.Config
	}
)

func (p *Pipeline) registerDiscoverers(conf Config) error {
	for _, cfg := range conf.Discover {
		switch cfg.Discoverer {
		case "net_listeners":
			cfg.NetListeners.Source = conf.Source
			td, err := netlistensd.NewDiscoverer(cfg.NetListeners)
			if err != nil {
				return fmt.Errorf("failed to create '%s' discoverer: %v", cfg.Discoverer, err)
			}
			p.discoverers = append(p.discoverers, td)
		case "docker":
			if hostinfo.IsInsideK8sCluster() {
				p.Infof("not registering '%s' discoverer: disabled in k8s environment", cfg.Discoverer)
				continue
			}
			cfg.Docker.Source = conf.Source
			td, err := dockersd.NewDiscoverer(cfg.Docker)
			if err != nil {
				return fmt.Errorf("failed to create '%s' discoverer: %v", cfg.Discoverer, err)
			}
			p.discoverers = append(p.discoverers, td)
		case "k8s":
			for _, k8sCfg := range cfg.K8s {
				k8sCfg.Source = conf.Source
				td, err := k8ssd.NewKubeDiscoverer(k8sCfg)
				if err != nil {
					return fmt.Errorf("failed to create '%s' discoverer: %v", cfg.Discoverer, err)
				}
				p.discoverers = append(p.discoverers, td)
			}
		case "snmp":
			cfg.SNMP.Source = conf.Source
			td, err := snmpsd.NewDiscoverer(cfg.SNMP)
			if err != nil {
				return fmt.Errorf("failed to create '%s' discoverer: %v", cfg.Discoverer, err)
			}
			p.discoverers = append(p.discoverers, td)
		default:
			return fmt.Errorf("unknown discoverer: '%s'", cfg.Discoverer)
		}
	}

	if len(p.discoverers) == 0 {
		return errors.New("no discoverers registered")
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
			case <-time.After(time.Second * 10):
			}
			return
		case <-done:
			return
		case tggs := <-updates:
			p.Debugf("received %d target groups", len(tggs))
			if cfggs := p.processGroups(tggs); len(cfggs) > 0 {
				select {
				case <-ctx.Done():
				case in <- cfggs: // FIXME: potentially stale configs if upstream cannot receive (blocking)
				}
			}
		}
	}
}

func (p *Pipeline) processGroups(tggs []model.TargetGroup) []*confgroup.Group {
	var groups []*confgroup.Group
	// updates come from the accumulator; this ensures that all groups have different sources
	for _, tgg := range tggs {
		p.Debugf("processing group '%s' with %d target(s)", tgg.Source(), len(tgg.Targets()))
		if v := p.processGroup(tgg); v != nil {
			groups = append(groups, v)
		}
	}
	return groups
}

func (p *Pipeline) processGroup(tgg model.TargetGroup) *confgroup.Group {
	if len(tgg.Targets()) == 0 {
		if _, ok := p.configs[tgg.Source()]; !ok {
			return nil
		}
		delete(p.configs, tgg.Source())

		return &confgroup.Group{Source: tgg.Source()}
	}

	targetsCache, ok := p.configs[tgg.Source()]
	if !ok {
		targetsCache = make(map[uint64][]confgroup.Config)
		p.configs[tgg.Source()] = targetsCache
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

		targetsCache[hash] = nil

		if p.svr != nil {
			if cfgs := p.svr.compose(tgt); len(cfgs) > 0 {
				targetsCache[hash] = cfgs
				changed = true
				for _, cfg := range cfgs {
					cfg.SetProvider(tgg.Provider())
					cfg.SetSource(tgg.Source())
					cfg.SetSourceType(confgroup.TypeDiscovered)
					if def, ok := p.configDefaults.Lookup(cfg.Module()); ok {
						cfg.ApplyDefaults(def)
					}
				}
			}
			continue
		}

		// Legacy:
		if tags := p.clr.classify(tgt); len(tags) > 0 {
			tgt.Tags().Merge(tags)

			if cfgs := p.cmr.compose(tgt); len(cfgs) > 0 {
				targetsCache[hash] = cfgs
				changed = true

				for _, cfg := range cfgs {
					cfg.SetProvider(tgg.Provider())
					cfg.SetSource(tgg.Source())
					cfg.SetSourceType(confgroup.TypeDiscovered)
					if def, ok := p.configDefaults.Lookup(cfg.Module()); ok {
						cfg.ApplyDefaults(def)
					}
				}
			}
		}

	}

	for hash := range targetsCache {
		if seen[hash] {
			continue
		}
		if cfgs := targetsCache[hash]; len(cfgs) > 0 {
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
