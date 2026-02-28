// SPDX-License-Identifier: GPL-3.0-or-later

package functions

import (
	"bytes"
	"context"
	"fmt"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/netdata/netdata/go/plugins/pkg/netdataapi"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

type chanInput struct {
	ch chan string
}

func (m *chanInput) lines() chan string {
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
	return fmt.Sprintf(`FUNCTION %s 10 "%s" 0xFFFF "method=api,role=test"`, uid, name)
}

func payloadStartCmd(uid, name string) string {
	return fmt.Sprintf(`FUNCTION_PAYLOAD %s 10 "%s" 0xFFFF "method=api,role=test" application/json`, uid, name)
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

func countSubstring(s, substr string) int {
	count := 0
	for {
		idx := strings.Index(s, substr)
		if idx < 0 {
			return count
		}
		count++
		s = s[idx+len(substr):]
	}
}

func TestManager_CancelQueued_Emits499AndSkipsExecution(t *testing.T) {
	mgr, out := newFlowManager()
	in := &chanInput{ch: make(chan string, 8)}
	mgr.input = in

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

	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()

	done := make(chan struct{})
	go func() { defer close(done); mgr.Run(ctx, nil) }()

	in.ch <- functionLine("tx1", "fn")
	<-started
	in.ch <- functionLine("tx2", "fn")
	in.ch <- "FUNCTION_CANCEL tx2"
	close(release)
	close(in.ch)
	<-done

	waitForSubstring(t, out.String, "FUNCTION_RESULT_BEGIN tx2 499", time.Second)
	mu.Lock()
	defer mu.Unlock()
	assert.Equal(t, []string{"tx1"}, executed)
}

func TestManager_CancelRunning_Fallback499Once(t *testing.T) {
	mgr, out := newFlowManager()
	mgr.cancelFallbackDelay = 50 * time.Millisecond
	in := &chanInput{ch: make(chan string, 8)}
	mgr.input = in

	started := make(chan struct{}, 1)
	release := make(chan struct{})

	mgr.Register("fn", func(fn Function) {
		if fn.UID == "tx1" {
			started <- struct{}{}
			<-release
		}
	})

	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()
	done := make(chan struct{})
	go func() { defer close(done); mgr.Run(ctx, nil) }()

	in.ch <- functionLine("tx1", "fn")
	<-started
	in.ch <- "FUNCTION_CANCEL tx1"

	waitForSubstring(t, out.String, "FUNCTION_RESULT_BEGIN tx1 499", time.Second)

	close(release)
	close(in.ch)
	<-done

	got := out.String()
	assert.Equal(t, 1, countSubstring(got, "FUNCTION_RESULT_BEGIN tx1 499"))
}

func TestManager_PreAdmissionPayloadCancel_Emits499(t *testing.T) {
	mgr, out := newFlowManager()
	in := &chanInput{ch: make(chan string, 8)}
	mgr.input = in

	calls := 0
	mgr.Register("fn", func(Function) {
		calls++
	})

	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()
	done := make(chan struct{})
	go func() { defer close(done); mgr.Run(ctx, nil) }()

	in.ch <- payloadStartCmd("tx1", "fn")
	in.ch <- "payload line"
	in.ch <- "FUNCTION_CANCEL tx1"
	close(in.ch)
	<-done

	waitForSubstring(t, out.String, "FUNCTION_RESULT_BEGIN tx1 499", time.Second)
	assert.Equal(t, 0, calls)
}

func TestManager_DuplicateUID_Rejected409(t *testing.T) {
	mgr, out := newFlowManager()
	in := &chanInput{ch: make(chan string, 8)}
	mgr.input = in

	started := make(chan struct{}, 1)
	release := make(chan struct{})
	calls := 0
	mgr.Register("fn", func(fn Function) {
		calls++
		if fn.UID == "tx1" {
			started <- struct{}{}
			<-release
		}
	})

	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()
	done := make(chan struct{})
	go func() { defer close(done); mgr.Run(ctx, nil) }()

	in.ch <- functionLine("tx1", "fn")
	<-started
	in.ch <- functionLine("tx1", "fn")
	close(release)
	close(in.ch)
	<-done

	waitForSubstring(t, out.String, "FUNCTION_RESULT_BEGIN tx1 409", time.Second)
	assert.Equal(t, 1, calls)
}

func TestManager_QueueFull_Rejected503(t *testing.T) {
	mgr, out := newFlowManager()
	mgr.queueSize = 1
	mgr.workerCount = 1
	in := &chanInput{ch: make(chan string, 8)}
	mgr.input = in

	started := make(chan struct{}, 1)
	release := make(chan struct{})
	mgr.Register("fn", func(fn Function) {
		if fn.UID == "tx1" {
			started <- struct{}{}
			<-release
		}
	})

	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()
	done := make(chan struct{})
	go func() { defer close(done); mgr.Run(ctx, nil) }()

	in.ch <- functionLine("tx1", "fn")
	<-started
	in.ch <- functionLine("tx2", "fn")
	in.ch <- functionLine("tx3", "fn")

	waitForSubstring(t, out.String, "FUNCTION_RESULT_BEGIN tx3 503", time.Second)

	close(release)
	close(in.ch)
	<-done
}

func TestManager_PanicRecovery_Emits500(t *testing.T) {
	mgr, out := newFlowManager()
	in := &chanInput{ch: make(chan string, 4)}
	mgr.input = in

	mgr.Register("fn", func(Function) {
		panic("boom")
	})

	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()
	done := make(chan struct{})
	go func() { defer close(done); mgr.Run(ctx, nil) }()

	in.ch <- functionLine("tx1", "fn")
	close(in.ch)
	<-done

	waitForSubstring(t, out.String, "FUNCTION_RESULT_BEGIN tx1 500", time.Second)
}

func TestManager_TryFinalize_TombstoneReuseWindow(t *testing.T) {
	mgr := NewManager()

	calls := 0
	ok := mgr.TryFinalize("uid1", "test.first", func() { calls++ })
	require.True(t, ok)
	assert.Equal(t, 1, calls)

	ok = mgr.TryFinalize("uid1", "test.late", func() { calls++ })
	require.False(t, ok)
	assert.Equal(t, 1, calls)

	cancel := func() {}
	admitted := mgr.trySetInvocationState("uid1", stateQueued, cancel)
	assert.False(t, admitted)

	mgr.invStateMux.Lock()
	mgr.tombstones["uid1"] = time.Now().Add(-time.Second)
	mgr.invStateMux.Unlock()

	admitted = mgr.trySetInvocationState("uid1", stateQueued, cancel)
	assert.True(t, admitted)
}
