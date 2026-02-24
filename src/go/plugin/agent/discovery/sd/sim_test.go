// SPDX-License-Identifier: GPL-3.0-or-later

package sd

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"sync"
	"testing"
	"time"

	"github.com/netdata/netdata/go/plugins/logger"
	"github.com/netdata/netdata/go/plugins/pkg/executable"
	"github.com/netdata/netdata/go/plugins/pkg/netdataapi"
	"github.com/netdata/netdata/go/plugins/pkg/safewriter"
	"github.com/netdata/netdata/go/plugins/plugin/agent/discovery/sd/pipeline"
	"github.com/netdata/netdata/go/plugins/plugin/framework/confgroup"
	"github.com/netdata/netdata/go/plugins/plugin/framework/dyncfg"

	"github.com/stretchr/testify/assert"
)

var lock = &sync.Mutex{}

type discoverySim struct {
	configs       []confFile
	wantPipelines []*mockPipeline
}

// discoverySimExt is an extended simulation that also checks exposed configs
type discoverySimExt struct {
	configs          []confFile
	wantPipelines    []*mockPipeline
	wantExposedCount int
	wantExposed      []wantExposedCfg
}

type wantExposedCfg struct {
	discovererType string
	name           string
	sourceType     string
	status         dyncfg.Status
}

func (sim *discoverySimExt) run(t *testing.T) {
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
		dyncfgApi:   dyncfg.NewResponder(netdataapi.New(safewriter.New(&buf))),
		seen:        dyncfg.NewSeenCache[sdConfig](),
		exposed:     dyncfg.NewExposedCache[sdConfig](),
		discoverers: testDiscovererRegistry(),
		// dyncfgCh is intentionally nil to trigger auto-enable in tests
	}
	mgr.sdCb = &sdCallbacks{sd: mgr}
	mgr.handler = dyncfg.NewHandler(dyncfg.HandlerOpts[sdConfig]{
		Logger:    mgr.Logger,
		API:       mgr.dyncfgApi,
		Seen:      mgr.seen,
		Exposed:   mgr.exposed,
		Callbacks: mgr.sdCb,

		Path:           fmt.Sprintf(dyncfgSDPath, executable.Name),
		EnableFailCode: 422,
		JobCommands: []dyncfg.Command{
			dyncfg.CommandSchema,
			dyncfg.CommandGet,
			dyncfg.CommandEnable,
			dyncfg.CommandDisable,
			dyncfg.CommandUpdate,
			dyncfg.CommandTest,
			dyncfg.CommandUserconfig,
		},
	})

	in := make(chan<- []*confgroup.Group)
	done := make(chan struct{})
	ctx, cancel := context.WithCancel(context.Background())

	go func() { defer close(done); mgr.Run(ctx, in) }()

	time.Sleep(time.Second * 3)

	lock.Lock()
	if sim.wantPipelines != nil {
		assert.Equalf(t, sim.wantPipelines, fact.pipelines, "pipelines mismatch")
	}
	lock.Unlock()

	cancel()

	timeout := time.Second * 5
	select {
	case <-done:
	case <-time.After(timeout):
		t.Errorf("sd failed to exit in %s", timeout)
	}

	// Check exposed configs after SD goroutine has stopped (no race on entry.Status).
	// Caches survive shutdown — StopAll only stops pipelines, doesn't clear caches.
	if sim.wantExposedCount > 0 {
		assert.Equal(t, sim.wantExposedCount, mgr.exposed.Count(), "exposed configs count")
	}
	for _, want := range sim.wantExposed {
		entry, ok := mgr.exposed.LookupByKey(want.discovererType + ":" + want.name)
		if !assert.Truef(t, ok, "exposed config '%s:%s' not found", want.discovererType, want.name) {
			continue
		}
		assert.Equal(t, want.sourceType, entry.Cfg.SourceType(), "exposed config '%s:%s' sourceType", want.discovererType, want.name)
		assert.Equal(t, want.status, entry.Status, "exposed config '%s:%s' status", want.discovererType, want.name)
	}
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
		dyncfgApi:   dyncfg.NewResponder(netdataapi.New(safewriter.New(&buf))),
		seen:        dyncfg.NewSeenCache[sdConfig](),
		exposed:     dyncfg.NewExposedCache[sdConfig](),
		discoverers: testDiscovererRegistry(),
		// dyncfgCh is intentionally nil to trigger auto-enable in tests
		// (simulates terminal mode where netdata is not available)
	}
	mgr.sdCb = &sdCallbacks{sd: mgr}
	mgr.handler = dyncfg.NewHandler(dyncfg.HandlerOpts[sdConfig]{
		Logger:    mgr.Logger,
		API:       mgr.dyncfgApi,
		Seen:      mgr.seen,
		Exposed:   mgr.exposed,
		Callbacks: mgr.sdCb,

		Path:           fmt.Sprintf(dyncfgSDPath, executable.Name),
		EnableFailCode: 422,
		JobCommands: []dyncfg.Command{
			dyncfg.CommandSchema,
			dyncfg.CommandGet,
			dyncfg.CommandEnable,
			dyncfg.CommandDisable,
			dyncfg.CommandUpdate,
			dyncfg.CommandTest,
			dyncfg.CommandUserconfig,
		},
	})

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
