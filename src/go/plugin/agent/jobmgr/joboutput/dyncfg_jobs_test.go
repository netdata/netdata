// SPDX-License-Identifier: GPL-3.0-or-later

package joboutput

import (
	"bytes"
	"context"
	"errors"
	"strings"
	"testing"
	"time"

	"github.com/netdata/netdata/go/plugins/plugin/agent/jobmgr/lifecycle"
	secretresolver "github.com/netdata/netdata/go/plugins/plugin/agent/secrets/resolver"
	"github.com/netdata/netdata/go/plugins/plugin/framework/collectorapi"
	"github.com/netdata/netdata/go/plugins/plugin/framework/confgroup"
	"github.com/netdata/netdata/go/plugins/plugin/framework/dyncfg"
	"github.com/netdata/netdata/go/plugins/plugin/framework/vnoderegistry"
	"github.com/stretchr/testify/require"
	"gopkg.in/yaml.v2"
)

func TestDynCfgAddCommitsOrDisposesOneGraphTransaction(t *testing.T) {
	tests := map[string]struct {
		apply     bool
		wantGraph bool
	}{
		"apply response before notification": {apply: true, wantGraph: true},
		"dispose preserves empty graph":      {},
	}
	for name, test := range tests {
		t.Run(name, func(t *testing.T) {
			controller, graph, supervisor, output, state := newDynCfgJobTestHarness(t)
			scope := lifecycle.ResourceTransactionScope{
				ID: "module_job",
			}
			request := DynCfgJobRequest{
				Args:         []string{"go.d:collector:module", "add", "job"},
				Payload:      []byte(`{"option":"value"}`),
				ContentType:  "application/json",
				CallerSource: "user=test",
				HasPayload:   true,
			}
			plan, err := lifecycle.NewResourceTransactionTaskPlan(
				lifecycle.SourceFunction,
				time.Time{},
				lifecycle.TransactionTaskPhases,
				nil,
				scope,
				func(
					ctx context.Context,
					current lifecycle.ReadyResource,
					taskScope lifecycle.ResourceTransactionScope,
					permit lifecycle.LongLivedPermit,
				) (lifecycle.PreparedResourceTransaction, error) {
					return controller.Prepare(ctx, request, current, taskScope, permit)
				},
			)
			require.NoError(t, err)
			ref := startDynCfgJobTestTask(t, supervisor, plan)
			first := <-supervisor.CompletionCh()
			require.False(t, first.Ref != ref ||
				first.Sequence != 1 ||
				first.Kind != lifecycle.TaskOutcomePreparedResourceTransaction ||
				first.Err != nil)

			if test.apply {
				require.NoError(t, supervisor.SendAction(
					lifecycle.TaskAction{
						Ref:      ref,
						Sequence: 2,
						Kind:     lifecycle.TaskActionApplyResourceTransaction,
					},
				),
				)

				second := <-supervisor.CompletionCh()
				require.False(t, second.Ref != ref ||
					second.Sequence != 2 ||
					second.Kind != lifecycle.TaskOutcomeAppliedResourceTransaction ||
					second.Err != nil)
				disposition, current, err := supervisor.TakeAppliedResourceTransaction(ref, 2, scope)
				require.NoError(t, err)
				require.False(t, disposition != lifecycle.ResourceTransactionUnchanged || current != nil)

				preflightResultErr := supervisor.PreflightResult(ref, "add", 1)
				require.NoError(t, preflightResultErr)

				sendDynCfgJobTestAction(
					t,
					supervisor,
					lifecycle.TaskAction{
						Ref:      ref,
						Sequence: 3,
						Kind:     lifecycle.TaskActionEncodeWrite,
						UID:      "add",
						Expiry:   1,
					},
				)
				sendDynCfgJobTestAction(
					t,
					supervisor,
					lifecycle.TaskAction{
						Ref:      ref,
						Sequence: 4,
						Kind:     lifecycle.TaskActionCleanup,
					},
				)
				sendDynCfgJobTestAction(
					t,
					supervisor,
					lifecycle.TaskAction{
						Ref:      ref,
						Sequence: 5,
						Kind:     lifecycle.TaskActionTerminate,
					},
				)
			} else {
				sendDynCfgJobTestAction(
					t,
					supervisor,
					lifecycle.TaskAction{
						Ref:      ref,
						Sequence: 2,
						Kind:     lifecycle.TaskActionDispose,
					},
				)
				current, err := supervisor.TakeDisposedResourceTransaction(ref, 2, scope)
				require.NoError(t, err)
				require.Nil(t, current)
				sendDynCfgJobTestAction(
					t,
					supervisor,
					lifecycle.TaskAction{
						Ref:      ref,
						Sequence: 3,
						Kind:     lifecycle.TaskActionTerminate,
					},
				)
			}

			require.NoError(t, supervisor.Release(ref))

			record, exists := graph.Lookup("module_job")
			require.EqualValues(t, test.wantGraph, exists)
			require.False(t, exists && record.Status != dyncfg.StatusAccepted.String())
			require.EqualValues(t, 1, state.collectorCleanup)
			wire := output.String()
			if !test.apply {
				require.EqualValues(t, "", wire)
				return
			}
			resultAt := strings.Index(wire, "FUNCTION_RESULT_BEGIN add 202 application/json 1\n")
			notificationAt := strings.Index(wire, "CONFIG go.d:collector:module:job create accepted job")
			require.False(t, resultAt < 0 || notificationAt < 0 || resultAt >= notificationAt)
		})
	}
}

