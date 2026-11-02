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

func TestFailedJobRestartPreservesNewRetryDeadline(t *testing.T) {
	controller, graph, _, _, state := newDynCfgJobTestHarness(t)
	base := time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)
	controller.candidates.wallNow = func() time.Time { return base }
	failure := collectorapi.WithRetryNotBefore(errors.New("provider wait"), base.Add(10*time.Minute))
	useV2CheckFailureCollector(controller, state, failure)
	cfg := acceptedActivationTestConfig(confgroup.TypeDyncfg).Set("autodetection_retry", 1)
	seedDynCfgJobGraphRecord(t, controller, graph, cfg, dyncfg.StatusFailed)
	seedTestCandidateBinding(t, controller, cfg, dyncfg.StatusFailed)
	permit, _ := issueTestJobPermit(t, cfg.FullName(), 1)
	prepared, err := controller.Prepare(t.Context(), DynCfgJobRequest{Args: []string{controller.externalID(cfg.FullName()), "restart"}}, nil, lifecycle.ResourceTransactionScope{ID: cfg.FullName(), Successor: lifecycle.ResourceIdentity{ID: cfg.FullName(), Generation: 1}}, permit)
	require.NoError(t, err)
	require.Nil(t, runtimeTestApply(t, prepared))
	wake := controller.candidates.snapshot(cfg.FullName()).Wake
	require.NotNil(t, wake)
	assert.GreaterOrEqual(t, wake.Due, int64(600), "a fresh restart failure must constrain its subsequent automatic recovery")
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
					assert.Equal(t, tc.retry, failure.retryPolicy().cadence > 0)
					assert.Equal(t, deadline, failure.retryPolicy().notBefore, "classification never discards provider metadata")
					require.NoError(t, permit.AbortUnused())
					requireFactoryAttemptsIdle(t, factory)
				})
			}
		})
	}
}

func TestCandidateRetryConstraints(t *testing.T) {
	wall := time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)
	tests := map[string]struct {
		logical int64
		elapsed time.Duration
		ready   bool
		wake    int64
	}{
		"cadence not elapsed":          {logical: 9, elapsed: time.Second, ready: false, wake: 10},
		"provider minimum not elapsed": {logical: 10, ready: false, wake: 11},
		"both elapsed":                 {logical: 10, elapsed: 200 * time.Millisecond, ready: true, wake: 10},
		"compressed ticks":             {logical: 100, ready: false, wake: 101},
	}
	for name, tc := range tests {
		t.Run(name, func(t *testing.T) {
			e := &candidateEntry{}
			e.setRetry(candidateRetryPolicy{cadence: 10, notBefore: wall.Add(200 * time.Millisecond)}, 0)
			assert.Equal(t, tc.ready, e.retryReady(tc.logical, wall.Add(tc.elapsed)))
			assert.Equal(t, tc.wake, e.retryWake(tc.logical, wall.Add(tc.elapsed)))
			assert.Equal(t, int64(10), e.Due, "wake estimates do not restart the configured cadence")
			e.clearRetry()
			assert.False(t, e.retryReady(100, wall.Add(time.Hour)))
			assert.Equal(t, wall.Add(200*time.Millisecond), e.NotBefore, "cadence clearing preserves the provider minimum")
			e.resetRetry()
			assert.True(t, e.NotBefore.IsZero())
		})
	}
	e := &candidateEntry{}
	e.setRetry(candidateRetryPolicy{cadence: 1, notBefore: wall.Add(time.Duration(math.MaxInt64))}, 0)
	assert.Equal(t, int64(9223372037), e.retryWake(0, wall))
	assert.False(t, e.retryReady(math.MaxInt64, wall), "a distant wake estimate never authorizes early admission")
	assert.Equal(t, int64(math.MaxInt64), retryLogicalDue(math.MaxInt64-1, 10), "logical deadlines do not wrap")
}

func TestCandidateRetryDeadlineKeepsSiblingsIndependent(t *testing.T) {
	c := newCandidateController(1)
	wall := time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)
	c.wallNow = func() time.Time { return wall }
	inv := candidateTestInventory(1, true, candidateTestConfig("a", "a"), candidateTestConfig("b", "b"))
	first := candidateInventory(t, c, inv, 0).Decision.Work
	second := candidateResult(t, c, first, candidateOutcome{Kind: candidateFailed, Listed: true,
		Retry: candidateRetryPolicy{cadence: 1, notBefore: wall.Add(time.Minute)}}, 0).Decision.Work
	require.NotNil(t, second, "a provider minimum cannot block a sibling's initial attempt")
	waiting := candidateResult(t, c, second, candidateOutcome{Kind: candidateFailed, Listed: true,
		Retry: candidateRetryPolicy{cadence: 1}}, 0)
	require.NotNil(t, waiting.Decision.Wake)
	assert.EqualValues(t, 1, waiting.Decision.Wake.Due)
	wake, err := c.prepareWake(inv.FullName, waiting.Decision.Wake.Token, 1)
	applyCandidateDelta(t, wake, err)
	require.NotNil(t, wake.Decision.Work)
	assert.Equal(t, second.Token.Candidate, wake.Decision.Work.Token.Candidate)
	assert.Equal(t, wall.Add(time.Minute), c.snapshot(inv.FullName).entry(first.Token.Candidate).NotBefore)
}

