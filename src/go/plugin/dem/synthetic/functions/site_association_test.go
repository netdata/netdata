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
	hub := registry.New()
	for i, site := range []string{"storefront", "other-site", ""} {
		name := []string{"checkout", "search", "legacy"}[i]
		jobID := "journey:" + name
		job, err := hub.Register(registry.Job{JobID: jobID, Name: name, Kind: model.Journey, SiteID: site})
		require.NoError(t, err)
		defer job.Retire()
		run := model.Run{ID: name, JobID: jobID, Name: name, Kind: model.Journey, SiteID: site, Outcome: model.Success, StartedUS: time.Now().UnixMicro(), CompletedUS: time.Now().UnixMicro()}
		if i == 0 {
			run.SiteID = "previous-site"
		}
		_, err = store.AppendRun(ctx, "complete", run)
		require.NoError(t, err)
		job.Complete(run)
	}
	h := functions.New(query.New(hub, store, nil, nil))
	checks := call(h, "synthetics-checks", "site_id:storefront", "kind:journey", "job_id:journey:checkout")
	rows := responseData(t, checks)
	require.Len(t, rows, 1)
	assert.Equal(t, "storefront", rowValues(t, checks, rows[0])["site_id"])
	validate(t, checks)
	assert.Empty(t, responseData(t, call(h, "synthetics-checks", "site_id:storefront", "job_id:journey:search")))
	runs := call(h, "synthetics-runs", "site_id:previous-site", "kind:journey", "job_id:journey:checkout")
	rows = responseData(t, runs)
	require.Len(t, rows, 1)
	assert.Equal(t, "previous-site", rowValues(t, runs, rows[0])["site_id"])
	validate(t, runs)
	assert.Empty(t, responseData(t, call(h, "synthetics-runs", "site_id:storefront")))
	assert.Len(t, responseData(t, call(h, "synthetics-runs", "site_id:")), 3)
	detail := call(h, "synthetics-run", "job_id:journey:checkout", "run_id:checkout")
	require.NotNil(t, detail.RawResponse)
	assert.Equal(t, "previous-site", detail.RawResponse["run"].(map[string]any)["site_id"])
	for _, method := range []string{"synthetics-checks", "synthetics-runs", "synthetics-artifacts"} {
		assert.Equal(t, 400, call(h, method, "site_id:INVALID").Status)
		denied := h.HandleRaw(ctx, funcapi.RawMethodRequest{Method: method, Args: []string{"site_id:storefront"}, Permissions: "0"})
		assert.Equal(t, 403, denied.Status)
	}
}
