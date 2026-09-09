// SPDX-License-Identifier: GPL-3.0-or-later

package sd

import (
	"context"
	"encoding/json"
	"errors"
	"strconv"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/netdata/netdata/go/plugins/logger"
	"github.com/netdata/netdata/go/plugins/plugin/agent/discovery/sd/model"
	"github.com/netdata/netdata/go/plugins/plugin/agent/discovery/sd/pipeline"
	"github.com/netdata/netdata/go/plugins/plugin/agent/jobmgr"
	"github.com/netdata/netdata/go/plugins/plugin/agent/jobmgr/containment"
	"github.com/netdata/netdata/go/plugins/plugin/framework/confgroup"
	"github.com/netdata/netdata/go/plugins/plugin/framework/dyncfg"
	"github.com/netdata/netdata/go/plugins/plugin/framework/functions"
	"github.com/stretchr/testify/require"
)

func newTestAttemptAuthority(t *testing.T) *containment.Authority {
	t.Helper()
	authority, err := containment.NewAuthority(nil)
	require.NoError(t, err)
	t.Cleanup(func() {
		ctx, cancel := context.WithTimeout(context.Background(), time.Second)
		defer cancel()
		require.NoError(t, authority.Shutdown(ctx))
	})
	return authority
}

func TestDescriptorParsingIsContainedPerConfigurationIdentity(t *testing.T) {
	attempts := newTestAttemptAuthority(t)
	entered := make(chan struct{})
	release := make(chan struct{})
	var enteredOnce sync.Once
	discovery, err := NewServiceDiscovery(Config{
		Epoch:      1,
		Attempts:   attempts,
		PluginName: "test",
		Discoverers: NewRegistry(Descriptor{
			Type: "fixture",
			ParseJSONConfig: func(raw json.RawMessage) (any, error) {
				var config struct {
					Block bool `json:"block"`
				}
				require.NoError(t, json.Unmarshal(raw, &config))
				if config.Block {
					enteredOnce.Do(func() { close(entered) })
					<-release
				}
				return config, nil
			},
			NewDiscoverers: func(any, string) ([]model.Discoverer, error) {
				return []model.Discoverer{containedTestDiscoverer{}}, nil
			},
		}),
	})
	require.NoError(t, err)

	blockedPayload := []byte(`{
		"name":"ignored",
		"discoverer":{"fixture":{"block":true}},
		"services":[{"id":"service","match":"true"}]
	}`)
	ctx, cancel := context.WithCancel(context.Background())
	blockedFn := dyncfg.NewFunction(ctx, functions.Function{
		UID:     "blocked",
		Args:    []string{"test:sd:fixture", "test", "job"},
		Payload: blockedPayload,
		Source:  "user=test",
	})
	done := make(chan error, 1)
	go func() {
		_, err := discovery.testDyncfgConfig(blockedFn, "job")
		done <- err
	}()
	select {
	case <-entered:
	case <-time.After(time.Second):
		require.FailNow(t, "test failed", "descriptor parser was not entered")
	}
	cancel()
	select {
	case err := <-done:
		require.ErrorIs(t, err, context.Canceled)
	case <-time.After(time.Second):
		require.FailNow(t, "test failed", "canceled descriptor parser did not settle")
	}

	_, err = discovery.testDyncfgConfig(
		dyncfg.NewFunction(t.Context(), functions.Function{
			UID:     "duplicate",
			Args:    []string{"test:sd:fixture", "test", "job"},
			Payload: blockedPayload,
			Source:  "user=test",
		}),
		"job",
	)
	var busy *materializationError
	require.ErrorAs(t, err, &busy)

	healthyPayload := []byte(`{
		"name":"ignored",
		"discoverer":{"fixture":{"block":false}},
		"services":[{"id":"service","match":"true"}]
	}`)
	fullyTested, err := discovery.testDyncfgConfig(
		dyncfg.NewFunction(t.Context(), functions.Function{
			UID:     "healthy",
			Args:    []string{"test:sd:fixture", "test", "job"},
			Payload: healthyPayload,
			Source:  "user=test",
		}),
		"job",
	)
	require.NoError(t, err)
	require.False(t, fullyTested)

	close(release)
	require.Eventually(t, func() bool {
		return attempts.Census() == (containment.Census{})
	}, time.Second, time.Millisecond)
}

