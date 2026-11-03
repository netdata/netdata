// SPDX-License-Identifier: GPL-3.0-or-later
package aggregate

import (
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

func TestActivityRetainsReceiptEvidenceAfterQuietWindow(t *testing.T) {
	a, now := newAgg(5 * time.Minute)
	require.True(t, a.Activity().LastBeaconAt.IsZero())
	require.Equal(t, -1, a.Activity().LastBeaconAgeS)
	a.Ingest(mk(*now, "session", "/"))
	accepted := *now
	*now = now.Add(2 * time.Hour)
	activity := a.Activity()
	require.Equal(t, accepted, activity.LastBeaconAt)
	require.Equal(t, 7200, activity.LastBeaconAgeS)
	require.Zero(t, activity.BeaconsPerMin)
	replacement, _ := newAgg(5 * time.Minute)
	require.True(t, replacement.Activity().LastBeaconAt.IsZero())
}
