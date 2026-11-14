//go:build linux || darwin || freebsd

// SPDX-License-Identifier: GPL-3.0-or-later

package runner

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"testing"
	"time"

	"github.com/netdata/netdata/go/plugins/plugin/dem/synthetic"
	"github.com/netdata/netdata/go/plugins/plugin/go.d/pkg/ndexec"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestEngineEvidenceSurvivesHardDeadlineBeforeTerminal(t *testing.T) {
	engine, history, captures := testEngine(t)
	engine.start = starter(t, func(ctx context.Context, _ []string, raw []byte, out, _ *os.File) ndexec.TreeResult {
		var request wireRequest
		if err := json.Unmarshal(raw, &request); !assert.NoError(t, err) {
			return ndexec.TreeResult{Drained: true, Err: err}
		}
		assert.Equal(t, []string{failureEvidenceFeature}, request.Extensions)
		fmt.Fprintln(out, evidenceInit)
		fmt.Fprintln(out, evidenceRecord("test", "test", evidenceStart("t1")))
		fmt.Fprintln(out, evidenceRecord("step", "step", evidenceStep("s1", "")))
		<-ctx.Done()
		return ndexec.TreeResult{Drained: true, Err: ctx.Err()}
	})
	request := journey()
	request.Timeout = 100 * time.Millisecond
	execution := engine.execute(context.Background(), request)
	require.True(t, execution.Drained)
	require.Equal(t, synthetic.Timeout, execution.Run.Outcome)
	require.NotNil(t, execution.Run.FailureEvidence)
	assert.Equal(t, "partial", execution.Run.FailureEvidence.ReportingStatus())
	assert.Contains(t, execution.Run.FailureEvidence.LastObservedStepSummary(), "Submit order")
	require.Len(t, history.runs, 2)
	assert.Equal(t, execution.Run.FailureEvidence, history.runs[1].FailureEvidence)
	assert.Equal(t, int32(1), captures.finalized.Load())
}

func TestEngineOptionalCorruptionDoesNotCancelOrOverrideResult(t *testing.T) {
	engine, history, _ := testEngine(t)
	engine.start = starter(t, func(ctx context.Context, _ []string, _ []byte, out, _ *os.File) ndexec.TreeResult {
		fmt.Fprintln(out, evidenceInit)
		fmt.Fprintln(out, evidenceRecord("test", "test", evidenceStart("t1")))
		fmt.Fprintln(out, `{"type":"extension","extension":false}`)
		assert.NoError(t, ctx.Err())
		emit(out, frame{Type: "result", Result: new(success())})
		return ndexec.TreeResult{Drained: true}
	})
	execution := engine.execute(context.Background(), journey())
	require.Equal(t, synthetic.Success, execution.Run.Outcome)
	assert.Empty(t, execution.Run.Error)
	assert.Empty(t, execution.Run.HistoryError)
	require.NotNil(t, execution.Run.FailureEvidence)
	assert.Equal(t, "invalid", execution.Run.FailureEvidence.ReportingStatus())
	require.Len(t, execution.Run.FailureEvidence.Tests, 1)
	require.Len(t, history.runs, 2)
}

func TestEngineCoreCorruptionStillKeepsEarlierOptionalObservations(t *testing.T) {
	engine, _, _ := testEngine(t)
	engine.start = starter(t, func(ctx context.Context, _ []string, _ []byte, out, _ *os.File) ndexec.TreeResult {
		fmt.Fprintln(out, evidenceInit)
		fmt.Fprintln(out, evidenceRecord("test", "test", evidenceStart("t1")))
		fmt.Fprintln(out, `{"type":"event","event":{"kind":"invalid"}}`)
		<-ctx.Done()
		return ndexec.TreeResult{Drained: true, Err: ctx.Err()}
	})
	execution := engine.execute(context.Background(), journey())
	require.Equal(t, synthetic.Error, execution.Run.Outcome)
	require.NotNil(t, execution.Run.FailureEvidence)
	require.Len(t, execution.Run.FailureEvidence.Tests, 1)
	assert.False(t, execution.Run.FailureEvidence.ReporterComplete)
}
