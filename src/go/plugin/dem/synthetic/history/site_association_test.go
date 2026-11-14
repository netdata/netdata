// SPDX-License-Identifier: GPL-3.0-or-later
package history_test

import (
	"context"
	"testing"
	"time"

	demjournal "github.com/netdata/netdata/go/plugins/plugin/dem/journal"
	"github.com/netdata/netdata/go/plugins/plugin/dem/synthetic"
	"github.com/netdata/netdata/go/plugins/plugin/dem/synthetic/history"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestHistoryRetainsAssociationAcrossJobReassignment(t *testing.T) {
	ctx := context.Background()
	root := t.TempDir()
	journal, err := demjournal.Open(ctx, root)
	require.NoError(t, err)
	store := history.NewStore(journal)
	now := time.Now().UnixMicro()
	for i, site := range []string{"11111111-1111-4111-8111-111111111111", "22222222-2222-4222-8222-222222222222"} {
		run := synthetic.Run{ID: []string{"before-move", "after-move"}[i], JobID: "journey:checkout", Kind: synthetic.Journey, VnodeGUID: site, StartedUS: now + int64(i), Outcome: synthetic.Unknown}
		_, err = store.AppendRun(ctx, "start", run)
		require.NoError(t, err)
		run.CompletedUS = run.StartedUS + 100
		run.Outcome = synthetic.Success
		_, err = store.AppendRun(ctx, "complete", run)
		require.NoError(t, err)
	}
	require.NoError(t, journal.Close())
	journal, err = demjournal.Open(ctx, root)
	require.NoError(t, err)
	t.Cleanup(func() { require.NoError(t, journal.Close()) })
	store = history.NewStore(journal)
	upper := now/1e6 + 1
	filtered, err := store.QueryRuns(ctx, history.RunFilter{VnodeGUID: "11111111-1111-4111-8111-111111111111", JobID: "journey:checkout", Kind: synthetic.Journey, Outcome: synthetic.Success, After: now / 1e6, Before: &upper})
	require.NoError(t, err)
	require.Len(t, filtered.Runs, 1)
	assert.Equal(t, "before-move", filtered.Runs[0].ID)
	assert.Equal(t, "11111111-1111-4111-8111-111111111111", filtered.Runs[0].VnodeGUID)
	filtered, err = store.QueryRuns(ctx, history.RunFilter{VnodeGUID: "11111111-1111-4111-8111-111111111111", Kind: synthetic.Lighthouse})
	require.NoError(t, err)
	assert.Empty(t, filtered.Runs)
	all, err := store.QueryRuns(ctx, history.RunFilter{})
	require.NoError(t, err)
	require.Len(t, all.Runs, 2)
	detail, err := store.GetRun(ctx, "journey:checkout", "before-move")
	require.NoError(t, err)
	assert.Equal(t, "11111111-1111-4111-8111-111111111111", detail.VnodeGUID)
}
