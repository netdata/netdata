// SPDX-License-Identifier: GPL-3.0-or-later

package joboutput

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/netdata/netdata/go/plugins/pkg/metrix"
	"github.com/netdata/netdata/go/plugins/plugin/agent/jobmgr"
	"github.com/netdata/netdata/go/plugins/plugin/agent/jobmgr/containment"
	"github.com/netdata/netdata/go/plugins/plugin/agent/jobmgr/lifecycle"
	secretresolver "github.com/netdata/netdata/go/plugins/plugin/agent/secrets/resolver"
	"github.com/netdata/netdata/go/plugins/plugin/framework/collectorapi"
	"github.com/netdata/netdata/go/plugins/plugin/framework/confgroup"
	"github.com/netdata/netdata/go/plugins/plugin/framework/dyncfg"
	"github.com/netdata/netdata/go/plugins/plugin/framework/hostoutput"
	"github.com/netdata/netdata/go/plugins/plugin/framework/jobruntime"
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

func TestDynCfgPassiveAdmissionPreservesUnavailableDependencies(t *testing.T) {
	for _, command := range []string{"add", "disabled update"} {
		t.Run(command, func(t *testing.T) {
			controller, graph, _, _, state := newDynCfgJobTestHarness(t)
			providerCalls, storeCalls, vnodeCalls := 0, 0, 0
			resolver, err := secretresolver.NewAtomicResolver(map[string]secretresolver.AtomicProvider{
				"fixture": secretresolver.AtomicProviderFunc(func(context.Context, string) ([]byte, error) {
					providerCalls++
					return nil, errors.New("provider unavailable")
				}),
			})
			require.NoError(t, err)
			controller.configModules.config.Configs = testConfigResolver(t, resolver, func([]string) (secretresolver.AtomicScope, error) {
				storeCalls++
				return nil, errors.New("Store unavailable")
			})
			controller.factory.config.Vnode = func(string) (jobruntime.VnodeSnapshot, bool) {
				vnodeCalls++
				return jobruntime.VnodeSnapshot{}, false
			}

			args := []string{"go.d:collector:module", "add", "job"}
			wantStatus, wantCode := dyncfg.StatusAccepted, 202
			if command == "disabled update" {
				config := factoryTestConfig(false).SetSourceType(confgroup.TypeDyncfg)
				seedDynCfgJobGraphRecord(t, graph, config, dyncfg.StatusDisabled)
				args = []string{"go.d:collector:module:job", "update"}
				wantStatus, wantCode = dyncfg.StatusDisabled, 200
			}
			transaction, err := controller.Prepare(context.Background(), DynCfgJobRequest{
				Args:        args,
				Payload:     []byte(`{"vnode":"missing", "option_int":"${store:vault:missing:key}", "option_str":"${fixture:value}"}`),
				ContentType: "application/json", CallerSource: "user=test", HasPayload: true,
			}, nil, lifecycle.ResourceTransactionScope{ID: "module_job"}, lifecycle.LongLivedPermit{})
			require.NoError(t, err)
			applied, err := transaction.Apply(context.Background())
			require.NoError(t, err)
			require.Equal(t, wantCode, applied.ResultStatus())
			record, exists := graph.Lookup("module_job")
			require.True(t, exists)
			require.Equal(t, wantStatus.String(), record.Status)
			config, err := graphRecordConfig(record)
			require.NoError(t, err)
			require.Equal(t, "${store:vault:missing:key}", config.Get("option_int"))
			require.Equal(t, "${fixture:value}", config.Get("option_str"))
			require.Equal(t, "missing", config.Vnode())
			require.Equal(t, confgroup.TypeDyncfg, config.SourceType())
			require.False(t, controller.ActivationEnabled(config.FullName()))
			require.Zero(t, providerCalls)
			require.Zero(t, storeCalls)
			require.Zero(t, vnodeCalls)
			require.Equal(t, 1, state.collectorCleanup)
		})
	}
}

func TestPlanDiscoveredClassifiesOnlyExternalProposalErrors(t *testing.T) {
	controller, _, _, _, _ := newDynCfgJobTestHarness(t)
	discoveredConfig := func() confgroup.Config {
		return factoryTestConfig(false).
			SetSourceType(confgroup.TypeUser).
			SetSource("file=/etc/netdata/go.d/job.conf")
	}
	tests := map[string]struct {
		change DiscoveredJobChange
		want   bool
	}{
		"clone failure": {
			change: DiscoveredJobChange{
				Config: discoveredConfig().Set("unsupported", make(chan struct{})),
				Status: dyncfg.StatusRunning,
			},
			want: true,
		},
		"unregistered module": {
			change: DiscoveredJobChange{
				Config: discoveredConfig().SetModule("missing"),
				Status: dyncfg.StatusRunning,
			},
			want: true,
		},
		"invalid identity": {
			change: DiscoveredJobChange{
				Config: discoveredConfig().SetName(""),
				Status: dyncfg.StatusRunning,
			},
			want: true,
		},
		"unpublishable job name": {
			change: DiscoveredJobChange{
				Config: discoveredConfig().SetName("bad=name"),
				Status: dyncfg.StatusRunning,
			},
			want: true,
		},
		"invalid protocol metadata": {
			change: DiscoveredJobChange{
				Config: discoveredConfig().SetSource("file=/etc/netdata/go.d/operator's.conf"),
				Status: dyncfg.StatusRunning,
			},
			want: true,
		},
		"invalid source type token": {
			change: DiscoveredJobChange{
				Config: discoveredConfig().SetSourceType("user type"),
				Status: dyncfg.StatusRunning,
			},
			want: true,
		},
		"invalid internal status": {
			change: DiscoveredJobChange{
				Config: discoveredConfig(),
				Status: dyncfg.StatusFailed,
			},
		},
	}
	for name, test := range tests {
		t.Run(name, func(t *testing.T) {
			_, err := controller.PlanDiscovered(test.change)
			require.Error(t, err)
			require.Equal(t, test.want, jobmgr.IsProposalRejection(err))
		})
	}
}

func TestPlanDiscoveredAcceptsWindowsConfigSource(t *testing.T) {
	controller, _, _, _, _ := newDynCfgJobTestHarness(t)
	config := factoryTestConfig(false).
		SetSourceType(confgroup.TypeUser).
		SetSource(`discoverer=file_reader,file=C:\Program Files\Netdata\go.d\job.conf`)

	_, err := controller.PlanDiscovered(DiscoveredJobChange{
		Config: config,
		Status: dyncfg.StatusRunning,
	})
	require.NoError(t, err)
}

func TestDynCfgAddCollisionIsReplayUpsert(t *testing.T) {
	controller, graph, _, _, _ := newDynCfgJobTestHarness(t)
	config := factoryTestConfig(false)
	config.SetSourceType(confgroup.TypeUser)
	config.SetSource("file=test")
	config.SetProvider(confgroup.TypeUser)
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

	identity := lifecycle.ResourceIdentity{
		ID:         config.FullName(),
		Generation: 1,
	}
	current := &transactionTestReadyResource{
		identity: identity,
		prefix:   "current",
		events:   new([]string),
	}
	transaction, err := controller.Prepare(
		context.Background(),
		DynCfgJobRequest{
			Args:         []string{"go.d:collector:module", "add", "job"},
			Payload:      []byte(`{"option":"replacement"}`),
			ContentType:  "application/json",
			CallerSource: "user=test",
			HasPayload:   true,
		},
		current,
		lifecycle.ResourceTransactionScope{
			ID:      config.FullName(),
			Current: identity,
		},
		lifecycle.LongLivedPermit{},
	)
	require.NoError(t, err)

	applied, err := transaction.Apply(context.Background())
	require.NoError(t, err)
	_, disposition, owned := applied.Ownership()
	require.Equal(t, lifecycle.ResourceTransactionRemoved, disposition)
	require.Nil(t, owned)
	require.Equal(t, 202, applied.ResultStatus())

	record, exists := graph.Lookup(config.FullName())
	require.True(t, exists)
	require.Equal(t, dyncfg.StatusAccepted.String(), record.Status)
	replayed, err := graphRecordConfig(record)
	require.NoError(t, err)
	require.Equal(t, confgroup.TypeDyncfg, replayed.SourceType())
	require.Equal(t, "replacement", replayed.Get("option"))
	require.NotEmpty(t, *current.events)
}

