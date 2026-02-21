// SPDX-License-Identifier: GPL-3.0-or-later

package jobmgr

import (
	"context"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/netdata/netdata/go/plugins/pkg/funcapi"
	"github.com/netdata/netdata/go/plugins/pkg/metrix"
	"github.com/netdata/netdata/go/plugins/plugin/go.d/agent/module"
	"github.com/netdata/netdata/go/plugins/plugin/go.d/agent/vnodes"
)

type testV1Module struct {
	module.Base
}

func (m *testV1Module) Configuration() any                       { return nil }
func (m *testV1Module) Init(context.Context) error               { return nil }
func (m *testV1Module) Check(context.Context) error              { return nil }
func (m *testV1Module) Collect(context.Context) map[string]int64 { return map[string]int64{"value": 1} }
func (m *testV1Module) Charts() *module.Charts                   { return &module.Charts{} }
func (m *testV1Module) Cleanup(context.Context)                  {}
func (m *testV1Module) VirtualNode() *vnodes.VirtualNode         { return nil }

type testV2Module struct {
	module.Base
	store metrix.CollectorStore
}

func (m *testV2Module) Configuration() any                 { return nil }
func (m *testV2Module) Init(context.Context) error         { return nil }
func (m *testV2Module) Check(context.Context) error        { return nil }
func (m *testV2Module) Collect(context.Context) error      { return nil }
func (m *testV2Module) Cleanup(context.Context)            {}
func (m *testV2Module) VirtualNode() *vnodes.VirtualNode   { return nil }
func (m *testV2Module) MetricStore() metrix.CollectorStore { return m.store }
func (m *testV2Module) ChartTemplateYAML() string {
	return `
version: "1"
groups:
  - family: "test"
    metrics: ["test.value"]
    charts:
      - context: "value"
        dimensions:
          - selector: 'test.value'
            name: "value"
`
}

func TestManagerCreateCollectorJobV2Branching(t *testing.T) {
	tests := map[string]struct {
		creator      module.Creator
		functionOnly bool
		wantV2       bool
		wantErr      string
	}{
		"prefer v2 when hooks do not require legacy runtime": {
			creator: module.Creator{
				Create: func() module.Module { return &testV1Module{} },
				CreateV2: func() module.ModuleV2 {
					return &testV2Module{store: metrix.NewCollectorStore()}
				},
			},
			wantV2: true,
		},
		"prefer v2 when job methods are configured": {
			creator: module.Creator{
				Create: func() module.Module { return &testV1Module{} },
				CreateV2: func() module.ModuleV2 {
					return &testV2Module{store: metrix.NewCollectorStore()}
				},
				JobMethods: func(_ module.RuntimeJob) []funcapi.MethodConfig { return nil },
			},
			wantV2: true,
		},
		"allow v2 only creator when job methods are configured": {
			creator: module.Creator{
				CreateV2: func() module.ModuleV2 {
					return &testV2Module{store: metrix.NewCollectorStore()}
				},
				JobMethods: func(_ module.RuntimeJob) []funcapi.MethodConfig { return nil },
			},
			wantV2: true,
		},
		"allow function_only config for v2 when methods exist": {
			creator: module.Creator{
				CreateV2: func() module.ModuleV2 {
					return &testV2Module{store: nil}
				},
				JobMethods: func(_ module.RuntimeJob) []funcapi.MethodConfig { return nil },
			},
			functionOnly: true,
			wantV2:       true,
		},
	}

	for name, tc := range tests {
		t.Run(name, func(t *testing.T) {
			mgr := New()
			mgr.Modules = module.Registry{
				"testmod": tc.creator,
			}
			cfg := prepareUserCfg("testmod", "job1")
			if tc.functionOnly {
				cfg.Set("function_only", true)
			}

			job, err := mgr.createCollectorJob(cfg)
			if tc.wantErr != "" {
				require.Error(t, err)
				assert.Contains(t, err.Error(), tc.wantErr)
				return
			}

			require.NoError(t, err)
			_, isV2 := job.(*module.JobV2)
			assert.Equal(t, tc.wantV2, isV2)
		})
	}
}
