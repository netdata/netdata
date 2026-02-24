// SPDX-License-Identifier: GPL-3.0-or-later

package jobmgr

import (
	"context"
	"testing"

	"github.com/netdata/netdata/go/plugins/plugin/framework/jobruntime"
	"github.com/netdata/netdata/go/plugins/plugin/framework/vnodes"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/netdata/netdata/go/plugins/pkg/funcapi"
	"github.com/netdata/netdata/go/plugins/pkg/metrix"
	"github.com/netdata/netdata/go/plugins/plugin/framework/collectorapi"
)

type testV1Module struct {
	collectorapi.Base
}

func (m *testV1Module) Configuration() any                       { return nil }
func (m *testV1Module) Init(context.Context) error               { return nil }
func (m *testV1Module) Check(context.Context) error              { return nil }
func (m *testV1Module) Collect(context.Context) map[string]int64 { return map[string]int64{"value": 1} }
func (m *testV1Module) Charts() *collectorapi.Charts             { return &collectorapi.Charts{} }
func (m *testV1Module) Cleanup(context.Context)                  {}
func (m *testV1Module) VirtualNode() *vnodes.VirtualNode         { return nil }

type testV2Module struct {
	collectorapi.Base
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
		creator      collectorapi.Creator
		functionOnly bool
		wantV2       bool
		wantErr      string
	}{
		"prefer v2 when hooks do not require legacy runtime": {
			creator: collectorapi.Creator{
				Create: func() collectorapi.CollectorV1 { return &testV1Module{} },
				CreateV2: func() collectorapi.CollectorV2 {
					return &testV2Module{store: metrix.NewCollectorStore()}
				},
			},
			wantV2: true,
		},
		"prefer v2 when job methods are configured": {
			creator: collectorapi.Creator{
				Create: func() collectorapi.CollectorV1 { return &testV1Module{} },
				CreateV2: func() collectorapi.CollectorV2 {
					return &testV2Module{store: metrix.NewCollectorStore()}
				},
				JobMethods: func(_ collectorapi.RuntimeJob) []funcapi.MethodConfig { return nil },
			},
			wantV2: true,
		},
		"allow v2 only creator when job methods are configured": {
			creator: collectorapi.Creator{
				CreateV2: func() collectorapi.CollectorV2 {
					return &testV2Module{store: metrix.NewCollectorStore()}
				},
				JobMethods: func(_ collectorapi.RuntimeJob) []funcapi.MethodConfig { return nil },
			},
			wantV2: true,
		},
		"allow function_only config for v2 when methods exist": {
			creator: collectorapi.Creator{
				CreateV2: func() collectorapi.CollectorV2 {
					return &testV2Module{store: nil}
				},
				JobMethods: func(_ collectorapi.RuntimeJob) []funcapi.MethodConfig { return nil },
			},
			functionOnly: true,
			wantV2:       true,
		},
	}

	for name, tc := range tests {
		t.Run(name, func(t *testing.T) {
			mgr := New(Config{})
			mgr.modules = collectorapi.Registry{
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
			_, isV2 := job.(*jobruntime.JobV2)
			assert.Equal(t, tc.wantV2, isV2)
		})
	}
}
