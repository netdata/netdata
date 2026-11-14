// SPDX-License-Identifier: GPL-3.0-or-later

package syntheticfunc_test

import (
	"context"
	"crypto/rand"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/netdata/netdata/go/plugins/pkg/funcapi"
	demjournal "github.com/netdata/netdata/go/plugins/plugin/dem/journal"
	model "github.com/netdata/netdata/go/plugins/plugin/dem/synthetic"
	"github.com/netdata/netdata/go/plugins/plugin/dem/synthetic/artifacts"
	synthetichistory "github.com/netdata/netdata/go/plugins/plugin/dem/synthetic/history"
	"github.com/netdata/netdata/go/plugins/plugin/dem/synthetic/monitor"
	"github.com/netdata/netdata/go/plugins/plugin/dem/synthetic/query"
	syntheticregistry "github.com/netdata/netdata/go/plugins/plugin/dem/synthetic/registry"
	"github.com/netdata/netdata/go/plugins/plugin/dem/synthetic/syntheticfunc"
	"github.com/santhosh-tekuri/jsonschema/v6"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

type source struct {
	active    query.Observations
	history   map[string]model.Run
	filter    synthetichistory.RunFilter
	page      synthetichistory.RunPage
	queryErr  error
	getCalls  int
	artifacts *artifacts.Store
	fetchErr  error
}

func (s *source) Catalogue(ctx context.Context, filter artifacts.Filter) (artifacts.Page, error) {
	return s.artifacts.Catalogue(ctx, filter)
}

func (s *source) QueryRuns(ctx context.Context, f synthetichistory.RunFilter) (synthetichistory.RunPage, error) {
	s.filter = f
	return s.page, s.queryErr
}

func (s *source) GetRun(ctx context.Context, jobID, runID string) (model.Run, error) {
	s.getCalls++
	r, ok := s.history[jobID+"/"+runID]
	if !ok {
		return model.Run{}, os.ErrNotExist
	}
	return r, nil
}

func (s *source) Manifest(ctx context.Context, jobID, runID string) (artifacts.Manifest, error) {
	if s.artifacts == nil {
		return artifacts.Manifest{}, artifacts.ErrNotFound
	}
	return s.artifacts.Manifest(ctx, jobID, runID)
}

func (s *source) Fetch(ctx context.Context, jobID, runID, artifactID string) (model.Artifact, []byte, error) {
	if s.fetchErr != nil {
		return model.Artifact{}, nil, s.fetchErr
	}
	return s.artifacts.Fetch(ctx, jobID, runID, artifactID)
}

func newHandler(s *source) *syntheticfunc.Handler {
	if s == nil {
		return syntheticfunc.New(nil, nil)
	}
	active := s.active
	if active == nil {
		active = syntheticregistry.New()
	}
	return syntheticfunc.New(query.New(active, s, s, noPolicyError), nil)
}

// activeJobs is a fixed set of active jobs and their latest runs, keyed by job ID.
type activeJobs struct {
	jobs []syntheticregistry.Job
	runs map[string]model.Run
}

func (a activeJobs) Snapshot(time.Time) []syntheticregistry.Job { return a.jobs }

func (a activeJobs) Latest(jobID, runID string) (model.Run, bool) {
	run, ok := a.runs[jobID]
	return run, ok && run.ID == runID
}

func ptr(n float64) *float64 { return &n }

// fixtureRunID returns a fresh run ID: artifacts.Store.Begin creates a /tmp/nd-dem-<id> alias that is shared by
// every test process on the host, so fixed IDs collide across parallel packages and concurrent checkouts.
func fixtureRunID() string {
	var id [16]byte
	if _, err := rand.Read(id[:]); err != nil {
		panic(err)
	}
	return hex.EncodeToString(id[:])
}

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
	raw, err := os.ReadFile("../../../../../plugins.d/FUNCTION_UI_SCHEMA.json")
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
	h := newHandler(nil)
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
			got := newHandler(nil).HandleRaw(context.Background(), tc.request)
			assert.Equal(t, tc.status, got.Status)
		})
	}
	h := newHandler(&source{})
	for _, args := range [][]string{{"limit:0"}, {"limit:2001"}, {"limit:NaN"}, {"kind:other"}, {"after:NaN"}, {"after:200", "before:100"}, {"outcome:other"}} {
		got := call(h, "synthetics-runs", args...)
		assert.Equal(t, 400, got.Status)
	}
	// With both bounds invalid, the after bound is reported on every call.
	for range 64 {
		require.Equal(
			t,
			funcapi.ErrorResponse(400, "after must be Unix seconds or a relative negative offset"),
			call(h, "synthetics-runs", "after:NaN", "before:NaN"),
		)
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

func TestChecksRowPresentsTheActiveSnapshot(t *testing.T) {
	completed := time.Now().Add(-time.Minute).UnixMicro()
	job := syntheticregistry.Job{
		Snapshot: monitor.Snapshot{
			JobID:    "journey:checkout",
			Kind:     model.Journey,
			Name:     "checkout",
			Interval: time.Second,
			Status: monitor.Status{
				Scheduled:       12,
				Admitted:        2,
				Coalesced:       9,
				Pending:         true,
				Active:          true,
				AdmittedUS:      completed,
				SchedulingDelay: 3 * time.Second,
			},
			State: model.JobRunning,
			Latest: monitor.Observation{
				ID:          "r",
				Outcome:     model.Failed,
				CompletedUS: completed,
				DurationMS: monitor.Scalar{
					Value:   500.25,
					Present: true,
				},
				Error:        "afterAll failed",
				HistoryError: "history write failed",
			},
			HasObservation: true,
			LastFailureUS:  completed,
		},
	}
	h := newHandler(&source{
		active: activeJobs{jobs: []syntheticregistry.Job{job}},
	})
	got := call(h, "synthetics-checks")
	rows := responseData(t, got)
	require.Len(t, rows, 1)
	assert.Equal(
		t,
		map[string]any{
			"job_id":           "journey:checkout",
			"vnode_guid":       "",
			"kind":             model.Journey,
			"name":             "checkout",
			"target":           "",
			"state":            "running",
			"fresh":            false,
			"run_id":           "r",
			"outcome":          "failed",
			"completed_us":     completed,
			"duration_ms":      500.25,
			"last_success_us":  nil,
			"last_failure_us":  completed,
			"error":            "afterAll failed",
			"history_error":    "history write failed",
			"check_every":      1,
			"scheduled":        uint64(12),
			"admitted":         uint64(2),
			"coalesced":        uint64(9),
			"pending":          true,
			"queue_age":        nil,
			"scheduling_delay": 3.0,
		},
		rowValues(t, got, rows[0]),
	)
	validate(t, got)
	require.Empty(t, responseData(t, call(h, "synthetics-checks", "kind:lighthouse")))
}

func TestInventoryTruncation(t *testing.T) {
	var active activeJobs
	for _, name := range []string{"a", "b"} {
		active.jobs = append(active.jobs, syntheticregistry.Job{
			Snapshot: monitor.Snapshot{
				JobID:    "journey:" + name,
				Name:     name,
				Kind:     model.Journey,
				Interval: 900 * time.Second,
			},
		})
	}
	got := call(newHandler(&source{
		active: active,
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
		page: synthetichistory.RunPage{
			Runs:      []model.Run{evidence},
			Truncated: true,
		},
	}
	got := call(
		newHandler(src),
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
		synthetichistory.RunFilter{
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
	responseData(t, call(newHandler(src), "synthetics-runs", "after:-60"))
	assert.GreaterOrEqual(t, src.filter.After, before-60)
	require.NotNil(t, src.filter.Before)
	assert.LessOrEqual(t, *src.filter.Before, time.Now().Unix())
	assert.Equal(t, 2000, src.filter.Limit)
	src.queryErr = errors.New("private path and credential")
	got = call(newHandler(src), "synthetics-runs")
	assert.Equal(t, 500, got.Status)
	assert.NotContains(t, got.Message, "credential")
}

func TestRunDetailPreservesPhaseAndNullableLabData(t *testing.T) {
	run := model.Run{
		Runtime: &model.RuntimeIdentity{
			Version:  "release-a",
			SHA256:   "aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa",
			Contract: 1,
		},
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
		},
		CaptureState: "unavailable",
	}
	src := &source{
		history: map[string]model.Run{"journey:a/r": run},
	}
	got := call(newHandler(src), "synthetics-run", "job_id:journey:a", "run_id:r")
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
	assert.Equal(t, run.Runtime, detail["runtime"])
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
	src.active = activeJobs{runs: map[string]model.Run{run.JobID: run}}
	call(newHandler(src), "synthetics-run", "job_id:journey:a", "run_id:r")
	assert.Equal(t, 1, src.getCalls, "active exact run avoids a second history read")
}

func TestArtifactAccessAndExpiryThroughRealStore(t *testing.T) {
	store, err := artifacts.Open(t.TempDir())
	require.NoError(t, err)
	defer store.Close()
	runID := fixtureRunID()
	dir, err := store.Begin(runID, "lighthouse:home", "")
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
	h := newHandler(src)
	inventory := call(h, "synthetics-checks")
	assert.NotContains(t, inventory.RawResponse, "artifacts")
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
	assert.Equal(t, "Lighthouse report", rowValues(t, index, responseData(t, index)[0])["label"])
	assert.Empty(t, rowValues(t, index, responseData(t, index)[0])["test"])
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
	assert.Equal(t, model.CaptureExpired, got.RawResponse["capture_state"])
	assert.Equal(t, "expired_or_unavailable", rowValues(t, got, responseData(t, got)[0])["availability"])
	validate(t, got)
	got = call(h, "synthetics-artifact", append(args, "artifact_id:report")...)
	require.NotNil(t, got.RawResponse)
	assert.Equal(t, 404, got.RawResponse["status"])
	assert.Equal(t, model.CaptureExpired, got.RawResponse["capture_state"])
	validate(t, got)
}

func TestCaptureLabelsThroughRealStore(t *testing.T) {
	store, err := artifacts.Open(t.TempDir())
	require.NoError(t, err)
	defer store.Close()
	runID := fixtureRunID()
	dir, err := store.Begin(runID, "journey:checkout", "")
	require.NoError(t, err)
	require.NoError(t, os.Mkdir(filepath.Join(dir, "output"), 0700))
	cases := []struct {
		id    string
		test  model.ArtifactTest
		title string
		trace bool
	}{
		{id: "trace-associated", trace: true, test: model.ArtifactTest{ID: "test1", Title: "Checkout"}, title: "Checkout"},
		{id: "trace-unassociated", trace: true, title: "Test not recorded"},
		{id: "associated", test: model.ArtifactTest{ID: "test1", Title: "Checkout"}, title: "Checkout"},
		{id: "unassociated", title: "Test not recorded"},
	}
	var candidates []model.Capture
	for _, tc := range cases {
		path := "output/" + tc.id + ".png"
		require.NoError(t, os.WriteFile(filepath.Join(dir, path), []byte("\x89PNG\r\n\x1a\nfixture"), 0600))
		c := model.Capture{ID: tc.id, Kind: "screenshot", MIME: "image/png", Path: path, Test: tc.test}
		if tc.trace {
			c.Kind, c.MIME = "trace", "application/zip"
			require.NoError(t, os.WriteFile(filepath.Join(dir, path), []byte("PK\x03\x04fixture"), 0600))
		}
		candidates = append(candidates, c)
	}
	_, err = store.Finalize(t.Context(), runID, candidates)
	require.NoError(t, err)
	h := newHandler(&source{artifacts: store})
	got := call(h, "synthetics-artifact", "job_id:journey:checkout", "run_id:"+runID)
	rows := responseData(t, got)
	require.Len(t, rows, len(cases))
	for i, tc := range cases {
		row := rowValues(t, got, rows[i])
		assert.Equal(t, tc.id, row["artifact_id"])
		label := "Screenshot"
		if tc.trace {
			label = "Trace"
		}
		assert.Equal(t, label, row["label"])
		assert.Equal(t, tc.title, row["test"])
	}
	validate(t, got)
}

func TestCaptureAbsenceStates(t *testing.T) {
	for _, state := range []model.CaptureState{model.CaptureDisabled, model.CaptureNotNeeded, model.CaptureUnavailable} {
		t.Run(string(state), func(t *testing.T) {
			src := &source{
				history: map[string]model.Run{"journey:a/r": {ID: "r", JobID: "journey:a", CaptureState: state}},
			}
			got := call(newHandler(src), "synthetics-artifact", "job_id:journey:a", "run_id:r")
			assert.Empty(t, responseData(t, got))
			assert.Equal(t, state, got.RawResponse["capture_state"])
			validate(t, got)
		})
	}
}

func TestHistoryExplicitEpochBoundThroughJournal(t *testing.T) {
	ctx := context.Background()
	journal, err := demjournal.Open(ctx, t.TempDir())
	require.NoError(t, err)
	t.Cleanup(func() { require.NoError(t, journal.Close()) })
	run := model.Run{
		ID:        "epoch-bound",
		JobID:     "journey:epoch",
		Kind:      model.Journey,
		StartedUS: time.Now().UnixMicro(),
		Outcome:   model.Success,
	}
	run.CompletedUS = run.StartedUS + 1
	_, err = synthetichistory.NewStore(journal).AppendRun(ctx, "complete", run)
	require.NoError(t, err)
	h := syntheticfunc.New(query.New(syntheticregistry.New(), synthetichistory.NewStore(journal), emptyArtifacts(t), noPolicyError), nil)
	require.Len(t, responseData(t, call(h, "synthetics-runs")), 1)
	got := call(h, "synthetics-runs", "before:0")
	assert.Empty(t, responseData(t, got), "Unix epoch upper bound must exclude contemporary records")
	assert.Equal(t, int64(0), got.RawResponse["before"])
	// Internal detail lookup still has no picker bound.
	retained, err := synthetichistory.NewStore(journal).GetRun(ctx, run.JobID, run.ID)
	require.NoError(t, err)
	assert.Equal(t, run.ID, retained.ID)
}

func TestHistoryPickerUsesStartAndLatestCompletionThroughFunction(t *testing.T) {
	ctx := context.Background()
	journal, err := demjournal.Open(ctx, t.TempDir())
	require.NoError(t, err)
	t.Cleanup(func() { require.NoError(t, journal.Close()) })
	retained := synthetichistory.NewStore(journal)
	run := model.Run{
		ID:        "past-attempt",
		JobID:     "journey:past",
		Kind:      model.Journey,
		StartedUS: 100999999,
		Outcome:   model.Unknown,
	}
	_, err = retained.AppendRun(ctx, "start", run)
	require.NoError(t, err)
	h := syntheticfunc.New(query.New(syntheticregistry.New(), retained, emptyArtifacts(t), noPolicyError), nil)
	beforeCompletion := call(h, "synthetics-runs", "after:100", "before:100", "outcome:unknown")
	require.Len(t, responseData(t, beforeCompletion), 1)
	validate(t, beforeCompletion)
	run.CompletedUS = 300000000
	run.Outcome = model.Failed
	run.Events = []model.Event{{Kind: "error", Message: "later completion"}}
	_, err = retained.AppendRun(ctx, "complete", run)
	require.NoError(t, err)
	got := call(h, "synthetics-runs", "after:100", "before:100", "outcome:failed")
	rows := responseData(t, got)
	require.Len(t, rows, 1)
	values := rowValues(t, got, rows[0])
	assert.Equal(t, run.ID, values["run_id"])
	assert.Equal(t, run.StartedUS, values["started_us"])
	assert.Equal(t, run.CompletedUS, values["completed_us"])
	assert.Equal(t, run.Outcome, values["outcome"])
	validate(t, got)
	assert.Empty(t, responseData(t, call(h, "synthetics-runs", "after:100", "before:100", "outcome:unknown")))
	assert.Empty(t, responseData(t, call(h, "synthetics-runs", "after:101", "before:300")))
	detail := call(h, "synthetics-run", "job_id:journey:past", "run_id:past-attempt")
	require.NotNil(t, detail.RawResponse)
	assert.Equal(t, run.CompletedUS, detail.RawResponse["run"].(map[string]any)["completed_us"])
	validate(t, detail)
}

func TestTerminationReasonIsIndependentOfFirstError(t *testing.T) {
	run := model.Run{ID: "r", JobID: "journey:a", Kind: model.Journey, Outcome: model.Timeout,
		TerminationReason: model.ExecutionBudget, Error: "earlier assertion failed"}
	src := &source{history: map[string]model.Run{"journey:a/r": run}}
	got := call(newHandler(src), "synthetics-run", "job_id:journey:a", "run_id:r")
	detail := got.RawResponse["run"].(map[string]any)
	assert.Equal(t, run.TerminationReason, detail["termination_reason"])
	assert.Equal(t, run.Error, detail["error"])
}

func TestFailureEvidenceFunctionsAreAdditive(t *testing.T) {
	for _, rich := range []bool{false, true} {
		name := "without optional outputs"
		if rich {
			name = "rich history"
		}
		t.Run(name, func(t *testing.T) {
			run := model.Run{ID: "r", JobID: "journey:a", Kind: model.Journey, Outcome: model.Timeout, TerminationReason: model.ExecutionBudget, Error: "core error", Events: []model.Event{{Kind: "step", Title: "observed step"}}}
			if rich {
				run.Captures = &model.CaptureReport{Reporting: "partial", Unpublished: []model.CaptureOmission{{ID: "s1", Reason: "not_in_final_report"}}}
				run.BrowserSummary = &model.BrowserSummary{Availability: "supported", Coverage: "fixture_context", Run: &model.BrowserAggregate{Tests: model.BrowserTestCounts{Observed: 1}, Counts: model.BrowserCounts{ConsoleError: 2}, Omissions: []string{"candidate_limit"}}}
				run.FailureEvidence = &model.FailureEvidence{
					Availability: "supported", Truncated: []string{"user_steps"},
					Steps:        []model.EvidenceStep{{ID: "s1", TestID: "t", Title: "pending submit", State: "running"}},
					FirstFailure: &model.FailureDetail{Test: model.EvidenceTest{ID: "t", Title: "checkout", Status: "failed", ExpectedStatus: "passed"}, Message: "expected receipt", PathIncomplete: true},
					StopContext:  &model.FailureDetail{Test: model.EvidenceTest{ID: "later", Title: "login", Status: "interrupted", ExpectedStatus: "passed"}, PathIncomplete: true},
				}
			}
			src := &source{history: map[string]model.Run{"journey:a/r": run}, page: synthetichistory.RunPage{Runs: []model.Run{run}}}
			h := newHandler(src)
			detailResponse := call(h, "synthetics-run", "job_id:journey:a", "run_id:r")
			rows := responseData(t, detailResponse)
			require.Len(t, rows, 1)
			assert.Equal(t, "observed step", rowValues(t, detailResponse, rows[0])["title"])
			detail := detailResponse.RawResponse["run"].(map[string]any)
			status := "not_recorded"
			if rich {
				status = "partial"
			}
			assert.Equal(t, map[string]any{
				"id": "r", "job_id": "journey:a", "kind": model.Journey,
				"vnode_guid": "", "name": "", "target": "",
				"started_us": nil, "scheduled_us": nil, "queued_us": nil, "completed_us": nil,
				"duration_ms": nil, "outcome": model.Timeout, "termination_reason": model.ExecutionBudget,
				"error": "core error", "tests": (*model.TestCounts)(nil),
				"capture_state": model.CaptureState(""), "artifacts": []model.Artifact{}, "history_error": "",
				"captures":       run.Captures,
				"dropped_events": 0, "metrics": nil, "runtime": (*model.RuntimeIdentity)(nil),
				"failure_evidence": run.FailureEvidence, "failure_evidence_status": status,
				"browser_summary": run.BrowserSummary, "browser_summary_status": status,
			}, detail)
			validate(t, detailResponse)
			listResponse := call(h, "synthetics-runs")
			listRows := responseData(t, listResponse)
			require.Len(t, listRows, 1)
			values := rowValues(t, listResponse, listRows[0])
			assert.Equal(t, run.FailureEvidence.ReportingStatus(), values["failure_evidence_status"])
			assert.Equal(t, run.FailureEvidence.FailureSummary(), values["first_failure"])
			if rich {
				assert.Contains(t, values["first_failure"], "[path incomplete]")
			}
			assert.Equal(t, run.FailureEvidence.LastObservedStepSummary(), values["last_observed_step"])
			assert.Equal(t, run.FailureEvidence.StopSummary(), values["stop_context"])
			assert.Equal(t, run.BrowserSummary.ReportingStatus(), values["browser_summary_status"])
			if rich {
				assert.Contains(t, values["browser_errors"], "tests: observed=1")
				assert.Contains(t, values["browser_errors"], "console_error=2")
				assert.Contains(t, values["browser_errors"], "[omissions: candidate_limit]")
			} else {
				assert.Equal(t, "", values["browser_errors"])
			}
			columns := listResponse.RawResponse["columns"].(map[string]any)
			assert.Equal(t, 15, columns["termination_reason"].(map[string]any)["index"])
			assert.Equal(t, 16, columns["failure_evidence_status"].(map[string]any)["index"])
			assert.Equal(t, 17, columns["first_failure"].(map[string]any)["index"])
			assert.Equal(t, 18, columns["last_observed_step"].(map[string]any)["index"])
			assert.Equal(t, 19, columns["stop_context"].(map[string]any)["index"])
			assert.Equal(t, 20, columns["browser_summary_status"].(map[string]any)["index"])
			assert.Equal(t, 21, columns["browser_errors"].(map[string]any)["index"])
			validate(t, listResponse)
		})
	}
}

func TestFunctionSiteFilterUsesIndependentCurrentAndHistoricalAssociations(t *testing.T) {
	ctx := context.Background()
	journal, err := demjournal.Open(ctx, t.TempDir())
	require.NoError(t, err)
	t.Cleanup(func() { require.NoError(t, journal.Close()) })
	store := synthetichistory.NewStore(journal)
	active := activeJobs{runs: map[string]model.Run{}}
	for i, site := range []string{"11111111-1111-4111-8111-111111111111", "22222222-2222-4222-8222-222222222222", ""} {
		name := []string{"checkout", "search", "legacy"}[i]
		jobID := "journey:" + name
		active.jobs = append(active.jobs, syntheticregistry.Job{Snapshot: monitor.Snapshot{JobID: jobID, Name: name, Kind: model.Journey, VnodeGUID: site}})
		run := model.Run{ID: name, JobID: jobID, Name: name, Kind: model.Journey, VnodeGUID: site, Outcome: model.Success, StartedUS: time.Now().UnixMicro(), CompletedUS: time.Now().UnixMicro()}
		if i == 0 {
			run.VnodeGUID = "33333333-3333-4333-8333-333333333333"
		}
		_, err = store.AppendRun(ctx, "complete", run)
		require.NoError(t, err)
		active.runs[jobID] = run
	}
	h := syntheticfunc.New(query.New(active, store, emptyArtifacts(t), noPolicyError), nil)
	checks := call(h, "synthetics-checks", "vnode_guid:11111111-1111-4111-8111-111111111111", "kind:journey", "job_id:journey:checkout")
	rows := responseData(t, checks)
	require.Len(t, rows, 1)
	assert.Equal(t, "11111111-1111-4111-8111-111111111111", rowValues(t, checks, rows[0])["vnode_guid"])
	validate(t, checks)
	assert.Empty(t, responseData(t, call(h, "synthetics-checks", "vnode_guid:11111111-1111-4111-8111-111111111111", "job_id:journey:search")))
	runs := call(h, "synthetics-runs", "vnode_guid:33333333-3333-4333-8333-333333333333", "kind:journey", "job_id:journey:checkout")
	rows = responseData(t, runs)
	require.Len(t, rows, 1)
	assert.Equal(t, "33333333-3333-4333-8333-333333333333", rowValues(t, runs, rows[0])["vnode_guid"])
	validate(t, runs)
	assert.Empty(t, responseData(t, call(h, "synthetics-runs", "vnode_guid:11111111-1111-4111-8111-111111111111")))
	assert.Len(t, responseData(t, call(h, "synthetics-runs", "vnode_guid:")), 3)
	detail := call(h, "synthetics-run", "job_id:journey:checkout", "run_id:checkout")
	require.NotNil(t, detail.RawResponse)
	assert.Equal(t, "33333333-3333-4333-8333-333333333333", detail.RawResponse["run"].(map[string]any)["vnode_guid"])
	for _, method := range []string{"synthetics-checks", "synthetics-runs", "synthetics-artifacts"} {
		assert.Equal(t, 400, call(h, method, "vnode_guid:INVALID").Status)
		denied := h.HandleRaw(ctx, funcapi.RawMethodRequest{Method: method, Args: []string{"vnode_guid:11111111-1111-4111-8111-111111111111"}, Permissions: "0"})
		assert.Equal(t, 403, denied.Status)
	}
}

// emptyHistory is a real run history holding no runs.
func emptyHistory(t *testing.T) *synthetichistory.Store {
	t.Helper()
	store, err := demjournal.Open(t.Context(), "")
	require.NoError(t, err)
	t.Cleanup(func() { require.NoError(t, store.Close()) })
	return synthetichistory.NewStore(store)
}

// emptyArtifacts is a real capture store holding no captures.
func emptyArtifacts(t *testing.T) *artifacts.Store {
	t.Helper()
	store, err := artifacts.Open(t.TempDir())
	require.NoError(t, err)
	t.Cleanup(func() { require.NoError(t, store.Close()) })
	return store
}

func noPolicyError() string { return "" }
