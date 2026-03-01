// SPDX-License-Identifier: GPL-3.0-or-later

package functions

import (
	"context"
	"sync"
	"testing"
	"time"

	"github.com/netdata/netdata/go/plugins/pkg/metrix"
	"github.com/netdata/netdata/go/plugins/plugin/framework/runtimecomp"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

type runtimeServiceMock struct {
	mu sync.Mutex

	registered   []runtimecomp.ComponentConfig
	unregistered []string
}

func (m *runtimeServiceMock) RegisterComponent(cfg runtimecomp.ComponentConfig) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.registered = append(m.registered, cfg)
	return nil
}

func (m *runtimeServiceMock) UnregisterComponent(name string) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.unregistered = append(m.unregistered, name)
}

func (m *runtimeServiceMock) RegisterProducer(string, func() error) error { return nil }

func (m *runtimeServiceMock) snapshot() ([]runtimecomp.ComponentConfig, []string) {
	m.mu.Lock()
	defer m.mu.Unlock()

	registered := append([]runtimecomp.ComponentConfig(nil), m.registered...)
	unregistered := append([]string(nil), m.unregistered...)
	return registered, unregistered
}

func runtimeMetricValue(t *testing.T, store metrix.RuntimeStore, name string, labels metrix.Labels) float64 {
	t.Helper()
	require.NotNil(t, store)

	reader := store.Read(metrix.ReadRaw())
	v, ok := reader.Value(name, labels)
	require.Truef(t, ok, "metric %q not found (labels=%v)", name, labels)
	return v
}

func TestManager_RuntimeMetricsScenarios(t *testing.T) {
	tests := map[string]struct {
		run func(t *testing.T, mgr *Manager, in *chanInput, out *safeBuffer)
	}{
		"registers and unregisters runtime component around Run lifecycle": {
			run: func(t *testing.T, mgr *Manager, in *chanInput, _ *safeBuffer) {
				mockSvc := &runtimeServiceMock{}
				mgr.SetRuntimeService(mockSvc)
				close(in.ch)

				ctx, cancel := context.WithTimeout(context.Background(), time.Second)
				defer cancel()
				mgr.Run(ctx, nil)

				registered, unregistered := mockSvc.snapshot()
				require.Len(t, registered, 1)
				assert.Equal(t, functionsRuntimeComponentName, registered[0].Name)
				assert.Equal(t, mgr.runtimeStore, registered[0].Store)
				assert.True(t, registered[0].Autogen.Enabled)
				assert.Equal(t, "functions", registered[0].Module)
				assert.Equal(t, "manager", registered[0].JobName)

				require.Len(t, unregistered, 1)
				assert.Equal(t, functionsRuntimeComponentName, unregistered[0])
			},
		},
		"pathology counters and gauges are updated": {
			run: func(t *testing.T, mgr *Manager, in *chanInput, _ *safeBuffer) {
				mgr.workerCount = 1
				mgr.queueSize = 1
				mgr.cancelFallbackDelay = 50 * time.Millisecond

				started := make(chan struct{}, 1)
				release := make(chan struct{})

				mgr.Register("fn", func(fn Function) {
					if fn.UID == "tx1" {
						started <- struct{}{}
						<-release
						mgr.respUID(fn.UID, 200, "late")
						return
					}
					mgr.respUID(fn.UID, 200, "ok")
				})

				cancel, done := startFlowManager(t, mgr)
				defer cancel()

				in.ch <- functionLine("tx1", "fn")
				<-started
				in.ch <- functionLine("tx2", "fn")
				in.ch <- functionLine("tx3", "fn") // queue-full
				in.ch <- functionLine("tx1", "fn") // duplicate-active
				in.ch <- "FUNCTION_CANCEL tx1"     // fallback->499

				waitForCondition(t, time.Second, func() bool {
					reader := mgr.runtimeStore.Read(metrix.ReadRaw())
					v, ok := reader.Value(functionsRuntimeMetricPrefix+".cancel_fallback_total", nil)
					return ok && v >= 1
				}, "cancel fallback metric increments")

				in.ch <- functionLine("tx1", "fn") // duplicate-tombstone
				close(release)
				close(in.ch)
				waitForDone(t, done)

				assert.GreaterOrEqual(t, runtimeMetricValue(t, mgr.runtimeStore, functionsRuntimeMetricPrefix+".queue_full_total", nil), float64(1))
				assert.GreaterOrEqual(t, runtimeMetricValue(t, mgr.runtimeStore, functionsRuntimeMetricPrefix+".cancel_fallback_total", nil), float64(1))
				assert.GreaterOrEqual(t, runtimeMetricValue(t, mgr.runtimeStore, functionsRuntimeMetricPrefix+".late_terminal_dropped_total", nil), float64(1))
				assert.GreaterOrEqual(t, runtimeMetricValue(t, mgr.runtimeStore, functionsRuntimeMetricPrefix+".duplicate_uid_ignored_total", nil), float64(2))
				assert.Equal(t, float64(5), runtimeMetricValue(t, mgr.runtimeStore, functionsRuntimeMetricPrefix+".calls_total", nil))

				assert.Equal(t, float64(0), runtimeMetricValue(t, mgr.runtimeStore, functionsRuntimeMetricPrefix+".invocations_active", nil))
				assert.Equal(t, float64(0), runtimeMetricValue(t, mgr.runtimeStore, functionsRuntimeMetricPrefix+".invocations_awaiting_result", nil))
				assert.Equal(t, float64(0), runtimeMetricValue(t, mgr.runtimeStore, functionsRuntimeMetricPrefix+".scheduler_pending", nil))
			},
		},
	}

	for name, tc := range tests {
		t.Run(name, func(t *testing.T) {
			mgr, out := newFlowManager()
			in := &chanInput{ch: make(chan string, 32)}
			mgr.input = in
			tc.run(t, mgr, in, out)
		})
	}
}
