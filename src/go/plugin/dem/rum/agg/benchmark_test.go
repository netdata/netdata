// SPDX-License-Identifier: GPL-3.0-or-later
package agg_test

import (
	"testing"
	"time"

	"github.com/netdata/netdata/go/plugins/plugin/dem/rum/agg"
	"github.com/netdata/netdata/go/plugins/plugin/dem/rum/beacon"
)

type discardHistory struct{}

func (discardHistory) Event(agg.HistoryEvent) {}

// Fixed active identities isolate per-beacon work after bounded sample buffers
// fill. Cardinality growth and the sliding window remain covered by domain tests.
func BenchmarkIngest(b *testing.B) {
	for _, history := range []bool{false, true} {
		name := "measure"
		if history {
			name = "investigate"
		}
		b.Run(name, func(b *testing.B) {
			a := agg.New(5 * time.Minute)
			a.Configure(5*time.Minute, []agg.SiteCfg{{Key: "shop", PageGroups: 20, Countries: 20}})
			if history {
				a.SetHistorySink(discardHistory{})
			}
			input := &beacon.Beacon{
				Site:      "shop",
				SessionID: "browser",
				Path:      "/products",
				PageGroup: "/products",
				Browser:   "Chrome",
				Country:   "GR",
				Device:    "desktop",
				Received:  time.Now(),
				Vitals:    []beacon.Vital{{Name: beacon.LCP, Value: 1200}, {Name: beacon.CLS, Value: .1}},
			}
			b.ReportAllocs()
			b.ResetTimer()
			for range b.N {
				a.Ingest(input)
			}
		})
	}
}
func BenchmarkSnapshot(b *testing.B) {
	a := agg.New(5 * time.Minute)
	a.Configure(5*time.Minute, []agg.SiteCfg{{Key: "shop", PageGroups: 20, Countries: 20}})
	for i := range 100 {
		a.Ingest(
			&beacon.Beacon{
				Site:      "shop",
				SessionID: "browser",
				Path:      "/products",
				PageGroup: "/products",
				Browser:   "Chrome",
				Device:    "desktop",
				Country:   "GR",
				Received:  time.Now(),
				Vitals:    []beacon.Vital{{Name: beacon.LCP, Value: float64(i + 1)}},
			},
		)
	}
	b.ReportAllocs()
	b.ResetTimer()
	for range b.N {
		a.Snapshot()
	}
}

// Error ingestion adds one immutable metadata object per error to retain detail
// for later session promotion. Work stays bounded by the beacon and live ring;
// it does not traverse persisted history. Timings are local trends, not CI gates.
func BenchmarkIngestErrors(b *testing.B) {
	a := agg.New(5 * time.Minute)
	a.Configure(5*time.Minute, []agg.SiteCfg{{Key: "shop", PageGroups: 20, Countries: 20}})
	a.SetHistorySink(discardHistory{})
	input := &beacon.Beacon{
		Site:      "shop",
		SessionID: "browser",
		PageGroup: "/checkout",
		Browser:   "Chrome",
		Received:  time.Now(),
		Errors: []beacon.Error{
			{Type: "TypeError", Message: "checkout failed", Stack: "at checkout()", Fingerprint: "checkout"},
		},
	}
	b.ReportAllocs()
	b.ResetTimer()
	for range b.N {
		a.Ingest(input)
	}
}
