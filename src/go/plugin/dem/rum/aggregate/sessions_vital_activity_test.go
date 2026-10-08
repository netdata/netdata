// SPDX-License-Identifier: GPL-3.0-or-later

package aggregate

import (
	"testing"
	"time"

	"github.com/netdata/netdata/go/plugins/plugin/dem/rum/beacon"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestHistoryRecordsAcceptedGoodVitalActivity(t *testing.T) {
	a, now := newAgg(time.Minute)
	sink := &fakeHistorySink{}
	a.SetHistorySink(sink)
	b := mk(*now, "session", "/entry")
	a.Ingest(b)
	require.Len(t, sink.events, 1)

	*now = now.Add(10 * time.Second)
	b.Received = *now
	b.Events = nil
	b.Vitals = []beacon.Vital{v(beacon.LCP, 100)}
	result := a.Ingest(b)
	require.Len(t, result.Observation.Vitals, 1)
	require.Len(t, sink.events, 2)
	activity := sink.events[1]
	assert.Equal(t, "activity", activity.Type)
	assert.Equal(t, now.UnixMicro(), activity.ObservedUS)
	assert.Equal(t, b.SessionID, activity.SessionID)
	assert.Equal(t, b.ExperienceID, activity.ExperienceID)
	assert.Equal(t, "/entry", activity.Page)

	*now = now.Add(10 * time.Second)
	b.Received = *now
	result = a.Ingest(b)
	assert.Empty(t, result.Observation.Vitals)
	assert.Len(t, sink.events, 2, "replayed vital does not write fresh activity")
	timeline, ok := a.SessionEvents(b.SessionID)
	require.True(t, ok)
	require.Len(t, timeline, 1)
	assert.Equal(t, "pageview", timeline[0].Type)
}
