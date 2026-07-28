// SPDX-License-Identifier: GPL-3.0-or-later

package joboutput

import (
	"bytes"
	"context"
	"errors"
	"testing"
	"time"

	"github.com/netdata/netdata/go/plugins/pkg/funcapi"
	"github.com/netdata/netdata/go/plugins/pkg/metrix"
	"github.com/netdata/netdata/go/plugins/plugin/agent/jobmgr"
	"github.com/netdata/netdata/go/plugins/plugin/agent/jobmgr/containment"
	"github.com/netdata/netdata/go/plugins/plugin/agent/jobmgr/lifecycle"
	secretresolver "github.com/netdata/netdata/go/plugins/plugin/agent/secrets/resolver"
	"github.com/netdata/netdata/go/plugins/plugin/framework/collectorapi"
	"github.com/netdata/netdata/go/plugins/plugin/framework/confgroup"
	"github.com/netdata/netdata/go/plugins/plugin/framework/runtimecomp"
	"github.com/netdata/netdata/go/plugins/plugin/framework/vnoderegistry"
	"github.com/netdata/netdata/go/plugins/plugin/framework/vnodes"
	"github.com/stretchr/testify/require"
)

const factoryTestChartTemplate = `
version: "v1"
groups:
  - family: "factory"
    metrics: ["factory.value"]
    charts:
      - context: "factory.value"
        title: "Factory"
        units: "value"
        dimensions:
          - selector: "factory.value"
            name: "value"
`

func TestCreatorDeclaresFunctions(t *testing.T) {
	tests := map[string]struct {
		creator collectorapi.Creator
		want    bool
	}{
		"none": {},
		"shared": {
			creator: collectorapi.Creator{
				SharedFunctions: func() []funcapi.FunctionConfig { return nil },
			},
			want: true,
		},
		"agent": {
			creator: collectorapi.Creator{
				AgentFunctions: func() []funcapi.FunctionConfig { return nil },
			},
			want: true,
		},
		"instance": {
			creator: collectorapi.Creator{
				InstanceFunctions: func(collectorapi.RuntimeJob) []funcapi.FunctionConfig { return nil },
			},
			want: true,
		},
	}
	for name, test := range tests {
		t.Run(name, func(t *testing.T) {
			require.Equal(t, test.want, creatorDeclaresFunctions(test.creator))
		})
	}
}

