// SPDX-License-Identifier: GPL-3.0-or-later

package runtimechartemit

import (
	"bytes"
	"errors"
	"maps"
	"slices"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/netdata/netdata/go/plugins/plugin/framework/chartemit"
	"github.com/netdata/netdata/go/plugins/plugin/framework/chartengine"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/netdata/netdata/go/plugins/pkg/metrix"
)

type safeBuffer struct {
	mu  sync.Mutex
	buf bytes.Buffer
}

func (b *safeBuffer) Write(p []byte) (int, error) {
	b.mu.Lock()
	defer b.mu.Unlock()
	return b.buf.Write(p)
}

func (b *safeBuffer) Len() int {
	b.mu.Lock()
	defer b.mu.Unlock()
	return b.buf.Len()
}

func (b *safeBuffer) String() string {
	b.mu.Lock()
	defer b.mu.Unlock()
	return b.buf.String()
}

type failingWriter struct{}

func (failingWriter) Write(_ []byte) (int, error) {
	return 0, errors.New("boom")
}

func requireInOrder(t *testing.T, text string, parts ...string) {
	t.Helper()

	offset := 0
	for _, part := range parts {
		idx := strings.Index(text[offset:], part)
		require.NotEqualf(t, -1, idx, "missing ordered fragment %q in %q", part, text)
		offset += idx + len(part)
	}
}

func TestRuntimeMetricsJobStartStopLifecycle(t *testing.T) {
	tests := map[string]struct {
		clock int
	}{
		"start tick stop emits output and toggles running state": {
			clock: 1,
		},
	}

	for name, tc := range tests {
		t.Run(name, func(t *testing.T) {
			reg := newComponentRegistry()
			store := metrix.NewRuntimeStore()
			store.Write().StatefulMeter("component").Gauge("load").Set(5)

			reg.upsert(componentSpec{
				Name:         "component",
				Store:        store,
				TemplateYAML: []byte(runtimeGaugeTemplateYAML()),
				UpdateEvery:  1,
				EmitEnv: chartemit.EmitEnv{
					TypeID:      "netdata.go.d.internal.component",
					UpdateEvery: 1,
					Plugin:      "go.d",
					Module:      "internal",
					JobName:     "component",
				},
			})

			out := &safeBuffer{}
			job := newRuntimeMetricsJob(out, reg, nil)

			done := make(chan struct{})
			go func() {
				job.Start()
				close(done)
			}()

			require.Eventually(t, func() bool { return job.running.Load() }, time.Second, 10*time.Millisecond)

			job.Tick(tc.clock)
			require.Eventually(t, func() bool { return out.Len() > 0 }, time.Second, 10*time.Millisecond)

			job.Stop()
			select {
			case <-done:
			case <-time.After(time.Second):
				t.Fatal("runtime metrics job did not stop")
			}

			assert.False(t, job.running.Load())
			requireInOrder(t, out.String(), "HOST ''", "BEGIN")
		})
	}
}

func TestRuntimeMetricsJobTickSkipWhenBusy(t *testing.T) {
	tests := map[string]struct {
		skippedTicks int
	}{
		"one tick skipped when queue is full once": {
			skippedTicks: 1,
		},
		"multiple ticks skipped while queue remains full": {
			skippedTicks: 2,
		},
	}

	for name, tc := range tests {
		t.Run(name, func(t *testing.T) {
			job := newRuntimeMetricsJob(&safeBuffer{}, newComponentRegistry(), nil)

			// Fill single-slot queue to force skip path.
			job.tick <- 1
			for i := 0; i < tc.skippedTicks; i++ {
				job.Tick(2 + i)
			}

			resume := job.skipTracker.MarkRunStart(time.Now())
			assert.Equal(t, tc.skippedTicks, resume.Skipped)
		})
	}
}

