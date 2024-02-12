// SPDX-License-Identifier: GPL-3.0-or-later

package jobmgr

import (
	"bytes"
	"context"
	"sync"
	"testing"
	"time"

	"github.com/netdata/go.d.plugin/agent/confgroup"
	"github.com/netdata/go.d.plugin/agent/module"
	"github.com/netdata/go.d.plugin/agent/safewriter"
	"github.com/stretchr/testify/assert"
)

// TODO: tech dept
func TestNewManager(t *testing.T) {

}

// TODO: tech dept
func TestManager_Run(t *testing.T) {
	groups := []*confgroup.Group{
		{
			Source: "source",
			Configs: []confgroup.Config{
				{
					"name":                "name",
					"module":              "success",
					"update_every":        module.UpdateEvery,
					"autodetection_retry": module.AutoDetectionRetry,
					"priority":            module.Priority,
				},
				{
					"name":                "name",
					"module":              "success",
					"update_every":        module.UpdateEvery + 1,
					"autodetection_retry": module.AutoDetectionRetry,
					"priority":            module.Priority,
				},
				{
					"name":                "name",
					"module":              "fail",
					"update_every":        module.UpdateEvery + 1,
					"autodetection_retry": module.AutoDetectionRetry,
					"priority":            module.Priority,
				},
			},
		},
	}
	var buf bytes.Buffer
	mgr := NewManager()
	mgr.Modules = prepareMockRegistry()
	mgr.Out = safewriter.New(&buf)
	mgr.PluginName = "test.plugin"

	ctx, cancel := context.WithCancel(context.Background())
	in := make(chan []*confgroup.Group)
	var wg sync.WaitGroup

	wg.Add(1)
	go func() { defer wg.Done(); mgr.Run(ctx, in) }()

	select {
	case in <- groups:
	case <-time.After(time.Second * 2):
	}

	time.Sleep(time.Second * 5)
	cancel()
	wg.Wait()

	assert.True(t, buf.String() != "")
}

func prepareMockRegistry() module.Registry {
	reg := module.Registry{}
	reg.Register("success", module.Creator{
		Create: func() module.Module {
			return &module.MockModule{
				InitFunc:  func() bool { return true },
				CheckFunc: func() bool { return true },
				ChartsFunc: func() *module.Charts {
					return &module.Charts{
						&module.Chart{ID: "id", Title: "title", Units: "units", Dims: module.Dims{{ID: "id1"}}},
					}
				},
				CollectFunc: func() map[string]int64 {
					return map[string]int64{"id1": 1}
				},
			}
		},
	})
	reg.Register("fail", module.Creator{
		Create: func() module.Module {
			return &module.MockModule{
				InitFunc: func() bool { return false },
			}
		},
	})
	return reg
}
