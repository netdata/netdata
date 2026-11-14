// SPDX-License-Identifier: GPL-3.0-or-later

package query_test

import (
	"context"
	"testing"
	"time"

	"github.com/netdata/netdata/go/plugins/pkg/funcapi"
	"github.com/netdata/netdata/go/plugins/plugin/dem/journal"
	"github.com/netdata/netdata/go/plugins/plugin/dem/rum/beacon"
	"github.com/netdata/netdata/go/plugins/plugin/dem/rum/history"
	"github.com/netdata/netdata/go/plugins/plugin/dem/rum/query"
	rumregistry "github.com/netdata/netdata/go/plugins/plugin/dem/rum/registry"
	"github.com/netdata/netdata/go/plugins/plugin/dem/rum/rumfunc"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// The history store reports cancellation before reading; the query service
// must return that error rather than an empty result.
func TestSessionsReturnHistoryCancellation(t *testing.T) {
	journalStore, err := journal.Open(context.Background(), "")
	require.NoError(t, err)
	t.Cleanup(func() { require.NoError(t, journalStore.Close()) })
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	_, err = query.New(rumregistry.New(), history.NewStore(journalStore)).Sessions(ctx, "", "", 0, time.Now().Unix(), 2000, "")
	assert.ErrorIs(t, err, context.Canceled)
}

func TestSessionEventsMergePendingAndPersistedWithoutDuplicates(t *testing.T) {
	ctx := context.Background()
	journalStore, err := journal.Open(ctx, "")
	require.NoError(t, err)
	t.Cleanup(func() { require.NoError(t, journalStore.Close()) })
	st := history.NewStore(journalStore)
	hub := rumregistry.New()
	a, retire := addSite(t, hub, "site", "first")
	src := query.New(hub, st)
	handler := rumfunc.New(src, nil)
	request := func() [][]any {
		t.Helper()
		response := handler.HandleRaw(
			ctx,
			funcapi.RawMethodRequest{
				Method:      "rum-session-events",
				Args:        []string{"site:site", "session_id:session"},
				Permissions: "0x1b",
			},
		)
		require.NotNil(t, response.RawResponse, "status=%d message=%s", response.Status, response.Message)
		return response.RawResponse["data"].([][]any)
	}
	flush := func(w *history.Writer) {
		runCtx, cancel := context.WithCancel(ctx)
		cancel()
		w.Run(runCtx)
	}
	first := history.NewWriter(st, discardCounters{})
	a.SetHistorySink(first)
	a.Ingest(
		&beacon.Beacon{
			Site:         "site",
			SessionID:    "session",
			PageGroup:    "/first",
			ExperienceID: "first",
			Events: []beacon.Event{
				{Kind: beacon.EventDocument, ID: "first", Revision: 1},
				{Name: "[REDACTED]"},
				{Name: "[REDACTED]"},
			},
		},
	)
	pending := request()
	require.Len(t, pending, 3)
	assert.Equal(t, "[REDACTED]", pending[1][3])
	assert.Equal(t, pending[1], pending[2], "identical repeated events are real occurrences")
	flush(first)
	assert.Equal(t, pending, request(), "flushing must not duplicate the live ring")
	second := history.NewWriter(st, discardCounters{})
	a.SetHistorySink(second)
	a.Ingest(
		&beacon.Beacon{
			Site:      "site",
			SessionID: "session",
			PageGroup: "/second",
			Events:    []beacon.Event{{Name: "purchase"}},
		},
	)
	merged := request()
	require.Len(t, merged, 4)
	assert.Equal(t, pending, merged[:3])
	assert.Equal(t, []any{"event", "/second", "purchase", "", "", "", "", "", "", uint64(0), ""}, merged[3][1:])
	flush(second)
	assert.Equal(t, merged, request())
	retire()
	assert.Equal(t, merged, request(), "retired site history remains available")
	replacement, _ := addSite(t, hub, "site", "replacement")
	replacement.Ingest(
		&beacon.Beacon{
			Site:         "site",
			SessionID:    "session",
			PageGroup:    "/reloaded",
			ExperienceID: "reloaded",
			Events:       []beacon.Event{{Kind: beacon.EventDocument, ID: "reloaded", Revision: 1}, {Name: "reloaded"}},
		},
	)
	reloaded := request()
	require.Len(t, reloaded, 6)
	assert.Equal(t, merged, reloaded[:4], "new site generation must retain the persisted timeline")
	assert.Equal(t, "reloaded", reloaded[5][3])
}
