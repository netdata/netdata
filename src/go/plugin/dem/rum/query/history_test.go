// SPDX-License-Identifier: GPL-3.0-or-later

package query_test

import (
	"context"
	"testing"
	"time"

	"github.com/netdata/netdata/go/plugins/plugin/dem/journalstore"
	"github.com/netdata/netdata/go/plugins/plugin/dem/rum/history"
	"github.com/netdata/netdata/go/plugins/plugin/dem/rum/query"
	rumregistry "github.com/netdata/netdata/go/plugins/plugin/dem/rum/registry"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// The history store reports cancellation before reading; the query service
// must return that error rather than an empty result.
func TestSessionsReturnHistoryCancellation(t *testing.T) {
	journalStore, err := journalstore.Open(context.Background(), "")
	require.NoError(t, err)
	t.Cleanup(func() { require.NoError(t, journalStore.Close()) })
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	_, err = query.New(rumregistry.New(), history.NewStore(journalStore)).Sessions(ctx, "", "", 0, time.Now().Unix(), 2000, "")
	assert.ErrorIs(t, err, context.Canceled)
}

// History of a site that is no longer configured gets the credential patterns
// at output. Records written before ingestion redacted the cleaned text, or
// under an older pattern set, can still hold a credential.
func TestRetiredSiteHistoryGetsCredentialPatterns(t *testing.T) {
	ctx := context.Background()
	journalStore, err := journalstore.Open(ctx, "")
	require.NoError(t, err)
	t.Cleanup(func() { require.NoError(t, journalStore.Close()) })
	store := history.NewStore(journalStore)
	now := time.Now()
	_, err = store.AppendEvent(ctx, history.EventRecord{
		Site:        "shop",
		SessionID:   "session",
		ObservedUS:  now.UnixMicro(),
		Type:        "error",
		Page:        "/checkout",
		Text:        "Error: token=hunter2",
		Browser:     "Chrome",
		Version:     "token=hunter2",
		UserID:      "Bearer abcdef",
		View:        "token=hunter2",
		Fingerprint: "fingerprint",
		ErrorType:   "Error",
		Message:     "token=hunter2",
		SampleStack: "Bearer abcdef (app.js:1:2)\n",
	})
	require.NoError(t, err)
	service := query.New(rumregistry.New(), store)

	events, err := service.SessionEvents(ctx, "shop", "session", "")
	require.NoError(t, err)
	require.Len(t, events, 1)
	assert.Equal(t, "Error: token=[REDACTED]", events[0].Text)
	assert.Equal(t, "[REDACTED]", events[0].UserID)
	assert.Equal(t, "token=[REDACTED]", events[0].View)

	errors, err := service.Errors(ctx, "shop", "fingerprint", 0, now.Unix()+1, "")
	require.NoError(t, err)
	require.Len(t, errors, 1)
	assert.Equal(t, "token=[REDACTED]", errors[0].Message)
	assert.Equal(t, "[REDACTED] (app.js:1:2)\n", errors[0].SampleStack)

	sessions, err := service.Sessions(ctx, "shop", "", 0, now.Unix()+1, 2000, "")
	require.NoError(t, err)
	require.Len(t, sessions, 1)
	assert.Equal(t, []string{"[REDACTED]"}, sessions[0].UserIDs)
	assert.Equal(t, "token=[REDACTED]", sessions[0].Version)
}
