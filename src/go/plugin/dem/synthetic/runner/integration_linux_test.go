//go:build linux

// SPDX-License-Identifier: GPL-3.0-or-later
package runner_test

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/netdata/netdata/go/plugins/pkg/funcapi"
	syntheticfunctions "github.com/netdata/netdata/go/plugins/plugin/dem/synthetic/functions"
	synthetichistory "github.com/netdata/netdata/go/plugins/plugin/dem/synthetic/history"
	"github.com/netdata/netdata/go/plugins/plugin/dem/synthetic/query"
	runtime "github.com/netdata/netdata/go/plugins/plugin/dem/synthetic/runtime"

	demjournal "github.com/netdata/netdata/go/plugins/plugin/dem/journal"
	"github.com/netdata/netdata/go/plugins/plugin/dem/synthetic"
	"github.com/netdata/netdata/go/plugins/plugin/dem/synthetic/artifacts"
	"github.com/netdata/netdata/go/plugins/plugin/dem/synthetic/runner"
	"github.com/netdata/netdata/go/plugins/plugin/go.d/pkg/ndexec"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"golang.org/x/sys/unix"
)

// TestPreparedLinuxRuntime uses the selected production installation, actual
// acquired process/execution claims, nd-run, artifacts and journal. It never
// injects private component paths or downloads a runtime.
func TestPreparedLinuxRuntime(t *testing.T) {
	names := []string{"DEM_RUNNER_TEST_MANIFEST", "DEM_RUNNER_TEST_STORE", "DEM_RUNNER_TEST_HELPER"}
	values := make([]string, len(names))
	for i, name := range names {
		values[i] = os.Getenv(name)
		if values[i] == "" {
			t.Skip("real Linux validation requires " + name)
		}
	}
	require.NotZero(t, os.Geteuid(), "browser runtime must run unprivileged")
	restore := ndexec.SetRunnerPathsForTests(values[2], "")
	defer restore()
	raw, err := os.ReadFile(values[0])
	require.NoError(t, err)
	var manifest runtime.Manifest
	require.NoError(t, json.Unmarshal(raw, &manifest))
	require.NoError(t, manifest.Validate())
	store, err := runtime.Open(runtime.DefaultConfig(values[1]))
	require.NoError(t, err)
	consumer, err := store.AcquireSelected()
	require.NoError(t, err)
	require.Equal(t, manifest, consumer.Manifest(), "selected release must match the supplied trusted descriptor")
	expectedIdentity := synthetic.RuntimeIdentity{Version: manifest.Version, SHA256: manifest.Archive.SHA256, Contract: manifest.Contract}
	base, err := os.MkdirTemp("", "dem-real-runtime-")
	require.NoError(t, err)
	safe := true
	history, err := demjournal.Open(context.Background(), filepath.Join(base, "history"))
	require.NoError(t, err)
	captures, err := artifacts.Open(filepath.Join(base, "captures"))
	require.NoError(t, err)
	defer func() {
		if safe {
			assert.NoError(t, consumer.Close())
			assert.NoError(t, store.Close())
			assert.NoError(t, history.Close())
			assert.NoError(t, captures.Close())
			assert.NoError(t, os.RemoveAll(base))
		}
	}()
	observedHistory := &identityHistory{Store: synthetichistory.NewStore(history)}
	engine := runner.New(selectedRuntime{consumer}, observedHistory, captures)
	verifyIdentity := func(t *testing.T, run synthetic.Run) {
		t.Helper()
		require.Equal(t, &expectedIdentity, run.Runtime)
		records := observedHistory.records[run.ID]
		require.Len(t, records, 2, "both start and completion must carry the actual acquired identity")
		require.Zero(t, records[0].CompletedUS)
		require.Equal(t, &expectedIdentity, records[0].Runtime)
		require.Equal(t, &expectedIdentity, records[1].Runtime)
		retained, err := observedHistory.GetRun(context.Background(), run.JobID, run.ID)
		require.NoError(t, err)
		require.Equal(t, &expectedIdentity, retained.Runtime)
		handler := syntheticfunctions.New(query.New(nil, observedHistory, captures, nil))
		response := handler.HandleRaw(context.Background(), funcapi.RawMethodRequest{Method: "synthetics-run", Args: []string{"job_id:" + run.JobID, "run_id:" + run.ID}, Permissions: "0x7b"})
		require.NotNil(t, response.RawResponse, response.Message)
		require.Equal(t, &expectedIdentity, response.RawResponse["run"].(map[string]any)["runtime"])
	}
	require.NoError(t, engine.Check(context.Background(), synthetic.Journey))

	var mu sync.Mutex
	var descriptors []int
	var procIdentities []procIdentity
	var registered chan struct{}
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/pids" {
			w.Header().Set("Content-Type", "text/html")
			_, _ = io.WriteString(
				w,
				"<!doctype html><html lang=en><head><title>DEM synthetic fixture</title><meta name=viewport content='width=device-width,initial-scale=1'></head><body><h1>Prepared runtime fixture</h1><button>Checkout</button></body></html>",
			)
			return
		}
		var pids []int
		if err := json.NewDecoder(io.LimitReader(r.Body, 4096)).Decode(&pids); err != nil {
			http.Error(w, err.Error(), 400)
			return
		}
		mu.Lock()
		defer mu.Unlock()
		for _, pid := range pids {
			fd, err := unix.PidfdOpen(pid, 0)
			if errors.Is(err, unix.ESRCH) {
				continue
			}
			if errors.Is(err, unix.ENOSYS) {
				identity, readErr := readProcIdentity(pid)
				if errors.Is(readErr, os.ErrNotExist) {
					continue
				}
				if readErr != nil {
					http.Error(w, readErr.Error(), 500)
					return
				}
				procIdentities = append(procIdentities, identity)
				continue
			}
			if err != nil {
				http.Error(w, err.Error(), 500)
				return
			}
			descriptors = append(descriptors, fd)
		}
		if registered != nil {
			select {
			case registered <- struct{}{}:
			default:
			}
		}
		w.WriteHeader(http.StatusNoContent)
	}))
	defer server.Close()
	urlJSON, _ := json.Marshal(server.URL)
	pidURLJSON, _ := json.Marshal(server.URL + "/pids")
	prefix := "import {test,expect} from '@playwright/test';\n"
	browserPrefix := prefix + "test.beforeEach(async({page,browser})=>{const cdp=await browser.newBrowserCDPSession();const info=await cdp.send('SystemInfo.getProcessInfo');await cdp.detach();const response=await fetch(" + string(
		pidURLJSON,
	) + ",{method:'POST',body:JSON.stringify(info.processInfo.map(p=>p.id))});expect(response.ok,await response.text()).toBeTruthy();});\n"
	verifyGone := func(t *testing.T) {
		t.Helper()
		mu.Lock()
		defer mu.Unlock()
		require.Positive(t, len(descriptors)+len(procIdentities), "fixture must observe owned Chromium processes")
		t.Logf("completion observers: pidfd=%d proc-starttime=%d", len(descriptors), len(procIdentities))
		for _, fd := range descriptors {
			poll := []unix.PollFd{{Fd: int32(fd), Events: unix.POLLIN}}
			n, err := unix.Poll(poll, 0)
			assert.NoError(t, err)
			assert.Equal(t, 1, n, "process must have exited before Execute releases admission")
			assert.NotZero(t, poll[0].Revents&unix.POLLIN)
			assert.NoError(t, unix.Close(fd))
		}
		for _, original := range procIdentities {
			current, err := readProcIdentity(original.pid)
			if errors.Is(err, os.ErrNotExist) {
				continue
			}
			require.NoError(t, err)
			assert.NotEqual(
				t,
				original.starttime,
				current.starttime,
				"original process still exists after drain (pid=%d state=%s)",
				original.pid,
				current.state,
			)
		}
		procIdentities = nil
		descriptors = nil
	}
	run := func(t *testing.T, ctx context.Context, name, script string, timeout time.Duration, capture bool) synthetic.Execution {
		t.Helper()
		execution := engine.Execute(
			ctx,
			synthetic.Request{
				Kind:    synthetic.Journey,
				Name:    name,
				Script:  script,
				Timeout: timeout,
				Capture: capture,
			},
			nil,
		)
		if !execution.Drained {
			safe = false
			t.Fatalf("tree not drained: %+v", execution.Run)
		}
		require.Empty(t, execution.Run.HistoryError)
		verifyIdentity(t, execution.Run)
		t.Logf("%s: %s (%s)", name, execution.Run.Outcome, execution.Run.Error)
		return execution
	}
	t.Run("browser-success-and-detached-leaf", func(t *testing.T) {
		script := browserPrefix + "import {spawn} from 'node:child_process';test('checkout',async({page})=>{const child=spawn(process.execPath,['-e','setInterval(()=>{},1000)'],{detached:true,stdio:'ignore'});child.unref();const response=await fetch(" + string(
			pidURLJSON,
		) + ",{method:'POST',body:JSON.stringify([child.pid])});expect(response.ok,await response.text()).toBeTruthy();await page.goto(" + string(
			urlJSON,
		) + ");await expect(page).toHaveTitle('DEM synthetic fixture');});"
		got := run(t, context.Background(), "browser-pass", script, 30*time.Second, false)
		require.Equal(t, synthetic.Success, got.Run.Outcome)
		verifyGone(t)
	})
	t.Run("assertion-failure-PNG", func(t *testing.T) {
		script := browserPrefix + "test('fails',async({page})=>{await page.goto(" + string(
			urlJSON,
		) + ");expect(1).toBe(2)});"
		got := run(t, context.Background(), "browser-fail", script, 30*time.Second, true)
		require.Equal(t, synthetic.Failed, got.Run.Outcome)
		verifyGone(t)
		require.NotEmpty(t, got.Run.Artifacts)
		assert.Equal(t, "available", got.Run.CaptureState)
		_, raw, err := captures.Fetch(context.Background(), got.Run.JobID, got.Run.ID, got.Run.Artifacts[0].ID)
		require.NoError(t, err)
		assert.Equal(t, "\x89PNG\r\n\x1a\n", string(raw[:8]))
	})
	t.Run("strict-ordinary-coverage", func(t *testing.T) {
		for _, source := range []string{"", "test('pass',async()=>{});test.skip('skip',async()=>{});", "test('expected',async()=>{test.fail();expect(1).toBe(2)});"} {
			got := run(t, context.Background(), "coverage", prefix+source, 15*time.Second, false)
			assert.Equal(t, synthetic.Inconclusive, got.Run.Outcome)
		}
	})
	t.Run("single-entry-helpers-fixtures-and-worker-policy", func(t *testing.T) {
		input := filepath.Join(base, "script-source")
		require.NoError(t, os.MkdirAll(input, 0700))
		require.NoError(t, os.WriteFile(filepath.Join(input, "helper.ts"), []byte(`export const label='fixture ready';`), 0600))
		require.NoError(t, os.WriteFile(filepath.Join(input, "fixtures.ts"), []byte(`import {test as base} from '@playwright/test';
export const test=base.extend({message:async({},use)=>{await use('fixture ready')}});
export {expect} from '@playwright/test';`), 0600))
		entry := filepath.Join(input, "chosen.spec.ts")
		require.NoError(t, os.WriteFile(entry, []byte(`import {test,expect} from './fixtures';
import {label} from './helper';
test('selected entry',async({message},info)=>{
 expect(message).toBe(label);
 expect(info.config.workers).toBe(1);
 expect(info.config.fullyParallel).toBe(false);
 expect(info.project.retries).toBe(0);
 expect(info.retry).toBe(0);
 expect(process.env.DEM_RUNTIME_INHERITED).toBeUndefined();
 expect(process.env.DEM_SECRET_TEST).toBe('private-fixture-value');
 console.log('private-fixture-value');
});`), 0600))
		require.NoError(t, os.WriteFile(filepath.Join(input, "unselected.spec.ts"), []byte(`throw new Error('unselected entry must never load');`), 0600))
		t.Setenv("DEM_RUNTIME_INHERITED", "must-not-reach-worker")
		got := engine.Execute(context.Background(), synthetic.Request{Kind: synthetic.Journey, Name: "file-contract", ScriptPath: entry, Timeout: 30 * time.Second, Secrets: map[string]string{"DEM_SECRET_TEST": "private-fixture-value"}}, nil)
		if !got.Drained {
			safe = false
			t.Fatal("file-contract tree not drained")
		}
		require.Equal(t, synthetic.Success, got.Run.Outcome, got.Run.Error)
		require.Equal(t, &synthetic.TestCounts{Declared: 1, Passed: 1}, got.Run.Tests)
		require.Equal(t, "disabled", got.Run.CaptureState)
		require.Empty(t, got.Run.Artifacts)
		encoded, err := json.Marshal(got.Run)
		require.NoError(t, err)
		require.NotContains(t, string(encoded), "private-fixture-value")
		verifyIdentity(t, got.Run)
	})
	t.Run("capture-disabled-browser-failure", func(t *testing.T) {
		got := run(t, context.Background(), "no-capture", browserPrefix+"test('fails',async({page})=>{await page.goto("+string(urlJSON)+");expect(1).toBe(2)});", 30*time.Second, false)
		require.Equal(t, synthetic.Failed, got.Run.Outcome)
		require.Equal(t, "disabled", got.Run.CaptureState)
		require.Empty(t, got.Run.Artifacts)
		verifyGone(t)
	})
	t.Run("setup-inability", func(t *testing.T) {
		unavailable := runner.New(nil, synthetichistory.NewStore(history), captures)
		require.Error(t, unavailable.Check(context.Background(), synthetic.Journey))
		got := unavailable.Execute(
			context.Background(),
			synthetic.Request{
				Kind:    synthetic.Journey,
				Name:    "unavailable",
				Script:  prefix + "test('unused',async()=>{});",
				Timeout: time.Second,
			},
			nil,
		)
		assert.True(t, got.Drained)
		assert.Equal(t, synthetic.Error, got.Run.Outcome)
		assert.Nil(t, got.Run.DurationMS)
	})
	for _, scenario := range []struct{ name, body string }{{"busy-loop", "test('busy',async({page})=>{while(true){}});"}, {"hanging-hook", "test.afterEach(async()=>{await new Promise(()=>{})});test('hook',async({page})=>{});"}} {
		t.Run(scenario.name, func(t *testing.T) {
			got := run(t, context.Background(), scenario.name, browserPrefix+scenario.body, 5*time.Second, false)
			assert.Equal(t, synthetic.Timeout, got.Run.Outcome)
			verifyGone(t)
			next := run(
				t,
				context.Background(),
				"next-after-timeout",
				prefix+"test('next',async()=>{expect(1).toBe(1)});",
				15*time.Second,
				false,
			)
			assert.Equal(t, synthetic.Success, next.Run.Outcome)
		})
	}
	t.Run("lifecycle-cancel", func(t *testing.T) {
		mu.Lock()
		registered = make(chan struct{}, 1)
		signal := registered
		mu.Unlock()
		ctx, cancel := context.WithCancel(context.Background())
		defer cancel()
		done := make(chan synthetic.Execution, 1)
		go func() {
			done <- engine.Execute(ctx, synthetic.Request{
				Kind:    synthetic.Journey,
				Name:    "cancel",
				Script:  browserPrefix + "test('cancel',async({page})=>{await new Promise(()=>{})});",
				Timeout: 30 * time.Second,
			}, nil)
		}()
		select {
		case <-signal:
			cancel()
		case <-time.After(20 * time.Second):
			cancel()
			t.Error("browser registration did not arrive")
		}
		got := <-done
		if !got.Drained {
			safe = false
			t.Fatal("cancelled tree not drained")
		}
		assert.Equal(t, synthetic.Cancelled, got.Run.Outcome)
		verifyIdentity(t, got.Run)
		verifyGone(t)
		mu.Lock()
		registered = nil
		mu.Unlock()
		next := run(
			t,
			context.Background(),
			"next-after-cancel",
			prefix+"test('next',async()=>{});",
			15*time.Second,
			false,
		)
		assert.Equal(t, synthetic.Success, next.Run.Outcome)
	})
	t.Run("Lighthouse-desktop-report", func(t *testing.T) {
		require.NoError(t, engine.Check(context.Background(), synthetic.Lighthouse))
		got := engine.Execute(
			context.Background(),
			synthetic.Request{
				Kind:    synthetic.Lighthouse,
				Name:    "desktop",
				URL:     server.URL,
				Timeout: 90 * time.Second,
				Capture: true,
			},
			nil,
		)
		if !got.Drained {
			safe = false
			t.Fatal("Lighthouse tree not drained")
		}
		require.Equal(t, synthetic.Success, got.Run.Outcome, got.Run.Error)
		verifyIdentity(t, got.Run)
		require.NotNil(t, got.Run.Metrics)
		require.NotNil(t, got.Run.Metrics.Performance)
		assert.GreaterOrEqual(t, *got.Run.Metrics.Performance, 0.0)
		assert.LessOrEqual(t, *got.Run.Metrics.Performance, 100.0)
		require.NotNil(t, got.Run.Metrics.CLS)
		assert.Zero(t, *got.Run.Metrics.CLS)
		require.Len(t, got.Run.Artifacts, 1)
		_, raw, err := captures.Fetch(context.Background(), got.Run.JobID, got.Run.ID, got.Run.Artifacts[0].ID)
		require.NoError(t, err)
		assert.Contains(t, string(raw), "<!doctype html>")
		assert.Regexp(t, `"formFactor"\s*:\s*"desktop"`, string(raw))
	})
}

