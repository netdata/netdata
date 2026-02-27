// SPDX-License-Identifier: GPL-3.0-or-later

package agent

import (
	"bytes"
	"context"
	"sync"
	"testing"
	"time"

	"github.com/netdata/netdata/go/plugins/pkg/safewriter"
	"github.com/netdata/netdata/go/plugins/plugin/agent/discovery"
	"github.com/netdata/netdata/go/plugins/plugin/agent/discovery/dummy"
	"github.com/netdata/netdata/go/plugins/plugin/agent/policy"
	"github.com/netdata/netdata/go/plugins/plugin/framework/collectorapi"

	"github.com/stretchr/testify/assert"
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

func TestAgent_Run(t *testing.T) {
	a := New(Config{
		Name: "test",
		RunModePolicy: policy.RunModePolicy{
			IsTerminal:               false,
			AutoEnableDiscovered:     true,
			UseFileStatusPersistence: true,
		},
		DiscoveryProviders: []discovery.ProviderFactory{
			discovery.NewProviderFactory("dummy", func(ctx discovery.BuildContext) (discovery.Discoverer, bool, error) {
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
			}),
		},
	})

	var buf bytes.Buffer
	a.Out = safewriter.New(&buf)

	var mux sync.Mutex
	stats := make(map[string]int)
	a.ModuleRegistry = prepareRegistry(&mux, stats, "module1", "module2")

	ctx, cancel := context.WithCancel(context.Background())
	var wg sync.WaitGroup

	wg.Add(1)
	go func() { defer wg.Done(); a.run(ctx) }()

	time.Sleep(time.Second * 2)
	cancel()
	wg.Wait()

	assert.Equalf(t, 1, stats["module1_init"], "module1 init")
	assert.Equalf(t, 1, stats["module2_init"], "module2 init")
	assert.Equalf(t, 1, stats["module1_check"], "module1 check")
	assert.Equalf(t, 1, stats["module2_check"], "module2 check")
	assert.Equalf(t, 1, stats["module1_charts"], "module1 charts")
	assert.Equalf(t, 1, stats["module2_charts"], "module2 charts")
	assert.Truef(t, stats["module1_collect"] > 0, "module1 collect")
	assert.Truef(t, stats["module2_collect"] > 0, "module2 collect")
	assert.Equalf(t, 1, stats["module1_cleanup"], "module1 cleanup")
	assert.Equalf(t, 1, stats["module2_cleanup"], "module2 cleanup")
	assert.True(t, buf.String() != "")
}

func prepareRegistry(mux *sync.Mutex, stats map[string]int, names ...string) collectorapi.Registry {
	reg := collectorapi.Registry{}
	for _, name := range names {
		name := name
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
