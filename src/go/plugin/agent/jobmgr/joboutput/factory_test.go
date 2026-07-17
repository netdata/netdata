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
	"github.com/netdata/netdata/go/plugins/plugin/agent/secrets/secretstore"
	"github.com/netdata/netdata/go/plugins/plugin/framework/collectorapi"
	"github.com/netdata/netdata/go/plugins/plugin/framework/confgroup"
	"github.com/netdata/netdata/go/plugins/plugin/framework/vnoderegistry"
	"github.com/netdata/netdata/go/plugins/plugin/framework/vnodes"
)

func TestFactoryRejectsWithExactlyOneCollectorCleanup(t *testing.T) {
	tests := map[string]struct {
		configure          func(*factoryTestState, *collectorapi.Creator) JobHooks
		wantClose          int
		wantHandlerCleanup int
	}{
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
		"collector cleanup panic": {
			configure: func(state *factoryTestState, creator *collectorapi.Creator) JobHooks {
				creator.Create = func() collectorapi.CollectorV1 {
					return state.module(func(context.Context) error {
						return errors.New("check failed")
					}, true)
				}
				return nil
			},
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
			wantClose:          1,
			wantHandlerCleanup: 1,
		},
	}
	for name, test := range tests {
		t.Run(name, func(t *testing.T) {
			state := &factoryTestState{}
			creator := collectorapi.Creator{}
			hooks := test.configure(state, &creator)
			factory, output := newFactoryTestHarness(t, creator, hooks)
			constructed, err := factory.Build(
				context.Background(),
				factoryTestConfig(creator.FunctionOnly),
				1,
			)
			if err == nil {
				t.Fatal("factory rejection unexpectedly succeeded")
			}
			if constructed.Runtime != nil {
				t.Fatal("factory rejection returned constructed runtime ownership")
			}
			if state.collectorCleanup != 1 {
				t.Fatalf("collector cleanup calls=%d want=1", state.collectorCleanup)
			}
			if state.handlerClose != test.wantClose ||
				state.handlerCleanup != test.wantHandlerCleanup {
				t.Fatalf(
					"handler close=%d cleanup=%d want=%d/%d",
					state.handlerClose,
					state.handlerCleanup,
					test.wantClose,
					test.wantHandlerCleanup,
				)
			}
			if output.Len() != 0 {
				t.Fatalf("rejected construction emitted output: %q", output.String())
			}
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
			if _, err := factory.Build(
				context.Background(),
				factoryTestConfig(test.functionOnly),
				1,
			); err == nil {
				t.Fatal("factory rejection unexpectedly succeeded")
			}
			if state.collectorCleanup != 1 {
				t.Fatalf("collector cleanup calls=%d want=1", state.collectorCleanup)
			}
			if output.Len() != 0 {
				t.Fatalf("rejected construction emitted output: %q", output.String())
			}
		})
	}
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
	constructed, err := factory.Build(
		context.Background(),
		factoryTestConfig(false),
		1,
	)
	if err != nil {
		t.Fatal(err)
	}
	for range 2 {
		if err := constructed.CollectorCleanup(context.Background()); err != nil {
			t.Fatal(err)
		}
	}
	if state.collectorCleanup != 1 {
		t.Fatalf("collector cleanup calls=%d want=1", state.collectorCleanup)
	}
}

type factoryTestState struct {
	collectorCleanup int
	handlerClose     int
	handlerCleanup   int
}

type factoryTestV2 struct {
	collectorapi.Base
	state    *factoryTestState
	checkErr error
}

func (*factoryTestV2) Init(context.Context) error { return nil }

func (module *factoryTestV2) Check(context.Context) error { return module.checkErr }

func (*factoryTestV2) Collect(context.Context) error { return nil }

func (module *factoryTestV2) Cleanup(context.Context) {
	module.state.collectorCleanup++
}

func (*factoryTestV2) Configuration() any { return struct{}{} }

func (*factoryTestV2) VirtualNode() *vnodes.VirtualNode { return nil }

func (*factoryTestV2) MetricStore() metrix.CollectorStore { return nil }

func (*factoryTestV2) ChartTemplateYAML() string { return "" }

func (state *factoryTestState) module(
	check func(context.Context) error,
	panicCleanup bool,
) *collectorapi.MockCollectorV1 {
	return &collectorapi.MockCollectorV1{
		CheckFunc: check,
		CleanupFunc: func(context.Context) {
			state.collectorCleanup++
			if panicCleanup {
				panic("cleanup failed")
			}
		},
	}
}

type factoryTestHooks struct {
	prepare func(PublishedJob) (HandlerLifecycle, error)
}

func (hooks factoryTestHooks) Prepare(job PublishedJob) (HandlerLifecycle, error) {
	return hooks.prepare(job)
}

type factoryTestHandlers struct {
	state *factoryTestState
}

func (*factoryTestHandlers) Publish() error { return nil }

func (handlers *factoryTestHandlers) CloseAndDrain(context.Context) error {
	handlers.state.handlerClose++
	return nil
}

func (handlers *factoryTestHandlers) Cleanup(context.Context) error {
	handlers.state.handlerCleanup++
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
	if err != nil {
		t.Fatal(err)
	}
	tasks, err := lifecycle.NewTaskSupervisor(frames)
	if err != nil {
		t.Fatal(err)
	}
	resolver, err := secretresolver.NewAtomicResolver(nil)
	if err != nil {
		t.Fatal(err)
	}
	factory, err := NewFactory(FactoryConfig{
		PluginName: "test",
		Modules:    collectorapi.Registry{"module": creator},
		Tasks:      tasks,
		Frames:     frames,
		Resolver:   resolver,
		Stores:     secretstore.NewService(),
		Vnodes:     vnoderegistry.New(),
		Hooks:      hooks,
		Scheduler:  newTestScheduler(t),
	})
	if err != nil {
		t.Fatal(err)
	}
	return factory, output
}

func factoryTestConfig(functionOnly bool) confgroup.Config {
	return confgroup.Config{
		"module":        "module",
		"name":          "job",
		"update_every":  1,
		"function_only": functionOnly,
	}
}