func TestDynCfgAdoptionTransfersSecretAuthorityForFullPayload(t *testing.T) {
	controller, graph, _, _, state := newDynCfgJobTestHarness(t)
	notifications := runtimeTestNotifications(t, controller)
	activations := runtimeTestBindActivations(t, controller)
	creator := controller.modules["module"]
	creator.Create = func() collectorapi.CollectorV1 {
		module := state.module(nil, false)
		charts := collectorapi.Charts{}
		module.ChartsFunc = func() *collectorapi.Charts { return &charts }
		return module
	}
	controller.modules["module"] = creator
	providerCalls := 0
	resolver, err := secretresolver.NewAtomicResolver(map[string]secretresolver.AtomicProvider{
		"fixture": secretresolver.AtomicProviderFunc(func(context.Context, string) ([]byte, error) {
			providerCalls++
			return []byte("resolved"), nil
		}),
	})
	require.NoError(t, err)
	controller.factory.config.ConfigModules.config.Configs = testConfigResolver(t, resolver, unavailableStoreScope)

	discovered := factoryTestConfig(false)
	discovered.SetSourceType(confgroup.TypeDiscovered)
	discovered.SetSource("discovery-source")
	discovered.SetProvider("discovery")
	discovered.Set("option_str", "${fixture:value}")
	discovered.Set("option_int", 1)
	require.NoError(t, controller.factory.config.ConfigModules.Validate(context.Background(), discovered))
	require.Zero(t, providerCalls)
	seedDynCfgJobGraphRecord(t, graph, discovered, dyncfg.StatusRunning)

	identity := lifecycle.ResourceIdentity{ID: discovered.FullName(), Generation: 1}
	current := &transactionTestReadyResource{identity: identity, events: new([]string)}
	permit, tasks := issueTestJobPermit(t, discovered.FullName(), 2)
	transaction, err := controller.Prepare(
		context.Background(),
		DynCfgJobRequest{
			Args: []string{"go.d:collector:module:job", "update"},
			Payload: []byte(`{
				"option_str":"${fixture:value}",
				"option_int":1
			}`),
			ContentType:  "application/json",
			CallerSource: "user=test",
			HasPayload:   true,
		},
		current,
		lifecycle.ResourceTransactionScope{
			ID:        discovered.FullName(),
			Current:   identity,
			Successor: lifecycle.ResourceIdentity{ID: discovered.FullName(), Generation: 2},
		},
		permit,
	)
	require.NoError(t, err)

	applied, err := transaction.Apply(context.Background())
	require.NoError(t, err)
	_, disposition, active := applied.Ownership()
	require.Equal(t, lifecycle.ResourceTransactionRemoved, disposition)
	require.Nil(t, active)
	active = runtimeTestApplyActivation(t, activations, discovered.FullName(), 3)
	require.NotNil(t, active)
	require.Equal(t, 202, applied.ResultStatus())
	active = runtimeTestApplyNotification(t, runtimeTestNotificationPlan(t, notifications, "internal/jobs/runtime-ready"), active)
	record, exists := graph.Lookup(discovered.FullName())
	require.True(t, exists)
	adopted, err := graphRecordConfig(record)
	require.NoError(t, err)
	require.Equal(t, confgroup.TypeDyncfg, adopted.SourceType())
	require.Equal(t, "${fixture:value}", adopted.Get("option_str"))
	callsAfterAdoption := providerCalls
	require.Positive(t, callsAfterAdoption)

	require.NoError(t, controller.factory.config.ConfigModules.Validate(context.Background(), adopted))
	require.Equal(t, callsAfterAdoption, providerCalls, "structural validation does not resolve secrets")
	require.NoError(t, controller.configModules.Test(context.Background(), adopted))
	require.Greater(t, providerCalls, callsAfterAdoption)
	require.NoError(t, active.Stop(context.Background()))
	require.NoError(t, active.Finalize())
	require.EqualValues(t, lifecycle.LongLivedCensus{}, tasks.LongLivedCensus())
}

func TestDiscoveredChangeCannotReplaceHigherPriorityGraphOwner(t *testing.T) {
	controller, graph, supervisor, _, _ := newDynCfgJobTestHarness(t)
	winner := factoryTestConfig(false).
		Set("option", "dyncfg").
		SetSourceType(confgroup.TypeDyncfg).
		SetSource("user=operator").
		SetProvider(confgroup.TypeDyncfg)
	seedDynCfgJobGraphRecord(t, graph, winner, dyncfg.StatusRunning)

	lower := factoryTestConfig(false).
		Set("option", "changed-file").
		SetSourceType(confgroup.TypeUser).
		SetSource("file=/etc/netdata/go.d/module.conf").
		SetProvider(confgroup.TypeUser)
	currentIdentity := lifecycle.ResourceIdentity{
		ID:         winner.FullName(),
		Generation: 1,
	}
	current := &transactionTestReadyResource{
		identity: currentIdentity,
		prefix:   "current",
		events:   new([]string),
	}
	successorIdentity := lifecycle.ResourceIdentity{
		ID:         winner.FullName(),
		Generation: 2,
	}
	permit, err := supervisor.IssueLongLivedPermit(
		successorIdentity,
		lifecycle.NewJobLongLivedPlan(),
	)
	require.NoError(t, err)
	transaction, err := controller.prepareDiscovered(
		context.Background(),
		DiscoveredJobChange{
			Config: lower,
			Status: dyncfg.StatusRunning,
		},
		current,
		lifecycle.ResourceTransactionScope{
			ID:        winner.FullName(),
			Current:   currentIdentity,
			Successor: successorIdentity,
		},
		permit,
	)
	require.NoError(t, err)

	applied, err := transaction.Apply(context.Background())
	require.NoError(t, err)
	_, disposition, owned := applied.Ownership()
	require.Equal(t, lifecycle.ResourceTransactionUnchanged, disposition)
	require.Same(t, current, owned)
	require.Empty(t, *current.events)
	require.EqualValues(t, lifecycle.LongLivedCensus{}, supervisor.LongLivedCensus())

	record, exists := graph.Lookup(winner.FullName())
	require.True(t, exists)
	active, err := graphRecordConfig(record)
	require.NoError(t, err)
	require.Equal(t, winner.UID(), active.UID())
}

func TestDiscoveredRemovalCannotDeleteDifferentGraphOwner(t *testing.T) {
	tests := map[string]struct {
		winner  confgroup.Config
		removed confgroup.Config
	}{
		"lower source cannot remove dyncfg": {
			winner: factoryTestConfig(false).
				Set("option", "dyncfg").
				SetSourceType(confgroup.TypeDyncfg).
				SetSource("user=operator").
				SetProvider(confgroup.TypeDyncfg),
			removed: factoryTestConfig(false).
				Set("option", "file").
				SetSourceType(confgroup.TypeUser).
				SetSource("file=/etc/netdata/go.d/module.conf").
				SetProvider(confgroup.TypeUser),
		},
		"stale same-priority candidate cannot remove current candidate": {
			winner: factoryTestConfig(false).
				Set("option", "current").
				SetSourceType(confgroup.TypeUser).
				SetSource("file=/etc/netdata/go.d/current.conf").
				SetProvider(confgroup.TypeUser),
			removed: factoryTestConfig(false).
				Set("option", "stale").
				SetSourceType(confgroup.TypeUser).
				SetSource("file=/etc/netdata/go.d/stale.conf").
				SetProvider(confgroup.TypeUser),
		},
	}
	for name, test := range tests {
		t.Run(name, func(t *testing.T) {
			controller, graph, _, _, _ := newDynCfgJobTestHarness(t)
			seedDynCfgJobGraphRecord(t, graph, test.winner, dyncfg.StatusAccepted)

			transaction, err := controller.prepareDiscovered(
				context.Background(),
				DiscoveredJobChange{
					Config: test.removed,
					Remove: true,
				},
				nil,
				lifecycle.ResourceTransactionScope{
					ID: test.winner.FullName(),
				},
				lifecycle.LongLivedPermit{},
			)
			require.NoError(t, err)
			_, err = transaction.Apply(context.Background())
			require.NoError(t, err)

			record, exists := graph.Lookup(test.winner.FullName())
			require.True(t, exists)
			active, err := graphRecordConfig(record)
			require.NoError(t, err)
			require.Equal(t, test.winner.UID(), active.UID())
		})
	}
}

