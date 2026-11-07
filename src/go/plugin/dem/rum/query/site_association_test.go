// SPDX-License-Identifier: GPL-3.0-or-later

package query_test

import (
	"context"
	"testing"
	"time"

	"github.com/netdata/netdata/go/plugins/pkg/funcapi"
	"github.com/netdata/netdata/go/plugins/plugin/dem/journal"
	"github.com/netdata/netdata/go/plugins/plugin/dem/rum/aggregate"
	"github.com/netdata/netdata/go/plugins/plugin/dem/rum/beacon"
	"github.com/netdata/netdata/go/plugins/plugin/dem/rum/config"
	"github.com/netdata/netdata/go/plugins/plugin/dem/rum/diagnostics"
	rumfunctions "github.com/netdata/netdata/go/plugins/plugin/dem/rum/functions"
	"github.com/netdata/netdata/go/plugins/plugin/dem/rum/history"
	"github.com/netdata/netdata/go/plugins/plugin/dem/rum/httpapi"
	"github.com/netdata/netdata/go/plugins/plugin/dem/rum/query"
	rumregistry "github.com/netdata/netdata/go/plugins/plugin/dem/rum/registry"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestSiteAssociationCapturesHistoryAcrossReassignment(t *testing.T) {
	ctx := context.Background()
	owner, err := journal.Open(ctx, t.TempDir())
	require.NoError(t, err)
	t.Cleanup(func() { require.NoError(t, owner.Close()) })
	store := history.NewStore(owner)
	hub := rumregistry.New()
	service := query.New(hub, store)
	now := time.Now()
	type runtime struct {
		a      *aggregate.Aggregator
		w      *history.Writer
		retire func()
	}
	start := func(job, group, generation string) runtime {
		a := aggregate.New(5*time.Minute, aggregate.SiteCfg{
			Name:       job,
			SiteID:     group,
			PageGroups: 20,
			Countries:  20,
			Investigate: aggregate.InvestigateCfg{
				Rate:       0,
				KeepErrors: true,
			},
		})
		w := history.NewWriter(job, store, a)
		a.SetHistorySink(w)
		d := diagnostics.New()
		route := httpapi.NewRoute(config.Site{
			Name:   job,
			SiteID: group,
		}, measurementProcessor{a}, d)
		retire, err := hub.Register(job, &rumregistry.Site{
			Route:       route,
			Diagnostics: d,
			Aggregator:  a,
			Generation:  generation,
		})
		require.NoError(t, err)
		t.Cleanup(retire)
		return runtime{a, w, retire}
	}
	flush := func(w *history.Writer) { c, cancel := context.WithCancel(ctx); cancel(); w.Run(c) }
	ingest := func(r runtime, job string, at time.Time) {
		b := &beacon.Beacon{
			Site:         job,
			SiteID:       "spoofed",
			SessionID:    "shared",
			ExperienceID: "doc",
			PageGroup:    "/",
			Received:     at,
			Events:       []beacon.Event{{Kind: beacon.EventDocument, ID: "doc", Revision: 1}},
		}
		result := r.a.Ingest(b)
		require.True(t, result.Accepted)
		assert.Equal(t, "spoofed", b.SiteID, "ingestion must not mutate the browser observation")
		b.Events = nil
		b.Received = at.Add(time.Microsecond)
		b.Errors = []beacon.Error{{Fingerprint: "same", Type: "Error", Message: "broken"}}
		r.a.Ingest(b) // Promotion must retain the original association on preceding context.
	}
	old := start("job-a", "storefront", "old")
	other := start("job-b", "storefront", "other")
	ingest(old, "job-a", now)
	ingest(other, "job-b", now)
	live, cursor, err := service.Live(ctx, "", "", 2000, "storefront")
	require.NoError(t, err)
	require.Len(t, live, 4)
	for _, row := range live {
		assert.Equal(t, "storefront", row.SiteID)
	}
	pages, err := service.Pages(ctx, "", "storefront")
	require.NoError(t, err)
	require.Len(t, pages, 2)
	pending, err := service.SessionEvents(ctx, "job-a", "shared", "storefront")
	require.NoError(t, err)
	require.Len(t, pending, 2)
	for _, row := range pending {
		assert.Equal(t, "storefront", row.SiteID)
	}
	flush(old.w)
	flush(other.w)
	persisted, err := service.SessionEvents(ctx, "job-a", "shared", "storefront")
	require.NoError(t, err)
	assert.Equal(t, pending, persisted, "active and journal copies must deduplicate")
	old.retire()
	current := start("job-a", "checkout", "new")
	ingest(current, "job-a", now.Add(time.Second))
	flush(current.w)
	rows, err := service.Sessions(ctx, "", "", 0, now.Unix()+2, "")
	require.NoError(t, err)
	require.Len(t, rows, 3, "same group must not merge jobs; reassignment must not merge groups")
	selected, err := service.Sessions(ctx, "job-a", "", 0, now.Unix()+2, "storefront")
	require.NoError(t, err)
	require.Len(t, selected, 1)
	assert.Equal(t, int64(1), selected[0].Errors)
	errors, err := service.Errors(ctx, "", "same", 0, now.Unix()+2, "")
	require.NoError(t, err)
	require.Len(t, errors, 3)
	retained, err := service.SessionEvents(ctx, "job-a", "shared", "storefront")
	require.NoError(t, err)
	assert.Equal(t, pending, retained, "new config must not relabel previous events")
	filtered, err := service.SessionEvents(ctx, "job-b", "shared", "checkout")
	require.NoError(t, err)
	assert.Empty(t, filtered, "selectors combine")
	after, _, err := service.Live(ctx, "", cursor, 2000, "checkout")
	require.NoError(t, err)
	require.Len(t, after, 2, "an association filter must not advance another job cursor")
	sites, err := service.Sites(ctx, "storefront")
	require.NoError(t, err)
	require.Len(t, sites, 1)
	assert.Equal(t, "job-b", sites[0].Name)
	// Actual Function requests carry both selectors and expose the stored group.
	handler := rumfunctions.New(service)
	response := handler.HandleRaw(ctx, funcapi.RawMethodRequest{
		Method: "rum-session-events",
		Args:   []string{"site:job-a", "site_id:storefront", "session_id:shared"},
	})
	require.Equal(t, 200, response.RawResponse["status"])
	table := response.RawResponse["data"].([][]any)
	require.Len(t, table, 2)
	for _, row := range table {
		assert.Equal(t, "storefront", row[11])
	}
}
