// SPDX-License-Identifier: GPL-3.0-or-later
package history_test

import (
	"context"
	"encoding/json"
	"fmt"
	"testing"
	"time"

	demjournal "github.com/netdata/netdata/go/plugins/plugin/dem/journal"
	"github.com/netdata/netdata/go/plugins/plugin/dem/synthetic"
	"github.com/netdata/netdata/go/plugins/plugin/dem/synthetic/history"
	sdk "github.com/netdata/systemd-journal-sdk/go/journal"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestHistoryRetainsAssociationAcrossJobReassignmentAndLegacyRecords(t *testing.T) {
	ctx := context.Background()
	root := t.TempDir()
	journal, err := demjournal.Open(ctx, root)
	require.NoError(t, err)
	store := history.NewStore(journal)
	now := time.Now().UnixMicro()
	for i, site := range []string{"storefront", "other-site"} {
		run := synthetic.Run{ID: []string{"before-move", "after-move"}[i], JobID: "journey:checkout", Kind: synthetic.Journey, SiteID: site, StartedUS: now + int64(i), Outcome: synthetic.Unknown}
		_, err = store.AppendRun(ctx, "start", run)
		require.NoError(t, err)
		run.CompletedUS = run.StartedUS + 100
		run.Outcome = synthetic.Success
		_, err = store.AppendRun(ctx, "complete", run)
		require.NoError(t, err)
	}
	// The old journal format had neither a scalar nor a JSON site field.
	legacy := synthetic.Run{ID: "legacy", JobID: "lighthouse:home", Kind: synthetic.Lighthouse, StartedUS: now + 2, CompletedUS: now + 102, Outcome: synthetic.Success}
	raw, err := json.Marshal(legacy)
	require.NoError(t, err)
	_, err = journal.Append(ctx, []sdk.Field{sdk.StringField("DEM_KIND", "synthetic"), sdk.StringField("DEM_PHASE", "complete"), sdk.StringField("DEM_JOB_ID", legacy.JobID), sdk.StringField("DEM_RUN_ID", legacy.ID), sdk.StringField("DEM_STARTED_US", fmt.Sprint(legacy.StartedUS)), sdk.StringField("DEM_DATA", string(raw))})
	require.NoError(t, err)
	require.NoError(t, journal.Close())
	journal, err = demjournal.Open(ctx, root)
	require.NoError(t, err)
	t.Cleanup(func() { require.NoError(t, journal.Close()) })
	store = history.NewStore(journal)
	upper := now/1e6 + 1
	filtered, err := store.QueryRuns(ctx, history.RunFilter{SiteID: "storefront", JobID: "journey:checkout", Kind: synthetic.Journey, Outcome: synthetic.Success, After: now / 1e6, Before: &upper})
	require.NoError(t, err)
	require.Len(t, filtered.Runs, 1)
	assert.Equal(t, "before-move", filtered.Runs[0].ID)
	assert.Equal(t, "storefront", filtered.Runs[0].SiteID)
	filtered, err = store.QueryRuns(ctx, history.RunFilter{SiteID: "storefront", Kind: synthetic.Lighthouse})
	require.NoError(t, err)
	assert.Empty(t, filtered.Runs)
	all, err := store.QueryRuns(ctx, history.RunFilter{})
	require.NoError(t, err)
	require.Len(t, all.Runs, 3)
	old, err := store.GetRun(ctx, legacy.JobID, legacy.ID)
	require.NoError(t, err)
	assert.Empty(t, old.SiteID)
	detail, err := store.GetRun(ctx, "journey:checkout", "before-move")
	require.NoError(t, err)
	assert.Equal(t, "storefront", detail.SiteID)
}
