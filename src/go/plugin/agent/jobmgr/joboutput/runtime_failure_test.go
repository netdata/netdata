// SPDX-License-Identifier: GPL-3.0-or-later

package joboutput

import (
	"context"
	"errors"
	"fmt"
	"net"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/netdata/netdata/go/plugins/pkg/metrix"
	"github.com/netdata/netdata/go/plugins/plugin/agent/jobmgr"
	"github.com/netdata/netdata/go/plugins/plugin/agent/jobmgr/containment"
	"github.com/netdata/netdata/go/plugins/plugin/agent/jobmgr/lifecycle"
	"github.com/netdata/netdata/go/plugins/plugin/framework/collectorapi"
	"github.com/netdata/netdata/go/plugins/plugin/framework/confgroup"
	"github.com/netdata/netdata/go/plugins/plugin/framework/dyncfg"
	"github.com/stretchr/testify/require"
)

type runtimeTestCollector struct {
	*factoryTestV2
	run     func(context.Context, func()) error
	cleanup func()
}

func (c *runtimeTestCollector) Run(ctx context.Context, ready func()) error { return c.run(ctx, ready) }
func (c *runtimeTestCollector) Cleanup(context.Context) {
	if c.cleanup != nil {
		c.cleanup()
	}
}

func configureRuntimeTestCollector(controller *DynCfgJobController, create func() *runtimeTestCollector) {
	creator := controller.modules["module"]
	creator.Create = nil
	creator.CreateV2 = func() collectorapi.CollectorV2 {
		c := create()
		if c.factoryTestV2 == nil {
			c.factoryTestV2 = &factoryTestV2{}
		}
		c.store = metrix.NewCollectorStore()
		c.template = factoryTestChartTemplate
		return c
	}
	controller.modules["module"] = creator
}

// runtimeTestBindActivations runs the actual accepted owner while tests retain
// control of each submitted transaction and its terminal acknowledgement.
func runtimeTestBindActivations(t *testing.T, controller *DynCfgJobController) *activationTestCommands {
	t.Helper()
	index := controller.scheduler.accepted
	index.mu.Lock()
	bound, existing := index.bound, index.commands
	index.mu.Unlock()
	if bound {
		commands, ok := existing.(*activationTestCommands)
		require.True(t, ok, "runtime fixture needs the controlled activation command port")
		return commands
	}
	commands := &activationTestCommands{queue: make(chan activationTestSubmission, 8), stop: make(chan struct{})}
	require.NoError(t, index.bind(controller.factory, commands, controller.planAcceptedActivation, 9, func(err error) { t.Errorf("activation dispatch: %v", err) }))
	t.Cleanup(func() {
		close(commands.stop)
		index.stopWorker()
		ctx, cancel := context.WithTimeout(context.Background(), time.Second)
		defer cancel()
		require.NoError(t, index.wait(ctx))
	})
	return commands
}

type runtimeTestActivationTransaction struct {
	lifecycle.PreparedResourceTransaction
	ack chan error
}

func (transaction *runtimeTestActivationTransaction) Apply(ctx context.Context) (lifecycle.AppliedResourceTransaction, error) {
	applied, err := transaction.PreparedResourceTransaction.Apply(ctx)
	transaction.ack <- err
	return applied, err
}

func (transaction *runtimeTestActivationTransaction) Dispose(ctx context.Context) (lifecycle.ReadyResource, error) {
	current, err := transaction.PreparedResourceTransaction.Dispose(ctx)
	transaction.ack <- err
	return current, err
}

func runtimeTestPrepareActivation(t *testing.T, commands *activationTestCommands, id string, generation uint64) lifecycle.PreparedResourceTransaction {
	t.Helper()
	call := commands.next(t, "internal/jobs/accepted-activation")
	require.Equal(t, id, call.plan.Transaction.ID)
	permit, _ := issueTestJobPermit(t, id, generation)
	scope := lifecycle.ResourceTransactionScope{ID: id, Successor: lifecycle.ResourceIdentity{ID: id, Generation: generation}}
	prepared, err := call.plan.Transaction.Prepare(t.Context(), nil, scope, permit)
	require.NoError(t, err)
	return &runtimeTestActivationTransaction{PreparedResourceTransaction: prepared, ack: call.ack}
}

