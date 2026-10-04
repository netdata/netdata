// SPDX-License-Identifier: GPL-3.0-or-later

package syntheticfunc_test

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"errors"
	"math"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/netdata/netdata/go/plugins/pkg/funcapi"
	"github.com/netdata/netdata/go/plugins/plugin/dem/artifacts"
	"github.com/netdata/netdata/go/plugins/plugin/dem/store"
	model "github.com/netdata/netdata/go/plugins/plugin/dem/synthetic"
	"github.com/netdata/netdata/go/plugins/plugin/dem/syntheticfunc"
	"github.com/santhosh-tekuri/jsonschema/v6"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

type source struct {
	hub       *model.Hub
	history   map[string]model.Run
	filter    model.RunFilter
	page      model.RunPage
	queryErr  error
	getCalls  int
	artifacts *artifacts.Store
	fetchErr  error
}

func (s *source) ArtifactStats() artifacts.Stats {
	if s.artifacts == nil {
		return artifacts.Stats{}
	}
	return s.artifacts.Stats()
}

func (s *source) Snapshot(now time.Time) []model.Job {
	if s.hub == nil {
		return nil
	}
	return s.hub.Snapshot(now)
}
func (s *source) QuerySyntheticRuns(ctx context.Context, f model.RunFilter) (model.RunPage, error) {
	s.filter = f
	return s.page, s.queryErr
}
func (s *source) GetSyntheticRun(ctx context.Context, jobID, runID string) (model.Run, error) {
	s.getCalls++
	r, ok := s.history[jobID+"/"+runID]
	if !ok {
		return model.Run{}, os.ErrNotExist
	}
	return r, nil
}
func (s *source) Manifest(ctx context.Context, runID string) ([]model.Artifact, error) {
	return s.artifacts.Manifest(ctx, runID)
}
func (s *source) Fetch(ctx context.Context, runID, artifactID string) (model.Artifact, []byte, error) {
	if s.fetchErr != nil {
		return model.Artifact{}, nil, s.fetchErr
	}
	return s.artifacts.Fetch(ctx, runID, artifactID)
}
func ptr(n float64) *float64 { return &n }
func call(h *syntheticfunc.Handler, method string, args ...string) *funcapi.FunctionResponse {
	return h.HandleRaw(context.Background(), funcapi.RawMethodRequest{
		Method:      method,
		Args:        args,
		Permissions: "0x7b",
	})
}
func responseData(t *testing.T, r *funcapi.FunctionResponse) [][]any {
	t.Helper()
	require.NotNil(t, r.RawResponse, r.Message)
	require.Equal(t, 200, r.RawResponse["status"])
	rows, ok := r.RawResponse["data"].([][]any)
	require.True(t, ok)
	return rows
}
func rowValues(t *testing.T, r *funcapi.FunctionResponse, row []any) map[string]any {
	t.Helper()
	columns := r.RawResponse["columns"].(map[string]any)
	require.Len(t, row, len(columns))
	out := make(map[string]any)
	for key, raw := range columns {
		idx := raw.(map[string]any)["index"].(int)
		out[key] = row[idx]
	}
	return out
}
func validate(t *testing.T, r *funcapi.FunctionResponse) {
	t.Helper()
	raw, err := os.ReadFile("../../../../plugins.d/FUNCTION_UI_SCHEMA.json")
	require.NoError(t, err)
	var doc any
	require.NoError(t, json.Unmarshal(raw, &doc))
	compiler := jsonschema.NewCompiler()
	require.NoError(t, compiler.AddResource("schema.json", doc))
	schema, err := compiler.Compile("schema.json")
	require.NoError(t, err)
	data, err := json.Marshal(r.RawResponse)
	require.NoError(t, err)
	var payload any
	require.NoError(t, json.Unmarshal(data, &payload))
	require.NoError(t, schema.Validate(payload))
}

