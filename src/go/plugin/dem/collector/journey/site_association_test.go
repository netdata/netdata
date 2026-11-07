// SPDX-License-Identifier: GPL-3.0-or-later
package journey_test

import (
	"context"
	"testing"
	"time"

	"github.com/netdata/netdata/go/plugins/pkg/metrix"
	model "github.com/netdata/netdata/go/plugins/plugin/dem/synthetic"
	registry "github.com/netdata/netdata/go/plugins/plugin/dem/synthetic/registry"
	"github.com/netdata/netdata/go/plugins/plugin/framework/chartengine"
	"github.com/netdata/netdata/go/plugins/plugin/go.d/pkg/collecttest"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func siteExecutor(t *testing.T) executor {
	return executor{execute: func(_ context.Context, r model.Request, _ func(string)) model.Execution {
		assert.Equal(t, "storefront", r.SiteID)
		return model.Execution{Drained: true, Run: model.Run{JobID: r.JobID(), SiteID: r.SiteID, Kind: r.Kind, Name: r.Name, Outcome: model.Success, CompletedUS: time.Now().UnixMicro(), DurationMS: ptr(123), Tests: &model.TestCounts{Declared: 1, Passed: 1}, Metrics: &model.LabMetrics{Performance: ptr(90), FCPMS: ptr(100), LCPMS: ptr(200), TBTMS: ptr(0), SIMS: ptr(100), CLS: ptr(0)}}}
	}}
}

func TestSameSiteJobsHaveIndependentIdentityAndLifetime(t *testing.T) {
	hub := registry.New()
	first := newJourney(siteExecutor(t), hub)
	first.SiteID = "storefront"
	stopFirst := start(t, first)
	second := newJourney(siteExecutor(t), hub)
	second.Name = "search"
	second.SiteID = "storefront"
	start(t, second)
	_, err := collecttest.CollectScalarSeries(first, metrix.ReadFlatten())
	require.NoError(t, err)
	_, err = collecttest.CollectScalarSeries(second, metrix.ReadFlatten())
	require.NoError(t, err)
	jobs := hub.Snapshot(time.Now())
	require.Len(t, jobs, 2)
	assert.Equal(t, "journey:checkout", jobs[0].JobID)
	assert.Equal(t, "journey:search", jobs[1].JobID)
	for _, job := range jobs {
		assert.Equal(t, "storefront", job.SiteID)
		require.NotNil(t, job.Latest)
		assert.Equal(t, "storefront", job.Latest.SiteID)
	}
	stopFirst()
	jobs = hub.Snapshot(time.Now())
	require.Len(t, jobs, 1)
	assert.Equal(t, "journey:search", jobs[0].JobID)
}

func TestSiteLabelDoesNotChangeChartIdentity(t *testing.T) {
	var identity map[string]string
	for _, site := range []string{"", "storefront", "other-site"} {
		t.Run("site="+site, func(t *testing.T) {
			c := newJourney(executor{execute: func(_ context.Context, r model.Request, _ func(string)) model.Execution {
				assert.Equal(t, site, r.SiteID)
				return model.Execution{Drained: true, Run: model.Run{Outcome: model.Success, DurationMS: ptr(123), Tests: &model.TestCounts{Declared: 1, Passed: 1}, Metrics: &model.LabMetrics{Performance: ptr(90), FCPMS: ptr(100), LCPMS: ptr(200), TBTMS: ptr(0), SIMS: ptr(100), CLS: ptr(0)}}}
			}}, registry.New())
			c.SiteID = site
			start(t, c)
			_, err := collecttest.CollectScalarSeries(c, metrix.ReadFlatten())
			require.NoError(t, err)
			set, err := chartengine.NewTemplateSetYAML([]byte(c.ChartTemplateYAML()))
			require.NoError(t, err)
			engine, err := chartengine.New(chartengine.WithRuntimeStore(nil))
			require.NoError(t, err)
			attempt, err := engine.PreparePlanWithOptions(c.MetricStore().Read(metrix.ReadFlatten()), chartengine.PlanOptions{TemplateSet: set})
			require.NoError(t, err)
			defer attempt.Abort()
			ids := map[string]string{}
			for _, action := range attempt.Plan().Actions {
				if create, ok := action.(chartengine.CreateChartAction); ok {
					ids[create.Meta.Context] = create.ChartID
					if site == "" {
						assert.NotContains(t, create.Labels, "site_id")
					} else {
						assert.Equal(t, site, create.Labels["site_id"], create.Meta.Context)
					}
				}
			}
			require.NotEmpty(t, ids)
			if identity == nil {
				identity = ids
			} else {
				assert.Equal(t, identity, ids)
			}
		})
	}
}
