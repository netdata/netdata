// SPDX-License-Identifier: GPL-3.0-or-later

package prometheus

import (
	"bytes"
	"context"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"

	"github.com/netdata/netdata/go/plugins/logger"
	"github.com/netdata/netdata/go/plugins/pkg/metrix"
	"github.com/netdata/netdata/go/plugins/pkg/netdataapi"
	"github.com/netdata/netdata/go/plugins/plugin/framework/chartemit"
	"github.com/netdata/netdata/go/plugins/plugin/framework/chartengine"
	"github.com/netdata/netdata/go/plugins/plugin/framework/collectorapi"
	"github.com/netdata/netdata/go/plugins/plugin/go.d/collector/prometheus/promprofiles"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestCollectorOutputLimitUsesPublishedContext(t *testing.T) {
	for _, tc := range []struct {
		name, first, second string
		population          int
		rollup              bool
	}{
		{"quote", "work", "wo'rk", 2, false},
		{"backslash", "work/a", "work\\a", 2, false},
		{"newline", "work a", "work\na", 2, false},
		{"carriage_return", "work a", "work\ra", 2, false},
		{"nul", "work", "wo\x00rk", 2, false},
		{"partial_context", "work", "wo'rk", 4, true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			secondDimension := "name_from_label: client"
			if tc.rollup {
				secondDimension = "name: total"
			}
			profile := fmt.Sprintf(`app: review
match: work
template:
  family: Work
  metrics: [work]
  charts:
    - id: first
      title: Work
      context: %q
      units: tasks
      dimensions:
        - selector: work
          name_from_label: client
    - id: second
      title: Work
      context: %q
      units: tasks
      dimensions:
        - selector: work
          %s
`, tc.first, tc.second, secondDimension)
			dir := t.TempDir()
			require.NoError(t, os.WriteFile(filepath.Join(dir, "review.yaml"), []byte(profile), 0o600))
			catalog, err := promprofiles.LoadFromDirs([]promprofiles.DirSpec{{Path: dir, IsStock: false}})
			require.NoError(t, err)
			srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
				fmt.Fprintln(w, "# TYPE work gauge")
				for i := range tc.population {
					fmt.Fprintf(w, "work{client=%q} %d\n", fmt.Sprint(i), i+1)
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
			cc := cycle(t, c.store)
			cc.BeginCycle()
			require.NoError(t, c.Collect(ctx))
			set, err := source.Capture()
			require.NoError(t, err)
			require.NoError(t, cc.CommitCycleSuccess())
			var logs, wire bytes.Buffer
			eng, err := chartengine.New(chartengine.WithRuntimeStore(nil), chartengine.WithRuntimePlannerMode(), chartengine.WithLogger(logger.NewWithWriter(&logs)))
			require.NoError(t, err)
			attempt, err := eng.PreparePlanWithOptions(c.store.Read(metrix.ReadRaw(), metrix.ReadFlatten()), chartengine.PlanOptions{TemplateSet: set})
			require.NoError(t, err)
			defer attempt.Abort()
			require.NoError(t, chartemit.ApplyPlan(netdataapi.New(&wire), attempt.Plan(), chartemit.EmitEnv{
				TypeID: "prometheus.review", UpdateEvery: 1, Plugin: "go.d", Module: "prometheus", JobName: "review",
			}))
			assert.NotContains(t, wire.String(), "CHART ")
			assert.NotContains(t, wire.String(), "BEGIN ")
			assert.Empty(t, logs.String(), "transitions wait for publication commit")
			require.NoError(t, attempt.Commit())
			count := tc.population * 2
			if tc.rollup {
				count = tc.population + 1
			}
			assert.Contains(t, logs.String(), "newly omitted=1")
			assert.Contains(t, logs.String(), "prometheus.review."+tc.first)
			assert.Contains(t, logs.String(), fmt.Sprintf("(%d)", count))
		})
	}
}