func TestDynCfgProtocolIDsMatchDeclaredConfigType(t *testing.T) {
	tests := map[string]struct {
		policy       collectorapi.InstancePolicy
		name         string
		wantID       string
		wantType     dyncfg.ConfigType
		wantTemplate bool
	}{
		"per-job config with distinct job name": {
			name:         "job",
			wantID:       "go.d:collector:module:job",
			wantType:     dyncfg.ConfigTypeJob,
			wantTemplate: true,
		},
		"per-job config with module job name": {
			name:         "module",
			wantID:       "go.d:collector:module:module",
			wantType:     dyncfg.ConfigTypeJob,
			wantTemplate: true,
		},
		"single-instance config": {
			policy:   collectorapi.InstancePolicySingle,
			name:     "module",
			wantID:   "go.d:collector:module",
			wantType: dyncfg.ConfigTypeSingle,
		},
	}
	for name, test := range tests {
		t.Run(name, func(t *testing.T) {
			controller, _, _, output, _ := newDynCfgJobTestHarness(t)
			creator := controller.modules["module"]
			creator.InstancePolicy = test.policy
			controller.modules["module"] = creator

			releaseTemplates := controller.templatePublicationCleanup()
			require.NoError(t, releaseTemplates())
			releaseConfig := controller.configCreateCleanup(
				dyncfg.GraphConfig{
					Module: "module",
					Name:   test.name,
					Status: dyncfg.StatusAccepted.String(),
				},
				confgroup.TypeStock,
				"stock",
				controller.configType(creator),
			)
			require.NoError(t, releaseConfig())

			wire := output.String()
			template := "CONFIG go.d:collector:module create accepted template"
			require.Equal(t, test.wantTemplate, strings.Contains(wire, template))
			require.Contains(t, wire, "CONFIG "+test.wantID+" create accepted "+test.wantType.String())
			requireDynCfgJobTemplateParents(t, wire)
		})
	}
}

