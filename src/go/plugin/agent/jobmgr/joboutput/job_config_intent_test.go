// SPDX-License-Identifier: GPL-3.0-or-later

package joboutput

import (
	"context"
	"errors"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/netdata/netdata/go/plugins/plugin/agent/jobmgr/lifecycle"
	"github.com/netdata/netdata/go/plugins/plugin/framework/collectorapi"
	"github.com/netdata/netdata/go/plugins/plugin/framework/confgroup"
	"github.com/netdata/netdata/go/plugins/plugin/framework/dyncfg"
	"github.com/stretchr/testify/require"
)

type intentTestOwner struct {
	mu                           sync.Mutex
	enabled                      map[collectorapi.JobConfigIdentity]bool
	prepared, committed, aborted int
	prepareErr, abortErr         error
	onCommit                     func(collectorapi.JobConfigIdentity)
}

type intentTestReservation struct {
	owner          *intentTestOwner
	previous, next collectorapi.JobConfigIdentity
}

func (o *intentTestOwner) Prepare(
	previous, next collectorapi.JobConfigIdentity,
	_ map[string]any,
) (collectorapi.PreparedJobConfigIntent, error) {
	o.mu.Lock()
	defer o.mu.Unlock()
	if o.prepareErr != nil {
		return nil, o.prepareErr
	}
	o.prepared++
	return &intentTestReservation{
		owner:    o,
		previous: previous,
		next:     next,
	}, nil
}
func (r *intentTestReservation) Commit() {
	o := r.owner
	o.mu.Lock()
	o.committed++
	delete(o.enabled, r.previous)
	if r.next.Valid() {
		if o.enabled == nil {
			o.enabled = make(map[collectorapi.JobConfigIdentity]bool)
		}
		o.enabled[r.next] = true
	}
	o.mu.Unlock()
	if o.onCommit != nil {
		o.onCommit(r.next)
	}
}
func (r *intentTestReservation) Abort() error {
	r.owner.mu.Lock()
	defer r.owner.mu.Unlock()
	r.owner.aborted++
	return r.owner.abortErr
}
func (o *intentTestOwner) snapshot() (int, int, int, int) {
	o.mu.Lock()
	defer o.mu.Unlock()
	return o.prepared, o.committed, o.aborted, len(o.enabled)
}
func installIntentTestOwner(controller *DynCfgJobController, owner *intentTestOwner) {
	creator := controller.modules["module"]
	creator.JobConfigIntent = owner
	controller.modules["module"] = creator
}

func prepareIntentTestEnable(
	t *testing.T,
	controller *DynCfgJobController,
	config confgroup.Config,
) (lifecycle.PreparedResourceTransaction, error) {
	t.Helper()
	return controller.Prepare(
		t.Context(),
		DynCfgJobRequest{
			Args: []string{controller.externalID(config.FullName()), "enable"},
		},
		nil,
		lifecycle.ResourceTransactionScope{
			ID: config.FullName(),
		},
		lifecycle.LongLivedPermit{},
	)
}

func TestConfigIntentPassiveEnableDisposeAndRetirement(t *testing.T) {
	controller, graph, _, _, _ := newDynCfgJobTestHarness(t)
	owner := &intentTestOwner{}
	installIntentTestOwner(controller, owner)
	config := acceptedActivationTestConfig(confgroup.TypeUser)
	plan, err := controller.PlanDiscovered(DiscoveredJobChange{
		Config: config,
		Status: dyncfg.StatusAccepted,
	})
	require.NoError(t, err)
	applyActivationTestSubmission(t, activationTestSubmission{
		plan: plan,
	}, nil, 0)
	prepared, committed, aborted, enabled := owner.snapshot()
	require.Equal(t, []int{0, 0, 0, 0}, []int{prepared, committed, aborted, enabled})

	transaction, err := prepareIntentTestEnable(t, controller, config)
	require.NoError(t, err)
	prepared, committed, aborted, enabled = owner.snapshot()
	require.Equal(t, []int{1, 0, 0, 0}, []int{prepared, committed, aborted, enabled})
	_, err = transaction.Dispose(t.Context())
	require.NoError(t, err)
	prepared, committed, aborted, enabled = owner.snapshot()
	require.Equal(t, []int{1, 0, 1, 0}, []int{prepared, committed, aborted, enabled})

	owner.onCommit = func(id collectorapi.JobConfigIdentity) {
		if id.Valid() {
			record, exists := graph.Lookup(config.FullName())
			require.True(t, exists)
			require.Equal(t, dyncfg.StatusAccepted.String(), record.Status)
			require.False(t, controller.ActivationEnabled(config.FullName()), "intent precedes activation")
		}
	}
	transaction, err = prepareIntentTestEnable(t, controller, config)
	require.NoError(t, err)
	applied, err := transaction.Apply(t.Context())
	require.NoError(t, err)
	require.Equal(t, 202, applied.ResultStatus())
	transaction, err = prepareIntentTestEnable(t, controller, config)
	require.NoError(t, err)
	_, err = transaction.Apply(t.Context())
	require.NoError(t, err)
	prepared, committed, aborted, enabled = owner.snapshot()
	require.Equal(
		t,
		[]int{2, 1, 1, 1},
		[]int{prepared, committed, aborted, enabled},
		"same identity needs no new reservation",
	)
	owner.prepareErr = errors.New("retirement unavailable")
	require.Error(t, controller.FinalizeJobConfigIntents())
	_, _, _, enabled = owner.snapshot()
	require.Equal(t, 1, enabled, "failed retirement preserves ownership")
	owner.prepareErr = nil
	require.NoError(t, controller.FinalizeJobConfigIntents())
	require.NoError(t, controller.FinalizeJobConfigIntents())
	prepared, committed, aborted, enabled = owner.snapshot()
	require.Equal(t, []int{3, 2, 1, 0}, []int{prepared, committed, aborted, enabled})
}

