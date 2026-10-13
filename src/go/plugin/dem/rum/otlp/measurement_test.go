// SPDX-License-Identifier: GPL-3.0-or-later
package otlp

import (
	"testing"
	"time"

	"github.com/netdata/netdata/go/plugins/plugin/dem/rum/aggregate"
	"github.com/netdata/netdata/go/plugins/plugin/dem/rum/beacon"
	"github.com/stretchr/testify/require"
)

func TestExportUsesAcceptedMeasurementUpdatesAndIdentity(t *testing.T) {
	counters := newRecCounters()
	exporter := newExporter(t, "127.0.0.1:1", counters, nil)
	owner := aggregate.New(5*time.Minute, aggregate.SiteCfg{
		Name:       "s1",
		PageGroups: 20,
		Countries:  20,
		Investigate: aggregate.InvestigateCfg{
			Rate: 1,
		},
	})
	event := &beacon.Beacon{
		Browser:      "Chrome",
		AppVersion:   "first-release",
		Site:         "s1",
		Received:     time.Now(),
		SessionID:    "session",
		ExperienceID: "document",
		PageGroup:    "/entry",
		Path:         "/route",
		View:         "route",
		ViewID:       "view",
		Events:       []beacon.Event{{Kind: beacon.EventDocument, ID: "document", Revision: 1}},
		Vitals:       []beacon.Vital{{Name: beacon.CLS, ID: "metric", Revision: 1, Value: 0.1}},
	}
	exporter.Ingest(event, owner.Ingest(event))
	require.Len(t, exporter.ch, 2)
	page, vital := simplify(<-exporter.ch), simplify(<-exporter.ch)
	require.Equal(t, "pageview", page.attrs["rum.type"])
	require.Equal(t, "vital", vital.attrs["rum.type"])
	require.Equal(t, "document", vital.attrs["document.experience.id"])
	require.Equal(t, "view", vital.attrs["application.view.id"])
	require.Equal(t, "/entry", vital.attrs["page.group"])
	require.Equal(t, "metric", vital.attrs["metric.id"])
	require.Equal(t, "1", vital.attrs["metric.revision"])
	exporter.Ingest(event, owner.Ingest(event))
	require.Empty(t, exporter.ch, "filtered empty result must never fall back to original beacon")
	event.Vitals[0].Revision, event.Vitals[0].Value = 2, 0.4
	event.View, event.ViewID = "later-route", "later-view"
	event.Browser, event.AppVersion = "Firefox", "later-release"
	exporter.Ingest(event, owner.Ingest(event))
	require.Len(t, exporter.ch, 1, "a vital update is not a document activation")
	update := simplify(<-exporter.ch)
	require.Equal(t, "vital", update.attrs["rum.type"])
	require.Equal(t, "metric", update.attrs["metric.id"])
	require.Equal(t, "2", update.attrs["metric.revision"])
	require.Equal(t, "0.4", update.attrs["metric.value"])
	require.Equal(t, "later-route", update.attrs["page.view"], "each revision retains its application view at report time")
	require.Equal(t, "later-view", update.attrs["application.view.id"])
	require.Equal(t, "Chrome", update.attrs["browser.name"])
	require.Equal(t, "first-release", update.attrs["app.version"])
}

func TestExportDoesNotResurrectFilteredOrMissingObservation(t *testing.T) {
	counters := newRecCounters()
	logs := newExporter(t, "127.0.0.1:1", counters, nil)
	traces := newTraceExporter(t, "127.0.0.1:1", "s1", counters, nil)
	original := tracedBeacon()
	original.Errors = []beacon.Error{{Type: "Error", Message: "already processed"}}
	for _, observation := range []*beacon.Beacon{nil, {Site: "s1"}} {
		result := aggregate.Result{
			Accepted:     true,
			Investigated: true,
			Observation:  observation,
		}
		logs.Ingest(original, result)
		traces.Ingest(original, result)
	}
	require.Empty(t, logs.ch)
	require.Empty(t, traces.spanCh)
	require.Empty(t, counters.snapshot())
}
