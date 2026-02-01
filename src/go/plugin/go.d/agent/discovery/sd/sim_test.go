// SPDX-License-Identifier: GPL-3.0-or-later

package sd

import (
	"bytes"
	"context"
	"errors"
	"sync"
	"testing"
	"time"

	"github.com/netdata/netdata/go/plugins/logger"
	"github.com/netdata/netdata/go/plugins/pkg/netdataapi"
	"github.com/netdata/netdata/go/plugins/pkg/safewriter"
	"github.com/netdata/netdata/go/plugins/plugin/go.d/agent/confgroup"
	"github.com/netdata/netdata/go/plugins/plugin/go.d/agent/discovery/sd/pipeline"
	"github.com/netdata/netdata/go/plugins/plugin/go.d/agent/dyncfg"

	"github.com/stretchr/testify/assert"
)

var lock = &sync.Mutex{}

type discoverySim struct {
	configs       []confFile
	wantPipelines []*mockPipeline
}

func (sim *discoverySim) run(t *testing.T) {
	fact := &mockFactory{}
	var buf bytes.Buffer
	mgr := &ServiceDiscovery{
		Logger: logger.New(),
		newPipeline: func(config pipeline.Config) (sdPipeline, error) {
			return fact.create(config)
		},
		confProv: &mockConfigProvider{
			confFiles: sim.configs,
			ch:        make(chan confFile),
		},
		dyncfgApi:      dyncfg.NewResponder(netdataapi.New(safewriter.New(&buf))),
		seenConfigs:    newSeenSDConfigs(),
		exposedConfigs: newExposedSDConfigs(),
		// dyncfgCh is intentionally nil to trigger auto-enable in tests
		// (simulates terminal mode where netdata is not available)
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
	confFiles []confFile
	ch        chan confFile
}

func (m *mockConfigProvider) run(ctx context.Context) {
	for _, conf := range m.confFiles {
		select {
		case <-ctx.Done():
			return
		case m.ch <- conf:
		}
	}
	<-ctx.Done()
}

func (m *mockConfigProvider) configs() chan confFile {
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
