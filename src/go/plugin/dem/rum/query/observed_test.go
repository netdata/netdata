// SPDX-License-Identifier: GPL-3.0-or-later
package query_test

import (
	"context"
	"fmt"
	"testing"
	"time"

	"github.com/netdata/netdata/go/plugins/pkg/funcapi"
	"github.com/netdata/netdata/go/plugins/plugin/dem/journal"
	"github.com/netdata/netdata/go/plugins/plugin/dem/rum/aggregate"
	"github.com/netdata/netdata/go/plugins/plugin/dem/rum/beacon"
	"github.com/netdata/netdata/go/plugins/plugin/dem/rum/functions"
	"github.com/netdata/netdata/go/plugins/plugin/dem/rum/history"
	"github.com/netdata/netdata/go/plugins/plugin/dem/rum/query"
	"github.com/netdata/netdata/go/plugins/plugin/dem/rum/registry"
	"github.com/stretchr/testify/require"
)

func TestPromotionPreservesReceiptTimeThroughJournalAndFunction(t *testing.T) {
	ctx := context.Background()
	journalStore, err := journal.Open(ctx, "")
	require.NoError(t, err)
	t.Cleanup(func() { require.NoError(t, journalStore.Close()) })
	store := history.NewStore(journalStore)
	owner := aggregate.New(time.Hour, aggregate.SiteCfg{
		Name: "shop", PageGroups: 20, Countries: 20,
		Investigate: aggregate.InvestigateCfg{Rate: 0, KeepErrors: true},
	})
	writer := history.NewWriter("shop", store, owner)
	owner.SetHistorySink(writer)
	observed := time.Now().Add(-20 * time.Minute).Truncate(time.Second)
	prior := &beacon.Beacon{
		Site: "shop", SessionID: "promoted", ExperienceID: "document", Received: observed,
		PageGroup: "/prior", UserID: "prior-user",
		Events: []beacon.Event{{Kind: beacon.EventDocument, ID: "document", Revision: 1, Time: observed.Add(-24 * time.Hour)}},
	}
	require.False(t, owner.Ingest(prior).Investigated)
	rows, err := store.QuerySessions(ctx, "shop", "", observed.Unix(), observed.Unix(), 0)
	require.NoError(t, err)
	require.Empty(t, rows)
	trigger := &beacon.Beacon{
		Site: "shop", SessionID: "promoted", ExperienceID: "document", Received: observed.Add(time.Minute),
		PageGroup: "/error", Errors: []beacon.Error{{Fingerprint: "failure", Type: "Error", Message: "boom", Time: observed.Add(-24 * time.Hour)}},
	}
	require.True(t, owner.Ingest(trigger).Investigated)
	flush, cancel := context.WithCancel(ctx)
	cancel()
	writer.Run(flush)
	// Another session's later receipt is in the same whole second, saved after promotion.
	_, err = store.AppendEvent(ctx, history.EventRecord{Site: "shop", SessionID: "newer", Type: "pageview", ObservedUS: observed.UnixMicro() + 999_999})
	require.NoError(t, err)
	handler := functions.New(query.New(registry.New(), store))
	response := handler.HandleRaw(ctx, funcapi.RawMethodRequest{
		Method: "rum-sessions", Args: []string{"site:shop", fmt.Sprintf("after:%d", observed.Unix()), fmt.Sprintf("before:%d", observed.Unix())},
	}).RawResponse
	require.NotNil(t, response)
	require.Equal(t, "last_observed_us", response["default_sort_column"])
	columns := response["columns"].(map[string]any)
	last := columns["last_observed_us"].(map[string]any)
	require.Equal(t, "descending", last["sort"])
	require.Equal(t, "timestamp", last["type"])
	data := response["data"].([][]any)
	require.Len(t, data, 2)
	require.Equal(t, "newer", data[0][1])
	require.Equal(t, observed.UnixMicro()+999_999, data[0][last["index"].(int)])
	require.Equal(t, "promoted", data[1][1])
	require.EqualValues(t, 1, data[1][4])
	require.EqualValues(t, 0, data[1][5])
	require.Equal(t, []string{"prior-user"}, data[1][12])
	timeline := handler.HandleRaw(ctx, funcapi.RawMethodRequest{Method: "rum-session-events", Args: []string{"site:shop", "session_id:promoted"}}).RawResponse
	require.NotNil(t, timeline)
	events := timeline["data"].([][]any)
	require.Len(t, events, 2)
	require.Equal(t, observed.UnixMicro(), events[0][0], "browser event time must not become the native history clock")
	require.Equal(t, observed.Add(time.Minute).UnixMicro(), events[1][0], "identity timeline includes the later promotion trigger")
	// No observation in the gap implies no selected activity, despite observations on both sides.
	gap := handler.HandleRaw(ctx, funcapi.RawMethodRequest{Method: "rum-sessions", Args: []string{"site:shop", fmt.Sprintf("after:%d", observed.Unix()+10), fmt.Sprintf("before:%d", observed.Unix()+20)}}).RawResponse
	require.NotNil(t, gap)
	require.Empty(t, gap["data"])
}