func TestPipelineConstructionIsContainedPerPipelineIdentity(t *testing.T) {
	attempts := newTestAttemptAuthority(t)
	entered := make(chan struct{})
	release := make(chan struct{})
	var enteredOnce sync.Once
	discovery, err := NewServiceDiscovery(Config{
		Epoch:      1,
		Attempts:   attempts,
		PluginName: "test",
		Discoverers: NewRegistry(Descriptor{
			Type: "fixture",
			ParseJSONConfig: func(raw json.RawMessage) (any, error) {
				var config struct {
					Block bool `json:"block"`
				}
				if err := json.Unmarshal(raw, &config); err != nil {
					return nil, err
				}
				return config, nil
			},
			NewDiscoverers: func(config any, _ string) ([]model.Discoverer, error) {
				parsed, ok := config.(struct {
					Block bool `json:"block"`
				})
				if !ok {
					return nil, errors.New("test: unexpected descriptor config")
				}
				if parsed.Block {
					enteredOnce.Do(func() { close(entered) })
					<-release
				}
				return []model.Discoverer{containedTestDiscoverer{}}, nil
			},
		}),
	})
	require.NoError(t, err)
	configFor := func(name string, block bool) sdConfig {
		config, configErr := newSDConfigFromJSON(
			[]byte(`{
				"discoverer":{"fixture":{"block":`+
				map[bool]string{false: "false", true: "true"}[block]+`}},
				"services":[{"id":"service","match":"true"}]
			}`),
			name,
			"user=test",
			"dyncfg",
			"fixture",
			pipelineKey("fixture", name),
		)
		require.NoError(t, configErr)
		return config
	}
	blocked := configFor("blocked", true)
	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan error, 1)
	go func() {
		_, err := discovery.preparePipeline(ctx, blocked)
		done <- err
	}()
	select {
	case <-entered:
	case <-time.After(time.Second):
		require.FailNow(t, "test failed", "discoverer construction was not entered")
	}
	cancel()
	select {
	case err := <-done:
		require.ErrorIs(t, err, context.Canceled)
	case <-time.After(time.Second):
		require.FailNow(t, "test failed", "canceled pipeline construction did not settle")
	}

	_, err = discovery.preparePipeline(context.Background(), blocked)
	var busy *materializationError
	require.ErrorAs(t, err, &busy)

	healthy, err := discovery.preparePipeline(
		context.Background(),
		configFor("healthy", false),
	)
	require.NoError(t, err)
	require.NotNil(t, healthy)

	close(release)
	require.Eventually(t, func() bool {
		return attempts.Census() == (containment.Census{})
	}, time.Second, time.Millisecond)
}

func TestPersistentPipelineRetainsOnlyLatestDesiredUntilIdentityReleases(t *testing.T) {
	firstRelease := make(chan struct{})
	secondRelease := make(chan struct{})
	busy := &busyMaterializationAuthority{}
	busy.release.Store(firstRelease)
	discovery, err := NewServiceDiscovery(Config{
		Epoch:      1,
		Attempts:   busy,
		PluginName: "test",
		Discoverers: NewRegistry(Descriptor{
			Type: "fixture",
			ParseJSONConfig: func(json.RawMessage) (any, error) {
				return struct{}{}, nil
			},
			NewDiscoverers: func(any, string) ([]model.Discoverer, error) {
				return []model.Discoverer{containedTestDiscoverer{}}, nil
			},
		}),
	})
	require.NoError(t, err)
	discovery.Logger = logger.New()
	ctx, cancel := context.WithCancel(context.Background())
	defer func() {
		cancel()
		discovery.pending.wait()
	}()
	discovery.ctx = ctx
	discovery.pending = newPendingPipelineIndex(ctx)
	discovery.newPipeline = func(config pipeline.Config) (sdPipeline, error) {
		return newTestPipeline(config.Name), nil
	}
	discovery.mgr = NewPipelineManager(
		discovery.Logger,
		func(context.Context, []*confgroup.Group) {},
	)
	configFor := func(version int) sdConfig {
		config, configErr := newSDConfigFromJSON(
			[]byte(`{
				"version":`+strconv.Itoa(version)+`,
				"discoverer":{"fixture":{}},
				"services":[{"id":"service","match":"true"}]
			}`),
			"job",
			"/etc/netdata/sd/job.conf",
			confgroup.TypeUser,
			"fixture",
			"/etc/netdata/sd/job.conf",
		)
		require.NoError(t, configErr)
		return config
	}
	first := configFor(1)
	second := configFor(2)

	discovery.seen.Add(first)
	discovery.exposed.Add(&dyncfg.Entry[sdConfig]{
		Cfg:    first,
		Status: dyncfg.StatusFailed,
	})
	require.Error(t, discovery.sdCb.Start(
		dyncfg.NewFunction(t.Context(), functions.Function{}),
		first,
	))

	busy.release.Store(secondRelease)
	discovery.seen.Remove(first)
	discovery.seen.Add(second)
	discovery.exposed.Add(&dyncfg.Entry[sdConfig]{
		Cfg:    second,
		Status: dyncfg.StatusFailed,
	})
	require.Error(t, discovery.sdCb.Start(
		dyncfg.NewFunction(t.Context(), functions.Function{}),
		second,
	))

	close(firstRelease)
	staleWindow := time.NewTimer(20 * time.Millisecond)
staleRetries:
	for {
		select {
		case token := <-discovery.pending.retry:
			discovery.retryPendingPipeline(token)
			require.False(t, discovery.mgr.IsRunning(second.PipelineKey()))
		case <-staleWindow.C:
			break staleRetries
		}
	}

	healthyAttempts := newTestAttemptAuthority(t)
	discovery.attempts = healthyAttempts
	close(secondRelease)
	deadline := time.NewTimer(time.Second)
	for !discovery.mgr.IsRunning(second.PipelineKey()) {
		select {
		case token := <-discovery.pending.retry:
			discovery.retryPendingPipeline(token)
		case <-deadline.C:
			require.FailNow(t, "test failed", "latest pending pipeline was not retried")
		}
	}
	deadline.Stop()

	entry, exists := discovery.exposed.LookupByKey(second.ExposedKey())
	require.True(t, exists)
	require.Equal(t, second.Hash(), entry.Cfg.Hash())
	require.Equal(t, dyncfg.StatusRunning, entry.Status)
	require.True(t, discovery.mgr.IsRunning(second.PipelineKey()))
	discovery.mgr.StopAll()
}

