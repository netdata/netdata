//go:build linux || darwin || freebsd

// SPDX-License-Identifier: GPL-3.0-or-later
package runner

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/netdata/netdata/go/plugins/plugin/dem/synthetic"
	"github.com/netdata/netdata/go/plugins/plugin/dem/synthetic/artifacts"
	"github.com/netdata/netdata/go/plugins/plugin/go.d/pkg/ndexec"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"golang.org/x/sys/unix"
)

type fakeTree struct{ done chan ndexec.TreeResult }

func (p *fakeTree) Wait() ndexec.TreeResult { return <-p.done }

type fakeHistory struct {
	mu        sync.Mutex
	phases    []string
	runs      []synthetic.Run
	err       error
	syncErr   error
	syncCalls int
}

func (h *fakeHistory) AppendRun(_ context.Context, phase string, run synthetic.Run) (bool, error) {
	h.mu.Lock()
	defer h.mu.Unlock()
	h.phases = append(h.phases, phase)
	h.runs = append(h.runs, run)
	return true, h.err
}
func (h *fakeHistory) Sync(context.Context) error {
	h.mu.Lock()
	defer h.mu.Unlock()
	h.syncCalls++
	return h.syncErr
}

type observedArtifacts struct {
	*artifacts.Store
	finalized atomic.Int32
}

func (a *observedArtifacts) Finalize(
	ctx context.Context,
	id string,
	c []synthetic.Capture,
) ([]synthetic.Artifact, error) {
	a.finalized.Add(1)
	return a.Store.Finalize(ctx, id, c)
}

type payload func(context.Context, []string, []byte, *os.File, *os.File) ndexec.TreeResult

func starter(t *testing.T, body payload) startFunc {
	t.Helper()
	return func(ctx context.Context, opts ndexec.ProcessOptions, _ string, args ...string) (treeProcess, error) {
		dup := func(file *os.File) *os.File {
			fd, err := unix.Dup(int(file.Fd()))
			require.NoError(t, err)
			return os.NewFile(uintptr(fd), "test-owned-child-pipe")
		}
		input, output, stderr := dup(opts.Stdin), dup(opts.Stdout), dup(opts.Stderr)
		process := &fakeTree{
			done: make(chan ndexec.TreeResult, 1),
		}
		go func() {
			defer input.Close()
			defer output.Close()
			defer stderr.Close()
			raw, _ := io.ReadAll(input)
			process.done <- body(ctx, args, raw, output, stderr)
		}()
		return process, nil
	}
}
func testEngine(t *testing.T) (*Engine, *fakeHistory, *observedArtifacts) {
	t.Helper()
	base := t.TempDir()
	bin, err := os.Executable()
	require.NoError(t, err)
	assets, err := filepath.Abs("assets")
	require.NoError(t, err)
	for name, pin := range packagePins {
		dir := filepath.Join(base, "deps", "node_modules", name)
		require.NoError(t, os.MkdirAll(dir, 0700))
		require.NoError(t, os.WriteFile(filepath.Join(dir, "package.json"), []byte(`{"version":"`+pin+`"}`), 0600))
	}
	store, err := artifacts.Open(filepath.Join(base, "artifacts"))
	require.NoError(t, err)
	t.Cleanup(func() {
		require.NoError(t, store.Close())
		entries, err := os.ReadDir(filepath.Join(base, "artifacts", "work"))
		require.NoError(t, err)
		// All fake-payload pipe owners have joined before this cleanup runs.
		for _, entry := range entries {
			alias := filepath.Join("/tmp", "nd-dem-"+entry.Name())
			if target, err := os.Readlink(alias); err == nil &&
				target == filepath.Join(base, "artifacts", "work", entry.Name()) {
				require.NoError(t, os.Remove(alias))
			}
		}
	})
	history := &fakeHistory{}
	owner := &observedArtifacts{
		Store: store,
	}
	engine := New(
		Config{
			NodePath:         bin,
			BrowserPath:      bin,
			AssetsPath:       assets,
			DependenciesPath: filepath.Join(base, "deps"),
		},
		history,
		owner,
	)
	engine.platform = func() error { return nil }
	return engine, history, owner
}
func journey() synthetic.Request {
	return synthetic.Request{
		Kind:    synthetic.Journey,
		Name:    "checkout",
		Script:  "synthetic source",
		Timeout: time.Second,
	}
}
func emit(writer io.Writer, value any) {
	raw, _ := json.Marshal(value)
	_, _ = fmt.Fprintln(writer, string(raw))
}
func success() reporterResult {
	return reporterResult{
		Status: synthetic.Success,
		Tests: &synthetic.TestCounts{
			Declared: 1,
			Passed:   1,
		},
		CaptureState: "disabled",
	}
}
func resultPayload(result reporterResult) payload {
	return func(_ context.Context, _ []string, _ []byte, out, _ *os.File) ndexec.TreeResult {
		emit(out, frame{
			Type:   "result",
			Result: &result,
		})
		return ndexec.TreeResult{
			Drained: true,
		}
	}
}