func TestDynCfgCommandsPropagateRetainedConstructionFailure(t *testing.T) {
	type prepareCommand func(
		context.Context,
		*DynCfgJobController,
		dynCfgTarget,
		dyncfg.GraphRecord,
		lifecycle.ResourceTransactionScope,
		lifecycle.LongLivedPermit,
	) (lifecycle.PreparedResourceTransaction, error)
	tests := map[string]struct {
		status        dyncfg.Status
		validateFirst bool
		prepare       prepareCommand
	}{
		"enable": {
			status: dyncfg.StatusDisabled,
			prepare: func(
				ctx context.Context,
				controller *DynCfgJobController,
				target dynCfgTarget,
				record dyncfg.GraphRecord,
				scope lifecycle.ResourceTransactionScope,
				permit lifecycle.LongLivedPermit,
			) (lifecycle.PreparedResourceTransaction, error) {
				return controller.prepareEnable(ctx, target, record, true, nil, scope, permit)
			},
		},
		"restart": {
			status: dyncfg.StatusFailed,
			prepare: func(
				ctx context.Context,
				controller *DynCfgJobController,
				target dynCfgTarget,
				record dyncfg.GraphRecord,
				scope lifecycle.ResourceTransactionScope,
				permit lifecycle.LongLivedPermit,
			) (lifecycle.PreparedResourceTransaction, error) {
				return controller.prepareRestart(ctx, target, record, true, nil, scope, permit)
			},
		},
		"update": {
			status:        dyncfg.StatusRunning,
			validateFirst: true,
			prepare: func(
				ctx context.Context,
				controller *DynCfgJobController,
				target dynCfgTarget,
				record dyncfg.GraphRecord,
				scope lifecycle.ResourceTransactionScope,
				permit lifecycle.LongLivedPermit,
			) (lifecycle.PreparedResourceTransaction, error) {
				return controller.prepareUpdate(
					ctx,
					DynCfgJobRequest{
						Payload:      []byte(`{"option":"replacement"}`),
						ContentType:  "application/json",
						CallerSource: "user=test",
						HasPayload:   true,
					},
					target,
					record,
					true,
					nil,
					scope,
					permit,
				)
			},
		},
	}
	for name, test := range tests {
		t.Run(name, func(t *testing.T) {
			controller, graph, _, _, state := newDynCfgJobTestHarness(t)
			creator := controller.modules["module"]
			var constructions int
			creator.Create = func() collectorapi.CollectorV1 {
				constructions++
				if test.validateFirst && constructions == 1 {
					return state.module(nil, false)
				}
				panic("construction failed")
			}
			controller.modules["module"] = creator
			config := factoryTestConfig(false)
			payload, err := yaml.Marshal(config)
			require.NoError(t, err)
			mutation, err := graph.PrepareMutation(
				[]dyncfg.GraphChange{{
					ID: config.FullName(),
					Config: &dyncfg.GraphConfig{
						ID:      config.FullName(),
						Module:  config.Module(),
						Name:    config.Name(),
						Status:  test.status.String(),
						Payload: payload,
					},
				}},
			)
			require.NoError(t, err)
			require.NoError(t, graph.Commit(mutation))
			record, exists := graph.Lookup(config.FullName())
			require.True(t, exists)
			permit, permitTasks := issueTestJobPermit(t, config.FullName(), 1)
			scope := lifecycle.ResourceTransactionScope{
				ID: config.FullName(),
				Successor: lifecycle.ResourceIdentity{
					ID:         config.FullName(),
					Generation: 1,
				},
			}
			target := dynCfgTarget{
				module:     config.Module(),
				name:       config.Name(),
				resourceID: config.FullName(),
				creator:    creator,
			}

			transaction, err := test.prepare(context.Background(), controller, target, record, scope, permit)
			require.Nil(t, transaction)
			require.True(t, lifecycle.OwnershipRetained(err))
			census := permitTasks.LongLivedCensus()
			require.EqualValues(t, 1, census.Active)
		})
	}
}

