// SPDX-License-Identifier: GPL-3.0-or-later

package runtimechartemit

import (
	"sync"
	"testing"
	"time"

	"github.com/netdata/netdata/go/plugins/plugin/framework/chartemit"

	"github.com/netdata/netdata/go/plugins/pkg/metrix"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func newQuarantineTestSpec(name string) componentSpec {
	store := metrix.NewRuntimeStore()
	store.Write().StatefulMeter(name).Gauge("load").Set(5)

	return componentSpec{
		Name:         name,
		Store:        store,
		TemplateYAML: []byte(runtimeGaugeTemplateYAML()),
		UpdateEvery:  1,
		EmitEnv: chartemit.EmitEnv{
			TypeID:      "netdata.go.d.internal." + name,
			UpdateEvery: 1,
			Plugin:      "go.d",
			Module:      "internal",
			JobName:     name,
		},
	}
}

// gatedWriter blocks the first Write until released, so a test can hold an
// in-progress tick inside its write while probing the tick mutex.
type gatedWriter struct {
	safeBuffer

	once    sync.Once
	started chan struct{}
	release chan struct{}
}

func (w *gatedWriter) Write(p []byte) (int, error) {
	w.once.Do(func() {
		close(w.started)
		<-w.release
	})
	return w.safeBuffer.Write(p)
}

func TestRuntimeMetricsJobQuarantineComponent(t *testing.T) {
	tests := map[string]struct {
		run func(t *testing.T)
	}{
		"quarantine waits out an in-progress tick": {
			run: func(t *testing.T) {
				reg := newComponentRegistry()
				reg.upsert(newQuarantineTestSpec("component"))
				out := &gatedWriter{started: make(chan struct{}), release: make(chan struct{})}
				job := newRuntimeMetricsJob(out, reg, nil)

				tickDone := make(chan struct{})
				go func() {
					job.runOnce(1)
					close(tickDone)
				}()

				select {
				case <-out.started:
				case <-time.After(time.Second):
					t.Fatal("tick did not reach its write")
				}

				quarantineDone := make(chan struct{})
				go func() {
					job.quarantineComponent("component")
					close(quarantineDone)
				}()

				quarantineReturned := func() bool {
					select {
					case <-quarantineDone:
						return true
					default:
						return false
					}
				}
				require.Never(t, quarantineReturned, 200*time.Millisecond, 10*time.Millisecond,
					"quarantine returned while a tick was still inside its critical section")

				close(out.release)
				select {
				case <-tickDone:
				case <-time.After(time.Second):
					t.Fatal("tick did not finish after release")
				}
				require.Eventually(t, quarantineReturned, time.Second, 10*time.Millisecond,
					"quarantine did not return after the in-progress tick completed")

				mark := len(out.String())
				job.runOnce(2)
				assert.Equal(t, mark, len(out.String()),
					"no output may follow the barrier for a quarantined sole component")
			},
		},
		"tick mutex is held through post-write finalizers and buffer reset": {
			run: func(t *testing.T) {
				reg := newComponentRegistry()
				reg.upsert(newQuarantineTestSpec("component"))
				out := &gatedWriter{started: make(chan struct{}), release: make(chan struct{})}
				job := newRuntimeMetricsJob(out, reg, nil)

				tickDone := make(chan struct{})
				go func() {
					job.runOnce(1) // first emission: its commit finalizer installs the component state
					close(tickDone)
				}()

				select {
				case <-out.started:
				case <-time.After(time.Second):
					t.Fatal("tick did not reach its write")
				}
				require.False(t, job.tickMu.TryLock(), "tick mutex must be held while the tick writes")

				close(out.release)

				// The first successful acquisition happens-after runOnce's unlock;
				// if the mutex is released before the commit finalizer and buffer
				// reset, the state below would be observed missing.
				deadline := time.Now().Add(5 * time.Second)
				for !job.tickMu.TryLock() {
					if time.Now().After(deadline) {
						t.Fatal("tick mutex was never released")
					}
					time.Sleep(time.Millisecond)
				}
				defer job.tickMu.Unlock()
				_, ok := job.components["component"]
				assert.True(t, ok, "the post-write commit finalizer must run before the tick mutex is released")
				assert.Zero(t, job.buf.Len(), "the buffer reset must happen before the tick mutex is released")

				select {
				case <-tickDone:
				case <-time.After(time.Second):
					t.Fatal("tick did not finish")
				}
			},
		},
		"quarantine removes without obsolete output": {
			run: func(t *testing.T) {
				reg := newComponentRegistry()
				reg.upsert(newQuarantineTestSpec("component"))
				out := &safeBuffer{}
				job := newRuntimeMetricsJob(out, reg, nil)

				job.runOnce(1)
				require.Contains(t, out.String(), "component_load", "component must emit before quarantine")

				job.quarantineComponent("component")

				mark := len(out.String())
				job.runOnce(2)
				job.runOnce(3)
				tail := out.String()[mark:]
				assert.NotContains(t, tail, "component_load", "quarantined component emitted samples")
				assert.NotContains(t, tail, "obsolete", "quarantined component emitted removal-obsolete output")
			},
		},
		"re-registration after quarantine emits normally": {
			run: func(t *testing.T) {
				reg := newComponentRegistry()
				reg.upsert(newQuarantineTestSpec("component"))
				out := &safeBuffer{}
				job := newRuntimeMetricsJob(out, reg, nil)

				job.runOnce(1)
				job.quarantineComponent("component")

				reg.upsert(newQuarantineTestSpec("component"))
				mark := len(out.String())
				job.runOnce(2)
				tail := out.String()[mark:]
				assert.Contains(t, tail, "component_load", "re-registered component must emit again")
				assert.NotContains(t, tail, "obsolete", "fresh registration must not inherit removal output")
			},
		},
	}

	for name, tc := range tests {
		t.Run(name, tc.run)
	}
}

func TestServiceQuarantineComponent(t *testing.T) {
	tests := map[string]struct {
		run func(t *testing.T)
	}{
		"barrier waits out an in-progress tick even during service Stop": {
			run: func(t *testing.T) {
				svc := New(nil)
				store := metrix.NewRuntimeStore()
				store.Write().StatefulMeter("component").Gauge("load").Set(5)
				require.NoError(t, svc.RegisterComponent(ComponentConfig{
					Name:         "component",
					Store:        store,
					TemplateYAML: []byte(runtimeGaugeTemplateYAML()),
				}))

				out := &gatedWriter{started: make(chan struct{}), release: make(chan struct{})}
				svc.Start("go.d", out)

				select {
				case <-out.started:
				case <-time.After(5 * time.Second):
					t.Fatal("no tick reached its write")
				}

				stopDone := make(chan struct{})
				go func() {
					svc.Stop()
					close(stopDone)
				}()

				quarantineDone := make(chan struct{})
				go func() {
					svc.QuarantineComponent("component")
					close(quarantineDone)
				}()

				quarantineReturned := func() bool {
					select {
					case <-quarantineDone:
						return true
					default:
						return false
					}
				}
				require.Never(t, quarantineReturned, 200*time.Millisecond, 10*time.Millisecond,
					"quarantine degraded to a plain registry remove while a tick was in flight")

				close(out.release)
				require.Eventually(t, quarantineReturned, 5*time.Second, 10*time.Millisecond)
				select {
				case <-stopDone:
				case <-time.After(5 * time.Second):
					t.Fatal("service Stop did not finish")
				}
				assert.Empty(t, svc.registry.snapshot(), "registration must be gone after quarantine")
			},
		},
		"quarantine on a never-started service removes the registration": {
			run: func(t *testing.T) {
				svc := New(nil)
				store := metrix.NewRuntimeStore()
				require.NoError(t, svc.RegisterComponent(ComponentConfig{
					Name:         "component",
					Store:        store,
					TemplateYAML: []byte(runtimeGaugeTemplateYAML()),
				}))

				svc.QuarantineComponent("component")

				assert.Empty(t, svc.registry.snapshot())
			},
		},
	}

	for name, tc := range tests {
		t.Run(name, tc.run)
	}
}
