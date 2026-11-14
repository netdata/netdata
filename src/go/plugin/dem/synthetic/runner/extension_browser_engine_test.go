//go:build linux || darwin || freebsd

// SPDX-License-Identifier: GPL-3.0-or-later

package runner

import (
	"context"
	"encoding/json"
	"os"
	"slices"
	"testing"

	"github.com/netdata/netdata/go/plugins/plugin/dem/synthetic"
	"github.com/netdata/netdata/go/plugins/plugin/go.d/pkg/ndexec"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestBrowserSummaryRequestSelectionAndOutcomeIndependence(t *testing.T) {
	for _, enabled := range []bool{false, true} {
		t.Run(map[bool]string{false: "default off", true: "enabled"}[enabled], func(t *testing.T) {
			e, _, _ := testEngine(t)
			e.start = starter(t, func(ctx context.Context, args []string, raw []byte, out, stderr *os.File) ndexec.TreeResult {
				var request wireRequest
				assert.NoError(t, json.Unmarshal(raw, &request))
				assert.Equal(t, synthetic.ExecutionContract, request.Version)
				assert.Equal(t, enabled, slices.Contains(request.Extensions, browserSummaryFeature))
				encoder := json.NewEncoder(out)
				assert.NoError(t, encoder.Encode(map[string]any{"type": "extension_init", "extension_init": map[string]any{"selected": request.Extensions}}))
				if enabled {
					assert.NoError(t, encoder.Encode(map[string]any{"type": "extension", "extension": map[string]any{"feature": browserSummaryFeature, "data": map[string]any{"record": "complete", "run": browserRun(1)}}}))
				}
				return resultPayload(success())(ctx, args, raw, out, stderr)
			})
			request := journey()
			request.BrowserErrorSummary = enabled
			execution := e.execute(t.Context(), request)
			require.True(t, execution.Drained)
			assert.Equal(t, synthetic.Success, execution.Run.Outcome)
			if enabled {
				require.NotNil(t, execution.Run.BrowserSummary)
				assert.Equal(t, "complete", execution.Run.BrowserSummary.ReportingStatus())
				assert.Equal(t, int64(1), execution.Run.BrowserSummary.Run.Counts.Uncaught)
			} else {
				assert.Nil(t, execution.Run.BrowserSummary)
			}
		})
	}
}

func TestBrowserRehearsalEvidenceIsFailureOnly(t *testing.T) {
	for _, failure := range []bool{false, true} {
		t.Run(map[bool]string{false: "success", true: "failure"}[failure], func(t *testing.T) {
			e, history, _ := testEngine(t)
			e.start = starter(t, func(ctx context.Context, args []string, raw []byte, out, stderr *os.File) ndexec.TreeResult {
				encoder := json.NewEncoder(out)
				assert.NoError(t, encoder.Encode(map[string]any{"type": "extension_init", "extension_init": map[string]any{"selected": []string{failureEvidenceFeature, browserSummaryFeature}}}))
				assert.NoError(t, encoder.Encode(map[string]any{"type": "extension", "extension": map[string]any{"feature": browserSummaryFeature, "data": map[string]any{"record": "complete", "run": browserRun(1)}}}))
				result := success()
				if failure {
					result = reporterResult{Status: synthetic.Failed, Tests: &synthetic.TestCounts{Declared: 1, Failed: 1}, Error: "core assertion", CaptureState: "disabled"}
				}
				return resultPayload(result)(ctx, args, raw, out, stderr)
			})
			request := journey()
			request.BrowserErrorSummary = true
			err := e.Rehearse(t.Context(), request)
			if failure {
				require.Error(t, err)
				assert.Contains(t, err.Error(), "core assertion")
				assert.Contains(t, err.Error(), "Browser errors: fixture_context")
				assert.Contains(t, err.Error(), "uncaught=1")
			} else {
				require.NoError(t, err)
			}
			assert.Empty(t, history.runs)
		})
	}
}