func TestResourceOnlyTransactionReplacesWithoutGraphMutation(t *testing.T) {
	var events []string
	currentIdentity := lifecycle.ResourceIdentity{
		ID:         "job",
		Generation: 1,
	}
	successorIdentity := lifecycle.ResourceIdentity{
		ID:         "job",
		Generation: 2,
	}
	current := &transactionTestReadyResource{
		identity: currentIdentity,
		prefix:   "current",
		events:   &events,
	}
	successorReady := &transactionTestReadyResource{
		identity: successorIdentity,
		prefix:   "successor",
		events:   &events,
	}
	successor := &transactionTestPreparedResource{
		identity: successorIdentity,
		ready:    successorReady,
		events:   &events,
	}
	result, err := lifecycle.NewSealedResult(200, "application/json", []byte(`{"status":200,"message":""}`))
	require.NoError(t, err)
	transaction, err := PrepareResourceTransaction(
		ResourceTransactionSpec{
			Scope: lifecycle.ResourceTransactionScope{
				ID:        "job",
				Current:   currentIdentity,
				Successor: successorIdentity,
			},
			Disposition: lifecycle.ResourceTransactionReplaced,
			Current:     current,
			Successor:   successor,
			Result:      result,
			Cleanup:     func() error { return nil },
		},
	)
	require.NoError(t, err)

	_, applyErr := transaction.Apply(context.Background())
	require.NoError(t, applyErr)

	want := []string{"current-stop", "current-finalize", "successor-accept", "successor-publish"}
	require.EqualValues(t, strings.Join(want, ","), strings.Join(events, ","))
}

