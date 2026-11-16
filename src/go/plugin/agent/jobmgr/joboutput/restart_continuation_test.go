// SPDX-License-Identifier: GPL-3.0-or-later

package joboutput

import (
	"context"
	"errors"
	"fmt"
	"sync/atomic"
	"testing"
	"time"

	"github.com/netdata/netdata/go/plugins/pkg/confopt"
	"github.com/netdata/netdata/go/plugins/plugin/agent/jobmgr"
	"github.com/netdata/netdata/go/plugins/plugin/agent/jobmgr/lifecycle"
	secretresolver "github.com/netdata/netdata/go/plugins/plugin/agent/secrets/resolver"
	"github.com/netdata/netdata/go/plugins/plugin/framework/collectorapi"
	"github.com/netdata/netdata/go/plugins/plugin/framework/confgroup"
	"github.com/netdata/netdata/go/plugins/plugin/framework/dyncfg"
	"github.com/stretchr/testify/require"
)

type restartContinuationOutcome struct {
	result lifecycle.SealedResult
	err    error
}

type restartContinuationFixture struct {
	controller  *DynCfgJobController
	graph       *dyncfg.Graph
	commands    *activationTestCommands
	config      confgroup.Config
	current     lifecycle.ReadyResource
	result      <-chan restartContinuationOutcome
	cancel      context.CancelFunc
	snapshot    atomic.Pointer[factoryTestAtomicScope]
	resolutions atomic.Int32
	storeValue  func(int32) string
}

// The transport controls command ordering only. restart() owns the real receipt,
// and every config, resume, install, and readiness change uses its shipped plan.
func restartContinuationNew(t *testing.T, create func(int32) *runtimeTestCollector) *restartContinuationFixture {
	t.Helper()
	controller, graph, _, _, _ := newDynCfgJobTestHarness(t)
	fixture := &restartContinuationFixture{
		controller: controller,
		graph:      graph,
	}
	var constructions atomic.Int32
	configureRuntimeTestCollector(controller, func() *runtimeTestCollector {
		ordinal := constructions.Add(1)
		if create != nil {
			return create(ordinal)
		}
		return restartContinuationCollector()
	})
	controller.factory.config.ConfigModules.config.Configs = testConfigResolver(
		t,
		testAtomicResolver(t),
		func([]string) (secretresolver.AtomicScope, error) {
			ordinal := fixture.resolutions.Add(1)
			value := fmt.Sprintf("generation-%d", ordinal)
			if fixture.storeValue != nil {
				value = fixture.storeValue(ordinal)
			}
			snapshot := &factoryTestAtomicScope{
				value: value,
			}
			snapshot.current.Store(true)
			fixture.snapshot.Store(snapshot)
			return snapshot, nil
		},
	)
	fixture.commands = bindHandoffTestWorkers(t, controller)
	fixture.config = factoryTestConfig(
		false,
	).SetSourceType(confgroup.TypeDyncfg).
		Set("option_str", "${store:vault:test:key}")
	seedDynCfgJobGraphRecord(t, controller, graph, fixture.config, dyncfg.StatusFailed)
	t.Cleanup(func() { stopRuntimeTestResource(t, fixture.current) })
	return fixture
}

func restartContinuationCollector() *runtimeTestCollector {
	return &runtimeTestCollector{
		run: func(ctx context.Context, ready func()) error { ready(); <-ctx.Done(); return ctx.Err() },
	}
}

func (fixture *restartContinuationFixture) start(t *testing.T) {
	t.Helper()
	ctx, cancel := context.WithTimeout(t.Context(), 5*time.Second)
	fixture.cancel = cancel
	t.Cleanup(cancel)
	result := make(chan restartContinuationOutcome, 1)
	fixture.result = result
	go func() {
		response, err := fixture.controller.restart(ctx, DynCfgJobRequest{
			Args: []string{fixture.controller.externalID(fixture.config.FullName()), "restart"},
		}, dynCfgTarget{
			resourceID: fixture.config.FullName(),
			module:     fixture.config.Module(),
			name:       fixture.config.Name(),
			creator:    fixture.controller.modules[fixture.config.Module()],
		})
		result <- restartContinuationOutcome{
			result: response,
			err:    err,
		}
	}()
	fixture.apply(t, "internal/jobs/restart", 1)
	require.Nil(t, fixture.current)
	require.EqualValues(t, 1, fixture.resolutions.Load())
}

