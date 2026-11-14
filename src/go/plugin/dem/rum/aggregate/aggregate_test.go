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

var t0 = time.Date(2030, 1, 1, 12, 0, 0, 0, time.UTC)

const (
	vnodeA = "11111111-1111-4111-8111-111111111111"
	vnodeB = "22222222-2222-4222-8222-222222222222"
)

func newAgg(window time.Duration, cfgs ...SiteCfg) (*Aggregator, *time.Time) {
	cfg := SiteCfg{
		Investigate: InvestigateCfg{
			Rate: 1,
		},
		Name:        "s",
		DisplayName: "S",
		PageGroups:  20,
		Countries:   20,
	}
	if len(cfgs) > 1 {
		panic("newAgg accepts one site config")
	}
	if len(cfgs) == 1 {
		cfg = cfgs[0]
	}
	now := t0
	a := New(window, cfg)
	a.now = func() time.Time { return now }
	return a, &now
}

func mk(now time.Time, session, page string, vitals ...beacon.Vital) *beacon.Beacon {
	return &beacon.Beacon{
		ExperienceID: fmt.Sprintf("%s|%s|%d", session, page, now.UnixNano()),
		Events: []beacon.Event{
			{ID: "activation", Revision: 1, Kind: beacon.EventDocument, Name: "document_activated"},
		},
		Site:      "s",
		Received:  now,
		SessionID: session,
		Path:      page,
		PageGroup: page,
		Browser:   "Chrome",
		Device:    "desktop",
		Country:   "GR",
		Vitals:    vitals,
	}
}

func v(name string, val float64) beacon.Vital {
	return beacon.Vital{
		ID:       name,
		Revision: 1,
		Name:     name,
		Value:    val,
	}
}

func itoa(i int) string { return fmt.Sprint(i) }

type fakeHistorySink struct{ events []HistoryEvent }

func (f *fakeHistorySink) Event(rec HistoryEvent) { f.events = append(f.events, rec) }

func TestVnodeChangeResetsMeasurementsAndPreservesEvidence(t *testing.T) {
	a, now := newAgg(5 * time.Minute)
	a.SetVnodeGUID(vnodeA)
	a.site.cfg.Investigate = InvestigateCfg{
		KeepErrors: true,
	}
	sink := &fakeHistorySink{}
	a.SetHistorySink(sink)
	b := mk(*now, "session", "/old", v(beacon.LCP, 9000))
	b.VnodeGUID = "untrusted"
	b.PageHost = "example.org"
	b.Resources = []beacon.Resource{{ID: "resource", Host: "example.org", HasDuration: true, DurationMS: 100}}
	admitted := a.Ingest(b)
	require.Equal(t, vnodeA, admitted.Observation.VnodeGUID)
	require.Equal(t, "untrusted", b.VnodeGUID)
	require.Empty(t, sink.events)
	require.Empty(t, a.Pages(vnodeB))
	require.Equal(t, a.Pages(""), a.Pages(vnodeA))
	before := a.Snapshot()
	require.Equal(t, 1, before.Vitals[beacon.LCP].N)
	a.SetVnodeGUID(vnodeA)
	require.Equal(t, before, a.Snapshot(), "metadata-only updates do not reset measurements")
	a.SetVnodeGUID(vnodeB)
	empty := a.Snapshot()
	assert.Empty(t, a.Pages(""))
	assert.Zero(t, empty.Vitals[beacon.LCP].N)
	assert.Zero(t, empty.ObservedSessions)
	assert.Zero(t, empty.Counters[CounterPageviews])
	assert.Zero(t, empty.Counters[CounterAccepted])
	assert.False(t, empty.ResourcesSeen)
	assert.Zero(t, empty.FirstPartyResources)
	retained, _ := a.Live(0, 10, "")
	require.Len(t, retained, 2)
	assert.Equal(t, vnodeA, retained[0].VnodeGUID)
	// Promotion after reassignment retains preceding A context and adds B context.
	promoted := &beacon.Beacon{
		Site:      "s",
		SessionID: "session",
		Received:  now.Add(time.Second),
		PageGroup: "/new",
		Errors:    []beacon.Error{{Fingerprint: "failure", Type: "Error", Message: "failed"}},
	}
	a.Ingest(promoted)
	require.Len(t, sink.events, 3)
	assert.Equal(
		t,
		[]string{vnodeA, vnodeA, vnodeB},
		[]string{sink.events[0].VnodeGUID, sink.events[1].VnodeGUID, sink.events[2].VnodeGUID},
	)
	// Reusing the same identified measurements is admitted in B's fresh ledger.
	a.Ingest(b)
	require.Empty(t, a.Pages(vnodeA))
	require.Equal(t, a.Pages(""), a.Pages(vnodeB))
	fresh := a.Snapshot()
	assert.Equal(t, 1, fresh.Vitals[beacon.LCP].N)
	assert.EqualValues(t, 1, fresh.Counters[CounterPageviews])
	rows, _ := a.Live(retained[1].Seq, 10, vnodeB)
	require.Len(t, rows, 3)
	for _, row := range rows {
		assert.Equal(t, vnodeB, row.VnodeGUID)
	}
	a.SetVnodeGUID(vnodeA)
	assert.Empty(t, a.Pages(""), "returning to A starts another fresh measurement window")
	events, ok := a.SessionEvents("session")
	require.True(t, ok)
	require.NotEmpty(t, events)
	assert.Equal(t, vnodeA, events[0].VnodeGUID)
}
