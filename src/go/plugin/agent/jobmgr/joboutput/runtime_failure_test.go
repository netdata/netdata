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

func prepareRuntimeTestChange(t *testing.T, controller *DynCfgJobController, cfg confgroup.Config, current lifecycle.ReadyResource, generation uint64) lifecycle.PreparedResourceTransaction {
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
			applied, err := prepareRuntimeTestChange(t, controller, cfg, nil, 1).Apply(t.Context())
			require.NoError(t, err, "collector failure must not become a transaction/manager failure")
			_, _, current := applied.Ownership()
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
			current = runtimeTestApply(t, prepareRuntimeTestChange(t, controller, cfg, nil, 2))
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
	require.Nil(t, runtimeTestApply(t, prepareRuntimeTestChange(t, controller, cfg, nil, 1)))
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
				if number == 3 {
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
	current := runtimeTestApply(t, prepareRuntimeTestChange(t, controller, cfg, nil, 1))
	require.NotNil(t, current)
	require.NoError(t, controller.configModules.Test(t.Context(), cfg), "configuration test must not bind the active endpoint")
	prepared := prepareRuntimeTestChange(t, controller, cfg, current, 2)
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
	cleanupRelease.Do(func() { close(releaseCleanup) })
	resultValue := <-done
	require.NoError(t, resultValue.err)
	_, _, next := resultValue.applied.Ownership()
	require.NotNil(t, next)
	require.Equal(t, uint64(2), next.Identity().Generation)
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
	done := make(chan error, 1)
	prepared := prepareRuntimeTestChange(t, controller, cfg, nil, 1)
	go func() { _, err := prepared.Apply(t.Context()); done <- err }()
	select {
	case err := <-done:
		require.NoError(t, err)
	case <-time.After(time.Second):
		t.Fatal("logical startup wait did not time out")
	}
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
			prepared := prepareRuntimeTestChange(t, controller, cfg, nil, 1).(*PreparedResourceTransaction)
			prepared.spec.Successor = pauseAcceptedRuntime{PreparedResource: prepared.spec.Successor, afterReady: func() {
				close(failRun)
				commands.waitForSubmissions(t, 1)
			}}
			current := runtimeTestApply(t, prepared)
			require.NotNil(t, current, "accepted readiness must still install")
			record, _ := graph.Lookup(cfg.FullName())
			require.Equal(t, dyncfg.StatusRunning.String(), record.Status)
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
			next := runtimeTestApply(t, prepareRuntimeTestChange(t, controller, cfg, nil, 2))
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
	commands := &autoDetectionRetryTestCommands{}
	controller.bindRuntimeFailures(commands, 9, func(err error) { t.Errorf("dispatch failed: %v", err) })
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
	current := runtimeTestApply(t, prepareRuntimeTestChange(t, controller, cfg, nil, 1))
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
	commands.waitForSubmissions(t, 1)
	_, err := generation.resources.outputGate.Write([]byte("LATE ORDINARY OUTPUT\n"))
	require.ErrorIs(t, err, errGenerationOutputFenced)
	_, plans, _ := commands.snapshot()
	reconcile, err := plans[0].Transaction.Prepare(t.Context(), current,
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
			if path == "update" || path == "restart" {
				configureRuntimeTestCollector(controller, func() *runtimeTestCollector {
					return &runtimeTestCollector{run: func(ctx context.Context, ready func()) error { ready(); <-ctx.Done(); return ctx.Err() }}
				})
				current = runtimeTestApply(t, prepareRuntimeTestChange(t, controller, cfg, nil, 1))
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
			scope := lifecycle.ResourceTransactionScope{ID: cfg.FullName(), Successor: lifecycle.ResourceIdentity{ID: cfg.FullName(), Generation: 2}}
			if current != nil {
				scope.Current = current.Identity()
			}
			permit, _ := issueTestJobPermit(t, cfg.FullName(), 2)
			var prepared lifecycle.PreparedResourceTransaction
			var err error
			var secretState *SecretDependentStart
			switch path {
			case "secret":
				var plan jobmgr.WorkPlan
				plan, secretState, err = controller.PlanSecretDependentStart(cfg.FullName())
				require.NoError(t, err)
				prepared, err = plan.Transaction.Prepare(t.Context(), nil, scope, permit)
			case "accepted":
				releaseSubmission := make(chan struct{})
				commands := &autoDetectionRetryTestCommands{block: releaseSubmission}
				require.NoError(t, controller.BindBackgroundWorkers(commands, 9, func(err error) { t.Errorf("worker failed: %v", err) }))
				t.Cleanup(func() {
					close(releaseSubmission)
					controller.scheduler.StopBackgroundWorkers()
					require.NoError(t, controller.scheduler.WaitBackgroundWorkers(context.Background()))
				})
				applyAcceptedEnableForTest(t, controller, graph, cfg, 1)
				commands.waitForSubmissions(t, 1)
				_, plans, _ := commands.snapshot()
				prepared, err = plans[0].Transaction.Prepare(t.Context(), nil, scope, permit)
			default:
				request := DynCfgJobRequest{Args: []string{"go.d:collector:module:job", path}}
				if path == "update" {
					request.Payload = []byte(`{"option_str":"updated","autodetection_retry":7}`)
					request.HasPayload = true
					request.ContentType = "application/json"
				}
				prepared, err = controller.Prepare(t.Context(), request, current, scope, permit)
			}
			require.NoError(t, err)
			applied, err := prepared.Apply(t.Context())
			require.NoError(t, err, "startup failure must remain an operational result on every path")
			_, _, current = applied.Ownership()
			require.Nil(t, current)
			if path == "enable" || path == "restart" || path == "update" {
				require.Equal(t, 503, applied.ResultStatus())
			}
			if secretState != nil {
				require.ErrorContains(t, secretState.Err(), "127.0.0.1:8125")
			}
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
	require.Nil(t, runtimeTestApply(t, prepareRuntimeTestChange(t, controller, cfg, nil, 1)))
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
	require.Nil(t, runtimeTestApply(t, prepareRuntimeTestChange(t, controller, cfg, nil, 1)))
	record, exists := graph.Lookup(cfg.FullName())
	require.True(t, exists, "runtime acquisition failure must remain visible after successful detection")
	require.Equal(t, dyncfg.StatusFailed.String(), record.Status)
	requireFactoryAttemptsIdle(t, controller.factory)
}
