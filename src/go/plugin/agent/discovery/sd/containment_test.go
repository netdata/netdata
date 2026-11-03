// SPDX-License-Identifier: GPL-3.0-or-later

package sd

import (
	"context"
	"encoding/json"
	"errors"
	"sync"
	"testing"
	"time"

	"github.com/netdata/netdata/go/plugins/plugin/agent/discovery/sd/model"
	"github.com/netdata/netdata/go/plugins/plugin/agent/jobmgr"
	"github.com/netdata/netdata/go/plugins/plugin/agent/jobmgr/containment"
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
	// A new epoch still cannot re-enter the same physically retained constructor.
	nextRun, err := NewServiceDiscovery(Config{
		Epoch:       2,
		Attempts:    attempts,
		PluginName:  "test",
		Discoverers: discovery.discoverers,
	})
	require.NoError(t, err)
	_, err = nextRun.preparePipeline(t.Context(), blocked)
	require.ErrorIs(t, err, jobmgr.ErrProcessAttemptBusy)
	// Changed inputs under the same logical name are a different candidate.
	replacement, err := nextRun.preparePipeline(t.Context(), configFor("blocked", false))
	require.NoError(t, err)
	require.NotNil(t, replacement)

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

type containedTestDiscoverer struct{}

func (containedTestDiscoverer) Discover(context.Context, chan<- []model.TargetGroup) {}