func TestFactoryRejectsWithExactlyOneCollectorCleanup(t *testing.T) {
	tests := map[string]struct {
		configure     func(*factoryTestState, *collectorapi.Creator) JobHooks
		wantClose     int
		wantRetained  bool
		wantNoCleanup bool
	}{
		"creator panic": {
			configure: func(*factoryTestState, *collectorapi.Creator) JobHooks {
				return nil
			},
			wantRetained:  true,
			wantNoCleanup: true,
		},
		"autodetection failure": {
			configure: func(state *factoryTestState, creator *collectorapi.Creator) JobHooks {
				creator.Create = func() collectorapi.CollectorV1 {
					return state.module(func(context.Context) error {
						return errors.New("check failed")
					}, false)
				}
				return nil
			},
		},
		"autodetection panic": {
			configure: func(state *factoryTestState, creator *collectorapi.Creator) JobHooks {
				creator.Create = func() collectorapi.CollectorV1 {
					return state.module(func(context.Context) error {
						panic("check failed")
					}, false)
				}
				return nil
			},
		},
		"collector cleanup panic": {
			configure: func(state *factoryTestState, creator *collectorapi.Creator) JobHooks {
				creator.Create = func() collectorapi.CollectorV1 {
					return state.module(func(context.Context) error {
						return errors.New("check failed")
					}, true)
				}
				return nil
			},
			wantRetained: true,
		},
		"function-bearing job without hooks": {
			configure: func(state *factoryTestState, creator *collectorapi.Creator) JobHooks {
				creator.FunctionOnly = true
				creator.SharedFunctions = func() []funcapi.FunctionConfig { return nil }
				creator.Create = func() collectorapi.CollectorV1 {
					return state.module(nil, false)
				}
				return nil
			},
		},
		"partial handler preparation failure": {
			configure: func(state *factoryTestState, creator *collectorapi.Creator) JobHooks {
				creator.FunctionOnly = true
				creator.SharedFunctions = func() []funcapi.FunctionConfig { return nil }
				creator.Create = func() collectorapi.CollectorV1 {
					return state.module(nil, false)
				}
				return factoryTestHooks{
					prepare: func(RuntimeJob) (StagedHandlerLifecycle, error) {
						return &factoryTestHandlers{
							state: state,
						}, errors.New("prepare failed")
					},
				}
			},
			wantClose: 1,
		},
		"handler preparation panic": {
			configure: func(state *factoryTestState, creator *collectorapi.Creator) JobHooks {
				creator.FunctionOnly = true
				creator.SharedFunctions = func() []funcapi.FunctionConfig { return nil }
				creator.Create = func() collectorapi.CollectorV1 {
					return state.module(nil, false)
				}
				return factoryTestHooks{
					prepare: func(RuntimeJob) (StagedHandlerLifecycle, error) {
						panic("prepare failed")
					},
				}
			},
			wantRetained: true,
		},
	}
	for name, test := range tests {
		t.Run(name, func(t *testing.T) {
			state := &factoryTestState{}
			creator := collectorapi.Creator{}
			if test.wantNoCleanup {
				creator.Create = func() collectorapi.CollectorV1 {
					panic("create failed")
				}
			}
			hooks := test.configure(state, &creator)
			factory, output := newFactoryTestHarness(t, creator, hooks)
			permit, tasks := issueTestJobPermit(t, "module_job", 1)
			prepared, err := factory.Prepare(
				context.Background(),
				factoryTestConfig(creator.FunctionOnly),
				lifecycle.ResourceIdentity{
					ID:         "module_job",
					Generation: 1,
				},
				permit,
			)
			if err == nil {
				var failure *autoDetectionFailure
				failure, err = factory.Probe(context.Background(), prepared)
				if err != nil {
					err = errors.Join(err, permit.AbortUnused())
				}
				if err == nil && failure != nil {
					err = errors.Join(failure, permit.AbortUnused())
				}
				if err == nil {
					_, err = prepared.AcceptStart(context.Background(), 1)
				}
			} else {
				err = errors.Join(err, permit.AbortUnused())
			}
			require.Error(t, err)
			wantCollectorCleanup := 1
			if test.wantNoCleanup {
				wantCollectorCleanup = 0
			}
			require.EqualValues(t, wantCollectorCleanup, state.collectorCleanup)
			require.EqualValues(t, test.wantClose, state.handlerClose)
			require.EqualValues(t, 0, output.Len())
			require.Equal(t, test.wantRetained, lifecycle.OwnershipRetained(err))
			require.EqualValues(t, lifecycle.LongLivedCensus{}, tasks.LongLivedCensus())
		})
	}
}

func TestFactoryV2RejectsWithExactlyOneCollectorCleanup(t *testing.T) {
	tests := map[string]struct {
		functionOnly bool
		checkErr     error
		hooks        JobHooks
	}{
		"autodetection failure":              {checkErr: errors.New("check failed")},
		"function-bearing job without hooks": {functionOnly: true},
	}
	for name, test := range tests {
		t.Run(name, func(t *testing.T) {
			state := &factoryTestState{}
			creator := collectorapi.Creator{
				FunctionOnly: test.functionOnly,
				CreateV2: func() collectorapi.CollectorV2 {
					return &factoryTestV2{
						state:    state,
						checkErr: test.checkErr,
					}
				},
			}
			if test.functionOnly {
				creator.SharedFunctions = func() []funcapi.FunctionConfig { return nil }
			}
			factory, output := newFactoryTestHarness(t, creator, test.hooks)
			permit, tasks := issueTestJobPermit(t, "module_job", 1)

			prepared, err := factory.Prepare(
				context.Background(),
				factoryTestConfig(test.functionOnly),
				lifecycle.ResourceIdentity{
					ID:         "module_job",
					Generation: 1,
				},
				permit,
			)
			if err == nil {
				var failure *autoDetectionFailure
				failure, err = factory.Probe(context.Background(), prepared)
				if err != nil {
					err = errors.Join(err, permit.AbortUnused())
				}
				if err == nil && failure != nil {
					err = errors.Join(failure, permit.AbortUnused())
				}
				if err == nil {
					_, err = prepared.AcceptStart(context.Background(), 1)
				}
			} else {
				err = errors.Join(err, permit.AbortUnused())
			}
			require.Error(t, err)

			require.EqualValues(t, 1, state.collectorCleanup)
			require.EqualValues(t, 0, output.Len())
			require.EqualValues(t, lifecycle.LongLivedCensus{}, tasks.LongLivedCensus())
		})
	}
}

