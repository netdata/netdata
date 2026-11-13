// SPDX-License-Identifier: GPL-3.0-or-later

package history_test

import (
	"testing"
	"time"

	demjournal "github.com/netdata/netdata/go/plugins/plugin/dem/journal"
	"github.com/netdata/netdata/go/plugins/plugin/dem/synthetic"
	"github.com/netdata/netdata/go/plugins/plugin/dem/synthetic/history"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestFailureEvidenceSurvivesHistoryReopen(t *testing.T) {
	ctx := t.Context()
	root := t.TempDir()
	journal, err := demjournal.Open(ctx, root)
	require.NoError(t, err)
	now := time.Now().UnixMicro()
	duration := 5.25
	location := &synthetic.EvidenceLocation{File: "checkout.spec.js", Line: 12, Column: 3}
	test := synthetic.EvidenceTest{ID: "t", Title: "checkout", Location: location, Status: "failed", ExpectedStatus: "passed", DurationMS: &duration, ReportingComplete: true}
	step := synthetic.EvidenceStep{ID: "s1", TestID: "t", Title: "submit", Category: "test.step", Location: location, State: "running"}
	evidence := &synthetic.FailureEvidence{Availability: "supported", ReporterComplete: false, Truncated: []string{"other_steps", "text"}, Tests: []synthetic.EvidenceTest{test}, Steps: []synthetic.EvidenceStep{step}, FirstFailure: &synthetic.FailureDetail{Test: test, Path: []synthetic.EvidenceStep{step}, Message: "expected receipt", Location: location, TextTruncated: true}}
	stopped := test
	stopped.ID, stopped.Status = "stopped", "interrupted"
	pending := step
	pending.TestID = stopped.ID
	evidence.StopContext = &synthetic.FailureDetail{Test: stopped, Path: []synthetic.EvidenceStep{pending}, PathIncomplete: true}
	runs := []synthetic.Run{
		{ID: "old", JobID: "journey:a", Kind: synthetic.Journey, StartedUS: now, CompletedUS: now + 1, Outcome: synthetic.Failed},
		{ID: "rich", JobID: "journey:a", Kind: synthetic.Journey, StartedUS: now + 2, CompletedUS: now + 3, Outcome: synthetic.Timeout, TerminationReason: synthetic.ExecutionBudget, FailureEvidence: evidence},
	}
	for _, run := range runs {
		_, err = history.NewStore(journal).AppendRun(ctx, "complete", run)
		require.NoError(t, err)
	}
	require.NoError(t, journal.Sync(ctx))
	require.NoError(t, journal.Close())
	journal, err = demjournal.Open(ctx, root)
	require.NoError(t, err)
	defer func() { require.NoError(t, journal.Close()) }()
	store := history.NewStore(journal)
	for _, run := range runs {
		retained, err := store.GetRun(ctx, run.JobID, run.ID)
		require.NoError(t, err)
		assert.Equal(t, run, retained)
	}
	page, err := store.QueryRuns(ctx, history.RunFilter{})
	require.NoError(t, err)
	require.Len(t, page.Runs, 2)
	assert.Equal(t, runs[1], page.Runs[0])
	assert.Nil(t, page.Runs[1].FailureEvidence)
}