func TestInfoNeverReadsDependencies(t *testing.T) {
	h := syntheticfunc.New(nil)
	for _, decl := range syntheticfunc.Declarations() {
		t.Run(decl.ID, func(t *testing.T) {
			assert.True(t, decl.RawRequest)
			assert.True(t, decl.ManagedInfo)
			assert.Equal(t, decl.ID, decl.FunctionName)
			got := h.HandleRaw(
				context.Background(),
				funcapi.RawMethodRequest{
					Method: decl.ID,
					Info:   true,
					Args:   []string{"info"},
				},
			)
			assert.Equal(t, 200, got.Status)
			assert.NotEmpty(t, got.Help)
			assert.NotEmpty(t, got.Columns)
			assert.NotContains(t, decl.AcceptedParams, "__job")
		})
	}
}

func TestPermissionsAndInvalidRequests(t *testing.T) {
	for name, tc := range map[string]struct {
		request funcapi.RawMethodRequest
		status  int
	}{
		"anonymous": {funcapi.RawMethodRequest{
			Method: "synthetics-checks",
		}, 403},
		"partial member": {funcapi.RawMethodRequest{
			Method:      "synthetics-runs",
			Permissions: "0x19",
		}, 403},
		"malformed permissions": {funcapi.RawMethodRequest{
			Method:      "synthetics-runs",
			Permissions: "member",
		}, 403},
		"member cannot fetch": {funcapi.RawMethodRequest{
			Method:      "synthetics-artifact",
			Permissions: "0x1b",
			Args:        []string{"job_id:journey:a", "run_id:r", "artifact_id:screenshot"},
		}, 403},
		"payload": {funcapi.RawMethodRequest{
			Method:      "synthetics-checks",
			Permissions: "0x1b",
			Payload:     []byte("{}"),
		}, 400},
		"unsupported argument": {funcapi.RawMethodRequest{
			Method:      "synthetics-checks",
			Permissions: "0x1b",
			Args:        []string{"password:secret"},
		}, 400},
		"duplicate argument": {funcapi.RawMethodRequest{
			Method:      "synthetics-checks",
			Permissions: "0x1b",
			Args:        []string{"kind:journey", "kind:lighthouse"},
		}, 400},
		"malformed argument": {funcapi.RawMethodRequest{
			Method:      "synthetics-checks",
			Permissions: "0x1b",
			Args:        []string{"kind"},
		}, 400},
		"unknown method": {funcapi.RawMethodRequest{
			Method:      "not-a-function",
			Permissions: "0x7b",
		}, 404},
	} {
		t.Run(name, func(t *testing.T) {
			got := syntheticfunc.New(nil).HandleRaw(context.Background(), tc.request)
			assert.Equal(t, tc.status, got.Status)
		})
	}
	h := syntheticfunc.New(&source{})
	for _, args := range [][]string{{"limit:0"}, {"limit:2001"}, {"limit:NaN"}, {"kind:other"}, {"after:NaN"}, {"after:200", "before:100"}, {"outcome:other"}} {
		got := call(h, "synthetics-runs", args...)
		assert.Equal(t, 400, got.Status)
	}
	assert.Equal(t, 400, call(h, "synthetics-run", "run_id:r").Status)
	assert.Equal(t, 404, call(h, "synthetics-run", "job_id:journey:a", "run_id:missing").Status)
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	assert.Equal(
		t,
		499,
		h.HandleRaw(ctx, funcapi.RawMethodRequest{
			Method:      "synthetics-checks",
			Permissions: "0x1b",
		}).Status,
	)
}

