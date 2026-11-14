// SPDX-License-Identifier: GPL-3.0-or-later

package syntheticjob_test

import (
	"context"
	"testing"
	"time"

	"github.com/netdata/netdata/go/plugins/pkg/metrix"
	"github.com/netdata/netdata/go/plugins/plugin/dem/collector/internal/syntheticjobtest"
	"github.com/netdata/netdata/go/plugins/plugin/dem/synthetic"
	"github.com/netdata/netdata/go/plugins/plugin/dem/synthetic/registry"
	"github.com/netdata/netdata/go/plugins/plugin/framework/chartengine"
	"github.com/netdata/netdata/go/plugins/plugin/framework/vnodes"
	"github.com/netdata/netdata/go/plugins/plugin/go.d/pkg/collecttest"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

const siteGUID = "11111111-1111-4111-8111-111111111111"

func siteExecutor(t *testing.T, guid string) syntheticjobtest.Executor {
	return syntheticjobtest.Executor{
		ExecuteFunc: func(_ context.Context, r synthetic.Request, _ func(synthetic.JobState)) synthetic.Execution {
			assert.Equal(t, guid, r.VnodeGUID)
			run := fixture()
			run.Outcome = synthetic.Success
			return synthetic.Execution{
				Drained: true,
				Run:     run,
			}
		},
	}
}

func TestSameSiteJobsHaveIndependentIdentityAndLifetime(t *testing.T) {
	forEachKind(t, func(t *testing.T, module string, k kind) {
		hub := registry.New()
		first := k.new(siteExecutor(t, siteGUID), hub, settings{})
		first.SetConfiguredVnode(vnodes.VirtualNode{
			GUID:     siteGUID,
			Hostname: "site",
		})
		stopFirst := start(t, first)
		second := k.new(siteExecutor(t, siteGUID), hub, settings{
			name: "search",
		})
		second.SetConfiguredVnode(vnodes.VirtualNode{
			GUID:     siteGUID,
			Hostname: "site",
		})
		start(t, second)
		waitLatest(t, hub, module+":checkout")
		waitLatest(t, hub, module+":search")
		_, err := collecttest.CollectScalarSeries(first, metrix.ReadFlatten())
		require.NoError(t, err)
		_, err = collecttest.CollectScalarSeries(second, metrix.ReadFlatten())
		require.NoError(t, err)
		jobs := hub.Snapshot(time.Now())
		require.Len(t, jobs, 2)
		assert.Equal(t, module+":checkout", jobs[0].JobID)
		assert.Equal(t, module+":search", jobs[1].JobID)
		for _, job := range jobs {
			assert.Equal(t, siteGUID, job.VnodeGUID)
			latest, ok := latestRun(hub, job)
			require.True(t, ok)
			assert.Equal(t, siteGUID, latest.VnodeGUID)
		}
		stopFirst()
		jobs = hub.Snapshot(time.Now())
		require.Len(t, jobs, 1)
		assert.Equal(t, module+":search", jobs[0].JobID)
	})
}

func TestVnodeIdentityDoesNotBecomeChartLabel(t *testing.T) {
	forEachKind(t, func(t *testing.T, module string, k kind) {
		var identity map[string]string
		for _, site := range []string{"", siteGUID, "22222222-2222-4222-8222-222222222222"} {
			t.Run("site="+site, func(t *testing.T) {
				hub := registry.New()
				c := k.new(siteExecutor(t, site), hub, settings{})
				c.SetConfiguredVnode(vnodes.VirtualNode{
					GUID:     site,
					Hostname: "site",
				})
				start(t, c)
				waitLatest(t, hub, module+":checkout")
				_, err := collecttest.CollectScalarSeries(c, metrix.ReadFlatten())
				require.NoError(t, err)
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
				ids := map[string]string{}
				for _, action := range attempt.Plan().Actions {
					if create, ok := action.(chartengine.CreateChartAction); ok {
						ids[create.Meta.Context] = create.ChartID
						assert.NotContains(t, create.Labels, "vnode_guid")
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
	})
}
