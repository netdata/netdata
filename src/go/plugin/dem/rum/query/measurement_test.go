// SPDX-License-Identifier: GPL-3.0-or-later
package query_test

import (
	"context"
	"fmt"
	"testing"
	"time"

	"github.com/netdata/netdata/go/plugins/pkg/funcapi"
	"github.com/netdata/netdata/go/plugins/plugin/dem/journal"
	"github.com/netdata/netdata/go/plugins/plugin/dem/rum/beacon"
	"github.com/netdata/netdata/go/plugins/plugin/dem/rum/functions"
	"github.com/netdata/netdata/go/plugins/plugin/dem/rum/history"
	"github.com/netdata/netdata/go/plugins/plugin/dem/rum/query"
	"github.com/netdata/netdata/go/plugins/plugin/dem/rum/registry"
	"github.com/stretchr/testify/require"
)

func TestMeasurementIdentitySurvivesLiveHistoryAndFunctions(t *testing.T) {
	ctx := context.Background()
	journalStore, err := journal.Open(ctx, "")
	require.NoError(t, err)
	t.Cleanup(func() { require.NoError(t, journalStore.Close()) })
	store := history.NewStore(journalStore)
	hub := registry.New()
	owner, retire := addSite(t, hub, "shop", "first")
	writer := history.NewWriter("shop", store, owner)
	owner.SetHistorySink(writer)
	service := query.New(hub, store)
	handler := functions.New(service)
	now := time.Now()
	b := &beacon.Beacon{
		Site:         "shop",
		Received:     now,
		SessionID:    "session",
		ExperienceID: "opaque-secret-document",
		PageGroup:    "/entry",
		View:         "checkout",
		ViewID:       "view-one",
		Events:       []beacon.Event{{Kind: beacon.EventDocument, ID: "document", Revision: 1}},
		Vitals:       []beacon.Vital{{Name: beacon.CLS, ID: "opaque-secret-metric", Revision: 1, Value: 0.3}},
	}
	owner.Ingest(b)
	owner.Ingest(b)
	b.Events = []beacon.Event{{Kind: beacon.EventView, ID: "view-two", Revision: 1, Attrs: map[string]string{"toView": "payment"}}}
	b.View, b.ViewID = "payment", "view-two"
	b.Vitals[0].Revision, b.Vitals[0].Value = 2, 0.5
	owner.Ingest(b)
	pages, err := service.Pages(ctx, "shop")
	require.NoError(t, err)
	require.Len(t, pages, 1)
	require.Equal(t, "/entry", pages[0].Page)
	require.Equal(t, 1, pages[0].PageviewsWindow)
	require.Equal(t, 1, pages[0].Sessions)
	require.Equal(t, 1, pages[0].Vitals[beacon.CLS].N)
	require.Equal(t, 0.5, pages[0].Vitals[beacon.CLS].P75)
	sites, err := service.Sites(ctx)
	require.NoError(t, err)
	require.Equal(t, 1, sites[0].Activity.ObservedSessions)
	require.EqualValues(t, 1, sites[0].Activity.ApplicationViewsWindow)
	response := handler.HandleRaw(ctx, funcapi.RawMethodRequest{
		Method: "rum-live",
	}).RawResponse
	require.NotNil(t, response)
	columns := response["columns"].(map[string]any)
	value := func(row []any, key string) any { return row[columns[key].(map[string]any)["index"].(int)] }
	var updates []uint64
	for _, row := range response["data"].([][]any) {
		require.Equal(t, "opaque-secret-document", value(row, "experience_id"), "measurement IDs are not credential-masked display text")
		if value(row, "kind") == "vital" {
			require.Equal(t, "opaque-secret-metric", value(row, "metric_id"))
			require.Equal(t, 0, value(row, "pageview"))
			updates = append(updates, value(row, "revision").(uint64))
		}
	}
	require.Equal(t, []uint64{1, 2}, updates)
	pending, err := service.SessionEvents(ctx, "shop", "session")
	require.NoError(t, err)
	flush, cancel := context.WithCancel(ctx)
	cancel()
	writer.Run(flush)
	persisted, err := service.SessionEvents(ctx, "shop", "session")
	require.NoError(t, err)
	require.Equal(t, pending, persisted, "journal overlap must include all identity fields")
	retire()
	afterRetirement, err := service.SessionEvents(ctx, "shop", "session")
	require.NoError(t, err)
	require.Equal(t, pending, afterRetirement)
	sessions, err := service.Sessions(ctx, "shop", "", 0, now.Unix()+5)
	require.NoError(t, err)
	require.Len(t, sessions, 1)
	require.EqualValues(t, 1, sessions[0].Pageviews)
	require.EqualValues(t, 1, sessions[0].ApplicationViews)
}

func TestPagesExposeRetainedGroupsBeyondChartTopNAndWindowedSessions(t *testing.T) {
	hub := registry.New()
	owner, _ := addSite(t, hub, "shop", "first")
	now := time.Now()
	for i := range 25 {
		id := fmt.Sprint(i)
		owner.Ingest(&beacon.Beacon{
			Site:         "shop",
			Received:     now,
			SessionID:    id,
			ExperienceID: id,
			PageGroup:    "/page-" + id,
			Events:       []beacon.Event{{Kind: beacon.EventDocument, ID: id, Revision: 1}},
		})
	}
	owner.Ingest(&beacon.Beacon{
		Site:         "shop",
		Received:     now.Add(-10 * time.Minute),
		SessionID:    "expired",
		ExperienceID: "expired",
		PageGroup:    "/page-0",
		Events:       []beacon.Event{{Kind: beacon.EventDocument, ID: "expired", Revision: 1}},
	})
	service := query.New(hub, nil)
	pages, err := service.Pages(context.Background(), "shop")
	require.NoError(t, err)
	require.Len(t, pages, 25, "chart top-N is 20 but Functions must expose all retained groups")
	for _, page := range pages {
		require.Equal(t, 1, page.Sessions)
		require.Equal(t, 1, page.PageviewsWindow)
	}
	sites, err := service.Sites(context.Background())
	require.NoError(t, err)
	require.Equal(t, 25, sites[0].Activity.ObservedSessions)
}
