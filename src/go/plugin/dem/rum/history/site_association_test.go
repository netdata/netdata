// SPDX-License-Identifier: GPL-3.0-or-later

package history

import (
	"context"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestOptionalStoredSiteAssociation(t *testing.T) {
	ctx := context.Background()
	store, _ := newTestStore(t)
	now := time.Now().UnixMicro()
	for _, group := range []string{"", "storefront", "checkout"} {
		appendEvent(t, store, EventRecord{
			Site:        "job",
			SiteID:      group,
			SessionID:   "same",
			Type:        "error",
			Fingerprint: "same",
			ObservedUS:  now,
		})
	}
	require.NoError(t, store.Sync(ctx))
	all, err := store.QuerySessions(ctx, "", "", 0, now/1e6+1, 2000, "")
	require.NoError(t, err)
	require.Len(t, all, 3)
	assert.Equal(t, "", all[0].SiteID, "old records lacking DEM_SITE_ID stay unassigned")
	selected, err := store.QueryErrors(ctx, "job", "same", 0, now/1e6+1, "storefront")
	require.NoError(t, err)
	require.Len(t, selected, 1)
	assert.Equal(t, "storefront", selected[0].SiteID)
	wrong, err := store.QuerySessionEvents(ctx, "another-job", "same", "storefront")
	require.NoError(t, err)
	assert.Empty(t, wrong)
	attempted, err := store.AppendEvent(ctx, EventRecord{
		Site:   "job",
		SiteID: "Invalid",
		Type:   "error",
	})
	assert.False(t, attempted)
	require.ErrorContains(t, err, "site_id")
	fields := eventFields(EventRecord{
		Site: "job",
		Type: "error",
	})
	for _, field := range fields {
		assert.NotEqual(t, "DEM_SITE_ID", field.Name, "unassigned writes omit the optional field")
	}
}

func TestStoredSiteAssociationTransitionsRemainIndependent(t *testing.T) {
	ctx := context.Background()
	store, _ := newTestStore(t)
	now := time.Now().UnixMicro()
	groups := []string{"storefront", "storefront", "", "checkout", "checkout", "", "storefront"}
	for i, group := range groups {
		appendEvent(t, store, EventRecord{Site: "job", SiteID: group, SessionID: "session", Type: "event", ObservedUS: now + int64(i)})
	}
	require.NoError(t, store.Sync(ctx))
	events, err := store.QuerySessionEvents(ctx, "job", "session", "")
	require.NoError(t, err)
	require.Len(t, events, len(groups))
	for i, event := range events {
		assert.Equal(t, groups[i], event.SiteID, "each persisted event retains its own association")
	}
	var scanned []EventRecord
	require.NoError(t, store.scan(ctx, "job", now/1e6, now/1e6+1, "", func(event EventRecord) { scanned = append(scanned, event) }))
	require.Len(t, scanned, len(groups))
	for i, event := range scanned {
		assert.Equal(t, groups[i], event.SiteID, "missing fields must not inherit the previous association")
	}
}
