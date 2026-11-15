// SPDX-License-Identifier: GPL-3.0-or-later

package syntheticjob_test

import (
	"context"
	"strings"
	"testing"

	"github.com/netdata/netdata/go/plugins/pkg/metrix"
	"github.com/netdata/netdata/go/plugins/plugin/dem/collector/internal/syntheticjobtest"
	"github.com/netdata/netdata/go/plugins/plugin/dem/synthetic"
	"github.com/netdata/netdata/go/plugins/plugin/dem/synthetic/registry"
	"github.com/netdata/netdata/go/plugins/plugin/framework/chartengine"
	"github.com/netdata/netdata/go/plugins/plugin/go.d/pkg/collecttest"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// The journey and lighthouse modules share one chart template, and charts materialize only for observed series: a job
// creates the shared charts and its own kind's charts, never the other kind's, even from an observation carrying both
// kinds' values.
func TestJobMaterializesOnlyItsKindCharts(t *testing.T) {
	forEachKind(t, func(t *testing.T, kindName string, k kind) {
		hub := registry.New()
		c := k.new(syntheticjobtest.Executor{
			ExecuteFunc: func(_ context.Context, _ synthetic.Request, state func(synthetic.JobState)) synthetic.Execution {
				state(synthetic.JobRunning)
				return synthetic.Execution{
					Drained: true,
					Run:     fixture(),
				}
			},
		}, hub, settings{})
		start(t, c)
		waitLatest(t, hub, kindName+":checkout")
		collect(t, c)
		set, err := chartengine.NewTemplateSetYAML([]byte(c.ChartTemplateYAML()))
		require.NoError(t, err)
		engine, err := chartengine.New(chartengine.WithRuntimeStore(nil))
		require.NoError(t, err)
		attempt, err := engine.PreparePlanWithOptions(
			c.MetricStore().Read(metrix.ReadFlatten()),
			chartengine.PlanOptions{
				TemplateSet: set,
			},
		)
		require.NoError(t, err)
		defer attempt.Abort()
		created := map[string]bool{}
		for _, action := range attempt.Plan().Actions {
			if create, ok := action.(chartengine.CreateChartAction); ok {
				created[create.Meta.Context] = true
			}
		}
		require.Contains(t, created, "dem_synthetic.execution_state")
		// A kind's charts are the template contexts under its own namespace (dem_synthetic.<kind>.<chart>).
		templateCharts, err := collecttest.ChartTemplateCharts(c.ChartTemplateYAML())
		require.NoError(t, err)
		own := 0
		for context := range templateCharts {
			_, leaf, _ := strings.Cut(context, ".")
			namespace, _, kindChart := strings.Cut(leaf, ".")
			if !kindChart {
				continue
			}
			if namespace == kindName {
				own++
			}
			assert.Equalf(t, namespace == kindName, created[context], "%s job and chart %s", kindName, context)
		}
		assert.Positive(t, own, "the template has charts of kind %s", kindName)
	})
}