func TestSuccessfulExecutionJoinsAndFinalizes(t *testing.T) {
	e, h, a := testEngine(t)
	e.start = starter(t, resultPayload(success()))
	var states []string
	execution := e.Execute(context.Background(), journey(), func(state string) { states = append(states, state) })
	require.True(t, execution.Drained)
	assert.Equal(t, synthetic.Success, execution.Run.Outcome)
	require.NotNil(t, execution.Run.DurationMS)
	assert.Equal(t, []string{"waiting", "running"}, states)
	assert.Equal(t, int32(1), a.finalized.Load())
	assert.Zero(t, a.Stats().ProtectedBytes)
	assert.Equal(t, []string{"start", "complete"}, h.phases)
	assert.Equal(t, 2, h.syncCalls)
	assert.Nil(t, h.runs[0].Tests)
	assert.Nil(t, h.runs[0].DurationMS)
	assert.Zero(t, h.runs[0].CompletedUS)
	assert.Equal(t, execution.Run.Tests, h.runs[1].Tests)
	execution.Run.Tests.Passed = 0
	assert.Equal(t, 1, h.runs[1].Tests.Passed)
}
func TestRealFailureSurvivesNonzeroExit(t *testing.T) {
	e, _, _ := testEngine(t)
	result := reporterResult{
		Status: synthetic.Failed,
		Tests: &synthetic.TestCounts{
			Declared: 2,
			Failed:   1,
			Skipped:  1,
		},
		Error:        "assertion failed",
		CaptureState: "disabled",
	}
	e.start = starter(t, func(ctx context.Context, args []string, raw []byte, out, stderr *os.File) ndexec.TreeResult {
		resultPayload(result)(ctx, args, raw, out, stderr)
		return ndexec.TreeResult{
			Drained: true,
			Err:     errors.New("exit status 1"),
		}
	})
	got := e.Execute(context.Background(), journey(), nil)
	assert.Equal(t, synthetic.Failed, got.Run.Outcome)
	assert.Equal(t, 1, got.Run.Tests.Failed)
}
func TestMalformedReporterCancelsAndDrains(t *testing.T) {
	for _, which := range []string{"invalid", "duplicate", "oversize", "contradictory-success", "negative-metric", "missing"} {
		t.Run(which, func(t *testing.T) {
			e, _, a := testEngine(t)
			cancelled := make(chan struct{})
			e.start = starter(t, func(ctx context.Context, _ []string, _ []byte, out, _ *os.File) ndexec.TreeResult {
				switch which {
				case "invalid":
					fmt.Fprintln(out, "not JSON")
				case "duplicate":
					r := success()
					emit(out, frame{
						Type:   "result",
						Result: &r,
					})
					emit(out, frame{
						Type:   "result",
						Result: &r,
					})
				case "oversize":
					fmt.Fprintln(out, strings.Repeat("x", FrameMaxBytes+1))
				case "contradictory-success":
					r := success()
					r.Tests.Skipped = 1
					emit(out, frame{
						Type:   "result",
						Result: &r,
					})
				case "negative-metric":
					negative := -1.0
					emit(
						out,
						frame{
							Type: "result",
							Result: &reporterResult{
								Status: synthetic.Success,
								Metrics: &synthetic.LabMetrics{
									CLS: &negative,
								},
								CaptureState: "disabled",
							},
						},
					)
				case "missing":
					return ndexec.TreeResult{
						Drained: true,
					}
				}
				select {
				case <-ctx.Done():
					close(cancelled)
				case <-time.After(time.Second):
					return ndexec.TreeResult{
						Drained: true,
						Err:     errors.New("cancellation was not requested"),
					}
				}
				return ndexec.TreeResult{
					Drained: true,
				}
			})
			r := journey()
			if which == "negative-metric" {
				r = synthetic.Request{
					Kind:    synthetic.Lighthouse,
					Name:    "audit",
					URL:     "https://example.test",
					Timeout: time.Second,
				}
			}
			got := e.Execute(context.Background(), r, nil)
			assert.True(t, got.Drained)
			assert.Equal(t, synthetic.Error, got.Run.Outcome)
			assert.NotEmpty(t, got.Run.Error)
			assert.Equal(t, int32(1), a.finalized.Load())
			if which != "missing" {
				select {
				case <-cancelled:
				default:
					t.Error("parser did not request cancellation")
				}
			}
		})
	}
}
func TestDeadlineKeepsIncrementalEvidence(t *testing.T) {
	e, _, a := testEngine(t)
	e.start = starter(t, func(ctx context.Context, _ []string, _ []byte, out, _ *os.File) ndexec.TreeResult {
		emit(
			out,
			frame{
				Type: "event",
				Event: &synthetic.Event{
					Kind:  "suite",
					Title: "Discovered 3 tests",
					Phase: "discovery",
				},
			},
		)
		emit(
			out,
			frame{
				Type: "event",
				Event: &synthetic.Event{
					Kind:           "test",
					TestID:         "one",
					Phase:          "end",
					Status:         "passed",
					ExpectedStatus: "passed",
				},
			},
		)
		emit(
			out,
			frame{
				Type: "event",
				Event: &synthetic.Event{
					Kind:   "step",
					Title:  "pending navigation",
					Phase:  "pw:api",
					Status: "running",
				},
			},
		)
		<-ctx.Done()
		return ndexec.TreeResult{
			Drained: true,
			Err:     ctx.Err(),
		}
	})
	r := journey()
	r.Timeout = 30 * time.Millisecond
	got := e.Execute(context.Background(), r, nil)
	require.True(t, got.Drained)
	assert.Equal(t, synthetic.Timeout, got.Run.Outcome)
	require.NotNil(t, got.Run.Tests)
	assert.Equal(t, 3, got.Run.Tests.Declared)
	assert.Equal(t, 1, got.Run.Tests.Passed)
	assert.Equal(t, 2, got.Run.Tests.NotRun)
	assert.Len(t, got.Run.Events, 3)
	assert.Equal(t, int32(1), a.finalized.Load())
}
func TestQueuedCancellationNeverStartsAChild(t *testing.T) {
	e, _, _ := testEngine(t)
	started := make(chan struct{})
	release := make(chan struct{})
	var calls atomic.Int32
	e.start = starter(t, func(ctx context.Context, args []string, raw []byte, out, stderr *os.File) ndexec.TreeResult {
		calls.Add(1)
		close(started)
		<-release
		return resultPayload(success())(ctx, args, raw, out, stderr)
	})
	done := make(chan synthetic.Execution, 1)
	go func() { done <- e.Execute(context.Background(), journey(), nil) }()
	<-started
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	got := e.Execute(ctx, journey(), nil)
	assert.Equal(t, synthetic.Cancelled, got.Run.Outcome)
	assert.Nil(t, got.Run.DurationMS)
	assert.Zero(t, got.Run.StartedUS)
	assert.Equal(t, int32(1), calls.Load())
	expiry, cancelExpiry := context.WithTimeout(context.Background(), 5*time.Millisecond)
	defer cancelExpiry()
	expired := e.Execute(expiry, journey(), nil)
	assert.Equal(t, synthetic.Cancelled, expired.Run.Outcome, "queued lifecycle expiry is not a workflow timeout")
	assert.Zero(t, expired.Run.StartedUS)
	assert.Nil(t, expired.Run.DurationMS)
	assert.Equal(t, int32(1), calls.Load())
	close(release)
	assert.Equal(t, synthetic.Success, (<-done).Run.Outcome)
}
func TestUnverifiedDrainPoisonsBeforeReaderJoin(t *testing.T) {
	e, _, a := testEngine(t)
	held := make(chan struct{})
	e.start = func(_ context.Context, opts ndexec.ProcessOptions, _ string, _ ...string) (treeProcess, error) {
		outFD, err := unix.Dup(int(opts.Stdout.Fd()))
		require.NoError(t, err)
		out := os.NewFile(uintptr(outFD), "unknown-test-survivor")
		errFD, err := unix.Dup(int(opts.Stderr.Fd()))
		require.NoError(t, err)
		stderr := os.NewFile(uintptr(errFD), "unknown-test-survivor-stderr")
		inFD, err := unix.Dup(int(opts.Stdin.Fd()))
		require.NoError(t, err)
		input := os.NewFile(uintptr(inFD), "unknown-test-survivor-input")
		t.Cleanup(func() { close(held); _ = out.Close(); _ = stderr.Close(); _ = input.Close() })
		p := &fakeTree{
			done: make(chan ndexec.TreeResult, 1),
		}
		p.done <- ndexec.TreeResult{
			Drained: false,
			Err:     errors.New("missing completion frame"),
		}
		return p, nil
	}
	done := make(chan synthetic.Execution, 1)
	go func() { done <- e.Execute(context.Background(), journey(), nil) }()
	select {
	case err := <-e.Fatal():
		assert.ErrorIs(t, err, ndexec.ErrTreeNotDrained)
	case <-time.After(time.Second):
		t.Fatal("fatal was blocked on surviving stdio")
	}
	select {
	case got := <-done:
		assert.False(t, got.Drained)
		assert.Zero(t, got.Run.CompletedUS)
	case <-time.After(time.Second):
		t.Fatal("owned readers did not join")
	}
	assert.Zero(t, a.finalized.Load())
	assert.Equal(t, 1, len(e.admission))
	assert.False(t, e.Execute(context.Background(), journey(), nil).Drained)
}
func TestSecretInputRedactionAndDiagnosticBounds(t *testing.T) {
	e, h, _ := testEngine(t)
	request := journey()
	request.Timeout = 10 * time.Second
	request.Secrets = map[string]string{"DEM_SECRET_token": "long-secret", "DEM_SECRET_short": "x"}
	e.start = starter(t, func(_ context.Context, args []string, raw []byte, out, _ *os.File) ndexec.TreeResult {
		assert.NotContains(t, strings.Join(args, " "), "long-secret")
		var sent wireRequest
		require.NoError(t, json.Unmarshal(raw, &sent))
		assert.Equal(t, request.Secrets, sent.Secrets)
		for i := 0; i < EventLimit+4; i++ {
			emit(
				out,
				frame{
					Type: "event",
					Event: &synthetic.Event{
						Kind:    "stdout",
						Message: "long-secret x " + strings.Repeat("界", TextLimit+10) + "\n",
					},
				},
			)
		}
		r := success()
		emit(out, frame{
			Type:   "result",
			Result: &r,
		})
		return ndexec.TreeResult{
			Drained: true,
		}
	})
	got := e.Execute(context.Background(), request, nil)
	assert.Equal(t, synthetic.Success, got.Run.Outcome)
	assert.Len(t, got.Run.Events, EventLimit)
	assert.Equal(t, 4, got.Run.DroppedEvents)
	for _, event := range got.Run.Events {
		assert.NotContains(t, event.Message, "long-secret")
		assert.NotContains(t, event.Message, " x ")
		assert.LessOrEqual(t, len([]rune(event.Message)), TextLimit)
		assert.Equal(t, "stdout", event.Kind)
	}
	raw, err := json.Marshal(h.runs)
	require.NoError(t, err)
	assert.NotContains(t, string(raw), "long-secret")
	assert.Contains(t, string(raw), "checkout")
}
func TestHistoryUncertaintyIsNotReplayed(t *testing.T) {
	e, h, _ := testEngine(t)
	h.err = errors.New("uncertain write")
	h.syncErr = errors.New("sync failed")
	e.start = starter(t, resultPayload(success()))
	got := e.Execute(context.Background(), journey(), nil)
	assert.Equal(t, synthetic.Success, got.Run.Outcome)
	assert.Contains(t, got.Run.HistoryError, "append outcome uncertain")
	assert.Contains(t, got.Run.HistoryError, "sync failed")
	assert.Equal(t, []string{"start", "complete"}, h.phases)
	assert.Contains(t, h.runs[1].HistoryError, "start history")
}
func TestOversizeRequestNeverStartsWorkflow(t *testing.T) {
	e, _, a := testEngine(t)
	e.start = func(context.Context, ndexec.ProcessOptions, string, ...string) (treeProcess, error) {
		t.Error("oversize request started a child")
		return nil, errors.New("unexpected start")
	}
	r := journey()
	r.Script = strings.Repeat("x", FrameMaxBytes)
	got := e.Execute(context.Background(), r, nil)
	assert.Equal(t, synthetic.Error, got.Run.Outcome)
	assert.Nil(t, got.Run.DurationMS)
	assert.Contains(t, got.Run.Error, "4 MiB")
	assert.Equal(t, int32(1), a.finalized.Load())
}
func TestCheckVersionProbeIsSupervisedAndDoesNotLoadWorkflow(t *testing.T) {
	if os.Geteuid() == 0 {
		t.Skip("the prepared runtime explicitly requires unprivileged plugin identity")
	}
	e, _, a := testEngine(t)
	calls := 0
	e.start = starter(t, func(_ context.Context, args []string, raw []byte, out, _ *os.File) ndexec.TreeResult {
		calls++
		assert.Empty(t, raw)
		assert.Equal(t, "-e", args[0])
		assert.NotContains(t, strings.Join(args, " "), "run.mjs")
		if calls == 1 {
			fmt.Fprintf(out, `{"version":"24.20.0","uid":%d,"gid":%d}`, os.Geteuid(), os.Getegid())
		} else {
			fmt.Fprint(out, "Chromium 153.0.8010.12")
		}
		return ndexec.TreeResult{
			Drained: true,
		}
	})
	require.NoError(t, e.Check(context.Background(), synthetic.Journey))
	assert.Equal(t, 2, calls)
	assert.Zero(t, a.finalized.Load())
}