func TestReleasedPipelineMaterializationRetainsImmediateRetry(t *testing.T) {
	ctx := t.Context()
	discovery := &ServiceDiscovery{
		attempts: &releasedMaterializationAuthority{},
		pending:  newPendingPipelineIndex(ctx),
	}
	defer discovery.pending.wait()
	config, err := newSDConfigFromJSON(
		[]byte(`{
			"discoverer":{"fixture":{}},
			"services":[{"id":"service","match":"true"}]
		}`),
		"job",
		"/etc/netdata/sd/job.conf",
		confgroup.TypeUser,
		"fixture",
		"/etc/netdata/sd/job.conf",
	)
	require.NoError(t, err)
	identity := pipelineMaterializationIdentity(config)

	discovery.retainPendingPipeline(config, &materializationError{
		cause:    jobmgr.ErrProcessAttemptBusy,
		identity: identity,
	})

	select {
	case token := <-discovery.pending.retry:
		retained, ok := discovery.pending.take(token)
		require.True(t, ok)
		require.Equal(t, config.Hash(), retained.Hash())
	case <-time.After(time.Second):
		require.FailNow(t, "test failed", "released materialization did not schedule an immediate retry")
	}
}

type containedTestDiscoverer struct{}

func (containedTestDiscoverer) Discover(context.Context, chan<- []model.TargetGroup) {}

type busyMaterializationAuthority struct {
	release atomic.Value
}

func (*busyMaterializationAuthority) StartProcessAttempt(
	context.Context,
	jobmgr.ProcessAttemptPlan,
) (jobmgr.ProcessAttempt, error) {
	return nil, errors.New("test: unexpected materialization start")
}

func (*busyMaterializationAuthority) SupersedeProcessAttempt(
	context.Context,
	jobmgr.ProcessAttemptIdentity,
) error {
	return jobmgr.ErrProcessAttemptBusy
}

func (*busyMaterializationAuthority) CutProcessAttempt(
	jobmgr.ProcessAttemptIdentity,
	error,
) bool {
	return false
}

func (authority *busyMaterializationAuthority) ProcessAttemptReleased(
	jobmgr.ProcessAttemptIdentity,
) (<-chan struct{}, bool) {
	release, ok := authority.release.Load().(chan struct{})
	return release, ok
}

type releasedMaterializationAuthority struct{}

func (*releasedMaterializationAuthority) StartProcessAttempt(
	context.Context,
	jobmgr.ProcessAttemptPlan,
) (jobmgr.ProcessAttempt, error) {
	return nil, errors.New("test: unexpected materialization start")
}

func (*releasedMaterializationAuthority) SupersedeProcessAttempt(
	context.Context,
	jobmgr.ProcessAttemptIdentity,
) error {
	return jobmgr.ErrProcessAttemptBusy
}

func (*releasedMaterializationAuthority) CutProcessAttempt(
	jobmgr.ProcessAttemptIdentity,
	error,
) bool {
	return false
}

func (*releasedMaterializationAuthority) ProcessAttemptReleased(
	jobmgr.ProcessAttemptIdentity,
) (<-chan struct{}, bool) {
	return nil, false
}
