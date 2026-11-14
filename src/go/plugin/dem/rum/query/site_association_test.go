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
	rumfunctions "github.com/netdata/netdata/go/plugins/plugin/dem/rum/functions"
	"github.com/netdata/netdata/go/plugins/plugin/dem/rum/history"
	"github.com/netdata/netdata/go/plugins/plugin/dem/rum/httpapi"
	"github.com/netdata/netdata/go/plugins/plugin/dem/rum/query"
	rumregistry "github.com/netdata/netdata/go/plugins/plugin/dem/rum/registry"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestVnodeCapturesHistoryAcrossInPlaceReassignment(t *testing.T) {
	ctx := context.Background()
	owner, err := journal.Open(ctx, t.TempDir())
	require.NoError(t, err)
	t.Cleanup(func() { require.NoError(t, owner.Close()) })
	store := history.NewStore(owner)
	hub := rumregistry.New()
	service := query.New(hub, store)
	now := time.Now()
	type runtime struct {
		a *aggregate.Aggregator
		w *history.Writer
	}
	start := func(job, group, generation string) runtime {
		a := aggregate.New(5*time.Minute, aggregate.SiteCfg{
			Name:       job,
			VnodeGUID:  group,
			PageGroups: 20,
			Countries:  20,
			Investigate: aggregate.InvestigateCfg{
				Rate:       0,
				KeepErrors: true,
			},
		})
		w := history.NewWriter(store, discardCounters{})
		a.SetHistorySink(w)
		route := httpapi.NewRoute(config.Site{
			Name: job,
		}, measurementProcessor{a}, nil)
		retire, err := hub.Register(job, &rumregistry.Site{
			Route:      route,
			Aggregator: a,
			Generation: generation,
		})
		require.NoError(t, err)
		t.Cleanup(retire)
		return runtime{a, w}
	}
	flush := func(w *history.Writer) { c, cancel := context.WithCancel(ctx); cancel(); w.Run(c) }
	ingest := func(r runtime, job string, at time.Time) {
		b := &beacon.Beacon{
			Site:         job,
			VnodeGUID:    "spoofed",
			SessionID:    "shared",
			ExperienceID: "doc",
			PageGroup:    "/",
			Received:     at,
			Events:       []beacon.Event{{Kind: beacon.EventDocument, ID: "doc", Revision: 1}},
		}
		r.a.Ingest(b)
		assert.Equal(t, "spoofed", b.VnodeGUID, "ingestion must not mutate the browser observation")
		b.Events = nil
		b.Received = at.Add(time.Microsecond)
		b.Errors = []beacon.Error{{Fingerprint: "same", Type: "Error", Message: "broken"}}
		r.a.Ingest(b) // Promotion must retain the original association on preceding context.
	}
	old := start("job-a", "11111111-1111-4111-8111-111111111111", "old")
	other := start("job-b", "11111111-1111-4111-8111-111111111111", "other")
	ingest(old, "job-a", now)
	ingest(other, "job-b", now)
	live, cursor, err := service.Live(ctx, "", "", 2000, "11111111-1111-4111-8111-111111111111")
	require.NoError(t, err)
	require.Len(t, live, 4)
	for _, row := range live {
		assert.Equal(t, "11111111-1111-4111-8111-111111111111", row.VnodeGUID)
	}
	pages, err := service.Pages(ctx, "", "11111111-1111-4111-8111-111111111111")
	require.NoError(t, err)
	require.Len(t, pages, 2)
	pending, err := service.SessionEvents(ctx, "job-a", "shared", "11111111-1111-4111-8111-111111111111")
	require.NoError(t, err)
	require.Len(t, pending, 2)
	for _, row := range pending {
		assert.Equal(t, "11111111-1111-4111-8111-111111111111", row.VnodeGUID)
	}
	flush(old.w)
	flush(other.w)
	persisted, err := service.SessionEvents(ctx, "job-a", "shared", "11111111-1111-4111-8111-111111111111")
	require.NoError(t, err)
	assert.Equal(t, pending, persisted, "active and journal copies must deduplicate")
	old.a.SetVnodeGUID("22222222-2222-4222-8222-222222222222")
	current := old
	ingest(current, "job-a", now.Add(time.Second))
	flush(current.w)
	rows, err := service.Sessions(ctx, "", "", 0, now.Unix()+2, "")
	require.NoError(t, err)
	require.Len(t, rows, 3, "same group must not merge jobs; reassignment must not merge groups")
	selected, err := service.Sessions(ctx, "job-a", "", 0, now.Unix()+2, "11111111-1111-4111-8111-111111111111")
	require.NoError(t, err)
	require.Len(t, selected, 1)
	assert.Equal(t, int64(1), selected[0].Errors)
	errors, err := service.Errors(ctx, "", "same", 0, now.Unix()+2, "")
	require.NoError(t, err)
	require.Len(t, errors, 3)
	retained, err := service.SessionEvents(ctx, "job-a", "shared", "11111111-1111-4111-8111-111111111111")
	require.NoError(t, err)
	assert.Equal(t, pending, retained, "new config must not relabel previous events")
	filtered, err := service.SessionEvents(ctx, "job-b", "shared", "22222222-2222-4222-8222-222222222222")
	require.NoError(t, err)
	assert.Empty(t, filtered, "selectors combine")
	after, _, err := service.Live(ctx, "", cursor, 2000, "22222222-2222-4222-8222-222222222222")
	require.NoError(t, err)
	require.Len(t, after, 2, "an association filter must not advance another job cursor")
	sites, err := service.Sites(ctx, "11111111-1111-4111-8111-111111111111")
	require.NoError(t, err)
	require.Len(t, sites, 1)
	assert.Equal(t, "job-b", sites[0].Name)
	// The old GUID still returns retained rows after in-place reassignment.
	for _, method := range []string{"rum-sessions", "rum-errors"} {
		response := rumfunctions.New(service, nil).HandleRaw(ctx, funcapi.RawMethodRequest{
			Method: method,
			Args:   []string{"site:job-a", "vnode_guid:11111111-1111-4111-8111-111111111111"},
		})
		require.NotEmpty(t, response.RawResponse["data"].([][]any))
		help := response.RawResponse["help"].(string)
		assert.Contains(t, help, "earlier policies", "historical caveat must survive current-identity reassignment")
		assert.Contains(t, help, "do not estimate all visitors or errors")
	}
	// Actual Function requests carry both selectors and expose the stored group.
	handler := rumfunctions.New(service, nil)
	response := handler.HandleRaw(ctx, funcapi.RawMethodRequest{
		Method: "rum-session-events",
		Args:   []string{"site:job-a", "vnode_guid:11111111-1111-4111-8111-111111111111", "session_id:shared"},
	})
	require.Equal(t, 200, response.RawResponse["status"])
	table := response.RawResponse["data"].([][]any)
	require.Len(t, table, 2)
	for _, row := range table {
		assert.Equal(t, "11111111-1111-4111-8111-111111111111", row[11])
	}
}
