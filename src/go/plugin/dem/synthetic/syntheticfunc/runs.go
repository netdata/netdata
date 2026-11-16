// SPDX-License-Identifier: GPL-3.0-or-later

package syntheticfunc

import (
	"context"
	"slices"
	"time"

	"github.com/netdata/netdata/go/plugins/pkg/funcapi"
	"github.com/netdata/netdata/go/plugins/plugin/dem/internal/rawfunc"
	model "github.com/netdata/netdata/go/plugins/plugin/dem/synthetic"
)

// Run history has one method per kind, so each table shows only the columns its
// kind reports.
var (
	journeyRunsMethod = runsMethod(
		model.Journey,
		"synthetics-journey-runs",
		"Journey runs",
		"Journey runs, newest first, with test counts, the first failure, the last step reached and browser "+
			"errors. Use it to see when a journey started failing and why.",
		journeyRunsColumns,
	)
	lighthouseRunsMethod = runsMethod(
		model.Lighthouse,
		"synthetics-lighthouse-runs",
		"Lighthouse runs",
		"Lighthouse audits, newest first, with the performance score and lab timings of each. Use it to see "+
			"when a page got slower.",
		lighthouseRunsColumns,
	)
)

// runsHelp follows each run history summary.
var runsHelp = " Runs are selected by start time and show their latest outcome. Pass after and before (Unix " +
	"seconds, or negative seconds from now) to set the time range; before defaults to now, and without after the " +
	"range starts at the oldest saved run. Pass job_id to show one job and outcome (such as failed or timeout) to " +
	"show runs with that outcome. " + limitHelp + " " + rawfunc.VnodeHelp + " " + rawfunc.RecordedVnodeHelp

func runsMethod(kind model.Kind, id, title, summary string, columns rawfunc.Columns[model.Run]) method {
	return method{
		Method: rawfunc.Method{
			ID:          id,
			Title:       title,
			Help:        summary + runsHelp,
			Sort:        "started_us",
			UpdateEvery: 10,
			History:     true,
			Params:      []string{"job_id", "outcome", "after", "before", "limit", "vnode", "vnode_guid"},
			Columns:     columns.Build(),
		},
		serve: func(
			h *Handler,
			ctx context.Context,
			args map[string]string,
			response map[string]any,
		) *funcapi.FunctionResponse {
			return h.runs(ctx, kind, columns, args, response)
		},
	}
}

func (h *Handler) runs(
	ctx context.Context,
	kind model.Kind,
	columns rawfunc.Columns[model.Run],
	args map[string]string,
	response map[string]any,
) *funcapi.FunctionResponse {
	filter, err := runFilter(kind, args, time.Now().Unix())
	if err != nil {
		return funcapi.ErrorResponse(400, "%v", err)
	}
	page, err := h.deps.Runs(ctx, filter)
	if err != nil {
		return functionError(err)
	}
	rows := make([][]any, 0, len(page.Runs))
	for _, run := range page.Runs {
		rows = append(rows, columns.Row(run))
	}
	response["data"] = rows
	response["truncated"] = page.Truncated
	response["limit"] = filter.Limit
	response["after"] = filter.After
	response["before"] = *filter.Before
	return funcapi.RawResponse(response)
}

// runsColumns are the run history columns every kind reports.
var runsColumns = rawfunc.Columns[model.Run]{
	column("run_id", "Run", funcapi.FieldTypeString, "", func(r model.Run) any { return r.ID }),
	column("job_id", "Monitor", funcapi.FieldTypeString, "", func(r model.Run) any { return r.JobID }),
	column("name", "Name", funcapi.FieldTypeString, "", func(r model.Run) any { return r.Name }),
	column("target", "Target", funcapi.FieldTypeString, "", func(r model.Run) any { return r.Target }),
	column(
		"started_us",
		"Started",
		funcapi.FieldTypeTimestamp,
		"",
		func(r model.Run) any { return timestamp(r.StartedUS) },
	),
	column("completed_us", "Completed", funcapi.FieldTypeTimestamp, "", func(r model.Run) any {
		return timestamp(r.CompletedUS)
	}),
	column("outcome", "Outcome", funcapi.FieldTypeString, "", func(r model.Run) any { return r.Outcome }),
	column(
		"duration_ms",
		"Attempt duration",
		funcapi.FieldTypeFloat,
		"ms",
		func(r model.Run) any { return number(r.DurationMS) },
	),
	column(
		"capture_state",
		"Capture state",
		funcapi.FieldTypeString,
		"",
		func(r model.Run) any { return r.CaptureState },
	),
	column("error", "Error", funcapi.FieldTypeString, "", func(r model.Run) any { return r.Error }),
	column(
		"history_error",
		"History error",
		funcapi.FieldTypeString,
		"",
		func(r model.Run) any { return r.HistoryError },
	),
	column("vnode_guid", "Vnode GUID", funcapi.FieldTypeString, "", func(r model.Run) any { return r.VnodeGUID }),
	column("scheduled_us", "Selected due time", funcapi.FieldTypeTimestamp, "", func(r model.Run) any {
		return timestamp(r.ScheduledUS)
	}),
	column("queued_us", "Original admission queue time", funcapi.FieldTypeTimestamp, "", func(r model.Run) any {
		return timestamp(r.QueuedUS)
	}),
	column("termination_reason", "Termination reason", funcapi.FieldTypeString, "", func(r model.Run) any {
		return r.TerminationReason
	}),
}