func TestHistorySnapshotOwnsNullableMetricsAndEventDurations(t *testing.T) {
	e, h, _ := testEngine(t)
	e.start = starter(t, func(_ context.Context, _ []string, _ []byte, out, _ *os.File) ndexec.TreeResult {
		duration := 3.5
		emit(
			out,
			frame{
				Type: "event",
				Event: &synthetic.Event{
					Kind:       "audit",
					Phase:      "complete",
					DurationMS: &duration,
				},
			},
		)
		performance, fcp, lcp, tbt, si, cls := 97.0, 12.0, 13.0, 0.0, 55.0, 0.0
		result := reporterResult{
			Status: synthetic.Success,
			Metrics: &synthetic.LabMetrics{
				Performance: &performance,
				FCPMS:       &fcp,
				LCPMS:       &lcp,
				TBTMS:       &tbt,
				SIMS:        &si,
				CLS:         &cls,
			},
			CaptureState: "disabled",
		}
		emit(out, frame{
			Type:   "result",
			Result: &result,
		})
		return ndexec.TreeResult{
			Drained: true,
		}
	})
	got := e.Execute(
		context.Background(),
		synthetic.Request{
			Kind:    synthetic.Lighthouse,
			Name:    "snapshot",
			URL:     "https://example.test",
			Timeout: time.Second,
		},
		nil,
	)
	require.Equal(t, synthetic.Success, got.Run.Outcome)
	require.Len(t, h.runs, 2)
	m := got.Run.Metrics
	for _, value := range []*float64{m.Performance, m.FCPMS, m.LCPMS, m.TBTMS, m.SIMS, m.CLS} {
		*value = 1234
	}
	*got.Run.Events[0].DurationMS = 1234
	saved := h.runs[1]
	assert.Equal(t, 97.0, *saved.Metrics.Performance)
	assert.Equal(t, 12.0, *saved.Metrics.FCPMS)
	assert.Equal(t, 13.0, *saved.Metrics.LCPMS)
	assert.Zero(t, *saved.Metrics.TBTMS)
	assert.Equal(t, 55.0, *saved.Metrics.SIMS)
	assert.Zero(t, *saved.Metrics.CLS)
	assert.Equal(t, 3.5, *saved.Events[0].DurationMS)
}