func (fixture *restartContinuationFixture) apply(t *testing.T, route string, generation uint64) {
	t.Helper()
	fixture.current = applyActivationTestSubmission(t, fixture.commands.next(t, route), fixture.current, generation)
}

func (fixture *restartContinuationFixture) outcome(t *testing.T) restartContinuationOutcome {
	t.Helper()
	select {
	case outcome := <-fixture.result:
		return outcome
	case <-time.After(time.Second):
		t.Fatal("RESTART observation did not settle")
		return restartContinuationOutcome{}
	}
}

func (fixture *restartContinuationFixture) running(t *testing.T) {
	t.Helper()
	fixture.apply(t, "internal/jobs/runtime-ready", 0)
	record, exists := fixture.graph.Lookup(fixture.config.FullName())
	require.True(t, exists)
	require.Equal(t, dyncfg.StatusRunning.String(), record.Status)
	outcome := fixture.outcome(t)
	require.NoError(t, outcome.err)
	require.Equal(t, messageResult(200, ""), outcome.result)
}

func TestRestartContinuationDependencyReconstruction(t *testing.T) {
	for _, rebuilds := range []int{0, 1, 3} {
		t.Run(fmt.Sprintf("rebuilds=%d", rebuilds), func(t *testing.T) {
			fixture := restartContinuationNew(t, nil)
			fixture.start(t)
			generation := uint64(2)
			for i := 0; i < rebuilds; i++ {
				activation := fixture.commands.next(t, "internal/jobs/accepted-activation")
				fixture.snapshot.Load().current.Store(false)
				fixture.current = applyActivationTestSubmission(t, activation, nil, generation)
				require.Nil(t, fixture.current)
				fixture.apply(t, "internal/jobs/candidate-resume", 0)
				generation++
			}
			fixture.apply(t, "internal/jobs/accepted-activation", generation)
			require.NotNil(t, fixture.current)
			fixture.running(t)
			require.EqualValues(t, rebuilds+1, fixture.resolutions.Load())
		})
	}
}

func TestRestartContinuationDeferredProbeFailure(t *testing.T) {
	for _, test := range []struct {
		name    string
		failure error
		status  int
		retry   bool
	}{
		{name: "permanent", failure: collectorapi.PermanentError(errors.New("probe refused")), status: 422},
		{name: "unclassified", failure: errors.New("probe refused"), status: 422},
		{name: "temporary retry", failure: collectorapi.TemporaryError(errors.New("probe refused")), status: 503, retry: true},
	} {
		t.Run(test.name, func(t *testing.T) {
			fixture := restartContinuationNew(t, func(ordinal int32) *runtimeTestCollector {
				collector := restartContinuationCollector()
				if ordinal == 2 {
					collector.factoryTestV2 = &factoryTestV2{
						checkErr: test.failure,
					}
				}
				return collector
			})
			fixture.config.Set("autodetection_retry", 1)
			// Seed the public config again before RESTART chooses its stored payload.
			seedDynCfgJobGraphRecord(t, fixture.controller, fixture.graph, fixture.config, dyncfg.StatusFailed)
			fixture.start(t)
			activation := fixture.commands.next(t, "internal/jobs/accepted-activation")
			fixture.snapshot.Load().current.Store(false)
			fixture.current = applyActivationTestSubmission(t, activation, nil, 2)
			fixture.apply(t, "internal/jobs/candidate-resume", 0)
			fixture.apply(t, "internal/jobs/accepted-activation", 3)
			require.Nil(t, fixture.current)
			record, exists := fixture.graph.Lookup(fixture.config.FullName())
			require.True(t, exists)
			require.Equal(t, dyncfg.StatusFailed.String(), record.Status)
			outcome := fixture.outcome(t)
			require.NoError(t, outcome.err)
			require.Equal(
				t,
				messageResult(
					test.status,
					"job restart failed: job output: collector lifecycle failed; resolved configuration details redacted",
				),
				outcome.result,
			)
			if test.retry {
				// Advance the real scheduler clock, dispatch its exact scheduled wake,
				// and let a new construction succeed after the original terminal reply.
				require.NoError(t, fixture.controller.scheduler.Tick(t.Context(), 100))
				require.NoError(t, fixture.controller.scheduler.Tick(t.Context(), 102))
				fixture.apply(t, "internal/jobs/autodetection-retry", 0)
				fixture.apply(t, "internal/jobs/accepted-activation", 4)
				fixture.apply(t, "internal/jobs/runtime-ready", 0)
				record, exists = fixture.graph.Lookup(fixture.config.FullName())
				require.True(t, exists)
				require.Equal(t, dyncfg.StatusRunning.String(), record.Status)
			}
		})
	}
}

