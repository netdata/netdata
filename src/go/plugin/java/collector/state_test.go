// SPDX-License-Identifier: GPL-3.0-or-later

package collector

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/json"
	"net"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/netdata/netdata/go/plugins/plugin/java/protocol"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	collectorv1 "go.opentelemetry.io/proto/otlp/collector/metrics/v1"
	"google.golang.org/protobuf/encoding/protojson"
	"google.golang.org/protobuf/proto"
)

type fixtureHelper struct {
	mu        sync.Mutex
	processes []protocol.Process
	requests  []protocol.AttachRequest
	outcome   string
}

func (h *fixtureHelper) discover(context.Context) ([]protocol.Process, error) {
	h.mu.Lock()
	defer h.mu.Unlock()
	return append([]protocol.Process(nil), h.processes...), nil
}
func (h *fixtureHelper) attach(_ context.Context, req protocol.AttachRequest) protocol.AttachResult {
	h.mu.Lock()
	defer h.mu.Unlock()
	h.requests = append(h.requests, req)
	return protocol.AttachResult{Status: h.outcome, Detail: "fixture outcome"}
}
func (h *fixtureHelper) count() int { h.mu.Lock(); defer h.mu.Unlock(); return len(h.requests) }

func runtimeFixture(t *testing.T) (RuntimeConfig, *fixtureHelper) {
	t.Helper()
	root := t.TempDir()
	proc := filepath.Join(root, "proc")
	require.NoError(t, os.MkdirAll(filepath.Join(proc, "sys/kernel/random"), 0700))
	require.NoError(t, os.WriteFile(filepath.Join(proc, "sys/kernel/random/boot_id"), []byte("boot-one\n"), 0600))
	return RuntimeConfig{StateDir: filepath.Join(root, "state"), ProcDir: proc}, &fixtureHelper{
		processes: []protocol.Process{{PID: 42, StartTime: 9876, BootID: "boot-one", UID: 10001, GID: 10001, Application: "checkout", Location: "Host", Eligible: true}}, outcome: protocol.Attached,
	}
}
func startFixture(t *testing.T, runtime RuntimeConfig, helper *fixtureHelper) (*Collector, func()) {
	t.Helper()
	c := New(runtime)
	c.helper = helper
	c.isTerminal = func() bool { return false }
	require.NoError(t, c.Init(context.Background()))
	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan error, 1)
	ready := make(chan struct{})
	go func() { done <- c.Run(ctx, func() { close(ready) }) }()
	select {
	case <-ready:
	case err := <-done:
		t.Fatalf("run failed: %v", err)
	case <-time.After(5 * time.Second):
		t.Fatal("run not ready")
	}
	var once sync.Once
	stop := func() {
		once.Do(func() {
			cancel()
			select {
			case err := <-done:
				require.NoError(t, err)
			case <-time.After(5 * time.Second):
				t.Fatal("run did not stop")
			}
		})
	}
	t.Cleanup(stop)
	return c, stop
}
func readState(t *testing.T, runtime RuntimeConfig) persistentState {
	t.Helper()
	b, err := os.ReadFile(filepath.Join(runtime.StateDir, "state.json"))
	require.NoError(t, err)
	var s persistentState
	require.NoError(t, json.Unmarshal(b, &s))
	return s
}

func TestRunPersistsReceiverAndDoesNotReattach(t *testing.T) {
	for _, outcome := range []string{protocol.Attached, protocol.Unknown, protocol.Blocked} {
		t.Run(outcome, func(t *testing.T) {
			runtime, h := runtimeFixture(t)
			h.outcome = outcome
			c, stop := startFixture(t, runtime, h)
			require.Eventually(t, func() bool { rows := c.Applications(); return len(rows) == 1 && rows[0].Status != "Attaching" }, 5*time.Second, 10*time.Millisecond)
			stop()
			before := readState(t, runtime)
			require.Equal(t, 1, h.count())
			require.Len(t, before.Attempts, 1)
			a := before.Attempts[h.processes[0].Instance()]
			require.Len(t, a.Token, 64)
			assert.Equal(t, outcome, a.Status)
			candidate := New(runtime)
			require.NoError(t, candidate.Init(context.Background()))
			require.Equal(t, before, readState(t, runtime), "Init must not mutate or load active ownership")
			second, stopSecond := startFixture(t, runtime, h)
			require.Eventually(t, func() bool { return len(second.Applications()) == 1 }, 5*time.Second, 10*time.Millisecond)
			assert.Equal(t, 1, h.count(), "process identity must never be reinjected after restart")
			assert.Equal(t, before, readState(t, runtime))
			second.mu.Lock()
			_, admitted := second.credentials[sha256.Sum256([]byte(a.Token))]
			second.mu.Unlock()
			assert.True(t, admitted, "original credentials must remain valid")
			stopSecond()
		})
	}
}