func runtimeTestApplyActivation(t *testing.T, commands *activationTestCommands, id string, generation uint64) lifecycle.ReadyResource {
	t.Helper()
	return runtimeTestApply(t, runtimeTestPrepareActivation(t, commands, id, generation))
}

func prepareRuntimeTestAdoption(t *testing.T, controller *DynCfgJobController, cfg confgroup.Config, current lifecycle.ReadyResource, generation uint64) lifecycle.PreparedResourceTransaction {
	t.Helper()
	plan, err := controller.PlanDiscovered(DiscoveredJobChange{Config: cfg, Status: dyncfg.StatusRunning, Restart: true})
	require.NoError(t, err)
	permit, _ := issueTestJobPermit(t, cfg.FullName(), generation)
	scope := lifecycle.ResourceTransactionScope{ID: cfg.FullName(), Successor: lifecycle.ResourceIdentity{ID: cfg.FullName(), Generation: generation}}
	if current != nil {
		scope.Current = current.Identity()
	}
	prepared, err := plan.Transaction.Prepare(t.Context(), current, scope, permit)
	require.NoError(t, err)
	return prepared
}

// Each logical fixture change uses separate adoption and activation generations.
// Acceptance is applied before returning the real installation transaction.
func prepareRuntimeTestChange(t *testing.T, controller *DynCfgJobController, cfg confgroup.Config, current lifecycle.ReadyResource, generation uint64) lifecycle.PreparedResourceTransaction {
	t.Helper()
	commands := runtimeTestBindActivations(t, controller)
	require.Nil(t, runtimeTestApply(t, prepareRuntimeTestAdoption(t, controller, cfg, current, 2*generation-1)))
	return runtimeTestPrepareActivation(t, commands, cfg.FullName(), 2*generation)
}

func runtimeTestApply(t *testing.T, prepared lifecycle.PreparedResourceTransaction) lifecycle.ReadyResource {
	t.Helper()
	applied, err := prepared.Apply(t.Context())
	require.NoError(t, err)
	_, _, current := applied.Ownership()
	return current
}

func stopRuntimeTestResource(t *testing.T, resource lifecycle.ReadyResource) {
	t.Helper()
	if resource != nil {
		require.NoError(t, resource.Stop(context.Background()))
		require.NoError(t, resource.Finalize())
	}
}

func runtimeTestHasRetry(controller *DynCfgJobController, id string) bool {
	controller.scheduler.retries.mu.Lock()
	defer controller.scheduler.retries.mu.Unlock()
	return controller.scheduler.retries.entries[id] != nil
}