func TestFailedAutoDetectionCommitsFailedStateAndSchedulesRetry(t *testing.T) {
	controller, graph, _, _, state := newDynCfgJobTestHarness(t)
	var events []string
	creator := controller.modules["module"]
	creator.Create = func() collectorapi.CollectorV1 {
		return state.module(func(context.Context) error {
			events = append(events, "autodetection")
			return errors.New("check failed")
		}, false)
	}
	controller.modules["module"] = creator
	commands := &autoDetectionRetryTestCommands{}
	require.NoError(t, controller.BindAutoDetectionRetries(
		commands,
		1,
		func(error) {},
	))
	config := factoryTestConfig(false)
	config.Set("autodetection_retry", 1)
	config.SetSourceType(confgroup.TypeDyncfg)
	config.SetSource("user=test")
	config.SetProvider("test")
	payload, err := yaml.Marshal(config)
	require.NoError(t, err)
	mutation, err := graph.PrepareMutation([]dyncfg.GraphChange{{
		ID: config.FullName(),
		Config: &dyncfg.GraphConfig{
			ID:      config.FullName(),
			Module:  config.Module(),
			Name:    config.Name(),
			Status:  dyncfg.StatusRunning.String(),
			Payload: payload,
		},
	}})
	require.NoError(t, err)
	require.NoError(t, graph.Commit(mutation))
	currentIdentity := lifecycle.ResourceIdentity{
		ID:         config.FullName(),
		Generation: 1,
	}
	current := &transactionTestReadyResource{
		identity: currentIdentity,
		prefix:   "current",
		events:   &events,
	}
	permit, tasks := issueTestJobPermit(t, config.FullName(), 2)
	scope := lifecycle.ResourceTransactionScope{
		ID:      config.FullName(),
		Current: currentIdentity,
		Successor: lifecycle.ResourceIdentity{
			ID:         config.FullName(),
			Generation: 2,
		},
	}

	transaction, err := controller.prepareDiscovered(
		context.Background(),
		DiscoveredJobChange{
			Config:  config,
			Status:  dyncfg.StatusRunning,
			Restart: true,
		},
		current,
		scope,
		permit,
	)
	require.NoError(t, err)
	_, err = transaction.Apply(context.Background())
	require.NoError(t, err)
	require.Equal(t, []string{"current-stop", "current-finalize", "autodetection"}, events)

	record, ok := graph.Lookup(config.FullName())
	require.True(t, ok)
	require.Equal(t, dyncfg.StatusFailed.String(), record.Status)
	require.EqualValues(t, 1, state.collectorCleanup)
	require.EqualValues(t, lifecycle.LongLivedCensus{}, tasks.LongLivedCensus())

	require.NoError(t, controller.scheduler.Tick(context.Background(), 0))
	require.NoError(t, controller.scheduler.Tick(context.Background(), 1))
	commands.waitForSubmissions(t, 1)
	submitted, plans, waited := commands.snapshot()
	require.Len(t, submitted, 1)
	require.Len(t, plans, 1)
	require.False(t, waited)

	replacement, err := config.Clone()
	require.NoError(t, err)
	replacement.Set("option", "replacement")
	replacementPayload, err := yaml.Marshal(replacement)
	require.NoError(t, err)
	replacementMutation, err := graph.PrepareMutation(
		[]dyncfg.GraphChange{{
			ID: replacement.FullName(),
			Config: &dyncfg.GraphConfig{
				ID:      replacement.FullName(),
				Module:  replacement.Module(),
				Name:    replacement.Name(),
				Status:  dyncfg.StatusFailed.String(),
				Payload: replacementPayload,
			},
		}},
	)
	require.NoError(t, err)
	require.NoError(t, graph.Commit(replacementMutation))
	retryPermit, retryTasks := issueTestJobPermit(t, config.FullName(), 3)
	retryScope := lifecycle.ResourceTransactionScope{
		ID: config.FullName(),
		Successor: lifecycle.ResourceIdentity{
			ID:         config.FullName(),
			Generation: 3,
		},
	}
	retryTransaction, err := plans[0].Transaction.Prepare(context.Background(), nil, retryScope, retryPermit)
	require.NoError(t, err)
	_, err = retryTransaction.Apply(context.Background())
	require.NoError(t, err)
	require.Equal(t, []string{"current-stop", "current-finalize", "autodetection"}, events)
	require.EqualValues(t, lifecycle.LongLivedCensus{}, retryTasks.LongLivedCensus())

	controller.scheduler.StopAutoDetectionRetries()
	require.NoError(t, controller.scheduler.WaitAutoDetectionRetries(context.Background()))
}