func TestFactoryProbesAndRejectsBeforePreparedJobAcceptance(t *testing.T) {
	state := &factoryTestState{}
	creator := collectorapi.Creator{
		Create: func() collectorapi.CollectorV1 {
			return state.module(func(context.Context) error {
				state.autoDetection++
				return errors.New("check failed")
			}, false)
		},
	}
	factory, _ := newFactoryTestHarness(t, creator, nil)
	permit, tasks := issueTestJobPermit(t, "module_job", 1)

	prepared, err := factory.Prepare(
		context.Background(),
		factoryTestConfig(false),
		lifecycle.ResourceIdentity{
			ID:         "module_job",
			Generation: 1,
		},
		permit,
	)
	require.NoError(t, err)
	require.Zero(t, state.autoDetection)
	require.Zero(t, state.collectorCleanup)

	failure, err := factory.Probe(context.Background(), prepared)
	require.NoError(t, err)
	require.Error(t, failure)
	require.NoError(t, permit.AbortUnused())
	require.EqualValues(t, 1, state.autoDetection)
	require.EqualValues(t, 1, state.collectorCleanup)
	require.EqualValues(t, lifecycle.LongLivedCensus{}, tasks.LongLivedCensus())

}

func TestFactoryProbeDoesNotActivateRunPermitOrLiveV2Runtime(t *testing.T) {
	state := &factoryTestState{}
	runtime := &factoryTestRuntimeService{}
	creator := collectorapi.Creator{
		CreateV2: func() collectorapi.CollectorV2 {
			return &factoryTestV2{
				state:    state,
				store:    metrix.NewCollectorStore(),
				template: factoryTestChartTemplate,
			}
		},
	}
	factory, _ := newFactoryTestHarness(t, creator, nil)
	factory.config.Runtime = runtime
	permit, tasks := issueTestJobPermit(t, "module_job", 1)

	prepared, err := factory.Prepare(
		context.Background(),
		factoryTestConfig(false),
		lifecycle.ResourceIdentity{
			ID:         "module_job",
			Generation: 1,
		},
		permit,
	)
	require.NoError(t, err)
	// A candidate must leave the run-owned external facet reserved.
	require.NoError(t, permit.ActivateExternal())
	require.NoError(t, permit.ReleaseExternal())
	require.NoError(t, permit.Return())
	require.NoError(t, prepared.reject(context.Background()))

	permit, tasks = issueTestJobPermit(t, "module_job", 2)
	prepared, err = factory.Prepare(
		context.Background(),
		factoryTestConfig(false),
		lifecycle.ResourceIdentity{
			ID:         "module_job",
			Generation: 2,
		},
		permit,
	)
	require.NoError(t, err)
	failure, err := factory.Probe(context.Background(), prepared)
	require.NoError(t, err)
	require.Nil(t, failure)
	require.Zero(t, runtime.registrations)
	require.Zero(t, tasks.Active())

	require.NoError(t, prepared.Dispose(context.Background()))
	require.EqualValues(t, 2, state.collectorCleanup)
	require.EqualValues(t, lifecycle.LongLivedCensus{}, tasks.LongLivedCensus())
}

