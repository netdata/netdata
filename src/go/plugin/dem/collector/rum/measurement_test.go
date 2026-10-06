// SPDX-License-Identifier: GPL-3.0-or-later
package rum

import (
	"context"
	"fmt"
	"strings"
	"testing"
	"time"

	"github.com/netdata/netdata/go/plugins/pkg/metrix"
	demjournal "github.com/netdata/netdata/go/plugins/plugin/dem/journal"
	"github.com/netdata/netdata/go/plugins/plugin/dem/rum/beacon"
	"github.com/netdata/netdata/go/plugins/plugin/dem/rum/history"
	"github.com/netdata/netdata/go/plugins/plugin/dem/rum/registry"
	"github.com/netdata/netdata/go/plugins/plugin/framework/chartengine"
	"github.com/stretchr/testify/require"
)

func measurementCollector(t *testing.T) (*Collector, *registry.Registry) {
	t.Helper()
	ctx := context.Background()
	db, err := demjournal.Open(ctx, "")
	require.NoError(t, err)
	t.Cleanup(func() { require.NoError(t, db.Close()) })
	hub := registry.New()
	t.Cleanup(hub.PublishReceiver(registry.Availability{
		Serving: true,
	}))
	c := New(Dependencies{
		Registry: hub,
		History:  history.NewStore(db),
	})
	c.Name = "shop"
	c.AllowedOrigins = []string{"https://example.org"}
	c.PageGroups = 1
	require.NoError(t, c.Init(ctx))
	t.Cleanup(func() { c.Cleanup(ctx) })
	return c, hub
}

// Use the same cycle boundary as the job runtime and read only fresh values.
// Raw reads would conceal a collector that accidentally keeps publishing stale data.
func collectMeasurements(t *testing.T, c *Collector) metrix.Reader {
	t.Helper()
	managed, ok := metrix.AsCycleManagedStore(c.MetricStore())
	require.True(t, ok)
	cycle := managed.CycleController()
	cycle.BeginCycle()
	require.NoError(t, c.Collect(context.Background()))
	require.NoError(t, cycle.CommitCycleSuccess())
	return c.MetricStore().Read()
}

func measurementValues(reader metrix.Reader, names ...string) map[string]float64 {
	out := map[string]float64{}
	for _, name := range names {
		if value, ok := reader.Value(name, nil); ok {
			out[name] = value
		}
	}
	return out
}

func TestMeasurementPublicationReplacesVitalWithoutCountingAnotherDocument(t *testing.T) {
	c, _ := measurementCollector(t)
	population := []string{"cls_observations", "cls_lost_reports", "cls_good", "cls_needs_improvement", "cls_poor", "cls_p50", "cls_p75", "cls_p95", "document_views", "window_document_views", "observed_sessions"}
	empty := collectMeasurements(t, c)
	require.Equal(t, map[string]float64{
		"cls_observations": 0, "cls_lost_reports": 0, "cls_good": 0, "cls_needs_improvement": 0, "cls_poor": 0,
		"document_views": 0, "window_document_views": 0, "observed_sessions": 0,
	}, measurementValues(empty, population...))
	b := &beacon.Beacon{
		Site:         "shop",
		SessionID:    "session",
		ExperienceID: "document",
		PageGroup:    "/entry",
		Received:     time.Now(),
		Events:       []beacon.Event{{Kind: beacon.EventDocument, ID: "document", Revision: 1}},
		Vitals:       []beacon.Vital{{Name: beacon.CLS, ID: "cls", Revision: 1, Value: 0.1}},
	}
	c.aggregator.Ingest(b)
	first := collectMeasurements(t, c)
	want := map[string]float64{
		"cls_observations": 1, "cls_lost_reports": 0, "cls_good": 1, "cls_needs_improvement": 0, "cls_poor": 0,
		"cls_p50": 0.1, "cls_p75": 0.1, "cls_p95": 0.1,
		"document_views": 1, "window_document_views": 1, "observed_sessions": 1,
	}
	require.Equal(t, want, measurementValues(first, population...))
	c.aggregator.Ingest(b)
	require.Equal(t, want, measurementValues(collectMeasurements(t, c), population...), "a replay cannot increase measurement population or activity")
	b.Vitals[0].Revision, b.Vitals[0].Value = 2, 0.4
	c.aggregator.Ingest(b)
	want["cls_good"], want["cls_poor"] = 0, 1
	want["cls_p50"], want["cls_p75"], want["cls_p95"] = 0.4, 0.4, 0.4
	require.Equal(t, want, measurementValues(collectMeasurements(t, c), population...), "a newer revision replaces the prior rating and value")
}