func TestRuntimeStartupFailureIsOperationalAndKeepsConfiguredRetry(t *testing.T) {
	for _, test := range []struct {
		name       string
		run        func(context.Context, func()) error
		retryEvery int
		wantRetry  bool
	}{
		{"error retry enabled", func(context.Context, func()) error { return errors.New("listen 127.0.0.1:8125: address in use") }, 7, true},
		{"error retry disabled", func(context.Context, func()) error { return errors.New("bind failed") }, 0, false},
		{"permanent error", func(context.Context, func()) error {
			return collectorapi.PermanentError(errors.New("invalid listen address"))
		}, 7, false},
		{"unexpected nil", func(context.Context, func()) error { return nil }, 7, false},
		{"recovered panic", func(context.Context, func()) error { panic("bind panic") }, 7, false},
	} {
		t.Run(test.name, func(t *testing.T) {
			controller, graph, _, _, _ := newDynCfgJobTestHarness(t)
			var cleaned atomic.Int32
			configureRuntimeTestCollector(controller, func() *runtimeTestCollector {
				return &runtimeTestCollector{run: test.run, cleanup: func() { cleaned.Add(1) }}
			})
			cfg := factoryTestConfig(false).Set("autodetection_retry", test.retryEvery)
			commands := runtimeTestNotifications(t, controller)
			applied, err := prepareRuntimeTestChange(t, controller, cfg, nil, 1).Apply(t.Context())
			require.NoError(t, err, "collector failure must not become a transaction/manager failure")
			_, _, current := applied.Ownership()
			require.NotNil(t, current, "startup is accepted before settlement")
			current = runtimeTestApplyNotification(t, runtimeTestNotificationPlan(t, commands, "internal/jobs/runtime-ready"), current)
			require.Nil(t, current)
			record, ok := graph.Lookup(cfg.FullName())
			require.True(t, ok)
			require.Equal(t, dyncfg.StatusFailed.String(), record.Status)
			require.Equal(t, test.wantRetry, runtimeTestHasRetry(controller, cfg.FullName()))
			requireFactoryAttemptsIdle(t, controller.factory)
			require.EqualValues(t, 1, cleaned.Load())

			// A cleanly released operational failure, including panic, allows restart.
			configureRuntimeTestCollector(controller, func() *runtimeTestCollector {
				return &runtimeTestCollector{run: func(ctx context.Context, ready func()) error { ready(); <-ctx.Done(); return ctx.Err() }}
			})
			current = runtimeTestStartAndSettle(t, controller, cfg, nil, 2)
			require.NotNil(t, current)
			require.False(t, runtimeTestHasRetry(controller, cfg.FullName()))
			stopRuntimeTestResource(t, current)
			requireFactoryAttemptsIdle(t, controller.factory)
		})
	}
}

func TestRuntimePartialBindFailureReleasesAcquiredListener(t *testing.T) {
	controller, graph, _, _, _ := newDynCfgJobTestHarness(t)
	occupied, err := net.Listen("tcp", "127.0.0.1:0")
	require.NoError(t, err)
	defer occupied.Close()
	firstAddress := make(chan string, 1)
	configureRuntimeTestCollector(controller, func() *runtimeTestCollector {
		var first net.Listener
		return &runtimeTestCollector{
			run: func(ctx context.Context, ready func()) error {
				var err error
				first, err = net.Listen("tcp", "127.0.0.1:0")
				if err != nil {
					return err
				}
				firstAddress <- first.Addr().String()
				second, err := net.Listen("tcp", occupied.Addr().String())
				if err != nil {
					return fmt.Errorf("second listener: %w", err)
				}
				defer second.Close()
				ready()
				<-ctx.Done()
				return ctx.Err()
			},
			cleanup: func() {
				if first != nil {
					_ = first.Close()
				}
			},
		}
	})
	cfg := factoryTestConfig(false)
	require.Nil(t, runtimeTestStartAndSettle(t, controller, cfg, nil, 1))
	record, _ := graph.Lookup(cfg.FullName())
	require.Equal(t, dyncfg.StatusFailed.String(), record.Status)
	requireFactoryAttemptsIdle(t, controller.factory)
	released, err := net.Listen("tcp", <-firstAddress)
	require.NoError(t, err, "partial startup must release the first acquired listener")
	require.NoError(t, released.Close())
}