func TestFactoryCandidateStageOwnsProbeUntilInstallationAcknowledgement(t *testing.T) {
	state := &factoryTestState{}
	runtime := &factoryTestRuntimeService{}
	creator := collectorapi.Creator{
		CreateV2: func() collectorapi.CollectorV2 {
			return &factoryTestV2{
				state:    state,
				store:    metrix.NewCollectorStore(),
				template: factoryTestChartTemplate,
			}
		},
	}
	factory, _ := newFactoryTestHarness(t, creator, nil)
	attempts, err := containment.NewAuthority(nil)
	require.NoError(t, err)
	factory.config.Epoch = 1
	factory.config.Attempts = attempts
	factory.config.Runtime = runtime

	stage, err := factory.NewCandidate(factoryTestConfig(false))
	require.NoError(t, err)
	stage.Start()
	<-stage.Ready()
	require.Zero(t, runtime.registrations)

	permit, tasks := issueTestJobPermit(t, "module_job", 1)
	prepared, failure, err := factory.PrepareCandidate(
		lifecycle.ResourceIdentity{
			ID:         "module_job",
			Generation: 1,
		},
		permit,
		stage,
	)
	require.NoError(t, err)
	require.Nil(t, failure)
	require.NoError(t, prepared.validateLivePermit())
	require.Zero(t, tasks.Active())
	require.Zero(t, runtime.registrations)

	result, err := lifecycle.NewSealedResult(200, "application/json", []byte(`{}`))
	require.NoError(t, err)
	transaction, err := PrepareResourceTransaction(ResourceTransactionSpec{
		Scope: lifecycle.ResourceTransactionScope{
			ID: "module_job",
			Successor: lifecycle.ResourceIdentity{
				ID:         "module_job",
				Generation: 1,
			},
		},
		Disposition: lifecycle.ResourceTransactionInstalled,
		Successor:   prepared,
		Result:      result,
		Cleanup:     func() error { return nil },
	})
	require.NoError(t, err)
	applied, err := transaction.Apply(context.Background())
	require.NoError(t, err)
	_, _, resource := applied.Ownership()
	generation := resource.(*JobGeneration)
	require.Positive(t, runtime.registrations)
	released, ok := attempts.ProcessAttemptReleased(jobmgr.ProcessAttemptIdentity{
		Namespace: jobmgr.ProcessAttemptJobRuntime,
		Key:       stage.identity.Key,
		Resource:  stage.identity.Resource,
	})
	require.True(t, ok)
	stage.Release()

	require.NoError(t, generation.Stop(context.Background()))
	require.NoError(t, generation.Finalize())
	<-released
	require.EqualValues(t, 1, state.collectorCleanup)
	require.EqualValues(t, containment.Census{}, attempts.Census())
	require.EqualValues(t, lifecycle.LongLivedCensus{}, tasks.LongLivedCensus())
}

func TestFactoryCandidateStageSettlesWhileNonCooperativeProbeRemainsOwned(t *testing.T) {
	entered := make(chan struct{})
	release := make(chan struct{})
	state := &factoryTestState{}
	creator := collectorapi.Creator{
		Create: func() collectorapi.CollectorV1 {
			return state.module(func(context.Context) error {
				close(entered)
				<-release
				return nil
			}, false)
		},
	}
	factory, _ := newFactoryTestHarness(t, creator, nil)
	attempts, err := containment.NewAuthority(nil)
	require.NoError(t, err)
	factory.config.Epoch = 1
	factory.config.Attempts = attempts

	stage, err := factory.NewCandidate(factoryTestConfig(false))
	require.NoError(t, err)
	stage.Start()
	<-entered
	stage.Cancel(jobmgr.ErrProcessAttemptSuperseded)
	<-stage.Ready()

	_, failure, err := factory.PrepareCandidate(
		lifecycle.ResourceIdentity{
			ID:         "module_job",
			Generation: 1,
		},
		lifecycle.LongLivedPermit{},
		stage,
	)
	require.ErrorIs(t, err, jobmgr.ErrProcessAttemptSuperseded)
	require.Nil(t, failure)
	require.EqualValues(t, containment.Census{
		Active:    1,
		Contained: 1,
	}, attempts.Census())

	stage.Release()
	released, ok := attempts.ProcessAttemptReleased(stage.identity)
	require.True(t, ok)
	close(release)
	<-released
	require.EqualValues(t, containment.Census{}, attempts.Census())
	require.EqualValues(t, 1, state.collectorCleanup)
}

func TestFactoryCandidateIdentityRemainsExclusiveAcrossRunEpochs(t *testing.T) {
	state := &factoryTestState{}
	creator := collectorapi.Creator{
		Create: func() collectorapi.CollectorV1 {
			module := state.module(nil, false)
			charts := collectorapi.Charts{}
			module.ChartsFunc = func() *collectorapi.Charts { return &charts }
			return module
		},
	}
	firstFactory, _ := newFactoryTestHarness(t, creator, nil)
	secondFactory, _ := newFactoryTestHarness(t, creator, nil)
	attempts, err := containment.NewAuthority(nil)
	require.NoError(t, err)
	firstFactory.config.Epoch = 1
	firstFactory.config.Attempts = attempts
	secondFactory.config.Epoch = 2
	secondFactory.config.Attempts = attempts

	first, err := firstFactory.NewCandidate(factoryTestConfig(false))
	require.NoError(t, err)
	first.Start()
	<-first.Ready()
	require.EqualValues(t, containment.Census{
		Active:   1,
		Admitted: 1,
	}, attempts.Census())

	second, err := secondFactory.NewCandidate(factoryTestConfig(false))
	require.NoError(t, err)
	require.Equal(t, first.identity.Key, second.identity.Key)
	second.Start()
	<-second.Ready()
	_, failure, err := secondFactory.PrepareCandidate(
		lifecycle.ResourceIdentity{ID: "module_job", Generation: 1},
		lifecycle.LongLivedPermit{},
		second,
	)
	require.ErrorIs(t, err, jobmgr.ErrProcessAttemptBusy)
	require.Nil(t, failure)

	released, ok := attempts.ProcessAttemptReleased(first.identity)
	require.True(t, ok)
	second.Release()
	first.Release()
	<-released
	require.EqualValues(t, containment.Census{}, attempts.Census())
}

