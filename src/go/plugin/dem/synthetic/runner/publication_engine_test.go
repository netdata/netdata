//go:build linux || darwin || freebsd

// SPDX-License-Identifier: GPL-3.0-or-later

package runner

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/netdata/netdata/go/plugins/plugin/dem/synthetic"
	"github.com/netdata/netdata/go/plugins/plugin/dem/synthetic/artifacts"
	"github.com/netdata/netdata/go/plugins/plugin/go.d/pkg/ndexec"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// Candidate bodies a capture-reporting runtime writes.
const (
	pngBody      = iota // a small valid screenshot
	oversizeBody        // a valid screenshot above the fetch limit
	invalidBody         // a file without the PNG signature
)

// screenshotFixture is the content of every published screenshot.
var screenshotFixture = []byte("\x89PNG\r\n\x1a\nfixture")

// captureTests returns the capture-details test records t0..t(n-1) of a capture-reporting runtime.
func captureTests(n int) []synthetic.CaptureTest {
	var tests []synthetic.CaptureTest
	for i := range n {
		tests = append(tests, synthetic.CaptureTest{
			ArtifactTest: synthetic.ArtifactTest{
				ID:    fmt.Sprintf("t%d", i),
				Title: "Checkout",
			},
			Status:         "failed",
			ExpectedStatus: "passed",
			Accepted:       1,
		})
	}
	return tests
}

// captureLinks returns the associations of screenshots s0..s(n-1) with tests t0..t(n-1).
func captureLinks(n int) []synthetic.CaptureAssociation {
	var links []synthetic.CaptureAssociation
	for i := range n {
		links = append(links, synthetic.CaptureAssociation{
			ID:     fmt.Sprintf("s%d", i),
			TestID: fmt.Sprintf("t%d", i),
		})
	}
	return links
}

// screenshots returns published screenshots s<first>..s<last>; the first labeled ones carry their test.
func screenshots(first, last, labeled int) []synthetic.Artifact {
	digest := sha256.Sum256(screenshotFixture)
	var published []synthetic.Artifact
	for i := first; i <= last; i++ {
		artifact := synthetic.Artifact{
			ID:     fmt.Sprintf("s%d", i),
			Kind:   synthetic.ScreenshotCapture,
			MIME:   "image/png",
			Bytes:  int64(len(screenshotFixture)),
			SHA256: hex.EncodeToString(digest[:]),
		}
		if i < labeled {
			artifact.Test = synthetic.ArtifactTest{
				ID:    fmt.Sprintf("t%d", i),
				Title: "Checkout",
			}
		}
		published = append(published, artifact)
	}
	return published
}