func TestRuntimeReplacementWaitsForPhysicalCleanupAndConfigTestDoesNotRun(t *testing.T) {
	controller, _, _, _, _ := newDynCfgJobTestHarness(t)
	address := make(chan string, 1)
	cleanupEntered, releaseCleanup, successorEntered := make(chan struct{}), make(chan struct{}), make(chan struct{})
	var cleanupRelease sync.Once
	defer cleanupRelease.Do(func() { close(releaseCleanup) })
	var creates atomic.Int32
	configureRuntimeTestCollector(controller, func() *runtimeTestCollector {
		number := creates.Add(1)
		var listener net.Listener
		return &runtimeTestCollector{
			run: func(ctx context.Context, ready func()) error {
				endpoint := "127.0.0.1:0"
				if number >= 3 {
					close(successorEntered)
					endpoint = <-address
				}
				var err error
				listener, err = net.Listen("tcp", endpoint)
				if err != nil {
					return err
				}
				if number == 1 {
					address <- listener.Addr().String()
				}
				ready()
				<-ctx.Done()
				return ctx.Err()
			},
			cleanup: func() {
				if number == 1 {
					close(cleanupEntered)
					<-releaseCleanup
				}
				if listener != nil {
					_ = listener.Close()
				}
			},
		}
	})
	cfg := factoryTestConfig(false)
	current := runtimeTestStartAndSettle(t, controller, cfg, nil, 1)
	require.NotNil(t, current)
	require.NoError(t, controller.configModules.Test(t.Context(), cfg), "configuration test must not bind the active endpoint")
	commands := runtimeTestBindActivations(t, controller)
	prepared := prepareRuntimeTestAdoption(t, controller, cfg, current, 3)
	type result struct {
		applied lifecycle.AppliedResourceTransaction
		err     error
	}
	done := make(chan result, 1)
	go func() { applied, err := prepared.Apply(t.Context()); done <- result{applied, err} }()
	select {
	case <-cleanupEntered:
	case <-time.After(time.Second):
		t.Fatal("old cleanup did not start")
	}
	select {
	case <-successorEntered:
		t.Fatal("successor started before predecessor cleanup")
	default:
	}
	var resultValue result
	select {
	case resultValue = <-done:
	case <-time.After(time.Second):
		t.Fatal("accepted replacement waited for predecessor cleanup")
	}
	cleanupRelease.Do(func() { close(releaseCleanup) })
	require.NoError(t, resultValue.err)
	_, _, next := resultValue.applied.Ownership()
	require.Nil(t, next, "busy physical identity keeps the replacement pending")
	notifications := runtimeTestNotifications(t, controller)
	next = runtimeTestApplyActivation(t, commands, cfg.FullName(), 4)
	next = runtimeTestApplyNotification(t, runtimeTestNotificationPlan(t, notifications, "internal/jobs/runtime-ready"), next)
	require.NotNil(t, next)
	require.Equal(t, uint64(4), next.Identity().Generation)
	require.EqualValues(t, 3, creates.Load(), "replacement must reuse its checked candidate after the separate configuration test")
	stopRuntimeTestResource(t, next)
	requireFactoryAttemptsIdle(t, controller.factory)
}

func TestRuntimeStartupTimeoutRetainsOwnershipUntilRunExits(t *testing.T) {
	controller, graph, _, _, _ := newDynCfgJobTestHarness(t)
	controller.factory.startupTimeout = 20 * time.Millisecond
	release, cleaned := make(chan struct{}), make(chan struct{})
	var releaseOnce sync.Once
	defer releaseOnce.Do(func() { close(release) })
	configureRuntimeTestCollector(controller, func() *runtimeTestCollector {
		return &runtimeTestCollector{
			run:     func(ctx context.Context, ready func()) error { <-release; ready(); return ctx.Err() },
			cleanup: func() { close(cleaned) },
		}
	})
	cfg := factoryTestConfig(false)
	commands := runtimeTestNotifications(t, controller)
	current := runtimeTestApply(t, prepareRuntimeTestChange(t, controller, cfg, nil, 1))
	require.NotNil(t, current)
	plan := runtimeTestNotificationPlan(t, commands, "internal/jobs/runtime-ready")
	require.Nil(t, runtimeTestApplyNotification(t, plan, current))
	record, _ := graph.Lookup(cfg.FullName())
	require.Equal(t, dyncfg.StatusFailed.String(), record.Status)
	select {
	case <-cleaned:
		t.Fatal("Cleanup overlapped a live Run")
	default:
	}
	releaseOnce.Do(func() { close(release) })
	requireFactoryAttemptsIdle(t, controller.factory)
	<-cleaned
	record, _ = graph.Lookup(cfg.FullName())
	require.Equal(t, dyncfg.StatusFailed.String(), record.Status, "late readiness cannot revive timed-out startup")
}

// Pause only the transaction return after the real runtime has settled readiness.
// Run completion must not invalidate installation while Apply is at this boundary.
type pauseAcceptedRuntime struct {
	lifecycle.PreparedResource
	afterReady func()
}

