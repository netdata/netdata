// SPDX-License-Identifier: GPL-3.0-or-later

package agent

import (
	"bytes"
	"context"
	"sync"
	"testing"
	"time"

	"github.com/netdata/netdata/go/plugins/pkg/safewriter"
	"github.com/netdata/netdata/go/plugins/plugin/go.d/agent/module"

	"github.com/stretchr/testify/assert"
)

// TODO: tech debt
func TestNew(t *testing.T) {

}

func TestAgent_Run(t *testing.T) {
	a := New(Config{Name: "nodyncfg"})

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

func prepareRegistry(mux *sync.Mutex, stats map[string]int, names ...string) module.Registry {
	reg := module.Registry{}
	for _, name := range names {
		name := name
		reg.Register(name, module.Creator{
			Create: func() module.Module {
				return prepareMockModule(name, mux, stats)
			},
		})
	}
	return reg
}

func prepareMockModule(name string, mux *sync.Mutex, stats map[string]int) module.Module {
	return &module.MockModule{
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
		ChartsFunc: func() *module.Charts {
			mux.Lock()
			defer mux.Unlock()
			stats[name+"_charts"]++
			return &module.Charts{
				&module.Chart{ID: "id", Title: "title", Units: "units", Dims: module.Dims{{ID: "id1"}}},
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
