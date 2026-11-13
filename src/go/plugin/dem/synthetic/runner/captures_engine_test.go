//go:build linux || darwin || freebsd

// SPDX-License-Identifier: GPL-3.0-or-later
package runner

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/netdata/netdata/go/plugins/plugin/dem/synthetic"
	"github.com/netdata/netdata/go/plugins/plugin/dem/synthetic/artifacts"
	"github.com/netdata/netdata/go/plugins/plugin/go.d/pkg/ndexec"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestCaptureReportingThroughEnginePublication(t *testing.T) {
	for _, mode := range []string{"published", "missing terminal", "oversize", "invalid metadata", "unknown metadata ID", "unlabeled beyond cap"} {
		t.Run(mode, func(t *testing.T) {
			e, h, store := testEngine(t)
			e.start = starter(t, func(ctx context.Context, args []string, raw []byte, out, stderr *os.File) ndexec.TreeResult {
				var req wireRequest
				if err := json.Unmarshal(raw, &req); !assert.NoError(t, err) {
					return ndexec.TreeResult{Drained: true, Err: err}
				}
				assert.Contains(t, req.Extensions, captureDetailsFeature)
				if err := os.MkdirAll(filepath.Join(req.WorkDir, "output"), 0700); !assert.NoError(t, err) {
					return ndexec.TreeResult{Drained: true, Err: err}
				}
				count := 2
				if mode == "unlabeled beyond cap" {
					count = 150
				}
				_, err := fmt.Fprintln(out, `{"type":"extension_init","extension_init":{"selected":["capture_details.v1"]}}`)
				assert.NoError(t, err)
				var candidates []synthetic.Capture
				for i := 0; i < count; i++ {
					id := fmt.Sprintf("s%d", i)
					testID := fmt.Sprintf("t%d", i)
					name := "output/" + id + ".png"
					body := []byte("\x89PNG\r\n\x1a\nfixture")
					if mode == "oversize" && i == 1 {
						body = append(body, []byte(strings.Repeat("x", int(artifacts.FetchMaxBytes)))...)
					}
					if err := os.WriteFile(filepath.Join(req.WorkDir, name), body, 0600); !assert.NoError(t, err) {
						return ndexec.TreeResult{Drained: true, Err: err}
					}
					candidates = append(candidates, synthetic.Capture{ID: id, Kind: "screenshot", Path: name, MIME: "image/png"})
					_, err = fmt.Fprintln(out, captureRecord(captureTest(testID, 1, 0)), "\n"+captureRecord(captureLink(id, testID)))
					assert.NoError(t, err)
				}
				if mode == "invalid metadata" {
					_, err = fmt.Fprintln(out, captureRecord(map[string]any{"record": "future"}))
					assert.NoError(t, err)
				}
				if mode == "unknown metadata ID" {
					_, err = fmt.Fprintln(out, captureRecord(captureLink("unoffered", "t0")))
					assert.NoError(t, err)
				}
				if mode == "missing terminal" {
					return ndexec.TreeResult{Drained: true}
				}
				_, err = fmt.Fprintln(out, captureRecord(map[string]any{"record": "complete"}))
				assert.NoError(t, err)
				r := reporterResult{Status: synthetic.Failed, Error: "assertion", CaptureState: "available", Artifacts: candidates, Tests: &synthetic.TestCounts{Declared: count, Failed: count}}
				assert.Zero(t, store.finalized.Load())
				return resultPayload(r)(ctx, args, raw, out, stderr)
			})
			req := journey()
			req.Capture = true
			ex := e.Execute(t.Context(), req, nil)
			require.True(t, ex.Drained)
			require.NotNil(t, ex.Run.Captures)
			assert.Equal(t, ex.Run.Captures, h.runs[len(h.runs)-1].Captures)
			stored := h.runs[len(h.runs)-1].Artifacts
			require.Len(t, stored, len(ex.Run.Artifacts))
			for i, artifact := range ex.Run.Artifacts {
				assert.Equal(t, artifact, stored[i])
			}
			switch mode {
			case "missing terminal":
				assert.Equal(t, synthetic.Error, ex.Run.Outcome)
				assert.Empty(t, ex.Run.Artifacts)
				assert.Equal(t, "partial", ex.Run.Captures.Reporting)
				assert.Equal(t, []synthetic.CaptureOmission{{ID: "s0", Reason: "not_in_final_report"}, {ID: "s1", Reason: "not_in_final_report"}}, ex.Run.Captures.Unpublished)
			case "oversize":
				assert.Equal(t, synthetic.Failed, ex.Run.Outcome)
				assert.Empty(t, ex.Run.Artifacts)
				assert.Equal(t, "unavailable", ex.Run.CaptureState)
				assert.Equal(t, []synthetic.CaptureOmission{{ID: "s0", Reason: "publication_failed"}, {ID: "s1", Reason: "too_large"}}, ex.Run.Captures.Unpublished)
			default:
				assert.Equal(t, synthetic.Failed, ex.Run.Outcome)
				require.NotEmpty(t, ex.Run.Artifacts)
				assert.Equal(t, "t0", ex.Run.Artifacts[0].Test.ID)
				m, err := store.Manifest(t.Context(), ex.Run.JobID, ex.Run.ID)
				require.NoError(t, err)
				assert.Equal(t, ex.Run.Artifacts, m.Artifacts)
				if mode == "invalid metadata" {
					assert.Equal(t, "invalid", ex.Run.Captures.Reporting)
				}
				if mode == "unknown metadata ID" {
					assert.Equal(t, []synthetic.CaptureOmission{{ID: "unoffered", Reason: "not_in_final_report"}}, ex.Run.Captures.Unpublished)
				}
				if mode == "unlabeled beyond cap" {
					require.Len(t, ex.Run.Artifacts, 150)
					assert.True(t, ex.Run.Captures.Truncated)
					assert.Empty(t, ex.Run.Artifacts[149].Test.ID)
				}
			}
		})
	}
}

