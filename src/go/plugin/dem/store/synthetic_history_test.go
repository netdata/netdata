// SPDX-License-Identifier: GPL-3.0-or-later
package store_test

import (
	"context"
	"github.com/netdata/netdata/go/plugins/plugin/dem/store"
	"github.com/netdata/netdata/go/plugins/plugin/dem/synthetic"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"os"
	"path/filepath"
	"testing"
	"time"
)

func TestSyntheticHistoryReopenMixedKindsAndUnmatchedStart(t *testing.T) {
	ctx := context.Background()
	root := filepath.Join(t.TempDir(), "history")
	st, err := store.Open(ctx, root)
	require.NoError(t, err)
	t.Cleanup(func() { require.NoError(t, st.Close()) })
	old := time.Now().Add(-24 * time.Hour).UnixMicro()
	run := synthetic.Run{ID: "a", JobID: "journey:disabled", Kind: synthetic.Journey, Name: "disabled", StartedUS: old, Outcome: synthetic.Unknown, CaptureState: "disabled"}
	attempted, err := st.AppendSyntheticRun(ctx, "start", run)
	require.NoError(t, err)
	require.True(t, attempted)
	run.CompletedUS = old + 1000
	run.Outcome = synthetic.Failed
	run.Events = []synthetic.Event{{Kind: "error", Message: "failure detail"}}
	run.Tests = &synthetic.TestCounts{Declared: 1, Failed: 1}
	_, err = st.AppendSyntheticRun(ctx, "complete", run)
	require.NoError(t, err)
	pending := synthetic.Run{ID: "b", JobID: "lighthouse:page", Kind: synthetic.Lighthouse, Name: "page", StartedUS: old + 2000, Outcome: synthetic.Unknown}
	_, err = st.AppendSyntheticRun(ctx, "start", pending)
	require.NoError(t, err)
	_, err = st.AppendRumEvent(ctx, store.RumEventRecord{Site: "shop", SessionID: "s", Type: "pageview", TSUnixUS: old})
	require.NoError(t, err)
	require.NoError(t, st.Sync(ctx))
	require.NoError(t, st.Close())
	st, err = store.Open(ctx, root)
	require.NoError(t, err)
	page, err := st.QuerySyntheticRuns(ctx, synthetic.RunFilter{After: time.Now().Add(-time.Minute).Unix(), Limit: 1})
	require.NoError(t, err)
	require.Len(t, page.Runs, 1)
	assert.True(t, page.Truncated)
	assert.Equal(t, synthetic.Unknown, page.Runs[0].Outcome)
	filtered, err := st.QuerySyntheticRuns(ctx, synthetic.RunFilter{Outcome: synthetic.Unknown})
	require.NoError(t, err)
	require.Len(t, filtered.Runs, 1)
	assert.Equal(t, "b", filtered.Runs[0].ID)
	detail, err := st.GetSyntheticRun(ctx, run.JobID, run.ID)
	require.NoError(t, err)
	assert.Equal(t, run, detail)
	_, err = st.GetSyntheticRun(ctx, "wrong", run.ID)
	assert.ErrorIs(t, err, os.ErrNotExist)
	rum, err := st.QueryRumSessions(ctx, "", 0, time.Now().Unix()+1, 100)
	require.NoError(t, err)
	require.Len(t, rum, 1)
	cancelled, cancel := context.WithCancel(ctx)
	cancel()
	_, err = st.QuerySyntheticRuns(cancelled, synthetic.RunFilter{})
	assert.ErrorIs(t, err, context.Canceled)
	require.NoError(t, st.EnforceHistoryRetention(ctx, 30, 1<<30))
	detail, err = st.GetSyntheticRun(ctx, run.JobID, run.ID)
	require.NoError(t, err)
	assert.Equal(t, synthetic.Failed, detail.Outcome)
}

func TestSyntheticHistoryOptionalUpperBound(t *testing.T) {
	ctx := context.Background()
	st, err := store.Open(ctx, t.TempDir())
	require.NoError(t, err)
	t.Cleanup(func() { require.NoError(t, st.Close()) })
	run := synthetic.Run{ID: "upper-bound", JobID: "journey:bound", Kind: synthetic.Journey, StartedUS: time.Now().UnixMicro(), Outcome: synthetic.Unknown}
	_, err = st.AppendSyntheticRun(ctx, "start", run)
	require.NoError(t, err)
	page, err := st.QuerySyntheticRuns(ctx, synthetic.RunFilter{})
	require.NoError(t, err)
	require.Len(t, page.Runs, 1)
	epoch := int64(0)
	page, err = st.QuerySyntheticRuns(ctx, synthetic.RunFilter{Before: &epoch})
	require.NoError(t, err)
	assert.Empty(t, page.Runs)
	_, err = st.QuerySyntheticRuns(ctx, synthetic.RunFilter{After: 1, Before: &epoch})
	require.ErrorContains(t, err, "invalid synthetic saved-time range")
}
