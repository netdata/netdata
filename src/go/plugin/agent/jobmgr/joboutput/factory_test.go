// SPDX-License-Identifier: GPL-3.0-or-later

package joboutput

import (
	"bytes"
	"context"
	"errors"
	"testing"

	"github.com/netdata/netdata/go/plugins/pkg/funcapi"
	"github.com/netdata/netdata/go/plugins/pkg/metrix"
	"github.com/netdata/netdata/go/plugins/plugin/agent/jobmgr/lifecycle"
	secretresolver "github.com/netdata/netdata/go/plugins/plugin/agent/secrets/resolver"
	"github.com/netdata/netdata/go/plugins/plugin/framework/collectorapi"
	"github.com/netdata/netdata/go/plugins/plugin/framework/confgroup"
	"github.com/netdata/netdata/go/plugins/plugin/framework/vnoderegistry"
	"github.com/netdata/netdata/go/plugins/plugin/framework/vnodes"
	"github.com/stretchr/testify/require"
)

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
					prepare: func(PublishedJob) (HandlerLifecycle, error) {
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
					prepare: func(PublishedJob) (HandlerLifecycle, error) {
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
				if err == nil && failure != nil {
					err = errors.Join(failure, permit.Return())
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
			require.Equal(t, test.wantRetained, tasks.LongLivedCensus().Active != 0)
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
				if err == nil && failure != nil {
					err = errors.Join(failure, permit.Return())
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
	require.NoError(t, permit.Return())
	require.EqualValues(t, 1, state.autoDetection)
	require.EqualValues(t, 1, state.collectorCleanup)
	require.EqualValues(t, lifecycle.LongLivedCensus{}, tasks.LongLivedCensus())

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
	require.NoError(t, permit.Return())
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
	require.NoError(t, permit.Return())
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
	require.NoError(t, permit.Return())
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
				require.NoError(t, permit.Return())
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

func (*factoryTestV2) MetricStore() metrix.CollectorStore { return nil }

func (*factoryTestV2) ChartTemplateYAML() string { return "" }

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
	prepare func(PublishedJob) (HandlerLifecycle, error)
}

func (fth factoryTestHooks) Prepare(job PublishedJob) (HandlerLifecycle, error) {
	return fth.prepare(job)
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
		Hooks:            hooks,
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
