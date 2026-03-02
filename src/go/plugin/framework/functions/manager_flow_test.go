// SPDX-License-Identifier: GPL-3.0-or-later

package functions

import (
	"bytes"
	"context"
	"fmt"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/netdata/netdata/go/plugins/pkg/netdataapi"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

const (
	testPermissions = "0xFFFF"
	testSource      = "method=api,role=test"
)

type chanInput struct {
	ch chan string
}

func (m *chanInput) lines() <-chan string {
	return m.ch
}

type safeBuffer struct {
	mu sync.Mutex
	b  bytes.Buffer
}

func (s *safeBuffer) Write(p []byte) (int, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.b.Write(p)
}

func (s *safeBuffer) String() string {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.b.String()
}

func newFlowManager() (*Manager, *safeBuffer) {
	mgr := NewManager()
	buf := &safeBuffer{}
	mgr.api = netdataapi.New(buf)
	return mgr, buf
}

func functionLine(uid, name string) string {
	return fmt.Sprintf(`FUNCTION %s 10 "%s" %s "%s"`, uid, name, testPermissions, testSource)
}

func payloadStartCmd(uid, name string) string {
	return fmt.Sprintf(`FUNCTION_PAYLOAD %s 10 "%s" %s "%s" application/json`, uid, name, testPermissions, testSource)
}

func waitForSubstring(t *testing.T, f func() string, substr string, timeout time.Duration) {
	t.Helper()

	deadline := time.Now().Add(timeout)
	for time.Now().Before(deadline) {
		if strings.Contains(f(), substr) {
			return
		}
		time.Sleep(10 * time.Millisecond)
	}
	t.Fatalf("timeout waiting for substring %q in output: %s", substr, f())
}

func startFlowManager(t *testing.T, mgr *Manager) (context.CancelFunc, chan struct{}) {
	t.Helper()

	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	done := make(chan struct{})
	go func() {
		defer close(done)
		mgr.Run(ctx, nil)
	}()
	return cancel, done
}

func waitForDone(t *testing.T, done <-chan struct{}) {
	t.Helper()

	select {
	case <-done:
	case <-time.After(3 * time.Second):
		t.Fatal("timeout waiting for manager run to complete")
	}
}

func waitForCondition(t *testing.T, timeout time.Duration, fn func() bool, desc string) {
	t.Helper()

	deadline := time.Now().Add(timeout)
	for time.Now().Before(deadline) {
		if fn() {
			return
		}
		time.Sleep(10 * time.Millisecond)
	}
	t.Fatalf("timeout waiting for condition: %s", desc)
}

func TestManager_FlowScenarios(t *testing.T) {
	tests := map[string]struct {
		run func(t *testing.T, mgr *Manager, in *chanInput, out *safeBuffer)
	}{
		"queued cancel emits 499 and skips execution": {
			run: func(t *testing.T, mgr *Manager, in *chanInput, out *safeBuffer) {
				var (
					mu       sync.Mutex
					executed []string
				)
				started := make(chan struct{}, 1)
				release := make(chan struct{})

				mgr.Register("fn", func(fn Function) {
					mu.Lock()
					executed = append(executed, fn.UID)
					mu.Unlock()
					if fn.UID == "tx1" {
						started <- struct{}{}
						<-release
					}
				})

				cancel, done := startFlowManager(t, mgr)
				defer cancel()

				in.ch <- functionLine("tx1", "fn")
				<-started
				in.ch <- functionLine("tx2", "fn")
				in.ch <- "FUNCTION_CANCEL tx2"
				close(release)
				close(in.ch)
				waitForDone(t, done)

				waitForSubstring(t, out.String, "FUNCTION_RESULT_BEGIN tx2 499", time.Second)
				mu.Lock()
				defer mu.Unlock()
				assert.Equal(t, []string{"tx1"}, executed)
			},
		},
		"running cancel fallback emits 499 once": {
			run: func(t *testing.T, mgr *Manager, in *chanInput, out *safeBuffer) {
				mgr.cancelFallbackDelay = 50 * time.Millisecond
				started := make(chan struct{}, 1)
				release := make(chan struct{})

				mgr.Register("fn", func(fn Function) {
					if fn.UID == "tx1" {
						started <- struct{}{}
						<-release
					}
				})

				cancel, done := startFlowManager(t, mgr)
				defer cancel()

				in.ch <- functionLine("tx1", "fn")
				<-started
				in.ch <- "FUNCTION_CANCEL tx1"
				waitForSubstring(t, out.String, "FUNCTION_RESULT_BEGIN tx1 499", time.Second)

				close(release)
				close(in.ch)
				waitForDone(t, done)

				assert.Equal(t, 1, strings.Count(out.String(), "FUNCTION_RESULT_BEGIN tx1 499"))
			},
		},
		"repeated cancel for same uid still emits one terminal response": {
			run: func(t *testing.T, mgr *Manager, in *chanInput, out *safeBuffer) {
				mgr.cancelFallbackDelay = 50 * time.Millisecond
				started := make(chan struct{}, 1)
				release := make(chan struct{})

				mgr.Register("fn", func(fn Function) {
					if fn.UID == "tx1" {
						started <- struct{}{}
						<-release
					}
				})

				cancel, done := startFlowManager(t, mgr)
				defer cancel()

				in.ch <- functionLine("tx1", "fn")
				<-started
				in.ch <- "FUNCTION_CANCEL tx1"
				in.ch <- "FUNCTION_CANCEL tx1"
				waitForSubstring(t, out.String, "FUNCTION_RESULT_BEGIN tx1 499", time.Second)

				close(release)
				close(in.ch)
				waitForDone(t, done)

				assert.Equal(t, 1, strings.Count(out.String(), "FUNCTION_RESULT_BEGIN tx1 499"))
			},
		},
		"running cancel drops late terminal response": {
			run: func(t *testing.T, mgr *Manager, in *chanInput, out *safeBuffer) {
				mgr.cancelFallbackDelay = 50 * time.Millisecond
				started := make(chan struct{}, 1)
				release := make(chan struct{})

				mgr.Register("fn", func(fn Function) {
					if fn.UID == "tx1" {
						started <- struct{}{}
						<-release
						mgr.respUID(fn.UID, 200, "late response")
					}
				})

				cancel, done := startFlowManager(t, mgr)
				defer cancel()

				in.ch <- functionLine("tx1", "fn")
				<-started
				in.ch <- "FUNCTION_CANCEL tx1"
				waitForSubstring(t, out.String, "FUNCTION_RESULT_BEGIN tx1 499", time.Second)

				close(release)
				close(in.ch)
				waitForDone(t, done)

				got := out.String()
				assert.Equal(t, 1, strings.Count(got, "FUNCTION_RESULT_BEGIN tx1 499"))
				assert.Equal(t, 0, strings.Count(got, "FUNCTION_RESULT_BEGIN tx1 200"))
			},
		},
		"payload pre-admission cancel emits 499 and skips handler": {
			run: func(t *testing.T, mgr *Manager, in *chanInput, out *safeBuffer) {
				var calls atomic.Int32
				mgr.Register("fn", func(Function) { calls.Add(1) })

				cancel, done := startFlowManager(t, mgr)
				defer cancel()

				in.ch <- payloadStartCmd("tx1", "fn")
				in.ch <- "payload line"
				in.ch <- "FUNCTION_CANCEL tx1"
				close(in.ch)
				waitForDone(t, done)

				waitForSubstring(t, out.String, "FUNCTION_RESULT_BEGIN tx1 499", time.Second)
				assert.EqualValues(t, 0, calls.Load())
			},
		},
		"duplicate active uid is ignored without corrupting lane progression": {
			run: func(t *testing.T, mgr *Manager, in *chanInput, out *safeBuffer) {
				mgr.workerCount = 2
				started := make(chan struct{}, 1)
				tx2Started := make(chan struct{}, 1)
				release := make(chan struct{})
				var calls atomic.Int32

				mgr.Register("fn", func(fn Function) {
					calls.Add(1)

					if fn.UID == "tx1" {
						started <- struct{}{}
						<-release
						mgr.respUID(fn.UID, 200, "ok")
						return
					}

					if fn.UID == "tx2" {
						select {
						case tx2Started <- struct{}{}:
						default:
						}
					}

					mgr.respUID(fn.UID, 200, "ok")
				})

				cancel, done := startFlowManager(t, mgr)
				defer cancel()

				in.ch <- functionLine("tx1", "fn")
				<-started
				// Same-key request should stay queued until tx1 completes.
				in.ch <- functionLine("tx2", "fn")
				// Duplicate active UID must not advance lanes or finalize tx1.
				in.ch <- functionLine("tx1", "fn")

				select {
				case <-tx2Started:
					t.Fatal("tx2 started before tx1 was released")
				default:
				}

				close(release)
				select {
				case <-tx2Started:
				case <-time.After(time.Second):
					t.Fatal("tx2 did not start after tx1 was released")
				}
				close(in.ch)
				waitForDone(t, done)

				got := out.String()
				assert.Equal(t, 1, strings.Count(got, "FUNCTION_RESULT_BEGIN tx1 200"))
				assert.Equal(t, 1, strings.Count(got, "FUNCTION_RESULT_BEGIN tx2 200"))
				assert.Equal(t, 0, strings.Count(got, "FUNCTION_RESULT_BEGIN tx1 409"))
				assert.EqualValues(t, 2, calls.Load())
			},
		},
		"duplicate tombstoned uid is ignored without extra terminal output": {
			run: func(t *testing.T, mgr *Manager, in *chanInput, out *safeBuffer) {
				var calls atomic.Int32

				mgr.Register("fn", func(fn Function) {
					calls.Add(1)
					mgr.respUID(fn.UID, 200, "ok")
				})

				cancel, done := startFlowManager(t, mgr)
				defer cancel()

				in.ch <- functionLine("tx1", "fn")
				waitForSubstring(t, out.String, "FUNCTION_RESULT_BEGIN tx1 200", time.Second)

				// Re-send same UID while tombstone is still active.
				in.ch <- functionLine("tx1", "fn")
				close(in.ch)
				waitForDone(t, done)

				got := out.String()
				assert.Equal(t, 1, strings.Count(got, "FUNCTION_RESULT_BEGIN tx1 200"))
				assert.Equal(t, 0, strings.Count(got, "FUNCTION_RESULT_BEGIN tx1 409"))
				assert.EqualValues(t, 1, calls.Load())
			},
		},
		"queue full is rejected with 503": {
			run: func(t *testing.T, mgr *Manager, in *chanInput, out *safeBuffer) {
				mgr.queueSize = 1
				mgr.workerCount = 1
				started := make(chan struct{}, 1)
				release := make(chan struct{})

				mgr.Register("fn", func(fn Function) {
					if fn.UID == "tx1" {
						started <- struct{}{}
						<-release
					}
					mgr.respUID(fn.UID, 200, "ok")
				})

				cancel, done := startFlowManager(t, mgr)
				defer cancel()

				in.ch <- functionLine("tx1", "fn")
				<-started
				in.ch <- functionLine("tx2", "fn")
				in.ch <- functionLine("tx3", "fn")
				waitForSubstring(t, out.String, "FUNCTION_RESULT_BEGIN tx3 503", time.Second)

				close(release)
				close(in.ch)
				waitForDone(t, done)
			},
		},
		"panic in handler emits 500": {
			run: func(t *testing.T, mgr *Manager, in *chanInput, out *safeBuffer) {
				mgr.Register("fn", func(Function) { panic("boom") })

				cancel, done := startFlowManager(t, mgr)
				defer cancel()

				in.ch <- functionLine("tx1", "fn")
				close(in.ch)
				waitForDone(t, done)

				waitForSubstring(t, out.String, "FUNCTION_RESULT_BEGIN tx1 500", time.Second)
			},
		},
		"cancel for unknown uid is no-op": {
			run: func(t *testing.T, mgr *Manager, in *chanInput, out *safeBuffer) {
				cancel, done := startFlowManager(t, mgr)
				defer cancel()

				in.ch <- "FUNCTION_CANCEL unknown"
				close(in.ch)
				waitForDone(t, done)

				assert.Equal(t, 0, strings.Count(out.String(), "FUNCTION_RESULT_BEGIN"))
			},
		},
		"cancel after completion is no-op": {
			run: func(t *testing.T, mgr *Manager, in *chanInput, out *safeBuffer) {
				mgr.Register("fn", func(fn Function) {
					mgr.respUID(fn.UID, 200, "done")
				})

				cancel, done := startFlowManager(t, mgr)
				defer cancel()

				in.ch <- functionLine("tx1", "fn")
				waitForSubstring(t, out.String, "FUNCTION_RESULT_BEGIN tx1 200", time.Second)
				in.ch <- "FUNCTION_CANCEL tx1"
				close(in.ch)
				waitForDone(t, done)

				got := out.String()
				assert.Equal(t, 1, strings.Count(got, "FUNCTION_RESULT_BEGIN tx1 200"))
				assert.Equal(t, 0, strings.Count(got, "FUNCTION_RESULT_BEGIN tx1 499"))
			},
		},
		"stdin close uses canceling shutdown and force-finalizes unresolved requests": {
			run: func(t *testing.T, mgr *Manager, in *chanInput, out *safeBuffer) {
				mgr.shutdownDrainTimeout = 50 * time.Millisecond
				started := make(chan struct{}, 1)
				block := make(chan struct{})

				mgr.Register("fn", func(fn Function) {
					started <- struct{}{}
					<-block
					mgr.respUID(fn.UID, 200, "late")
				})

				cancel, done := startFlowManager(t, mgr)
				defer cancel()

				in.ch <- functionLine("tx1", "fn")
				<-started
				close(in.ch)
				waitForDone(t, done)

				got := out.String()
				assert.Equal(t, 1, strings.Count(got, "FUNCTION_RESULT_BEGIN tx1 499"))
				assert.Equal(t, 0, strings.Count(got, "FUNCTION_RESULT_BEGIN tx1 200"))
			},
		},
		"late terminal output after shutdown is dropped by tombstone guard": {
			run: func(t *testing.T, mgr *Manager, in *chanInput, out *safeBuffer) {
				mgr.shutdownDrainTimeout = 50 * time.Millisecond
				started := make(chan struct{}, 1)
				block := make(chan struct{})
				doneResp := make(chan struct{})

				mgr.Register("fn", func(fn Function) {
					started <- struct{}{}
					<-block
					mgr.respUID(fn.UID, 200, "late")
					close(doneResp)
				})

				cancel, done := startFlowManager(t, mgr)
				defer cancel()

				in.ch <- functionLine("tx1", "fn")
				<-started
				close(in.ch)
				waitForDone(t, done)

				// Release handler after manager has already force-finalized and returned.
				close(block)
				select {
				case <-doneResp:
				case <-time.After(time.Second):
					t.Fatal("timed out waiting for late handler response")
				}

				got := out.String()
				assert.Equal(t, 1, strings.Count(got, "FUNCTION_RESULT_BEGIN tx1 499"))
				assert.Equal(t, 0, strings.Count(got, "FUNCTION_RESULT_BEGIN tx1 200"))
			},
		},
		"shutdown finalizes unresolved awaiting_result even when workers are drained": {
			run: func(t *testing.T, mgr *Manager, in *chanInput, out *safeBuffer) {
				mgr.shutdownDrainTimeout = 250 * time.Millisecond
				mgr.awaitingWarnDelay = time.Second
				returned := make(chan struct{}, 1)

				mgr.Register("fn", func(Function) {
					// Return without terminal response.
					returned <- struct{}{}
				})

				cancel, done := startFlowManager(t, mgr)
				defer cancel()

				in.ch <- functionLine("tx1", "fn")
				<-returned
				waitForCondition(t, time.Second, func() bool {
					mgr.invStateMux.Lock()
					defer mgr.invStateMux.Unlock()
					rec, ok := mgr.invState["tx1"]
					return ok && rec != nil && rec.state == stateAwaitingResult
				}, "tx1 reaches awaiting_result before shutdown")

				close(in.ch)
				waitForDone(t, done)

				got := out.String()
				assert.Equal(t, 1, strings.Count(got, "FUNCTION_RESULT_BEGIN tx1 499"))
				assert.Equal(t, 0, strings.Count(got, "FUNCTION_RESULT_BEGIN tx1 200"))
			},
		},
	}

	for name, tc := range tests {
		t.Run(name, func(t *testing.T) {
			mgr, out := newFlowManager()
			in := &chanInput{ch: make(chan string, 16)}
			mgr.input = in
			tc.run(t, mgr, in, out)
		})
	}
}

func TestManager_InvocationStateScenarios(t *testing.T) {
	tests := map[string]struct {
		run func(t *testing.T, mgr *Manager, in *chanInput, out *safeBuffer)
	}{
		"worker return transitions to awaiting_result until terminal output": {
			run: func(t *testing.T, mgr *Manager, in *chanInput, out *safeBuffer) {
				mgr.workerCount = 1
				returned := make(chan struct{}, 1)
				releaseResponse := make(chan struct{})

				mgr.Register("fn", func(fn Function) {
					go func() {
						<-releaseResponse
						mgr.respUID(fn.UID, 200, "ok")
					}()
					returned <- struct{}{}
				})

				cancel, done := startFlowManager(t, mgr)
				defer cancel()

				in.ch <- functionLine("tx1", "fn")
				<-returned

				waitForCondition(t, time.Second, func() bool {
					mgr.invStateMux.Lock()
					defer mgr.invStateMux.Unlock()
					rec, ok := mgr.invState["tx1"]
					return ok && rec != nil && rec.state == stateAwaitingResult
				}, "transaction tx1 reaches awaiting_result state")

				close(releaseResponse)
				waitForSubstring(t, out.String, "FUNCTION_RESULT_BEGIN tx1 200", time.Second)

				waitForCondition(t, time.Second, func() bool {
					mgr.invStateMux.Lock()
					defer mgr.invStateMux.Unlock()
					_, ok := mgr.invState["tx1"]
					return !ok
				}, "transaction tx1 is removed from active state after finalization")

				close(in.ch)
				waitForDone(t, done)
			},
		},
	}

	for name, tc := range tests {
		t.Run(name, func(t *testing.T) {
			mgr, out := newFlowManager()
			in := &chanInput{ch: make(chan string, 16)}
			mgr.input = in
			tc.run(t, mgr, in, out)
		})
	}
}

func TestManager_tryFinalize(t *testing.T) {
	tests := map[string]struct {
		run func(t *testing.T, mgr *Manager)
	}{
		"first finalization wins and tombstone blocks immediate reuse": {
			run: func(t *testing.T, mgr *Manager) {
				calls := 0
				ok := mgr.tryFinalize("uid1", "test.first", func() { calls++ })
				require.True(t, ok)
				assert.Equal(t, 1, calls)

				ok = mgr.tryFinalize("uid1", "test.late", func() { calls++ })
				require.False(t, ok)
				assert.Equal(t, 1, calls)

				admitted := mgr.trySetInvocationState("uid1", stateQueued, func() {}, "fn")
				assert.Equal(t, invocationAdmissionDuplicateTombstone, admitted)

				mgr.invStateMux.Lock()
				mgr.tombstones["uid1"] = time.Now().Add(-time.Second)
				mgr.invStateMux.Unlock()

				admitted = mgr.trySetInvocationState("uid1", stateQueued, func() {}, "fn")
				assert.Equal(t, invocationAdmissionAccepted, admitted)
			},
		},
		"empty uid or nil emitter is rejected": {
			run: func(t *testing.T, mgr *Manager) {
				assert.False(t, mgr.tryFinalize("", "source", func() {}))
				assert.False(t, mgr.tryFinalize("uid1", "source", nil))
			},
		},
	}

	for name, tc := range tests {
		t.Run(name, func(t *testing.T) {
			tc.run(t, NewManager())
		})
	}
}

func TestManager_WorkerPoolConcurrencyBound(t *testing.T) {
	tests := map[string]struct {
		workerCount int
		input       []string
		register    func(t *testing.T, mgr *Manager, current, maxSeen *atomic.Int32)
		assertions  func(t *testing.T, maxSeen int32)
	}{
		"same key is serialized": {
			workerCount: 4,
			input: []string{
				functionLine("tx-1", "fn"),
				functionLine("tx-2", "fn"),
				functionLine("tx-3", "fn"),
				functionLine("tx-4", "fn"),
			},
			register: func(t *testing.T, mgr *Manager, current, maxSeen *atomic.Int32) {
				t.Helper()
				mgr.Register("fn", func(fn Function) {
					c := current.Add(1)
					for {
						prev := maxSeen.Load()
						if c <= prev || maxSeen.CompareAndSwap(prev, c) {
							break
						}
					}

					time.Sleep(30 * time.Millisecond)
					current.Add(-1)
					mgr.respUID(fn.UID, 200, "ok")
				})
			},
			assertions: func(t *testing.T, maxSeen int32) {
				t.Helper()
				assert.Equal(t, int32(1), maxSeen)
			},
		},
		"different keys run concurrently": {
			workerCount: 4,
			input: []string{
				functionLine("tx-a1", "fnA"),
				functionLine("tx-b1", "fnB"),
				functionLine("tx-a2", "fnA"),
				functionLine("tx-b2", "fnB"),
			},
			register: func(t *testing.T, mgr *Manager, current, maxSeen *atomic.Int32) {
				t.Helper()
				registerFn := func(name string) {
					mgr.Register(name, func(fn Function) {
						c := current.Add(1)
						for {
							prev := maxSeen.Load()
							if c <= prev || maxSeen.CompareAndSwap(prev, c) {
								break
							}
						}

						time.Sleep(40 * time.Millisecond)
						current.Add(-1)
						mgr.respUID(fn.UID, 200, "ok")
					})
				}
				registerFn("fnA")
				registerFn("fnB")
			},
			assertions: func(t *testing.T, maxSeen int32) {
				t.Helper()
				assert.GreaterOrEqual(t, maxSeen, int32(2))
				assert.LessOrEqual(t, maxSeen, int32(4))
			},
		},
	}

	for name, tc := range tests {
		t.Run(name, func(t *testing.T) {
			mgr, out := newFlowManager()
			mgr.workerCount = tc.workerCount
			mgr.queueSize = len(tc.input) + tc.workerCount
			in := &chanInput{ch: make(chan string, len(tc.input)+tc.workerCount)}
			mgr.input = in

			var current atomic.Int32
			var maxSeen atomic.Int32

			tc.register(t, mgr, &current, &maxSeen)

			cancel, done := startFlowManager(t, mgr)
			defer cancel()

			for _, line := range tc.input {
				in.ch <- line
			}
			close(in.ch)
			waitForDone(t, done)

			got := out.String()
			assert.Equal(t, len(tc.input), strings.Count(got, "FUNCTION_RESULT_BEGIN tx-"))
			tc.assertions(t, maxSeen.Load())
		})
	}
}

func TestManager_DispatchInvocationStopping(t *testing.T) {
	tests := map[string]struct {
		run func(t *testing.T, mgr *Manager, out *safeBuffer)
	}{
		"stopping manager rejects dispatch without tracking state": {
			run: func(t *testing.T, mgr *Manager, out *safeBuffer) {
				mgr.Register("fn", func(Function) { t.Fatal("handler should not execute when manager is stopping") })
				mgr.setStopping(true)

				fn := &Function{UID: "tx-stop", Name: "fn"}
				mgr.dispatchInvocation(context.Background(), fn)

				waitForSubstring(t, out.String, "FUNCTION_RESULT_BEGIN tx-stop 503", time.Second)

				mgr.invStateMux.Lock()
				_, ok := mgr.invState["tx-stop"]
				mgr.invStateMux.Unlock()
				assert.False(t, ok)
			},
		},
	}

	for name, tc := range tests {
		t.Run(name, func(t *testing.T) {
			mgr, out := newFlowManager()
			tc.run(t, mgr, out)
		})
	}
}