func TestConfigIntentEnableFailureAndApplyRollback(t *testing.T) {
	for _, mode := range []string{"prepare failure", "nil-context apply rollback", "abort failure"} {
		t.Run(mode, func(t *testing.T) {
			controller, graph, _, _, _ := newDynCfgJobTestHarness(t)
			owner := &intentTestOwner{}
			installIntentTestOwner(controller, owner)
			config := acceptedActivationTestConfig(confgroup.TypeUser)
			seedDynCfgJobGraphRecord(t, graph, config, dyncfg.StatusDisabled)
			sentinel := errors.New("storage unavailable")
			if mode == "prepare failure" {
				owner.prepareErr = sentinel
			}
			transaction, err := prepareIntentTestEnable(t, controller, config)
			if mode == "prepare failure" {
				require.ErrorIs(t, err, sentinel)
			} else {
				require.NoError(t, err)
				if mode == "abort failure" {
					owner.abortErr = sentinel
				}
				_, err = transaction.Apply(nil)
				require.Error(t, err)
				if mode == "abort failure" {
					require.True(t, lifecycle.OwnershipRetained(err))
					require.ErrorIs(t, err, sentinel)
				}
			}
			record, _ := graph.Lookup(config.FullName())
			require.Equal(t, dyncfg.StatusDisabled.String(), record.Status)
			_, committed, _, enabled := owner.snapshot()
			require.Zero(t, committed)
			require.Zero(t, enabled)
		})
	}
}