func TestActiveSnapshotPreservesStaleFailureDuringRunning(t *testing.T) {
	hub := model.NewHub()
	registration, err := hub.Register(
		model.Job{
			JobID:          "journey:checkout",
			Name:           "checkout",
			Kind:           model.Journey,
			CadenceSeconds: 1,
		},
	)
	require.NoError(t, err)
	completed := time.Now().Add(-time.Minute).UnixMicro()
	evidence := model.Run{
		ID:           "r",
		JobID:        "journey:checkout",
		Outcome:      model.Failed,
		CompletedUS:  completed,
		DurationMS:   ptr(500.25),
		Error:        "afterAll failed",
		HistoryError: "history write failed",
	}
	registration.Complete(evidence)
	registration.SetState("running")
	h := syntheticfunc.New(&source{
		hub: hub,
	})
	got := call(h, "synthetics-checks")
	rows := responseData(t, got)
	require.Len(t, rows, 1)
	assert.Equal(
		t,
		map[string]any{
			"job_id":          "journey:checkout",
			"kind":            model.Journey,
			"name":            "checkout",
			"target":          "",
			"state":           "running",
			"fresh":           false,
			"run_id":          "r",
			"outcome":         "failed",
			"completed_us":    completed,
			"duration_ms":     500.25,
			"last_success_us": nil,
			"last_failure_us": completed,
			"error":           "afterAll failed",
			"history_error":   "history write failed",
		},
		rowValues(t, got, rows[0]),
	)
	validate(t, got)
	require.Empty(t, responseData(t, call(h, "synthetics-checks", "kind:lighthouse")))
	registration.Retire()
	require.Empty(t, responseData(t, call(h, "synthetics-checks")))
	next, err := hub.Register(
		model.Job{
			JobID:          "journey:checkout",
			Name:           "checkout",
			Kind:           model.Journey,
			CadenceSeconds: 1,
		},
	)
	require.NoError(t, err)
	defer next.Retire()
	got = call(h, "synthetics-checks")
	row := rowValues(t, got, responseData(t, got)[0])
	assert.Nil(t, row["completed_us"])
	assert.Nil(t, row["last_failure_us"])
	assert.Equal(t, false, row["fresh"])
}

func TestInventoryTruncation(t *testing.T) {
	hub := model.NewHub()
	for _, name := range []string{"a", "b"} {
		_, err := hub.Register(
			model.Job{
				JobID:          "journey:" + name,
				Name:           name,
				Kind:           model.Journey,
				CadenceSeconds: 900,
			},
		)
		require.NoError(t, err)
	}
	got := call(syntheticfunc.New(&source{
		hub: hub,
	}), "synthetics-checks", "limit:1")
	assert.Len(t, responseData(t, got), 1)
	assert.Equal(t, true, got.RawResponse["truncated"])
	assert.Equal(t, 1, got.RawResponse["limit"])
}

func TestHistoryFiltersAndUnavailableValues(t *testing.T) {
	upperBound := int64(200)
	evidence := model.Run{
		ID:           "r",
		JobID:        "lighthouse:home",
		Name:         "home",
		Kind:         model.Lighthouse,
		Outcome:      model.Unknown,
		CaptureState: "disabled",
	}
	src := &source{
		page: model.RunPage{
			Runs:      []model.Run{evidence},
			Truncated: true,
		},
	}
	got := call(
		syntheticfunc.New(src),
		"synthetics-runs",
		"job_id:lighthouse:home",
		"kind:lighthouse",
		"outcome:unknown",
		"after:100",
		"before:200",
		"limit:3",
	)
	assert.Equal(
		t,
		model.RunFilter{
			JobID:   "lighthouse:home",
			Kind:    model.Lighthouse,
			Outcome: model.Unknown,
			After:   100,
			Before:  &upperBound,
			Limit:   3,
		},
		src.filter,
	)
	row := rowValues(t, got, responseData(t, got)[0])
	assert.Nil(t, row["duration_ms"])
	assert.Nil(t, row["started_us"])
	assert.Nil(t, row["completed_us"])
	assert.Equal(t, true, got.RawResponse["truncated"])
	validate(t, got)
	before := time.Now().Unix()
	responseData(t, call(syntheticfunc.New(src), "synthetics-runs", "after:-60"))
	assert.GreaterOrEqual(t, src.filter.After, before-60)
	require.NotNil(t, src.filter.Before)
	assert.LessOrEqual(t, *src.filter.Before, time.Now().Unix())
	assert.Equal(t, 2000, src.filter.Limit)
	src.queryErr = errors.New("private path and credential")
	got = call(syntheticfunc.New(src), "synthetics-runs")
	assert.Equal(t, 500, got.Status)
	assert.NotContains(t, got.Message, "credential")
}