func TestRunLockAndPortOwnership(t *testing.T) {
	runtime, h := runtimeFixture(t)
	c, stop := startFixture(t, runtime, h)
	require.Eventually(t, func() bool { return len(c.Applications()) == 1 }, 5*time.Second, 10*time.Millisecond)
	other := New(runtime)
	other.helper = h
	other.isTerminal = func() bool { return false }
	require.ErrorContains(t, other.Run(context.Background(), func() {}), "another java.plugin")
	stop()
	s := readState(t, runtime)
	listener, err := net.Listen("tcp4", "127.0.0.1:"+strconv.Itoa(int(s.Port)))
	require.NoError(t, err)
	defer listener.Close()
	require.ErrorContains(t, other.Run(context.Background(), func() {}), "bind persisted")
	assert.Equal(t, 1, h.count())
}

func TestRunRefusesDamagedStateAndInteractiveMutation(t *testing.T) {
	runtime, h := runtimeFixture(t)
	require.NoError(t, os.MkdirAll(runtime.StateDir, 0700))
	path := filepath.Join(runtime.StateDir, "state.json")
	require.NoError(t, os.WriteFile(path, []byte("{damaged"), 0600))
	c := New(runtime)
	c.helper = h
	c.isTerminal = func() bool { return false }
	require.ErrorContains(t, c.Run(context.Background(), func() {}), "attachment state")
	c.isTerminal = func() bool { return true }
	require.ErrorContains(t, c.Run(context.Background(), func() {}), "interactive")
	b, err := os.ReadFile(path)
	require.NoError(t, err)
	assert.Equal(t, "{damaged", string(b))
	assert.Zero(t, h.count())
}

func TestExcludedUnsupportedAndRetiredProcesses(t *testing.T) {
	runtime, h := runtimeFixture(t)
	h.processes = append(h.processes, protocol.Process{PID: 43, StartTime: 1, BootID: "boot-one", Application: "root-app", Location: "Host", Reason: "root-owned JVMs are unsupported"})
	c := New(runtime)
	c.helper = h
	c.ExcludeApplications = []string{"checkout"}
	j, err := openJournal(runtime.StateDir, "boot-one")
	require.NoError(t, err)
	defer j.close()
	j.state.Port = 54321
	require.NoError(t, c.scan(context.Background(), j))
	assert.Zero(t, h.count())
	require.Len(t, c.Applications(), 2)
	assert.Equal(t, "Excluded", c.Applications()[0].Status)
	assert.Equal(t, "Unsupported", c.Applications()[1].Status)
	c.ExcludeApplications = nil
	require.NoError(t, c.scan(context.Background(), j))
	assert.Equal(t, 1, h.count())
	a := j.state.Attempts[h.processes[0].Instance()]
	c.ExcludeApplications = []string{"checkout"}
	require.NoError(t, c.scan(context.Background(), j))
	assert.Empty(t, c.credentials)
	c.ExcludeApplications = nil
	require.NoError(t, c.scan(context.Background(), j))
	assert.Equal(t, 1, h.count())
	assert.Equal(t, a.Token, j.state.Attempts[h.processes[0].Instance()].Token)
	h.processes = nil
	require.NoError(t, c.scan(context.Background(), j))
	assert.Empty(t, c.Applications())
	assert.Empty(t, c.credentials)
	assert.Empty(t, j.state.Attempts)
}

func TestReceiverAuthenticatesBeforeDecodeAndBindsIdentity(t *testing.T) {
	c := New(RuntimeConfig{})
	a := attempt{Process: protocol.Process{PID: 42, StartTime: 1, BootID: "test", Application: "fixture-service"}, Token: strings.Repeat("a", 64)}
	c.admit(a)
	send := func(token string, data []byte) int {
		r := httptest.NewRequest(http.MethodPost, "/v1/metrics", bytes.NewReader(data))
		r.Header.Set("Content-Type", "application/x-protobuf")
		r.Header.Set("x-netdata-java-token", token)
		w := httptest.NewRecorder()
		c.receiver()(w, r)
		return w.Code
	}
	assert.Equal(t, http.StatusUnauthorized, send("", []byte("bad protobuf")))
	assert.Equal(t, http.StatusUnauthorized, send(strings.Repeat("b", 64), []byte("bad protobuf")))
	data, err := os.ReadFile("../ingest/testdata/java-2.32.0.json")
	require.NoError(t, err)
	var req collectorv1.ExportMetricsServiceRequest
	require.NoError(t, protojson.Unmarshal(data, &req))
	wire, err := proto.Marshal(&req)
	require.NoError(t, err)
	assert.Equal(t, http.StatusForbidden, send(a.Token, wire), "another process identity cannot be forged by an admitted application")
	c.revoke(a.Process.Instance())
	assert.Equal(t, http.StatusUnauthorized, send(a.Token, wire))
}
