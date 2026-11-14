// SPDX-License-Identifier: GPL-3.0-or-later
package functions_test

import (
	"context"
	"testing"
	"time"

	"github.com/netdata/netdata/go/plugins/pkg/funcapi"
	demjournal "github.com/netdata/netdata/go/plugins/plugin/dem/journal"
	model "github.com/netdata/netdata/go/plugins/plugin/dem/synthetic"
	functions "github.com/netdata/netdata/go/plugins/plugin/dem/synthetic/functions"
	history "github.com/netdata/netdata/go/plugins/plugin/dem/synthetic/history"
	"github.com/netdata/netdata/go/plugins/plugin/dem/synthetic/monitor"
	"github.com/netdata/netdata/go/plugins/plugin/dem/synthetic/query"
	registry "github.com/netdata/netdata/go/plugins/plugin/dem/synthetic/registry"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestFunctionSiteFilterUsesIndependentCurrentAndHistoricalAssociations(t *testing.T) {
	ctx := context.Background()
	journal, err := demjournal.Open(ctx, t.TempDir())
	require.NoError(t, err)
	t.Cleanup(func() { require.NoError(t, journal.Close()) })
	store := history.NewStore(journal)
	active := activeJobs{runs: map[string]model.Run{}}
	for i, site := range []string{"11111111-1111-4111-8111-111111111111", "22222222-2222-4222-8222-222222222222", ""} {
		name := []string{"checkout", "search", "legacy"}[i]
		jobID := "journey:" + name
		active.jobs = append(active.jobs, registry.Job{Snapshot: monitor.Snapshot{JobID: jobID, Name: name, Kind: model.Journey, VnodeGUID: site}})
		run := model.Run{ID: name, JobID: jobID, Name: name, Kind: model.Journey, VnodeGUID: site, Outcome: model.Success, StartedUS: time.Now().UnixMicro(), CompletedUS: time.Now().UnixMicro()}
		if i == 0 {
			run.VnodeGUID = "33333333-3333-4333-8333-333333333333"
		}
		_, err = store.AppendRun(ctx, "complete", run)
		require.NoError(t, err)
		active.runs[jobID] = run
	}
	h := functions.New(query.New(active, store, nil, nil), nil)
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