func TestRunDetailPreservesPhaseAndNullableLabData(t *testing.T) {
	run := model.Run{
		ID:          "r",
		JobID:       "journey:a",
		Name:        "a",
		Kind:        model.Journey,
		Outcome:     model.Failed,
		CompletedUS: 123000000,
		DurationMS:  ptr(10.5),
		Error:       "fixture failed",
		Tests: &model.TestCounts{
			Declared: 2,
			Failed:   1,
			Skipped:  1,
		},
		Events: []model.Event{
			{
				Kind:           "test_end",
				AtMS:           1700000000000,
				TestID:         "one",
				Title:          "checkout",
				Phase:          "afterAll",
				Status:         "failed",
				ExpectedStatus: "passed",
				DurationMS:     ptr(10.5),
				Message:        "fixture failed",
			},
		},
		DroppedEvents: 2,
		Metrics: &model.LabMetrics{
			Performance: ptr(0),
			LCPMS:       ptr(math.NaN()),
			CLS:         ptr(math.Inf(1)),
		},
		CaptureState: "unavailable",
	}
	src := &source{
		history: map[string]model.Run{"journey:a/r": run},
	}
	got := call(syntheticfunc.New(src), "synthetics-run", "job_id:journey:a", "run_id:r")
	rows := responseData(t, got)
	require.Len(t, rows, 1)
	assert.Equal(
		t,
		[]any{
			int64(1700000000000),
			"test_end",
			"one",
			"checkout",
			"afterAll",
			"failed",
			"passed",
			10.5,
			"fixture failed",
		},
		rows[0],
	)
	detail := got.RawResponse["run"].(map[string]any)
	assert.Equal(t, run.Tests, detail["tests"])
	assert.Equal(t, 2, detail["dropped_events"])
	assert.Equal(
		t,
		map[string]any{
			"performance": float64(0),
			"fcp_ms":      nil,
			"lcp_ms":      nil,
			"tbt_ms":      nil,
			"si_ms":       nil,
			"cls":         nil,
		},
		detail["metrics"],
	)
	validate(t, got)
	// Current failed journal writes are still diagnosable through their exact ID.
	src.hub = model.NewHub()
	registration, err := src.hub.Register(model.Job{
		JobID:          run.JobID,
		CadenceSeconds: 900,
	})
	require.NoError(t, err)
	defer registration.Retire()
	registration.Complete(run)
	call(syntheticfunc.New(src), "synthetics-run", "job_id:journey:a", "run_id:r")
	assert.Equal(t, 1, src.getCalls, "active exact run avoids a second history read")
}