func TestCancellationRedactsLateBootstrapAndProtocolOutputBeforeHistory(t *testing.T) {
	e, h, _ := testEngine(t)
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	request := journey()
	request.Timeout = 10 * time.Second
	request.Secrets = map[string]string{"DEM_SECRET_TOKEN": "fixture-only-secret-0123456789"}
	e.start = starter(
		t,
		func(processCtx context.Context, _ []string, _ []byte, out, stderr *os.File) ndexec.TreeResult {
			fmt.Fprint(stderr, "fixture-only-secret-")
			emit(
				out,
				frame{
					Type: "event",
					Event: &synthetic.Event{
						Kind:    "stdout",
						Phase:   "worker",
						Message: "fixture-only-secret-",
					},
				},
			)
			emit(
				out,
				frame{
					Type: "event",
					Event: &synthetic.Event{
						Kind:   "step",
						Title:  "observable evidence",
						Status: "failed",
					},
				},
			)
			cancel()
			<-processCtx.Done()
			// Pipes may still produce bytes during the verified-drain wait.
			fmt.Fprint(stderr, "0123456789\nunfinished fixture-only-secret-")
			emit(
				out,
				frame{
					Type: "event",
					Event: &synthetic.Event{
						Kind:    "stdout",
						Phase:   "worker",
						Message: "0123456789\nunfinished fixture-only-secret-",
					},
				},
			)
			return ndexec.TreeResult{
				Drained: true,
				Err:     processCtx.Err(),
			}
		},
	)
	got := e.Execute(ctx, request, nil)
	require.True(t, got.Drained)
	assert.Equal(t, synthetic.Cancelled, got.Run.Outcome)
	assert.Contains(t, diagnosticText(got.Run.Events), "[REDACTED]")
	raw, err := json.Marshal(h.runs)
	require.NoError(t, err)
	assert.NotContains(t, string(raw), "fixture-only-secret-")
	assert.Contains(t, string(raw), "observable evidence")
}