func TestNonRetryableAutoDetectionFailureSettlesExistingRetry(t *testing.T) {
	controller, graph, _, _, state := newDynCfgJobTestHarness(t)
	creator := controller.modules["module"]
	creator.Create = func() collectorapi.CollectorV1 {
		return state.module(func(context.Context) error {
			return nonRetryableAutoDetectionError{}
		}, false)
	}
	controller.modules["module"] = creator
	config := factoryTestConfig(false)
	config.Set("autodetection_retry", 1)
	config.SetSourceType(confgroup.TypeDyncfg)
	config.SetSource("user=test")
	config.SetProvider("test")
	controller.scheduler.retries.schedule(config, 1)
	controller.scheduler.retries.mu.Lock()
	token := controller.scheduler.retries.entries[config.FullName()].token
	controller.scheduler.retries.mu.Unlock()
	payload, err := yaml.Marshal(config)
	require.NoError(t, err)
	mutation, err := graph.PrepareMutation([]dyncfg.GraphChange{{
		ID: config.FullName(),
		Config: &dyncfg.GraphConfig{
			ID:      config.FullName(),
			Module:  config.Module(),
			Name:    config.Name(),
			Status:  dyncfg.StatusRunning.String(),
			Payload: payload,
		},
	}})
	require.NoError(t, err)
	require.NoError(t, graph.Commit(mutation))
	currentIdentity := lifecycle.ResourceIdentity{
		ID:         config.FullName(),
		Generation: 1,
	}
	var events []string
	current := &transactionTestReadyResource{
		identity: currentIdentity,
		prefix:   "current",
		events:   &events,
	}
	permit, tasks := issueTestJobPermit(t, config.FullName(), 2)
	scope := lifecycle.ResourceTransactionScope{
		ID:      config.FullName(),
		Current: currentIdentity,
		Successor: lifecycle.ResourceIdentity{
			ID:         config.FullName(),
			Generation: 2,
		},
	}

	transaction, err := controller.prepareDiscovered(
		context.Background(),
		DiscoveredJobChange{
			Config:  config,
			Status:  dyncfg.StatusRunning,
			Restart: true,
		},
		current,
		scope,
		permit,
	)
	require.NoError(t, err)
	_, err = transaction.Apply(context.Background())
	require.NoError(t, err)
	require.False(t, controller.scheduler.retries.isCurrent(config.FullName(), token))
	record, exists := graph.Lookup(config.FullName())
	require.True(t, exists)
	require.Equal(t, dyncfg.StatusFailed.String(), record.Status)
	require.EqualValues(t, lifecycle.LongLivedCensus{}, tasks.LongLivedCensus())
}

type nonRetryableAutoDetectionError struct{}

func (nonRetryableAutoDetectionError) Error() string {
	return "non-retryable autodetection failure"
}

func (nonRetryableAutoDetectionError) DyncfgCode() int {
	return 422
}

func TestManualNoChangeRemovalSettlesAutoDetectionRetry(t *testing.T) {
	controller, _, _, _, _ := newDynCfgJobTestHarness(t)
	config := factoryTestConfig(false)
	config.SetSourceType(confgroup.TypeStock)
	config.SetSource("stock")
	config.SetProvider("stock")
	controller.scheduler.retries.schedule(config, 1)
	controller.scheduler.retries.mu.Lock()
	token := controller.scheduler.retries.entries[config.FullName()].token
	controller.scheduler.retries.mu.Unlock()

	transaction, err := controller.prepareDiscovered(
		context.Background(),
		DiscoveredJobChange{
			Config: config,
			Remove: true,
		},
		nil,
		lifecycle.ResourceTransactionScope{
			ID: config.FullName(),
		},
		lifecycle.LongLivedPermit{},
	)
	require.NoError(t, err)
	_, err = transaction.Apply(context.Background())
	require.NoError(t, err)
	require.False(t, controller.scheduler.retries.isCurrent(config.FullName(), token))
}

