// SPDX-License-Identifier: GPL-3.0-or-later

package filestatus

import (
	"context"
	"log/slog"
	"os"
	"time"

	"github.com/netdata/netdata/go/plugins/logger"
	"github.com/netdata/netdata/go/plugins/plugin/go.d/agent/confgroup"
)

func NewManager(path string) *Manager {
	return &Manager{
		Logger: logger.New().With(
			slog.String("component", "filestatus manager"),
		),
		path:       path,
		store:      &Store{},
		flushEvery: time.Second * 5,
		flushCh:    make(chan struct{}, 1),
	}
}

type Manager struct {
	*logger.Logger

	path string

	store *Store

	flushEvery time.Duration
	flushCh    chan struct{}
}

func (m *Manager) Run(ctx context.Context) {
	m.Info("instance is started")
	defer func() { m.Info("instance is stopped") }()

	tk := time.NewTicker(m.flushEvery)
	defer tk.Stop()
	defer m.flush()

	for {
		select {
		case <-ctx.Done():
			return
		case <-tk.C:
			m.tryFlush()
		}
	}
}

func (m *Manager) Save(cfg confgroup.Config, status string) {
	if v, ok := m.store.lookup(cfg); !ok || status != v {
		m.store.add(cfg, status)
		m.triggerFlush()
	}
}

func (m *Manager) Remove(cfg confgroup.Config) {
	if _, ok := m.store.lookup(cfg); ok {
		m.store.remove(cfg)
		m.triggerFlush()
	}
}

func (m *Manager) triggerFlush() {
	select {
	case m.flushCh <- struct{}{}:
	default:
	}
}

func (m *Manager) tryFlush() {
	select {
	case <-m.flushCh:
		m.flush()
	default:
	}
}

func (m *Manager) flush() {
	bs, err := m.store.bytes()
	if err != nil {
		return
	}
	_ = os.WriteFile(m.path, bs, 0644)
}
