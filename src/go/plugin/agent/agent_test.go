// SPDX-License-Identifier: GPL-3.0-or-later

package agent

import (
	"bytes"
	"context"
	"io"
	"sync"
	"testing"
	"time"

	"github.com/netdata/netdata/go/plugins/pkg/safewriter"
	"github.com/netdata/netdata/go/plugins/plugin/agent/discovery"
	"github.com/netdata/netdata/go/plugins/plugin/agent/discovery/dummy"
	"github.com/netdata/netdata/go/plugins/plugin/agent/policy"
	"github.com/netdata/netdata/go/plugins/plugin/framework/collectorapi"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestNew(t *testing.T) {
	t.Run("uses injected module registry", func(t *testing.T) {
		reg := prepareRegistry(&sync.Mutex{}, map[string]int{}, "module1")
		a := New(Config{Name: "test", ModuleRegistry: reg})
		assert.Equal(t, reg, a.ModuleRegistry)
	})

	t.Run("keeps nil module registry when not provided", func(t *testing.T) {
		a := New(Config{Name: "test"})
		assert.Nil(t, a.ModuleRegistry)
	})
}

func TestAgent_serviceDiscoveryEnabled(t *testing.T) {
	tests := map[string]struct {
		agent *Agent
		want  bool
	}{
		"non-terminal policy enables service discovery": {
			agent: &Agent{runModePolicy: policy.Agent(false)},
			want:  true,
		},
		"terminal policy disables service discovery": {
			agent: &Agent{runModePolicy: policy.Agent(true)},
			want:  false,
		},
		"plugin-level disable wins over policy": {
			agent: &Agent{
				runModePolicy:           policy.Agent(false),
				DisableServiceDiscovery: true,
			},
			want: false,
		},
	}

	for name, test := range tests {
		t.Run(name, func(t *testing.T) {
			require.NotNil(t, test.agent)
			assert.Equal(t, test.want, test.agent.serviceDiscoveryEnabled())
		})
	}
}

func TestAgent_setupRuntimeService(t *testing.T) {
	tests := map[string]struct {
		policy      policy.RunModePolicy
		wantEnabled bool
	}{
		"terminal mode disables runtime service": {
			policy:      policy.Agent(true),
			wantEnabled: false,
		},
		"non-terminal mode enables runtime service": {
			policy:      policy.Agent(false),
			wantEnabled: true,
		},
	}

	for name, test := range tests {
		t.Run(name, func(t *testing.T) {
			a := New(Config{
				Name:          "test",
				RunModePolicy: test.policy,
			})
			require.NotNil(t, a)
			a.Out = io.Discard

			svc := a.setupRuntimeService()
			if !test.wantEnabled {
				assert.Nil(t, svc)
				return
			}

			require.NotNil(t, svc)
		})
	}
}

func TestAgent_Run(t *testing.T) {
	tests := map[string]struct {
		restarts int
	}{
		"collects and terminates": {},
		"acknowledged restart rotates the complete generation": {
			restarts: 1,
		},
	}
	for name, test := range tests {
		t.Run(name, func(t *testing.T) {
			a := newLifecycleTestAgent()
			var buf bytes.Buffer
			a.Out = safewriter.New(&buf)
			reader, writer := io.Pipe()
			a.In = reader

			var mux sync.Mutex
			stats := make(map[string]int)
			a.ModuleRegistry = prepareRegistry(&mux, stats, "module1", "module2")

			runDone := make(chan error, 1)
			go func() { runDone <- a.run(context.Background()) }()
			waitForCollection(t, &mux, stats, 1)

			for restart := 0; restart < test.restarts; restart++ {
				ctx, cancel := context.WithTimeout(context.Background(), 4*time.Second)
				require.NoError(t, a.Restart(ctx))
				cancel()
				waitForCollection(t, &mux, stats, restart+2)
			}

			ctx, cancel := context.WithTimeout(context.Background(), 4*time.Second)
			require.NoError(t, a.Terminate(ctx))
			cancel()
			require.NoError(t, writer.Close())
			require.NoError(t, <-runDone)

			generations := test.restarts + 1
			for _, module := range []string{"module1", "module2"} {
				assert.Equalf(t, generations, stats[module+"_init"], "%s init", module)
				assert.Equalf(t, generations, stats[module+"_check"], "%s check", module)
				assert.Equalf(t, generations, stats[module+"_charts"], "%s charts", module)
				assert.GreaterOrEqualf(
					t,
					stats[module+"_collect"],
					generations,
					"%s collect",
					module,
				)
				assert.Equalf(t, generations, stats[module+"_cleanup"], "%s cleanup", module)
			}
			assert.NotEmpty(t, buf.String())
		})
	}
}

func newLifecycleTestAgent() *Agent {
	return New(Config{
		Name: "test",
		RunModePolicy: policy.RunModePolicy{
			IsTerminal:           false,
			AutoEnableDiscovered: true,
			EnableRuntimeCharts:  true,
		},
		DiscoveryProviders: []discovery.ProviderFactory{
			discovery.NewProviderFactory(
				"dummy",
				func(ctx discovery.BuildContext) (discovery.Discoverer, bool, error) {
					if len(ctx.DummyNames) == 0 {
						return nil, false, nil
					}
					d, err := dummy.NewDiscovery(dummy.Config{
						Registry: ctx.Registry,
						Names:    ctx.DummyNames,
					})
					if err != nil {
						return nil, false, err
					}
					return d, true, nil
				},
			),
		},
	})
}

func waitForCollection(
	t *testing.T,
	mux *sync.Mutex,
	stats map[string]int,
	generations int,
) {
	t.Helper()
	require.Eventually(t, func() bool {
		mux.Lock()
		defer mux.Unlock()
		return stats["module1_collect"] >= generations &&
			stats["module2_collect"] >= generations
	}, 4*time.Second, 50*time.Millisecond)
}

func prepareRegistry(mux *sync.Mutex, stats map[string]int, names ...string) collectorapi.Registry {
	reg := collectorapi.Registry{}
	for _, name := range names {
		reg.Register(name, collectorapi.Creator{
			Create: func() collectorapi.CollectorV1 {
				return prepareMockModule(name, mux, stats)
			},
		})
	}
	return reg
}

func prepareMockModule(name string, mux *sync.Mutex, stats map[string]int) collectorapi.CollectorV1 {
	return &collectorapi.MockCollectorV1{
		InitFunc: func(context.Context) error {
			mux.Lock()
			defer mux.Unlock()
			stats[name+"_init"]++
			return nil
		},
		CheckFunc: func(context.Context) error {
			mux.Lock()
			defer mux.Unlock()
			stats[name+"_check"]++
			return nil
		},
		ChartsFunc: func() *collectorapi.Charts {
			mux.Lock()
			defer mux.Unlock()
			stats[name+"_charts"]++
			return &collectorapi.Charts{
				&collectorapi.Chart{ID: "id", Title: "title", Units: "units", Dims: collectorapi.Dims{{ID: "id1"}}},
			}
		},
		CollectFunc: func(context.Context) map[string]int64 {
			mux.Lock()
			defer mux.Unlock()
			stats[name+"_collect"]++
			return map[string]int64{"id1": 1}
		},
		CleanupFunc: func(context.Context) {
			mux.Lock()
			defer mux.Unlock()
			stats[name+"_cleanup"]++
		},
	}
}
