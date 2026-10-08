// SPDX-License-Identifier: GPL-3.0-or-later

package history

import (
	"context"
	"testing"
	"time"

	demjournal "github.com/netdata/netdata/go/plugins/plugin/dem/journal"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestSessionIdentityMembershipPrecedesLimitAndPreservesWholeSummary(t *testing.T) {
	s, _ := newTestStore(t)
	ctx := context.Background()
	now := time.Now()
	// IDs that resemble paths, numeric keys, and UUIDs retain their application meaning.
	ids := []string{"/accounts/123456", "", "550e8400-e29b-41d4-a716-446655440000", "123456", "123456"}
	for i, id := range ids {
		appendEvent(t, s, EventRecord{
			Site:       "shop",
			SessionID:  "shared",
			ObservedUS: now.Add(time.Duration(i) * time.Second).UnixMicro(),
			Type:       "pageview",
			Page:       "/page",
			UserID:     id,
		})
	}
	appendEvent(t, s, EventRecord{
		Site:       "shop",
		SessionID:  "newer",
		ObservedUS: now.Add(time.Minute).UnixMicro(),
		Type:       "error",
		UserID:     "other",
	})
	for _, id := range []string{ids[0], ids[2], ids[3]} {
		rows, err := s.QuerySessions(ctx, "shop", id, 0, now.Unix()+10, 1)
		require.NoError(t, err)
		require.Len(t, rows, 1)
		assert.Equal(t, "shared", rows[0].SessionID)
		assert.EqualValues(t, 5, rows[0].Pageviews, "all session events count, including anonymous and other IDs")
		assert.Equal(t, []string{ids[0], ids[3], ids[2]}, rows[0].UserIDs)
	}
	rows, err := s.QuerySessions(ctx, "shop", "123", 0, now.Unix()+10, 1)
	require.NoError(t, err)
	assert.Empty(t, rows, "lookup is exact, not substring")
	events, err := s.QuerySessionEvents(ctx, "shop", "shared")
	require.NoError(t, err)
	require.Len(t, events, len(ids))
	for i, id := range ids {
		assert.Equal(t, id, events[i].UserID)
	}
}

func TestSessionUserIDsOnlyIncludeReceiptTimeRange(t *testing.T) {
	ctx := context.Background()
	root := t.TempDir()
	now := time.Now()
	old := now.Add(-time.Hour)
	// Delayed persistence must not bring an older observation into the selected range.
	seedOldJournal(t, root, EventRecord{
		Site:       "shop",
		SessionID:  "shared",
		ObservedUS: old.UnixMicro(),
		Type:       "pageview",
		Page:       "/old",
		UserID:     "prior-user",
	}, now)
	owner, err := demjournal.Open(ctx, root)
	require.NoError(t, err)
	t.Cleanup(func() { require.NoError(t, owner.Close()) })
	s := NewStore(owner)
	appendEvent(t, s, EventRecord{
		Site:       "shop",
		SessionID:  "shared",
		ObservedUS: now.UnixMicro(),
		Type:       "pageview",
		Page:       "/anonymous",
	})
	rows, err := s.QuerySessions(ctx, "shop", "prior-user", now.Unix()-5, now.Unix()+5, 0)
	require.NoError(t, err)
	assert.Empty(t, rows, "an ID outside the selected receipt-time range must not match")
	rows, err = s.QuerySessions(ctx, "shop", "", now.Unix()-5, now.Unix()+5, 0)
	require.NoError(t, err)
	require.Len(t, rows, 1)
	assert.Equal(t, []string{}, rows[0].UserIDs)
	assert.EqualValues(t, 1, rows[0].Pageviews)
	rows, err = s.QuerySessions(ctx, "shop", "prior-user", old.Unix()-5, now.Unix()+5, 0)
	require.NoError(t, err)
	require.Len(t, rows, 1)
	assert.Equal(t, []string{"prior-user"}, rows[0].UserIDs)
	assert.EqualValues(t, 2, rows[0].Pageviews)
}