func TestPlainStockRetryCanRestartAfterFailedGraphRecordWasRemoved(t *testing.T) {
	controller, graph, supervisor, _, state := newDynCfgJobTestHarness(t)
	creator := controller.modules["module"]
	creator.Create = func() collectorapi.CollectorV1 {
		module := state.module(nil, false)
		charts := collectorapi.Charts{}
		module.ChartsFunc = func() *collectorapi.Charts { return &charts }
		return module
	}
	controller.modules["module"] = creator
	config := factoryTestConfig(false)
	config.SetSourceType(confgroup.TypeStock)
	config.SetSource("stock")
	config.SetProvider("stock")
	controller.scheduler.retries.schedule(config, 1)
	controller.scheduler.retries.mu.Lock()
	token := controller.scheduler.retries.entries[config.FullName()].token
	controller.scheduler.retries.mu.Unlock()
	permitPlan := lifecycle.NewJobLongLivedPlan()
	scope := lifecycle.ResourceTransactionScope{
		ID: config.FullName(),
		Successor: lifecycle.ResourceIdentity{
			ID:         config.FullName(),
			Generation: 1,
		},
	}
	plan, err := lifecycle.NewResourceTransactionPermitTaskPlan(
		lifecycle.SourceJobManager,
		time.Time{},
		lifecycle.TransactionTaskPhases,
		nil,
		scope,
		permitPlan,
		func(
			ctx context.Context,
			current lifecycle.ReadyResource,
			taskScope lifecycle.ResourceTransactionScope,
			permit lifecycle.LongLivedPermit,
		) (lifecycle.PreparedResourceTransaction, error) {
			return controller.prepareDiscovered(
				ctx,
				DiscoveredJobChange{
					Config:  config,
					Status:  dyncfg.StatusRunning,
					Restart: true,
					retry:   token,
				},
				current,
				taskScope,
				permit,
			)
		},
	)
	require.NoError(t, err)
	ref := startDynCfgJobTestTask(t, supervisor, plan)
	first := <-supervisor.CompletionCh()
	require.NoError(t, first.Err)
	require.Equal(t, lifecycle.TaskOutcomePreparedResourceTransaction, first.Kind)
	require.NoError(t, supervisor.SendAction(lifecycle.TaskAction{
		Ref:      ref,
		Sequence: 2,
		Kind:     lifecycle.TaskActionApplyResourceTransaction,
	}))
	second := <-supervisor.CompletionCh()
	require.NoError(t, second.Err)
	disposition, current, err := supervisor.TakeAppliedResourceTransaction(ref, 2, scope)
	require.NoError(t, err)
	require.Equal(t, lifecycle.ResourceTransactionInstalled, disposition)
	require.NotNil(t, current)
	record, exists := graph.Lookup(config.FullName())
	require.True(t, exists)
	require.Equal(t, dyncfg.StatusRunning.String(), record.Status)
	require.False(t, controller.scheduler.retries.isCurrent(config.FullName(), token))

	sendDynCfgJobTestAction(t, supervisor, lifecycle.TaskAction{
		Ref:      ref,
		Sequence: 3,
		Kind:     lifecycle.TaskActionDispose,
	})
	sendDynCfgJobTestAction(t, supervisor, lifecycle.TaskAction{
		Ref:      ref,
		Sequence: 4,
		Kind:     lifecycle.TaskActionCleanup,
	})
	sendDynCfgJobTestAction(t, supervisor, lifecycle.TaskAction{
		Ref:      ref,
		Sequence: 5,
		Kind:     lifecycle.TaskActionTerminate,
	})
	require.NoError(t, supervisor.Release(ref))
	require.NoError(t, current.Stop(context.Background()))
	require.NoError(t, current.Finalize())
	require.EqualValues(t, lifecycle.LongLivedCensus{}, supervisor.LongLivedCensus())
}

func TestRetryPreparationFailureSettlesExactToken(t *testing.T) {
	controller, _, _, _, _ := newDynCfgJobTestHarness(t)
	creator := controller.modules["module"]
	creator.Create = func() collectorapi.CollectorV1 { return nil }
	controller.modules["module"] = creator
	config := factoryTestConfig(false)
	config.SetSourceType(confgroup.TypeStock)
	config.SetSource("stock")
	config.SetProvider("stock")
	controller.scheduler.retries.schedule(config, 1)
	controller.scheduler.retries.mu.Lock()
	token := controller.scheduler.retries.entries[config.FullName()].token
	controller.scheduler.retries.mu.Unlock()
	permit, tasks := issueTestJobPermit(t, config.FullName(), 1)
	scope := lifecycle.ResourceTransactionScope{
		ID: config.FullName(),
		Successor: lifecycle.ResourceIdentity{
			ID:         config.FullName(),
			Generation: 1,
		},
	}

	transaction, err := controller.prepareDiscovered(
		context.Background(),
		DiscoveredJobChange{
			Config:  config,
			Status:  dyncfg.StatusRunning,
			Restart: true,
			retry:   token,
		},
		nil,
		scope,
		permit,
	)
	require.Nil(t, transaction)
	require.Error(t, err)
	require.False(t, controller.scheduler.retries.isCurrent(config.FullName(), token))
	require.NoError(t, permit.AbortUnused())
	require.EqualValues(t, lifecycle.LongLivedCensus{}, tasks.LongLivedCensus())
}