func (p pauseAcceptedRuntime) AcceptStart(ctx context.Context, generation uint64) (lifecycle.ReadyResource, error) {
	current, err := p.PreparedResource.AcceptStart(ctx, generation)
	if err == nil {
		if generation, ok := current.(*JobGeneration); ok {
			select {
			case <-generation.StartupDone():
				err = generation.StartupResult()
			case <-ctx.Done():
				err = ctx.Err()
			}
		}
		p.afterReady()
	}
	return current, err
}

func TestRuntimeReadyThenFailedBeforeInstallationReconcilesExactGeneration(t *testing.T) {
	for _, terminal := range []string{"error", "nil", "panic"} {
		t.Run(terminal, func(t *testing.T) {
			controller, graph, _, output, _ := newDynCfgJobTestHarness(t)
			commands := &autoDetectionRetryTestCommands{}
			controller.bindRuntimeFailures(commands, 9, func(err error) { t.Errorf("dispatch failed: %v", err) })
			failRun := make(chan struct{})
			configureRuntimeTestCollector(controller, func() *runtimeTestCollector {
				return &runtimeTestCollector{run: func(_ context.Context, funcReady func()) error {
					funcReady()
					<-failRun
					switch terminal {
					case "panic":
						panic("serving panic")
					case "nil":
						return nil
					}
					return errors.New("required reader failed")
				}}
			})
			cfg := factoryTestConfig(false).Set("autodetection_retry", 1)
			prepared := prepareRuntimeTestChange(t, controller, cfg, nil, 1).(*runtimeTestActivationTransaction)
			installation := prepared.PreparedResourceTransaction.(*PreparedResourceTransaction)
			installation.spec.Successor = pauseAcceptedRuntime{PreparedResource: installation.spec.Successor, afterReady: func() {
				close(failRun)
				runtimeTestNotificationPlan(t, commands, "internal/jobs/runtime-failure")
			}}
			current := runtimeTestApply(t, prepared)
			require.NotNil(t, current, "accepted readiness must still install")
			record, _ := graph.Lookup(cfg.FullName())
			require.Equal(t, dyncfg.StatusAccepted.String(), record.Status)
			generation := current.(*JobGeneration)
			_, err := generation.resources.outputGate.Write([]byte("LATE ORDINARY OUTPUT\n"))
			require.ErrorIs(t, err, errGenerationOutputFenced)
			require.NotContains(t, output.String(), "LATE ORDINARY OUTPUT")
			requests, plans, waited := commands.snapshot()
			require.False(t, waited, "failure notification must not await its own teardown transaction")
			require.Equal(t, cfg.FullName(), requests[0].LaneKey)
			require.NoError(t, requests[0].Validate())
			scope := lifecycle.ResourceTransactionScope{ID: cfg.FullName(), Current: current.Identity()}
			reconcile, err := plans[0].Transaction.Prepare(t.Context(), current, scope, lifecycle.LongLivedPermit{})
			require.NoError(t, err)
			require.Nil(t, runtimeTestApply(t, reconcile))
			record, _ = graph.Lookup(cfg.FullName())
			require.Equal(t, dyncfg.StatusFailed.String(), record.Status)
			require.False(t, runtimeTestHasRetry(controller, cfg.FullName()))
			requireFactoryAttemptsIdle(t, controller.factory)

			configureRuntimeTestCollector(controller, func() *runtimeTestCollector {
				return &runtimeTestCollector{run: func(ctx context.Context, ready func()) error { ready(); <-ctx.Done(); return nil }}
			})
			next := runtimeTestStartAndSettle(t, controller, cfg, nil, 2)
			scope.Current = next.Identity()
			stale, err := plans[0].Transaction.Prepare(t.Context(), next, scope, lifecycle.LongLivedPermit{})
			require.NoError(t, err)
			require.Same(t, next, runtimeTestApply(t, stale))
			record, _ = graph.Lookup(cfg.FullName())
			require.Equal(t, dyncfg.StatusRunning.String(), record.Status)
			stopRuntimeTestResource(t, next)
			requireFactoryAttemptsIdle(t, controller.factory)
		})
	}
}