var journeyRunsColumns = slices.Concat(runsColumns, rawfunc.Columns[model.Run]{
	testCount("tests_declared", "Declared tests", func(c model.TestCounts) int { return c.Declared }),
	testCount("tests_passed", "Passed tests", func(c model.TestCounts) int { return c.Passed }),
	testCount("tests_failed", "Failed tests", func(c model.TestCounts) int { return c.Failed }),
	testCount("tests_timed_out", "Timed-out tests", func(c model.TestCounts) int { return c.TimedOut }),
	testCount("tests_skipped", "Skipped tests", func(c model.TestCounts) int { return c.Skipped }),
	testCount(
		"tests_expected_failure",
		"Expected-failure tests",
		func(c model.TestCounts) int { return c.ExpectedFailure },
	),
	testCount("tests_not_run", "Tests not run", func(c model.TestCounts) int { return c.NotRun }),
	column("failure_evidence_status", "Evidence reporting", funcapi.FieldTypeString, "", func(r model.Run) any {
		return r.FailureEvidence.ReportingStatus()
	}),
	column("first_failure", model.FirstFailureLabel, funcapi.FieldTypeString, "", func(r model.Run) any {
		return r.FailureEvidence.FailureSummary()
	}),
	column("last_observed_step", model.LastObservedStepLabel, funcapi.FieldTypeString, "", func(r model.Run) any {
		return r.FailureEvidence.LastObservedStepSummary()
	}),
	column("stop_context", model.StopContextLabel, funcapi.FieldTypeString, "", func(r model.Run) any {
		return r.FailureEvidence.StopSummary()
	}),
	column("browser_summary_status", "Browser summary reporting", funcapi.FieldTypeString, "", func(r model.Run) any {
		return r.BrowserSummary.ReportingStatus()
	}),
	column("browser_errors", model.BrowserErrorsLabel, funcapi.FieldTypeString, "", func(r model.Run) any {
		return r.BrowserSummary.Summary()
	}),
})

// testCount reports one journey test count, or null when the run reported none.
func testCount(name, tooltip string, count func(model.TestCounts) int) rawfunc.Column[model.Run] {
	return column(name, tooltip, funcapi.FieldTypeInteger, "", func(r model.Run) any {
		if r.Tests == nil {
			return nil
		}
		return count(*r.Tests)
	})
}

var lighthouseRunsColumns = slices.Concat(runsColumns, rawfunc.Columns[model.Run]{
	labMetric("performance", "Performance score", "points", func(m model.LabMetrics) *float64 { return m.Performance }),
	labMetric("fcp_ms", "First Contentful Paint", "ms", func(m model.LabMetrics) *float64 { return m.FCPMS }),
	labMetric("lcp_ms", "Largest Contentful Paint", "ms", func(m model.LabMetrics) *float64 { return m.LCPMS }),
	labMetric("tbt_ms", "Total Blocking Time", "ms", func(m model.LabMetrics) *float64 { return m.TBTMS }),
	labMetric("si_ms", "Speed Index", "ms", func(m model.LabMetrics) *float64 { return m.SIMS }),
	labMetric("cls", "Cumulative Layout Shift", "", func(m model.LabMetrics) *float64 { return m.CLS }),
})

// labMetric reports one Lighthouse lab measurement, or null when it is missing.
func labMetric(name, tooltip, units string, value func(model.LabMetrics) *float64) rawfunc.Column[model.Run] {
	return column(name, tooltip, funcapi.FieldTypeFloat, units, func(r model.Run) any {
		if r.Metrics == nil {
			return nil
		}
		return number(value(*r.Metrics))
	})
}
