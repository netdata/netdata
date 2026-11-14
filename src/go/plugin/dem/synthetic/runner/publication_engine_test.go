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
	synthetichistory "github.com/netdata/netdata/go/plugins/plugin/dem/synthetic/history"
	"github.com/netdata/netdata/go/plugins/plugin/dem/synthetic/query"
	"github.com/netdata/netdata/go/plugins/plugin/dem/synthetic/registry"
	"github.com/netdata/netdata/go/plugins/plugin/go.d/pkg/ndexec"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestCaptureReportingThroughEnginePublication(t *testing.T) {
	for _, mode := range []string{"published", "missing terminal", "oversize", "oversize first", "all oversize", "invalid after oversize", "invalid metadata", "unknown metadata ID", "unlabeled beyond cap"} {
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
					if mode == "oversize" && i == 1 || mode == "all oversize" ||
						(mode == "oversize first" || mode == "invalid after oversize") && i == 0 {
						body = append(body, []byte(strings.Repeat("x", int(artifacts.FetchMaxBytes)))...)
					}
					if mode == "invalid after oversize" && i == 1 {
						body = []byte("not a PNG")
					}
					if err := os.WriteFile(filepath.Join(req.WorkDir, name), body, 0600); !assert.NoError(t, err) {
						return ndexec.TreeResult{Drained: true, Err: err}
					}
					candidates = append(candidates, synthetic.Capture{ID: id, Kind: "screenshot", Path: name, MIME: "image/png"})
					_, err = fmt.Fprintln(out, extensionLine(captureDetailsFeature, captureTest(testID, 1, 0)), "\n"+extensionLine(captureDetailsFeature, captureLink(id, testID)))
					assert.NoError(t, err)
				}
				if mode == "invalid metadata" {
					_, err = fmt.Fprintln(out, extensionLine(captureDetailsFeature, map[string]any{"record": "future"}))
					assert.NoError(t, err)
				}
				if mode == "unknown metadata ID" {
					_, err = fmt.Fprintln(out, extensionLine(captureDetailsFeature, captureLink("unoffered", "t0")))
					assert.NoError(t, err)
				}
				if mode == "missing terminal" {
					return ndexec.TreeResult{Drained: true}
				}
				_, err = fmt.Fprintln(out, extensionLine(captureDetailsFeature, map[string]any{"record": "complete"}))
				assert.NoError(t, err)
				r := reporterResult{Status: synthetic.Failed, Error: "assertion", CaptureState: "available", Artifacts: candidates, Tests: &synthetic.TestCounts{Declared: count, Failed: count}}
				assert.Zero(t, store.finalized.Load())
				return resultPayload(r)(ctx, args, raw, out, stderr)
			})
			req := journey()
			req.Capture = true
			// This test writes up to 150 files; deadline behavior is tested separately.
			req.Timeout = 30 * time.Second
			ex := e.execute(t.Context(), req)
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
			case "oversize", "oversize first":
				publishedID, testID, skippedID := "s0", "t0", "s1"
				if mode == "oversize first" {
					publishedID, testID, skippedID = "s1", "t1", "s0"
				}
				assert.Equal(t, synthetic.Failed, ex.Run.Outcome)
				assert.Equal(t, "assertion", ex.Run.Error)
				assert.Equal(t, synthetic.CaptureAvailable, ex.Run.CaptureState)
				require.Len(t, ex.Run.Artifacts, 1)
				assert.Equal(t, publishedID, ex.Run.Artifacts[0].ID)
				assert.Equal(t, testID, ex.Run.Artifacts[0].Test.ID)
				assert.Equal(t, []synthetic.CaptureOmission{{ID: skippedID, Reason: "too_large"}}, ex.Run.Captures.Unpublished)
			case "all oversize":
				assert.Equal(t, synthetic.Failed, ex.Run.Outcome)
				assert.Equal(t, "assertion", ex.Run.Error)
				assert.Empty(t, ex.Run.Artifacts)
				assert.Equal(t, synthetic.CaptureUnavailable, ex.Run.CaptureState)
				assert.Equal(t, []synthetic.CaptureOmission{{ID: "s0", Reason: "too_large"}, {ID: "s1", Reason: "too_large"}}, ex.Run.Captures.Unpublished)
			case "invalid after oversize":
				assert.Equal(t, synthetic.Failed, ex.Run.Outcome)
				assert.Contains(t, ex.Run.Error, "artifact publication:")
				assert.Empty(t, ex.Run.Artifacts)
				assert.Equal(t, synthetic.CaptureUnavailable, ex.Run.CaptureState)
				assert.Equal(t, []synthetic.CaptureOmission{{ID: "s0", Reason: "publication_failed"}, {ID: "s1", Reason: "publication_failed"}}, ex.Run.Captures.Unpublished)
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
			if strings.Contains(mode, "oversize") {
				service := query.New(activeRun(ex.Run), noRunHistory{}, store.Store, func() string { return "" })
				evidence, err := service.Evidence(t.Context(), ex.Run.JobID, ex.Run.ID, "")
				require.NoError(t, err)
				assert.Equal(t, ex.Run.CaptureState, evidence.CaptureState)
				require.Len(t, evidence.Artifacts, len(ex.Run.Artifacts))
				for i, a := range evidence.Artifacts {
					assert.Equal(t, ex.Run.Artifacts[i], a.Artifact)
				}
				for _, omission := range ex.Run.Captures.Unpublished {
					_, err = service.Evidence(t.Context(), ex.Run.JobID, ex.Run.ID, omission.ID)
					assert.ErrorIs(t, err, query.ErrArtifactNotRecorded)
				}
				detail, err := service.Run(t.Context(), ex.Run.JobID, ex.Run.ID)
				require.NoError(t, err)
				assert.Equal(t, ex.Run.Captures, detail.Captures)
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
	ex := e.execute(t.Context(), synthetic.Request{Kind: synthetic.Lighthouse, Name: "audit", URL: "https://example.org", Capture: true, Timeout: time.Second})
	require.True(t, ex.Drained)
	assert.Equal(t, synthetic.Success, ex.Run.Outcome)
	assert.Empty(t, ex.Run.Error)
	require.NotNil(t, ex.Run.Metrics)
	require.NotNil(t, ex.Run.Metrics.Performance)
	assert.Equal(t, 90.0, *ex.Run.Metrics.Performance)
	assert.Empty(t, ex.Run.Artifacts)
	assert.Equal(t, synthetic.CaptureUnavailable, ex.Run.CaptureState)
	require.NotNil(t, ex.Run.Captures)
	assert.Empty(t, ex.Run.Captures.Reporting)
	assert.Equal(t, []synthetic.CaptureOmission{{ID: "lighthouse-report", Reason: "too_large"}}, ex.Run.Captures.Unpublished)
}