func TestMeasurementPublicationWithdrawsPercentilesAfterCapacityLoss(t *testing.T) {
	c, _ := measurementCollector(t)
	now := time.Now()
	ingest := func(i int) {
		c.aggregator.Ingest(&beacon.Beacon{
			Site:         "shop",
			ExperienceID: fmt.Sprint(i),
			SessionID:    fmt.Sprint(i),
			PageGroup:    "/entry",
			Received:     now,
			Vitals:       []beacon.Vital{{Name: beacon.LCP, ID: "lcp", Revision: 1, Value: 100}},
		})
	}
	ingest(0)
	before := collectMeasurements(t, c)
	require.Equal(t, map[string]float64{"lcp_p75": 100, "lcp_observations": 1, "lcp_lost_reports": 0}, measurementValues(before, "lcp_p75", "lcp_observations", "lcp_lost_reports"))
	// The production canonical population holds 10,000 observations. Cross that
	// budget through admitted measurements, without replacing internal state or limits.
	for i := 1; i <= 10000; i++ {
		ingest(i)
	}
	after := collectMeasurements(t, c)
	require.Equal(t, map[string]float64{"lcp_observations": 10000, "lcp_good": 10000, "lcp_needs_improvement": 0, "lcp_poor": 0}, measurementValues(after, "lcp_observations", "lcp_good", "lcp_needs_improvement", "lcp_poor"))
	loss, ok := after.Value("lcp_lost_reports", nil)
	require.True(t, ok)
	require.Positive(t, loss)
	sessionLoss, present := after.Value("sessions_lost_reports", nil)
	require.True(t, present)
	require.Positive(t, sessionLoss)
	_, present = after.Value("observed_sessions", nil)
	require.False(t, present, "an incomplete session population is unavailable")
	require.Empty(t, measurementValues(after, "lcp_p50", "lcp_p75", "lcp_p95"), "a previously published percentile must become unavailable")
	labels := metrix.Labels{
		"page":   "/entry",
		"bucket": "value",
	}
	_, ok = after.Value("breakdown_page_lcp", labels)
	require.False(t, ok, "incomplete facet percentiles must also disappear")
	count, ok := after.Value("breakdown_page_lcp_observations", labels)
	require.True(t, ok)
	require.Equal(t, float64(10000), count)
	loss, ok = after.Value("breakdown_page_lcp_lost_reports", labels)
	require.True(t, ok)
	require.Positive(t, loss)
}

func TestMeasurementPublicationStopsWhenMeasurementUnavailable(t *testing.T) {
	for name, disable := range map[string]func(*Collector, *registry.Registry){
		"sampling disabled": func(c *Collector, _ *registry.Registry) { c.MeasureSampleRate = new(float64) },
		"receiver unavailable": func(_ *Collector, hub *registry.Registry) {
			hub.PublishReceiver(registry.Availability{
				Serving: false,
			})
		},
	} {
		t.Run(name, func(t *testing.T) {
			c, hub := measurementCollector(t)
			c.aggregator.Ingest(&beacon.Beacon{
				Site:         "shop",
				ExperienceID: "document",
				SessionID:    "session",
				PageGroup:    "/entry",
				Events:       []beacon.Event{{Kind: beacon.EventDocument, ID: "document", Revision: 1}},
				Vitals:       []beacon.Vital{{Name: beacon.LCP, ID: "lcp", Revision: 1, Value: 1200}},
			})
			before := collectMeasurements(t, c)
			require.Equal(t, map[string]float64{"lcp_p75": 1200, "lcp_observations": 1, "document_views": 1, "observed_sessions": 1}, measurementValues(before, "lcp_p75", "lcp_observations", "document_views", "observed_sessions"))
			disable(c, hub)
			after := collectMeasurements(t, c)
			require.Empty(t, measurementValues(after, "lcp_p75", "lcp_observations", "lcp_lost_reports", "document_views", "observed_sessions", "window_document_views", "window_application_views", "window_js_errors", "window_lost_reports", "sessions_lost_reports"))
			diagnostics := map[string]float64{}
			after.ForEachSeries(func(name string, _ metrix.LabelView, value metrix.SampleValue) { diagnostics[name] = value })
			require.Equal(t, map[string]float64{"history_written": 0, "history_dropped": 0}, diagnostics)
			_, ok := after.StateSet("ingress_state", nil)
			require.True(t, ok, "receiver diagnostics remain observable")
		})
	}
}

