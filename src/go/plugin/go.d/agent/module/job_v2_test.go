// SPDX-License-Identifier: GPL-3.0-or-later

package module

import (
	"bytes"
	"context"
	"errors"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/netdata/netdata/go/plugins/pkg/metrix"
	"github.com/netdata/netdata/go/plugins/plugin/go.d/agent/vnodes"
)

type mockModuleV2 struct {
	Base

	initFunc    func(context.Context) error
	checkFunc   func(context.Context) error
	collectFunc func(context.Context) error
	cleanupFunc func(context.Context)

	store    metrix.CollectorStore
	template []byte
	cleaned  bool
}

func (m *mockModuleV2) Init(ctx context.Context) error {
	if m.initFunc == nil {
		return nil
	}
	return m.initFunc(ctx)
}

func (m *mockModuleV2) Check(ctx context.Context) error {
	if m.checkFunc == nil {
		return nil
	}
	return m.checkFunc(ctx)
}

func (m *mockModuleV2) Collect(ctx context.Context) error {
	if m.collectFunc == nil {
		return nil
	}
	return m.collectFunc(ctx)
}

func (m *mockModuleV2) Cleanup(ctx context.Context) {
	if m.cleanupFunc != nil {
		m.cleanupFunc(ctx)
	}
	m.cleaned = true
}

func (m *mockModuleV2) Configuration() any                 { return nil }
func (m *mockModuleV2) VirtualNode() *vnodes.VirtualNode   { return nil }
func (m *mockModuleV2) MetricStore() metrix.CollectorStore { return m.store }
func (m *mockModuleV2) ChartTemplateYAML() []byte          { return m.template }

func newTestJobV2(mod ModuleV2, out *bytes.Buffer) *JobV2 {
	return NewJobV2(JobV2Config{
		PluginName:  pluginName,
		Name:        jobName,
		ModuleName:  modName,
		FullName:    modName + "_" + jobName,
		Module:      mod,
		Out:         out,
		UpdateEvery: 1,
		Labels: map[string]string{
			"instance": "localhost",
		},
	})
}

func chartTemplateV2() []byte {
	return []byte(`
version: v1
groups:
  - family: Workers
    metrics:
      - apache.workers_busy
    charts:
      - id: workers_busy
        title: Workers Busy
        context: workers_busy
        units: workers
        dimensions:
          - selector: apache.workers_busy
            name: busy
`)
}

func TestJobV2Scenarios(t *testing.T) {
	tests := map[string]struct {
		run func(t *testing.T)
	}{
		"auto detection succeeds with valid store/template": {
			run: func(t *testing.T) {
				mod := &mockModuleV2{
					store:    metrix.NewCollectorStore(),
					template: chartTemplateV2(),
				}
				job := newTestJobV2(mod, &bytes.Buffer{})
				require.NoError(t, job.AutoDetection())
				require.NotNil(t, job.store)
				require.NotNil(t, job.cycle)
				require.NotNil(t, job.engine)
				assert.True(t, job.engine.Ready())
			},
		},
		"auto detection fails when metric store is nil": {
			run: func(t *testing.T) {
				mod := &mockModuleV2{
					store:    nil,
					template: chartTemplateV2(),
				}
				job := newTestJobV2(mod, &bytes.Buffer{})
				require.ErrorContains(t, job.AutoDetection(), "nil metric store")
			},
		},
		"runOnce collects and emits chart actions": {
			run: func(t *testing.T) {
				store := metrix.NewCollectorStore()
				mod := &mockModuleV2{
					store:    store,
					template: chartTemplateV2(),
					collectFunc: func(context.Context) error {
						store.Write().SnapshotMeter("apache").Gauge("workers_busy").Observe(7)
						return nil
					},
				}

				var out bytes.Buffer
				job := newTestJobV2(mod, &out)
				require.NoError(t, job.AutoDetection())
				job.runOnce()

				wire := out.String()
				assert.Contains(t, wire, "CHART 'module_job.workers_busy'")
				assert.Contains(t, wire, "CLABEL '_collect_job' 'job' '1'")
				assert.Contains(t, wire, "BEGIN 'module_job.workers_busy'")
				assert.Contains(t, wire, "SET 'busy' = 7")
				assert.False(t, job.Panicked())
			},
		},
		"collect error aborts cycle and emits nothing": {
			run: func(t *testing.T) {
				store := metrix.NewCollectorStore()
				mod := &mockModuleV2{
					store:    store,
					template: chartTemplateV2(),
					collectFunc: func(context.Context) error {
						return errors.New("collect failed")
					},
				}

				var out bytes.Buffer
				job := newTestJobV2(mod, &out)
				require.NoError(t, job.AutoDetection())
				job.runOnce()

				assert.Equal(t, "", out.String())
				assert.Equal(t, metrix.CollectStatusFailed, store.ReadRaw().CollectMeta().LastAttemptStatus)
			},
		},
		"panic in collect aborts active cycle and next cycle still succeeds": {
			run: func(t *testing.T) {
				store := metrix.NewCollectorStore()
				collectCalls := 0
				mod := &mockModuleV2{
					store:    store,
					template: chartTemplateV2(),
					collectFunc: func(context.Context) error {
						collectCalls++
						if collectCalls == 1 {
							panic("boom")
						}
						store.Write().SnapshotMeter("apache").Gauge("workers_busy").Observe(11)
						return nil
					},
				}

				var out bytes.Buffer
				job := newTestJobV2(mod, &out)
				require.NoError(t, job.AutoDetection())

				job.runOnce()
				assert.True(t, job.Panicked())
				assert.Equal(t, metrix.CollectStatusFailed, store.ReadRaw().CollectMeta().LastAttemptStatus)
				out.Reset()

				job.runOnce()
				assert.False(t, job.Panicked())
				assert.Equal(t, metrix.CollectStatusSuccess, store.ReadRaw().CollectMeta().LastAttemptStatus)
				assert.Contains(t, out.String(), "SET 'busy' = 11")
			},
		},
	}

	for name, tc := range tests {
		t.Run(name, tc.run)
	}
}
