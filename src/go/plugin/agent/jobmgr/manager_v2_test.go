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

type namedTestV1Module struct {
	testV1Module
	jobName string
}

func (m *namedTestV1Module) SetJobName(name string) { m.jobName = name }

type namedTestV2Module struct {
	testV2Module
	jobName string
}

func (m *namedTestV2Module) SetJobName(name string) { m.jobName = name }

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
		"prefer v2 when instance functions are configured": {
			creator: collectorapi.Creator{
				Create: func() collectorapi.CollectorV1 { return &testV1Module{} },
				CreateV2: func() collectorapi.CollectorV2 {
					return &testV2Module{store: metrix.NewCollectorStore()}
				},
				InstanceFunctions: func(_ collectorapi.RuntimeJob) []funcapi.FunctionConfig { return nil },
			},
			wantV2: true,
		},
		"allow v2 only creator when instance functions are configured": {
			creator: collectorapi.Creator{
				CreateV2: func() collectorapi.CollectorV2 {
					return &testV2Module{store: metrix.NewCollectorStore()}
				},
				InstanceFunctions: func(_ collectorapi.RuntimeJob) []funcapi.FunctionConfig { return nil },
			},
			wantV2: true,
		},
		"allow function_only config for v2 when methods exist": {
			creator: collectorapi.Creator{
				CreateV2: func() collectorapi.CollectorV2 {
					return &testV2Module{store: nil}
				},
				InstanceFunctions: func(_ collectorapi.RuntimeJob) []funcapi.FunctionConfig { return nil },
			},
			functionOnly: true,
			wantV2:       true,
		},
	}

	for name, tc := range tests {
		t.Run(name, func(t *testing.T) {
			mgr := New(Config{PluginName: testPluginName})
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

func TestManagerCreateCollectorJobSingleInstancePolicyAllowsV2FunctionOnly(t *testing.T) {
	mod := &namedTestV2Module{}
	mgr := New(Config{PluginName: testPluginName})
	mgr.modules = collectorapi.Registry{
		"testmod": {
			InstancePolicy:    collectorapi.InstancePolicySingle,
			CreateV2:          func() collectorapi.CollectorV2 { return mod },
			InstanceFunctions: func(_ collectorapi.RuntimeJob) []funcapi.FunctionConfig { return nil },
		},
	}

	job, err := mgr.createCollectorJob(prepareFunctionOnlyCfg("testmod", "testmod"))
	require.NoError(t, err)
	require.NotNil(t, job)
	require.NoError(t, job.AutoDetection())

	_, isV2 := job.(*jobruntime.JobV2)
	assert.True(t, isV2)
	assert.Same(t, mod, job.Collector())
	assert.Equal(t, "testmod", mod.jobName)
	assert.Equal(t, "testmod", job.FullName())
}

func TestManagerCreateCollectorJobSetsJobNameV2(t *testing.T) {
	mod := &namedTestV2Module{testV2Module: testV2Module{store: metrix.NewCollectorStore()}}
	mgr := New(Config{PluginName: testPluginName})
	mgr.modules = collectorapi.Registry{
		"testmod": {
			CreateV2: func() collectorapi.CollectorV2 { return mod },
		},
	}

	job, err := mgr.createCollectorJob(prepareUserCfg("testmod", "job1"))
	require.NoError(t, err)
	assert.Same(t, mod, job.Collector())
	assert.Equal(t, "job1", mod.jobName)
}

func TestManagerCreateCollectorJobSetsJobNameV1(t *testing.T) {
	mod := &namedTestV1Module{}
	mgr := New(Config{PluginName: testPluginName})
	mgr.modules = collectorapi.Registry{
		"testmod": {
			Create: func() collectorapi.CollectorV1 { return mod },
		},
	}

	job, err := mgr.createCollectorJob(prepareUserCfg("testmod", "job1"))
	require.NoError(t, err)
	assert.Same(t, mod, job.Collector())
	assert.Equal(t, "job1", mod.jobName)
}

func TestManagerCreateCollectorJobSingleInstancePolicyRequiresCanonicalName(t *testing.T) {
	tests := map[string]struct {
		creator func(*bool) collectorapi.Creator
		jobName string
		wantErr string
	}{
		"v1 canonical name is accepted": {
			creator: func(created *bool) collectorapi.Creator {
				return collectorapi.Creator{
					InstancePolicy: collectorapi.InstancePolicySingle,
					Create: func() collectorapi.CollectorV1 {
						*created = true
						return &testV1Module{}
					},
				}
			},
			jobName: "testmod",
		},
		"v1 non-canonical name is rejected before create": {
			creator: func(created *bool) collectorapi.Creator {
				return collectorapi.Creator{
					InstancePolicy: collectorapi.InstancePolicySingle,
					Create: func() collectorapi.CollectorV1 {
						*created = true
						return &testV1Module{}
					},
				}
			},
			jobName: "job1",
			wantErr: `single-instance collector testmod must use config name "testmod", got "job1"`,
		},
		"v2 non-canonical name is rejected before create": {
			creator: func(created *bool) collectorapi.Creator {
				return collectorapi.Creator{
					InstancePolicy: collectorapi.InstancePolicySingle,
					CreateV2: func() collectorapi.CollectorV2 {
						*created = true
						return &testV2Module{store: metrix.NewCollectorStore()}
					},
				}
			},
			jobName: "job1",
			wantErr: `single-instance collector testmod must use config name "testmod", got "job1"`,
		},
	}

	for name, tc := range tests {
		t.Run(name, func(t *testing.T) {
			created := false
			mgr := New(Config{PluginName: testPluginName})
			mgr.modules = collectorapi.Registry{
				"testmod": tc.creator(&created),
			}

			job, err := mgr.createCollectorJob(prepareUserCfg("testmod", tc.jobName))
			if tc.wantErr != "" {
				require.Error(t, err)
				assert.Contains(t, err.Error(), tc.wantErr)
				assert.False(t, created)
				assert.Nil(t, job)
				return
			}

			require.NoError(t, err)
			assert.True(t, created)
			assert.NotNil(t, job)
		})
	}
}