// Each reported screenshot s<i> has a capture-details test t<i> and a link; the runtime then writes the extra frames
// and, unless the case drops it, the completion record and a failed result offering every screenshot.
func TestCaptureReportingThroughEnginePublication(t *testing.T) {
	for name, tc := range map[string]struct {
		bodies        []int
		frames        []string
		withoutResult bool
		outcome       synthetic.Outcome
		err           string
		state         synthetic.CaptureState
		artifacts     []synthetic.Artifact
		captures      synthetic.CaptureReport
	}{
		"published": {
			bodies:    []int{pngBody, pngBody},
			outcome:   synthetic.Failed,
			err:       "assertion",
			state:     synthetic.CaptureAvailable,
			artifacts: screenshots(0, 1, 2),
			captures: synthetic.CaptureReport{
				Reporting:    "complete",
				Tests:        captureTests(2),
				Associations: captureLinks(2),
			},
		},
		"missing terminal": {
			bodies:        []int{pngBody, pngBody},
			withoutResult: true,
			outcome:       synthetic.Error,
			err:           "runner exited without a terminal result: no result",
			state:         synthetic.CaptureUnavailable,
			captures: synthetic.CaptureReport{
				Reporting:    "partial",
				Tests:        captureTests(2),
				Associations: captureLinks(2),
				Unpublished: []synthetic.CaptureOmission{
					{ID: "s0", Reason: "not_in_final_report"},
					{ID: "s1", Reason: "not_in_final_report"},
				},
			},
		},
		"oversize": {
			bodies:    []int{pngBody, oversizeBody},
			outcome:   synthetic.Failed,
			err:       "assertion",
			state:     synthetic.CaptureAvailable,
			artifacts: screenshots(0, 0, 2),
			captures: synthetic.CaptureReport{
				Reporting:    "complete",
				Tests:        captureTests(2),
				Associations: captureLinks(2),
				Unpublished:  []synthetic.CaptureOmission{{ID: "s1", Reason: "too_large"}},
			},
		},
		"oversize first": {
			bodies:    []int{oversizeBody, pngBody},
			outcome:   synthetic.Failed,
			err:       "assertion",
			state:     synthetic.CaptureAvailable,
			artifacts: screenshots(1, 1, 2),
			captures: synthetic.CaptureReport{
				Reporting:    "complete",
				Tests:        captureTests(2),
				Associations: captureLinks(2),
				Unpublished:  []synthetic.CaptureOmission{{ID: "s0", Reason: "too_large"}},
			},
		},
		"all oversize": {
			bodies:  []int{oversizeBody, oversizeBody},
			outcome: synthetic.Failed,
			err:     "assertion",
			state:   synthetic.CaptureUnavailable,
			captures: synthetic.CaptureReport{
				Reporting:    "complete",
				Tests:        captureTests(2),
				Associations: captureLinks(2),
				Unpublished: []synthetic.CaptureOmission{
					{ID: "s0", Reason: "too_large"},
					{ID: "s1", Reason: "too_large"},
				},
			},
		},
		"invalid after oversize": {
			bodies:  []int{oversizeBody, invalidBody},
			outcome: synthetic.Failed,
			err:     "assertion; artifact publication: " + artifacts.ErrInvalidCapture.Error() + ": invalid screenshot signature",
			state:   synthetic.CaptureUnavailable,
			captures: synthetic.CaptureReport{
				Reporting:    "complete",
				Tests:        captureTests(2),
				Associations: captureLinks(2),
				Unpublished: []synthetic.CaptureOmission{
					{ID: "s0", Reason: "publication_failed"},
					{ID: "s1", Reason: "publication_failed"},
				},
			},
		},
		"invalid metadata": {
			bodies:    []int{pngBody, pngBody},
			frames:    []string{extensionLine(captureDetailsFeature, map[string]any{"record": "future"})},
			outcome:   synthetic.Failed,
			err:       "assertion",
			state:     synthetic.CaptureAvailable,
			artifacts: screenshots(0, 1, 2),
			captures: synthetic.CaptureReport{
				Reporting:    "invalid",
				Tests:        captureTests(2),
				Associations: captureLinks(2),
			},
		},
		"unknown metadata ID": {
			bodies:    []int{pngBody, pngBody},
			frames:    []string{extensionLine(captureDetailsFeature, captureLink("unoffered", "t0"))},
			outcome:   synthetic.Failed,
			err:       "assertion",
			state:     synthetic.CaptureAvailable,
			artifacts: screenshots(0, 1, 2),
			captures: synthetic.CaptureReport{
				Reporting: "complete",
				Tests:     captureTests(2),
				Associations: append(captureLinks(2), synthetic.CaptureAssociation{
					ID:     "unoffered",
					TestID: "t0",
				}),
				Unpublished: []synthetic.CaptureOmission{{ID: "unoffered", Reason: "not_in_final_report"}},
			},
		},
		// The detail budget holds 128 test and link pairs; later screenshots still publish, unlabeled.
		"unlabeled beyond cap": {
			bodies:    slices.Repeat([]int{pngBody}, 150),
			outcome:   synthetic.Failed,
			err:       "assertion",
			state:     synthetic.CaptureAvailable,
			artifacts: screenshots(0, 149, 128),
			captures: synthetic.CaptureReport{
				Reporting:    "complete",
				Truncated:    true,
				Tests:        captureTests(128),
				Associations: captureLinks(128),
			},
		},
	} {
		t.Run(name, func(t *testing.T) {
			e, h, store := testEngine(t)
			e.start = starter(
				t,
				func(ctx context.Context, args []string, raw []byte, out, stderr *os.File) ndexec.TreeResult {
					var req wireRequest
					if err := json.Unmarshal(raw, &req); !assert.NoError(t, err) {
						return ndexec.TreeResult{
							Drained: true,
							Err:     err,
						}
					}
					assert.Contains(t, req.Extensions, captureDetailsFeature)
					if err := os.MkdirAll(filepath.Join(req.WorkDir, "output"), 0700); !assert.NoError(t, err) {
						return ndexec.TreeResult{
							Drained: true,
							Err:     err,
						}
					}
					emitLines(t, out, `{"type":"extension_init","extension_init":{"selected":["capture_details.v1"]}}`)
					var candidates []synthetic.Capture
					for i, body := range tc.bodies {
						id, testID := fmt.Sprintf("s%d", i), fmt.Sprintf("t%d", i)
						name := "output/" + id + ".png"
						if err := writeCaptureBody(filepath.Join(req.WorkDir, name), body); !assert.NoError(t, err) {
							return ndexec.TreeResult{
								Drained: true,
								Err:     err,
							}
						}
						candidates = append(candidates, synthetic.Capture{
							ID:   id,
							Kind: synthetic.ScreenshotCapture,
							Path: name,
							MIME: "image/png",
						})
						emitLines(
							t,
							out,
							extensionLine(captureDetailsFeature, captureTest(testID, 1, 0)),
							extensionLine(captureDetailsFeature, captureLink(id, testID)),
						)
					}
					emitLines(t, out, tc.frames...)
					if tc.withoutResult {
						return ndexec.TreeResult{
							Drained: true,
						}
					}
					emitLines(t, out, extensionLine(captureDetailsFeature, map[string]any{"record": "complete"}))
					assert.Zero(t, store.finalized.Load(), "publication follows the result")
					return resultPayload(reporterResult{
						Status:       synthetic.Failed,
						Error:        "assertion",
						CaptureState: "available",
						Artifacts:    candidates,
						Tests: &synthetic.TestCounts{
							Declared: len(tc.bodies),
							Failed:   len(tc.bodies),
						},
					})(ctx, args, raw, out, stderr)
				},
			)
			req := journey()
			req.Capture = true
			// This test writes up to 150 files; deadline behavior is tested separately.
			req.Timeout = 30 * time.Second
			ex := e.execute(t.Context(), req)
			require.True(t, ex.Drained)
			assert.Equal(t, tc.outcome, ex.Run.Outcome)
			assert.Equal(t, tc.err, ex.Run.Error)
			assert.Equal(t, tc.state, ex.Run.CaptureState)
			assert.Equal(t, tc.artifacts, ex.Run.Artifacts)
			assert.Equal(t, &tc.captures, ex.Run.Captures)
			completed := h.runs[len(h.runs)-1]
			assert.Equal(t, tc.artifacts, completed.Artifacts)
			assert.Equal(t, &tc.captures, completed.Captures)
			assertManifest(t, store, ex.Run, tc.artifacts)
		})
	}
}