func TestDependencyPreparationFailureLeavesPermitForTaskSupervisor(t *testing.T) {
	controller, _, _, _, state := newDynCfgJobTestHarness(t)
	sentinel := errors.New("dependency preparation failed")
	controller.dependencies = jobDependencyIndexFunc(
		func(string, *dyncfg.GraphConfig) (func(), error) {
			return nil, sentinel
		},
	)
	config := factoryTestConfig(false)
	config.SetSourceType(confgroup.TypeDyncfg)
	config.SetSource("user=test")
	config.SetProvider("test")
	permit, tasks := issueTestJobPermit(t, config.FullName(), 1)
	scope := lifecycle.ResourceTransactionScope{
		ID: config.FullName(),
		Successor: lifecycle.ResourceIdentity{
			ID:         config.FullName(),
			Generation: 1,
		},
	}

	transaction, err := controller.prepareDiscovered(
		context.Background(),
		DiscoveredJobChange{
			Config: config,
			Status: dyncfg.StatusRunning,
		},
		nil,
		scope,
		permit,
	)
	require.Nil(t, transaction)
	require.ErrorIs(t, err, sentinel)
	require.EqualValues(t, 1, state.collectorCleanup)

	require.NoError(t, permit.AbortUnused())
	require.EqualValues(t, lifecycle.LongLivedCensus{}, tasks.LongLivedCensus())
}

func TestPrepareMutationLeavesUnusedPermitForTaskSupervisor(t *testing.T) {
	controller, _, _, _, _ := newDynCfgJobTestHarness(t)
	sentinel := errors.New("dependency preparation failed")
	controller.dependencies = jobDependencyIndexFunc(
		func(string, *dyncfg.GraphConfig) (func(), error) {
			return nil, sentinel
		},
	)
	permit, tasks := issueTestJobPermit(t, "module_job", 1)
	scope := lifecycle.ResourceTransactionScope{
		ID: "module_job",
		Successor: lifecycle.ResourceIdentity{
			ID:         "module_job",
			Generation: 1,
		},
	}

	transaction, err := controller.prepareMutation(
		scope,
		nil,
		nil,
		permit,
		lifecycle.ResourceTransactionUnchanged,
		&dyncfg.GraphConfig{
			ID:     "module_job",
			Module: "module",
			Name:   "job",
		},
		mustDynCfgMessage(200, ""),
		func() error { return nil },
	)
	require.Nil(t, transaction)
	require.ErrorIs(t, err, sentinel)
	require.NoError(t, permit.AbortUnused())
	require.EqualValues(t, lifecycle.LongLivedCensus{}, tasks.LongLivedCensus())
}

func TestPrepareMutationRollsBackAfterTransactionValidationFailure(t *testing.T) {
	controller, graph, _, _, _ := newDynCfgJobTestHarness(t)
	var events []string
	successor := &transactionTestPreparedResource{
		identity: lifecycle.ResourceIdentity{
			ID:         "module_job",
			Generation: 1,
		},
		events: &events,
	}
	scope := lifecycle.ResourceTransactionScope{
		ID:        "module_job",
		Successor: successor.identity,
	}

	transaction, err := controller.prepareMutation(
		scope,
		nil,
		successor,
		lifecycle.LongLivedPermit{},
		lifecycle.ResourceTransactionRemoved,
		&dyncfg.GraphConfig{
			ID:     "module_job",
			Module: "module",
			Name:   "job",
		},
		mustDynCfgMessage(200, ""),
		func() error { return nil },
	)
	require.Nil(t, transaction)
	require.Error(t, err)
	require.Equal(t, []string{"successor-dispose"}, events)

	mutation, err := graph.PrepareMutation([]dyncfg.GraphChange{{
		ID: "module_job",
		Config: &dyncfg.GraphConfig{
			ID:     "module_job",
			Module: "module",
			Name:   "job",
		},
	}})
	require.NoError(t, err)
	require.NoError(t, graph.Abort(mutation))
}