func TestCandidateRetryRechecksFrozenPass(t *testing.T) {
	c := newCandidateController(1)
	base := time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)
	wall := base
	c.wallNow = func() time.Time { return wall }
	inv := candidateTestInventory(1, true, candidateTestConfig("a", "a"))
	first := candidateInventory(t, c, inv, 0).Decision.Work
	candidateResult(t, c, first, candidateOutcome{Kind: candidateFailed, Listed: true,
		Retry: candidateRetryPolicy{cadence: 1, notBefore: base.Add(time.Minute)}}, 0)
	// Both clocks permit recovery when the pass freezes behind a new initial
	// candidate. A later wall-clock correction must still constrain admission.
	wall = base.Add(time.Minute)
	other := candidateTestConfig("b", "b")
	inv.Version = 2
	inv.Candidates = append(inv.Candidates, DiscoveredJobCandidate{Config: other, Source: "b", Incarnation: 2})
	inv.Sources["b"] = 1
	initial := candidateInventory(t, c, inv, 1).Decision.Work
	require.NotNil(t, initial)
	require.Equal(t, other, initial.Config)
	wall = base
	waiting := candidateResult(t, c, initial, candidateOutcome{Kind: candidateFailed, Listed: true}, 2)
	assert.Nil(t, waiting.Decision.Work)
	require.NotNil(t, waiting.Decision.Wake)
	assert.EqualValues(t, 62, waiting.Decision.Wake.Due)
	wall = base.Add(time.Minute)
	wake, err := c.prepareWake(inv.FullName, waiting.Decision.Wake.Token, 62)
	applyCandidateDelta(t, wake, err)
	require.NotNil(t, wake.Decision.Work)
	assert.Equal(t, first.Token.Candidate, wake.Decision.Work.Token.Candidate)
}

func TestCandidateBlockedRetryChecksDeadlineOnWakeAndResume(t *testing.T) {
	for _, via := range []string{"wake", "resume"} {
		t.Run(via, func(t *testing.T) {
			c := newCandidateController(1)
			base := time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)
			wall := base
			c.wallNow = func() time.Time { return wall }
			inv := candidateTestInventory(1, true, candidateTestConfig("a", "a"))
			work := candidateInventory(t, c, inv, 0).Decision.Work
			candidateResult(t, c, work, candidateOutcome{Kind: candidateAdmitted}, 0)
			blocked := candidateResult(t, c, work, candidateOutcome{Kind: candidateFrameworkBlocked,
				Block: candidateBlockFailedRetry, Listed: true,
				Retry: candidateRetryPolicy{cadence: 1, notBefore: base.Add(200 * time.Millisecond)}}, 0)
			require.NotNil(t, blocked.Decision.Wake)
			var early *candidateDelta
			var err error
			if via == "wake" {
				early, err = c.prepareWake(inv.FullName, blocked.Decision.Wake.Token, 1)
			} else {
				early, err = c.prepareResume(inv.FullName, work.Token, 1)
			}
			applyCandidateDelta(t, early, err)
			assert.Nil(t, early.Decision.Work)
			require.NotNil(t, early.Decision.Wake)
			assert.EqualValues(t, 2, early.Decision.Wake.Due)
			snapshot := c.snapshot(inv.FullName)
			assert.Equal(t, work.Token, snapshot.Active.Token, "an early wake cannot start a new attempt")
			assert.EqualValues(t, 1, snapshot.entry(work.Token.Candidate).Due)
			wall = base.Add(200 * time.Millisecond)
			ready, err := c.prepareWake(inv.FullName, early.Decision.Wake.Token, 2)
			applyCandidateDelta(t, ready, err)
			require.NotNil(t, ready.Decision.Work)
			assert.Equal(t, work.Token.Candidate, ready.Decision.Work.Token.Candidate)
			assert.NotEqual(t, work.Token.Attempt, ready.Decision.Work.Token.Attempt)
			assert.True(t, c.snapshot(inv.FullName).entry(work.Token.Candidate).NotBefore.IsZero())
		})
	}
}

func TestStartupRetryDeadlineAcrossFactoryReconstruction(t *testing.T) {
	for _, variant := range []string{"V1", "V2"} {
		t.Run(variant, func(t *testing.T) {
			controller, graph, _, _, state := newDynCfgJobTestHarness(t)
			var created, checked atomic.Int32
			var elapsed atomic.Int64
			base := time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)
			wallNow := func() time.Time { return base.Add(time.Duration(elapsed.Load())) }
			controller.candidates.wallNow = wallNow
			check := func(context.Context) error {
				checked.Add(1)
				return collectorapi.WithRetryNotBefore(errors.New("provider wait"), wallNow().Add(200*time.Millisecond))
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
			// Model received, pending and imminent ticks without advancing real time.
			for clock := 1; clock <= 3; clock++ {
				wake := controller.candidates.snapshot(cfg.FullName()).Wake
				require.NotNil(t, wake)
				require.EqualValues(t, clock, wake.Due)
				require.NoError(t, controller.scheduler.Tick(t.Context(), clock))
				request := commands.next(t, "internal/jobs/autodetection-retry")
				require.Nil(t, applyActivationTestSubmission(t, request, nil, 0))
				assert.EqualValues(t, 1, created.Load(), "early wake must not reconstruct a collector")
				assert.EqualValues(t, 1, checked.Load())
			}
			snapshot := controller.candidates.snapshot(cfg.FullName())
			assert.Equal(t, int64(1), snapshot.entry(snapshot.Binding.Key).Due)
			elapsed.Store(int64(200 * time.Millisecond))
			require.NoError(t, controller.scheduler.Tick(t.Context(), int(snapshot.Wake.Due)))
			request := commands.next(t, "internal/jobs/autodetection-retry")
			require.Nil(t, applyActivationTestSubmission(t, request, nil, 0))
			require.Nil(t, runtimeTestApplyActivation(t, commands, cfg.FullName(), 3))
			assert.EqualValues(t, 2, created.Load(), "automatic retry reconstructs the collector")
			assert.EqualValues(t, 2, checked.Load())
		})
	}
}
