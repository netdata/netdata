// SPDX-License-Identifier: GPL-3.0-or-later

package history

import (
	"context"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestSelfContainedErrorsAndSelectedDetails(t *testing.T) {
	s, _ := newTestStore(t)
	ctx := context.Background()
	now := time.Now()
	records := []EventRecord{
		{
			Site:        "shop",
			ObservedUS:  now.Add(-time.Minute).UnixMicro(),
			Type:        "error",
			Fingerprint: "fp",
			ErrorType:   "TypeError",
			Message:     "parentless",
			SampleStack: "stack",
			Page:        "/b",
			Browser:     "z",
		},
		{
			Site:        "shop",
			SessionID:   "one",
			ObservedUS:  now.UnixMicro(),
			Type:        "error",
			Fingerprint: "fp",
			ErrorType:   "TypeError",
			Message:     "parentless",
			Page:        "/a",
			Browser:     "a",
		},
		{
			Site:        "shop",
			SessionID:   "one",
			ObservedUS:  now.UnixMicro(),
			Type:        "error",
			Fingerprint: "fp",
			Page:        "/a",
			Browser:     "z",
		},
		{
			Site:        "shop",
			SessionID:   "two",
			ObservedUS:  now.UnixMicro(),
			Type:        "error",
			Fingerprint: "other",
			Page:        "/other",
		},
	}
	for _, r := range records {
		appendEvent(t, s, r)
	}
	groups, err := s.QueryErrors(ctx, "shop", "", now.Unix()-61, now.Unix()+5, "")
	require.NoError(t, err)
	require.Len(t, groups, 2)
	assert.Equal(t, 3, groups[0].CountWindow)
	assert.False(t, groups[0].Details)
	assert.Zero(t, groups[0].SessionsAffected)
	assert.Empty(t, groups[0].TopPage)
	assert.Nil(t, groups[0].Browsers)
	assert.Equal(t, "parentless", groups[0].Message)
	assert.Equal(t, "stack", groups[0].SampleStack)
	assert.Equal(t, now.Add(-time.Minute).Unix(), groups[0].FirstSeen)
	groups, err = s.QueryErrors(ctx, "shop", "fp", now.Unix()-61, now.Unix()+5, "")
	require.NoError(t, err)
	require.Len(t, groups, 1)
	assert.True(t, groups[0].Details)
	assert.Equal(t, 1, groups[0].SessionsAffected)
	assert.Equal(t, "/a", groups[0].TopPage)
	assert.Equal(t, []string{"z", "a"}, groups[0].Browsers)
	groups, err = s.QueryErrors(ctx, "shop", "missing", 0, now.Unix()+5, "")
	require.NoError(t, err)
	assert.Empty(t, groups)
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
	rows, err := s.QueryErrors(ctx, "shop", "", 1, 1, "")
	require.NoError(t, err)
	require.Len(t, rows, 1)
	require.Equal(t, 2, rows[0].CountWindow)
	require.False(t, rows[0].Details)
	rows, err = s.QueryErrors(ctx, "shop", "fp", 1, 1, "")
	require.NoError(t, err)
	require.Len(t, rows, 1)
	require.Equal(t, 2, rows[0].CountWindow)
	require.Equal(t, 2, rows[0].SessionsAffected)
	require.Equal(t, "/selected", rows[0].TopPage)
	require.Equal(t, []string{"Browser"}, rows[0].Browsers)
}