func TestConfigIntentNamedWaitRunningFailureAndDisable(t *testing.T) {
	for _, ending := range []string{"disable", "remove", "joined retirement"} {
		t.Run(ending, func(t *testing.T) {
			controller, graph, _, _, _ := newDynCfgJobTestHarness(t)
			failure := make(chan struct{})
			configureRuntimeTestCollector(controller, func() *runtimeTestCollector {
				return &runtimeTestCollector{
					run: func(ctx context.Context, ready func()) error {
						ready()
						select {
						case <-failure:
							return collectorapi.TemporaryError(errors.New("runtime failed"))
						case <-ctx.Done():
							return ctx.Err()
						}
					},
				}
			})
			owner := &intentTestOwner{}
			installIntentTestOwner(controller, owner)
			var available atomic.Bool
			creator := controller.modules["module"]
			creator.ActivationDependencies = func(map[string]any) ([]collectorapi.ActivationDependency, error) {
				return []collectorapi.ActivationDependency{{Kind: "runtime", Name: "browser", Ready: func() error {
					if !available.Load() {
						return errors.New("preparing")
					}
					return nil
				}}}, nil
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
			var current lifecycle.ReadyResource
			var stopOnce sync.Once
			t.Cleanup(func() {
				stopOnce.Do(func() { close(commands.stop) })
				controller.scheduler.StopBackgroundWorkers()
				require.NoError(t, controller.scheduler.WaitBackgroundWorkers(context.Background()))
				stopRuntimeTestResource(t, current)
			})
			config := acceptedActivationTestConfig(confgroup.TypeDyncfg).Set("autodetection_retry", 0)
			seedDynCfgJobGraphRecord(t, graph, config, dyncfg.StatusAccepted)
			applyAcceptedEnableForTest(t, controller, graph, config, 1)
			waiting := commands.next(t, "internal/jobs/accepted-activation")
			// Notify after the failed lookup but before the waiting transaction.
			available.Store(true)
			controller.NotifyDependencyChanged("runtime", "browser")
			current = applyActivationTestSubmission(t, waiting, nil, 2)
			require.Nil(t, current)
			current = applyActivationTestSubmission(t, commands.next(t, "internal/jobs/accepted-activation"), nil, 3)
			require.NotNil(t, current)
			current = applyActivationTestSubmission(t, commands.next(t, "internal/jobs/runtime-ready"), current, 0)
			record, _ := graph.Lookup(config.FullName())
			require.Equal(t, dyncfg.StatusRunning.String(), record.Status)
			_, committed, _, enabled := owner.snapshot()
			require.Equal(t, 1, committed)
			require.Equal(t, 1, enabled, "Running retains enabled demand")
			require.False(t, controller.ActivationEnabled(config.FullName()), "transient activation ended")

			close(failure)
			current = applyActivationTestSubmission(t, commands.next(t, "internal/jobs/runtime-failure"), current, 0)
			require.Nil(t, current)
			record, _ = graph.Lookup(config.FullName())
			require.Equal(t, dyncfg.StatusFailed.String(), record.Status)
			_, committed, _, enabled = owner.snapshot()
			require.Equal(t, 1, committed)
			require.Equal(t, 1, enabled, "Failed retains enabled demand")
			if ending == "joined retirement" {
				stopOnce.Do(func() { close(commands.stop) })
				controller.scheduler.StopBackgroundWorkers()
				require.NoError(t, controller.scheduler.WaitBackgroundWorkers(context.Background()))
				require.NoError(t, controller.FinalizeJobConfigIntents())
			} else {
				transaction, err := controller.Prepare(t.Context(), DynCfgJobRequest{
					Args: []string{controller.externalID(config.FullName()), ending},
				}, nil,
					lifecycle.ResourceTransactionScope{
						ID: config.FullName(),
					}, lifecycle.LongLivedPermit{})
				require.NoError(t, err)
				_, err = transaction.Apply(t.Context())
				require.NoError(t, err)
			}
			_, committed, _, enabled = owner.snapshot()
			require.Equal(t, 2, committed)
			require.Zero(t, enabled)
		})
	}
}

func TestNamedDependenciesDoNotPrepareDuringValidationOrTest(t *testing.T) {
	controller, _, _, _, _ := newDynCfgJobTestHarness(t)
	owner := &intentTestOwner{}
	installIntentTestOwner(controller, owner)
	creator := controller.modules["module"]
	var lookups atomic.Int32
	creator.ActivationDependencies = func(map[string]any) ([]collectorapi.ActivationDependency, error) {
		return []collectorapi.ActivationDependency{
			{
				Kind:  "runtime",
				Name:  "browser",
				Ready: func() error { lookups.Add(1); return errors.New("not installed") },
			},
		}, nil
	}
	controller.modules["module"] = creator
	config := acceptedActivationTestConfig(confgroup.TypeDyncfg)
	require.NoError(t, controller.configModules.Validate(t.Context(), config))
	require.Zero(t, lookups.Load())
	err := controller.configModules.Test(t.Context(), config)
	var dependency *collectorapi.ActivationDependencyUnavailableError
	require.ErrorAs(t, err, &dependency)
	require.True(t, controller.configModules.activationWaitsForDependency(config, redactResolvedLifecycleError(err)))
	require.EqualValues(t, 1, lookups.Load())
	prepared, committed, aborted, enabled := owner.snapshot()
	require.Equal(t, []int{0, 0, 0, 0}, []int{prepared, committed, aborted, enabled})
}

func TestConfigIntentRejectedAndAbortedUpdatePreservesIncumbent(t *testing.T) {
	controller, graph, _, _, _ := newDynCfgJobTestHarness(t)
	configureRuntimeTestCollector(controller, func() *runtimeTestCollector {
		return &runtimeTestCollector{
			run: func(ctx context.Context, ready func()) error { ready(); <-ctx.Done(); return ctx.Err() },
		}
	})
	owner := &intentTestOwner{}
	installIntentTestOwner(controller, owner)
	var unavailable atomic.Bool
	creator := controller.modules["module"]
	creator.ActivationDependencies = func(map[string]any) ([]collectorapi.ActivationDependency, error) {
		return []collectorapi.ActivationDependency{{Kind: "runtime", Name: "browser", Ready: func() error {
			if unavailable.Load() {
				return errors.New("unavailable")
			}
			return nil
		}}}, nil
	}
	controller.modules["module"] = creator
	config := acceptedActivationTestConfig(confgroup.TypeDyncfg)
	current := runtimeTestStartAndSettle(t, controller, config, nil, 1)
	t.Cleanup(func() { stopRuntimeTestResource(t, current) })
	before, _ := graph.Lookup(config.FullName())
	request := DynCfgJobRequest{
		Args: []string{controller.externalID(config.FullName()), "update"},
		Payload: []byte(
			`{"option_str":"replacement"}`,
		),
		HasPayload:   true,
		ContentType:  "application/json",
		CallerSource: "user",
	}
	for i, mode := range []string{"unavailable candidate", "reservation failure", "aborted adoption", "committed adoption"} {
		t.Run(mode, func(t *testing.T) {
			unavailable.Store(mode == "unavailable candidate")
			if mode == "reservation failure" {
				owner.prepareErr = errors.New("cannot reserve")
			} else {
				owner.prepareErr = nil
			}
			generation := uint64(10 + i)
			permit, _ := issueTestJobPermit(t, config.FullName(), generation)
			scope := lifecycle.ResourceTransactionScope{
				ID:      config.FullName(),
				Current: current.Identity(),
				Successor: lifecycle.ResourceIdentity{
					ID:         config.FullName(),
					Generation: generation,
				},
			}
			prepared, err := controller.Prepare(t.Context(), request, current, scope, permit)
			if mode == "reservation failure" {
				require.ErrorContains(t, err, "cannot reserve")
				require.NoError(t, permit.AbortUnused())
			} else {
				require.NoError(t, err)
				if mode == "aborted adoption" {
					kept, err := prepared.Dispose(t.Context())
					require.NoError(t, err)
					require.Same(t, current, kept)
				} else {
					applied, err := prepared.Apply(t.Context())
					require.NoError(t, err)
					_, _, next := applied.Ownership()
					if mode == "unavailable candidate" {
						require.Equal(t, 503, applied.ResultStatus())
						require.Same(t, current, next)
					} else {
						current = next
						require.Equal(t, 202, applied.ResultStatus())
					}
				}
			}
			if mode != "committed adoption" {
				after, _ := graph.Lookup(config.FullName())
				require.Equal(t, before, after)
				_, committed, _, enabled := owner.snapshot()
				require.Equal(t, 1, committed)
				require.Equal(t, 1, enabled)
			}
		})
	}
	prepared, committed, aborted, enabled := owner.snapshot()
	require.Equal(t, []int{3, 2, 1, 1}, []int{prepared, committed, aborted, enabled})
}

func TestConfigIntentStartupRetryKeepsOneDemand(t *testing.T) {
	controller, graph, _, _, _ := newDynCfgJobTestHarness(t)
	var runs atomic.Int32
	configureRuntimeTestCollector(controller, func() *runtimeTestCollector {
		return &runtimeTestCollector{
			run: func(ctx context.Context, ready func()) error {
				if runs.Add(1) == 1 {
					return collectorapi.TemporaryError(errors.New("startup unavailable"))
				}
				ready()
				<-ctx.Done()
				return ctx.Err()
			},
		}
	})
	owner := &intentTestOwner{}
	installIntentTestOwner(controller, owner)
	commands := &activationTestCommands{
		queue: make(chan activationTestSubmission, 16),
		stop:  make(chan struct{}),
	}
	require.NoError(
		t,
		controller.BindBackgroundWorkers(commands, 9, func(err error) { t.Errorf("background failure: %v", err) }),
	)
	var current lifecycle.ReadyResource
	t.Cleanup(func() {
		close(commands.stop)
		controller.scheduler.StopBackgroundWorkers()
		require.NoError(t, controller.scheduler.WaitBackgroundWorkers(context.Background()))
		stopRuntimeTestResource(t, current)
	})
	require.NoError(t, controller.scheduler.Tick(t.Context(), 0))
	config := acceptedActivationTestConfig(confgroup.TypeUser).Set("autodetection_retry", 1)
	seedDynCfgJobGraphRecord(t, graph, config, dyncfg.StatusDisabled)
	applyAcceptedEnableForTest(t, controller, graph, config, 1)
	current = applyActivationTestSubmission(t, commands.next(t, "internal/jobs/accepted-activation"), nil, 2)
	sawFailed, sawRetry := false, false
	deadline := time.After(3 * time.Second)
	for {
		var call activationTestSubmission
		select {
		case call = <-commands.queue:
		case <-deadline:
			t.Fatal("startup retry did not reach Running")
		}
		var generation uint64
		switch call.request.Route {
		case "internal/jobs/autodetection-retry":
			require.True(t, sawFailed)
			sawRetry = true
		case "internal/jobs/accepted-activation":
			generation = 3
		case "internal/jobs/runtime-ready", "internal/jobs/runtime-failure":
		default:
			t.Fatalf("unexpected route %s", call.request.Route)
		}
		current = applyActivationTestSubmission(t, call, current, generation)
		record, _ := graph.Lookup(config.FullName())
		if record.Status == dyncfg.StatusFailed.String() && !sawFailed {
			sawFailed = true
			require.NoError(t, controller.scheduler.Tick(t.Context(), 1))
		}
		if record.Status == dyncfg.StatusRunning.String() {
			break
		}
	}
	require.True(t, sawFailed)
	require.True(t, sawRetry)
	prepared, committed, aborted, enabled := owner.snapshot()
	require.Equal(t, []int{1, 1, 0, 1}, []int{prepared, committed, aborted, enabled})
}
