// SPDX-License-Identifier: GPL-3.0-or-later

package file

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"sync"

	"github.com/netdata/netdata/go/plugins/logger"
	"github.com/netdata/netdata/go/plugins/plugin/framework/confgroup"
)

var log = logger.New().With(
	slog.String("component", "discovery"),
	slog.String("discoverer", "file"),
)

func NewDiscovery(cfg Config) (*Discovery, error) {
	if err := validateConfig(cfg); err != nil {
		return nil, fmt.Errorf("file discovery config validation: %v", err)
	}

	d := Discovery{
		Logger: log,
	}

	if err := d.registerDiscoverers(cfg); err != nil {
		return nil, fmt.Errorf("file discovery initialization: %v", err)
	}

	return &d, nil
}

type (
	Discovery struct {
		*logger.Logger
		discoverers []discoverer
	}
	discoverer interface {
		Run(ctx context.Context, in chan<- []*confgroup.Group)
		RunInitial(ctx context.Context, in chan<- []*confgroup.Group, initial func())
	}
)

func (d *Discovery) registerDiscoverers(cfg Config) error {
	if len(cfg.Read) != 0 {
		d.discoverers = append(d.discoverers, NewReader(cfg.Registry, cfg.Read))
	}
	if len(cfg.Watch) != 0 {
		d.discoverers = append(d.discoverers, NewWatcher(cfg.Registry, cfg.Watch))
	}
	if len(d.discoverers) == 0 {
		return errors.New("zero registered discoverers")
	}
	return nil
}

func (d *Discovery) Run(ctx context.Context, in chan<- []*confgroup.Group) {
	d.RunInitial(ctx, in, nil)
}

func (d *Discovery) RunInitial(ctx context.Context, in chan<- []*confgroup.Group, initial func()) {
	d.Info("instance is started")
	defer func() { d.Info("instance is stopped") }()

	var wg sync.WaitGroup
	var initialMu sync.Mutex
	remaining := len(d.discoverers)
	onInitial := func() {
		initialMu.Lock()
		defer initialMu.Unlock()
		remaining--
		if remaining == 0 && initial != nil && ctx.Err() == nil {
			initial()
		}
	}

	for _, dd := range d.discoverers {
		wg.Add(1)
		go func(dd discoverer) {
			defer wg.Done()
			d.runDiscoverer(ctx, dd, in, onInitial)
		}(dd)
	}

	wg.Wait()
	<-ctx.Done()
}

func (d *Discovery) runDiscoverer(ctx context.Context, dd discoverer, in chan<- []*confgroup.Group, initial func()) {
	updates := make(chan []*confgroup.Group)
	ready := make(chan struct{})
	done := make(chan struct{})
	go func() {
		defer close(done)
		dd.RunInitial(ctx, updates, func() { close(ready) })
	}()
	defer func() { <-done }()
	for {
		select {
		case <-ctx.Done():
			return
		case <-ready:
			ready = nil
			initial()
		case groups, ok := <-updates:
			if !ok {
				if ready != nil {
					select {
					case <-ready:
						initial()
					default:
					}
				}
				return
			}
			select {
			case <-ctx.Done():
				return
			case in <- groups:
			}
		}
	}
}