func TestMeasurementPublicationSeparatesApplicationViewAndDocumentFacets(t *testing.T) {
	c, _ := measurementCollector(t)
	c.aggregator.Ingest(&beacon.Beacon{
		Site:         "shop",
		ExperienceID: "document",
		PageGroup:    "/entry",
		View:         "checkout",
		ViewID:       "view",
		SessionID:    "session",
		Events:       []beacon.Event{{Kind: beacon.EventDocument, ID: "document", Revision: 1}, {Kind: beacon.EventView, ID: "view", Revision: 1}},
		Vitals:       []beacon.Vital{{Name: beacon.LCP, ID: "lcp", Revision: 1, Value: 1200}},
	})
	reader := collectMeasurements(t, c)
	views := map[string]float64{}
	reader.ForEachSeries(func(name string, labels metrix.LabelView, value metrix.SampleValue) {
		if strings.HasPrefix(name, "breakdown_view_") {
			views[name] = value
		}
	})
	require.Equal(t, map[string]float64{"breakdown_view_application_views": 1, "breakdown_view_js_errors": 0, "breakdown_view_lost_reports": 0}, views)
	contexts := map[string]bool{}
	for _, create := range measurementCharts(t, c) {
		if strings.HasPrefix(create.Meta.Context, "rum.view_") {
			contexts[create.Meta.Context] = true
		}
	}
	require.Equal(t, map[string]bool{"rum.view_activity": true, "rum.view_loss": true}, contexts)

}

func TestMeasurementPublicationDistinguishesLiteralAndPooledOther(t *testing.T) {
	c, _ := measurementCollector(t)
	for i, page := range []string{"other", "other", "/less-traffic"} {
		id := fmt.Sprint(i)
		c.aggregator.Ingest(&beacon.Beacon{
			Site:         "shop",
			ExperienceID: id,
			PageGroup:    page,
			Events:       []beacon.Event{{Kind: beacon.EventDocument, ID: id, Revision: 1}},
			Vitals:       []beacon.Vital{{Name: beacon.LCP, ID: "lcp", Revision: 1, Value: float64(i+1) * 100}},
		})
	}
	reader := collectMeasurements(t, c)
	for bucket, want := range map[string]map[string]float64{
		"value": {"breakdown_page_document_views": 2, "breakdown_page_lcp_observations": 2, "breakdown_page_lcp": 200},
		"other": {"breakdown_page_document_views": 1, "breakdown_page_lcp_observations": 1, "breakdown_page_lcp": 300},
	} {
		got := map[string]float64{}
		for name := range want {
			if value, ok := reader.Value(name, metrix.Labels{
				"page":   "other",
				"bucket": bucket,
			}); ok {
				got[name] = value
			}
		}
		require.Equal(t, want, got, bucket)
	}
	charts := map[string]string{}
	for _, create := range measurementCharts(t, c) {
		if create.Meta.Context == "rum.page_lcp" {
			require.Equal(t, "other", create.Labels["page"])
			charts[create.Labels["bucket"]] = create.ChartID
		}
	}
	require.Len(t, charts, 2, "literal and pooled values must produce distinct chart instances")
	require.NotEmpty(t, charts["value"])
	require.NotEmpty(t, charts["other"])
	require.NotEqual(t, charts["value"], charts["other"])
}

func measurementCharts(t *testing.T, c *Collector) []chartengine.CreateChartAction {
	t.Helper()
	set, err := chartengine.NewTemplateSetYAML([]byte(c.ChartTemplateYAML()))
	require.NoError(t, err)
	engine, err := chartengine.New(chartengine.WithRuntimeStore(nil))
	require.NoError(t, err)
	attempt, err := engine.PreparePlanWithOptions(c.MetricStore().Read(metrix.ReadFlatten()), chartengine.PlanOptions{
		TemplateSet: set,
	})
	require.NoError(t, err)
	defer attempt.Abort()
	var charts []chartengine.CreateChartAction
	for _, action := range attempt.Plan().Actions {
		if create, ok := action.(chartengine.CreateChartAction); ok {
			charts = append(charts, create)
		}
	}
	return charts
}

func TestMeasurementPublicationDistinguishesMissingAndLiteralUnknownHost(t *testing.T) {
	c, _ := measurementCollector(t)
	c.aggregator.Ingest(&beacon.Beacon{
		Site: "shop", ExperienceID: "document", PageHost: "unknown", PageGroup: "/",
		Resources: []beacon.Resource{
			{ID: "missing", Host: "", HasDuration: true, DurationMS: 100},
			{ID: "literal", Host: "unknown", HasDuration: true, DurationMS: 200, Initiator: "fetch"},
		},
	})
	reader := collectMeasurements(t, c)
	for bucket, want := range map[string]float64{"unknown": 100, "value": 200} {
		got, ok := reader.Value("resource_host_p75", metrix.Labels{"host": "unknown", "bucket": bucket})
		require.True(t, ok, bucket)
		require.Equal(t, want, got)
	}
	ids := map[string]string{}
	for _, chart := range measurementCharts(t, c) {
		if chart.Meta.Context == "rum.resource_host_p75" {
			ids[chart.Labels["bucket"]] = chart.ChartID
		}
	}
	require.Len(t, ids, 2)
	require.NotEqual(t, ids["unknown"], ids["value"])
}
