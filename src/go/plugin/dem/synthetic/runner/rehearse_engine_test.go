//go:build linux || darwin || freebsd

// SPDX-License-Identifier: GPL-3.0-or-later

package runner

import (
	"context"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"sync/atomic"
	"testing"
	"time"

	"github.com/netdata/netdata/go/plugins/plugin/dem/synthetic"
	"github.com/netdata/netdata/go/plugins/plugin/go.d/pkg/ndexec"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestRehearseIsolatesOutputsAndUsesResolvedRequest(t *testing.T) {
	e, history, captures := testEngine(t)
	request := journey()
	request.Capture = true
	request.Trace = true
	request.Secrets = map[string]string{"DEM_SECRET_TOKEN": "synthetic-secret"}
	var work string
	e.start = starter(t, func(ctx context.Context, args []string, raw []byte, out, stderr *os.File) ndexec.TreeResult {
		var wire wireRequest
		if err := json.Unmarshal(raw, &wire); err != nil {
			return ndexec.TreeResult{Drained: true, Err: err}
		}
		work = wire.WorkDir
		assert.False(t, wire.Capture)
		assert.NotContains(t, wire.Extensions, failureTraceFeature)
		assert.Equal(t, request.Script, wire.Script)
		assert.Equal(t, request.Secrets, wire.Secrets)
		return resultPayload(success())(ctx, args, raw, out, stderr)
	})
	require.NoError(t, e.Rehearse(t.Context(), request))
	assert.True(t, request.Capture, "rehearsal must not mutate the monitor request")
	assert.True(t, request.Trace)
	require.NotEmpty(t, work)
	_, err := os.Stat(work)
	assert.ErrorIs(t, err, os.ErrNotExist)
	assert.Empty(t, history.runs)
	assert.Zero(t, history.syncCalls)
	assert.Zero(t, captures.finalized.Load())
	// Ordinary monitoring still records through the same engine afterward.
	e.start = starter(t, resultPayload(success()))
	result := e.execute(t.Context(), journey())
	require.True(t, result.Drained)
	require.Equal(t, synthetic.Success, result.Run.Outcome)
	assert.Len(t, history.runs, 2)
	assert.Equal(t, int32(1), captures.finalized.Load())
}

func TestRehearseRejectsFailureAndIncompleteCoverage(t *testing.T) {
	for _, result := range []reporterResult{
		{Status: synthetic.Failed, Tests: &synthetic.TestCounts{Declared: 1, Failed: 1}, Error: "assertion synthetic-secret", CaptureState: "disabled"},
		{Status: synthetic.Inconclusive, Tests: &synthetic.TestCounts{Declared: 1, Skipped: 1}, Error: "skipped", CaptureState: "disabled"},
		{Status: synthetic.Error, Error: "load synthetic-secret", CaptureState: "disabled"},
	} {
		t.Run(string(result.Status), func(t *testing.T) {
			e, history, _ := testEngine(t)
			e.start = starter(t, resultPayload(result))
			request := journey()
			request.Secrets = map[string]string{"DEM_SECRET_TOKEN": "synthetic-secret"}
			err := e.Rehearse(t.Context(), request)
			require.Error(t, err)
			assert.Contains(t, err.Error(), string(result.Status))
			assert.NotContains(t, err.Error(), "synthetic-secret")
			assert.Empty(t, history.runs)
		})
	}
}

func TestRehearseReportsWorkspaceCleanupFailure(t *testing.T) {
	if os.Geteuid() == 0 {
		t.Skip("requires unprivileged filesystem permissions")
	}
	t.Setenv("TMPDIR", t.TempDir())
	e, history, _ := testEngine(t)
	var locked, alias string
	e.start = starter(t, func(ctx context.Context, args []string, raw []byte, out, stderr *os.File) ndexec.TreeResult {
		var wire wireRequest
		if err := json.Unmarshal(raw, &wire); err != nil {
			return ndexec.TreeResult{Drained: true, Err: err}
		}
		alias = wire.WorkDir
		work, err := filepath.EvalSymlinks(alias)
		if err != nil {
			return ndexec.TreeResult{Drained: true, Err: err}
		}
		locked = filepath.Join(work, "locked")
		if err := os.Mkdir(locked, 0700); err != nil {
			return ndexec.TreeResult{Drained: true, Err: err}
		}
		if err := os.WriteFile(filepath.Join(locked, "script-output"), []byte("synthetic data"), 0600); err != nil {
			return ndexec.TreeResult{Drained: true, Err: err}
		}
		if err := os.Chmod(locked, 0); err != nil {
			return ndexec.TreeResult{Drained: true, Err: err}
		}
		return resultPayload(success())(ctx, args, raw, out, stderr)
	})
	t.Cleanup(func() {
		if locked != "" {
			require.NoError(t, os.Chmod(locked, 0700))
		}
		if alias != "" {
			if err := os.Remove(alias); !errors.Is(err, os.ErrNotExist) {
				require.NoError(t, err)
			}
		}
	})
	err := e.Rehearse(t.Context(), journey())
	require.ErrorContains(t, err, "temporary artifact store cleanup is incomplete")
	assert.Empty(t, history.runs)
}

func TestRehearseSharesAdmissionAndHonorsCancellation(t *testing.T) {
	e, history, _ := testEngine(t)
	started, release := make(chan struct{}), make(chan struct{})
	var calls atomic.Int32
	e.start = starter(t, func(ctx context.Context, args []string, raw []byte, out, stderr *os.File) ndexec.TreeResult {
		if calls.Add(1) == 1 {
			close(started)
			<-release
			return resultPayload(success())(ctx, args, raw, out, stderr)
		}
		<-ctx.Done()
		return ndexec.TreeResult{Drained: true, Err: ctx.Err()}
	})
	done := make(chan error, 1)
	go func() { done <- e.Rehearse(t.Context(), journey()) }()
	<-started
	ctx, cancel := context.WithTimeout(t.Context(), 30*time.Millisecond)
	defer cancel()
	queued := e.execute(ctx, journey())
	assert.Equal(t, synthetic.Cancelled, queued.Run.Outcome)
	assert.Zero(t, queued.Run.StartedUS)
	assert.Equal(t, int32(1), calls.Load())
	// A second draft waits on the occupied slot and must not start after expiry.
	waitCtx, waitCancel := context.WithTimeout(t.Context(), 30*time.Millisecond)
	defer waitCancel()
	require.ErrorIs(t, e.Rehearse(waitCtx, journey()), context.DeadlineExceeded)
	assert.Equal(t, int32(1), calls.Load())
	close(release)
	require.NoError(t, <-done)
	request := journey()
	request.Timeout = 30 * time.Millisecond
	require.ErrorIs(t, e.Rehearse(t.Context(), request), context.DeadlineExceeded)
	assert.Empty(t, history.runs)
	assert.Equal(t, int32(2), calls.Load())
}

func TestRehearseUnverifiedDrainPoisonsSharedOwnerAndPreservesWork(t *testing.T) {
	// The rehearsal store is created under TMPDIR; the test directory removes
	// its preserved uncertain work.
	t.Setenv("TMPDIR", t.TempDir())
	e, history, _ := testEngine(t)
	var work, alias string
	e.start = starter(t, func(_ context.Context, _ []string, raw []byte, _, _ *os.File) ndexec.TreeResult {
		var wire wireRequest
		if err := json.Unmarshal(raw, &wire); err != nil {
			return ndexec.TreeResult{Drained: true, Err: err}
		}
		alias = wire.WorkDir
		work, _ = filepath.EvalSymlinks(alias)
		return ndexec.TreeResult{Drained: false, Err: errors.New("no drainage proof")}
	})
	require.ErrorIs(t, e.Rehearse(t.Context(), journey()), ndexec.ErrTreeNotDrained)
	select {
	case err := <-e.Fatal():
		require.ErrorIs(t, err, ndexec.ErrTreeNotDrained)
	default:
		t.Fatal("rehearsal failed to notify the process owner")
	}
	require.NotEmpty(t, work)
	_, err := os.Stat(work)
	require.NoError(t, err, "uncertain child ownership must preserve the workspace")
	assert.False(t, e.execute(t.Context(), journey()).Drained)
	assert.Empty(t, history.runs)
	if alias != work {
		require.NoError(t, os.Remove(alias))
	}
}

func TestRehearseUsesWireFailureEvidence(t *testing.T) {
	e, history, _ := testEngine(t)
	e.start = starter(t, func(ctx context.Context, args []string, raw []byte, out, stderr *os.File) ndexec.TreeResult {
		encoder := json.NewEncoder(out)
		if err := encoder.Encode(map[string]any{"type": "extension_init", "extension_init": map[string]any{"selected": []string{failureEvidenceFeature}}}); !assert.NoError(t, err) {
			return ndexec.TreeResult{Drained: true, Err: err}
		}
		failure := synthetic.FailureDetail{Test: synthetic.EvidenceTest{ID: "t", Title: "checkout", Status: "failed", ExpectedStatus: "passed", ReportingComplete: true}, Message: "receipt synthetic-secret missing", Location: &synthetic.EvidenceLocation{File: "checkout.spec.js", Line: 10, Column: 2}}
		if err := encoder.Encode(map[string]any{"type": "extension", "extension": map[string]any{"feature": failureEvidenceFeature, "data": map[string]any{"record": "failure", "failure": failure}}}); !assert.NoError(t, err) {
			return ndexec.TreeResult{Drained: true, Err: err}
		}
		return resultPayload(reporterResult{Status: synthetic.Failed, Tests: &synthetic.TestCounts{Declared: 1, Failed: 1}, Error: "core assertion", CaptureState: "disabled"})(ctx, args, raw, out, stderr)
	})
	request := journey()
	request.Secrets = map[string]string{"DEM_SECRET_TOKEN": "synthetic-secret"}
	err := e.Rehearse(t.Context(), request)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "First unexpected failure: checkout [failed; expected passed] (checkout.spec.js:10:2): receipt")
	assert.NotContains(t, err.Error(), "synthetic-secret")
	assert.Empty(t, history.runs)
}

func TestRehearseOptionalInvalidityPreservesSuccess(t *testing.T) {
	e, history, _ := testEngine(t)
	e.start = starter(t, func(ctx context.Context, args []string, raw []byte, out, stderr *os.File) ndexec.TreeResult {
		encoder := json.NewEncoder(out)
		if err := encoder.Encode(map[string]any{"type": "extension_init", "extension_init": map[string]any{"selected": []string{failureEvidenceFeature}}}); !assert.NoError(t, err) {
			return ndexec.TreeResult{Drained: true, Err: err}
		}
		if err := encoder.Encode(map[string]any{"type": "extension", "extension": map[string]any{"feature": failureEvidenceFeature, "data": map[string]any{"record": "invalid"}}}); !assert.NoError(t, err) {
			return ndexec.TreeResult{Drained: true, Err: err}
		}
		return resultPayload(success())(ctx, args, raw, out, stderr)
	})
	require.NoError(t, e.Rehearse(t.Context(), journey()))
	assert.Empty(t, history.runs)
}