func TestRestartContinuationStorePause(t *testing.T) {
	for _, installed := range []bool{false, true} {
		t.Run(fmt.Sprintf("installed=%t", installed), func(t *testing.T) {
			fixture := restartContinuationNew(t, nil)
			fixture.start(t)
			stale := fixture.commands.next(t, "internal/jobs/accepted-activation")
			var staleReady activationTestSubmission
			if installed {
				fixture.current = applyActivationTestSubmission(t, stale, nil, 2)
				require.NotNil(t, fixture.current)
				staleReady = fixture.commands.next(t, "internal/jobs/runtime-ready")
			}
			stop, stopped, err := fixture.controller.PlanSecretDependentStop(fixture.config.FullName())
			require.NoError(t, err)
			fixture.current = applyActivationTestSubmission(t, activationTestSubmission{
				plan: stop,
			}, fixture.current, 0)
			didStop, err := stopped.Stopped()
			require.NoError(t, err)
			require.True(t, didStop)
			require.Nil(t, fixture.current)
			fixture.snapshot.Load().current.Store(false)
			start, _, err := fixture.controller.PlanSecretDependentStart(fixture.config.FullName())
			require.NoError(t, err)
			fixture.current = applyActivationTestSubmission(t, activationTestSubmission{
				plan: start,
			}, nil, 0)
			if !installed {
				fixture.current = applyActivationTestSubmission(t, stale, nil, 3)
			}
			// The old installed generation's queued readiness cannot complete the
			// paused operation; it runs before the new generation is installed.
			if installed {
				fixture.current = applyActivationTestSubmission(t, staleReady, nil, 0)
			}
			for fixture.current == nil {
				call := restartContinuationNext(t, fixture.commands)
				fixture.current = applyActivationTestSubmission(t, call, nil, 4)
			}
			fixture.running(t)
		})
	}
}

func restartContinuationNext(t *testing.T, commands *activationTestCommands) activationTestSubmission {
	t.Helper()
	select {
	case call := <-commands.queue:
		return call
	case <-time.After(time.Second):
		t.Fatal("missing continuation transaction")
		return activationTestSubmission{}
	}
}

func TestRestartContinuationInterruption(t *testing.T) {
	for _, mode := range []string{"rejected", "disposed", "canceled"} {
		t.Run(mode, func(t *testing.T) {
			var cancelPreflight context.CancelFunc
			fixture := restartContinuationNew(t, func(ordinal int32) *runtimeTestCollector {
				collector := restartContinuationCollector()
				if ordinal == 2 && mode == "canceled" {
					collector.factoryTestV2 = &factoryTestV2{
						check: func(context.Context) error { cancelPreflight(); return context.Canceled },
					}
				}
				if ordinal == 2 && mode == "rejected" {
					collector.factoryTestV2 = &factoryTestV2{
						checkErr: collectorapi.PermanentError(errors.New("edit refused")),
					}
				}
				return collector
			})
			fixture.start(t)
			stale := fixture.commands.next(t, "internal/jobs/accepted-activation")
			id := fixture.config.FullName()
			permit, _ := issueTestJobPermit(t, id, 3)
			preflightCtx, cancel := context.WithCancel(t.Context())
			defer cancel()
			cancelPreflight = cancel
			prepared, err := fixture.controller.Prepare(preflightCtx, DynCfgJobRequest{
				Args:         []string{fixture.controller.externalID(id), "update"},
				Payload:      []byte(`{"option_str":"edited"}`),
				HasPayload:   true,
				ContentType:  "application/json",
				CallerSource: "user",
			}, nil, lifecycle.ResourceTransactionScope{
				ID: id,
				Successor: lifecycle.ResourceIdentity{
					ID:         id,
					Generation: 3,
				},
			}, permit)
			if mode == "canceled" {
				require.ErrorIs(t, err, context.Canceled)
			} else {
				require.NoError(t, err)
				if mode == "disposed" {
					_, err = prepared.Dispose(t.Context())
					require.NoError(t, err)
				} else {
					applied, applyErr := prepared.Apply(t.Context())
					require.NoError(t, applyErr)
					require.Equal(t, 422, applied.ResultStatus())
				}
			}
			fixture.current = applyActivationTestSubmission(t, stale, nil, 2)
			fixture.apply(t, "internal/jobs/candidate-resume", 0)
			fixture.apply(t, "internal/jobs/accepted-activation", 4)
			require.NotNil(t, fixture.current)
			fixture.running(t)
		})
	}
}