func TestTraceReportingThroughEnginePublication(t *testing.T) {
	for _, mode := range []string{"selected", "old runtime", "declined", "malformed init", "invalid capture details", "oversize"} {
		t.Run(mode, func(t *testing.T) {
			e, h, store := testEngine(t)
			e.start = starter(t, func(ctx context.Context, args []string, raw []byte, out, stderr *os.File) ndexec.TreeResult {
				var req wireRequest
				if err := json.Unmarshal(raw, &req); !assert.NoError(t, err) {
					return ndexec.TreeResult{Drained: true, Err: err}
				}
				assert.Contains(t, req.Extensions, failureTraceFeature)
				assert.Contains(t, req.Extensions, captureDetailsFeature)
				if err := os.MkdirAll(filepath.Join(req.WorkDir, "output"), 0700); !assert.NoError(t, err) {
					return ndexec.TreeResult{Drained: true, Err: err}
				}
				selection := `{"type":"extension_init","extension_init":{"selected":["capture_details.v1","failure_trace.v1"]}}`
				if mode == "old runtime" {
					selection = ""
				}
				if mode == "declined" {
					selection = `{"type":"extension_init","extension_init":{"selected":["capture_details.v1"]}}`
				}
				if mode == "malformed init" {
					selection = `{"type":"extension_init","extension_init":{"selected":null}}`
				}
				if selection != "" {
					_, err := fmt.Fprintln(out, selection)
					assert.NoError(t, err)
				}
				if mode != "old runtime" {
					_, err := fmt.Fprintln(out, extensionLine(captureDetailsFeature, captureTest("t1", 1, 0)), "\n"+extensionLine(captureDetailsFeature, captureLink("screen", "t1")), "\n"+extensionLine(captureDetailsFeature, captureLink("trace1", "t1")))
					assert.NoError(t, err)
					if mode == "invalid capture details" {
						_, err = fmt.Fprintln(out, extensionLine(captureDetailsFeature, map[string]any{"record": "future"}))
						assert.NoError(t, err)
					}
					_, err = fmt.Fprintln(out, extensionLine(captureDetailsFeature, map[string]any{"record": "complete"}))
					assert.NoError(t, err)
				}
				for path, body := range map[string][]byte{"output/screen.png": []byte("\x89PNG\r\n\x1a\nfixture"), "output/trace.zip": []byte("PK\x03\x04fixture")} {
					if err := os.WriteFile(filepath.Join(req.WorkDir, path), body, 0600); !assert.NoError(t, err) {
						return ndexec.TreeResult{Drained: true, Err: err}
					}
				}
				if mode == "oversize" {
					err := os.Truncate(filepath.Join(req.WorkDir, "output/trace.zip"), artifacts.FetchMaxBytes+1)
					assert.NoError(t, err)
				}
				r := reporterResult{Status: synthetic.Failed, Tests: &synthetic.TestCounts{Declared: 1, Failed: 1}, CaptureState: "available", Artifacts: []synthetic.Capture{{ID: "screen", Kind: "screenshot", MIME: "image/png", Path: "output/screen.png"}, {ID: "trace1", Kind: "trace", MIME: "application/zip", Path: "output/trace.zip"}}}
				if mode == "old runtime" {
					r.Artifacts = r.Artifacts[:1]
				}
				return resultPayload(r)(ctx, args, raw, out, stderr)
			})
			req := journey()
			req.Capture = true
			req.Trace = true
			ex := e.execute(t.Context(), req)
			require.True(t, ex.Drained)
			assert.Equal(t, synthetic.Failed, ex.Run.Outcome)
			require.NotNil(t, ex.Run.Captures)
			want := "supported"
			if mode == "old runtime" {
				want = "negotiation_not_reported"
			}
			if mode == "declined" {
				want = "unsupported"
			}
			if mode == "malformed init" {
				want = "invalid"
			}
			assert.Equal(t, want, ex.Run.Captures.Trace)
			assert.Equal(t, ex.Run.Captures, h.runs[len(h.runs)-1].Captures)
			count := 2
			if mode == "old runtime" {
				count = 1
				assert.Equal(t, "negotiation_not_reported", ex.Run.Captures.Reporting)
				assert.Empty(t, ex.Run.Captures.Tests)
			}
			if mode == "oversize" {
				count = 1
				assert.Equal(t, []synthetic.CaptureOmission{{ID: "trace1", Reason: "too_large"}}, ex.Run.Captures.Unpublished)
			}
			require.Len(t, ex.Run.Artifacts, count)
			if mode == "old runtime" {
				assert.Equal(t, synthetic.ScreenshotCapture, ex.Run.Artifacts[0].Kind)
				assert.Empty(t, ex.Run.Artifacts[0].Test)
			}
			if mode != "old runtime" && mode != "malformed init" {
				assert.Equal(t, int64(1), ex.Run.Captures.Tests[0].Accepted)
				assert.Equal(t, "t1", ex.Run.Artifacts[0].Test.ID)
			}
			if count == 2 {
				meta, body, err := store.Fetch(t.Context(), ex.Run.JobID, ex.Run.ID, "trace1")
				require.NoError(t, err)
				assert.Equal(t, synthetic.TraceCapture, meta.Kind)
				assert.Equal(t, []byte("PK\x03\x04fixture"), body)
			}
		})
	}
}

// activeRun is an active job whose latest run is the wrapped run.
type activeRun synthetic.Run

func (r activeRun) Snapshot(time.Time) []registry.Job { return nil }

func (r activeRun) Latest(jobID, runID string) (synthetic.Run, bool) {
	return synthetic.Run(r), jobID == r.JobID && runID == r.ID
}

// noRunHistory retains no runs: evidence comes from the capture manifest and the active run.
type noRunHistory struct{}

func (noRunHistory) QueryRuns(context.Context, synthetichistory.RunFilter) (synthetichistory.RunPage, error) {
	return synthetichistory.RunPage{}, nil
}

func (noRunHistory) GetRun(context.Context, string, string) (synthetic.Run, error) {
	return synthetic.Run{}, os.ErrNotExist
}
