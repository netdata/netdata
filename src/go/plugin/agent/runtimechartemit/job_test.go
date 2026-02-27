// SPDX-License-Identifier: GPL-3.0-or-later

package runtimechartemit

import (
	"bytes"
	"sync"
	"testing"
	"time"

	"github.com/netdata/netdata/go/plugins/plugin/framework/chartemit"
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
			assert.Contains(t, out.String(), "BEGIN")
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
