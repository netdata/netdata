// SPDX-License-Identifier: GPL-3.0-or-later

package discovery

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"sync"
	"time"

	"github.com/netdata/netdata/go/plugins/logger"
	"github.com/netdata/netdata/go/plugins/plugin/go.d/agent/confgroup"
	"github.com/netdata/netdata/go/plugins/plugin/go.d/agent/discovery/dummy"
	"github.com/netdata/netdata/go/plugins/plugin/go.d/agent/discovery/file"
	"github.com/netdata/netdata/go/plugins/plugin/go.d/agent/discovery/sd"
)

func NewManager(cfg Config) (*Manager, error) {
	if err := validateConfig(cfg); err != nil {
		return nil, fmt.Errorf("discovery manager config validation: %v", err)
	}

	mgr := &Manager{
		Logger: logger.New().With(
			slog.String("component", "discovery manager"),
		),
		send:        make(chan struct{}, 1),
		sendEvery:   time.Second * 2, // timeout to aggregate changes
		discoverers: make([]discoverer, 0),
		mux:         &sync.RWMutex{},
		cache:       newCache(),
	}

	if err := mgr.registerDiscoverers(cfg); err != nil {
		return nil, fmt.Errorf("discovery manager initializaion: %v", err)
	}

	return mgr, nil
}

type discoverer interface {
	Run(ctx context.Context, in chan<- []*confgroup.Group)
}

type Manager struct {
	*logger.Logger
	discoverers []discoverer
	send        chan struct{}
	sendEvery   time.Duration
	mux         *sync.RWMutex
	cache       *cache
}

func (m *Manager) String() string {
	return fmt.Sprintf("discovery manager: %v", m.discoverers)
}

func (m *Manager) Run(ctx context.Context, in chan<- []*confgroup.Group) {
	m.Info("instance is started")
	defer func() { m.Info("instance is stopped") }()

	var wg sync.WaitGroup

	for _, d := range m.discoverers {
		wg.Add(1)
		go func(d discoverer) {
			defer wg.Done()
			m.runDiscoverer(ctx, d)
		}(d)
	}

	wg.Add(1)
	go func() {
		defer wg.Done()
		m.sendLoop(ctx, in)
	}()

	wg.Wait()
	<-ctx.Done()
}

func (m *Manager) registerDiscoverers(cfg Config) error {
	if len(cfg.File.Read) > 0 || len(cfg.File.Watch) > 0 {
		cfg.File.Registry = cfg.Registry
		d, err := file.NewDiscovery(cfg.File)
		if err != nil {
			return err
		}
		m.discoverers = append(m.discoverers, d)
	}

	if len(cfg.Dummy.Names) > 0 {
		cfg.Dummy.Registry = cfg.Registry
		d, err := dummy.NewDiscovery(cfg.Dummy)
		if err != nil {
			return err
		}
		m.discoverers = append(m.discoverers, d)
	}

	if len(cfg.SD.ConfDir) != 0 {
		cfg.SD.ConfigDefaults = cfg.Registry
		d, err := sd.NewServiceDiscovery(cfg.SD)
		if err != nil {
			return err
		}
		m.discoverers = append(m.discoverers, d)
	}

	if len(m.discoverers) == 0 {
		return errors.New("zero registered discoverers")
	}

	m.Infof("registered discoverers: %v", m.discoverers)

	return nil
}

func (m *Manager) runDiscoverer(ctx context.Context, d discoverer) {
	updates := make(chan []*confgroup.Group)
	go d.Run(ctx, updates)

	for {
		select {
		case <-ctx.Done():
			return
		case groups, ok := <-updates:
			if !ok {
				return
			}
			func() {
				m.mux.Lock()
				defer m.mux.Unlock()

				m.cache.update(groups)
				m.triggerSend()
			}()
		}
	}
}

func (m *Manager) sendLoop(ctx context.Context, in chan<- []*confgroup.Group) {
	m.mustSend(ctx, in)

	tk := time.NewTicker(m.sendEvery)
	defer tk.Stop()

	for {
		select {
		case <-ctx.Done():
			return
		case <-tk.C:
			select {
			case <-m.send:
				m.trySend(in)
			default:
			}
		}
	}
}

func (m *Manager) mustSend(ctx context.Context, in chan<- []*confgroup.Group) {
	select {
	case <-ctx.Done():
		return
	case <-m.send:
		m.mux.Lock()
		groups := m.cache.groups()
		m.cache.reset()
		m.mux.Unlock()

		select {
		case <-ctx.Done():
		case in <- groups:
		}
		return
	}
}

func (m *Manager) trySend(in chan<- []*confgroup.Group) {
	m.mux.Lock()
	defer m.mux.Unlock()

	select {
	case in <- m.cache.groups():
		m.cache.reset()
	default:
		m.triggerSend()
	}
}

func (m *Manager) triggerSend() {
	select {
	case m.send <- struct{}{}:
	default:
	}
}