func TestFactoryReplacementCandidateCoexistsWithIncumbentUntilRuntimePromotion(t *testing.T) {
	cleanupEntered := make(chan struct{})
	cleanupRelease := make(chan struct{})
	state := &factoryTestState{}
	created := 0
	creator := collectorapi.Creator{
		Create: func() collectorapi.CollectorV1 {
			current := created
			created++
			module := state.module(nil, false)
			charts := collectorapi.Charts{}
			module.ChartsFunc = func() *collectorapi.Charts { return &charts }
			if current == 0 {
				module.CleanupFunc = func(context.Context) {
					close(cleanupEntered)
					<-cleanupRelease
					state.collectorCleanup++
				}
			}
			return module
		},
	}
	firstFactory, _ := newFactoryTestHarness(t, creator, nil)
	secondFactory, _ := newFactoryTestHarness(t, creator, nil)
	attempts, err := containment.NewAuthority(nil)
	require.NoError(t, err)
	firstFactory.config.Epoch = 1
	firstFactory.config.Attempts = attempts
	secondFactory.config.Epoch = 2
	secondFactory.config.Attempts = attempts

	firstStage, err := firstFactory.NewCandidate(factoryTestConfig(false))
	require.NoError(t, err)
	firstStage.Start()
	<-firstStage.Ready()
	firstPermit, firstTasks := issueTestJobPermit(t, "module_job", 1)
	firstPrepared, failure, err := firstFactory.PrepareCandidate(
		lifecycle.ResourceIdentity{ID: "module_job", Generation: 1},
		firstPermit,
		firstStage,
	)
	require.NoError(t, err)
	require.Nil(t, failure)
	firstResource, err := firstPrepared.AcceptStart(context.Background(), 1)
	require.NoError(t, err)
	firstGeneration := firstResource.(*JobGeneration)
	require.NoError(t, firstGeneration.Publish())
	require.NoError(t, firstGeneration.reserveInstallation())
	require.NoError(t, firstGeneration.acknowledgeInstallation())

	secondStage, err := secondFactory.NewCandidate(factoryTestConfig(false))
	require.NoError(t, err)
	secondStage.Start()
	<-secondStage.Ready()
	secondPermit, secondTasks := issueTestJobPermit(t, "module_job", 2)
	secondPrepared, failure, err := secondFactory.PrepareCandidate(
		lifecycle.ResourceIdentity{ID: "module_job", Generation: 2},
		secondPermit,
		secondStage,
	)
	require.NoError(t, err)
	require.Nil(t, failure)
	require.EqualValues(t, containment.Census{
		Active:   2,
		Admitted: 2,
	}, attempts.Census())

	require.NoError(t, firstGeneration.Stop(context.Background()))
	require.NoError(t, firstGeneration.Finalize())
	<-cleanupEntered

	type acceptedGeneration struct {
		generation *JobGeneration
		err        error
	}
	accepted := make(chan acceptedGeneration, 1)
	go func() {
		resource, acceptErr := secondPrepared.AcceptStart(context.Background(), 2)
		generation, _ := resource.(*JobGeneration)
		accepted <- acceptedGeneration{
			generation: generation,
			err:        acceptErr,
		}
	}()
	select {
	case result := <-accepted:
		require.FailNowf(t, "test failed", "replacement did not wait for incumbent physical release: %v", result.err)
	case <-time.After(100 * time.Millisecond):
	}
	close(cleanupRelease)

	var secondGeneration *JobGeneration
	select {
	case result := <-accepted:
		require.NoError(t, result.err)
		secondGeneration = result.generation
	case <-time.After(time.Second):
		require.FailNow(t, "test failed", "replacement did not promote after incumbent physical release")
	}
	require.NoError(t, secondGeneration.Publish())
	require.NoError(t, secondGeneration.reserveInstallation())
	require.NoError(t, secondGeneration.acknowledgeInstallation())
	require.NoError(t, secondGeneration.Stop(context.Background()))
	require.NoError(t, secondGeneration.Finalize())

	firstStage.Release()
	secondStage.Release()
	require.Eventually(t, func() bool {
		return attempts.Census() == (containment.Census{})
	}, time.Second, time.Millisecond)
	require.EqualValues(t, 2, state.collectorCleanup)
	require.EqualValues(t, lifecycle.LongLivedCensus{}, firstTasks.LongLivedCensus())
	require.EqualValues(t, lifecycle.LongLivedCensus{}, secondTasks.LongLivedCensus())
}

