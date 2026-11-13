// SPDX-License-Identifier: GPL-3.0-or-later
package aggregate

import (
	"sync"
	"testing"
	"time"

	"github.com/netdata/netdata/go/plugins/plugin/dem/rum/beacon"
	"github.com/stretchr/testify/require"
)

func TestConcurrentReadsCaptureRevisionAndVnodeTogether(t *testing.T) {
	a, now := newAgg(5*time.Minute, SiteCfg{Name: "s", PageGroups: 20, Countries: 20})
	var workers sync.WaitGroup
	start := make(chan struct{})
	workers.Go(func() {
		<-start
		for i := range 1000 {
			guid, page, value := vnodeA, "/alpha", 100.
			if i/10%2 != 0 {
				guid, page, value = vnodeB, "/beta", 200
			}
			value += float64(i % 10)
			a.SetVnodeGUID(guid)
			b := mk(*now, "session", page, v(beacon.LCP, value))
			b.Vitals[0].Revision = uint64(i + 1)
			a.Ingest(b)
			if i%10 > 0 {
				b.Vitals[0].Revision--
				b.Vitals[0].Value = 9999
				a.Ingest(b)
			}
		}
	})
	for range 4 {
		workers.Go(func() {
			<-start
			for range 500 {
				for _, page := range a.Pages("") {
					want := 100.
					if page.VnodeGUID == vnodeB {
						want = 200
					}
					require.Equal(t, map[string]string{vnodeA: "/alpha", vnodeB: "/beta"}[page.VnodeGUID], page.Page)
					require.Equal(t, 1, page.PageviewsWindow)
					require.Equal(t, 1, page.Sessions)
					require.Equal(t, 1, page.Vitals[beacon.LCP].N)
					require.GreaterOrEqual(t, page.Vitals[beacon.LCP].P75, want)
					require.LessOrEqual(t, page.Vitals[beacon.LCP].P75, want+9)
				}
				s := a.Snapshot()
				require.EqualValues(t, s.Vitals[beacon.LCP].N, s.PageviewsWindow)
				require.Equal(t, s.Vitals[beacon.LCP].N, s.ObservedSessions)
				for _, group := range s.Breakdowns[KindPage] {
					want := 100.
					if group.Value == "/beta" {
						want = 200
					}
					require.GreaterOrEqual(t, group.Vitals[beacon.LCP].P75, want)
					require.LessOrEqual(t, group.Vitals[beacon.LCP].P75, want+9)
					require.Equal(t, s.Vitals[beacon.LCP], group.Vitals[beacon.LCP])
				}
			}
		})
	}
	close(start)
	workers.Wait()
	pages := a.Pages("")
	require.Len(t, pages, 1)
	require.Equal(t, vnodeB, pages[0].VnodeGUID)
	require.Equal(t, 209., pages[0].Vitals[beacon.LCP].P75)
	require.Equal(t, 209., a.Snapshot().Vitals[beacon.LCP].P75)
}

func TestResourceClassificationSurvivesInputMutation(t *testing.T) {
	a, now := newAgg(time.Minute)
	b := mk(*now, "session", "/")
	b.PageHost = "example.org"
	b.Resources = []beacon.Resource{
		{ID: "api", Host: "api.example.org", Initiator: "fetch", HasDuration: true, DurationMS: 0},
		{ID: "image", Host: "example.org", Initiator: "img", HasDuration: true, DurationMS: 40},
		{ID: "invalid", Host: "example.org", Initiator: "fetch", HasDuration: true, DurationMS: -1},
		{ID: "third", Host: "example.net", Initiator: "fetch", HasDuration: true, DurationMS: 80},
	}
	a.Ingest(b)
	b.PageHost = "example.net"
	b.Resources[0].Host = "example.net"
	b.Resources[0].DurationMS = 999
	first := a.Snapshot()
	require.Equal(t, VitalStats{N: 1}, first.API, "zero duration is an observed API value")
	require.EqualValues(t, 3, first.FirstPartyResources)
	require.EqualValues(t, 1, first.ThirdPartyResources)
	require.EqualValues(t, 1, first.Counters[CounterInvalidMeasurements])
	require.Equal(t, first, a.Snapshot(), "sorting owned distributions must not change the next read")
}
