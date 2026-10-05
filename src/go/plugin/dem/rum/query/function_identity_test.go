// SPDX-License-Identifier: GPL-3.0-or-later
package query_test

import (
	"context"
	"testing"
	"time"

	"github.com/netdata/netdata/go/plugins/pkg/funcapi"
	"github.com/netdata/netdata/go/plugins/plugin/dem/journal"
	"github.com/netdata/netdata/go/plugins/plugin/dem/rum/beacon"
	"github.com/netdata/netdata/go/plugins/plugin/dem/rum/functions"
	"github.com/netdata/netdata/go/plugins/plugin/dem/rum/history"
	"github.com/netdata/netdata/go/plugins/plugin/dem/rum/query"
	"github.com/netdata/netdata/go/plugins/plugin/dem/rum/registry"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestFunctionRowsHaveSiteScopedIdentities(t *testing.T) {
	ctx := context.Background()
	journalStore, err := journal.Open(ctx, "")
	require.NoError(t, err)
	t.Cleanup(func() { require.NoError(t, journalStore.Close()) })
	retained := history.NewStore(journalStore)
	sites := registry.New()
	fingerprint := beacon.Fingerprint("TypeError", "failed", "app.js")
	for _, site := range []string{"a", "b"} {
		owner, _ := addSite(t, sites, site, "first")
		page(owner, site, "/shared")
		owner.Snapshot()
		attempted, err := retained.AppendEvent(
			ctx,
			history.EventRecord{
				Site:        site,
				SessionID:   "shared-session",
				TSUnixUS:    time.Now().UnixMicro(),
				Type:        "error",
				Page:        "/shared",
				Fingerprint: fingerprint,
				ErrorType:   "TypeError",
				Message:     "failed",
			},
		)
		require.True(t, attempted)
		require.NoError(t, err)
	}
	handler := functions.New(query.New(sites, retained))
	for method, field := range map[string]string{"rum-errors": "fingerprint", "rum-pages": "page", "rum-sessions": "session_id"} {
		t.Run(method, func(t *testing.T) {
			response := handler.HandleRaw(ctx, funcapi.RawMethodRequest{
				Method: method,
			})
			require.NotNil(t, response.RawResponse)
			rows := response.RawResponse["data"].([][]any)
			require.Len(t, rows, 2)
			columns := response.RawResponse["columns"].(map[string]any)
			keyIndex := -1
			for _, value := range columns {
				column := value.(map[string]any)
				if unique, _ := column["unique_key"].(bool); unique {
					require.Equal(t, -1, keyIndex, "one collision-safe row identity")
					keyIndex = column["index"].(int)
				}
			}
			require.NotEqual(t, -1, keyIndex)
			assert.NotEqual(
				t,
				rows[0][keyIndex],
				rows[1][keyIndex],
				"identical site-local values must identify different rows",
			)
			valueIndex := columns[field].(map[string]any)["index"].(int)
			assert.Equal(t, rows[0][valueIndex], rows[1][valueIndex], "the original filter value remains unchanged")
		})
	}
}

func TestFunctionsExposeRetainedIdentityAfterLogoutAndRetirement(t *testing.T) {
	ctx := context.Background()
	journalStore, err := journal.Open(ctx, "")
	require.NoError(t, err)
	t.Cleanup(func() { require.NoError(t, journalStore.Close()) })
	retained := history.NewStore(journalStore)
	sites := registry.New()
	owner, retire := addSite(t, sites, "shop", "first")
	writer := history.NewWriter("shop", retained, owner)
	owner.SetHistorySink(writer)
	// A shared SDK session can contain login, logout, and another login.
	ids := []string{"550e8400-e29b-41d4-a716-446655440000", "", "123456"}
	now := time.Now()
	for i, id := range ids {
		owner.Ingest(&beacon.Beacon{
			Site:         "shop",
			ExperienceID: "document",
			SessionID:    "shared",
			UserID:       id,
			PageGroup:    "/checkout",
			Received:     now.Add(time.Duration(i) * time.Second),
			Events:       []beacon.Event{{Kind: beacon.EventDocument, ID: "document", Revision: 1}, {Name: "milestone"}},
		})
	}
	handler := functions.New(query.New(sites, retained))
	timeline := func() [][]any {
		response := handler.HandleRaw(ctx, funcapi.RawMethodRequest{
			Method: "rum-session-events",
			Args:   []string{"site:shop", "session_id:shared"},
		})
		require.NotNil(t, response.RawResponse)
		return response.RawResponse["data"].([][]any)
	}
	pending := timeline()
	// Only the explicit document activation creates a document view.
	require.Len(t, pending, 4)
	assert.Equal(t, ids[0], pending[0][5])
	for i, id := range ids {
		assert.Equal(t, id, pending[i+1][5])
	}
	flushCtx, cancel := context.WithCancel(ctx)
	cancel()
	writer.Run(flushCtx)
	assert.Equal(t, pending, timeline(), "journal/live overlap includes event attribution")
	retire()
	assert.Equal(t, pending, timeline(), "history remains attributable without a running site")
	for _, id := range []string{ids[0], ids[2]} {
		response := handler.HandleRaw(ctx, funcapi.RawMethodRequest{
			Method: "rum-sessions",
			Args:   []string{"site:shop", "user_id:" + id},
		})
		require.NotNil(t, response.RawResponse)
		rows := response.RawResponse["data"].([][]any)
		require.Len(t, rows, 1)
		assert.Equal(t, []string{ids[2], ids[0]}, rows[0][12])
		assert.EqualValues(t, 1, rows[0][4])
		columns := response.RawResponse["columns"].(map[string]any)
		assert.Contains(t, columns, "user_ids")
		assert.NotContains(t, columns, "user")
	}
	response := handler.HandleRaw(ctx, funcapi.RawMethodRequest{
		Method: "rum-sessions",
		Args:   []string{"site:shop", "user_id:123"},
	})
	require.NotNil(t, response.RawResponse)
	assert.Empty(t, response.RawResponse["data"])
}
