// SPDX-License-Identifier: GPL-3.0-or-later
package history_test

import (
	"context"
	"encoding/json"
	"math"
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"testing"
	"time"

	demjournal "github.com/netdata/netdata/go/plugins/plugin/dem/journal"
	rumhistory "github.com/netdata/netdata/go/plugins/plugin/dem/rum/history"
	"github.com/netdata/netdata/go/plugins/plugin/dem/synthetic"
	"github.com/netdata/netdata/go/plugins/plugin/dem/synthetic/history"
	sdk "github.com/netdata/systemd-journal-sdk/go/journal"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestSyntheticHistoryReopenMixedKindsAndUnmatchedStart(t *testing.T) {
	ctx := context.Background()
	root := filepath.Join(t.TempDir(), "history")
	st, err := demjournal.Open(ctx, root)
	require.NoError(t, err)
	t.Cleanup(func() { require.NoError(t, st.Close()) })
	old := time.Now().Add(-24 * time.Hour).UnixMicro()
	run := synthetic.Run{
		ID:           "a",
		JobID:        "journey:disabled",
		Kind:         synthetic.Journey,
		Name:         "disabled",
		StartedUS:    old,
		Outcome:      synthetic.Unknown,
		CaptureState: "disabled",
	}
	attempted, err := history.NewStore(st).AppendRun(ctx, "start", run)
	require.NoError(t, err)
	require.True(t, attempted)
	run.CompletedUS = old + 1000
	run.Outcome = synthetic.Failed
	run.Events = []synthetic.Event{{Kind: "error", Message: "failure detail"}}
	run.Tests = &synthetic.TestCounts{
		Declared: 1,
		Failed:   1,
	}
	_, err = history.NewStore(st).AppendRun(ctx, "complete", run)
	require.NoError(t, err)
	pending := synthetic.Run{
		ID:        "b",
		JobID:     "lighthouse:page",
		Kind:      synthetic.Lighthouse,
		Name:      "page",
		StartedUS: old + 2000,
		Outcome:   synthetic.Unknown,
	}
	_, err = history.NewStore(st).AppendRun(ctx, "start", pending)
	require.NoError(t, err)
	_, err = rumhistory.NewStore(st).
		AppendEvent(ctx, rumhistory.EventRecord{
			Site:       "shop",
			SessionID:  "s",
			Type:       "pageview",
			ObservedUS: old,
		})
	require.NoError(t, err)
	require.NoError(t, st.Sync(ctx))
	require.NoError(t, st.Close())
	st, err = demjournal.Open(ctx, root)
	require.NoError(t, err)
	page, err := history.NewStore(st).
		QueryRuns(ctx, history.RunFilter{
			After: old / 1000000,
			Limit: 1,
		})
	require.NoError(t, err)
	require.Len(t, page.Runs, 1)
	assert.True(t, page.Truncated)
	assert.Equal(t, synthetic.Unknown, page.Runs[0].Outcome)
	filtered, err := history.NewStore(st).QueryRuns(ctx, history.RunFilter{
		Outcome: synthetic.Unknown,
	})
	require.NoError(t, err)
	require.Len(t, filtered.Runs, 1)
	assert.Equal(t, "b", filtered.Runs[0].ID)
	detail, err := history.NewStore(st).GetRun(ctx, run.JobID, run.ID)
	require.NoError(t, err)
	assert.Equal(t, run, detail)
	_, err = history.NewStore(st).GetRun(ctx, "wrong", run.ID)
	assert.ErrorIs(t, err, os.ErrNotExist)
	rum, err := rumhistory.NewStore(st).QuerySessions(ctx, "", "", 0, time.Now().Unix()+1, 100)
	require.NoError(t, err)
	require.Len(t, rum, 1)
	cancelled, cancel := context.WithCancel(ctx)
	cancel()
	_, err = history.NewStore(st).QueryRuns(cancelled, history.RunFilter{})
	assert.ErrorIs(t, err, context.Canceled)
	require.NoError(t, st.EnforceHistoryRetention(ctx, 30, 1<<30))
	detail, err = history.NewStore(st).GetRun(ctx, run.JobID, run.ID)
	require.NoError(t, err)
	assert.Equal(t, synthetic.Failed, detail.Outcome)
}

func TestSyntheticHistoryOptionalUpperBound(t *testing.T) {
	ctx := context.Background()
	st, err := demjournal.Open(ctx, t.TempDir())
	require.NoError(t, err)
	t.Cleanup(func() { require.NoError(t, st.Close()) })
	run := synthetic.Run{
		ID:        "upper-bound",
		JobID:     "journey:bound",
		Kind:      synthetic.Journey,
		StartedUS: time.Now().UnixMicro(),
		Outcome:   synthetic.Unknown,
	}
	_, err = history.NewStore(st).AppendRun(ctx, "start", run)
	require.NoError(t, err)
	page, err := history.NewStore(st).QueryRuns(ctx, history.RunFilter{})
	require.NoError(t, err)
	require.Len(t, page.Runs, 1)
	epoch := int64(0)
	page, err = history.NewStore(st).QueryRuns(ctx, history.RunFilter{
		Before: &epoch,
	})
	require.NoError(t, err)
	assert.Empty(t, page.Runs)
	_, err = history.NewStore(st).QueryRuns(ctx, history.RunFilter{
		After:  1,
		Before: &epoch,
	})
	require.ErrorContains(t, err, "invalid synthetic start-time range")
}

// Saved time is contemporary; every selected attempt below started near epoch.
// Whole-second membership must resolve final evidence regardless of finish time.
func TestSyntheticStartMembershipAndRetainedPhases(t *testing.T) {
	ctx := context.Background()
	st, err := demjournal.Open(ctx, t.TempDir())
	require.NoError(t, err)
	t.Cleanup(func() { require.NoError(t, st.Close()) })
	historyStore := history.NewStore(st)
	duration := 350.25
	runs := []synthetic.Run{
		{ID: "before", JobID: "journey:one", Kind: synthetic.Journey, StartedUS: 119999999, CompletedUS: 130000000, Outcome: synthetic.Success},
		{ID: "lower", JobID: "journey:one", Kind: synthetic.Journey, StartedUS: 120000000, CompletedUS: 250000000, Outcome: synthetic.Failed},
		{ID: "rollback", JobID: "journey:one", Kind: synthetic.Journey, StartedUS: 125000000, CompletedUS: 10000000, DurationMS: &duration, Outcome: synthetic.Success},
		{ID: "complete-only", JobID: "lighthouse:one", Kind: synthetic.Lighthouse, StartedUS: 130000000, CompletedUS: 260000000, Outcome: synthetic.Timeout},
		{ID: "unmatched", JobID: "journey:one", Kind: synthetic.Journey, StartedUS: 140000000, Outcome: synthetic.Unknown},
		{ID: "upper", JobID: "journey:one", Kind: synthetic.Journey, StartedUS: 179999999, CompletedUS: 300000000, Outcome: synthetic.Success},
		{ID: "after", JobID: "journey:one", Kind: synthetic.Journey, StartedUS: 180000000, CompletedUS: 310000000, Outcome: synthetic.Success},
	}
	for _, run := range runs {
		if run.CompletedUS != 0 {
			_, err := historyStore.AppendRun(ctx, "complete", run)
			require.NoError(t, err)
		}
		if run.ID != "complete-only" {
			start := run
			start.CompletedUS = 0
			start.DurationMS = nil
			start.Outcome = synthetic.Unknown
			_, err := historyStore.AppendRun(ctx, "start", start)
			require.NoError(t, err)
		}
	}
	upper := int64(179)
	selected := []synthetic.Run{runs[5], runs[4], runs[3], runs[2], runs[1]}
	for name, test := range map[string]struct {
		filter    history.RunFilter
		want      []synthetic.Run
		truncated bool
	}{
		"whole seconds":                {history.RunFilter{After: 120, Before: &upper}, selected, false},
		"outcome after resolution":     {history.RunFilter{After: 120, Before: &upper, Outcome: synthetic.Unknown}, []synthetic.Run{runs[4]}, false},
		"completion after picker":      {history.RunFilter{After: 120, Before: &upper, Outcome: synthetic.Failed}, []synthetic.Run{runs[1]}, false},
		"completion survives rollback": {history.RunFilter{After: 120, Before: &upper, Outcome: synthetic.Success}, []synthetic.Run{runs[5], runs[2]}, false},
		"complete only":                {history.RunFilter{After: 120, Before: &upper, Kind: synthetic.Lighthouse}, []synthetic.Run{runs[3]}, false},
		"ordered limit":                {history.RunFilter{After: 120, Before: &upper, Limit: 2}, selected[:2], true},
		"maximum seconds":              {history.RunFilter{After: math.MaxInt64}, []synthetic.Run{}, false},
	} {
		t.Run(name, func(t *testing.T) {
			page, err := historyStore.QueryRuns(ctx, test.filter)
			require.NoError(t, err)
			assert.Equal(t, history.RunPage{Runs: test.want, Truncated: test.truncated}, page)
		})
	}
	for _, run := range runs {
		got, err := historyStore.GetRun(ctx, run.JobID, run.ID)
		require.NoError(t, err)
		assert.Equal(t, run, got)
	}
}

func TestSyntheticRejectsInvalidProducerRecordBeforeAppend(t *testing.T) {
	ctx := context.Background()
	st, err := demjournal.Open(ctx, t.TempDir())
	require.NoError(t, err)
	t.Cleanup(func() { require.NoError(t, st.Close()) })
	for name, test := range map[string]struct {
		phase string
		run   synthetic.Run
	}{
		"missing run":         {"start", synthetic.Run{JobID: "job", StartedUS: 1}},
		"missing job":         {"start", synthetic.Run{ID: "run", StartedUS: 1}},
		"missing start":       {"start", synthetic.Run{ID: "run", JobID: "job"}},
		"negative start":      {"start", synthetic.Run{ID: "run", JobID: "job", StartedUS: -1}},
		"missing completion":  {"complete", synthetic.Run{ID: "run", JobID: "job", StartedUS: 1}},
		"negative completion": {"complete", synthetic.Run{ID: "run", JobID: "job", StartedUS: 1, CompletedUS: -1}},
		"invalid phase":       {"other", synthetic.Run{ID: "run", JobID: "job", StartedUS: 1}},
	} {
		t.Run(name, func(t *testing.T) {
			attempted, err := history.NewStore(st).AppendRun(ctx, test.phase, test.run)
			require.Error(t, err)
			assert.False(t, attempted)
		})
	}
	page, err := history.NewStore(st).QueryRuns(ctx, history.RunFilter{})
	require.NoError(t, err)
	assert.Empty(t, page.Runs)
}

func TestSyntheticValidatesSelectedScalarAndJSONAgreement(t *testing.T) {
	cases := map[string]func(map[string][]string){
		"scalar JSON run mismatch":   func(fields map[string][]string) { fields["DEM_RUN_ID"] = []string{"different"} },
		"scalar JSON job mismatch":   func(fields map[string][]string) { fields["DEM_JOB_ID"] = []string{"different"} },
		"scalar JSON start mismatch": func(fields map[string][]string) { fields["DEM_STARTED_US"] = []string{"120000001"} },
		"invalid phase":              func(fields map[string][]string) { fields["DEM_PHASE"] = []string{"other"} },
		"malformed JSON":             func(fields map[string][]string) { fields["DEM_DATA"] = []string{"{"} },
	}
	for _, field := range []string{"DEM_PHASE", "DEM_JOB_ID", "DEM_RUN_ID", "DEM_DATA"} {
		cases["missing "+field] = func(fields map[string][]string) { delete(fields, field) }
		cases["empty "+field] = func(fields map[string][]string) { fields[field] = []string{""} }
		cases["multiple "+field] = func(fields map[string][]string) { fields[field] = append(fields[field], "other") }
	}
	for name, mutate := range cases {
		t.Run(name, func(t *testing.T) {
			ctx := context.Background()
			st, err := demjournal.Open(ctx, t.TempDir())
			require.NoError(t, err)
			t.Cleanup(func() { require.NoError(t, st.Close()) })
			run := synthetic.Run{ID: "run", JobID: "job", Kind: synthetic.Journey, StartedUS: 120000000, CompletedUS: 130000000, Outcome: synthetic.Success}
			data, err := json.Marshal(run)
			require.NoError(t, err)
			fields := map[string][]string{
				"DEM_KIND": {"synthetic"}, "DEM_PHASE": {"complete"}, "DEM_JOB_ID": {run.JobID},
				"DEM_RUN_ID": {run.ID}, "DEM_STARTED_US": {strconv.FormatInt(run.StartedUS, 10)}, "DEM_DATA": {string(data)},
			}
			mutate(fields)
			keys := make([]string, 0, len(fields))
			for key := range fields {
				keys = append(keys, key)
			}
			sort.Strings(keys)
			var encoded []sdk.Field
			for _, key := range keys {
				for _, value := range fields[key] {
					encoded = append(encoded, sdk.StringField(key, value))
				}
			}
			attempted, err := st.Append(ctx, encoded)
			require.NoError(t, err)
			require.True(t, attempted)
			upper := int64(120)
			_, err = history.NewStore(st).QueryRuns(ctx, history.RunFilter{After: 120, Before: &upper})
			require.Error(t, err, "malformed selected record: %s", name)
		})
	}
}