// Some amd64 emulators do not implement pidfd_open even when seccomp allows it.
// This observer never signals a PID. A matching identity, including a zombie,
// fails completion; only absence or a different starttime proves it is gone.
type procIdentity struct {
	pid       int
	starttime string
	state     string
}

func readProcIdentity(pid int) (procIdentity, error) {
	raw, err := os.ReadFile(filepath.Join("/proc", strconv.Itoa(pid), "stat"))
	if err != nil {
		return procIdentity{}, err
	}
	close := strings.LastIndexByte(string(raw), ')')
	if close < 0 {
		return procIdentity{}, fmt.Errorf("invalid proc stat for pid %d", pid)
	}
	fields := strings.Fields(string(raw[close+1:]))
	// Fields start at state (3); process starttime is field 22.
	if len(fields) <= 19 {
		return procIdentity{}, fmt.Errorf("short proc stat for pid %d", pid)
	}
	if _, err := strconv.ParseUint(fields[19], 10, 64); err != nil {
		return procIdentity{}, err
	}
	return procIdentity{
		pid:       pid,
		starttime: fields[19],
		state:     fields[0],
	}, nil
}

// This adapter exposes only the public execution contract of the acquired
// consumer. It cannot select a different generation or inject private paths.
type selectedRuntime struct{ consumer *runtime.Consumer }