func TestFactoryProbeUsesYieldedContextWithoutInventingDeadline(t *testing.T) {
	state := &factoryTestState{}
	probeFailure := errors.New("probe failed")
	var received context.Context
	creator := collectorapi.Creator{
		Create: func() collectorapi.CollectorV1 {
			return state.module(func(ctx context.Context) error {
				received = ctx
				return probeFailure
			}, false)
		},
	}
	factory, _ := newFactoryTestHarness(t, creator, nil)
	type contextKey struct{}
	parent := context.Background()
	yielded := context.WithValue(parent, contextKey{}, "yielded")
	factory.config.RunWithoutClaims = func(
		ctx context.Context,
		work func(context.Context) error,
	) (error, error) {
		require.Equal(t, parent, ctx)
		return work(yielded), nil
	}
	permit, tasks := issueTestJobPermit(t, "module_job", 1)
	prepared, err := factory.Prepare(
		parent,
		factoryTestConfig(false),
		lifecycle.ResourceIdentity{
			ID:         "module_job",
			Generation: 1,
		},
		permit,
	)
	require.NoError(t, err)

	failure, err := factory.Probe(parent, prepared)
	require.NoError(t, err)
	require.ErrorIs(t, failure, probeFailure)
	require.Equal(t, yielded, received)
	require.EqualValues(t, "yielded", received.Value(contextKey{}))
	_, hasDeadline := received.Deadline()
	require.False(t, hasDeadline)
	require.EqualValues(t, 1, state.collectorCleanup)
	require.NoError(t, permit.AbortUnused())
	require.EqualValues(t, lifecycle.LongLivedCensus{}, tasks.LongLivedCensus())
}

func TestFactoryProbePropagatesCallerCancellation(t *testing.T) {
	state := &factoryTestState{}
	cancellation := errors.New("caller canceled")
	creator := collectorapi.Creator{
		Create: func() collectorapi.CollectorV1 {
			return state.module(func(ctx context.Context) error {
				<-ctx.Done()
				return context.Cause(ctx)
			}, false)
		},
	}
	factory, _ := newFactoryTestHarness(t, creator, nil)
	permit, tasks := issueTestJobPermit(t, "module_job", 1)
	parent, cancel := context.WithCancelCause(context.Background())
	cancel(cancellation)
	prepared, err := factory.Prepare(
		context.Background(),
		factoryTestConfig(false),
		lifecycle.ResourceIdentity{
			ID:         "module_job",
			Generation: 1,
		},
		permit,
	)
	require.NoError(t, err)

	failure, err := factory.Probe(parent, prepared)
	require.NoError(t, err)
	require.ErrorIs(t, failure, cancellation)
	require.EqualValues(t, 1, state.collectorCleanup)
	require.NoError(t, permit.AbortUnused())
	require.EqualValues(t, lifecycle.LongLivedCensus{}, tasks.LongLivedCensus())
}

func TestFactoryProbeRejectsFailedCandidateBeforeClaimReacquisition(t *testing.T) {
	cleanupDuringYield := make(chan bool, 1)
	yielded := false
	creator := collectorapi.Creator{
		Create: func() collectorapi.CollectorV1 {
			return &collectorapi.MockCollectorV1{
				CheckFunc: func(context.Context) error {
					return errors.New("check failed")
				},
				CleanupFunc: func(context.Context) {
					cleanupDuringYield <- yielded
				},
			}
		},
	}
	factory, _ := newFactoryTestHarness(t, creator, nil)
	factory.config.RunWithoutClaims = func(
		ctx context.Context,
		work func(context.Context) error,
	) (error, error) {
		yielded = true
		workErr := work(ctx)
		yielded = false
		return workErr, nil
	}
	permit, tasks := issueTestJobPermit(t, "module_job", 1)
	prepared, err := factory.Prepare(
		context.Background(),
		factoryTestConfig(false),
		lifecycle.ResourceIdentity{
			ID:         "module_job",
			Generation: 1,
		},
		permit,
	)
	require.NoError(t, err)

	failure, err := factory.Probe(context.Background(), prepared)
	require.NoError(t, err)
	require.Error(t, failure)
	require.True(t, <-cleanupDuringYield)
	require.NoError(t, permit.AbortUnused())
	require.Equal(t, lifecycle.LongLivedCensus{}, tasks.LongLivedCensus())
}

