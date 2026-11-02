// SPDX-License-Identifier: GPL-3.0-or-later

package joboutput

import (
	"context"
	"errors"
	"math"
	"sync/atomic"
	"testing"
	"time"

	"github.com/netdata/netdata/go/plugins/pkg/metrix"
	"github.com/netdata/netdata/go/plugins/plugin/agent/jobmgr/lifecycle"
	"github.com/netdata/netdata/go/plugins/plugin/framework/collectorapi"
	"github.com/netdata/netdata/go/plugins/plugin/framework/confgroup"
	"github.com/netdata/netdata/go/plugins/plugin/framework/dyncfg"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestRetryNotBeforeSurvivesSecretRedaction(t *testing.T) {
	deadline := time.Now().Add(10 * time.Minute)
	cause := errors.New("synthetic credential must not survive")
	err := collectorapi.TemporaryError(collectorapi.WithRetryNotBefore(cause, deadline))
	for range 3 {
		err = redactResolvedLifecycleError(err)
		assert.Equal(t, deadline, collectorapi.RetryNotBefore(err))
		assert.Equal(t, collectorapi.LifecycleErrorTemporary, collectorapi.ClassifyLifecycleError(err))
		assert.NotContains(t, err.Error(), "synthetic credential")
		assert.False(t, errors.Is(err, cause))
	}
}

func TestRetryDeadlinePreservesLifecyclePolicy(t *testing.T) {
	for _, variant := range []string{"V1", "V2"} {
		t.Run(variant, func(t *testing.T) {
			cause := errors.New("provider wait")
			deadline := time.Now().Add(10 * time.Minute)
			tests := map[string]struct {
				phase   string
				cadence int
				err     error
				retry   bool
				class   collectorapi.LifecycleErrorClass
			}{
				"unclassified Init": {phase: "init", cadence: 1, err: cause},
				"temporary Init": {
					phase:   "init",
					cadence: 1,
					err:     collectorapi.TemporaryError(cause),
					retry:   true,
					class:   collectorapi.LifecycleErrorTemporary,
				},
				"disabled Check": {phase: "check", err: cause},
				"ordinary Check": {phase: "check", cadence: 1, err: cause, retry: true},
				"permanent Check": {
					phase:   "check",
					cadence: 1,
					err:     collectorapi.PermanentError(cause),
					class:   collectorapi.LifecycleErrorPermanent,
				},
			}
			for name, tc := range tests {
				t.Run(name, func(t *testing.T) {
					state := &factoryTestState{}
					fail := func(context.Context) error { return collectorapi.WithRetryNotBefore(tc.err, deadline) }
					creator := collectorapi.Creator{}
					if variant == "V1" {
						creator.Create = func() collectorapi.CollectorV1 {
							module := state.module(nil, false)
							if tc.phase == "init" {
								module.InitFunc = fail
							} else {
								module.CheckFunc = fail
							}
							return module
						}
					} else {
						creator.CreateV2 = func() collectorapi.CollectorV2 {
							module := &factoryTestV2{
								state:    state,
								store:    metrix.NewCollectorStore(),
								template: factoryTestChartTemplate,
							}
							if tc.phase == "init" {
								module.init = fail
							} else {
								module.check = fail
							}
							return module
						}
					}
					factory, _ := newFactoryTestHarness(t, creator, nil)
					cfg := factoryTestConfig(false).Set("autodetection_retry", tc.cadence)
					permit, _ := issueTestJobPermit(t, cfg.FullName(), 1)
					prepared, failure, err := prepareFactoryTestCandidate(
						t.Context(),
						factory,
						cfg,
						lifecycle.ResourceIdentity{
							ID:         cfg.FullName(),
							Generation: 1,
						},
						permit,
					)
					require.NoError(t, err)
					require.False(t, prepared.Valid())
					require.NotNil(t, failure)
					assert.Equal(t, tc.retry, failure.retry)
					assert.Equal(t, tc.class, failure.class)
					assert.Equal(t, deadline, failure.notBefore)
					assert.Equal(
						t,
						tc.class != collectorapi.LifecycleErrorUnclassified,
						failure.keepsFailedStockListed(),
					)
					assert.Equal(t, tc.retry, failure.retryDelay(time.Now()) > 0)
					require.NoError(t, permit.AbortUnused())
					requireFactoryAttemptsIdle(t, factory)
				})
			}
		})
	}
}

func TestRetryDelayConversion(t *testing.T) {
	now := time.Date(2026, 1, 1, 0, 0, 0, 999999999, time.UTC)
	tests := map[string]struct {
		retry      bool
		configured int
		remaining  time.Duration
		want       int64
	}{
		"unannotated":                   {retry: true, configured: 7, want: 7},
		"disabled":                      {configured: 1, remaining: time.Hour},
		"zero cadence remains disabled": {retry: true, remaining: time.Hour},
		"expired":                       {retry: true, configured: 7, remaining: -time.Second, want: 7},
		"imminent boundary":             {retry: true, configured: 1, remaining: time.Second, want: 2},
		"fractional second":             {retry: true, configured: 1, remaining: time.Second + 1, want: 3},
		"elapsed cleanup":               {retry: true, configured: 1, remaining: time.Second / 2, want: 2},
		"configured longer":             {retry: true, configured: 60, remaining: time.Second, want: 60},
		"maximum duration": {
			retry:      true,
			configured: 1,
			remaining:  time.Duration(math.MaxInt64),
			want:       9223372038,
		},
	}
	for name, tc := range tests {
		t.Run(name, func(t *testing.T) {
			f := &autoDetectionFailure{
				retry:      tc.retry,
				retryAfter: tc.configured,
			}
			if tc.remaining != 0 {
				f.notBefore = now.Add(tc.remaining)
			}
			assert.Equal(t, tc.want, f.retryDelay(now))
		})
	}
}

func TestStartupRetryDeadlineAcrossFactoryReconstruction(t *testing.T) {
	for _, variant := range []string{"V1", "V2"} {
		t.Run(variant, func(t *testing.T) {
			controller, graph, _, _, state := newDynCfgJobTestHarness(t)
			var created, checked atomic.Int32
			check := func(context.Context) error {
				checked.Add(1)
				return collectorapi.WithRetryNotBefore(errors.New("provider wait"), time.Now().Add(10*time.Minute))
			}
			creator := controller.modules["module"]
			if variant == "V1" {
				creator.Create = func() collectorapi.CollectorV1 { created.Add(1); return state.module(check, false) }
			} else {
				creator.Create = nil
				creator.CreateV2 = func() collectorapi.CollectorV2 {
					created.Add(1)
					return &factoryTestV2{
						state:    state,
						check:    check,
						store:    metrix.NewCollectorStore(),
						template: factoryTestChartTemplate,
					}
				}
			}
			controller.modules["module"] = creator
			commands := &activationTestCommands{
				queue: make(chan activationTestSubmission, 16),
				stop:  make(chan struct{}),
			}
			require.NoError(
				t,
				controller.BindBackgroundWorkers(
					commands,
					9,
					func(err error) { t.Errorf("background failure: %v", err) },
				),
			)
			t.Cleanup(func() {
				close(commands.stop)
				controller.scheduler.StopBackgroundWorkers()
				require.NoError(t, controller.scheduler.WaitBackgroundWorkers(context.Background()))
			})
			cfg := acceptedActivationTestConfig(confgroup.TypeUser).Set("autodetection_retry", 1)
			seedDynCfgJobGraphRecord(t, controller, graph, cfg, dyncfg.StatusAccepted)
			require.NoError(t, controller.scheduler.Tick(t.Context(), 0))
			applyAcceptedEnableForTest(t, controller, graph, cfg, 1)
			require.Nil(t, runtimeTestApplyActivation(t, commands, cfg.FullName(), 2))
			require.EqualValues(t, 1, checked.Load())
			require.EqualValues(t, 1, created.Load())
			requireFactoryAttemptsIdle(t, controller.factory)
			wake := controller.candidates.snapshot(cfg.FullName()).Wake
			require.NotNil(t, wake)
			require.GreaterOrEqual(t, wake.Due, int64(600), "provider's ten-minute bound must reach the scheduler")
			require.NoError(t, controller.scheduler.Tick(t.Context(), 599))
			assert.EqualValues(t, 1, checked.Load())
			require.NoError(t, controller.scheduler.Tick(t.Context(), int(wake.Due)))
			request := commands.next(t, "internal/jobs/autodetection-retry")
			require.Nil(t, applyActivationTestSubmission(t, request, nil, 0))
			require.Nil(t, runtimeTestApplyActivation(t, commands, cfg.FullName(), 3))
			assert.EqualValues(t, 2, created.Load(), "automatic retry reconstructs the collector")
			assert.EqualValues(t, 2, checked.Load())
		})
	}
}