// emitLines writes one reporter line per value.
func emitLines(t *testing.T, out *os.File, lines ...string) {
	t.Helper()
	for _, line := range lines {
		_, err := fmt.Fprintln(out, line)
		assert.NoError(t, err)
	}
}

func writeCaptureBody(path string, body int) error {
	switch body {
	case invalidBody:
		return os.WriteFile(path, []byte("not a PNG"), 0600)
	case oversizeBody:
		if err := os.WriteFile(path, screenshotFixture, 0600); err != nil {
			return err
		}
		return os.Truncate(path, artifacts.FetchMaxBytes+1)
	default:
		return os.WriteFile(path, screenshotFixture, 0600)
	}
}

// assertManifest checks the capture manifest evidence reads membership from; nothing published leaves none.
func assertManifest(t *testing.T, store *observedArtifacts, run synthetic.Run, published []synthetic.Artifact) {
	t.Helper()
	m, err := store.Manifest(t.Context(), run.JobID, run.ID)
	if len(published) == 0 {
		assert.ErrorIs(t, err, artifacts.ErrNotFound)
		return
	}
	require.NoError(t, err)
	assert.Equal(t, published, m.Artifacts)
}

func TestLighthouseCaptureOmissionNeedsNoRuntimeFeature(t *testing.T) {
	e, _, _ := testEngine(t)
	e.start = starter(t, func(ctx context.Context, args []string, raw []byte, out, stderr *os.File) ndexec.TreeResult {
		var req wireRequest
		if err := json.Unmarshal(raw, &req); !assert.NoError(t, err) {
			return ndexec.TreeResult{
				Drained: true,
				Err:     err,
			}
		}
		assert.Empty(t, req.Extensions)
		if err := os.MkdirAll(filepath.Join(req.WorkDir, "output"), 0700); !assert.NoError(t, err) {
			return ndexec.TreeResult{
				Drained: true,
				Err:     err,
			}
		}
		if err := os.WriteFile(filepath.Join(req.WorkDir, "output/report.html"), []byte(strings.Repeat("x", int(artifacts.FetchMaxBytes)+1)), 0600); !assert.NoError(
			t,
			err,
		) {
			return ndexec.TreeResult{
				Drained: true,
				Err:     err,
			}
		}
		performance := 90.0
		r := reporterResult{
			Status:       synthetic.Success,
			CaptureState: "available",
			Metrics: &synthetic.LabMetrics{
				Performance: &performance,
			},
			Artifacts: []synthetic.Capture{
				{ID: "lighthouse-report", Kind: "report", MIME: "text/html", Path: "output/report.html"},
			},
		}
		return resultPayload(r)(ctx, args, raw, out, stderr)
	})
	ex := e.execute(
		t.Context(),
		synthetic.Request{
			Kind:    synthetic.Lighthouse,
			Name:    "audit",
			URL:     "https://example.org",
			Capture: true,
			Timeout: time.Second,
		},
	)
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
	assert.Equal(
		t,
		[]synthetic.CaptureOmission{{ID: "lighthouse-report", Reason: "too_large"}},
		ex.Run.Captures.Unpublished,
	)
}

