// SPDX-License-Identifier: GPL-3.0-or-later

package prometheus

import (
	"context"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/netdata/netdata/go/plugins/pkg/metrix"
	"github.com/netdata/netdata/go/plugins/plugin/framework/chartengine"
	"github.com/netdata/netdata/go/plugins/plugin/framework/collectorapi"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestCollectorOutputPolicyKeepsFullProfileRollups(t *testing.T) {
	catalog := loadTestCatalog(t, map[string]string{"work": `app: work
match: work
template:
  family: Work
  metrics: [work]
  charts:
    - id: detail
      title: Detail
      context: detail
      units: tasks
      dimensions:
        - selector: work
          name_from_label: client
    - id: model
      title: Model
      context: model
      units: tasks
      instances: {by_labels: [model]}
      aggregation: sum
      dimensions:
        - selector: work
          name: tasks
    - id: team
      title: Team
      context: team
      units: tasks
      instances: {by_labels: [team]}
      aggregation: sum
      dimensions:
        - selector: work
          name: tasks
`})
	population := 600
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		fmt.Fprintln(w, "# TYPE work gauge")
		for i := 0; i < population; i++ {
			fmt.Fprintf(w, "work{client=\"%d\",model=\"%d\",team=\"%d\"} 3\n", i, i%2, i%3)
		}
	}))
	defer srv.Close()
	c := NewWithOptions(WithProfileCatalog(catalog))
	c.URL = srv.URL
	c.MaxOutputSeriesPerContext = 3
	ctx := context.Background()
	require.NoError(t, c.Init(ctx))
	defer c.Cleanup(ctx)
	require.NoError(t, c.Check(ctx))
	source, err := collectorapi.NewChartTemplateSource(c)
	require.NoError(t, err)
	eng, err := chartengine.New()
	require.NoError(t, err)
	chartContexts := map[string]string{}
	for _, n := range []int{600, 3, 600, 3} {
		population = n
		totals := collectOutputLimitCycle(t, c, source, eng, chartContexts)
		_, detail := totals["prometheus.work.detail"]
		assert.Equal(t, n == 3, detail)
		assert.Equal(t, float64(n*3), totals["prometheus.work.model"])
		assert.Equal(t, float64(n*3), totals["prometheus.work.team"])
	}
}

func TestCollectorOutputPolicyAutomaticCounterCompanions(t *testing.T) {
	for _, mode := range []string{profilesModeNone, profilesModeAuto} {
		t.Run(mode, func(t *testing.T) {
			population := 4
			srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
				fmt.Fprintln(w, "# TYPE synthetic_requests_total counter")
				for i := 0; i < population; i++ {
					fmt.Fprintf(w, "synthetic_requests_total{id=\"%d\"} 3\n", i)
				}
			}))
			defer srv.Close()
			c := New()
			c.URL = srv.URL
			c.Profiles.Mode = mode
			c.MaxOutputSeriesPerContext = 3
			c.CounterRawCharts = []string{"synthetic_requests_total"}
			ctx := context.Background()
			require.NoError(t, c.Init(ctx))
			defer c.Cleanup(ctx)
			require.NoError(t, c.Check(ctx))
			source, err := collectorapi.NewChartTemplateSource(c)
			require.NoError(t, err)
			eng, err := chartengine.New()
			require.NoError(t, err)
			contexts := map[string]string{}
			for _, n := range []int{4, 3, 4, 3} {
				population = n
				totals := collectOutputLimitCycle(t, c, source, eng, contexts)
				if n == 4 {
					assert.Empty(t, totals)
					continue
				}
				require.Len(t, totals, 2)
				assert.Contains(t, totals, "prometheus.synthetic_requests_total")
				assert.Contains(t, totals, "prometheus.synthetic_requests_total.raw")
			}
		})
	}
}

func collectOutputLimitCycle(t *testing.T, c *Collector, source *collectorapi.ChartTemplateSource, eng *chartengine.Engine, contexts map[string]string) map[string]float64 {
	t.Helper()
	cc := cycle(t, c.store)
	cc.BeginCycle()
	require.NoError(t, c.Collect(context.Background()))
	set, err := source.Capture()
	require.NoError(t, err)
	require.NoError(t, cc.CommitCycleSuccess())
	attempt, err := eng.PreparePlanWithOptions(c.store.Read(metrix.ReadRaw(), metrix.ReadFlatten()), chartengine.PlanOptions{
		TemplateSet: set,
	})
	require.NoError(t, err)
	defer attempt.Abort()
	totals := map[string]float64{}
	for _, action := range attempt.Plan().Actions {
		switch a := action.(type) {
		case chartengine.CreateChartAction:
			contexts[a.ChartID] = a.Meta.Context
		case chartengine.UpdateChartAction:
			contextName, ok := contexts[a.ChartID]
			require.True(t, ok)
			require.True(t, strings.HasPrefix(contextName, "prometheus."))
			for _, v := range a.Values {
				totals[contextName] += v.Float64
			}
		}
	}
	require.NoError(t, attempt.Commit())
	return totals
}