func TestLighthouseCaptureOmissionNeedsNoRuntimeFeature(t *testing.T) {
	e, _, _ := testEngine(t)
	e.start = starter(t, func(ctx context.Context, args []string, raw []byte, out, stderr *os.File) ndexec.TreeResult {
		var req wireRequest
		if err := json.Unmarshal(raw, &req); !assert.NoError(t, err) {
			return ndexec.TreeResult{Drained: true, Err: err}
		}
		assert.Empty(t, req.Extensions)
		if err := os.MkdirAll(filepath.Join(req.WorkDir, "output"), 0700); !assert.NoError(t, err) {
			return ndexec.TreeResult{Drained: true, Err: err}
		}
		if err := os.WriteFile(filepath.Join(req.WorkDir, "output/report.html"), []byte(strings.Repeat("x", int(artifacts.FetchMaxBytes)+1)), 0600); !assert.NoError(t, err) {
			return ndexec.TreeResult{Drained: true, Err: err}
		}
		performance := 90.0
		r := reporterResult{Status: synthetic.Success, CaptureState: "available", Metrics: &synthetic.LabMetrics{Performance: &performance}, Artifacts: []synthetic.Capture{{ID: "lighthouse-report", Kind: "report", MIME: "text/html", Path: "output/report.html"}}}
		return resultPayload(r)(ctx, args, raw, out, stderr)
	})
	ex := e.Execute(t.Context(), synthetic.Request{Kind: synthetic.Lighthouse, Name: "audit", URL: "https://example.org", Capture: true, Timeout: time.Second}, nil)
	require.True(t, ex.Drained)
	assert.Equal(t, synthetic.Success, ex.Run.Outcome)
	assert.Empty(t, ex.Run.Artifacts)
	assert.Equal(t, "unavailable", ex.Run.CaptureState)
	require.NotNil(t, ex.Run.Captures)
	assert.Empty(t, ex.Run.Captures.Reporting)
	assert.Equal(t, []synthetic.CaptureOmission{{ID: "lighthouse-report", Reason: "too_large"}}, ex.Run.Captures.Unpublished)
}