func TestFactoryProbeRedactsResolvedValuesFromCollectorFailure(t *testing.T) {
	const resolvedFixture = "resolved-sensitive-fixture"
	for _, variant := range []string{"V1", "V2"} {
		for _, failureMode := range []string{"error", "panic"} {
			t.Run(variant+"/"+failureMode, func(t *testing.T) {
				var module *collectorapi.MockCollectorV1
				creator := collectorapi.Creator{}
				if variant == "V1" {
					creator.Create = func() collectorapi.CollectorV1 {
						module = &collectorapi.MockCollectorV1{}
						module.CheckFunc = func(context.Context) error {
							message := "check exposed " + module.Config.OptionStr
							if failureMode == "panic" {
								panic(message)
							}
							return errors.New(message)
						}
						return module
					}
				} else {
					creator.CreateV2 = func() collectorapi.CollectorV2 {
						return &factoryTestV2{
							state:          &factoryTestState{},
							exposeResolved: true,
							panicCheck:     failureMode == "panic",
						}
					}
				}
				factory, _ := newFactoryTestHarness(t, creator, nil)
				resolver, err := secretresolver.NewAtomicResolver(map[string]secretresolver.AtomicProvider{
					"fixture": secretresolver.AtomicProviderFunc(
						func(context.Context, string) ([]byte, error) {
							return []byte(resolvedFixture), nil
						},
					),
				})
				require.NoError(t, err)
				factory.config.ConfigModules.config.Resolver = resolver
				permit, tasks := issueTestJobPermit(t, "module_job", 1)
				config := factoryTestConfig(false)
				config["option_str"] = "${fixture:value}"
				config["option_int"] = 1
				prepared, err := factory.Prepare(
					context.Background(),
					config,
					lifecycle.ResourceIdentity{
						ID:         "module_job",
						Generation: 1,
					},
					permit,
				)
				require.NoError(t, err)

				failure, err := factory.Probe(context.Background(), prepared)
				require.NoError(t, err)
				require.Error(t, failure)
				require.NotContains(t, failure.Error(), resolvedFixture)
				require.Contains(t, failure.Error(), "redacted")
				require.NoError(t, permit.AbortUnused())
				require.Equal(t, lifecycle.LongLivedCensus{}, tasks.LongLivedCensus())
			})
		}
	}
}

func testRunWithoutClaims(
	ctx context.Context,
	work func(context.Context) error,
) (error, error) {
	return work(ctx), nil
}

func TestFactorySuccessfulCollectorCleanupIsExactlyOnce(t *testing.T) {
	state := &factoryTestState{}
	creator := collectorapi.Creator{
		Create: func() collectorapi.CollectorV1 {
			module := state.module(nil, false)
			charts := collectorapi.Charts{}
			module.ChartsFunc = func() *collectorapi.Charts { return &charts }
			return module
		},
	}
	factory, _ := newFactoryTestHarness(t, creator, nil)
	permit, tasks := issueTestJobPermit(t, "module_job", 1)
	prepared, err := factory.Prepare(
		context.Background(),
		factoryTestConfig(false),
		lifecycle.ResourceIdentity{
			ID:         "module_job",
			Generation: 1,
		},
		permit,
	)
	require.NoError(t, err)
	for range 2 {
		err = prepared.Dispose(context.Background())
	}
	require.Error(t, err)
	require.EqualValues(t, 1, state.collectorCleanup)
	require.EqualValues(t, lifecycle.LongLivedCensus{}, tasks.LongLivedCensus())
}

type factoryTestState struct {
	collectorCleanup int
	handlerClose     int
	autoDetection    int
}

type factoryTestV2 struct {
	collectorapi.Base
	OptionStr      string `yaml:"option_str"`
	state          *factoryTestState
	checkErr       error
	exposeResolved bool
	panicCheck     bool
	store          metrix.CollectorStore
	template       string
}

