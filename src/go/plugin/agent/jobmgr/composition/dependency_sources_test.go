// SPDX-License-Identifier: GPL-3.0-or-later

package composition

import (
	"context"
	agentdiscovery "github.com/netdata/netdata/go/plugins/plugin/agent/discovery"
	"github.com/netdata/netdata/go/plugins/plugin/agent/jobmgr/lifecycle"
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

// Registration can outlive run finalization because the kernel starts first.
func TestRunShutdownRetiresLateDependencySubscription(t *testing.T) {
	source := &testDependencySource{callbacks: make(map[int]func(string, string))}
	entered, release := make(chan struct{}), make(chan struct{})
	blocked := dependencySourceFunc(func(notify func(string, string)) func() {
		unsubscribe := source.SubscribeDependencies(notify)
		close(entered)
		<-release
		return unsubscribe
	})
	frames, err := lifecycle.NewFrameOwner(newProcessSynchronizedBuffer())
	require.NoError(t, err)
	uids := lifecycle.NewUIDLedger()
	generation, err := newTestRunGeneration(t, runGenerationConfig{
		Secrets: testRunSecrets(t), Generation: 1, ShutdownTimeout: time.Second,
		UIDs: uids, Frames: frames, Modules: collectorapi.Registry{},
		Jobs: testRunJobServices(t), Discovery: testRunDiscoveryServices(t),
		DependencySources: []DependencySource{blocked},
	})
	require.NoError(t, err)
	runCtx, cancel := context.WithCancel(context.Background())
	defer cancel()
	done := make(chan error, 1)
	go func() { done <- generation.startWithRunContext(context.Background(), runCtx) }()
	<-entered
	require.Equal(t, 1, source.count())
	cancel()
	select {
	case <-generation.kernel.Done():
	case <-time.After(3 * time.Second):
		close(release)
		t.Fatal("run did not finalize while subscription registration was pending")
	}
	close(release)
	<-done
	require.ErrorIs(t, generation.Wait(context.Background()), context.Canceled)
	closeRunTestUIDs(t, uids)
	require.Zero(t, source.count(), "late registration must not survive run finalization")
}

type dependencySourceFunc func(func(string, string)) func()

func (f dependencySourceFunc) SubscribeDependencies(notify func(string, string)) func() {
	return f(notify)
}

func TestDependencySubscriptionsConcurrentRegistrationAndClose(t *testing.T) {
	var subscriptions dependencySubscriptions
	source := &testDependencySource{callbacks: make(map[int]func(string, string))}
	start := make(chan struct{})
	var joined sync.WaitGroup
	for range 32 {
		joined.Add(2)
		go func() {
			defer joined.Done()
			<-start
			_ = subscriptions.bind([]DependencySource{source}, func(string, string) {})
		}()
		go func() { defer joined.Done(); <-start; require.NoError(t, subscriptions.close()) }()
	}
	close(start)
	joined.Wait()
	require.NoError(t, subscriptions.close())
	require.Zero(t, source.count())
}

func TestDependencySubscriptionsCleanupPanicsAndIdempotence(t *testing.T) {
	var subscriptions dependencySubscriptions
	var calls atomic.Int32
	source := dependencySourceFunc(func(func(string, string)) func() { return func() { calls.Add(1); panic("unsubscribe failure") } })
	require.NoError(t, subscriptions.bind([]DependencySource{source, source}, func(string, string) {}))
	require.ErrorContains(t, subscriptions.close(), "unsubscribe failure")
	require.ErrorContains(t, subscriptions.close(), "unsubscribe failure")
	require.EqualValues(t, 2, calls.Load(), "all registrations clean up exactly once even when one panics")
}

func TestDependencySubscriptionPanicRetiresEarlierRegistrations(t *testing.T) {
	var subscriptions dependencySubscriptions
	source := &testDependencySource{callbacks: make(map[int]func(string, string))}
	panics := dependencySourceFunc(func(func(string, string)) func() { panic("registration failure") })
	require.ErrorContains(t, subscriptions.bind([]DependencySource{source, panics}, func(string, string) {}), "registration failure")
	require.Zero(t, source.count())
	require.NoError(t, subscriptions.close())
}

func TestDependencySubscriptionLateUnsubscribePanicIsReported(t *testing.T) {
	var subscriptions dependencySubscriptions
	entered, release := make(chan struct{}), make(chan struct{})
	var calls atomic.Int32
	source := dependencySourceFunc(func(func(string, string)) func() {
		close(entered)
		<-release
		return func() { calls.Add(1); panic("late unsubscribe failure") }
	})
	done := make(chan error, 1)
	go func() { done <- subscriptions.bind([]DependencySource{source}, func(string, string) {}) }()
	<-entered
	require.NoError(t, subscriptions.close())
	close(release)
	require.ErrorContains(t, <-done, "late unsubscribe failure")
	require.EqualValues(t, 1, calls.Load())
	require.NoError(t, subscriptions.close())
}
