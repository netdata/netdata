//go:build linux || darwin || freebsd

// SPDX-License-Identifier: GPL-3.0-or-later

package runner

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"image"
	"image/png"
	"os"
	"path/filepath"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/netdata/netdata/go/plugins/plugin/dem/synthetic"
	"github.com/netdata/netdata/go/plugins/plugin/dem/synthetic/artifacts"
	"github.com/netdata/netdata/go/plugins/plugin/go.d/pkg/ndexec"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestSuccessfulExecutionJoinsAndFinalizes(t *testing.T) {
	e, h, a := testEngine(t)
	e.start = starter(t, resultPayload(success()))
	var states []synthetic.JobState
	execution := e.ExecuteScheduled(context.Background(), journey(), func(state synthetic.JobState) { states = append(states, state) }, nil)
	require.True(t, execution.Drained)
	assert.Equal(t, synthetic.Success, execution.Run.Outcome)
	require.NotNil(t, execution.Run.DurationMS)
	assert.Equal(t, []synthetic.JobState{synthetic.JobWaiting, synthetic.JobRunning}, states)
	assert.Equal(t, int32(1), a.finalized.Load())
	page, err := a.Catalogue(context.Background(), artifacts.Filter{
		Limit: 1,
	})
	require.NoError(t, err)
	require.NotNil(t, page.Status.Inventory.Workspace.Bytes)
	assert.Zero(t, *page.Status.Inventory.Workspace.Bytes)
	assert.Equal(t, []string{"start", "complete"}, h.phases)
	assert.Equal(t, 2, h.syncCalls)
	assert.Nil(t, h.runs[0].Tests)
	assert.Nil(t, h.runs[0].DurationMS)
	assert.Zero(t, h.runs[0].CompletedUS)
	assert.Positive(t, h.runs[0].StartedUS)
	assert.Equal(t, h.runs[0].StartedUS, h.runs[1].StartedUS)
	assert.Equal(t, h.runs[0].StartedUS, execution.Run.StartedUS)
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
	got := e.execute(context.Background(), journey())
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
					fmt.Fprintln(out, strings.Repeat("x", frameMaxBytes+1))
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
			got := e.execute(context.Background(), r)
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
					Kind:          "suite",
					Title:         "localized discovery label",
					DeclaredTests: intPointer(3),
					Phase:         "discovery",
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
	got := e.execute(context.Background(), r)
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
	go func() { done <- e.execute(context.Background(), journey()) }()
	<-started
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	got := e.execute(ctx, journey())
	assert.Equal(t, synthetic.Cancelled, got.Run.Outcome)
	assert.Nil(t, got.Run.DurationMS)
	assert.Zero(t, got.Run.StartedUS)
	assert.Equal(t, int32(1), calls.Load())
	expiry, cancelExpiry := context.WithTimeout(context.Background(), 5*time.Millisecond)
	defer cancelExpiry()
	expired := e.execute(expiry, journey())
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
	e.start = func(_ context.Context, opts ndexec.TreeOptions, _ string, _ ...string) (treeProcess, error) {
		pipes, err := duplicatePipes(opts.Stdin, opts.Stdout, opts.Stderr)
		if err != nil {
			return nil, err
		}
		input, out, stderr := pipes[0], pipes[1], pipes[2]
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
	go func() { done <- e.execute(context.Background(), journey()) }()
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
	assert.Equal(t, 1, e.active)
	assert.False(t, e.execute(context.Background(), journey()).Drained)
}

func TestSecretInputRedactionAndDiagnosticBounds(t *testing.T) {
	e, h, _ := testEngine(t)
	request := journey()
	request.Timeout = 10 * time.Second
	request.Secrets = map[string]string{"DEM_SECRET_token": "long-secret", "DEM_SECRET_short": "x"}
	e.start = starter(t, func(_ context.Context, args []string, raw []byte, out, _ *os.File) ndexec.TreeResult {
		assert.NotContains(t, strings.Join(args, " "), "long-secret")
		var sent wireRequest
		if err := json.Unmarshal(raw, &sent); err != nil {
			return ndexec.TreeResult{Drained: true, Err: err}
		}
		assert.Equal(t, request.Secrets, sent.Secrets)
		for i := 0; i < eventLimit+4; i++ {
			emit(
				out,
				frame{
					Type: "event",
					Event: &synthetic.Event{
						Kind:    "stdout",
						Message: "long-secret x " + strings.Repeat("界", textLimit+10) + "\n",
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
	got := e.execute(context.Background(), request)
	assert.Equal(t, synthetic.Success, got.Run.Outcome)
	assert.Len(t, got.Run.Events, eventLimit)
	assert.Equal(t, 4, got.Run.DroppedEvents)
	for _, event := range got.Run.Events {
		assert.NotContains(t, event.Message, "long-secret")
		assert.NotContains(t, event.Message, " x ")
		assert.LessOrEqual(t, len([]rune(event.Message)), textLimit)
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
	got := e.execute(context.Background(), journey())
	assert.Equal(t, synthetic.Success, got.Run.Outcome)
	assert.Contains(t, got.Run.HistoryError, "append outcome uncertain")
	assert.Contains(t, got.Run.HistoryError, "sync failed")
	assert.Equal(t, []string{"start", "complete"}, h.phases)
	assert.Contains(t, h.runs[1].HistoryError, "start history")
}

func TestOversizeRequestNeverStartsWorkflow(t *testing.T) {
	e, _, a := testEngine(t)
	e.start = func(context.Context, ndexec.TreeOptions, string, ...string) (treeProcess, error) {
		t.Error("oversize request started a child")
		return nil, errors.New("unexpected start")
	}
	r := journey()
	r.Script = strings.Repeat("x", frameMaxBytes)
	got := e.execute(context.Background(), r)
	assert.Equal(t, synthetic.Error, got.Run.Outcome)
	assert.Nil(t, got.Run.DurationMS)
	assert.Contains(t, got.Run.Error, "4 MiB")
	assert.Equal(t, int32(1), a.finalized.Load())
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
	got := e.execute(
		context.Background(),
		synthetic.Request{
			Kind:    synthetic.Lighthouse,
			Name:    "snapshot",
			URL:     "https://example.test",
			Timeout: time.Second,
		},
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
	got := e.execute(ctx, request)
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
	got := e.execute(context.Background(), request)
	assert.NotContains(t, got.Run.Target, "private-query-value")
	for _, run := range h.runs {
		assert.NotContains(t, run.Target, "private-query-value")
	}
}

func TestWhitespaceScriptRejectedBeforeExecution(t *testing.T) {
	e, _, _ := testEngine(t)
	e.start = func(context.Context, ndexec.TreeOptions, string, ...string) (treeProcess, error) {
		t.Error("invalid script started a child")
		return nil, errors.New("unexpected process start")
	}
	request := journey()
	request.Script = " \t\n\u2003"
	got := e.execute(context.Background(), request)
	require.True(t, got.Drained)
	assert.Equal(t, synthetic.Error, got.Run.Outcome)
	assert.Equal(t, "script must not be blank", got.Run.Error)
}

func TestExecutionRecordsAcquiredIdentityBeforeStart(t *testing.T) {
	e, h, _ := testEngine(t)
	expected := e.source.(fixtureRuntime).Paths.Identity
	e.start = starter(t, func(ctx context.Context, args []string, raw []byte, out, errOut *os.File) ndexec.TreeResult {
		if !assert.Len(t, h.runs, 1) {
			return ndexec.TreeResult{Drained: true, Err: errors.New("start history was not appended before execution")}
		}
		assert.Equal(t, &expected, h.runs[0].Runtime)
		var request map[string]any
		assert.NoError(t, json.Unmarshal(raw, &request))
		assert.NotContains(t, request, "dependencies_path")
		assert.NotContains(t, request, "browser_path")
		return resultPayload(success())(ctx, args, raw, out, errOut)
	})
	got := e.execute(context.Background(), journey())
	require.Equal(t, synthetic.Success, got.Run.Outcome)
	require.Len(t, h.runs, 2)
	assert.Equal(t, &expected, got.Run.Runtime)
	assert.Equal(t, &expected, h.runs[1].Runtime)
	got.Run.Runtime.Version = "modified observation"
	assert.Equal(t, expected.Version, h.runs[0].Runtime.Version)
	assert.Equal(t, expected.Version, h.runs[1].Runtime.Version)
}

func TestExecutionSnapshotsAssociationBeforeLaunchAndThroughCompletion(t *testing.T) {
	for _, prelaunch := range []bool{true, false} {
		t.Run(fmt.Sprint(prelaunch), func(t *testing.T) {
			engine, history, _ := testEngine(t)
			request := journey()
			request.VnodeGUID = "11111111-1111-4111-8111-111111111111"
			if prelaunch {
				engine.platform = func() error { return errors.New("unavailable runtime") }
			} else {
				engine.start = starter(t, resultPayload(success()))
			}
			execution := engine.execute(context.Background(), request)
			require.True(t, execution.Drained)
			assert.Equal(t, "journey:checkout", execution.Run.JobID)
			assert.Equal(t, "11111111-1111-4111-8111-111111111111", execution.Run.VnodeGUID)
			request.VnodeGUID = "22222222-2222-4222-8222-222222222222"
			assert.Equal(t, "11111111-1111-4111-8111-111111111111", execution.Run.VnodeGUID)
			for _, run := range history.runs {
				assert.Equal(t, "11111111-1111-4111-8111-111111111111", run.VnodeGUID)
			}
			if prelaunch {
				assert.Equal(t, synthetic.Error, execution.Run.Outcome)
			} else {
				assert.Equal(t, synthetic.Success, execution.Run.Outcome)
				assert.Len(t, history.runs, 2)
			}
		})
	}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	engine, _, _ := testEngine(t)
	request := journey()
	request.VnodeGUID = "11111111-1111-4111-8111-111111111111"
	result := engine.execute(ctx, request)
	assert.Equal(t, synthetic.Cancelled, result.Run.Outcome)
	assert.Equal(t, "11111111-1111-4111-8111-111111111111", result.Run.VnodeGUID)
}

// TestExecutionExits pins every way an attempt ends: its outcome, the history
// phases it records, whether its run work directory is finalized, and whether
// the process tree is known drained.
func TestExecutionExits(t *testing.T) {
	failure := errors.New("fixture failure")
	type want struct {
		outcome   synthetic.Outcome
		reason    synthetic.TerminationReason
		err       string
		phases    []string
		finalized int32
		work      bool // a run work directory was created
		kept      bool // the work directory survives the attempt
		undrained bool
		admitted  bool
		completed bool
	}
	unstarted := want{completed: true}
	prelaunch := want{outcome: synthetic.Error, phases: []string{"start", "complete"}, admitted: true, completed: true}
	launched := want{phases: []string{"start", "complete"}, finalized: 1, work: true, admitted: true, completed: true}
	with := func(base want, change func(*want)) want { change(&base); return base }
	waitForStop := func(ctx context.Context, _ []string, _ []byte, _, _ *os.File) ndexec.TreeResult {
		<-ctx.Done()
		return ndexec.TreeResult{Drained: true, Err: ctx.Err()}
	}
	tests := map[string]struct {
		setup func(*Engine, *observedArtifacts, *synthetic.Request) context.Context
		want  want
	}{
		"poisoned before admission": {
			setup: func(e *Engine, _ *observedArtifacts, _ *synthetic.Request) context.Context {
				e.poison(failure)
				return t.Context()
			},
			want: with(unstarted, func(w *want) {
				w.outcome, w.err, w.undrained = synthetic.Error, ndexec.ErrTreeNotDrained.Error(), true
			}),
		},
		"cancelled before admission": {
			setup: func(*Engine, *observedArtifacts, *synthetic.Request) context.Context {
				ctx, cancel := context.WithCancel(t.Context())
				cancel()
				return ctx
			},
			want: with(unstarted, func(w *want) {
				w.outcome, w.reason, w.err = synthetic.Cancelled, synthetic.CallerCancelled, "context canceled"
			}),
		},
		"expired before admission": {
			setup: func(*Engine, *observedArtifacts, *synthetic.Request) context.Context {
				ctx, cancel := context.WithDeadline(t.Context(), time.Now().Add(-time.Second))
				t.Cleanup(cancel)
				return ctx
			},
			want: with(unstarted, func(w *want) {
				w.outcome, w.reason, w.err = synthetic.Cancelled, synthetic.CallerDeadline, "context deadline exceeded"
			}),
		},
		"invalid request": {
			setup: func(_ *Engine, _ *observedArtifacts, r *synthetic.Request) context.Context {
				r.Script = " "
				return t.Context()
			},
			want: with(prelaunch, func(w *want) { w.err = "script must not be blank" }),
		},
		"platform rejected": {
			setup: func(e *Engine, _ *observedArtifacts, _ *synthetic.Request) context.Context {
				e.platform = func() error { return failure }
				return t.Context()
			},
			want: with(prelaunch, func(w *want) { w.err = failure.Error() }),
		},
		"runtime unavailable": {
			setup: func(e *Engine, _ *observedArtifacts, _ *synthetic.Request) context.Context {
				e.source = sourceFunc(func() (Use, error) { return nil, failure })
				return t.Context()
			},
			want: with(prelaunch, func(w *want) { w.err = failure.Error() }),
		},
		"untrusted release": {
			setup: func(e *Engine, _ *observedArtifacts, _ *synthetic.Request) context.Context {
				paths := e.source.(fixtureRuntime).Paths
				paths.Identity.SHA256 = "not-a-digest"
				e.source = fixtureRuntime{Paths: paths}
				return t.Context()
			},
			want: with(prelaunch, func(w *want) { w.err = "runtime source supplied an invalid trusted release identity" }),
		},
		"work directory": {
			setup: func(_ *Engine, a *observedArtifacts, _ *synthetic.Request) context.Context {
				a.beginErr = failure
				return t.Context()
			},
			want: with(prelaunch, func(w *want) { w.err = "create run work directory: " + failure.Error() }),
		},
		"oversize request": {
			setup: func(_ *Engine, _ *observedArtifacts, r *synthetic.Request) context.Context {
				r.Script = strings.Repeat("x", frameMaxBytes)
				return t.Context()
			},
			want: with(launched, func(w *want) {
				w.outcome, w.err = synthetic.Error, "runner request exceeds 4 MiB transport limit"
			}),
		},
		"launch failure": {
			setup: func(e *Engine, _ *observedArtifacts, _ *synthetic.Request) context.Context {
				e.start = func(context.Context, ndexec.TreeOptions, string, ...string) (treeProcess, error) {
					return nil, failure
				}
				return t.Context()
			},
			want: with(launched, func(w *want) { w.outcome, w.err = synthetic.Error, failure.Error() }),
		},
		"launch after attempt deadline": {
			setup: func(e *Engine, _ *observedArtifacts, r *synthetic.Request) context.Context {
				r.Timeout = 20 * time.Millisecond
				e.start = func(ctx context.Context, _ ndexec.TreeOptions, _ string, _ ...string) (treeProcess, error) {
					<-ctx.Done()
					return nil, ctx.Err()
				}
				return t.Context()
			},
			want: with(launched, func(w *want) {
				w.outcome, w.reason, w.err = synthetic.Timeout, synthetic.ExecutionDeadline, "context deadline exceeded"
			}),
		},
		"launch after caller cancellation": {
			setup: func(e *Engine, _ *observedArtifacts, _ *synthetic.Request) context.Context {
				ctx, cancel := context.WithCancel(t.Context())
				e.start = func(context.Context, ndexec.TreeOptions, string, ...string) (treeProcess, error) {
					cancel()
					return nil, failure
				}
				return ctx
			},
			want: with(launched, func(w *want) {
				w.outcome, w.reason, w.err = synthetic.Cancelled, synthetic.CallerCancelled, failure.Error()
			}),
		},
		"reported success": {
			setup: func(e *Engine, _ *observedArtifacts, _ *synthetic.Request) context.Context {
				e.start = starter(t, resultPayload(success()))
				return t.Context()
			},
			want: with(launched, func(w *want) { w.outcome = synthetic.Success }),
		},
		"no terminal result": {
			setup: func(e *Engine, _ *observedArtifacts, _ *synthetic.Request) context.Context {
				e.start = starter(t, func(context.Context, []string, []byte, *os.File, *os.File) ndexec.TreeResult {
					return ndexec.TreeResult{Drained: true}
				})
				return t.Context()
			},
			want: with(launched, func(w *want) {
				w.outcome, w.err = synthetic.Error, "runner exited without a terminal result: no result"
			}),
		},
		"attempt deadline": {
			setup: func(e *Engine, _ *observedArtifacts, r *synthetic.Request) context.Context {
				r.Timeout = 50 * time.Millisecond
				e.start = starter(t, waitForStop)
				return t.Context()
			},
			want: with(launched, func(w *want) {
				w.outcome, w.reason, w.err = synthetic.Timeout, synthetic.ExecutionDeadline, "overall execution deadline exceeded"
			}),
		},
		"caller deadline": {
			setup: func(e *Engine, _ *observedArtifacts, r *synthetic.Request) context.Context {
				r.Timeout = 10 * time.Second
				ctx, cancel := context.WithTimeout(t.Context(), 50*time.Millisecond)
				t.Cleanup(cancel)
				e.start = starter(t, waitForStop)
				return ctx
			},
			want: with(launched, func(w *want) {
				w.outcome, w.reason, w.err = synthetic.Timeout, synthetic.CallerDeadline, "caller deadline exceeded"
			}),
		},
		"caller cancellation": {
			setup: func(e *Engine, _ *observedArtifacts, _ *synthetic.Request) context.Context {
				ctx, cancel := context.WithCancel(t.Context())
				e.start = starter(t, func(ctx context.Context, args []string, raw []byte, out, stderr *os.File) ndexec.TreeResult {
					cancel()
					return waitForStop(ctx, args, raw, out, stderr)
				})
				return ctx
			},
			want: with(launched, func(w *want) {
				w.outcome, w.reason, w.err = synthetic.Cancelled, synthetic.CallerCancelled, "execution cancelled"
			}),
		},
		"undrained tree": {
			setup: func(e *Engine, _ *observedArtifacts, _ *synthetic.Request) context.Context {
				e.start = starter(t, func(context.Context, []string, []byte, *os.File, *os.File) ndexec.TreeResult {
					return ndexec.TreeResult{Err: failure}
				})
				return t.Context()
			},
			want: want{
				outcome:   synthetic.Error,
				err:       "process-tree completion is unverified: " + failure.Error(),
				phases:    []string{"start"},
				work:      true,
				kept:      true,
				undrained: true,
				admitted:  true,
			},
		},
	}
	for name, tc := range tests {
		t.Run(name, func(t *testing.T) {
			e, history, a := testEngine(t)
			e.start = func(context.Context, ndexec.TreeOptions, string, ...string) (treeProcess, error) {
				t.Error("unexpected launch")
				return nil, errors.New("unexpected launch")
			}
			request := journey()
			ctx := tc.setup(e, a, &request)
			got := e.execute(ctx, request)

			run := got.Run
			assert.Equal(t, tc.want.outcome, run.Outcome)
			assert.Equal(t, tc.want.reason, run.TerminationReason)
			assert.Equal(t, tc.want.err, run.Error)
			assert.Equal(t, !tc.want.undrained, got.Drained)
			assert.Equal(t, tc.want.admitted, run.StartedUS > 0, "admission timestamp")
			assert.Equal(t, tc.want.completed, run.CompletedUS > 0, "completion timestamp")
			assert.Equal(t, tc.want.phases, history.phases)
			assert.Equal(t, tc.want.finalized, a.finalized.Load())
			require.Equal(t, tc.want.work, len(a.begun) == 1, "run work directory")
			if tc.want.work {
				_, err := os.Stat(a.begun[0])
				assert.Equal(t, tc.want.kept, err == nil, "work directory kept: %v", err)
			}
			if !tc.want.undrained {
				requireSlotFree(t, e)
			}
		})
	}
}

func TestConstructorRequiresOwnersAndPositiveConcurrency(t *testing.T) {
	history, store := &fakeHistory{}, &observedArtifacts{}
	tests := map[string]struct {
		history     historyWriter
		artifacts   artifactStore
		concurrency int
		valid       bool
	}{
		"valid":                {history: history, artifacts: store, concurrency: 2, valid: true},
		"zero concurrency":     {history: history, artifacts: store, concurrency: 0},
		"negative concurrency": {history: history, artifacts: store, concurrency: -1},
		"no history":           {artifacts: store, concurrency: 1},
		"no artifacts":         {history: history, concurrency: 1},
	}
	for name, tc := range tests {
		t.Run(name, func(t *testing.T) {
			e, err := New(nil, tc.history, tc.artifacts, tc.concurrency)
			if tc.valid {
				require.NoError(t, err)
				assert.NotNil(t, e)
			} else {
				require.Error(t, err)
				assert.Nil(t, e)
			}
		})
	}
}

func TestJourneyStopAtKeepsAdmissionBudget(t *testing.T) {
	tests := map[string]struct {
		budget, startup, wantRemaining time.Duration
	}{
		"default":                {120 * time.Second, 2 * time.Second, 113 * time.Second},
		"earlier caller":         {2 * time.Second, 100 * time.Millisecond, 1500 * time.Millisecond},
		"startup exhausted work": {time.Second, 900 * time.Millisecond, -100 * time.Millisecond},
		"short budget":           {5 * time.Millisecond, 0, 4 * time.Millisecond},
		"long budget":            {time.Duration(1<<63-1) - time.Hour, 0, time.Duration(1<<63-1) - time.Hour - 5*time.Second},
	}
	for name, tc := range tests {
		t.Run(name, func(t *testing.T) {
			now, err := monotonicNow()
			require.NoError(t, err)
			admitted := time.Now().Add(-tc.startup)
			stop, err := journeyStopAt(admitted.Add(tc.budget), admitted)
			require.NoError(t, err)
			assert.InDelta(t, tc.wantRemaining.Milliseconds(), stop-now.Milliseconds(), 10)
		})
	}
}

func TestBudgetStopCapturePolicyAfterDrain(t *testing.T) {
	tests := map[string]struct {
		reason   synthetic.TerminationReason
		outcome  synthetic.Outcome
		captures int
	}{
		"budget":          {synthetic.ExecutionBudget, synthetic.Timeout, 2},
		"cancelled":       {synthetic.CallerCancelled, synthetic.Cancelled, 1},
		"hard deadline":   {synthetic.ExecutionDeadline, synthetic.Timeout, 1},
		"caller deadline": {synthetic.CallerDeadline, synthetic.Timeout, 1},
	}
	for name, tc := range tests {
		t.Run(name, func(t *testing.T) {
			e, history, a := testEngine(t)
			ctx, cancel := context.WithCancel(context.Background())
			defer cancel()
			request := journey()
			request.Capture = true
			if tc.reason == synthetic.ExecutionDeadline {
				request.Timeout = 200 * time.Millisecond
			}
			if tc.reason == synthetic.CallerDeadline {
				var expiry context.CancelFunc
				ctx, expiry = context.WithTimeout(ctx, 200*time.Millisecond)
				defer expiry()
			}
			e.start = starter(t, func(processCtx context.Context, _ []string, raw []byte, out, _ *os.File) ndexec.TreeResult {
				var req wireRequest
				if err := json.Unmarshal(raw, &req); !assert.NoError(t, err) {
					return ndexec.TreeResult{Drained: true, Err: err}
				}
				assert.Positive(t, req.StopAtMonotonicMS)
				if err := os.MkdirAll(filepath.Join(req.WorkDir, "output"), 0700); !assert.NoError(t, err) {
					return ndexec.TreeResult{Drained: true, Err: err}
				}
				var candidates []synthetic.Capture
				for _, id := range []string{"earlier-failure", "interrupted"} {
					name := "output/" + id + ".png"
					f, err := os.Create(filepath.Join(req.WorkDir, name))
					if !assert.NoError(t, err) {
						return ndexec.TreeResult{Drained: true, Err: err}
					}
					err = png.Encode(f, image.NewRGBA(image.Rect(0, 0, 1, 1)))
					assert.NoError(t, f.Close())
					if !assert.NoError(t, err) {
						return ndexec.TreeResult{Drained: true, Err: err}
					}
					candidates = append(candidates, synthetic.Capture{ID: id, Path: name, Kind: "screenshot", MIME: "image/png"})
				}
				terminal := reporterResult{Status: synthetic.Timeout, TerminationReason: synthetic.ExecutionBudget,
					Error: "first assertion failed", Tests: &synthetic.TestCounts{Declared: 2, Failed: 1, NotRun: 1},
					CaptureState: "available", Artifacts: candidates[:1], InterruptedArtifacts: candidates[1:]}
				emit(out, frame{Type: "result", Result: &terminal})
				if tc.reason == synthetic.CallerCancelled {
					cancel()
				}
				if tc.reason != synthetic.ExecutionBudget {
					<-processCtx.Done()
				}
				assert.Zero(t, a.finalized.Load(), "terminal reporting cannot publish before drainage")
				return ndexec.TreeResult{Drained: true}
			})
			ex := e.execute(ctx, request)
			require.True(t, ex.Drained)
			assert.Equal(t, tc.outcome, ex.Run.Outcome)
			assert.Equal(t, tc.reason, ex.Run.TerminationReason)
			assert.Equal(t, "first assertion failed", ex.Run.Error)
			require.NotNil(t, ex.Run.Tests)
			assert.Equal(t, 1, ex.Run.Tests.Failed)
			assert.Equal(t, 1, ex.Run.Tests.NotRun)
			require.Len(t, ex.Run.Artifacts, tc.captures)
			require.NotNil(t, ex.Run.Captures)
			if tc.reason == synthetic.ExecutionBudget {
				assert.Empty(t, ex.Run.Captures.Unpublished)
			} else {
				assert.Equal(t, []synthetic.CaptureOmission{{ID: "interrupted", Reason: "not_eligible"}}, ex.Run.Captures.Unpublished)
			}
			assert.Equal(t, "earlier-failure", ex.Run.Artifacts[0].ID)
			assert.Equal(t, int32(1), a.finalized.Load())
			require.Len(t, history.runs, 2)
			assert.Equal(t, synthetic.CloneRun(ex.Run), history.runs[1])
		})
	}
}

func TestTerminalTerminationReasonsAndInterruptedCandidates(t *testing.T) {
	tests := map[string]struct {
		status      synthetic.Outcome
		reason      synthetic.TerminationReason
		interrupted bool
		valid       bool
	}{
		"budget":                               {synthetic.Timeout, synthetic.ExecutionBudget, true, true},
		"native test":                          {synthetic.Timeout, synthetic.TestTimeout, false, true},
		"native suite":                         {synthetic.Timeout, synthetic.SuiteTimeout, false, true},
		"cancel":                               {synthetic.Cancelled, synthetic.CallerCancelled, false, true},
		"cancel capture":                       {synthetic.Cancelled, synthetic.CallerCancelled, true, false},
		"native capture cannot be interrupted": {synthetic.Timeout, synthetic.TestTimeout, true, false},
		"runner cannot claim hard deadline":    {synthetic.Timeout, synthetic.ExecutionDeadline, false, false},
		"reason contradicts outcome":           {synthetic.Failed, synthetic.ExecutionBudget, false, false},
	}
	for name, tc := range tests {
		t.Run(name, func(t *testing.T) {
			r := reporterResult{Status: tc.status, TerminationReason: tc.reason, CaptureState: "unavailable"}
			if tc.interrupted {
				r.InterruptedArtifacts = []synthetic.Capture{{ID: "screenshot", Path: "shot.png", Kind: "screenshot", MIME: "image/png"}}
				r.CaptureState = "available"
			}
			err := validateResult(synthetic.Journey, true, false, &r)
			if tc.valid {
				assert.NoError(t, err)
			} else {
				assert.Error(t, err)
			}
		})
	}
}

func TestBudgetTerminalWithoutCountsPreservesIncrementalTestFacts(t *testing.T) {
	e, _, _ := testEngine(t)
	e.start = starter(t, func(_ context.Context, _ []string, _ []byte, out, _ *os.File) ndexec.TreeResult {
		emit(out, frame{Type: "event", Event: &synthetic.Event{
			Kind: "suite", Phase: "discovery", DeclaredTests: intPointer(2),
		}})
		emit(out, frame{Type: "event", Event: &synthetic.Event{
			Kind: "test", Phase: "end", TestID: "completed", Status: "passed", ExpectedStatus: "passed",
		}})
		emit(out, frame{Type: "result", Result: &reporterResult{
			Status: synthetic.Timeout, TerminationReason: synthetic.ExecutionBudget,
			Error: "Runner exited without a terminal result (SIGINT)", CaptureState: "disabled",
		}})
		return ndexec.TreeResult{Drained: true}
	})
	ex := e.execute(context.Background(), journey())
	require.True(t, ex.Drained)
	assert.Equal(t, synthetic.Timeout, ex.Run.Outcome)
	require.NotNil(t, ex.Run.Tests)
	assert.Equal(t, synthetic.TestCounts{Declared: 2, Passed: 1, NotRun: 1}, *ex.Run.Tests)
}
