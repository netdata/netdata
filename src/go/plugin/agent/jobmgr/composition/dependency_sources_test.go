// SPDX-License-Identifier: GPL-3.0-or-later

package composition

import (
	"context"
	agentdiscovery "github.com/netdata/netdata/go/plugins/plugin/agent/discovery"
	"github.com/netdata/netdata/go/plugins/plugin/framework/collectorapi"
	"github.com/netdata/netdata/go/plugins/plugin/framework/confgroup"
	"github.com/stretchr/testify/require"
	"io"
	"sync"
	"sync/atomic"
	"testing"
	"time"
)

type testDependencySource struct {
	mu        sync.Mutex
	next      int
	callbacks map[int]func(string, string)
	ready     atomic.Bool
}

func (*testDependencySource) Run(ctx context.Context) { <-ctx.Done() }
func (s *testDependencySource) SubscribeDependencies(notify func(string, string)) func() {
	s.mu.Lock()
	s.next++
	id := s.next
	s.callbacks[id] = notify
	s.mu.Unlock()
	return func() { s.mu.Lock(); delete(s.callbacks, id); s.mu.Unlock() }
}
func (s *testDependencySource) count() int { s.mu.Lock(); defer s.mu.Unlock(); return len(s.callbacks) }
func (s *testDependencySource) publish() {
	s.ready.Store(true)
	s.mu.Lock()
	var callbacks []func(string, string)
	for _, cb := range s.callbacks {
		callbacks = append(callbacks, cb)
	}
	s.mu.Unlock()
	for _, cb := range callbacks {
		cb("test-runtime", "browser")
	}
}
func TestProcessDependencySourceWakesJobsAndRetiresSubscriptions(t *testing.T) {
	source := &testDependencySource{callbacks: make(map[int]func(string, string))}
	initialized := make(chan struct{}, 4)
	reader, writer := io.Pipe()
	output := newProcessSynchronizedBuffer()
	config := testProductionProcessConfig(reader, output)
	config.AutoEnable = true
	config.Services = []ProcessService{source}
	config.Modules["test"] = collectorapi.Creator{
		Config: func() any { return &collectorapi.MockConfiguration{} },
		Create: func() collectorapi.CollectorV1 {
			c := &collectorapi.MockCollectorV1{}
			c.InitFunc = func(context.Context) error { initialized <- struct{}{}; return context.Canceled }
			return c
		},
		ActivationDependencies: func(map[string]any) ([]collectorapi.ActivationDependency, error) {
			return []collectorapi.ActivationDependency{{Kind: "test-runtime", Name: "browser", Ready: func() error {
				if source.ready.Load() {
					return nil
				}
				return context.DeadlineExceeded
			}}}, nil
		},
	}
	config.DiscoveryProviders = []agentdiscovery.ProviderFactory{agentdiscovery.NewProviderFactory("test", func(agentdiscovery.BuildContext) (agentdiscovery.Discoverer, bool, error) {
		return runTestDiscoverer{configs: []confgroup.Config{{"module": "test", "name": "site", "__source_type__": "user", "__source__": "test", "__provider__": "test"}}}, true, nil
	})}
	process, err := NewProcess(config)
	require.NoError(t, err)
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	done := make(chan error, 1)
	go func() { done <- process.Run(ctx) }()
	t.Cleanup(func() {
		shutdown, stop := context.WithTimeout(context.Background(), 3*time.Second)
		defer stop()
		defer cancel()
		defer reader.Close()
		defer writer.Close()
		require.NoError(t, process.Terminate(shutdown))
		require.NoError(t, <-done)
		require.Zero(t, source.count())
	})
	output.waitContains(t, "CONFIG go.d:collector:test:site create accepted")
	require.Equal(t, 1, source.count())
	select {
	case <-initialized:
		t.Fatal("unavailable runtime reached Init")
	default:
	}
	source.publish()
	select {
	case <-initialized:
	case <-ctx.Done():
		t.Fatal("named event did not wake job")
	}
	require.NoError(t, process.Restart(ctx))
	require.Equal(t, 1, source.count(), "only successor subscription survives restart")
}