// Every case requests captures and traces. The runtime reports a failed test t1 with a screenshot and, unless it
// predates capture details, a trace; the selection line and the capture-details records vary.
func TestTraceReportingThroughEnginePublication(t *testing.T) {
	const bothFeatures = `{"type":"extension_init","extension_init":{"selected":["capture_details.v1","failure_trace.v1"]}}`
	fixtures := map[string][]byte{
		"screen": screenshotFixture,
		"trace1": []byte("PK\x03\x04fixture"),
	}
	published := func(id string, kind synthetic.CaptureKind, mime string, labeled bool) synthetic.Artifact {
		digest := sha256.Sum256(fixtures[id])
		artifact := synthetic.Artifact{
			ID:     id,
			Kind:   kind,
			MIME:   mime,
			Bytes:  int64(len(fixtures[id])),
			SHA256: hex.EncodeToString(digest[:]),
		}
		if labeled {
			artifact.Test = synthetic.ArtifactTest{
				ID:    "t1",
				Title: "Checkout",
			}
		}
		return artifact
	}
	screen := func(labeled bool) synthetic.Artifact {
		return published("screen", synthetic.ScreenshotCapture, "image/png", labeled)
	}
	trace := func(labeled bool) synthetic.Artifact {
		return published("trace1", synthetic.TraceCapture, "application/zip", labeled)
	}
	recorded := synthetic.CaptureReport{
		Tests: []synthetic.CaptureTest{{
			ArtifactTest: synthetic.ArtifactTest{
				ID:    "t1",
				Title: "Checkout",
			},
			Status:         "failed",
			ExpectedStatus: "passed",
			Accepted:       1,
		}},
		Associations: []synthetic.CaptureAssociation{
			{ID: "screen", TestID: "t1"},
			{ID: "trace1", TestID: "t1"},
		},
	}
	report := func(trace, reporting string, unpublished ...synthetic.CaptureOmission) *synthetic.CaptureReport {
		out := synthetic.CloneCaptureReport(&recorded)
		out.Trace, out.Reporting, out.Unpublished = trace, reporting, unpublished
		return out
	}
	for name, tc := range map[string]struct {
		selection     string   // extension_init line; empty for a runtime without extensions
		records       bool     // the runtime writes capture-details records
		frames        []string // extra capture-details records before completion
		oversizeTrace bool
		artifacts     []synthetic.Artifact
		captures      *synthetic.CaptureReport
	}{
		"selected": {
			selection: bothFeatures,
			records:   true,
			artifacts: []synthetic.Artifact{screen(true), trace(true)},
			captures:  report("supported", "complete"),
		},
		"old runtime": {
			artifacts: []synthetic.Artifact{screen(false)},
			captures: &synthetic.CaptureReport{
				Trace:     "unavailable",
				Reporting: "unavailable",
			},
		},
		"declined": {
			selection: `{"type":"extension_init","extension_init":{"selected":["capture_details.v1"]}}`,
			records:   true,
			artifacts: []synthetic.Artifact{screen(true), trace(true)},
			captures:  report("unsupported", "complete"),
		},
		"malformed init": {
			selection: `{"type":"extension_init","extension_init":{"selected":null}}`,
			records:   true,
			artifacts: []synthetic.Artifact{screen(false), trace(false)},
			captures: &synthetic.CaptureReport{
				Trace:     "invalid",
				Reporting: "invalid",
			},
		},
		"invalid capture details": {
			selection: bothFeatures,
			records:   true,
			frames:    []string{extensionLine(captureDetailsFeature, map[string]any{"record": "future"})},
			artifacts: []synthetic.Artifact{screen(true), trace(true)},
			captures:  report("supported", "invalid"),
		},
		"oversize": {
			selection:     bothFeatures,
			records:       true,
			oversizeTrace: true,
			artifacts:     []synthetic.Artifact{screen(true)},
			captures: report("supported", "complete", synthetic.CaptureOmission{
				ID:     "trace1",
				Reason: "too_large",
			}),
		},
	} {
		t.Run(name, func(t *testing.T) {
			e, h, store := testEngine(t)
			e.start = starter(
				t,
				func(ctx context.Context, args []string, raw []byte, out, stderr *os.File) ndexec.TreeResult {
					var req wireRequest
					if err := json.Unmarshal(raw, &req); !assert.NoError(t, err) {
						return ndexec.TreeResult{
							Drained: true,
							Err:     err,
						}
					}
					assert.Contains(t, req.Extensions, failureTraceFeature)
					assert.Contains(t, req.Extensions, captureDetailsFeature)
					if err := os.MkdirAll(filepath.Join(req.WorkDir, "output"), 0700); !assert.NoError(t, err) {
						return ndexec.TreeResult{
							Drained: true,
							Err:     err,
						}
					}
					if tc.selection != "" {
						emitLines(t, out, tc.selection)
					}
					if tc.records {
						emitLines(
							t,
							out,
							extensionLine(captureDetailsFeature, captureTest("t1", 1, 0)),
							extensionLine(captureDetailsFeature, captureLink("screen", "t1")),
							extensionLine(captureDetailsFeature, captureLink("trace1", "t1")),
						)
						emitLines(t, out, tc.frames...)
						emitLines(t, out, extensionLine(captureDetailsFeature, map[string]any{"record": "complete"}))
					}
					for path, body := range map[string][]byte{"output/screen.png": fixtures["screen"], "output/trace.zip": fixtures["trace1"]} {
						if err := os.WriteFile(filepath.Join(req.WorkDir, path), body, 0600); !assert.NoError(t, err) {
							return ndexec.TreeResult{
								Drained: true,
								Err:     err,
							}
						}
					}
					if tc.oversizeTrace {
						err := os.Truncate(filepath.Join(req.WorkDir, "output/trace.zip"), artifacts.FetchMaxBytes+1)
						assert.NoError(t, err)
					}
					offered := []synthetic.Capture{
						{ID: "screen", Kind: synthetic.ScreenshotCapture, MIME: "image/png", Path: "output/screen.png"},
					}
					if tc.records {
						offered = append(offered, synthetic.Capture{
							ID:   "trace1",
							Kind: synthetic.TraceCapture,
							MIME: "application/zip",
							Path: "output/trace.zip",
						})
					}
					return resultPayload(reporterResult{
						Status: synthetic.Failed,
						Tests: &synthetic.TestCounts{
							Declared: 1,
							Failed:   1,
						},
						CaptureState: "available",
						Artifacts:    offered,
					})(ctx, args, raw, out, stderr)
				},
			)
			req := journey()
			req.Capture = true
			req.Trace = true
			ex := e.execute(t.Context(), req)
			require.True(t, ex.Drained)
			assert.Equal(t, synthetic.Failed, ex.Run.Outcome)
			assert.Empty(t, ex.Run.Error)
			assert.Equal(t, synthetic.CaptureAvailable, ex.Run.CaptureState)
			assert.Equal(t, tc.artifacts, ex.Run.Artifacts)
			assert.Equal(t, tc.captures, ex.Run.Captures)
			completed := h.runs[len(h.runs)-1]
			assert.Equal(t, tc.artifacts, completed.Artifacts)
			assert.Equal(t, tc.captures, completed.Captures)
			assertManifest(t, store, ex.Run, tc.artifacts)
			for _, artifact := range tc.artifacts {
				_, body, err := store.Fetch(t.Context(), ex.Run.JobID, ex.Run.ID, artifact.ID)
				require.NoError(t, err)
				assert.Equal(t, fixtures[artifact.ID], body)
			}
		})
	}
}