func TestManualDynCfgPreflightRejectionAndAcceptedEnableFailure(t *testing.T) {
	tests := map[string]struct {
		command         dyncfg.Command
		status          dyncfg.Status
		collectorV2     bool
		checkErr        error
		retry           int
		current         bool
		payload         []byte
		wantCode        int
		wantCleanup     int
		wantDisposition lifecycle.ResourceTransactionDisposition
		wantMessage     string
	}{
		"v2 enable accepts before failure without retry": {
			command:         dyncfg.CommandEnable,
			status:          dyncfg.StatusDisabled,
			collectorV2:     true,
			checkErr:        errors.New("check failed"),
			wantCode:        202,
			wantCleanup:     1,
			wantDisposition: lifecycle.ResourceTransactionUnchanged,
			wantMessage:     "",
		},
		"v2 enable accepts before failure with retry": {
			command:         dyncfg.CommandEnable,
			status:          dyncfg.StatusDisabled,
			collectorV2:     true,
			checkErr:        errors.New("check failed"),
			retry:           1,
			wantCode:        202,
			wantCleanup:     1,
			wantDisposition: lifecycle.ResourceTransactionUnchanged,
			wantMessage:     "",
		},
		"v2 enable accepts before permanent failure": {
			command:         dyncfg.CommandEnable,
			status:          dyncfg.StatusDisabled,
			collectorV2:     true,
			checkErr:        collectorapi.PermanentError(errors.New("unknown profile")),
			retry:           1,
			wantCode:        202,
			wantCleanup:     1,
			wantDisposition: lifecycle.ResourceTransactionUnchanged,
			wantMessage:     "",
		},
		"enable accepts before failure without retry": {
			command:         dyncfg.CommandEnable,
			status:          dyncfg.StatusDisabled,
			checkErr:        errors.New("check failed"),
			wantCode:        202,
			wantCleanup:     1,
			wantDisposition: lifecycle.ResourceTransactionUnchanged,
			wantMessage:     "",
		},
		"update unclassified failure without retry keeps the running job": {
			command:         dyncfg.CommandUpdate,
			status:          dyncfg.StatusRunning,
			checkErr:        errors.New("check failed"),
			current:         true,
			payload:         []byte(`{"option":"replacement"}`),
			wantCode:        422,
			wantCleanup:     1,
			wantDisposition: lifecycle.ResourceTransactionUnchanged,
			wantMessage:     "config update failed: check failed",
		},
		"update failure with retry keeps the running job": {
			command:         dyncfg.CommandUpdate,
			status:          dyncfg.StatusRunning,
			checkErr:        errors.New("check failed"),
			current:         true,
			payload:         []byte(`{"option":"replacement","autodetection_retry":1}`),
			wantCode:        422,
			wantCleanup:     1,
			wantDisposition: lifecycle.ResourceTransactionUnchanged,
			wantMessage:     "config update failed: check failed",
		},
		"restart of a failed job without retry is rejected": {
			command:         dyncfg.CommandRestart,
			status:          dyncfg.StatusFailed,
			checkErr:        errors.New("check failed"),
			wantCode:        422,
			wantCleanup:     1,
			wantDisposition: lifecycle.ResourceTransactionUnchanged,
			wantMessage:     "config restart failed: check failed",
		},
	}
	for name, test := range tests {
		t.Run(name, func(t *testing.T) {
			controller, graph, supervisor, output, state := newDynCfgJobTestHarness(t)
			if test.collectorV2 {
				useV2CheckFailureCollector(controller, state, test.checkErr)
			} else {
				creator := controller.modules["module"]
				creator.Create = func() collectorapi.CollectorV1 {
					return state.module(func(context.Context) error { return test.checkErr }, false)
				}
				controller.modules["module"] = creator
			}

			var activation *activationTestCommands
			if test.command == dyncfg.CommandEnable {
				activation = dynCfgTestActivationCommands(t, controller)
			}
			config := factoryTestConfig(false)
			if test.retry > 0 {
				config.Set("autodetection_retry", test.retry)
			}
			config.SetSourceType(confgroup.TypeDyncfg)
			config.SetSource("user=test")
			config.SetProvider(confgroup.TypeDyncfg)
			payload, err := yaml.Marshal(config)
			require.NoError(t, err)
			mutation, err := graph.PrepareMutation([]dyncfg.GraphChange{{
				ID: config.FullName(),
				Config: &dyncfg.GraphConfig{
					ID:      config.FullName(),
					Module:  config.Module(),
					Name:    config.Name(),
					Status:  test.status.String(),
					Payload: payload,
				},
			}})
			require.NoError(t, err)
			require.NoError(t, graph.Commit(mutation))

			var events []string
			var current lifecycle.ReadyResource
			scope := lifecycle.ResourceTransactionScope{
				ID: config.FullName(),
			}
			nextGeneration := uint64(1)
			if test.current {
				scope.Current = lifecycle.ResourceIdentity{
					ID:         config.FullName(),
					Generation: 1,
				}
				current = &transactionTestReadyResource{
					identity: scope.Current,
					prefix:   "current",
					events:   &events,
				}
				nextGeneration++
			}
			scope.Successor = lifecycle.ResourceIdentity{
				ID:         config.FullName(),
				Generation: nextGeneration,
			}
			request := DynCfgJobRequest{
				Args:         []string{"go.d:collector:module:job", string(test.command)},
				Payload:      test.payload,
				ContentType:  "application/json",
				CallerSource: "user=test",
				HasPayload:   len(test.payload) != 0,
			}
			plan, err := lifecycle.NewResourceTransactionPermitTaskPlan(
				lifecycle.SourceFunction,
				time.Time{},
				lifecycle.TransactionTaskPhases,
				current,
				scope,
				lifecycle.NewJobLongLivedPlan(),
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

			uid := strings.ReplaceAll(name, " ", "-")
			disposition, active := applyAndEncodeDynCfgJobTestTask(t, supervisor, plan, scope, uid)
			require.Equal(t, test.wantDisposition, disposition)
			if test.current && disposition == lifecycle.ResourceTransactionUnchanged {
				// A rejected command keeps the running job untouched.
				require.Same(t, current, active)
				require.Empty(t, events)
			} else {
				require.Nil(t, active)
			}

			wire := output.String()
			require.Contains(t, wire, fmt.Sprintf("FUNCTION_RESULT_BEGIN %s %d application/json", uid, test.wantCode))
			require.Contains(t, wire, test.wantMessage)
			if activation != nil {
				record, _ := graph.Lookup(config.FullName())
				require.Equal(t, dyncfg.StatusAccepted.String(), record.Status)
				require.NotContains(t, wire, test.checkErr.Error())
				require.Nil(t, applyActivationTestSubmission(t, activation.next(t, "internal/jobs/accepted-activation"), nil, nextGeneration+1))
				record, _ = graph.Lookup(config.FullName())
				require.Equal(t, dyncfg.StatusFailed.String(), record.Status)
				require.Equal(t, test.retry > 0 && collectorapi.ClassifyLifecycleError(test.checkErr) != collectorapi.LifecycleErrorPermanent, runtimeTestHasRetry(controller, config.FullName()))
			} else if test.command == dyncfg.CommandUpdate {
				record, _ := graph.Lookup(config.FullName())
				require.Equal(t, test.status.String(), record.Status)
				require.Equal(t, string(payload), record.Payload())
				require.False(t, runtimeTestHasRetry(controller, config.FullName()))
			}
			require.EqualValues(t, test.wantCleanup, state.collectorCleanup)
			require.EqualValues(t, lifecycle.LongLivedCensus{}, supervisor.LongLivedCensus())
		})
	}
}

func requireDynCfgJobTemplateParents(t *testing.T, wire string) {
	t.Helper()
	templates := make(map[string]struct{})
	for line := range strings.SplitSeq(strings.TrimSpace(wire), "\n") {
		fields := strings.Fields(line)
		if len(fields) < 5 || fields[0] != "CONFIG" || fields[2] != "create" {
			continue
		}
		switch fields[4] {
		case dyncfg.ConfigTypeTemplate.String():
			templates[fields[1]] = struct{}{}
		case dyncfg.ConfigTypeJob.String():
			separator := strings.LastIndexByte(fields[1], ':')
			require.Positive(t, separator, "job config ID %q has no template parent", fields[1])
			_, exists := templates[fields[1][:separator]]
			require.True(t, exists, "job config ID %q was emitted without its template parent", fields[1])
		}
	}
}

func TestDynCfgQuarantineSettlesEnableAndRejectsPreflightEdits(t *testing.T) {
	type prepareCommand func(
		context.Context,
		*DynCfgJobController,
		dynCfgTarget,
		dyncfg.GraphRecord,
		lifecycle.ResourceTransactionScope,
		lifecycle.LongLivedPermit,
	) (lifecycle.PreparedResourceTransaction, error)
	tests := map[string]struct {
		status  dyncfg.Status
		prepare prepareCommand
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
			status: dyncfg.StatusRunning,
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
			controller, graph, _, _, _ := newDynCfgJobTestHarness(t)
			var activation *activationTestCommands
			if name == "enable" {
				activation = dynCfgTestActivationCommands(t, controller)
			}
			creator := controller.modules["module"]
			creator.Create = func() collectorapi.CollectorV1 {
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
			require.NoError(t, err)
			applied, err := transaction.Apply(context.Background())
			require.NoError(t, err)
			_, disposition, current := applied.Ownership()
			require.Equal(t, lifecycle.ResourceTransactionUnchanged, disposition)
			require.Nil(t, current)
			if activation != nil {
				require.Equal(t, 202, applied.ResultStatus())
				require.Nil(t, applyActivationTestSubmission(t, activation.next(t, "internal/jobs/accepted-activation"), nil, 2))
			} else {
				require.Equal(t, 503, applied.ResultStatus())
			}

			after, exists := graph.Lookup(config.FullName())
			require.True(t, exists)
			// Preflight rejection preserves the incumbent; accepted enable
			// reports quarantine through the later activation outcome.
			if activation != nil {
				require.Equal(t, dyncfg.StatusFailed.String(), after.Status)
			} else {
				require.Equal(t, record.Status, after.Status)
			}
			require.Equal(t, record.Payload(), after.Payload())
			require.EqualValues(t, lifecycle.LongLivedCensus{}, permitTasks.LongLivedCensus())
			attempts, ok := controller.factory.config.Attempts.(*containment.Authority)
			require.True(t, ok)
			require.Equal(t, containment.Census{
				Quarantined: 1,
			}, attempts.Census())
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
	require.NoError(t, controller.BindBackgroundWorkers(
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
	require.Equal(t, []string{"autodetection", "current-stop", "current-finalize"}, events)

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
	require.Equal(t, []string{"autodetection", "current-stop", "current-finalize"}, events)
	require.EqualValues(t, lifecycle.LongLivedCensus{}, retryTasks.LongLivedCensus())

	controller.scheduler.StopBackgroundWorkers()
	require.NoError(t, controller.scheduler.WaitBackgroundWorkers(context.Background()))
}

func TestNonRetryableAutoDetectionFailureSettlesExistingRetry(t *testing.T) {
	controller, graph, _, _, state := newDynCfgJobTestHarness(t)
	creator := controller.modules["module"]
	creator.Create = func() collectorapi.CollectorV1 {
		return state.module(func(context.Context) error {
			return collectorapi.PermanentError(errors.New("non-retryable autodetection failure"))
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

func TestV2CheckErrorClassificationControlsAutoDetectionRetry(t *testing.T) {
	type outcome struct {
		Listed bool
		Status string
		Retry  bool
	}
	failed := dyncfg.StatusFailed.String()
	tests := map[string]struct {
		sourceType string
		checkErr   error
		want       outcome
	}{
		"unclassified error keeps configured retry": {
			sourceType: confgroup.TypeUser,
			checkErr:   errors.New("endpoint unreachable"),
			want:       outcome{Listed: true, Status: failed, Retry: true},
		},
		"permanent error stops retry": {
			sourceType: confgroup.TypeUser,
			checkErr:   collectorapi.PermanentError(errors.New("unknown profile")),
			want:       outcome{Listed: true, Status: failed, Retry: false},
		},
		"temporary error keeps configured retry": {
			sourceType: confgroup.TypeUser,
			checkErr:   collectorapi.TemporaryError(errors.New("port in use")),
			want:       outcome{Listed: true, Status: failed, Retry: true},
		},
		"discovered permanent error stops retry": {
			sourceType: confgroup.TypeDiscovered,
			checkErr:   collectorapi.PermanentError(errors.New("unknown profile")),
			want:       outcome{Listed: true, Status: failed, Retry: false},
		},
		"stock unclassified error removes the job": {
			sourceType: confgroup.TypeStock,
			checkErr:   errors.New("endpoint unreachable"),
			want:       outcome{Listed: false, Retry: true},
		},
		"stock permanent error keeps the job listed": {
			sourceType: confgroup.TypeStock,
			checkErr:   collectorapi.PermanentError(errors.New("unknown profile")),
			want:       outcome{Listed: true, Status: failed, Retry: false},
		},
	}
	for name, test := range tests {
		t.Run(name, func(t *testing.T) {
			controller, graph, _, _, state := newDynCfgJobTestHarness(t)
			useV2CheckFailureCollector(controller, state, fmt.Errorf("check: %w", test.checkErr))
			config := factoryTestConfig(false).Set("autodetection_retry", 7)
			config.SetSourceType(test.sourceType)
			config.SetSource("source")
			config.SetProvider("provider")

			current := runtimeTestApply(t, prepareRuntimeTestAdoption(t, controller, config, nil, 1))
			require.Nil(t, current)

			record, listed := graph.Lookup(config.FullName())
			require.Equal(t, test.want, outcome{
				Listed: listed,
				Status: record.Status,
				Retry:  runtimeTestHasRetry(controller, config.FullName()),
			})
			requireFactoryAttemptsIdle(t, controller.factory)
		})
	}
}

func TestV2DynCfgTestCommandReportsCheckErrorClasses(t *testing.T) {
	tests := map[string]struct {
		checkErr error
		wantCode int
	}{
		"permanent error": {checkErr: collectorapi.PermanentError(errors.New("unknown profile")), wantCode: 422},
		"temporary error": {checkErr: collectorapi.TemporaryError(errors.New("dependency not ready")), wantCode: 503},
		"unclassified":    {checkErr: errors.New("endpoint unreachable"), wantCode: 422},
	}
	for name, test := range tests {
		t.Run(name, func(t *testing.T) {
			controller, _, _, _, state := newDynCfgJobTestHarness(t)
			useV2CheckFailureCollector(controller, state, test.checkErr)

			result, err := controller.Handle(context.Background(), DynCfgJobRequest{
				Args:         []string{"go.d:collector:module", string(dyncfg.CommandTest), "job"},
				Payload:      []byte(`{"option_str":"value","option_int":1}`),
				ContentType:  "application/json",
				CallerSource: "user=test",
				HasPayload:   true,
			})
			require.NoError(t, err)
			require.Equal(t, mustDynCfgMessage(test.wantCode, "job output: collector check: "+test.checkErr.Error()), result)
		})
	}
}

// useV2CheckFailureCollector registers a V2 test collector whose Check returns checkErr.
func useV2CheckFailureCollector(controller *DynCfgJobController, state *factoryTestState, checkErr error) {
	creator := controller.modules["module"]
	creator.Create = nil
	creator.CreateV2 = func() collectorapi.CollectorV2 {
		return &factoryTestV2{
			state:    state,
			store:    metrix.NewCollectorStore(),
			template: factoryTestChartTemplate,
			checkErr: checkErr,
		}
	}
	controller.modules["module"] = creator
}

func TestDiscoveredSecretReferenceRemainsLiteralAndDoesNotScheduleRetry(t *testing.T) {
	controller, graph, _, _, state := newDynCfgJobTestHarness(t)
	creator := controller.modules["module"]
	creator.Create = func() collectorapi.CollectorV1 {
		module := state.module(nil, false)
		charts := collectorapi.Charts{}
		module.ChartsFunc = func() *collectorapi.Charts { return &charts }
		return module
	}
	controller.modules["module"] = creator
	installFailingFixtureResolver(t, controller)
	commands := runtimeTestNotifications(t, controller)
	activations := runtimeTestBindActivations(t, controller)

	config := factoryTestConfig(false)
	config.Set("option_str", "${fixture:value}")
	config.Set("option_int", 1)
	config.Set("autodetection_retry", 1)
	config.SetSourceType(confgroup.TypeDiscovered)
	config.SetSource("discovery-source")
	config.SetProvider("discovery")
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
	require.NoError(t, err)
	applied, err := transaction.Apply(context.Background())
	require.NoError(t, err)
	_, disposition, current := applied.Ownership()
	require.Equal(t, lifecycle.ResourceTransactionUnchanged, disposition)
	require.Nil(t, current)
	current = runtimeTestApplyActivation(t, activations, config.FullName(), 2)
	require.NotNil(t, current)

	current = runtimeTestApplyNotification(t, runtimeTestNotificationPlan(t, commands, "internal/jobs/runtime-ready"), current)
	record, ok := graph.Lookup(config.FullName())
	require.True(t, ok)
	require.Equal(t, dyncfg.StatusRunning.String(), record.Status)
	require.Zero(t, state.collectorCleanup)

	require.NoError(t, controller.scheduler.Tick(context.Background(), 0))
	require.NoError(t, controller.scheduler.Tick(context.Background(), 1))
	submitted, _, _ := commands.snapshot()
	require.Len(t, submitted, 1)
	require.Equal(t, "internal/jobs/runtime-ready", submitted[0].Route)
	require.False(t, runtimeTestHasRetry(controller, config.FullName()))

	require.NoError(t, current.Stop(context.Background()))
	require.NoError(t, current.Finalize())
	require.EqualValues(t, lifecycle.LongLivedCensus{}, tasks.LongLivedCensus())
}

func TestDiscoveredSecretReferenceRemainsLiteralInV2Construction(t *testing.T) {
	controller, graph, _, _, state := newDynCfgJobTestHarness(t)
	notifications := runtimeTestNotifications(t, controller)
	activations := runtimeTestBindActivations(t, controller)
	creator := controller.modules["module"]
	creator.Create = nil
	creator.CreateV2 = func() collectorapi.CollectorV2 {
		return &factoryTestV2{
			state:    state,
			store:    metrix.NewCollectorStore(),
			template: factoryTestChartTemplate,
		}
	}
	controller.modules["module"] = creator
	installFailingFixtureResolver(t, controller)

	config := factoryTestConfig(false)
	config.Set("option_str", "${fixture:value}")
	config.Set("option_int", 1)
	config.SetSourceType(confgroup.TypeDiscovered)
	config.SetSource("discovery-source")
	config.SetProvider("discovery")
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
		DiscoveredJobChange{Config: config, Status: dyncfg.StatusRunning},
		nil,
		scope,
		permit,
	)
	require.NoError(t, err)
	applied, err := transaction.Apply(context.Background())
	require.NoError(t, err)
	_, disposition, current := applied.Ownership()
	require.Equal(t, lifecycle.ResourceTransactionUnchanged, disposition)
	require.Nil(t, current)
	current = runtimeTestApplyActivation(t, activations, config.FullName(), 2)
	require.NotNil(t, current)

	current = runtimeTestApplyNotification(t, runtimeTestNotificationPlan(t, notifications, "internal/jobs/runtime-ready"), current)
	record, ok := graph.Lookup(config.FullName())
	require.True(t, ok)
	require.Equal(t, dyncfg.StatusRunning.String(), record.Status)
	require.NoError(t, current.Stop(context.Background()))
	require.NoError(t, current.Finalize())
	require.EqualValues(t, lifecycle.LongLivedCensus{}, tasks.LongLivedCensus())
}

func TestDiscoveredInvalidConfigurationIsProposalRejection(t *testing.T) {
	controller, _, _, _, _ := newDynCfgJobTestHarness(t)
	config := factoryTestConfig(false)
	config.Set("option_int", "not-an-integer")
	config.SetSourceType(confgroup.TypeDiscovered)
	config.SetSource("discovery-source")
	config.SetProvider("discovery")
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
	require.True(t, jobmgr.IsProposalRejection(err), "error: %v", err)
	require.NoError(t, permit.AbortUnused())
	require.EqualValues(t, lifecycle.LongLivedCensus{}, tasks.LongLivedCensus())
}

func TestManualEnableAcceptsBeforeProviderFailureAndRetries(t *testing.T) {
	controller, graph, _, _, _ := newDynCfgJobTestHarness(t)
	installFailingFixtureResolver(t, controller)
	commands := dynCfgTestActivationCommands(t, controller)

	config := factoryTestConfig(false)
	config.Set("option", "${fixture:value}")
	config.Set("autodetection_retry", 1)
	config.SetSourceType(confgroup.TypeDyncfg)
	config.SetSource("user=test")
	config.SetProvider("dyncfg")
	payload, err := yaml.Marshal(config)
	require.NoError(t, err)
	mutation, err := graph.PrepareMutation([]dyncfg.GraphChange{{
		ID: config.FullName(),
		Config: &dyncfg.GraphConfig{
			ID:      config.FullName(),
			Module:  config.Module(),
			Name:    config.Name(),
			Status:  dyncfg.StatusDisabled.String(),
			Payload: payload,
		},
	}})
	require.NoError(t, err)
	require.NoError(t, graph.Commit(mutation))
	record, ok := graph.Lookup(config.FullName())
	require.True(t, ok)
	permit, tasks := issueTestJobPermit(t, config.FullName(), 1)
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
		creator:    controller.modules[config.Module()],
	}

	transaction, err := controller.prepareEnable(context.Background(), target, record, true, nil, scope, permit)
	require.NoError(t, err)
	_, err = transaction.Apply(context.Background())
	require.NoError(t, err)

	record, ok = graph.Lookup(config.FullName())
	require.True(t, ok)
	require.Equal(t, dyncfg.StatusAccepted.String(), record.Status)
	require.Nil(t, applyActivationTestSubmission(t, commands.next(t, "internal/jobs/accepted-activation"), nil, 2))
	record, _ = graph.Lookup(config.FullName())
	require.Equal(t, dyncfg.StatusFailed.String(), record.Status)
	require.EqualValues(t, lifecycle.LongLivedCensus{}, tasks.LongLivedCensus())

	require.NoError(t, controller.scheduler.Tick(context.Background(), 0))
	require.NoError(t, controller.scheduler.Tick(context.Background(), 1))
	retry := commands.next(t, "internal/jobs/autodetection-retry")
	require.False(t, retry.plan.Transaction.AllocateSuccessor)
	require.Nil(t, applyActivationTestSubmission(t, retry, nil, 0))
	record, _ = graph.Lookup(config.FullName())
	require.Equal(t, dyncfg.StatusAccepted.String(), record.Status)
	require.Nil(t, applyActivationTestSubmission(t, commands.next(t, "internal/jobs/accepted-activation"), nil, 3))
}

func TestRunningUpdateProviderFailurePreservesIncumbentDespiteRetry(t *testing.T) {
	controller, graph, _, _, _ := newDynCfgJobTestHarness(t)
	installFailingFixtureResolver(t, controller)
	commands := &autoDetectionRetryTestCommands{}
	require.NoError(t, controller.BindBackgroundWorkers(commands, 1, func(error) {}))

	config := factoryTestConfig(false)
	config.Set("autodetection_retry", 1)
	config.SetSourceType(confgroup.TypeDyncfg)
	config.SetSource("user=test")
	config.SetProvider("dyncfg")
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
	record, ok := graph.Lookup(config.FullName())
	require.True(t, ok)

	var events []string
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
	target := dynCfgTarget{
		module:     config.Module(),
		name:       config.Name(),
		resourceID: config.FullName(),
		creator:    controller.modules[config.Module()],
	}
	request := DynCfgJobRequest{
		Payload: []byte(`{
			"option":"${fixture:value}",
			"autodetection_retry":1
		}`),
		ContentType:  "application/json",
		CallerSource: "user=test",
		HasPayload:   true,
	}

	transaction, err := controller.prepareUpdate(
		context.Background(),
		request,
		target,
		record,
		true,
		current,
		scope,
		permit,
	)
	require.NoError(t, err)
	applied, err := transaction.Apply(context.Background())
	require.NoError(t, err)

	record, ok = graph.Lookup(config.FullName())
	require.True(t, ok)
	require.Equal(t, dyncfg.StatusRunning.String(), record.Status)
	require.Equal(t, string(payload), record.Payload())
	require.Equal(t, 503, applied.ResultStatus())
	_, disposition, active := applied.Ownership()
	require.Equal(t, lifecycle.ResourceTransactionUnchanged, disposition)
	require.Same(t, current, active)
	require.Empty(t, events)
	require.EqualValues(t, lifecycle.LongLivedCensus{}, tasks.LongLivedCensus())

	require.NoError(t, controller.scheduler.Tick(context.Background(), 0))
	require.NoError(t, controller.scheduler.Tick(context.Background(), 1))
	commands.waitForSubmissions(t, 0)
	controller.scheduler.StopBackgroundWorkers()
	require.NoError(t, controller.scheduler.WaitBackgroundWorkers(context.Background()))
}

func TestRunningUpdateRejectsStaleStoreCandidateAndPreservesIncumbent(t *testing.T) {
	controller, graph, _, _, state := newDynCfgJobTestHarness(t)
	scopeState := &factoryTestAtomicScope{
		value: "initial",
	}
	creator := controller.modules["module"]
	creator.Create = func() collectorapi.CollectorV1 {
		module := state.module(func(context.Context) error {
			scopeState.current.Store(false)
			return nil
		}, false)
		charts := collectorapi.Charts{}
		module.ChartsFunc = func() *collectorapi.Charts { return &charts }
		return module
	}
	controller.modules["module"] = creator
	controller.factory.config.ConfigModules.config.Configs = testConfigResolver(t, testAtomicResolver(t), func([]string) (secretresolver.AtomicScope, error) {
		scopeState.current.Store(true)
		return scopeState, nil
	})

	config := factoryTestConfig(false)
	config.SetSourceType(confgroup.TypeDyncfg)
	config.SetSource("user=test")
	config.SetProvider(confgroup.TypeDyncfg)
	seedDynCfgJobGraphRecord(t, graph, config, dyncfg.StatusRunning)
	record, exists := graph.Lookup(config.FullName())
	require.True(t, exists)
	incumbentPayload := record.Payload()

	currentIdentity := lifecycle.ResourceIdentity{
		ID:         config.FullName(),
		Generation: 1,
	}
	current := &transactionTestReadyResource{
		identity: currentIdentity,
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
	target := dynCfgTarget{
		module:     config.Module(),
		name:       config.Name(),
		resourceID: config.FullName(),
		creator:    creator,
	}
	transaction, err := controller.prepareUpdate(
		context.Background(),
		DynCfgJobRequest{
			Payload:      []byte(`{"option_str":"${store:vault:test:key}"}`),
			ContentType:  "application/json",
			CallerSource: "user=test",
			HasPayload:   true,
		},
		target,
		record,
		true,
		current,
		scope,
		permit,
	)
	require.NoError(t, err)
	applied, err := transaction.Apply(context.Background())
	require.NoError(t, err)
	_, disposition, active := applied.Ownership()
	require.Equal(t, lifecycle.ResourceTransactionUnchanged, disposition)
	require.Same(t, current, active)
	require.Equal(t, 503, applied.ResultStatus())
	record, exists = graph.Lookup(config.FullName())
	require.True(t, exists)
	require.Equal(t, dyncfg.StatusRunning.String(), record.Status)
	require.Equal(t, incumbentPayload, record.Payload())
	require.Eventually(t, func() bool {
		return tasks.LongLivedCensus() == (lifecycle.LongLivedCensus{})
	}, time.Second, time.Millisecond)
	requireFactoryAttemptsIdle(t, controller.factory)
	require.EqualValues(t, 1, state.collectorCleanup)
}

func TestRunningUpdateAcceptsAndWaitsForBusyRuntimeRelease(t *testing.T) {
	controller, graph, _, _, state := newDynCfgJobTestHarness(t)
	delegate := controller.factory.config.Attempts.(*containment.Authority)
	release := make(chan struct{})
	commands := dynCfgTestActivationCommands(t, controller)
	controller.factory.config.Attempts = runtimeBusyTestAuthority{
		delegate: delegate, release: release,
	}
	creator := controller.modules["module"]
	creator.Create = func() collectorapi.CollectorV1 {
		module := state.module(nil, false)
		charts := collectorapi.Charts{}
		module.ChartsFunc = func() *collectorapi.Charts { return &charts }
		return module
	}
	controller.modules["module"] = creator

	config := factoryTestConfig(false)
	config.SetSourceType(confgroup.TypeDyncfg)
	config.SetSource("user=test")
	config.SetProvider(confgroup.TypeDyncfg)
	seedDynCfgJobGraphRecord(t, graph, config, dyncfg.StatusRunning)
	record, exists := graph.Lookup(config.FullName())
	require.True(t, exists)

	var events []string
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
	target := dynCfgTarget{
		module:     config.Module(),
		name:       config.Name(),
		resourceID: config.FullName(),
		creator:    creator,
	}
	transaction, err := controller.prepareUpdate(
		context.Background(),
		DynCfgJobRequest{
			Payload:      []byte(`{"option":"replacement"}`),
			ContentType:  "application/json",
			CallerSource: "user=test",
			HasPayload:   true,
		},
		target,
		record,
		true,
		current,
		scope,
		permit,
	)
	require.NoError(t, err)
	applied, err := transaction.Apply(context.Background())
	require.NoError(t, err)
	_, disposition, active := applied.Ownership()
	require.Equal(t, lifecycle.ResourceTransactionRemoved, disposition)
	require.Nil(t, active)
	// Adoption replaces the incumbent while activation waits for runtime ownership.
	require.Equal(t, 202, applied.ResultStatus())
	record, exists = graph.Lookup(config.FullName())
	require.True(t, exists)
	require.Equal(t, dyncfg.StatusAccepted.String(), record.Status)
	require.Contains(t, record.Payload(), "replacement")
	require.Equal(t, []string{"current-stop", "current-finalize"}, events)
	require.EqualValues(t, lifecycle.LongLivedCensus{}, tasks.LongLivedCensus())
	require.EqualValues(t, 0, state.collectorCleanup, "checked candidate remains owned while waiting")
	require.True(t, controller.ActivationEnabled(config.FullName()))
	select {
	case call := <-commands.queue:
		t.Fatalf("runtime release was bypassed by %s", call.request.Route)
	case <-time.After(20 * time.Millisecond):
	}
	close(release)
	active = applyActivationTestSubmission(t, commands.next(t, "internal/jobs/accepted-activation"), nil, 3)
	require.NotNil(t, active)
	t.Cleanup(func() { stopRuntimeTestResource(t, active) })
	active = applyActivationTestSubmission(t, commands.next(t, "internal/jobs/runtime-ready"), active, 0)
	record, _ = graph.Lookup(config.FullName())
	require.Equal(t, dyncfg.StatusRunning.String(), record.Status)
}

func installFailingFixtureResolver(t *testing.T, controller *DynCfgJobController) {
	t.Helper()
	resolver, err := secretresolver.NewAtomicResolver(map[string]secretresolver.AtomicProvider{
		"fixture": secretresolver.AtomicProviderFunc(
			func(context.Context, string) ([]byte, error) {
				return nil, errors.New("fixture provider unavailable")
			},
		),
	})
	require.NoError(t, err)
	controller.factory.config.ConfigModules.config.Configs = testConfigResolver(t, resolver, unavailableStoreScope)
}

type runtimeBusyTestAuthority struct {
	delegate jobmgr.ProcessAttemptAuthority
	release  <-chan struct{}
}

func (rbta runtimeBusyTestAuthority) StartProcessAttempt(
	ctx context.Context,
	plan jobmgr.ProcessAttemptPlan,
) (jobmgr.ProcessAttempt, error) {
	if plan.Identity.Namespace == jobmgr.ProcessAttemptJobRuntime {
		select {
		case <-rbta.release:
		default:
			return nil, jobmgr.ErrProcessAttemptBusy
		}
	}
	return rbta.delegate.StartProcessAttempt(ctx, plan)
}

func (rbta runtimeBusyTestAuthority) SupersedeProcessAttempt(
	ctx context.Context,
	identity jobmgr.ProcessAttemptIdentity,
) error {
	if identity.Namespace == jobmgr.ProcessAttemptJobRuntime {
		select {
		case <-rbta.release:
		default:
			return jobmgr.ErrProcessAttemptBusy
		}
	}
	return rbta.delegate.SupersedeProcessAttempt(ctx, identity)
}

func (rbta runtimeBusyTestAuthority) CutProcessAttempt(
	identity jobmgr.ProcessAttemptIdentity,
	cause error,
) bool {
	return rbta.delegate.CutProcessAttempt(identity, cause)
}

func (rbta runtimeBusyTestAuthority) ProcessAttemptReleased(
	identity jobmgr.ProcessAttemptIdentity,
) (<-chan struct{}, bool) {
	if identity.Namespace == jobmgr.ProcessAttemptJobRuntime {
		select {
		case <-rbta.release:
		default:
			return rbta.release, true
		}
	}
	return rbta.delegate.ProcessAttemptReleased(identity)
}

func TestDiscoveredRemovalSettlesOnlyMatchingRetryWithoutGraphRecord(t *testing.T) {
	for _, sameOwner := range []bool{true, false} {
		t.Run(fmt.Sprintf("same owner %t", sameOwner), func(t *testing.T) {
			controller, graph, _, _, state := newDynCfgJobTestHarness(t)
			creator := controller.modules["module"]
			creator.Create = func() collectorapi.CollectorV1 {
				return state.module(func(context.Context) error {
					return errors.New("temporarily unavailable")
				}, false)
			}
			controller.modules["module"] = creator
			config := factoryTestConfig(false).SetSourceType(confgroup.TypeStock).
				SetSource("stock").SetProvider("stock").Set("autodetection_retry", 1)
			require.Nil(t, runtimeTestApply(t, prepareRuntimeTestAdoption(t, controller, config, nil, 1)))
			_, exists := graph.Lookup(config.FullName())
			require.False(t, exists)
			require.True(t, runtimeTestHasRetry(controller, config.FullName()))

			removed, err := config.Clone()
			require.NoError(t, err)
			if !sameOwner {
				removed.SetSource("different-stock-source")
			}
			plan, err := controller.PlanDiscovered(DiscoveredJobChange{Config: removed, Remove: true})
			require.NoError(t, err)
			transaction, err := plan.Transaction.Prepare(context.Background(), nil,
				lifecycle.ResourceTransactionScope{ID: config.FullName()}, lifecycle.LongLivedPermit{})
			require.NoError(t, err)
			_, err = transaction.Apply(context.Background())
			require.NoError(t, err)
			require.Equal(t, !sameOwner, runtimeTestHasRetry(controller, config.FullName()))
		})
	}
}

func TestPlainStockRetryAcceptsBeforeReactivatingRemovedFailedRecord(t *testing.T) {
	controller, graph, _, _, state := newDynCfgJobTestHarness(t)
	var healthy atomic.Bool
	creator := controller.modules["module"]
	creator.Create = func() collectorapi.CollectorV1 {
		module := state.module(func(context.Context) error {
			if !healthy.Load() {
				return errors.New("temporarily unavailable")
			}
			return nil
		}, false)
		charts := collectorapi.Charts{}
		module.ChartsFunc = func() *collectorapi.Charts { return &charts }
		return module
	}
	controller.modules["module"] = creator
	commands := dynCfgTestActivationCommands(t, controller)
	config := factoryTestConfig(false).SetSourceType(confgroup.TypeStock).SetSource("stock").SetProvider("stock").Set("autodetection_retry", 1)
	require.Nil(t, runtimeTestApply(t, prepareRuntimeTestAdoption(t, controller, config, nil, 1)))
	_, exists := graph.Lookup(config.FullName())
	require.False(t, exists)
	require.True(t, runtimeTestHasRetry(controller, config.FullName()))
	healthy.Store(true)
	require.NoError(t, controller.scheduler.Tick(t.Context(), 0))
	require.NoError(t, controller.scheduler.Tick(t.Context(), 1))
	retry := commands.next(t, "internal/jobs/autodetection-retry")
	require.False(t, retry.plan.Transaction.AllocateSuccessor)
	require.Nil(t, applyActivationTestSubmission(t, retry, nil, 0))
	record, exists := graph.Lookup(config.FullName())
	require.True(t, exists)
	require.Equal(t, dyncfg.StatusAccepted.String(), record.Status)
	require.False(t, runtimeTestHasRetry(controller, config.FullName()))
	current := applyActivationTestSubmission(t, commands.next(t, "internal/jobs/accepted-activation"), nil, 2)
	require.NotNil(t, current)
	t.Cleanup(func() { stopRuntimeTestResource(t, current) })
	current = applyActivationTestSubmission(t, commands.next(t, "internal/jobs/runtime-ready"), current, 0)
	record, _ = graph.Lookup(config.FullName())
	require.Equal(t, dyncfg.StatusRunning.String(), record.Status)
}

func TestRetryActivationPermanentFailureStopsRetry(t *testing.T) {
	controller, graph, _, _, state := newDynCfgJobTestHarness(t)
	var permanent atomic.Bool
	var checks atomic.Int32
	creator := controller.modules["module"]
	creator.Create = func() collectorapi.CollectorV1 {
		return state.module(func(context.Context) error {
			checks.Add(1)
			if permanent.Load() {
				return collectorapi.PermanentError(errors.New("invalid configuration"))
			}
			return errors.New("temporarily unavailable")
		}, false)
	}
	controller.modules["module"] = creator
	commands := dynCfgTestActivationCommands(t, controller)
	config := factoryTestConfig(false).SetSourceType(confgroup.TypeUser).SetSource("file=test").SetProvider("file").Set("autodetection_retry", 1)
	require.Nil(t, runtimeTestApply(t, prepareRuntimeTestAdoption(t, controller, config, nil, 1)))
	require.True(t, runtimeTestHasRetry(controller, config.FullName()))
	permanent.Store(true)
	require.NoError(t, controller.scheduler.Tick(t.Context(), 0))
	require.NoError(t, controller.scheduler.Tick(t.Context(), 1))
	retry := commands.next(t, "internal/jobs/autodetection-retry")
	transaction, err := retry.plan.Transaction.Prepare(t.Context(), nil,
		lifecycle.ResourceTransactionScope{ID: config.FullName()}, lifecycle.LongLivedPermit{})
	require.NoError(t, err)
	require.EqualValues(t, 1, checks.Load(), "retry command preparation must not probe the collector")
	_, err = transaction.Apply(t.Context())
	require.NoError(t, err)
	record, _ := graph.Lookup(config.FullName())
	require.Equal(t, dyncfg.StatusAccepted.String(), record.Status)
	require.Nil(t, applyActivationTestSubmission(t, commands.next(t, "internal/jobs/accepted-activation"), nil, 2))
	record, _ = graph.Lookup(config.FullName())
	require.Equal(t, dyncfg.StatusFailed.String(), record.Status)
	require.False(t, runtimeTestHasRetry(controller, config.FullName()))
	require.EqualValues(t, 2, checks.Load())
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
		internalReply(),
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
		internalReply(),
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
	return newDynCfgJobTestHarnessWithDiagnostics(t, nil)
}

func newDynCfgJobTestHarnessWithDiagnostics(
	t *testing.T,
	diagnostics jobmgr.DiagnosticObserver,
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
	cleanupOutput, err := NewCleanupOutputGate(frames)
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
			Modules: modules,
			Configs: testConfigResolver(t, resolver, unavailableStoreScope),
		},
	)
	require.NoError(t, err)
	attempts, err := containment.NewAuthority(diagnostics)
	require.NoError(t, err)
	t.Cleanup(func() {
		shutdownCtx, cancel := context.WithTimeout(context.Background(), time.Second)
		defer cancel()
		require.NoError(t, attempts.Shutdown(shutdownCtx))
	})
	factory, err := NewFactory(
		FactoryConfig{
			PluginName:    "go.d",
			Epoch:         9,
			Attempts:      attempts,
			Modules:       modules,
			Frames:        frames,
			CleanupOutput: cleanupOutput,
			ConfigModules: configModules,
			Publication:   hostoutput.New(),
			Scheduler:     newTestScheduler(t),
		},
	)
	require.NoError(t, err)
	factory.runWithoutClaims = testRunWithoutClaims
	graph, err := dyncfg.NewGraph(nil)
	require.NoError(t, err)
	controller, err := NewDynCfgJobController(
		DynCfgJobControllerConfig{
			PluginName: "go.d",
			Generation: 9,
			Modules:    modules,
			Defaults: confgroup.Registry{
				"module": {UpdateEvery: 1},
			},
			Factory:       factory,
			ConfigModules: configModules,
			Graph:         graph,
			Frames:        frames,
			Diagnostics:   diagnostics,
		},
	)
	require.NoError(t, err)
	return controller, graph, supervisor, output, state
}

func seedDynCfgJobGraphRecord(
	t *testing.T,
	graph *dyncfg.Graph,
	config confgroup.Config,
	status dyncfg.Status,
) {
	t.Helper()
	payload, err := yaml.Marshal(config)
	require.NoError(t, err)
	mutation, err := graph.PrepareMutation([]dyncfg.GraphChange{{
		ID: config.FullName(),
		Config: &dyncfg.GraphConfig{
			ID:      config.FullName(),
			Module:  config.Module(),
			Name:    config.Name(),
			Status:  status.String(),
			Payload: payload,
		},
	}})
	require.NoError(t, err)
	require.NoError(t, graph.Commit(mutation))
}

func TestFailedAutoDetectionPublishesConfigLifecycleOnlyAfterGraphCommit(t *testing.T) {
	controller, graph, _, _, state := newDynCfgJobTestHarness(t)
	events := []string{}
	lifecycleHook := &recordingJobConfigLifecycle{
		events: &events,
	}
	creator := controller.modules["module"]
	creator.JobConfigLifecycle = lifecycleHook
	creator.Create = func() collectorapi.CollectorV1 {
		return &collectorapi.MockCollectorV1{
			CheckFunc: func(context.Context) error {
				events = append(events, "check")
				return errors.New("check failed")
			},
			CleanupFunc: func(context.Context) {
				state.collectorCleanup++
				events = append(events, "cleanup")
			},
		}
	}
	controller.modules["module"] = creator

	config := factoryTestConfig(false)
	config.SetSourceType(confgroup.TypeDyncfg)
	config.SetSource("user=test")
	config.SetProvider("test")
	seedDynCfgJobGraphRecord(t, graph, config, dyncfg.StatusRunning)
	currentIdentity := lifecycle.ResourceIdentity{
		ID:         config.FullName(),
		Generation: 1,
	}
	current := &transactionTestReadyResource{
		identity: currentIdentity,
		prefix:   "current",
		events:   &events,
	}
	permit, _ := issueTestJobPermit(t, config.FullName(), 2)
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
	require.Equal(t, []string{"bind", "check", "capture", "cleanup"}, events)
	require.Empty(t, lifecycleHook.reconciliations, "candidate cleanup must not publish before graph commit")

	_, err = transaction.Apply(context.Background())
	require.NoError(t, err)
	record, ok := graph.Lookup(config.FullName())
	require.True(t, ok)
	require.Equal(t, dyncfg.StatusFailed.String(), record.Status)
	require.Len(t, lifecycleHook.reconciliations, 1)
	require.Equal(t, lifecycleHook.bound, lifecycleHook.reconciliations[0].current)
	require.Equal(t, lifecycleHook.bound, lifecycleHook.reconciliations[0].previous)
	require.Equal(t, "reconcile", events[len(events)-1])

	removeScope := lifecycle.ResourceTransactionScope{
		ID: config.FullName(),
	}
	removeTransaction, err := controller.prepareMutation(
		removeScope,
		nil,
		nil,
		lifecycle.LongLivedPermit{},
		lifecycle.ResourceTransactionUnchanged,
		nil,
		internalReply(),
		func() error { return nil },
	)
	require.NoError(t, err)
	require.Empty(t, lifecycleHook.removed)
	_, err = removeTransaction.Apply(context.Background())
	require.NoError(t, err)
	_, ok = graph.Lookup(config.FullName())
	require.False(t, ok)
	require.Equal(t, []collectorapi.JobConfigIdentity{lifecycleHook.bound}, lifecycleHook.removed)
}

func TestDependencyWaitPublishesConfigLifecycleOnlyAfterGraphCommit(t *testing.T) {
	controller, graph, _, _, _ := newDynCfgJobTestHarness(t)
	events := []string{}
	lifecycleHook := &recordingJobConfigLifecycle{
		events: &events,
	}
	creator := controller.modules["module"]
	creator.JobConfigLifecycle = lifecycleHook
	controller.modules["module"] = creator

	config := factoryTestConfig(false)
	config.SetSourceType(confgroup.TypeDyncfg)
	config.SetSource("user=test")
	config.SetProvider("test")
	config.Set("vnode", "missing-vnode")
	seedDynCfgJobGraphRecord(t, graph, config, dyncfg.StatusRunning)
	currentIdentity := lifecycle.ResourceIdentity{
		ID:         config.FullName(),
		Generation: 1,
	}
	current := &transactionTestReadyResource{
		identity: currentIdentity,
		prefix:   "current",
		events:   &events,
	}
	permit, _ := issueTestJobPermit(t, config.FullName(), 2)
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
	require.Empty(t, lifecycleHook.reconciliations)

	_, err = transaction.Apply(context.Background())
	require.NoError(t, err)
	record, ok := graph.Lookup(config.FullName())
	require.True(t, ok)
	require.Equal(t, dyncfg.StatusAccepted.String(), record.Status)
	require.Len(t, lifecycleHook.reconciliations, 1)
	require.Equal(t, jobConfigIdentity(config), lifecycleHook.reconciliations[0].current)
	require.Equal(t, jobConfigIdentity(config), lifecycleHook.reconciliations[0].previous)
}

type recordingJobConfigLifecycle struct {
	events          *[]string
	bound           collectorapi.JobConfigIdentity
	reconciliations []recordingJobConfigLifecycleReconciliation
	removed         []collectorapi.JobConfigIdentity
}

type recordingJobConfigLifecycleReconciliation struct {
	current  collectorapi.JobConfigIdentity
	previous collectorapi.JobConfigIdentity
}

func (r *recordingJobConfigLifecycle) Project(
	identity collectorapi.JobConfigIdentity,
	_ map[string]any,
) collectorapi.JobConfigLifecycleSnapshot {
	*r.events = append(*r.events, "project")
	return &recordingJobConfigLifecycleSnapshot{
		identity: identity,
	}
}

func (r *recordingJobConfigLifecycle) Bind(identity collectorapi.JobConfigIdentity, _ collectorapi.RuntimeJob) {
	r.bound = identity
	*r.events = append(*r.events, "bind")
}

func (r *recordingJobConfigLifecycle) Capture(
	identity collectorapi.JobConfigIdentity,
	_ collectorapi.RuntimeJob,
) collectorapi.JobConfigLifecycleSnapshot {
	*r.events = append(*r.events, "capture")
	return &recordingJobConfigLifecycleSnapshot{
		identity: identity,
	}
}

func (r *recordingJobConfigLifecycle) Reconcile(
	previous collectorapi.JobConfigIdentity,
	snapshot collectorapi.JobConfigLifecycleSnapshot,
	_ collectorapi.RuntimeJob,
) {
	r.recordReconciliation(previous, snapshot)
}

func (r *recordingJobConfigLifecycle) Remove(identity collectorapi.JobConfigIdentity) {
	r.removed = append(r.removed, identity)
	*r.events = append(*r.events, "remove")
}

type recordingJobConfigLifecycleSnapshot struct {
	identity collectorapi.JobConfigIdentity
}

func (r *recordingJobConfigLifecycleSnapshot) Identity() collectorapi.JobConfigIdentity {
	return r.identity
}

func (r *recordingJobConfigLifecycle) recordReconciliation(
	previous collectorapi.JobConfigIdentity,
	snapshot collectorapi.JobConfigLifecycleSnapshot,
) {
	current, _ := snapshot.(*recordingJobConfigLifecycleSnapshot)
	if current == nil {
		return
	}
	r.reconciliations = append(r.reconciliations, recordingJobConfigLifecycleReconciliation{
		current:  current.identity,
		previous: previous,
	})
	*r.events = append(*r.events, "reconcile")
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

func applyAndEncodeDynCfgJobTestTask(
	t *testing.T,
	supervisor *lifecycle.TaskSupervisor,
	plan lifecycle.TaskPlan,
	scope lifecycle.ResourceTransactionScope,
	uid string,
) (lifecycle.ResourceTransactionDisposition, lifecycle.ReadyResource) {
	t.Helper()
	ref := startDynCfgJobTestTask(t, supervisor, plan)
	prepared := <-supervisor.CompletionCh()
	require.NoError(t, prepared.Err)
	require.Equal(t, lifecycle.TaskOutcomePreparedResourceTransaction, prepared.Kind)
	require.NoError(t, supervisor.SendAction(lifecycle.TaskAction{
		Ref:      ref,
		Sequence: 2,
		Kind:     lifecycle.TaskActionApplyResourceTransaction,
	}))
	applied := <-supervisor.CompletionCh()
	require.NoError(t, applied.Err)
	require.Equal(t, lifecycle.TaskOutcomeAppliedResourceTransaction, applied.Kind)
	disposition, current, err := supervisor.TakeAppliedResourceTransaction(ref, 2, scope)
	require.NoError(t, err)
	require.NoError(t, supervisor.PreflightResult(ref, uid, 1))
	sendDynCfgJobTestAction(t, supervisor, lifecycle.TaskAction{
		Ref:      ref,
		Sequence: 3,
		Kind:     lifecycle.TaskActionEncodeWrite,
		UID:      uid,
		Expiry:   1,
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
	return disposition, current
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

func dynCfgTestActivationCommands(t *testing.T, controller *DynCfgJobController) *activationTestCommands {
	t.Helper()
	commands := &activationTestCommands{queue: make(chan activationTestSubmission, 8), stop: make(chan struct{})}
	require.NoError(t, controller.BindBackgroundWorkers(commands, 9, func(err error) { t.Errorf("activation failure: %v", err) }))
	t.Cleanup(func() {
		close(commands.stop)
		controller.scheduler.StopBackgroundWorkers()
		ctx, cancel := context.WithTimeout(context.Background(), time.Second)
		defer cancel()
		require.NoError(t, controller.scheduler.WaitBackgroundWorkers(ctx))
	})
	return commands
}