func (s selectedRuntime) BeginUse() (runner.Use, error) {
	use, err := s.consumer.BeginUse()
	if err != nil {
		return nil, err
	}
	m := s.consumer.Manifest()
	return selectedUse{use: use, identity: synthetic.RuntimeIdentity{Version: m.Version, SHA256: m.Archive.SHA256, Contract: m.Contract}}, nil
}

type selectedUse struct {
	use      *runtime.Use
	identity synthetic.RuntimeIdentity
}

func (u selectedUse) Paths() runner.Paths {
	return runner.Paths{Node: u.use.Paths.Node, EntryPoint: u.use.Paths.EntryPoint, Identity: u.identity}
}
func (u selectedUse) Completion() *ndexec.TreeCompletion { return u.use.Completion }
func (u selectedUse) Finish(drained bool) error          { return u.use.Finish(drained) }

type identityHistory struct {
	*synthetichistory.Store
	records map[string][]synthetic.Run
}

func (h *identityHistory) AppendRun(ctx context.Context, phase string, run synthetic.Run) (bool, error) {
	attempted, err := h.Store.AppendRun(ctx, phase, run)
	if err == nil {
		if h.records == nil {
			h.records = make(map[string][]synthetic.Run)
		}
		h.records[run.ID] = append(h.records[run.ID], synthetic.CloneRun(run))
	}
	return attempted, err
}