func (*factoryTestV2) Init(context.Context) error { return nil }

func (ft2 *factoryTestV2) Check(context.Context) error {
	if ft2.exposeResolved {
		message := "check exposed " + ft2.OptionStr
		if ft2.panicCheck {
			panic(message)
		}
		return errors.New(message)
	}
	return ft2.checkErr
}

func (*factoryTestV2) Collect(context.Context) error { return nil }

func (ft2 *factoryTestV2) Cleanup(context.Context) {
	ft2.state.collectorCleanup++
}

func (*factoryTestV2) Configuration() any { return struct{}{} }

func (*factoryTestV2) VirtualNode() *vnodes.VirtualNode { return nil }

func (ft2 *factoryTestV2) MetricStore() metrix.CollectorStore { return ft2.store }

func (ft2 *factoryTestV2) ChartTemplateYAML() string { return ft2.template }

type factoryTestRuntimeService struct {
	registrations int
}

func (service *factoryTestRuntimeService) RegisterComponent(runtimecomp.ComponentConfig) error {
	service.registrations++
	return nil
}

func (*factoryTestRuntimeService) UnregisterComponent(string)                  {}
func (*factoryTestRuntimeService) QuarantineComponent(string)                  {}
func (*factoryTestRuntimeService) FinalizeComponent(string)                    {}
func (*factoryTestRuntimeService) RegisterProducer(string, func() error) error { return nil }
func (*factoryTestRuntimeService) UnregisterProducer(string)                   {}

func (fts *factoryTestState) module(
	check func(context.Context) error,
	panicCleanup bool,
) *collectorapi.MockCollectorV1 {
	return &collectorapi.MockCollectorV1{
		CheckFunc: check,
		CleanupFunc: func(context.Context) {
			fts.collectorCleanup++
			if panicCleanup {
				panic("cleanup failed")
			}
		},
	}
}

type factoryTestHooks struct {
	prepare func(RuntimeJob) (StagedHandlerLifecycle, error)
}

func (fth factoryTestHooks) Stage(job RuntimeJob) (StagedHandlerLifecycle, error) {
	return fth.prepare(job)
}

func (factoryTestHooks) Attach(
	_ lifecycle.ResourceIdentity,
	staged StagedHandlerLifecycle,
) (HandlerLifecycle, error) {
	handle, ok := staged.(HandlerLifecycle)
	if !ok {
		return nil, errors.New("test staged handler is not attachable")
	}
	return handle, nil
}

type factoryTestHandlers struct {
	state *factoryTestState
}

func (*factoryTestHandlers) Publish() error { return nil }

func (fth *factoryTestHandlers) CloseAndDrain(context.Context) error {
	fth.state.handlerClose++
	return nil
}

func newFactoryTestHarness(t *testing.T, creator collectorapi.Creator, hooks JobHooks) (*Factory, *bytes.Buffer) {
	t.Helper()
	output := &bytes.Buffer{}
	frames, err := lifecycle.NewFrameOwner(output)
	require.NoError(t, err)
	tasks, err := lifecycle.NewTaskSupervisor(frames)
	require.NoError(t, err)
	resolver, err := secretresolver.NewAtomicResolver(nil)
	require.NoError(t, err)
	configModules, err := NewConfigModuleFactory(ConfigModuleFactoryConfig{
		Modules: collectorapi.Registry{
			"module": creator,
		},
		Resolver:   resolver,
		StoreScope: unavailableStoreScope,
	})
	require.NoError(t, err)
	factory, err := NewFactory(FactoryConfig{
		PluginName: "test",
		Modules: collectorapi.Registry{
			"module": creator,
		},
		Tasks:            tasks,
		Frames:           frames,
		ConfigModules:    configModules,
		Vnodes:           vnoderegistry.New(),
		HandlerStager:    hooks,
		HandlerAttacher:  hooks,
		Scheduler:        newTestScheduler(t),
		RunWithoutClaims: testRunWithoutClaims,
	})
	require.NoError(t, err)
	return factory, output
}

func unavailableStoreScope([]string) (secretresolver.AtomicScope, error) {
	return nil, errors.New("test Store scope is unavailable")
}

func factoryTestConfig(functionOnly bool) confgroup.Config {
	return confgroup.Config{
		"module":        "module",
		"name":          "job",
		"update_every":  1,
		"function_only": functionOnly,
	}
}
