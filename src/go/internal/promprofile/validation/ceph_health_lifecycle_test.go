// SPDX-License-Identifier: GPL-3.0-or-later

package promvalidation

import (
	"context"
	"net/http"
	"net/http/httptest"
	"sync"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/netdata/netdata/go/plugins/pkg/metrix"
	"github.com/netdata/netdata/go/plugins/plugin/framework/chartengine"
	promcollector "github.com/netdata/netdata/go/plugins/plugin/go.d/collector/prometheus"
)

func TestCephHealthDetailKeepsNameIdentityAcrossObservedLabelChangesThenExpires(t *testing.T) {
	const active = `
# TYPE ceph_health_status gauge
ceph_health_status 1
# TYPE ceph_health_detail gauge
ceph_health_detail{name="RECENT_CRASH",severity="HEALTH_WARN"} 1
`
	const resolved = `
# TYPE ceph_health_status gauge
ceph_health_status 0
# TYPE ceph_health_detail gauge
ceph_health_detail{name="RECENT_CRASH",severity="HEALTH_WARN"} 0
`
	// Ceph's supported health-history implementation records severity when a named
	// entry is created. This synthetic change proves profile identity if a source
	// version ever updates that metadata; it is not a claimed Ceph transition.
	const changedMetadata = `
# TYPE ceph_health_status gauge
ceph_health_status 2
# TYPE ceph_health_detail gauge
ceph_health_detail{name="RECENT_CRASH",severity="HEALTH_ERR"} 1
`
	const absent = `
# TYPE ceph_health_status gauge
ceph_health_status 0
`

	var input struct {
		sync.RWMutex
		body string
	}
	input.body = active
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		input.RLock()
		defer input.RUnlock()
		_, _ = w.Write([]byte(input.body))
	}))
	defer server.Close()

	collector := promcollector.New()
	collector.URL = server.URL
	collector.Profiles = promcollector.ProfilesConfig{Mode: "auto"}
	require.NoError(t, collector.Init(context.Background()))
	require.NoError(t, collector.Check(context.Background()))
	defer collector.Cleanup(context.Background())

	engine, err := chartengine.New()
	require.NoError(t, err)
	require.NoError(t, engine.LoadYAML([]byte(collector.ChartTemplateYAML()), 1))
	cycles, ok := metrix.AsCycleManagedStore(collector.MetricStore())
	require.True(t, ok)

	collectPlan := func(body string) chartengine.Plan {
		input.Lock()
		input.body = body
		input.Unlock()
		cycles.CycleController().BeginCycle()
		require.NoError(t, collector.Collect(context.Background()))
		require.NoError(t, cycles.CycleController().CommitCycleSuccess())
		attempt, err := engine.PreparePlan(collector.MetricStore().Read(metrix.ReadRaw(), metrix.ReadFlatten()))
		require.NoError(t, err)
		plan := attempt.Plan()
		require.NoError(t, attempt.Commit())
		return plan
	}

	activePlan := collectPlan(active)
	chartID := cephHealthLifecycleChartID(t, activePlan)
	if got := cephHealthLifecycleUpdate(t, activePlan, chartID); got != 1 {
		t.Fatalf("active health-detail value = %v, want 1", got)
	}

	changedMetadataPlan := collectPlan(changedMetadata)
	if cephHealthLifecycleHasRemove(changedMetadataPlan, chartID) {
		t.Fatalf("observed label change removed the named health-check chart: %#v", changedMetadataPlan.Actions)
	}
	cephHealthLifecycleHasNoCreate(t, changedMetadataPlan)
	require.Equal(t, map[string]string{
		"name":     "RECENT_CRASH",
		"severity": "HEALTH_ERR",
	}, cephHealthLifecycleLabelUpdate(t, changedMetadataPlan, chartID))
	if got := cephHealthLifecycleUpdate(t, changedMetadataPlan, chartID); got != 1 {
		t.Fatalf("health-detail value after observed label change = %v, want 1", got)
	}

	resolvedPlan := collectPlan(resolved)
	if cephHealthLifecycleHasRemove(resolvedPlan, chartID) {
		t.Fatalf("resolved ordinary health check removed its chart instead of emitting zero: %#v", resolvedPlan.Actions)
	}
	if got := cephHealthLifecycleUpdate(t, resolvedPlan, chartID); got != 0 {
		t.Fatalf("resolved health-detail value = %v, want 0", got)
	}

	for cycle := 1; cycle < 5; cycle++ {
		plan := collectPlan(absent)
		if cephHealthLifecycleHasRemove(plan, chartID) {
			t.Fatalf("absent health check expired early at successful cycle %d: %#v", cycle, plan.Actions)
		}
	}
	finalPlan := collectPlan(absent)
	if !cephHealthLifecycleHasRemove(finalPlan, chartID) {
		t.Fatalf("absent health check did not expire after five successful cycles: %#v", finalPlan.Actions)
	}
}

func cephHealthLifecycleChartID(t *testing.T, plan chartengine.Plan) string {
	t.Helper()
	for _, item := range plan.Actions {
		if action, ok := item.(chartengine.CreateChartAction); ok && action.Meta.Context == "prometheus.ceph.health.health_checks.state" {
			return action.ChartID
		}
	}
	t.Fatalf("initial fixture did not create the Ceph health-detail chart: %#v", plan.Actions)
	return ""
}

func cephHealthLifecycleUpdate(t *testing.T, plan chartengine.Plan, chartID string) float64 {
	t.Helper()
	for _, item := range plan.Actions {
		action, ok := item.(chartengine.UpdateChartAction)
		if !ok || action.ChartID != chartID {
			continue
		}
		for _, value := range action.Values {
			if value.Name != "value" {
				continue
			}
			if !value.IsFloat || value.IsEmpty {
				t.Fatalf("health-detail update is not a concrete float value: %#v", value)
			}
			return value.Float64
		}
	}
	t.Fatalf("plan has no health-detail update for chart %q: %#v", chartID, plan.Actions)
	return 0
}

func cephHealthLifecycleLabelUpdate(t *testing.T, plan chartengine.Plan, chartID string) map[string]string {
	t.Helper()
	for _, item := range plan.Actions {
		action, ok := item.(chartengine.UpdateChartLabelsAction)
		if ok && action.ChartID == chartID {
			return action.Labels
		}
	}
	t.Fatalf("plan has no health-detail label update for chart %q: %#v", chartID, plan.Actions)
	return nil
}

func cephHealthLifecycleHasNoCreate(t *testing.T, plan chartengine.Plan) {
	t.Helper()
	for _, item := range plan.Actions {
		if action, ok := item.(chartengine.CreateChartAction); ok && action.Meta.Context == "prometheus.ceph.health.health_checks.state" {
			t.Fatalf("severity change created a second named health-check chart: %#v", plan.Actions)
		}
	}
}

func cephHealthLifecycleHasRemove(plan chartengine.Plan, chartID string) bool {
	for _, item := range plan.Actions {
		if action, ok := item.(chartengine.RemoveChartAction); ok && action.ChartID == chartID {
			return true
		}
	}
	return false
}