func newDynCfgJobTestHarness(
	t *testing.T,
) (
	*DynCfgJobController,
	*dyncfg.Graph,
	*lifecycle.TaskSupervisor,
	*bytes.Buffer,
	*factoryTestState,
) {
	t.Helper()
	output := &bytes.Buffer{}
	frames, err := lifecycle.NewFrameOwner(output)
	require.NoError(t, err)
	supervisor, err := lifecycle.NewTaskSupervisor(frames)
	require.NoError(t, err)
	state := &factoryTestState{}
	modules := collectorapi.Registry{
		"module": {
			Create: func() collectorapi.CollectorV1 {
				return state.module(nil, false)
			},
			Config: func() any {
				return &collectorapi.MockConfiguration{}
			},
			JobConfigSchema: collectorapi.MockConfigSchema,
		},
	}
	resolver, err := secretresolver.NewAtomicResolver(nil)
	require.NoError(t, err)
	configModules, err := NewConfigModuleFactory(
		ConfigModuleFactoryConfig{
			Modules:    modules,
			Resolver:   resolver,
			StoreScope: unavailableStoreScope,
		},
	)
	require.NoError(t, err)
	factory, err := NewFactory(
		FactoryConfig{
			PluginName:    "go.d",
			Modules:       modules,
			Tasks:         supervisor,
			Frames:        frames,
			ConfigModules: configModules,
			Vnodes:        vnoderegistry.New(),
			Scheduler:     newTestScheduler(t),
		},
	)
	require.NoError(t, err)
	graph, err := dyncfg.NewGraph(nil)
	require.NoError(t, err)
	controller, err := NewDynCfgJobController(
		DynCfgJobControllerConfig{
			PluginName: "go.d",
			Modules:    modules,
			Defaults: confgroup.Registry{
				"module": {UpdateEvery: 1},
			},
			Factory:       factory,
			ConfigModules: configModules,
			Graph:         graph,
			Frames:        frames,
		},
	)
	require.NoError(t, err)
	return controller, graph, supervisor, output, state
}

type jobDependencyIndexFunc func(string, *dyncfg.GraphConfig) (func(), error)

func (fn jobDependencyIndexFunc) PrepareJobChange(id string, postimage *dyncfg.GraphConfig) (func(), error) {
	return fn(id, postimage)
}

func startDynCfgJobTestTask(
	t *testing.T,
	supervisor *lifecycle.TaskSupervisor,
	plan lifecycle.TaskPlan,
) lifecycle.TaskRef {
	t.Helper()
	request, err := supervisor.Enqueue(lifecycle.TaskClassFrameworkControl, plan)
	require.NoError(t, err)
	var starts [lifecycle.TaskStartServiceQuantum]lifecycle.TaskStart
	count, _, err := supervisor.Dispatch(context.Background(), 1, &starts)
	require.NoError(t, err)
	require.EqualValues(t, 1, count)
	require.Equal(t, request, starts[0].Request)
	return starts[0].Task
}

func sendDynCfgJobTestAction(t *testing.T, supervisor *lifecycle.TaskSupervisor, action lifecycle.TaskAction) {
	t.Helper()

	require.NoError(t, supervisor.SendAction(action))

	ack := <-supervisor.AcknowledgementCh()
	require.False(
		t,
		ack.Ref != action.Ref || ack.Sequence != action.Sequence || ack.Kind != action.Kind || ack.Err != nil,
	)
}
