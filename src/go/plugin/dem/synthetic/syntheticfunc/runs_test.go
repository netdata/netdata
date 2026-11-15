// SPDX-License-Identifier: GPL-3.0-or-later

package syntheticfunc_test

import (
	"maps"
	"strconv"
	"testing"
	"time"

	"github.com/netdata/netdata/go/plugins/plugin/dem/journalstore"
	model "github.com/netdata/netdata/go/plugins/plugin/dem/synthetic"
	synthetichistory "github.com/netdata/netdata/go/plugins/plugin/dem/synthetic/history"
	"github.com/netdata/netdata/go/plugins/plugin/dem/synthetic/query"
	syntheticregistry "github.com/netdata/netdata/go/plugins/plugin/dem/synthetic/registry"
	"github.com/netdata/netdata/go/plugins/plugin/dem/synthetic/syntheticfunc"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestRunHistoryRowsShowOnlyTheirKindColumns(t *testing.T) {
	counts := &model.TestCounts{
		Declared:        7,
		Passed:          1,
		Failed:          2,
		TimedOut:        1,
		Skipped:         1,
		ExpectedFailure: 1,
		NotRun:          1,
	}
	metrics := &model.LabMetrics{
		Performance: ptr(87),
		FCPMS:       ptr(900.5),
		LCPMS:       ptr(1800),
		SIMS:        ptr(1200),
		CLS:         ptr(0.02),
	}
	shared := map[string]any{
		"run_id":             "r",
		"job_id":             "journey:checkout",
		"name":               "checkout",
		"target":             "https://shop.example/",
		"started_us":         int64(100),
		"completed_us":       int64(200),
		"outcome":            model.Failed,
		"duration_ms":        12.5,
		"capture_state":      model.CaptureAvailable,
		"error":              "expected receipt",
		"history_error":      "history write failed",
		"vnode_guid":         "11111111-1111-4111-8111-111111111111",
		"scheduled_us":       int64(90),
		"queued_us":          int64(95),
		"termination_reason": model.ExecutionBudget,
	}
	noEvidence := map[string]any{
		"failure_evidence_status": "not_recorded",
		"first_failure":           "",
		"last_observed_step":      "",
		"stop_context":            "",
		"browser_summary_status":  "not_recorded",
		"browser_errors":          "",
	}
	for name, tc := range map[string]struct {
		method string
		kind   model.Kind
		tests  *model.TestCounts
		lab    *model.LabMetrics
		want   map[string]any
	}{
		// A Lighthouse measurement on a journey run is not a journey column, and the reverse.
		"journey with test counts": {
			method: "synthetics-journey-runs",
			kind:   model.Journey,
			tests:  counts,
			lab:    metrics,
			want: map[string]any{
				"tests_declared":         7,
				"tests_passed":           1,
				"tests_failed":           2,
				"tests_timed_out":        1,
				"tests_skipped":          1,
				"tests_expected_failure": 1,
				"tests_not_run":          1,
			},
		},
		"journey without test counts": {
			method: "synthetics-journey-runs",
			kind:   model.Journey,
			want: map[string]any{
				"tests_declared":         nil,
				"tests_passed":           nil,
				"tests_failed":           nil,
				"tests_timed_out":        nil,
				"tests_skipped":          nil,
				"tests_expected_failure": nil,
				"tests_not_run":          nil,
			},
		},
		"lighthouse with measurements": {
			method: "synthetics-lighthouse-runs",
			kind:   model.Lighthouse,
			tests:  counts,
			lab:    metrics,
			want: map[string]any{
				"performance": 87.0,
				"fcp_ms":      900.5,
				"lcp_ms":      1800.0,
				"tbt_ms":      nil,
				"si_ms":       1200.0,
				"cls":         0.02,
			},
		},
		"lighthouse without measurements": {
			method: "synthetics-lighthouse-runs",
			kind:   model.Lighthouse,
			want: map[string]any{
				"performance": nil,
				"fcp_ms":      nil,
				"lcp_ms":      nil,
				"tbt_ms":      nil,
				"si_ms":       nil,
				"cls":         nil,
			},
		},
	} {
		t.Run(name, func(t *testing.T) {
			run := model.Run{
				ID:                "r",
				JobID:             "journey:checkout",
				Kind:              tc.kind,
				Name:              "checkout",
				Target:            "https://shop.example/",
				StartedUS:         100,
				CompletedUS:       200,
				DurationMS:        ptr(12.5),
				Outcome:           model.Failed,
				CaptureState:      model.CaptureAvailable,
				Error:             "expected receipt",
				HistoryError:      "history write failed",
				VnodeGUID:         "11111111-1111-4111-8111-111111111111",
				ScheduledUS:       90,
				QueuedUS:          95,
				TerminationReason: model.ExecutionBudget,
				Tests:             tc.tests,
				Metrics:           tc.lab,
			}
			src := &source{
				page: synthetichistory.RunPage{
					Runs: []model.Run{run},
				},
			}
			got := call(newHandler(src), tc.method)
			rows := responseData(t, got)
			require.Len(t, rows, 1)
			want := maps.Clone(shared)
			maps.Copy(want, tc.want)
			if tc.kind == model.Journey {
				maps.Copy(want, noEvidence)
			}
			assert.Equal(t, want, rowValues(t, got, rows[0]))
			assert.Equal(t, tc.kind, src.filter.Kind)
			validate(t, got)
		})
	}
}

func TestRunHistoryTruncationCountsOnlyItsKind(t *testing.T) {
	ctx := t.Context()
	journalStore, err := journalstore.Open(ctx, t.TempDir())
	require.NoError(t, err)
	t.Cleanup(func() { require.NoError(t, journalStore.Close()) })
	store := synthetichistory.NewStore(journalStore)
	now := time.Now().UnixMicro()
	for i, kind := range []model.Kind{model.Journey, model.Journey, model.Journey, model.Lighthouse, model.Lighthouse} {
		_, err := store.AppendRun(ctx, model.HistoryComplete, model.Run{
			ID:          "run-" + strconv.Itoa(i),
			JobID:       string(kind) + ":site",
			Kind:        kind,
			StartedUS:   now - int64(i),
			CompletedUS: now,
			Outcome:     model.Success,
		})
		require.NoError(t, err)
	}
	h := syntheticfunc.New(query.New(syntheticregistry.New(), store, emptyArtifacts(t), noPolicyError), nil)
	for method, tc := range map[string]struct {
		runs      []any
		truncated bool
	}{
		"synthetics-journey-runs": {
			runs:      []any{"run-0", "run-1"},
			truncated: true,
		},
		"synthetics-lighthouse-runs": {
			runs:      []any{"run-3", "run-4"},
			truncated: false,
		},
	} {
		t.Run(method, func(t *testing.T) {
			got := call(h, method, "limit:2")
			var runs []any
			for _, row := range responseData(t, got) {
				runs = append(runs, rowValues(t, got, row)["run_id"])
			}
			assert.Equal(t, tc.runs, runs)
			assert.Equal(t, tc.truncated, got.RawResponse["truncated"])
			assert.Equal(t, 2, got.RawResponse["limit"])
			validate(t, got)
		})
	}
}
