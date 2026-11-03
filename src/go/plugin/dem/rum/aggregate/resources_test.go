// SPDX-License-Identifier: GPL-3.0-or-later
package aggregate

import (
	"fmt"
	"testing"
	"time"

	"github.com/netdata/netdata/go/plugins/plugin/dem/rum/beacon"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestResourceEligibilityZeroAndUnknownOwnership(t *testing.T) {
	a, now := newAgg(time.Minute)
	b := mk(*now, "s", "/")
	b.PageHost = "www.example.com"
	b.Resources = []beacon.Resource{
		{ID: "self", Self: true, Host: "www.example.com", HasDuration: true, DurationMS: 1},
		{ID: "zero", Host: "api.example.com", HasDuration: true, DurationMS: 0, Initiator: "fetch"},
		{ID: "missing", Host: "api.example.com", DurationMS: 99, Initiator: "fetch"},
		{ID: "third", Host: "example.net", HasDuration: true, DurationMS: 20},
		{ID: "unknown", HasDuration: true, DurationMS: 2},
	}
	a.Ingest(b)
	a.Ingest(b)
	s := a.Snapshot()
	assert.EqualValues(t, 2, s.FirstPartyResources)
	assert.EqualValues(t, 1, s.ThirdPartyResources)
	assert.EqualValues(t, 1, s.UnknownResources)
	assert.Equal(t, 1, s.API.N)
	assert.Zero(t, s.API.P75)
	for _, g := range s.ResourceHosts {
		if g.Host == "api.example.com" {
			assert.EqualValues(t, 2, g.Count)
			assert.Equal(t, 1, g.Duration.N)
			assert.Zero(t, g.Duration.P75)
		}
	}
}
func TestResourceHostsRankCurrentWindowAndFold(t *testing.T) {
	a, now := newAgg(time.Minute)
	b := mk(*now, "s", "/")
	b.Resources = nil
	for i := range 15 {
		b.Resources = append(b.Resources, beacon.Resource{
			ID:          fmt.Sprint(i),
			Host:        fmt.Sprintf("cdn%d.example.com", i),
			HasDuration: true,
			DurationMS:  10,
		})
	}
	a.Ingest(b)
	groups := a.Snapshot().ResourceHosts
	require.Len(t, groups, 11)
	var count uint64
	for _, g := range groups {
		count += g.Count
	}
	assert.EqualValues(t, 15, count)
	assert.True(t, groups[10].Other)
	*now = now.Add(time.Minute + time.Second)
	b = mk(*now, "s", "/")
	b.Resources = []beacon.Resource{{ID: "new", Host: "new.example.com"}}
	a.Ingest(b)
	groups = a.Snapshot().ResourceHosts
	require.Len(t, groups, 1)
	assert.Equal(t, "new.example.com", groups[0].Host)
	assert.Zero(t, groups[0].Duration.N)
}
func TestResourceCapacityWithholdsPercentiles(t *testing.T) {
	a, now := newAgg(time.Minute)
	b := mk(*now, "s", "/")
	b.Events = nil
	b.PageHost = "example.com"
	for i := range maxWindowObservations + 1 {
		b.Resources = []beacon.Resource{
			{ID: fmt.Sprint(i), Host: "example.com", HasDuration: true, DurationMS: float64(i), Initiator: "fetch"},
		}
		a.Ingest(b)
	}
	s := a.Snapshot()
	assert.EqualValues(t, maxWindowObservations+1, s.FirstPartyResources)
	assert.Positive(t, s.API.Lost)
	assert.Zero(t, s.API.P75)
}
func TestIsFirstParty(t *testing.T) {
	for _, tc := range []struct {
		page, resource string
		want           bool
	}{{"www.example.com", "cdn.example.com", true}, {"example.com", "other.net", false}, {"localhost", "localhost:8080", true}, {"", "example.com", false}, {"example.com", "unknown", false}, {"127.0.0.1", "127.0.0.1:80", true}} {
		assert.Equal(t, tc.want, isFirstParty(tc.page, tc.resource))
	}
}

func TestLiteralUnknownHostIsNotMissingHost(t *testing.T) {
	a, now := newAgg(time.Minute)
	b := mk(*now, "session", "/")
	b.PageHost = "unknown"
	b.Resources = []beacon.Resource{
		{ID: "real", Host: "unknown", Initiator: "fetch", HasDuration: true, DurationMS: 12},
		{ID: "missing", Host: "", Initiator: "fetch", HasDuration: true, DurationMS: 90},
	}
	a.Ingest(b)
	s := a.Snapshot()
	assert.True(t, isFirstParty("unknown", "unknown"))
	assert.EqualValues(t, 1, s.FirstPartyResources)
	assert.EqualValues(t, 1, s.UnknownResources)
	assert.Equal(t, 1, s.API.N)
	assert.Equal(t, 12., s.API.P75)
	require.Len(t, s.ResourceHosts, 2, "literal and unavailable hosts must have separate populations")
	for _, host := range s.ResourceHosts {
		assert.Equal(t, "unknown", host.Host)
		assert.EqualValues(t, 1, host.Count)
		assert.False(t, host.Other)
		if host.Unknown {
			assert.Equal(t, 90., host.Duration.P75)
		} else {
			assert.Equal(t, 12., host.Duration.P75)
		}
	}
	assert.NotEqual(t, s.ResourceHosts[0].Unknown, s.ResourceHosts[1].Unknown)
}

func TestResourceMissingIdentityIsExcluded(t *testing.T) {
	for _, missing := range []string{"resource", "experience"} {
		t.Run(missing, func(t *testing.T) {
			a, now := newAgg(time.Minute)
			b := mk(*now, "session", "/")
			b.Events = nil
			b.PageHost = "example.com"
			b.Resources = []beacon.Resource{{ID: "resource", Host: "example.com", HasDuration: true, DurationMS: 100, Initiator: "fetch"}}
			if missing == "resource" {
				b.Resources[0].ID = ""
			} else {
				b.ExperienceID = ""
			}
			result := a.Ingest(b)
			snapshot := a.Snapshot()
			assert.Empty(t, result.Observation.Resources)
			assert.False(t, snapshot.ResourcesSeen)
			assert.Zero(t, snapshot.FirstPartyResources)
			assert.Zero(t, snapshot.API.N)
			assert.Empty(t, snapshot.ResourceHosts)
			assert.EqualValues(t, 1, snapshot.Counters[CounterInvalidMeasurements])
		})
	}
}
