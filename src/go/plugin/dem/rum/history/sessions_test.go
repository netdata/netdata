// SPDX-License-Identifier: GPL-3.0-or-later

package history

import (
	"context"
	"math"
	"testing"
	"time"

	demjournal "github.com/netdata/netdata/go/plugins/plugin/dem/journal"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestReceiptTimeFiltersAndSelectedSummaries(t *testing.T) {
	s, _ := newTestStore(t)
	ctx := context.Background()
	saved := time.Now()
	old := saved.Add(-10 * time.Minute)
	records := []EventRecord{
		{
			Site:       "shop",
			SessionID:  "s",
			ObservedUS: old.Add(time.Minute).UnixMicro(),
			Type:       "error",
			Page:       "/last",
			Browser:    "latest",
			UserID:     "u",
		},
		{Site: "shop", SessionID: "s", ObservedUS: old.UnixMicro(), Type: "pageview", Page: "/first", Browser: "early"},
		{Site: "shop", SessionID: "s", ObservedUS: old.UnixMicro(), Type: "pageview", Page: "/first", Browser: "early"},
		{Site: "shop", SessionID: "s", ObservedUS: old.Add(time.Second).UnixMicro(), Type: "frustration", Page: "/first"},
		{Site: "shop", SessionID: "activity", ObservedUS: old.UnixMicro(), Type: "activity", Page: "/idle"},
		{Site: "other", SessionID: "s", ObservedUS: old.UnixMicro(), Type: "pageview"},
	}
	for _, r := range records {
		appendEvent(t, s, r)
	}
	sessions, err := s.QuerySessions(ctx, "shop", "", old.Unix()-1, old.Unix()+100, 0, "")
	require.NoError(t, err)
	require.Len(t, sessions, 2)
	got := sessions[0]
	assert.Equal(t, "s", got.SessionID)
	assert.Equal(t, int64(2), got.Pageviews)
	assert.Equal(t, int64(1), got.Errors)
	assert.Equal(t, int64(1), got.Frustrations)
	assert.Equal(t, old.Unix(), got.StartedAt)
	assert.Equal(t, old.Add(time.Minute).Unix(), got.LastAt)
	assert.Equal(t, "/first", got.EntryPage)
	assert.Equal(t, "/last", got.LastPage)
	assert.Equal(t, "latest", got.Browser)
	assert.Equal(t, []string{"u"}, got.UserIDs)
	events, err := s.QuerySessionEvents(ctx, "shop", "s", "")
	require.NoError(t, err)
	require.Len(t, events, 4)
	assert.Equal(t, "pageview", events[0].Type)
	assert.Equal(t, events[0], events[1], "identical records are separate immutable events")
	assert.Equal(t, "error", events[3].Type)
	events, err = s.QuerySessionEvents(ctx, "shop", "activity", "")
	require.NoError(t, err)
	assert.Empty(t, events)
	sessions, err = s.QuerySessions(ctx, "shop", "", saved.Unix()-1, saved.Unix()+5, 0, "")
	require.NoError(t, err)
	assert.Empty(t, sessions, "save time must not change receipt-time membership")
	sessions, err = s.QuerySessions(ctx, "", "", 0, saved.Unix()+5, 1, "")
	require.NoError(t, err)
	assert.Len(t, sessions, 1)
}

func TestObservedRangeWholeSecondsAndOrdering(t *testing.T) {
	s, _ := newTestStore(t)
	ctx := context.Background()
	// Deliberately append out of receipt-time order; all rows are saved much later.
	for _, r := range []EventRecord{
		{Site: "shop", SessionID: "a", Type: "pageview", ObservedUS: 2_000_001, Page: "/outside"},
		{Site: "shop", SessionID: "z", Type: "pageview", ObservedUS: 1_999_999, Page: "/last"},
		{Site: "shop", SessionID: "a", Type: "pageview", ObservedUS: 1_000_001, Page: "/early"},
		{Site: "shop", SessionID: "z", Type: "pageview", ObservedUS: 1_000_000, Page: "/first", UserID: "match"},
		{Site: "shop", SessionID: "a", Type: "pageview", ObservedUS: 999_999, Page: "/outside"},
		{Site: "other", SessionID: "z", Type: "pageview", ObservedUS: 1_999_999},
		{Site: "shop", SessionID: "epoch", Type: "activity", ObservedUS: 0},
	} {
		appendEvent(t, s, r)
	}
	rows, err := s.QuerySessions(ctx, "shop", "", 1, 1, 1, "")
	require.NoError(t, err)
	require.Len(t, rows, 1)
	require.Equal(t, "z", rows[0].SessionID, "microsecond ordering precedes limit and identity tie breaks")
	require.Equal(t, int64(1_999_999), rows[0].LastObservedUS)
	require.EqualValues(t, 2, rows[0].Pageviews)
	require.Equal(t, "/first", rows[0].EntryPage)
	require.Equal(t, "/last", rows[0].LastPage)
	rows, err = s.QuerySessions(ctx, "shop", "match", 1, 1, 1, "")
	require.NoError(t, err)
	require.Len(t, rows, 1)
	require.EqualValues(t, 2, rows[0].Pageviews)
	rows, err = s.QuerySessions(ctx, "shop", "", 0, 0, 0, "")
	require.NoError(t, err)
	require.Len(t, rows, 2)
	require.Equal(t, "epoch", rows[1].SessionID)
	timeline, err := s.QuerySessionEvents(ctx, "shop", "a", "")
	require.NoError(t, err)
	require.Len(t, timeline, 3)
	require.Equal(t, int64(999_999), timeline[0].ObservedUS)
	require.Equal(t, int64(2_000_001), timeline[2].ObservedUS)
	timeline, err = s.QuerySessionEvents(ctx, "shop", "epoch", "")
	require.NoError(t, err)
	require.Empty(t, timeline, "activity remains summary-only")
	rows, err = s.QuerySessions(ctx, "shop", "", math.MaxInt64, math.MaxInt64, 0, "")
	require.NoError(t, err)
	require.Empty(t, rows, "extreme seconds must not overflow microseconds")
	for _, bounds := range [][2]int64{{-1, 1}, {2, 1}} {
		_, err = s.QuerySessions(ctx, "shop", "", bounds[0], bounds[1], 0, "")
		require.Error(t, err)
	}
}

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
		rows, err := s.QuerySessions(ctx, "shop", id, 0, now.Unix()+10, 1, "")
		require.NoError(t, err)
		require.Len(t, rows, 1)
		assert.Equal(t, "shared", rows[0].SessionID)
		assert.EqualValues(t, 5, rows[0].Pageviews, "all session events count, including anonymous and other IDs")
		assert.Equal(t, []string{ids[0], ids[3], ids[2]}, rows[0].UserIDs)
	}
	rows, err := s.QuerySessions(ctx, "shop", "123", 0, now.Unix()+10, 1, "")
	require.NoError(t, err)
	assert.Empty(t, rows, "lookup is exact, not substring")
	events, err := s.QuerySessionEvents(ctx, "shop", "shared", "")
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
	rows, err := s.QuerySessions(ctx, "shop", "prior-user", now.Unix()-5, now.Unix()+5, 0, "")
	require.NoError(t, err)
	assert.Empty(t, rows, "an ID outside the selected receipt-time range must not match")
	rows, err = s.QuerySessions(ctx, "shop", "", now.Unix()-5, now.Unix()+5, 0, "")
	require.NoError(t, err)
	require.Len(t, rows, 1)
	assert.Equal(t, []string{}, rows[0].UserIDs)
	assert.EqualValues(t, 1, rows[0].Pageviews)
	rows, err = s.QuerySessions(ctx, "shop", "prior-user", old.Unix()-5, now.Unix()+5, 0, "")
	require.NoError(t, err)
	require.Len(t, rows, 1)
	assert.Equal(t, []string{"prior-user"}, rows[0].UserIDs)
	assert.EqualValues(t, 2, rows[0].Pageviews)
}