func TestRuntimeMetricsJobTransactionalScenarios(t *testing.T) {
	tests := map[string]struct {
		run func(t *testing.T)
	}{
		"flush failure aborts component state advancement": {
			run: func(t *testing.T) {
				reg := newComponentRegistry()
				store := metrix.NewRuntimeStore()
				store.Write().StatefulMeter("component").Gauge("load").Set(5)

				reg.upsert(componentSpec{
					Name:         "component",
					Store:        store,
					TemplateYAML: []byte(runtimeGaugeTemplateYAML()),
					UpdateEvery:  1,
					EmitEnv: chartemit.EmitEnv{
						TypeID:      "netdata.go.d.internal.component",
						UpdateEvery: 1,
						Plugin:      "go.d",
						Module:      "internal",
						JobName:     "component",
					},
				})

				job := newRuntimeMetricsJob(failingWriter{}, reg, nil)
				job.runOnce(1)

				state := job.components["component"]
				require.Nil(t, state)

				var out safeBuffer
				job.out = &out
				job.runOnce(2)

				state = job.components["component"]
				require.NotNil(t, state)
				require.NotEmpty(t, state.knownCharts)
				requireInOrder(
					t,
					out.String(),
					"HOST ''",
					"CHART 'netdata.go.d.internal.component.component_load'",
					"BEGIN",
				)
			},
		},
		"aborted tick keeps the installed chart set": {
			run: func(t *testing.T) {
				reg := newComponentRegistry()
				store := metrix.NewRuntimeStore()
				meter := store.Write().StatefulMeter("component")
				meter.Gauge("load").Set(5)

				reg.upsert(componentSpec{
					Name:         "component",
					Store:        store,
					TemplateYAML: []byte(runtimeTwoChartTemplateYAML()),
					UpdateEvery:  1,
					EmitEnv: chartemit.EmitEnv{
						TypeID:      "netdata.go.d.internal.component",
						UpdateEvery: 1,
						Plugin:      "go.d",
						Module:      "internal",
						JobName:     "component",
					},
				})

				job := newRuntimeMetricsJob(&safeBuffer{}, reg, nil)
				job.runOnce(1)
				state := job.components["component"]
				require.NotNil(t, state)
				require.Equal(t, []string{"component_load"}, slices.Sorted(maps.Keys(state.knownCharts)))

				// The second chart appears in a tick whose output is rejected.
				meter.Gauge("queue").Set(3)
				job.out = failingWriter{}
				job.runOnce(2)
				require.Same(t, state, job.components["component"])
				assert.Equal(t, []string{"component_load"}, slices.Sorted(maps.Keys(state.knownCharts)))

				job.out = &safeBuffer{}
				job.runOnce(3)
				assert.Equal(
					t,
					[]string{"component_load", "component_queue"},
					slices.Sorted(maps.Keys(job.components["component"].knownCharts)),
				)
			},
		},
		"generation replacement keeps old state when obsolete emit fails": {
			run: func(t *testing.T) {
				reg := newComponentRegistry()
				store := metrix.NewRuntimeStore()
				store.Write().StatefulMeter("component").Gauge("load").Set(7)
				reg.upsert(componentSpec{
					Name:         "component",
					Store:        store,
					TemplateYAML: []byte(runtimeGaugeTemplateYAML()),
					UpdateEvery:  1,
					EmitEnv: chartemit.EmitEnv{
						TypeID:      "netdata.go.d.internal.component",
						UpdateEvery: 1,
						Plugin:      "go.d",
						Module:      "internal",
						JobName:     "component",
					},
				})

				job := newRuntimeMetricsJob(&safeBuffer{}, reg, nil)
				current := &runtimeComponentState{
					spec: componentSpec{
						Name:       "component",
						Generation: 0,
						EmitEnv: chartemit.EmitEnv{
							TypeID:      " ",
							UpdateEvery: 1,
							Plugin:      "go.d",
							Module:      "internal",
							JobName:     "component",
						},
					},
					knownCharts: map[string]chartengine.ChartMeta{
						"component_load": {
							Title:   "Component Load",
							Context: "netdata.go.plugin.component.component_load",
							Units:   "load",
						},
					},
				}
				job.components["component"] = current

				job.runOnce(1)

				require.Same(t, current, job.components["component"])
				assert.Equal(t, uint64(0), job.components["component"].spec.Generation)
			},
		},
		"removal keeps old state when obsolete emit fails": {
			run: func(t *testing.T) {
				job := newRuntimeMetricsJob(&safeBuffer{}, newComponentRegistry(), nil)
				current := &runtimeComponentState{
					spec: componentSpec{
						Name:       "component",
						Generation: 1,
						EmitEnv: chartemit.EmitEnv{
							TypeID:      " ",
							UpdateEvery: 1,
							Plugin:      "go.d",
							Module:      "internal",
							JobName:     "component",
						},
					},
					knownCharts: map[string]chartengine.ChartMeta{
						"component_load": {
							Title:   "Component Load",
							Context: "netdata.go.plugin.component.component_load",
							Units:   "load",
						},
					},
				}
				job.components["component"] = current

				job.runOnce(1)

				require.Same(t, current, job.components["component"])
				require.Contains(t, job.components["component"].knownCharts, "component_load")
			},
		},
	}

	for name, tc := range tests {
		t.Run(name, tc.run)
	}
}

func runtimeTwoChartTemplateYAML() string {
	return `
version: v1
groups:
  - family: Runtime
    metrics:
      - component.load
      - component.queue
    charts:
      - id: component_load
        title: Component Load
        context: netdata.go.plugin.component.component_load
        units: load
        dimensions:
          - selector: component.load
            name: value
      - id: component_queue
        title: Component Queue
        context: netdata.go.plugin.component.component_queue
        units: items
        dimensions:
          - selector: component.queue
            name: value
`
}
