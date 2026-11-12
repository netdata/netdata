//go:build linux || darwin || freebsd

// SPDX-License-Identifier: GPL-3.0-or-later

package runner

import (
	"context"
	"encoding/json"
	"image"
	"image/png"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/netdata/netdata/go/plugins/plugin/dem/synthetic"
	"github.com/netdata/netdata/go/plugins/plugin/go.d/pkg/ndexec"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestJourneyStopAtKeepsAdmissionBudget(t *testing.T) {
	for _, tc := range []struct {
		name                           string
		budget, startup, wantRemaining time.Duration
	}{
		{"default", 120 * time.Second, 2 * time.Second, 113 * time.Second},
		{"earlier caller", 2 * time.Second, 100 * time.Millisecond, 1500 * time.Millisecond},
		{"startup exhausted work", time.Second, 900 * time.Millisecond, -100 * time.Millisecond},
		{"short budget", 5 * time.Millisecond, 0, 4 * time.Millisecond},
		{"long budget", time.Duration(1<<63-1) - time.Hour, 0, time.Duration(1<<63-1) - time.Hour - 5*time.Second},
	} {
		t.Run(tc.name, func(t *testing.T) {
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
	for _, tc := range []struct {
		name     string
		reason   synthetic.TerminationReason
		outcome  synthetic.Outcome
		captures int
	}{
		{"budget", synthetic.ExecutionBudget, synthetic.Timeout, 2},
		{"cancelled", synthetic.CallerCancelled, synthetic.Cancelled, 1},
		{"hard deadline", synthetic.ExecutionDeadline, synthetic.Timeout, 1},
		{"caller deadline", synthetic.CallerDeadline, synthetic.Timeout, 1},
	} {
		t.Run(tc.name, func(t *testing.T) {
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
			ex := e.Execute(ctx, request, nil)
			require.True(t, ex.Drained)
			assert.Equal(t, tc.outcome, ex.Run.Outcome)
			assert.Equal(t, tc.reason, ex.Run.TerminationReason)
			assert.Equal(t, "first assertion failed", ex.Run.Error)
			require.NotNil(t, ex.Run.Tests)
			assert.Equal(t, 1, ex.Run.Tests.Failed)
			assert.Equal(t, 1, ex.Run.Tests.NotRun)
			require.Len(t, ex.Run.Artifacts, tc.captures)
			assert.Equal(t, "earlier-failure", ex.Run.Artifacts[0].ID)
			assert.Equal(t, int32(1), a.finalized.Load())
			require.Len(t, history.runs, 2)
			assert.Equal(t, synthetic.CloneRun(ex.Run), history.runs[1])
		})
	}
}

func TestTerminalTerminationReasonsAndInterruptedCandidates(t *testing.T) {
	for _, tc := range []struct {
		name        string
		status      synthetic.Outcome
		reason      synthetic.TerminationReason
		interrupted bool
		valid       bool
	}{
		{"budget", synthetic.Timeout, synthetic.ExecutionBudget, true, true},
		{"native test", synthetic.Timeout, synthetic.TestTimeout, false, true},
		{"native suite", synthetic.Timeout, synthetic.SuiteTimeout, false, true},
		{"cancel", synthetic.Cancelled, synthetic.CallerCancelled, false, true},
		{"cancel capture", synthetic.Cancelled, synthetic.CallerCancelled, true, false},
		{"native capture cannot be interrupted", synthetic.Timeout, synthetic.TestTimeout, true, false},
		{"runner cannot claim hard deadline", synthetic.Timeout, synthetic.ExecutionDeadline, false, false},
		{"reason contradicts outcome", synthetic.Failed, synthetic.ExecutionBudget, false, false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			r := reporterResult{Status: tc.status, TerminationReason: tc.reason, CaptureState: "unavailable"}
			if tc.interrupted {
				r.InterruptedArtifacts = []synthetic.Capture{{ID: "screenshot", Path: "shot.png", Kind: "screenshot", MIME: "image/png"}}
				r.CaptureState = "available"
			}
			err := validateResult(synthetic.Journey, true, &r)
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
	ex := e.Execute(context.Background(), journey(), nil)
	require.True(t, ex.Drained)
	assert.Equal(t, synthetic.Timeout, ex.Run.Outcome)
	require.NotNil(t, ex.Run.Tests)
	assert.Equal(t, synthetic.TestCounts{Declared: 2, Passed: 1, NotRun: 1}, *ex.Run.Tests)
}
