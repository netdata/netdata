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
				return factoryTestHooks{prepare: func(PublishedJob) (HandlerLifecycle, error) {
					return &factoryTestHandlers{state: state}, errors.New("prepare failed")
				}}
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
				return factoryTestHooks{prepare: func(PublishedJob) (HandlerLifecycle, error) {
					panic("prepare failed")
				}}
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
			permit, tasks := issueTestJobPermit(
				t,
				"module_job",
				1,
			)
			prepared, err := factory.Prepare(
				context.Background(),
				factoryTestConfig(creator.FunctionOnly),
				lifecycle.ResourceIdentity{
					ID: "module_job", Generation: 1,
				},
				permit,
			)
			if err == nil {
				_, err = prepared.AcceptStart(context.Background(), 1)
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
			require.Equal(
				t,
				test.wantRetained,
				tasks.LongLivedCensus().Active != 0,
			)
		})
	}
}

func TestFactoryV2RejectsWithExactlyOneCollectorCleanup(t *testing.T) {
	tests := map[string]struct {
		functionOnly bool
		checkErr     error
		hooks        JobHooks
	}{
		"autodetection failure": {
			checkErr: errors.New("check failed"),
		},
		"function-bearing job without hooks": {
			functionOnly: true,
		},
	}
	for name, test := range tests {
		t.Run(name, func(t *testing.T) {
			state := &factoryTestState{}
			creator := collectorapi.Creator{
				FunctionOnly: test.functionOnly,
				CreateV2: func() collectorapi.CollectorV2 {
					return &factoryTestV2{
						state: state, checkErr: test.checkErr,
					}
				},
			}
			if test.functionOnly {
				creator.SharedFunctions = func() []funcapi.FunctionConfig { return nil }
			}
			factory, output := newFactoryTestHarness(t, creator, test.hooks)
			permit, tasks := issueTestJobPermit(
				t,
				"module_job",
				1,
			)

			prepared, err := factory.Prepare(
				context.Background(),
				factoryTestConfig(test.functionOnly),
				lifecycle.ResourceIdentity{
					ID: "module_job", Generation: 1,
				},
				permit,
			)
			if err == nil {
				_, err = prepared.AcceptStart(context.Background(), 1)
			} else {
				err = errors.Join(err, permit.AbortUnused())
			}
			require.Error(t, err)

			require.EqualValues(t, 1, state.collectorCleanup)
			require.EqualValues(t, 0, output.Len())
			require.EqualValues(
				t,
				lifecycle.LongLivedCensus{},
				tasks.LongLivedCensus(),
			)
		})
	}
}

func TestFactoryDefersAutoDetectionUntilPreparedJobAcceptance(t *testing.T) {
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
	permit, tasks := issueTestJobPermit(
		t,
		"module_job",
		1,
	)

	prepared, err := factory.Prepare(
		context.Background(),
		factoryTestConfig(false),
		lifecycle.ResourceIdentity{ID: "module_job", Generation: 1},
		permit,
	)
	require.NoError(t, err)
	require.Zero(t, state.autoDetection)
	require.Zero(t, state.collectorCleanup)

	_, err = prepared.AcceptStart(context.Background(), 1)
	require.Error(t, err)
	require.EqualValues(t, 1, state.autoDetection)
	require.EqualValues(t, 1, state.collectorCleanup)
	require.EqualValues(t, lifecycle.LongLivedCensus{}, tasks.LongLivedCensus())

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
	permit, tasks := issueTestJobPermit(
		t,
		"module_job",
		1,
	)
	prepared, err := factory.Prepare(
		context.Background(),
		factoryTestConfig(false),
		lifecycle.ResourceIdentity{ID: "module_job", Generation: 1},
		permit,
	)
	require.NoError(t, err)
	for range 2 {
		err = prepared.Dispose(context.Background())
	}
	require.Error(t, err)
	require.EqualValues(t, 1, state.collectorCleanup)
	require.EqualValues(
		t,
		lifecycle.LongLivedCensus{},
		tasks.LongLivedCensus(),
	)
}

type factoryTestState struct {
	collectorCleanup int
	handlerClose     int
	autoDetection    int
}

type factoryTestV2 struct {
	collectorapi.Base
	state    *factoryTestState
	checkErr error
}

func (*factoryTestV2) Init(context.Context) error { return nil }

func (ft2 *factoryTestV2) Check(context.Context) error { return ft2.checkErr }

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

func newFactoryTestHarness(
	t *testing.T,
	creator collectorapi.Creator,
	hooks JobHooks,
) (*Factory, *bytes.Buffer) {
	t.Helper()
	output := &bytes.Buffer{}
	frames, err := lifecycle.NewFrameOwner(output)
	require.NoError(t, err)
	tasks, err := lifecycle.NewTaskSupervisor(frames)
	require.NoError(t, err)
	resolver, err := secretresolver.NewAtomicResolver(nil)
	require.NoError(t, err)
	configModules, err := NewConfigModuleFactory(ConfigModuleFactoryConfig{
		Modules:    collectorapi.Registry{"module": creator},
		Resolver:   resolver,
		StoreScope: unavailableStoreScope,
	})
	require.NoError(t, err)
	factory, err := NewFactory(FactoryConfig{
		PluginName:    "test",
		Modules:       collectorapi.Registry{"module": creator},
		Tasks:         tasks,
		Frames:        frames,
		ConfigModules: configModules,
		Vnodes:        vnoderegistry.New(),
		Hooks:         hooks,
		Scheduler:     newTestScheduler(t),
	})
	require.NoError(t, err)
	return factory, output
}

func unavailableStoreScope(
	[]string,
) (secretresolver.AtomicScope, error) {
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