func TestRuntimeStartupClassificationDoesNotAbsorbIntegrityFailures(t *testing.T) {
	startup := &runtimeStartupFailure{failure: &autoDetectionFailure{cause: errors.New("secret startup details")}}
	for _, test := range []struct {
		name string
		err  error
		want bool
	}{
		{"ordinary", startup, true},
		{"wrapped", fmt.Errorf("outer: %w", startup), true},
		{"joined cleanup", errors.Join(startup, errors.New("cleanup failure")), false},
		{"joined owner panic", errors.Join(startup, lifecycle.ErrTaskPanic), false},
		{"retained ownership", lifecycle.RetainOwnership(startup), true},
	} {
		t.Run(test.name, func(t *testing.T) {
			_, ok := onlyRuntimeStartupFailure(test.err)
			require.Equal(t, test.want, ok)
			redacted := redactResolvedLifecycleError(test.err)
			require.NotContains(t, redacted.Error(), "secret startup details")
			_, ok = onlyRuntimeStartupFailure(redacted)
			require.Equal(t, test.want, ok, "redaction must not turn mixed integrity errors into operational failure")
			require.Equal(t, lifecycle.OwnershipRetained(test.err), lifecycle.OwnershipRetained(redacted))
		})
	}
}

func TestRuntimeFailureRevokesOutputWhileCollectIsBlocked(t *testing.T) {
	controller, graph, _, output, _ := newDynCfgJobTestHarness(t)
	collecting, releaseCollect, failRun, cleaned := make(chan struct{}), make(chan struct{}), make(chan struct{}), make(chan struct{})
	var releaseOnce sync.Once
	defer releaseOnce.Do(func() { close(releaseCollect) })
	configureRuntimeTestCollector(controller, func() *runtimeTestCollector {
		c := &runtimeTestCollector{
			factoryTestV2: &factoryTestV2{},
			run:           func(ctx context.Context, ready func()) error { ready(); <-failRun; return errors.New("reader failed") },
			cleanup:       func() { close(cleaned) },
		}
		c.collect = func(context.Context) error {
			close(collecting)
			<-releaseCollect
			c.store.Write().SnapshotMeter("factory").Gauge("value").Observe(42)
			return nil
		}
		return c
	})
	cfg := factoryTestConfig(false)
	commands := runtimeTestNotifications(t, controller)
	current := runtimeTestApply(t, prepareRuntimeTestChange(t, controller, cfg, nil, 1))
	current = runtimeTestApplyNotification(t, runtimeTestNotificationPlan(t, commands, "internal/jobs/runtime-ready"), current)
	generation := current.(*JobGeneration)
	require.Eventually(t, func() bool {
		generation.resources.candidateJob.Tick(1)
		select {
		case <-collecting:
			return true
		default:
			return false
		}
	}, time.Second, time.Millisecond)
	close(failRun)
	failurePlan := runtimeTestNotificationPlan(t, commands, "internal/jobs/runtime-failure")
	_, err := generation.resources.outputGate.Write([]byte("LATE ORDINARY OUTPUT\n"))
	require.ErrorIs(t, err, errGenerationOutputFenced)
	reconcile, err := failurePlan.Transaction.Prepare(t.Context(), current,
		lifecycle.ResourceTransactionScope{ID: cfg.FullName(), Current: current.Identity()}, lifecycle.LongLivedPermit{})
	require.NoError(t, err)
	require.Nil(t, runtimeTestApply(t, reconcile))
	record, _ := graph.Lookup(cfg.FullName())
	require.Equal(t, dyncfg.StatusFailed.String(), record.Status)
	select {
	case <-cleaned:
		t.Fatal("physical cleanup overlapped Collect")
	default:
	}
	releaseOnce.Do(func() { close(releaseCollect) })
	requireFactoryAttemptsIdle(t, controller.factory)
	<-cleaned
	require.NotContains(t, output.String(), "LATE ORDINARY OUTPUT")
	require.NotContains(t, output.String(), "BEGIN '", "failed collection must not commit ordinary chart output")
}

