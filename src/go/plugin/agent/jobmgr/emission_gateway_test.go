// SPDX-License-Identifier: GPL-3.0-or-later

package jobmgr

import (
	"bytes"
	"sync"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// blockingTestWriter blocks its first Write until released so tests can hold
// an in-flight write while probing the gateway lock.
type blockingTestWriter struct {
	mu      sync.Mutex
	buf     bytes.Buffer
	once    sync.Once
	started chan struct{}
	release chan struct{}
}

func (w *blockingTestWriter) Write(p []byte) (int, error) {
	w.once.Do(func() {
		close(w.started)
		<-w.release
	})
	w.mu.Lock()
	defer w.mu.Unlock()
	return w.buf.Write(p)
}

func TestEmissionGateway(t *testing.T) {
	tests := map[string]struct {
		run func(t *testing.T)
	}{
		"open gate passes writes through": {
			run: func(t *testing.T) {
				var buf bytes.Buffer
				gate := newEmissionGateway(&buf)

				n, err := gate.Write([]byte("payload"))

				require.NoError(t, err)
				assert.Equal(t, len("payload"), n)
				assert.Equal(t, "payload", buf.String())
				assert.Zero(t, gate.SuppressedWrites())
			},
		},
		"closed gate suppresses and counts writes": {
			run: func(t *testing.T) {
				var buf bytes.Buffer
				gate := newEmissionGateway(&buf)

				gate.Close()
				n, err := gate.Write([]byte("late"))

				require.NoError(t, err, "suppression must not surface as a write error")
				assert.Equal(t, len("late"), n)
				assert.Empty(t, buf.String(), "closed gate leaked a write")
				assert.Equal(t, 1, gate.SuppressedWrites())
			},
		},
		"close waits out an in-flight write": {
			run: func(t *testing.T) {
				w := &blockingTestWriter{started: make(chan struct{}), release: make(chan struct{})}
				gate := newEmissionGateway(w)

				writeDone := make(chan struct{})
				go func() {
					_, _ = gate.Write([]byte("in-flight"))
					close(writeDone)
				}()
				select {
				case <-w.started:
				case <-time.After(time.Second):
					t.Fatal("write did not start")
				}

				closeDone := make(chan struct{})
				go func() {
					gate.Close()
					close(closeDone)
				}()

				closeReturned := func() bool {
					select {
					case <-closeDone:
						return true
					default:
						return false
					}
				}
				require.Never(t, closeReturned, 200*time.Millisecond, 10*time.Millisecond,
					"Close returned while a write was still in flight")

				close(w.release)
				require.Eventually(t, closeReturned, time.Second, 10*time.Millisecond)
				<-writeDone
				assert.Equal(t, "in-flight", w.buf.String(), "the in-flight write must complete, not be cut")
			},
		},
		"factory-created job gets a tracked gateway until it stops": {
			run: func(t *testing.T) {
				mgr := newCollectorTestManager()
				cfg := prepareDyncfgCfg("success", "gw")

				job, err := mgr.createCollectorJob(cfg)
				require.NoError(t, err)
				gate, tracked := mgr.emissionGates.lookup(cfg.FullName())
				require.True(t, tracked, "creation must register the job's gateway")
				require.NotNil(t, gate)

				mgr.startRunningJob(job)
				mgr.stopRunningJob(cfg.FullName())
				_, tracked = mgr.emissionGates.lookup(cfg.FullName())
				assert.False(t, tracked, "gateway tracking must end when the job stops")
			},
		},
		"failed construction registers no gateway": {
			run: func(t *testing.T) {
				mgr := newCollectorTestManager()
				cfg := prepareDyncfgCfg("success", "gw-badcfg").Set("vnode", "no-such-vnode")

				_, err := mgr.createCollectorJob(cfg)

				require.Error(t, err)
				_, tracked := mgr.emissionGates.lookup(cfg.FullName())
				assert.False(t, tracked, "a job that failed to construct must leave no gateway entry")
			},
		},
		"detection failure drops the gateway entry": {
			run: func(t *testing.T) {
				mgr := newCollectorTestManager()
				cb := &collectorCallbacks{mgr: mgr}
				cfg := prepareDyncfgCfg("fail", "gw-detect")

				err := cb.Start(cfg)

				require.Error(t, err)
				_, tracked := mgr.emissionGates.lookup(cfg.FullName())
				assert.False(t, tracked, "a job that failed detection must leave no gateway entry")
			},
		},
		"same-name replacement keeps the new job's gateway tracked": {
			run: func(t *testing.T) {
				mgr := newCollectorTestManager()
				cfg := prepareDyncfgCfg("success", "gw-replace")

				oldJob, err := mgr.createCollectorJob(cfg)
				require.NoError(t, err)
				mgr.startRunningJob(oldJob)

				newJob, err := mgr.createCollectorJob(cfg)
				require.NoError(t, err)
				newGate, tracked := mgr.emissionGates.lookup(cfg.FullName())
				require.True(t, tracked)

				// The defensive stop inside startRunningJob replaces the old
				// same-name job; the new job's gate must survive it.
				mgr.startRunningJob(newJob)
				t.Cleanup(func() { mgr.stopRunningJob(cfg.FullName()) })

				gate, tracked := mgr.emissionGates.lookup(cfg.FullName())
				require.True(t, tracked, "replacement dropped the new job's gateway tracking")
				assert.Same(t, newGate, gate, "the tracked gate must be the new job's gate")
			},
		},
		"validation-only jobs register no gateway": {
			run: func(t *testing.T) {
				mgr := newCollectorTestManager()
				cfg := prepareDyncfgCfg("success", "gw-validate")

				require.NoError(t, mgr.validateCollectorJob(cfg))

				_, tracked := mgr.emissionGates.lookup(cfg.FullName())
				assert.False(t, tracked, "validation-only creation must not register a gateway")
			},
		},
	}

	for name, tc := range tests {
		t.Run(name, tc.run)
	}
}