func TestRestartContinuationRevocationAndDetachment(t *testing.T) {
	for _, mode := range []string{"disable", "run stop", "disable during interruption", "run stop during interruption", "caller cancel"} {
		t.Run(mode, func(t *testing.T) {
			fixture := restartContinuationNew(t, nil)
			fixture.start(t)
			stale := fixture.commands.next(t, "internal/jobs/accepted-activation")
			if mode == "caller cancel" {
				fixture.cancel()
				outcome := fixture.outcome(t)
				require.ErrorIs(t, outcome.err, context.Canceled)
				fixture.current = applyActivationTestSubmission(t, stale, nil, 2)
				fixture.apply(t, "internal/jobs/runtime-ready", 0)
				record, _ := fixture.graph.Lookup(fixture.config.FullName())
				require.Equal(t, dyncfg.StatusRunning.String(), record.Status)
				return
			}
			if mode == "disable during interruption" || mode == "run stop during interruption" {
				id := fixture.config.FullName()
				permit, _ := issueTestJobPermit(t, id, 3)
				prepared, err := fixture.controller.Prepare(t.Context(), DynCfgJobRequest{
					Args:         []string{fixture.controller.externalID(id), "update"},
					Payload:      []byte(`{"option_str":"unaccepted edit"}`),
					HasPayload:   true,
					ContentType:  "application/json",
					CallerSource: "user",
				}, nil, lifecycle.ResourceTransactionScope{
					ID: id,
					Successor: lifecycle.ResourceIdentity{
						ID:         id,
						Generation: 3,
					},
				}, permit)
				require.NoError(t, err)
				_, err = prepared.Dispose(t.Context())
				require.NoError(t, err)
			}
			if mode == "disable" || mode == "disable during interruption" {
				prepared, err := fixture.controller.Prepare(t.Context(), DynCfgJobRequest{
					Args: []string{fixture.controller.externalID(fixture.config.FullName()), "disable"},
				}, nil, lifecycle.ResourceTransactionScope{
					ID: fixture.config.FullName(),
				}, lifecycle.LongLivedPermit{})
				require.NoError(t, err)
				_, err = prepared.Apply(t.Context())
				require.NoError(t, err)
			} else {
				fixture.controller.scheduler.StopBackgroundWorkers()
			}
			outcome := fixture.outcome(t)
			require.NoError(t, outcome.err)
			require.Equal(t, rejectedResult(jobFailure{
				class:   failureUnavailable,
				message: "the restarted job was stopped or replaced before becoming ready.",
			}), outcome.result)
			fixture.current = applyActivationTestSubmission(t, stale, nil, 2)
			require.Nil(t, fixture.current, "late queued activation cannot resurrect revoked intent")
		})
	}
}

type restartContinuationDecodingCollector struct {
	*runtimeTestCollector
	Integer  int              `yaml:"option_int"`
	Duration confopt.Duration `yaml:"duration"`
}

func TestRestartContinuationRejectedSourceDisplacement(t *testing.T) {
	fixture := restartContinuationNew(t, nil)
	creator := fixture.controller.modules["module"]
	create := creator.CreateV2
	creator.CreateV2 = func() collectorapi.CollectorV2 {
		return &restartContinuationDecodingCollector{
			runtimeTestCollector: create().(*runtimeTestCollector),
		}
	}
	fixture.controller.modules["module"] = creator
	fixture.config.SetSourceType(confgroup.TypeStock).SetSource("stock.conf")
	inventory := sourceTestInventory(fixture.controller, fixture.config, false, false)
	plan, err := planInventoryTest(fixture.controller, inventory)
	require.NoError(t, err)
	fixture.current = applyActivationTestSubmission(t, activationTestSubmission{
		plan: plan,
	}, nil, 0)
	seedDynCfgJobGraphRecord(t, fixture.controller, fixture.graph, fixture.config, dyncfg.StatusFailed)
	fixture.start(t)
	stale := fixture.commands.next(t, "internal/jobs/accepted-activation")

	// A valid source identity with a real collector decoding error displaces the
	// physical executor, but cannot replace the admitted stored configuration.
	competitor, err := fixture.config.Clone()
	require.NoError(t, err)
	competitor.SetSourceType(confgroup.TypeUser).SetSource("user.conf").Set("option_int", "invalid integer")
	inventory = sourceTestInventory(fixture.controller, competitor, true, false)
	plan, err = planInventoryTest(fixture.controller, inventory)
	require.NoError(t, err)
	fixture.current = applyActivationTestSubmission(t, activationTestSubmission{
		plan: plan,
	}, nil, 0)
	fixture.current = applyActivationTestSubmission(t, stale, nil, 2)
	fixture.apply(t, "internal/jobs/accepted-activation", 3)
	fixture.apply(t, "internal/jobs/candidate-resume", 0)
	fixture.apply(t, "internal/jobs/accepted-activation", 4)
	require.NotNil(t, fixture.current)
	fixture.running(t)
	record, exists := fixture.graph.Lookup(fixture.config.FullName())
	require.True(t, exists)
	stored, err := graphRecordConfig(record)
	require.NoError(t, err)
	require.Equal(t, fixture.config.UID(), stored.UID())
}

