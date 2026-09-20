// SPDX-License-Identifier: GPL-3.0-or-later

package prometheus

import (
	"context"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"

	"github.com/netdata/netdata/go/plugins/pkg/metrix"
	"github.com/netdata/netdata/go/plugins/plugin/framework/chartengine"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

const cardinalityTestProfile = `match: app_*
app: app
template:
  family: requests
  metrics: [app_requests_total]
  charts:
    - title: Requests by model
      context: models
      units: requests/s
      instances: {by_labels: [model]}
      dimensions: [{selector: app_requests_total, name: requests}]
    - title: Requests by team
      context: teams
      units: requests/s
      instances: {by_labels: [team]}
      dimensions: [{selector: app_requests_total, name: requests}]
    - title: Request detail
      context: detail
      units: requests/s
      instances: {by_labels: [client, model, team]}
      dimensions: [{selector: app_requests_total, name: requests}]
`

func TestCollector_ProfileCardinality(t *testing.T) {
	for _, limits := range []struct {
		name          string
		total, metric int
		detail        bool
	}{
		{"context", 500, 1000, false},
		{"metric_in_context", 10000, 500, false},
		{"exact_boundary", 600, 600, true},
		{"disabled", 0, 0, true},
	} {
		t.Run(limits.name, func(t *testing.T) {
			var population atomic.Int64
			population.Store(600)
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
				var body strings.Builder
				body.WriteString("# TYPE app_requests_total counter\n")
				for i := range int(population.Load()) {
					fmt.Fprintf(&body, "app_requests_total{client=\"c%d\",model=\"m%d\",team=\"t%d\"} %d\n", i, i%2, i%3, i%5+1)
				}
				_, _ = w.Write([]byte(body.String()))
			}))
			defer server.Close()
			collector := NewWithOptions(WithProfileCatalog(loadTestCatalog(t, map[string]string{"app": cardinalityTestProfile})))
			collector.URL, collector.MaxTS, collector.MaxTSPerMetric = server.URL, limits.total, limits.metric
			require.NoError(t, collector.Init(context.Background()))
			t.Cleanup(func() { collector.Cleanup(context.Background()) })
			require.NoError(t, collector.Check(context.Background()), "raw source cardinality must not block rollups")
			var rejected map[string]chartengine.PlanRouteDiagnostic
			engine, err := chartengine.New(chartengine.WithEnginePolicy(collector.EnginePolicy()),
				chartengine.WithPlanRouteDiagnosticObserver(func(fact chartengine.PlanRouteDiagnostic) {
					if fact.Decision == chartengine.PlanRouteLifecycleRejected {
						rejected[fact.Context] = fact
					}
				}))
			require.NoError(t, err)
			require.NoError(t, engine.LoadYAML([]byte(collector.ChartTemplateYAML()), 1))
			contexts := make(map[string]string)
			for _, size := range []int64{600, 3, 600, 3} {
				population.Store(size)
				collectProfileRelabelOnce(t, collector)
				rejected = make(map[string]chartengine.PlanRouteDiagnostic)
				values := commitCardinalityPlan(t, collector, engine, contexts)
				if size == 600 {
					assert.ElementsMatch(t, []float64{900, 900}, values["prometheus.app.models"])
					assert.ElementsMatch(t, []float64{600, 600, 600}, values["prometheus.app.teams"])
				} else {
					assert.ElementsMatch(t, []float64{4, 2}, values["prometheus.app.models"])
					assert.ElementsMatch(t, []float64{1, 2, 3}, values["prometheus.app.teams"])
				}
				if size == 600 && !limits.detail {
					assert.NotContains(t, values, "prometheus.app.detail")
					require.Len(t, rejected, 1)
					assert.Equal(t, 600, rejected["prometheus.app.detail"].SeriesCount)
					assert.Equal(t, 500, rejected["prometheus.app.detail"].SeriesLimit)
				} else {
					assert.Len(t, values["prometheus.app.detail"], int(size))
					assert.Empty(t, rejected)
				}
			}
		})
	}
}

func commitCardinalityPlan(t *testing.T, collector *Collector, engine *chartengine.Engine, contexts map[string]string) map[string][]float64 {
	t.Helper()
	attempt, err := engine.PreparePlan(collector.MetricStore().Read(metrix.ReadRaw(), metrix.ReadFlatten()))
	require.NoError(t, err)
	values := make(map[string][]float64)
	for _, action := range attempt.Plan().Actions {
		switch action := action.(type) {
		case chartengine.CreateChartAction:
			contexts[action.ChartID] = action.Meta.Context
		case chartengine.UpdateChartAction:
			for _, value := range action.Values {
				values[contexts[action.ChartID]] = append(values[contexts[action.ChartID]], value.Float64)
			}
		}
	}
	require.NoError(t, attempt.Commit())
	return values
}
