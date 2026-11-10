// SPDX-License-Identifier: GPL-3.0-or-later

package history

import (
	"context"
	"testing"
	"time"

	"github.com/netdata/systemd-journal-sdk/go/journal"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestOptionalStoredSiteAssociation(t *testing.T) {
	ctx := context.Background()
	store, _ := newTestStore(t)
	now := time.Now().UnixMicro()
	for _, group := range []string{"", "11111111-1111-4111-8111-111111111111", "22222222-2222-4222-8222-222222222222"} {
		appendEvent(t, store, EventRecord{
			Site:        "job",
			VnodeGUID:   group,
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
	assert.Equal(t, "", all[0].VnodeGUID, "old records lacking DEM_VNODE_GUID stay unassigned")
	selected, err := store.QueryErrors(ctx, "job", "same", 0, now/1e6+1, "11111111-1111-4111-8111-111111111111")
	require.NoError(t, err)
	require.Len(t, selected, 1)
	assert.Equal(t, "11111111-1111-4111-8111-111111111111", selected[0].VnodeGUID)
	wrong, err := store.QuerySessionEvents(ctx, "another-job", "same", "11111111-1111-4111-8111-111111111111")
	require.NoError(t, err)
	assert.Empty(t, wrong)
	attempted, err := store.AppendEvent(ctx, EventRecord{
		Site:      "job",
		VnodeGUID: "Invalid",
		Type:      "error",
	})
	assert.False(t, attempted)
	require.ErrorContains(t, err, "vnode_guid")
	fields := eventFields(EventRecord{
		Site: "job",
		Type: "error",
	})
	for _, field := range fields {
		assert.NotEqual(t, "DEM_VNODE_GUID", field.Name, "unassigned writes omit the optional field")
	}
}

func TestStoredSiteAssociationTransitionsRemainIndependent(t *testing.T) {
	ctx := context.Background()
	store, _ := newTestStore(t)
	now := time.Now().UnixMicro()
	groups := []string{"11111111-1111-4111-8111-111111111111", "11111111-1111-4111-8111-111111111111", "", "22222222-2222-4222-8222-222222222222", "22222222-2222-4222-8222-222222222222", "", "11111111-1111-4111-8111-111111111111"}
	for i, group := range groups {
		appendEvent(t, store, EventRecord{Site: "job", VnodeGUID: group, SessionID: "session", Type: "event", ObservedUS: now + int64(i)})
	}
	require.NoError(t, store.Sync(ctx))
	events, err := store.QuerySessionEvents(ctx, "job", "session", "")
	require.NoError(t, err)
	require.Len(t, events, len(groups))
	for i, event := range events {
		assert.Equal(t, groups[i], event.VnodeGUID, "each persisted event retains its own association")
	}
	var scanned []EventRecord
	require.NoError(t, store.scan(ctx, "job", now/1e6, now/1e6+1, "", func(event EventRecord) { scanned = append(scanned, event) }))
	require.Len(t, scanned, len(groups))
	for i, event := range scanned {
		assert.Equal(t, groups[i], event.VnodeGUID, "missing fields must not inherit the previous association")
	}
}

func TestLegacySiteTextNeverBecomesVnodeIdentity(t *testing.T) {
	ctx := context.Background()
	store, _ := newTestStore(t)
	fields := eventFields(EventRecord{Site: "job", SessionID: "session", Type: "event", ObservedUS: time.Now().UnixMicro()})
	// Even UUID-looking legacy site text was not a captured vnode association.
	fields = append(fields, journal.StringField("DEM_SITE_ID", "11111111-1111-4111-8111-111111111111"))
	attempted, err := store.journal.Append(ctx, fields)
	require.True(t, attempted)
	require.NoError(t, err)
	require.NoError(t, store.Sync(ctx))
	rows, err := store.QuerySessionEvents(ctx, "job", "session", "")
	require.NoError(t, err)
	require.Len(t, rows, 1)
	assert.Empty(t, rows[0].VnodeGUID)
	selected, err := store.QuerySessionEvents(ctx, "job", "session", "11111111-1111-4111-8111-111111111111")
	require.NoError(t, err)
	assert.Empty(t, selected)
}