func TestRestartContinuationDeferredInvalidResolvedConfig(t *testing.T) {
	fixture := restartContinuationNew(t, nil)
	creator := fixture.controller.modules["module"]
	create := creator.CreateV2
	creator.CreateV2 = func() collectorapi.CollectorV2 {
		return &restartContinuationDecodingCollector{
			runtimeTestCollector: create().(*runtimeTestCollector),
		}
	}
	fixture.controller.modules["module"] = creator
	fixture.storeValue = func(ordinal int32) string {
		if ordinal == 1 {
			return "1s"
		}
		return "invalid duration"
	}
	fixture.config.Set("duration", "${store:vault:test:key}")
	seedDynCfgJobGraphRecord(t, fixture.controller, fixture.graph, fixture.config, dyncfg.StatusFailed)
	fixture.start(t)
	activation := fixture.commands.next(t, "internal/jobs/accepted-activation")
	fixture.snapshot.Load().current.Store(false)
	fixture.current = applyActivationTestSubmission(t, activation, nil, 2)
	fixture.apply(t, "internal/jobs/candidate-resume", 0)
	fixture.apply(t, "internal/jobs/accepted-activation", 3)
	require.Nil(t, fixture.current)
	record, exists := fixture.graph.Lookup(fixture.config.FullName())
	require.True(t, exists)
	require.Equal(t, dyncfg.StatusFailed.String(), record.Status)
	outcome := fixture.outcome(t)
	require.NoError(t, outcome.err)
	require.Equal(
		t,
		messageResult(400, "job output: collector lifecycle failed; resolved configuration details redacted"),
		outcome.result,
	)
}

func TestRestartContinuationDeadlineRetainsRecoveryAuthority(t *testing.T) {
	var fixture *restartContinuationFixture
	cutResult := make(chan bool, 1)
	fixture = restartContinuationNew(t, func(ordinal int32) *runtimeTestCollector {
		collector := restartContinuationCollector()
		if ordinal == 2 {
			collector.factoryTestV2 = &factoryTestV2{
				check: func(ctx context.Context) error {
					cutResult <- fixture.controller.factory.config.Attempts.CutProcessAttempt(
						jobAttemptIdentity(
							jobmgr.ProcessAttemptJob,
							fixture.config.FullName(),
						),
						jobmgr.ErrProcessAttemptDeadline,
					)
					<-ctx.Done()
					return context.Cause(ctx)
				},
			}
		}
		return collector
	})
	fixture.config.Set("autodetection_retry", 1)
	seedDynCfgJobGraphRecord(t, fixture.controller, fixture.graph, fixture.config, dyncfg.StatusFailed)
	fixture.start(t)
	activation := fixture.commands.next(t, "internal/jobs/accepted-activation")
	fixture.snapshot.Load().current.Store(false)
	fixture.current = applyActivationTestSubmission(t, activation, nil, 2)
	fixture.apply(t, "internal/jobs/candidate-resume", 0)
	fixture.apply(t, "internal/jobs/accepted-activation", 3)
	select {
	case cut := <-cutResult:
		require.True(t, cut, "the actual contained construction was cut at its deadline")
	case <-time.After(time.Second):
		t.Fatal("collector check did not report the deadline cut")
	}
	require.Nil(t, fixture.current)
	state := fixture.controller.candidates.view(fixture.config.FullName())
	require.True(t, state.FrameworkBlocked)
	require.NotNil(t, state.Active)
	require.True(
		t,
		fixture.controller.candidates.valid(fixture.config.FullName(), state.Active.Token),
		"terminal observation does not revoke retained recovery authority",
	)
	outcome := fixture.outcome(t)
	require.NoError(t, outcome.err)
	require.Equal(t, messageResult(503, jobmgr.ErrProcessAttemptDeadline.Error()), outcome.result)
}
