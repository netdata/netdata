// SPDX-License-Identifier: GPL-3.0-or-later
package history

import (
	"context"
	"math"
	"testing"

	"github.com/netdata/systemd-journal-sdk/go/journal"
	"github.com/stretchr/testify/require"
)

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
	rows, err := s.QuerySessions(ctx, "shop", "", 1, 1, 1)
	require.NoError(t, err)
	require.Len(t, rows, 1)
	require.Equal(t, "z", rows[0].SessionID, "microsecond ordering precedes limit and identity tie breaks")
	require.Equal(t, int64(1_999_999), rows[0].LastObservedUS)
	require.EqualValues(t, 2, rows[0].Pageviews)
	require.Equal(t, "/first", rows[0].EntryPage)
	require.Equal(t, "/last", rows[0].LastPage)
	rows, err = s.QuerySessions(ctx, "shop", "match", 1, 1, 1)
	require.NoError(t, err)
	require.Len(t, rows, 1)
	require.EqualValues(t, 2, rows[0].Pageviews)
	rows, err = s.QuerySessions(ctx, "shop", "", 0, 0, 0)
	require.NoError(t, err)
	require.Len(t, rows, 2)
	require.Equal(t, "epoch", rows[1].SessionID)
	timeline, err := s.QuerySessionEvents(ctx, "shop", "a")
	require.NoError(t, err)
	require.Len(t, timeline, 3)
	require.Equal(t, int64(999_999), timeline[0].ObservedUS)
	require.Equal(t, int64(2_000_001), timeline[2].ObservedUS)
	timeline, err = s.QuerySessionEvents(ctx, "shop", "epoch")
	require.NoError(t, err)
	require.Empty(t, timeline, "activity remains summary-only")
	rows, err = s.QuerySessions(ctx, "shop", "", math.MaxInt64, math.MaxInt64, 0)
	require.NoError(t, err)
	require.Empty(t, rows, "extreme seconds must not overflow microseconds")
	for _, bounds := range [][2]int64{{-1, 1}, {2, 1}} {
		_, err = s.QuerySessions(ctx, "shop", "", bounds[0], bounds[1], 0)
		require.Error(t, err)
	}
}

func TestObservedErrorsSelectOccurrenceAndDetails(t *testing.T) {
	s, _ := newTestStore(t)
	ctx := context.Background()
	for _, r := range []EventRecord{
		{Site: "shop", SessionID: "outside", Type: "error", ObservedUS: 999_999, Fingerprint: "fp", Page: "/outside", Browser: "Outside"},
		{Site: "shop", SessionID: "one", Type: "error", ObservedUS: 1_000_000, Fingerprint: "fp", Page: "/selected", Browser: "Browser"},
		{Site: "shop", SessionID: "two", Type: "error", ObservedUS: 1_999_999, Fingerprint: "fp", Page: "/selected", Browser: "Browser"},
		{Site: "shop", SessionID: "outside", Type: "error", ObservedUS: 2_000_000, Fingerprint: "fp", Page: "/outside", Browser: "Outside"},
	} {
		appendEvent(t, s, r)
	}
	rows, err := s.QueryErrors(ctx, "shop", "", 1, 1)
	require.NoError(t, err)
	require.Len(t, rows, 1)
	require.Equal(t, 2, rows[0].CountWindow)
	require.False(t, rows[0].Details)
	rows, err = s.QueryErrors(ctx, "shop", "fp", 1, 1)
	require.NoError(t, err)
	require.Len(t, rows, 1)
	require.Equal(t, 2, rows[0].CountWindow)
	require.Equal(t, 2, rows[0].SessionsAffected)
	require.Equal(t, "/selected", rows[0].TopPage)
	require.Equal(t, []string{"Browser"}, rows[0].Browsers)
}

func TestInvalidDomainRecordFailsBeforeAppend(t *testing.T) {
	s, _ := newTestStore(t)
	for _, r := range []EventRecord{{Type: "error"}, {Site: "shop"}, {Site: "shop", Type: "error", ObservedUS: -1}} {
		attempted, err := s.AppendEvent(context.Background(), r)
		require.Error(t, err)
		require.False(t, attempted)
	}
}

func TestSelectedDomainScalarsAreValidated(t *testing.T) {
	for _, tc := range []struct {
		name   string
		mutate func([]journal.Field) []journal.Field
	}{
		{"duplicate session", func(f []journal.Field) []journal.Field {
			return append(f, journal.StringField("DEM_SESSION_ID", "other"))
		}},
		{"duplicate site", func(f []journal.Field) []journal.Field { return append(f, journal.StringField("DEM_SITE", "other")) }},
		{"duplicate optional", func(f []journal.Field) []journal.Field { return append(f, journal.StringField("DEM_PAGE", "/other")) }},
		{"duplicate revision", func(f []journal.Field) []journal.Field { return append(f, journal.StringField("DEM_REVISION", "2")) }},
		{"missing site", func(f []journal.Field) []journal.Field { return omitField(f, "DEM_SITE") }},
		{"missing type", func(f []journal.Field) []journal.Field { return omitField(f, "DEM_TYPE") }},
		{"missing revision", func(f []journal.Field) []journal.Field { return omitField(f, "DEM_REVISION") }},
		{"noncanonical revision", func(f []journal.Field) []journal.Field {
			return append(omitField(f, "DEM_REVISION"), journal.StringField("DEM_REVISION", "01"))
		}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			s, _ := newTestStore(t)
			fields := eventFields(EventRecord{Site: "shop", SessionID: "session", Type: "pageview", ObservedUS: 1_000_000, Page: "/page"})
			_, err := s.journal.Append(context.Background(), tc.mutate(fields))
			require.NoError(t, err)
			rows, err := s.QuerySessions(context.Background(), "shop", "", 1, 1, 0)
			require.Error(t, err)
			require.Nil(t, rows, "malformed candidates cannot return partial aggregates")
		})
	}
}

func omitField(fields []journal.Field, name string) []journal.Field {
	var out []journal.Field
	for _, f := range fields {
		if string(f.Name) != name {
			out = append(out, f)
		}
	}
	return out
}