func TestRuntimeStartupFailureUsesEveryActivationPath(t *testing.T) {
	for _, path := range []string{"enable", "restart", "update", "accepted", "secret"} {
		t.Run(path, func(t *testing.T) {
			controller, graph, _, _, _ := newDynCfgJobTestHarness(t)
			cfg := factoryTestConfig(false).Set("autodetection_retry", 7)
			var current lifecycle.ReadyResource
			if path == "update" || path == "restart" || path == "secret" {
				configureRuntimeTestCollector(controller, func() *runtimeTestCollector {
					return &runtimeTestCollector{run: func(ctx context.Context, ready func()) error { ready(); <-ctx.Done(); return ctx.Err() }}
				})
				current = runtimeTestStartAndSettle(t, controller, cfg, nil, 1)
			} else {
				status := dyncfg.StatusFailed
				if path == "accepted" {
					status = dyncfg.StatusAccepted
				}
				if path == "secret" {
					status = dyncfg.StatusRunning
				}
				seedDynCfgJobGraphRecord(t, graph, cfg, status)
			}
			configureRuntimeTestCollector(controller, func() *runtimeTestCollector {
				return &runtimeTestCollector{run: func(context.Context, func()) error { return errors.New("listen 127.0.0.1:8125: bind failed") }}
			})
			activations := runtimeTestBindActivations(t, controller)
			commands := runtimeTestNotifications(t, controller)
			scope := lifecycle.ResourceTransactionScope{ID: cfg.FullName(), Successor: lifecycle.ResourceIdentity{ID: cfg.FullName(), Generation: 3}}
			if current != nil {
				scope.Current = current.Identity()
			}
			permit, _ := issueTestJobPermit(t, cfg.FullName(), 3)
			var prepared lifecycle.PreparedResourceTransaction
			var err error
			if path == "secret" {
				require.NoError(t, permit.AbortUnused())
				stopPlan, _, planErr := controller.PlanSecretDependentStop(cfg.FullName())
				require.NoError(t, planErr)
				current = runtimeTestApplyNotification(t, stopPlan, current)
				require.Nil(t, current)
				plan, _, planErr := controller.PlanSecretDependentStart(cfg.FullName())
				require.NoError(t, planErr)
				prepared, err = plan.Transaction.Prepare(t.Context(), nil,
					lifecycle.ResourceTransactionScope{ID: cfg.FullName()}, lifecycle.LongLivedPermit{})
			} else {
				command := path
				if command == "accepted" {
					command = "enable"
				}
				request := DynCfgJobRequest{Args: []string{"go.d:collector:module:job", command}}
				if path == "update" {
					request.Payload = []byte(`{"option_str":"updated","autodetection_retry":7}`)
					request.HasPayload = true
					request.ContentType = "application/json"
				}
				prepared, err = controller.Prepare(t.Context(), request, current, scope, permit)
			}
			require.NoError(t, err)
			applied, err := prepared.Apply(t.Context())
			require.NoError(t, err)
			_, _, current = applied.Ownership()
			if path == "enable" || path == "update" {
				require.Equal(t, 202, applied.ResultStatus())
			}
			if current == nil {
				if released, present := controller.factory.config.Attempts.ProcessAttemptReleased(jobAttemptIdentity(jobmgr.ProcessAttemptJobRuntime, cfg.FullName())); present {
					requireTestSignal(t, released, "predecessor did not release its physical identity")
				}
				current = runtimeTestApplyActivation(t, activations, cfg.FullName(), 4)
			}
			require.NotNil(t, current, "runtime initiation installs a starting generation")
			plan := runtimeTestNotificationPlan(t, commands, "internal/jobs/runtime-ready")
			require.Nil(t, runtimeTestApplyNotification(t, plan, current))
			record, _ := graph.Lookup(cfg.FullName())
			require.Equal(t, dyncfg.StatusFailed.String(), record.Status)
			requireFactoryAttemptsIdle(t, controller.factory)
		})
	}
}

