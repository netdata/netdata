// SPDX-License-Identifier: GPL-3.0-or-later

package functions_test

import (
	"context"
	"encoding/json"
	"testing"
	"time"

	demjournal "github.com/netdata/netdata/go/plugins/plugin/dem/journal"
	model "github.com/netdata/netdata/go/plugins/plugin/dem/synthetic"
	syntheticfunctions "github.com/netdata/netdata/go/plugins/plugin/dem/synthetic/functions"
	synthetichistory "github.com/netdata/netdata/go/plugins/plugin/dem/synthetic/history"
	"github.com/netdata/netdata/go/plugins/plugin/dem/synthetic/query"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// A retained run without events or artifacts reads back with null lists; synthetics-run shows them as empty.
func TestStoredRunWithoutEventsOrArtifactsShowsEmptyLists(t *testing.T) {
	ctx := context.Background()
	journal, err := demjournal.Open(ctx, t.TempDir())
	require.NoError(t, err)
	t.Cleanup(func() { require.NoError(t, journal.Close()) })
	store := synthetichistory.NewStore(journal)
	started := time.Now().UnixMicro()
	_, err = store.AppendRun(ctx, model.HistoryComplete, model.Run{
		ID:          "r",
		JobID:       "lighthouse:home",
		Kind:        model.Lighthouse,
		Name:        "home",
		Outcome:     model.Failed,
		StartedUS:   started,
		CompletedUS: started + 1,
	})
	require.NoError(t, err)
	stored, err := store.GetRun(ctx, "lighthouse:home", "r")
	require.NoError(t, err)
	require.Nil(t, stored.Events)
	require.Nil(t, stored.Artifacts)

	got := call(syntheticfunctions.New(query.New(nil, store, nil, nil), nil), "synthetics-run", "job_id:lighthouse:home", "run_id:r")
	validate(t, got)
	encoded, err := json.Marshal(got.RawResponse)
	require.NoError(t, err)
	var response struct {
		Data json.RawMessage `json:"data"`
		Run  struct {
			Artifacts json.RawMessage `json:"artifacts"`
		} `json:"run"`
	}
	require.NoError(t, json.Unmarshal(encoded, &response))
	assert.JSONEq(t, `[]`, string(response.Data))
	assert.JSONEq(t, `[]`, string(response.Run.Artifacts))
}