func TestArtifactAccessAndExpiryThroughRealStore(t *testing.T) {
	store, err := artifacts.Open(t.TempDir())
	require.NoError(t, err)
	defer store.Close()
	const runID = "0123456789abcdef0123456789abcdef"
	dir, err := store.Begin(runID)
	require.NoError(t, err)
	require.NoError(t, os.Mkdir(filepath.Join(dir, "output"), 0700))
	html := []byte("<html><script>privateReport()</script></html>")
	require.NoError(t, os.WriteFile(filepath.Join(dir, "output/report.html"), html, 0600))
	entries, err := store.Finalize(
		context.Background(),
		runID,
		[]model.Capture{{ID: "report", Kind: "report", MIME: "text/html", Path: "output/report.html"}},
	)
	require.NoError(t, err)
	run := model.Run{
		ID:           runID,
		JobID:        "lighthouse:home",
		CaptureState: "available",
		Artifacts:    entries,
	}
	src := &source{
		artifacts: store,
		history:   map[string]model.Run{"lighthouse:home/" + runID: run},
	}
	h := syntheticfunc.New(src)
	inventory := call(h, "synthetics-checks")
	assert.Equal(t, store.Stats(), inventory.RawResponse["artifacts"])
	args := []string{"job_id:lighthouse:home", "run_id:" + runID}
	index := h.HandleRaw(
		context.Background(),
		funcapi.RawMethodRequest{
			Method:      "synthetics-artifact",
			Permissions: "0x1b",
			Args:        args,
		},
	)
	assert.Equal(t, "available", rowValues(t, index, responseData(t, index)[0])["availability"])
	validate(t, index)
	got := call(h, "synthetics-artifact", append(args, "artifact_id:report")...)
	require.Len(t, responseData(t, got), 1)
	assert.Equal(t, "base64", got.RawResponse["encoding"])
	body, err := base64.StdEncoding.DecodeString(got.RawResponse["data_base64"].(string))
	require.NoError(t, err)
	assert.Equal(t, html, body)
	validate(t, got)
	assert.Equal(t, 404, call(h, "synthetics-artifact", append(args, "artifact_id:missing")...).Status)
	src.fetchErr = artifacts.ErrTooLarge
	assert.Equal(t, 413, call(h, "synthetics-artifact", append(args, "artifact_id:report")...).Status)
	src.fetchErr = nil
	_, err = store.Enforce(context.Background(), 7, 1)
	require.NoError(t, err)
	got = call(h, "synthetics-artifact", args...)
	assert.Equal(t, "expired_or_unavailable", got.RawResponse["capture_state"])
	assert.Equal(t, "expired_or_unavailable", rowValues(t, got, responseData(t, got)[0])["availability"])
	validate(t, got)
	got = call(h, "synthetics-artifact", append(args, "artifact_id:report")...)
	require.NotNil(t, got.RawResponse)
	assert.Equal(t, 404, got.RawResponse["status"])
	assert.Equal(t, "expired_or_unavailable", got.RawResponse["capture_state"])
	validate(t, got)
}

func TestCaptureAbsenceStates(t *testing.T) {
	for _, state := range []string{"disabled", "not_needed", "unavailable"} {
		t.Run(state, func(t *testing.T) {
			src := &source{
				history: map[string]model.Run{"journey:a/r": {ID: "r", JobID: "journey:a", CaptureState: state}},
			}
			got := call(syntheticfunc.New(src), "synthetics-artifact", "job_id:journey:a", "run_id:r")
			assert.Empty(t, responseData(t, got))
			assert.Equal(t, state, got.RawResponse["capture_state"])
			validate(t, got)
		})
	}
}

type journalSource struct {
	*source
	journal *store.Store
}

func (s *journalSource) QuerySyntheticRuns(ctx context.Context, filter model.RunFilter) (model.RunPage, error) {
	return s.journal.QuerySyntheticRuns(ctx, filter)
}
func TestHistoryExplicitEpochBoundThroughJournal(t *testing.T) {
	ctx := context.Background()
	journal, err := store.Open(ctx, t.TempDir())
	require.NoError(t, err)
	t.Cleanup(func() { require.NoError(t, journal.Close()) })
	run := model.Run{ID: "epoch-bound", JobID: "journey:epoch", Kind: model.Journey, StartedUS: time.Now().UnixMicro(), Outcome: model.Success}
	run.CompletedUS = run.StartedUS + 1
	_, err = journal.AppendSyntheticRun(ctx, "complete", run)
	require.NoError(t, err)
	h := syntheticfunc.New(&journalSource{source: &source{}, journal: journal})
	require.Len(t, responseData(t, call(h, "synthetics-runs")), 1)
	got := call(h, "synthetics-runs", "before:0")
	assert.Empty(t, responseData(t, got), "Unix epoch upper bound must exclude contemporary records")
	assert.Equal(t, int64(0), got.RawResponse["before"])
	// Internal detail lookup still has no saved-time bound.
	retained, err := journal.GetSyntheticRun(ctx, run.JobID, run.ID)
	require.NoError(t, err)
	assert.Equal(t, run.ID, retained.ID)
}