func TestRuntimeStartupFailureDoesNotHideCleanupPanic(t *testing.T) {
	controller, _, _, _, _ := newDynCfgJobTestHarness(t)
	configureRuntimeTestCollector(controller, func() *runtimeTestCollector {
		return &runtimeTestCollector{
			run:     func(context.Context, func()) error { return errors.New("startup failed") },
			cleanup: func() { panic("cleanup failed") },
		}
	})
	cfg := factoryTestConfig(false)
	require.Nil(t, runtimeTestStartAndSettle(t, controller, cfg, nil, 1))
	attempts := controller.factory.config.Attempts.(*containment.Authority)
	// No output/generation resource remains, but the process identity stays quarantined.
	// The candidate attempt releases asynchronously after handing off to the runtime attempt.
	require.Eventually(t, func() bool {
		return attempts.Census() == (containment.Census{Quarantined: 1})
	}, time.Second, time.Millisecond)
}

// Pause after the real admission commits, before the candidate worker can use it.
type runtimeCandidateAdmissionBarrier struct {
	jobmgr.ProcessAttemptAuthority
	entered chan struct{}
	release chan struct{}
}

func (a runtimeCandidateAdmissionBarrier) StartProcessAttempt(ctx context.Context, plan jobmgr.ProcessAttemptPlan) (jobmgr.ProcessAttempt, error) {
	original := plan.Work
	plan.Work = func(ctx context.Context, admission jobmgr.ProcessAttemptAdmission) error {
		return original(ctx, runtimeAdmissionFunc(func() error {
			if err := admission.Admit(); err != nil {
				return err
			}
			close(a.entered)
			<-a.release
			return nil
		}))
	}
	return a.ProcessAttemptAuthority.StartProcessAttempt(ctx, plan)
}

type runtimeAdmissionFunc func() error

func (f runtimeAdmissionFunc) Admit() error { return f() }

func TestRuntimeCandidateReleaseDuringAdmissionCleansWithoutQuarantine(t *testing.T) {
	controller, _, _, _, _ := newDynCfgJobTestHarness(t)
	var cleaned atomic.Int32
	configureRuntimeTestCollector(controller, func() *runtimeTestCollector {
		return &runtimeTestCollector{
			run:     func(context.Context, func()) error { panic("released candidate must never Run") },
			cleanup: func() { cleaned.Add(1) },
		}
	})
	authority := controller.factory.config.Attempts.(*containment.Authority)
	entered, release := make(chan struct{}), make(chan struct{})
	var releaseOnce sync.Once
	defer releaseOnce.Do(func() { close(release) })
	controller.factory.config.Attempts = runtimeCandidateAdmissionBarrier{ProcessAttemptAuthority: authority, entered: entered, release: release}
	candidate, err := controller.factory.newCandidate(factoryTestConfig(false))
	require.NoError(t, err)
	candidate.Start()
	select {
	case <-entered:
	case <-time.After(time.Second):
		t.Fatal("candidate was not admitted")
	}
	candidate.Release()
	releaseOnce.Do(func() { close(release) })
	require.Eventually(t, func() bool { return authority.Census().Active == 0 }, time.Second, time.Millisecond)
	require.Equal(t, containment.Census{}, authority.Census(), "canceled candidate must clean up, not panic or quarantine")
	require.EqualValues(t, 1, cleaned.Load())
}

func TestRuntimeStartupFailureRetainsStockConfig(t *testing.T) {
	controller, graph, _, _, _ := newDynCfgJobTestHarness(t)
	configureRuntimeTestCollector(controller, func() *runtimeTestCollector {
		return &runtimeTestCollector{run: func(context.Context, func()) error { return errors.New("listen 127.0.0.1:8125: bind failed") }}
	})
	cfg := factoryTestConfig(false).SetSourceType(confgroup.TypeStock)
	require.Nil(t, runtimeTestStartAndSettle(t, controller, cfg, nil, 1))
	record, exists := graph.Lookup(cfg.FullName())
	require.True(t, exists, "runtime acquisition failure must remain visible after successful detection")
	require.Equal(t, dyncfg.StatusFailed.String(), record.Status)
	requireFactoryAttemptsIdle(t, controller.factory)
}
