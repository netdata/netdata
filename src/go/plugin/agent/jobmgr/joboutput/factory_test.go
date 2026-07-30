// SPDX-License-Identifier: GPL-3.0-or-later

package joboutput

import (
	"bytes"
	"context"
	"errors"
	"sync"
	"sync/atomic"
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

func TestFactoryStagesFunctionsAfterManagedProbe(t *testing.T) {
	tests := map[string]func(*factoryTestState) collectorapi.Creator{
		"V1": func(state *factoryTestState) collectorapi.Creator {
			return collectorapi.Creator{
				Create: func() collectorapi.CollectorV1 {
					module := state.module(func(context.Context) error {
						state.events = append(state.events, "check")
						return nil
					}, false)
					module.InitFunc = func(context.Context) error {
						state.events = append(state.events, "init")
						return nil
					}
					charts := collectorapi.Charts{}
					module.ChartsFunc = func() *collectorapi.Charts { return &charts }
					return module
				},
				SharedFunctions: func() []funcapi.FunctionConfig {
					return []funcapi.FunctionConfig{{ID: "method"}}
				},
			}
		},
		"V2": func(state *factoryTestState) collectorapi.Creator {
			return collectorapi.Creator{
				CreateV2: func() collectorapi.CollectorV2 {
					return &factoryTestV2{
						state: state,
						init: func(context.Context) error {
							state.events = append(state.events, "init")
							return nil
						},
						check: func(context.Context) error {
							state.events = append(state.events, "check")
							return nil
						},
						store:    metrix.NewCollectorStore(),
						template: factoryTestChartTemplate,
					}
				},
				SharedFunctions: func() []funcapi.FunctionConfig {
					return []funcapi.FunctionConfig{{ID: "method"}}
				},
			}
		},
	}
	for name, create := range tests {
		t.Run(name, func(t *testing.T) {
			state := &factoryTestState{}
			hooks := factoryTestHooks{
				prepare: func(RuntimeJob) (StagedHandlerLifecycle, error) {
					state.events = append(state.events, "stage")
					return &factoryTestHandlers{state: state}, nil
				},
			}
			factory, _ := newFactoryTestHarness(t, create(state), hooks)
			permit, tasks := issueTestJobPermit(t, "module_job", 1)

			prepared, failure, err := prepareFactoryTestCandidate(
				context.Background(),
				factory,
				factoryTestConfig(false),
				lifecycle.ResourceIdentity{
					ID:         "module_job",
					Generation: 1,
				},
				permit,
			)
			require.NoError(t, err)
			require.Nil(t, failure)
			require.Equal(t, []string{"init", "check", "stage"}, state.events)

			require.NoError(t, prepared.Dispose(context.Background()))
			requireFactoryAttemptsIdle(t, factory)
			require.EqualValues(t, 1, state.handlerClose)
			require.EqualValues(t, 1, state.collectorCleanup)
			require.EqualValues(t, lifecycle.LongLivedCensus{}, tasks.LongLivedCensus())
		})
	}
}

func TestFactoryDoesNotStageFunctionsAfterFailedManagedProbe(t *testing.T) {
	probeErr := errors.New("probe failed")
	tests := map[string]struct {
		create     func(*factoryTestState) collectorapi.Creator
		wantEvents []string
	}{
		"V1 init": {
			create: func(state *factoryTestState) collectorapi.Creator {
				return collectorapi.Creator{
					Create: func() collectorapi.CollectorV1 {
						module := state.module(func(context.Context) error {
							state.events = append(state.events, "check")
							return nil
						}, false)
						module.InitFunc = func(context.Context) error {
							state.events = append(state.events, "init")
							return probeErr
						}
						return module
					},
					SharedFunctions: func() []funcapi.FunctionConfig {
						return []funcapi.FunctionConfig{{ID: "method"}}
					},
				}
			},
			wantEvents: []string{"init"},
		},
		"V1 check": {
			create: func(state *factoryTestState) collectorapi.Creator {
				return collectorapi.Creator{
					Create: func() collectorapi.CollectorV1 {
						module := state.module(func(context.Context) error {
							state.events = append(state.events, "check")
							return probeErr
						}, false)
						module.InitFunc = func(context.Context) error {
							state.events = append(state.events, "init")
							return nil
						}
						return module
					},
					SharedFunctions: func() []funcapi.FunctionConfig {
						return []funcapi.FunctionConfig{{ID: "method"}}
					},
				}
			},
			wantEvents: []string{"init", "check"},
		},
		"V2 init": {
			create: func(state *factoryTestState) collectorapi.Creator {
				return collectorapi.Creator{
					CreateV2: func() collectorapi.CollectorV2 {
						return &factoryTestV2{
							state: state,
							init: func(context.Context) error {
								state.events = append(state.events, "init")
								return probeErr
							},
						}
					},
					SharedFunctions: func() []funcapi.FunctionConfig {
						return []funcapi.FunctionConfig{{ID: "method"}}
					},
				}
			},
			wantEvents: []string{"init"},
		},
		"V2 check": {
			create: func(state *factoryTestState) collectorapi.Creator {
				return collectorapi.Creator{
					CreateV2: func() collectorapi.CollectorV2 {
						return &factoryTestV2{
							state: state,
							init: func(context.Context) error {
								state.events = append(state.events, "init")
								return nil
							},
							check: func(context.Context) error {
								state.events = append(state.events, "check")
								return probeErr
							},
						}
					},
					SharedFunctions: func() []funcapi.FunctionConfig {
						return []funcapi.FunctionConfig{{ID: "method"}}
					},
				}
			},
			wantEvents: []string{"init", "check"},
		},
	}
	for name, test := range tests {
		t.Run(name, func(t *testing.T) {
			state := &factoryTestState{}
			hooks := factoryTestHooks{
				prepare: func(RuntimeJob) (StagedHandlerLifecycle, error) {
					state.events = append(state.events, "stage")
					return &factoryTestHandlers{state: state}, nil
				},
			}
			factory, _ := newFactoryTestHarness(t, test.create(state), hooks)
			permit, tasks := issueTestJobPermit(t, "module_job", 1)

			prepared, failure, err := prepareFactoryTestCandidate(
				context.Background(),
				factory,
				factoryTestConfig(false),
				lifecycle.ResourceIdentity{
					ID:         "module_job",
					Generation: 1,
				},
				permit,
			)
			require.NoError(t, err)
			require.False(t, prepared.Valid())
			require.ErrorIs(t, failure, probeErr)
			require.Equal(t, test.wantEvents, state.events)

			require.NoError(t, permit.AbortUnused())
			requireFactoryAttemptsIdle(t, factory)
			require.Zero(t, state.handlerClose)
			require.EqualValues(t, 1, state.collectorCleanup)
			require.EqualValues(t, lifecycle.LongLivedCensus{}, tasks.LongLivedCensus())
		})
	}
}

func TestFactoryRejectsCandidateWhenResolvedStoreGenerationChangesDuringProbe(t *testing.T) {
	scope := &factoryTestAtomicScope{
		value: "initial",
	}
	state := &factoryTestState{}
	creator := collectorapi.Creator{
		CreateV2: func() collectorapi.CollectorV2 {
			return &factoryTestV2{
				state: state,
				check: func(context.Context) error {
					scope.current.Store(false)
					return nil
				},
				store:    metrix.NewCollectorStore(),
				template: factoryTestChartTemplate,
			}
		},
	}
	factory, _ := newFactoryTestHarness(t, creator, nil)
	resolver, err := secretresolver.NewAtomicResolver(nil)
	require.NoError(t, err)
	configModules, err := NewConfigModuleFactory(ConfigModuleFactoryConfig{
		Modules: collectorapi.Registry{
			"module": creator,
		},
		Resolver: resolver,
		StoreScope: func([]string) (secretresolver.AtomicScope, error) {
			scope.current.Store(true)
			return scope, nil
		},
	})
	require.NoError(t, err)
	factory.config.ConfigModules = configModules
	config := factoryTestConfig(false)
	config["option_str"] = "${store:vault:test:key}"
	permit, tasks := issueTestJobPermit(t, "module_job", 1)

	prepared, failure, err := prepareFactoryTestCandidate(
		context.Background(),
		factory,
		config,
		lifecycle.ResourceIdentity{
			ID:         "module_job",
			Generation: 1,
		},
		permit,
	)
	require.Error(t, err)
	require.False(t, prepared.Valid())
	require.Nil(t, failure)
	require.NoError(t, permit.AbortUnused())
	requireFactoryAttemptsIdle(t, factory)
	require.EqualValues(t, 1, state.collectorCleanup)
	require.EqualValues(t, lifecycle.LongLivedCensus{}, tasks.LongLivedCensus())
}

func TestFactoryCleanHandlerStagingFailureRejectsOnlyCandidate(t *testing.T) {
	state := &factoryTestState{}
	stageErr := errors.New("handler staging failed")
	creator := collectorapi.Creator{
		Create: func() collectorapi.CollectorV1 {
			module := state.module(nil, false)
			charts := collectorapi.Charts{}
			module.ChartsFunc = func() *collectorapi.Charts { return &charts }
			return module
		},
		SharedFunctions: func() []funcapi.FunctionConfig {
			return []funcapi.FunctionConfig{{ID: "method"}}
		},
	}
	hooks := factoryTestHooks{
		prepare: func(RuntimeJob) (StagedHandlerLifecycle, error) {
			return nil, stageErr
		},
	}
	factory, _ := newFactoryTestHarness(t, creator, hooks)
	permit, tasks := issueTestJobPermit(t, "module_job", 1)

	prepared, failure, err := prepareFactoryTestCandidate(
		context.Background(),
		factory,
		factoryTestConfig(false),
		lifecycle.ResourceIdentity{
			ID:         "module_job",
			Generation: 1,
		},
		permit,
	)
	require.NoError(t, err)
	require.False(t, prepared.Valid())
	require.ErrorIs(t, failure, stageErr)
	require.False(t, lifecycle.OwnershipRetained(failure))
	require.NoError(t, permit.AbortUnused())
	requireFactoryAttemptsIdle(t, factory)
	require.EqualValues(t, 1, state.collectorCleanup)
	require.EqualValues(t, lifecycle.LongLivedCensus{}, tasks.LongLivedCensus())
}

func TestFactoryCandidateCleanupPanicQuarantinesIdentity(t *testing.T) {
	state := &factoryTestState{}
	creator := collectorapi.Creator{
		Create: func() collectorapi.CollectorV1 {
			return state.module(func(context.Context) error {
				return errors.New("check failed")
			}, true)
		},
	}
	factory, _ := newFactoryTestHarness(t, creator, nil)
	permit, tasks := issueTestJobPermit(t, "module_job", 1)

	prepared, failure, err := prepareFactoryTestCandidate(
		context.Background(),
		factory,
		factoryTestConfig(false),
		lifecycle.ResourceIdentity{
			ID:         "module_job",
			Generation: 1,
		},
		permit,
	)
	require.False(t, prepared.Valid())
	require.Nil(t, failure)
	require.ErrorIs(t, err, jobmgr.ErrProcessAttemptQuarantined)
	require.False(t, lifecycle.OwnershipRetained(err))
	require.NoError(t, permit.AbortUnused())

	attempts, ok := factory.config.Attempts.(*containment.Authority)
	require.True(t, ok)
	require.Equal(t, containment.Census{Quarantined: 1}, attempts.Census())
	stage, err := factory.newCandidate(factoryTestConfig(false))
	require.NoError(t, err)
	defer stage.Release()
	stage.Start()
	<-stage.Ready()
	_, _, err = factory.prepareCandidate(
		lifecycle.ResourceIdentity{ID: "module_job", Generation: 2},
		lifecycle.LongLivedPermit{},
		stage,
	)
	require.ErrorIs(t, err, jobmgr.ErrProcessAttemptQuarantined)
	require.EqualValues(t, 1, state.collectorCleanup)
	require.EqualValues(t, lifecycle.LongLivedCensus{}, tasks.LongLivedCensus())
}

func TestFactoryCanonicalizesTypedNilLifecycleResults(t *testing.T) {
	stageErr := errors.New("stage failed")
	attachErr := errors.New("attach failed")
	hooks := factoryTypedNilLifecycleHooks{
		stageErr:  stageErr,
		attachErr: attachErr,
	}

	staged, err := callStageHandlers(hooks, nil)
	require.ErrorIs(t, err, stageErr)
	require.True(t, staged == nil)

	attached, err := callAttachHandlers(
		hooks,
		lifecycle.ResourceIdentity{ID: "module_job", Generation: 1},
		&factoryTestHandlers{state: &factoryTestState{}},
	)
	require.ErrorIs(t, err, attachErr)
	require.True(t, attached == nil)
}

func TestNewFactoryRejectsMismatchedHandlerLifecycleDependencies(t *testing.T) {
	hooks := factoryTestHooks{}
	factory, _ := newFactoryTestHarness(t, collectorapi.Creator{}, hooks)
	tests := map[string]FactoryConfig{
		"stager only":   factory.config,
		"attacher only": factory.config,
	}
	stagerOnly := tests["stager only"]
	stagerOnly.HandlerAttacher = nil
	tests["stager only"] = stagerOnly
	attacherOnly := tests["attacher only"]
	attacherOnly.HandlerStager = nil
	tests["attacher only"] = attacherOnly

	for name, config := range tests {
		t.Run(name, func(t *testing.T) {
			_, err := NewFactory(config)
			require.ErrorContains(t, err, "incomplete factory configuration")
		})
	}
}

func TestFactoryRejectsWithExactlyOneCollectorCleanup(t *testing.T) {
	tests := map[string]struct {
		configure       func(*factoryTestState, *collectorapi.Creator) factoryTestJobHooks
		wantClose       int
		wantQuarantined bool
		wantNoCleanup   bool
	}{
		"creator panic": {
			configure: func(*factoryTestState, *collectorapi.Creator) factoryTestJobHooks {
				return nil
			},
			wantQuarantined: true,
			wantNoCleanup:   true,
		},
		"autodetection failure": {
			configure: func(state *factoryTestState, creator *collectorapi.Creator) factoryTestJobHooks {
				creator.Create = func() collectorapi.CollectorV1 {
					return state.module(func(context.Context) error {
						return errors.New("check failed")
					}, false)
				}
				return nil
			},
		},
		"autodetection panic": {
			configure: func(state *factoryTestState, creator *collectorapi.Creator) factoryTestJobHooks {
				creator.Create = func() collectorapi.CollectorV1 {
					return state.module(func(context.Context) error {
						panic("check failed")
					}, false)
				}
				return nil
			},
		},
		"collector cleanup panic": {
			configure: func(state *factoryTestState, creator *collectorapi.Creator) factoryTestJobHooks {
				creator.Create = func() collectorapi.CollectorV1 {
					return state.module(func(context.Context) error {
						return errors.New("check failed")
					}, true)
				}
				return nil
			},
			wantQuarantined: true,
		},
		"function-bearing job without hooks": {
			configure: func(state *factoryTestState, creator *collectorapi.Creator) factoryTestJobHooks {
				creator.FunctionOnly = true
				creator.SharedFunctions = func() []funcapi.FunctionConfig { return nil }
				creator.Create = func() collectorapi.CollectorV1 {
					return state.module(nil, false)
				}
				return nil
			},
		},
		"partial handler preparation failure": {
			configure: func(state *factoryTestState, creator *collectorapi.Creator) factoryTestJobHooks {
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
			configure: func(state *factoryTestState, creator *collectorapi.Creator) factoryTestJobHooks {
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
			wantQuarantined: true,
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
			prepared, failure, err := prepareFactoryTestCandidate(
				context.Background(),
				factory,
				factoryTestConfig(creator.FunctionOnly),
				lifecycle.ResourceIdentity{
					ID:         "module_job",
					Generation: 1,
				},
				permit,
			)
			if err == nil {
				if failure != nil {
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
			require.False(t, lifecycle.OwnershipRetained(err))
			require.Equal(t, test.wantQuarantined, errors.Is(err, jobmgr.ErrProcessAttemptQuarantined))
			attempts, ok := factory.config.Attempts.(*containment.Authority)
			require.True(t, ok)
			wantCensus := containment.Census{}
			if test.wantQuarantined {
				wantCensus.Quarantined = 1
			}
			require.Equal(t, wantCensus, attempts.Census())
			require.EqualValues(t, lifecycle.LongLivedCensus{}, tasks.LongLivedCensus())
		})
	}
}

func TestFactoryV2RejectsWithExactlyOneCollectorCleanup(t *testing.T) {
	tests := map[string]struct {
		functionOnly bool
		checkErr     error
		hooks        factoryTestJobHooks
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

			prepared, failure, err := prepareFactoryTestCandidate(
				context.Background(),
				factory,
				factoryTestConfig(test.functionOnly),
				lifecycle.ResourceIdentity{
					ID:         "module_job",
					Generation: 1,
				},
				permit,
			)
			if err == nil {
				if failure != nil {
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

func TestFactoryCandidateProbesAndRejectsBeforeAttachment(t *testing.T) {
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

	prepared, failure, err := prepareFactoryTestCandidate(
		context.Background(),
		factory,
		factoryTestConfig(false),
		lifecycle.ResourceIdentity{
			ID:         "module_job",
			Generation: 1,
		},
		permit,
	)
	require.NoError(t, err)
	require.False(t, prepared.Valid())
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
	permit, _ := issueTestJobPermit(t, "module_job", 1)

	prepared, failure, err := prepareFactoryTestCandidate(
		context.Background(),
		factory,
		factoryTestConfig(false),
		lifecycle.ResourceIdentity{
			ID:         "module_job",
			Generation: 1,
		},
		permit,
	)
	require.NoError(t, err)
	require.Nil(t, failure)
	// A candidate must leave the run-owned external facet reserved.
	require.NoError(t, permit.ActivateExternal())
	require.NoError(t, permit.ReleaseExternal())
	require.NoError(t, permit.Return())
	require.NoError(t, prepared.reject(context.Background()))
	requireFactoryAttemptsIdle(t, factory)

	permit, tasks := issueTestJobPermit(t, "module_job", 2)
	prepared, failure, err = prepareFactoryTestCandidate(
		context.Background(),
		factory,
		factoryTestConfig(false),
		lifecycle.ResourceIdentity{
			ID:         "module_job",
			Generation: 2,
		},
		permit,
	)
	require.NoError(t, err)
	require.Nil(t, failure)
	require.Zero(t, runtime.registrations)
	require.Zero(t, tasks.Active())

	require.NoError(t, prepared.Dispose(context.Background()))
	requireFactoryAttemptsIdle(t, factory)
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

	stage, err := factory.newCandidate(factoryTestConfig(false))
	require.NoError(t, err)
	stage.Start()
	<-stage.Ready()
	require.Zero(t, runtime.registrations)

	permit, tasks := issueTestJobPermit(t, "module_job", 1)
	prepared, failure, err := factory.prepareCandidate(
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
	requireFactoryAttemptsIdle(t, factory)
	require.EqualValues(t, lifecycle.LongLivedCensus{}, tasks.LongLivedCensus())
}

func TestFactoryCandidateWaitYieldsGraphClaimForWholeMaterialization(t *testing.T) {
	var outsideClaims atomic.Bool
	var callbacks atomic.Int32
	var violations atomic.Int32
	state := &factoryTestState{}
	creator := collectorapi.Creator{
		Create: func() collectorapi.CollectorV1 {
			callbacks.Add(1)
			if !outsideClaims.Load() {
				violations.Add(1)
			}
			module := state.module(func(context.Context) error {
				callbacks.Add(1)
				if !outsideClaims.Load() {
					violations.Add(1)
				}
				return nil
			}, false)
			charts := collectorapi.Charts{}
			module.ChartsFunc = func() *collectorapi.Charts { return &charts }
			return module
		},
	}
	factory, _ := newFactoryTestHarness(t, creator, nil)
	attempts, err := containment.NewAuthority(nil)
	require.NoError(t, err)
	factory.config.Epoch = 1
	factory.config.Attempts = attempts
	factory.runWithoutClaims = func(
		ctx context.Context,
		work func(context.Context) error,
	) (error, error) {
		outsideClaims.Store(true)
		defer outsideClaims.Store(false)
		return work(ctx), nil
	}

	stage, err := factory.newCandidate(factoryTestConfig(false))
	require.NoError(t, err)
	require.NoError(t, factory.awaitCandidate(context.Background(), stage))
	require.EqualValues(t, 2, callbacks.Load())
	require.Zero(t, violations.Load())

	permit, tasks := issueTestJobPermit(t, "module_job", 1)
	prepared, failure, err := factory.prepareCandidate(
		lifecycle.ResourceIdentity{ID: "module_job", Generation: 1},
		permit,
		stage,
	)
	require.NoError(t, err)
	require.Nil(t, failure)
	stage.Release()
	resource, err := prepared.AcceptStart(context.Background(), 1)
	require.NoError(t, err)
	generation := resource.(*JobGeneration)
	require.NoError(t, generation.Publish())
	require.NoError(t, generation.reserveInstallation())
	require.NoError(t, generation.acknowledgeInstallation())
	require.NoError(t, generation.Stop(context.Background()))
	require.NoError(t, generation.Finalize())
	require.Eventually(t, func() bool {
		return attempts.Census() == (containment.Census{})
	}, time.Second, time.Millisecond)
	require.EqualValues(t, 1, state.collectorCleanup)
	require.EqualValues(t, lifecycle.LongLivedCensus{}, tasks.LongLivedCensus())
}

func TestCandidateWaitDoesNotStartAfterYieldedContextCancellation(t *testing.T) {
	factory, _ := newFactoryTestHarness(t, collectorapi.Creator{}, nil)
	attempts := &unexpectedPendingJobAuthority{}
	factory.config.Epoch = 1
	factory.config.Attempts = attempts
	factory.runWithoutClaims = func(
		ctx context.Context,
		work func(context.Context) error,
	) (error, error) {
		return work(ctx), nil
	}
	stage, err := factory.newCandidate(factoryTestConfig(false))
	require.NoError(t, err)
	t.Cleanup(stage.Release)
	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	err = factory.awaitCandidate(ctx, stage)

	require.ErrorIs(t, err, context.Canceled)
	stage.mu.Lock()
	started := stage.started
	stage.mu.Unlock()
	require.False(t, started)
	require.Zero(t, attempts.calls.Load())
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

	stage, err := factory.newCandidate(factoryTestConfig(false))
	require.NoError(t, err)
	stage.Start()
	<-entered
	stage.Cancel(jobmgr.ErrProcessAttemptSuperseded)
	<-stage.Ready()

	_, failure, err := factory.prepareCandidate(
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

func TestFactoryCandidateRejectsFailureReturnedAfterLogicalCut(t *testing.T) {
	entered := make(chan struct{})
	release := make(chan struct{})
	state := &factoryTestState{}
	creator := collectorapi.Creator{
		Create: func() collectorapi.CollectorV1 {
			return state.module(func(context.Context) error {
				close(entered)
				<-release
				return errors.New("test: late probe failure")
			}, false)
		},
	}
	factory, _ := newFactoryTestHarness(t, creator, nil)
	factory.config.Attempts = &delayedDispositionAuthority{}

	stage, err := factory.newCandidate(factoryTestConfig(false))
	require.NoError(t, err)
	stage.Start()
	<-entered
	require.Eventually(t, func() bool {
		stage.mu.Lock()
		defer stage.mu.Unlock()
		return stage.attempt != nil
	}, time.Second, time.Millisecond)

	stage.Cancel(jobmgr.ErrProcessAttemptSuperseded)
	close(release)
	<-stage.Ready()
	prepared, failure, err := factory.prepareCandidate(
		lifecycle.ResourceIdentity{
			ID:         "module_job",
			Generation: 1,
		},
		lifecycle.LongLivedPermit{},
		stage,
	)
	require.False(t, prepared.Valid())
	require.Nil(t, failure)
	require.ErrorIs(t, err, jobmgr.ErrProcessAttemptSuperseded)
	stage.Release()
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

	first, err := firstFactory.newCandidate(factoryTestConfig(false))
	require.NoError(t, err)
	first.Start()
	<-first.Ready()
	require.EqualValues(t, containment.Census{
		Active:   1,
		Admitted: 1,
	}, attempts.Census())

	secondFactory.config.Attempts = busySupersessionAuthority{ProcessAttemptAuthority: attempts}
	second, err := secondFactory.newCandidate(factoryTestConfig(false))
	require.NoError(t, err)
	require.Equal(t, first.identity.Key, second.identity.Key)
	second.Start()
	<-second.Ready()
	_, failure, err := secondFactory.prepareCandidate(
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

	firstStage, err := firstFactory.newCandidate(factoryTestConfig(false))
	require.NoError(t, err)
	firstStage.Start()
	<-firstStage.Ready()
	firstPermit, firstTasks := issueTestJobPermit(t, "module_job", 1)
	firstPrepared, failure, err := firstFactory.prepareCandidate(
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

	secondStage, err := secondFactory.newCandidate(factoryTestConfig(false))
	require.NoError(t, err)
	secondStage.Start()
	<-secondStage.Ready()
	secondPermit, secondTasks := issueTestJobPermit(t, "module_job", 2)
	secondPrepared, failure, err := secondFactory.prepareCandidate(
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

func TestFactoryCandidateUsesAuthorityContextWithoutInventingDeadline(t *testing.T) {
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
	factory.runWithoutClaims = func(
		ctx context.Context,
		work func(context.Context) error,
	) (error, error) {
		require.Equal(t, parent, ctx)
		return work(yielded), nil
	}
	permit, tasks := issueTestJobPermit(t, "module_job", 1)
	prepared, failure, err := prepareFactoryTestCandidate(
		parent,
		factory,
		factoryTestConfig(false),
		lifecycle.ResourceIdentity{
			ID:         "module_job",
			Generation: 1,
		},
		permit,
	)
	require.NoError(t, err)
	require.False(t, prepared.Valid())
	require.ErrorIs(t, failure, probeFailure)
	require.NotNil(t, received)
	require.NotEqual(t, yielded, received)
	_, hasDeadline := received.Deadline()
	require.False(t, hasDeadline)
	require.EqualValues(t, 1, state.collectorCleanup)
	require.NoError(t, permit.AbortUnused())
	require.EqualValues(t, lifecycle.LongLivedCensus{}, tasks.LongLivedCensus())
}

func TestFactoryCandidatePropagatesCallerCancellation(t *testing.T) {
	state := &factoryTestState{}
	cancellation := errors.New("caller canceled")
	entered := make(chan struct{})
	creator := collectorapi.Creator{
		Create: func() collectorapi.CollectorV1 {
			return state.module(func(ctx context.Context) error {
				close(entered)
				<-ctx.Done()
				return context.Cause(ctx)
			}, false)
		},
	}
	factory, _ := newFactoryTestHarness(t, creator, nil)
	permit, tasks := issueTestJobPermit(t, "module_job", 1)
	parent, cancel := context.WithCancelCause(context.Background())
	type candidateResult struct {
		prepared PreparedJob
		failure  *autoDetectionFailure
		err      error
	}
	result := make(chan candidateResult, 1)
	go func() {
		prepared, failure, err := prepareFactoryTestCandidate(
			parent,
			factory,
			factoryTestConfig(false),
			lifecycle.ResourceIdentity{
				ID:         "module_job",
				Generation: 1,
			},
			permit,
		)
		result <- candidateResult{prepared: prepared, failure: failure, err: err}
	}()
	<-entered
	cancel(cancellation)
	got := <-result
	require.False(t, got.prepared.Valid())
	require.Nil(t, got.failure)
	require.ErrorIs(t, got.err, cancellation)
	require.Eventually(t, func() bool {
		return factory.config.Attempts.(*containment.Authority).Census() == (containment.Census{})
	}, time.Second, time.Millisecond)
	require.EqualValues(t, 1, state.collectorCleanup)
	require.NoError(t, permit.AbortUnused())
	require.EqualValues(t, lifecycle.LongLivedCensus{}, tasks.LongLivedCensus())
}

func TestFactoryCandidateRejectsFailureBeforeClaimReacquisition(t *testing.T) {
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
	factory.runWithoutClaims = func(
		ctx context.Context,
		work func(context.Context) error,
	) (error, error) {
		yielded = true
		workErr := work(ctx)
		yielded = false
		return workErr, nil
	}
	permit, tasks := issueTestJobPermit(t, "module_job", 1)
	prepared, failure, err := prepareFactoryTestCandidate(
		context.Background(),
		factory,
		factoryTestConfig(false),
		lifecycle.ResourceIdentity{
			ID:         "module_job",
			Generation: 1,
		},
		permit,
	)
	require.NoError(t, err)
	require.False(t, prepared.Valid())
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
				prepared, failure, err := prepareFactoryTestCandidate(
					context.Background(),
					factory,
					config,
					lifecycle.ResourceIdentity{
						ID:         "module_job",
						Generation: 1,
					},
					permit,
				)
				require.NoError(t, err)
				require.False(t, prepared.Valid())
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
	tests := map[string]func(*factoryTestState) collectorapi.Creator{
		"V1": func(state *factoryTestState) collectorapi.Creator {
			return collectorapi.Creator{
				Create: func() collectorapi.CollectorV1 {
					module := state.module(nil, false)
					charts := collectorapi.Charts{
						&collectorapi.Chart{
							ID:    "work",
							Title: "Work",
							Units: "units",
							Dims:  collectorapi.Dims{{ID: "value"}},
						},
					}
					module.ChartsFunc = func() *collectorapi.Charts { return &charts }
					module.CollectFunc = func(context.Context) map[string]int64 {
						return map[string]int64{"value": 1}
					}
					return module
				},
			}
		},
		"V2": func(state *factoryTestState) collectorapi.Creator {
			store := metrix.NewCollectorStore()
			return collectorapi.Creator{
				CreateV2: func() collectorapi.CollectorV2 {
					return &factoryTestV2{
						state:    state,
						store:    store,
						template: factoryTestChartTemplate,
						collect: func(context.Context) error {
							store.Write().SnapshotMeter("factory").Gauge("value").Observe(1)
							return nil
						},
					}
				},
			}
		},
	}
	for name, newCreator := range tests {
		t.Run(name, func(t *testing.T) {
			state := &factoryTestState{}
			factory, output := newFactoryTestHarness(t, newCreator(state), nil)
			permit, tasks := issueTestJobPermit(t, "module_job", 1)
			prepared, failure, err := prepareFactoryTestCandidate(
				context.Background(),
				factory,
				factoryTestConfig(false),
				lifecycle.ResourceIdentity{
					ID:         "module_job",
					Generation: 1,
				},
				permit,
			)
			require.NoError(t, err)
			require.Nil(t, failure)
			resource, err := prepared.AcceptStart(context.Background(), 1)
			require.NoError(t, err)
			generation := resource.(*JobGeneration)
			require.NoError(t, generation.Publish())
			require.NoError(t, generation.reserveInstallation())
			require.NoError(t, generation.acknowledgeInstallation())

			clock := 0
			require.Eventually(t, func() bool {
				clock++
				generation.resources.candidateJob.Tick(clock)
				return bytes.Contains(output.Bytes(), []byte("CHART"))
			}, 2*time.Second, 10*time.Millisecond)

			require.NoError(t, generation.Stop(context.Background()))
			require.NoError(t, generation.Finalize())
			requireFactoryAttemptsIdle(t, factory)
			require.EqualValues(t, 1, state.collectorCleanup)
			require.Contains(t, output.String(), "obsolete")
			require.EqualValues(t, lifecycle.LongLivedCensus{}, tasks.LongLivedCensus())
		})
	}
}

type factoryTestState struct {
	collectorCleanup int
	handlerClose     int
	autoDetection    int
	events           []string
}

type factoryTestV2 struct {
	collectorapi.Base
	OptionStr      string `yaml:"option_str"`
	state          *factoryTestState
	init           func(context.Context) error
	check          func(context.Context) error
	checkErr       error
	collect        func(context.Context) error
	exposeResolved bool
	panicCheck     bool
	store          metrix.CollectorStore
	template       string
}

func (ft2 *factoryTestV2) Init(ctx context.Context) error {
	if ft2.init != nil {
		return ft2.init(ctx)
	}
	return nil
}

func (ft2 *factoryTestV2) Check(ctx context.Context) error {
	if ft2.check != nil {
		return ft2.check(ctx)
	}
	if ft2.exposeResolved {
		message := "check exposed " + ft2.OptionStr
		if ft2.panicCheck {
			panic(message)
		}
		return errors.New(message)
	}
	return ft2.checkErr
}

func (ft2 *factoryTestV2) Collect(ctx context.Context) error {
	if ft2.collect != nil {
		return ft2.collect(ctx)
	}
	return nil
}

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

func (ftrs *factoryTestRuntimeService) RegisterComponent(runtimecomp.ComponentConfig) error {
	ftrs.registrations++
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
) (ProcessHandlerLifecycle, error) {
	handle, ok := staged.(ProcessHandlerLifecycle)
	if !ok {
		return nil, errors.New("test staged handler is not attachable")
	}
	return handle, nil
}

type factoryTypedNilLifecycleHooks struct {
	stageErr  error
	attachErr error
}

func (hooks factoryTypedNilLifecycleHooks) Stage(RuntimeJob) (StagedHandlerLifecycle, error) {
	var handle *factoryTestHandlers
	return handle, hooks.stageErr
}

func (hooks factoryTypedNilLifecycleHooks) Attach(
	lifecycle.ResourceIdentity,
	StagedHandlerLifecycle,
) (ProcessHandlerLifecycle, error) {
	var handle *factoryTestHandlers
	return handle, hooks.attachErr
}

type factoryTestHandlers struct {
	state *factoryTestState
}

func (*factoryTestHandlers) Publish() error { return nil }

func (fth *factoryTestHandlers) CloseAndDrain(context.Context) error {
	return fth.Finalize(context.Background())
}

func (*factoryTestHandlers) Detach(context.Context) error {
	return nil
}

func (fth *factoryTestHandlers) Finalize(context.Context) error {
	fth.state.handlerClose++
	return nil
}

type factoryTestJobHooks interface {
	JobHandlerStager
	JobHandlerAttacher
}

type busySupersessionAuthority struct {
	jobmgr.ProcessAttemptAuthority
}

func (busySupersessionAuthority) SupersedeProcessAttempt(
	context.Context,
	jobmgr.ProcessAttemptIdentity,
) error {
	return jobmgr.ErrProcessAttemptBusy
}

func newFactoryTestHarness(
	t *testing.T,
	creator collectorapi.Creator,
	hooks factoryTestJobHooks,
) (*Factory, *factoryTestOutput) {
	t.Helper()
	output := &factoryTestOutput{}
	frames, err := lifecycle.NewFrameOwner(output)
	require.NoError(t, err)
	cleanupOutput, err := NewCleanupOutputGate(frames)
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
	attempts, err := containment.NewAuthority(nil)
	require.NoError(t, err)
	t.Cleanup(func() {
		ctx, cancel := context.WithTimeout(context.Background(), time.Second)
		defer cancel()
		require.NoError(t, attempts.Shutdown(ctx))
	})
	factory, err := NewFactory(FactoryConfig{
		PluginName: "test",
		Epoch:      1,
		Attempts:   attempts,
		Modules: collectorapi.Registry{
			"module": creator,
		},
		Frames:          frames,
		CleanupOutput:   cleanupOutput,
		ConfigModules:   configModules,
		Vnodes:          vnoderegistry.New(),
		HandlerStager:   hooks,
		HandlerAttacher: hooks,
		Scheduler:       newTestScheduler(t),
	})
	require.NoError(t, err)
	factory.runWithoutClaims = testRunWithoutClaims
	return factory, output
}

type factoryTestOutput struct {
	mu sync.Mutex
	bytes.Buffer
}

type factoryTestAtomicScope struct {
	current atomic.Bool
	value   string
}

func (scope *factoryTestAtomicScope) Resolve(
	context.Context,
	string,
	string,
) ([]byte, error) {
	return []byte(scope.value), nil
}

func (*factoryTestAtomicScope) Release(context.Context) error {
	return nil
}

func (scope *factoryTestAtomicScope) Snapshot() secretresolver.AtomicScopeSnapshot {
	return scope
}

func (scope *factoryTestAtomicScope) Current() bool {
	return scope.current.Load()
}

func (fto *factoryTestOutput) Write(payload []byte) (int, error) {
	fto.mu.Lock()
	defer fto.mu.Unlock()
	return fto.Buffer.Write(payload)
}

func (fto *factoryTestOutput) Len() int {
	fto.mu.Lock()
	defer fto.mu.Unlock()
	return fto.Buffer.Len()
}

func (fto *factoryTestOutput) String() string {
	fto.mu.Lock()
	defer fto.mu.Unlock()
	return fto.Buffer.String()
}

func (fto *factoryTestOutput) Bytes() []byte {
	fto.mu.Lock()
	defer fto.mu.Unlock()
	return append([]byte(nil), fto.Buffer.Bytes()...)
}

func prepareFactoryTestCandidate(
	ctx context.Context,
	factory *Factory,
	config confgroup.Config,
	identity lifecycle.ResourceIdentity,
	permit lifecycle.LongLivedPermit,
) (PreparedJob, *autoDetectionFailure, error) {
	stage, err := factory.newCandidate(config)
	if err != nil {
		return PreparedJob{}, nil, err
	}
	defer stage.Release()
	if err := factory.awaitCandidate(ctx, stage); err != nil {
		return PreparedJob{}, nil, err
	}
	return factory.prepareCandidate(identity, permit, stage)
}

func requireFactoryAttemptsIdle(t *testing.T, factory *Factory) {
	t.Helper()
	attempts, ok := factory.config.Attempts.(*containment.Authority)
	require.True(t, ok)
	require.Eventually(t, func() bool {
		return attempts.Census() == (containment.Census{})
	}, time.Second, time.Millisecond)
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
