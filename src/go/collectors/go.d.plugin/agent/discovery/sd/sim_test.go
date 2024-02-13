// SPDX-License-Identifier: GPL-3.0-or-later

package sd

import (
	"context"
	"errors"
	"sync"
	"testing"
	"time"

	"github.com/netdata/go.d.plugin/agent/confgroup"
	"github.com/netdata/go.d.plugin/agent/discovery/sd/pipeline"
	"github.com/netdata/go.d.plugin/logger"

	"github.com/stretchr/testify/assert"
)

var lock = &sync.Mutex{}

type discoverySim struct {
	configs       []ConfigFile
	wantPipelines []*mockPipeline
}

func (sim *discoverySim) run(t *testing.T) {
	fact := &mockFactory{}
	mgr := &ServiceDiscovery{
		Logger:    logger.New(),
		sdFactory: fact,
		confProv: &mockConfigProvider{
			configs: sim.configs,
			ch:      make(chan ConfigFile),
		},
		confCache: make(map[string]uint64),
		pipelines: make(map[string]func()),
	}

	in := make(chan<- []*confgroup.Group)
	done := make(chan struct{})
	ctx, cancel := context.WithCancel(context.Background())

	go func() { defer close(done); mgr.Run(ctx, in) }()

	time.Sleep(time.Second * 3)

	lock.Lock()
	assert.Equalf(t, sim.wantPipelines, fact.pipelines, "before stop")
	lock.Unlock()

	cancel()

	timeout := time.Second * 5

	select {
	case <-done:
		lock.Lock()
		for _, pl := range fact.pipelines {
			assert.Truef(t, pl.stopped, "pipeline '%s' is not stopped after cancel()", pl.name)
		}
		lock.Unlock()
	case <-time.After(timeout):
		t.Errorf("sd failed to exit in %s", timeout)
	}
}

type mockConfigProvider struct {
	configs []ConfigFile
	ch      chan ConfigFile
}

func (m *mockConfigProvider) Run(ctx context.Context) {
	for _, conf := range m.configs {
		select {
		case <-ctx.Done():
			return
		case m.ch <- conf:
		}
	}
	<-ctx.Done()
}

func (m *mockConfigProvider) Configs() chan ConfigFile {
	return m.ch
}

type mockFactory struct {
	pipelines []*mockPipeline
}

func (m *mockFactory) create(cfg pipeline.Config) (sdPipeline, error) {
	lock.Lock()
	defer lock.Unlock()

	if cfg.Name == "invalid" {
		return nil, errors.New("mock sdPipelineFactory.create() error")
	}

	pl := mockPipeline{name: cfg.Name}
	m.pipelines = append(m.pipelines, &pl)

	return &pl, nil
}

type mockPipeline struct {
	name    string
	started bool
	stopped bool
}

func (m *mockPipeline) Run(ctx context.Context, _ chan<- []*confgroup.Group) {
	lock.Lock()
	m.started = true
	lock.Unlock()
	defer func() { lock.Lock(); m.stopped = true; lock.Unlock() }()
	<-ctx.Done()
}