func TestLighthouseTargetRedactionBeforeHistory(t *testing.T) {
	e, h, _ := testEngine(t)
	request := journey()
	request.Kind = synthetic.Lighthouse
	request.URL = "https://example.org/?client%5Fsecret=private-query-value&view=home"
	e.start = starter(t, func(_ context.Context, _ []string, _ []byte, out, _ *os.File) ndexec.TreeResult {
		r := success()
		emit(out, frame{
			Type:   "result",
			Result: &r,
		})
		return ndexec.TreeResult{
			Drained: true,
		}
	})
	got := e.Execute(context.Background(), request, nil)
	assert.NotContains(t, got.Run.Target, "private-query-value")
	for _, run := range h.runs {
		assert.NotContains(t, run.Target, "private-query-value")
	}
}

func TestWhitespaceScriptRejectedBeforeExecution(t *testing.T) {
	e, _, _ := testEngine(t)
	e.start = func(context.Context, ndexec.ProcessOptions, string, ...string) (treeProcess, error) {
		t.Error("invalid script started a child")
		return nil, errors.New("unexpected process start")
	}
	request := journey()
	request.Script = " \t\n\u2003"
	got := e.Execute(context.Background(), request, nil)
	require.True(t, got.Drained)
	assert.Equal(t, synthetic.Error, got.Run.Outcome)
	assert.Equal(t, "script must not be blank", got.Run.Error)
}
