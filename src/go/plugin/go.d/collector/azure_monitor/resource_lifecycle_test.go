// SPDX-License-Identifier: GPL-3.0-or-later

package azure_monitor

import (
	"context"
	"errors"
	"fmt"
	"os"
	"reflect"
	"strings"
	"testing"
	"time"

	"github.com/Azure/azure-sdk-for-go/sdk/monitor/query/azmetrics"
	"github.com/netdata/netdata/go/plugins/pkg/metrix"
	"github.com/netdata/netdata/go/plugins/plugin/framework/chartengine"
	"github.com/netdata/netdata/go/plugins/plugin/framework/collectorapi"
	"github.com/netdata/netdata/go/plugins/plugin/go.d/collector/azure_monitor/azureprofiles"
	"github.com/netdata/netdata/go/plugins/plugin/go.d/pkg/collecttest"
	"github.com/stretchr/testify/require"
)

func TestCollector_PlanChangePreservesCounter(t *testing.T) {
	now := time.Date(2026, 10, 26, 12, 0, 0, 0, time.UTC)
	row := func(plan string) map[string]any {
		return testResource("Microsoft.Web/sites", "app-a", map[string]any{"nd_properties_serverFarmId": "/subscriptions/sub-1/resourceGroups/rg-a/providers/Microsoft.Web/serverfarms/" + plan})
	}
	rg := &mockResourceGraph{resources: []map[string]any{row("plan-a")}}
	mx := &mockMetricsClient{}
	c := newTestCollectorWithMocks(rg, mx)
	c.Config = testConfig()
	c.Config.Profiles.ModeExact = testProfilesModeEntries("app_service")
	c.Config.Discovery.RefreshEvery = 60
	c.now = func() time.Time { return now }
	require.NoError(t, c.Init(context.Background()))
	require.NoError(t, c.Check(context.Background()))
	engine, err := chartengine.New(chartengine.WithRuntimeStore(nil))
	require.NoError(t, err)
	require.NoError(t, engine.LoadYAML([]byte(c.ChartTemplateYAML()), 1))
	chartIDs := make(map[string]bool)
	for i := 0; i < 3; i++ {
		if i == 2 {
			rg.setResources([]map[string]any{row("plan-b")})
		}
		mx.queryResponse = appServiceResponse(now)
		series, err := collecttest.CollectScalarSeries(c)
		require.NoError(t, err)
		v, ok := seriesValue(series, "app_service.requests_total{")
		require.True(t, ok)
		require.Equal(t, metrix.SampleValue((i+1)*10), v, "metadata-only plan move must preserve cumulative requests")
		attempt, err := engine.PreparePlan(c.MetricStore().Read(metrix.ReadFlatten()))
		require.NoError(t, err)
		labelUpdates := 0
		for _, action := range attempt.Plan().Actions {
			switch action := action.(type) {
			case chartengine.CreateChartAction:
				require.Zero(t, i, "metadata changes must not recreate charts")
				chartIDs[action.ChartID] = true
			case chartengine.CreateDimensionAction:
				require.Zero(t, i, "metadata changes must not recreate dimensions")
			case chartengine.RemoveChartAction, chartengine.RemoveDimensionAction:
				t.Fatalf("unexpected retirement: %#v", action)
			case chartengine.UpdateChartLabelsAction:
				require.Equal(t, 2, i)
				require.True(t, chartIDs[action.ChartID])
				require.Equal(t, "plan-b", action.Labels["app_service_plan"])
				labelUpdates++
			}
		}
		require.Len(t, chartIDs, 2)
		if i == 2 {
			require.Equal(t, 2, labelUpdates)
		}
		require.NoError(t, attempt.Commit())
		now = now.Add(time.Minute)
	}
}

func TestCollector_FreshStateSurvivesMonitorFailure(t *testing.T) {
	now := time.Date(2026, 10, 26, 12, 0, 0, 0, time.UTC)
	rg := &mockResourceGraph{resources: []map[string]any{testVirtualMachine("vm-a", "Standard_B2s", "Linux", "PowerState/running")}}
	mx := &mockMetricsClient{queryErr: errors.New("Azure Monitor unavailable")}
	c := newTestCollectorWithMocks(rg, mx)
	c.Config = testConfig()
	c.Config.Profiles.ModeExact = testProfilesModeEntries("virtual_machines")
	c.now = func() time.Time { return now }
	require.NoError(t, c.Init(context.Background()))
	require.NoError(t, c.Check(context.Background()))
	series, err := collecttest.CollectScalarSeries(c, metrix.ReadFlatten())
	require.NoError(t, err, "successful Resource Graph state should remain publishable")
	v, ok := seriesValue(series, "virtual_machines.power_state{", `virtual_machines.power_state="running"`)
	require.True(t, ok)
	require.Equal(t, metrix.SampleValue(1), v)
}

func TestCollector_ResourceMembershipTransitions(t *testing.T) {
	type step struct {
		after                       time.Duration
		plan, workload, environment string
		absent                      bool
		want                        float64
	}
	tests := map[string][]step{
		"metadata refresh between metric polls": {
			{plan: "plan-a", want: 10},
			{after: time.Minute, plan: "plan-b", want: 10},
			{after: 4 * time.Minute, plan: "plan-b", want: 20},
		},
		"removed resource then reappears": {
			{plan: "plan-a", want: 10},
			{after: time.Minute, absent: true},
			{after: time.Minute, plan: "plan-a", want: 10},
		},
		"profile membership removed then restored": {
			{plan: "plan-a", environment: "prod", want: 10},
			{after: time.Minute, plan: "plan-a", environment: "dev"},
			{after: time.Minute, plan: "plan-a", environment: "prod", want: 10},
		},
		"scope move and return starts new totals": {
			{plan: "plan-a", workload: "api", want: 10},
			{after: time.Minute, plan: "plan-a", workload: "worker", want: 10},
			{after: time.Minute, plan: "plan-a", workload: "api", want: 10},
		},
	}
	for name, steps := range tests {
		t.Run(name, func(t *testing.T) {
			now := time.Date(2026, 10, 26, 12, 0, 0, 0, time.UTC)
			rg, mx := &mockResourceGraph{}, &mockMetricsClient{}
			setResource := func(s step) {
				if s.absent {
					rg.setResources(nil)
					return
				}
				rg.setResources([]map[string]any{testResource("Microsoft.Web/sites", "app-a", map[string]any{
					"nd_properties_serverFarmId": "/plans/" + s.plan,
					"tags":                       map[string]any{"workload": s.workload, "env": s.environment},
				})})
			}
			setResource(steps[0])
			c := newTestCollectorWithMocks(rg, mx)
			if name == "metadata refresh between metric polls" {
				data, err := os.ReadFile("../../config/go.d/azure_monitor.profiles/default/app_service.yaml")
				require.NoError(t, err)
				catalog := mustLoadStockCatalog(t, map[string]string{
					"app_service.yaml": strings.ReplaceAll(string(data), "time_grain: PT1M", "time_grain: PT5M"),
				})
				c.loadProfileCatalog = func() (azureprofiles.Catalog, error) { return catalog, nil }
			}
			c.Config = testConfig()
			c.Config.Profiles.ModeExact = testProfilesModeEntries("app_service")
			c.Config.Discovery.RefreshEvery = 60
			c.Config.VirtualNodes = &VirtualNodesConfig{ByResourceTag: "workload"}
			if steps[0].environment != "" {
				c.Config.Profiles.ModeExact.Entries[0].Filters = &ResourceFiltersConfig{Tags: map[string][]string{"env": {"prod"}}}
			}
			c.now = func() time.Time { return now }
			require.NoError(t, c.Init(context.Background()))
			require.NoError(t, c.Check(context.Background()))
			for _, s := range steps {
				now = now.Add(s.after)
				setResource(s)
				mx.queryResponse = appServiceResponse(now)
				scope, _ := workloadHostScope(resourceInfo{Tags: []resourceTag{{Key: "workload", Value: s.workload}}}, "workload")
				series, err := collecttest.CollectScalarSeries(c, metrix.ReadHostScope(scope.ScopeKey))
				require.NoError(t, err)
				if s.want == 0 {
					require.Empty(t, series)
					continue
				}
				require.Len(t, series, 2, "old metadata must not remain visible alongside current labels")
				counter, ok := seriesValue(series, "app_service.requests_total{", `app_service_plan="`+s.plan+`"`)
				require.True(t, ok)
				require.Equal(t, metrix.SampleValue(s.want), counter)
				gauge, ok := seriesValue(series, "app_service.http_response_time_average{", `app_service_plan="`+s.plan+`"`)
				require.True(t, ok)
				require.Equal(t, metrix.SampleValue(0.25), gauge)
			}
		})
	}
}

func appServiceResponse(now time.Time) azmetrics.QueryResourcesResponse {
	return azmetrics.QueryResourcesResponse{MetricResults: azmetrics.MetricResults{Values: []azmetrics.MetricData{{
		ResourceID: new("/subscriptions/sub-1/resourcegroups/rg-a/providers/microsoft.web/sites/app-a"),
		Values: []azmetrics.Metric{
			{Name: &azmetrics.LocalizableString{Value: new("Requests")}, TimeSeries: []azmetrics.TimeSeriesElement{{Data: []azmetrics.MetricValue{{TimeStamp: &now, Total: new(10.0)}}}}},
			metricWithAvg("HttpResponseTime", now, 0.25),
		},
	}}}}
}

func TestCollector_ResourceStateFreshness(t *testing.T) {
	tests := map[string]struct {
		refresh  int
		queryErr error
	}{
		"refresh enabled":     {refresh: 60},
		"refresh disabled":    {refresh: 0},
		"Monitor unavailable": {refresh: 60, queryErr: errors.New("Monitor unavailable")},
	}
	for name, tc := range tests {
		t.Run(name, func(t *testing.T) {
			now := time.Date(2026, 10, 26, 12, 0, 0, 0, time.UTC)
			rg := &mockResourceGraph{resources: []map[string]any{testVirtualMachine("vm-a", "Standard_B2s", "Linux", "PowerState/running")}}
			mx := &mockMetricsClient{queryErr: tc.queryErr}
			c := newTestCollectorWithMocks(rg, mx)
			c.Config = testConfig()
			c.Config.Profiles.ModeExact = testProfilesModeEntries("virtual_machines")
			c.Config.Discovery.RefreshEvery = tc.refresh
			c.now = func() time.Time { return now }
			require.NoError(t, c.Init(context.Background()))
			require.NoError(t, c.Check(context.Background()))
			steps := []struct {
				after        time.Duration
				discoveryErr error
				state, want  string
			}{
				{state: "PowerState/running", want: "running"},
				{after: 59 * time.Second, discoveryErr: errors.New("ARG unavailable"), state: "PowerState/deallocated", want: "running"},
				{after: time.Second, discoveryErr: errors.New("ARG unavailable"), state: "PowerState/deallocated"},
				{after: time.Minute, state: "PowerState/deallocated", want: "deallocated"},
				{after: time.Minute, state: ""},
				{after: time.Minute, state: "unrecognized"},
				{after: time.Minute, state: "PowerState/unknown", want: "unknown"},
			}
			for _, s := range steps {
				now = now.Add(s.after)
				rg.responseErr = s.discoveryErr
				rg.setResources([]map[string]any{testVirtualMachine("vm-a", "Standard_B2s", "Linux", s.state)})
				mx.queryResponse = azmetrics.QueryResourcesResponse{MetricResults: azmetrics.MetricResults{Values: []azmetrics.MetricData{{
					ResourceID: new("/subscriptions/sub-1/resourcegroups/rg-a/providers/microsoft.compute/virtualmachines/vm-a"),
					Values:     []azmetrics.Metric{metricWithAvg("Percentage CPU", now, 25)},
				}}}}
				series, err := collecttest.CollectScalarSeries(c, metrix.ReadFlatten())
				if tc.queryErr != nil && s.want == "" {
					require.ErrorContains(t, err, "all Azure Monitor batch queries failed")
					continue
				}
				require.NoError(t, err)
				want := s.want
				if tc.refresh == 0 {
					want = ""
					require.Equal(t, 1, rg.calls())
				}
				if want == "" {
					_, ok := seriesValue(series, "virtual_machines.power_state{")
					require.False(t, ok, "missing, unrecognized or stale state must be absent")
				} else {
					value, ok := seriesValue(series, "virtual_machines.power_state{", `virtual_machines.power_state="`+want+`"`)
					require.True(t, ok)
					require.Equal(t, metrix.SampleValue(1), value)
				}
				if tc.queryErr == nil {
					_, ok := seriesValue(series, "virtual_machines.percentage_cpu_average{")
					require.True(t, ok, "retained inventory must still collect ordinary metrics")
				}
			}
		})
	}
}

func TestCollector_ResourceStateChartContract(t *testing.T) {
	now := time.Date(2026, 10, 26, 12, 0, 0, 0, time.UTC)
	rg := &mockResourceGraph{resources: []map[string]any{testVirtualMachine("vm-a", "Standard_B2s", "Linux", "PowerState/running")}}
	c := newTestCollectorWithMocks(rg, &mockMetricsClient{})
	c.Config = testConfig()
	c.Config.Profiles.ModeExact = testProfilesModeEntries("virtual_machines")
	c.now = func() time.Time { return now }
	require.NoError(t, c.Init(context.Background()))
	require.NoError(t, c.Check(context.Background()))
	_, err := collecttest.CollectScalarSeries(c, metrix.ReadFlatten())
	require.NoError(t, err)
	engine, err := chartengine.New(chartengine.WithRuntimeStore(nil))
	require.NoError(t, err)
	require.NoError(t, engine.LoadYAML([]byte(c.ChartTemplateYAML()), 1))
	attempt, err := engine.PreparePlan(c.MetricStore().Read(metrix.ReadFlatten()))
	require.NoError(t, err)
	var chartID string
	var dimensions []string
	values := make(map[string]int64)
	for _, action := range attempt.Plan().Actions {
		switch action := action.(type) {
		case chartengine.CreateChartAction:
			require.Empty(t, chartID)
			require.True(t, strings.HasPrefix(action.ChartID, "am_azure_virtual_machines__power_state_"), action.ChartID)
			require.Equal(t, "azure_monitor.virtual_machines.power_state", action.Meta.Context)
			chartID = action.ChartID
		case chartengine.CreateDimensionAction:
			require.Equal(t, chartID, action.ChartID)
			dimensions = append(dimensions, action.Name)
		case chartengine.UpdateChartAction:
			require.Equal(t, chartID, action.ChartID)
			for _, value := range action.Values {
				require.False(t, value.IsEmpty)
				values[value.Name] = value.Int64
			}
		}
	}
	require.NotEmpty(t, chartID)
	require.ElementsMatch(t, []string{"running", "starting", "stopping", "stopped", "deallocating", "deallocated", "unknown"}, dimensions)
	require.Equal(t, map[string]int64{"running": 1, "starting": 0, "stopping": 0, "stopped": 0, "deallocating": 0, "deallocated": 0, "unknown": 0}, values)
	collecttest.AssertChartCoverage(t, c, collecttest.ChartCoverageExpectation{})
}

func TestCollector_CancellationDoesNotPublishResourceStates(t *testing.T) {
	rg := &mockResourceGraph{resources: []map[string]any{testVirtualMachine("vm-a", "Standard_B2s", "Linux", "PowerState/running")}}
	c := newTestCollectorWithMocks(rg, &mockMetricsClient{})
	c.Config = testConfig()
	c.Config.Profiles.ModeExact = testProfilesModeEntries("virtual_machines")
	require.NoError(t, c.Init(context.Background()))
	require.NoError(t, c.Check(context.Background()))
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	require.ErrorIs(t, c.Collect(ctx), context.Canceled)
}

func TestCollector_StateExpiresDuringMonitorQuery(t *testing.T) {
	for _, failed := range []bool{false, true} {
		name := "Monitor success"
		if failed {
			name = "Monitor failure"
		}
		t.Run(name, func(t *testing.T) {
			start := time.Date(2026, 10, 26, 12, 0, 0, 0, time.UTC)
			now := start
			rg := &mockResourceGraph{resources: []map[string]any{testVirtualMachine("vm-a", "Standard_B2s", "Linux", "PowerState/running")}}
			mx := &mockMetricsClient{onQuery: func() { now = start.Add(61 * time.Second) }}
			if failed {
				mx.queryErr = errors.New("Monitor unavailable")
			}
			c := newTestCollectorWithMocks(rg, mx)
			c.Config = testConfig()
			c.Config.Profiles.ModeExact = testProfilesModeEntries("virtual_machines")
			c.Config.Discovery.RefreshEvery = 60
			c.now = func() time.Time { return now }
			require.NoError(t, c.Init(context.Background()))
			require.NoError(t, c.Check(context.Background()))
			now = start.Add(59 * time.Second)
			mx.queryResponse = azmetrics.QueryResourcesResponse{MetricResults: azmetrics.MetricResults{Values: []azmetrics.MetricData{{
				ResourceID: new("/subscriptions/sub-1/resourcegroups/rg-a/providers/microsoft.compute/virtualmachines/vm-a"),
				Values:     []azmetrics.Metric{metricWithAvg("Percentage CPU", now, 25)},
			}}}}
			series, err := collecttest.CollectScalarSeries(c, metrix.ReadFlatten())
			require.Equal(t, 1, rg.calls(), "expiry must not trigger extra discovery polling")
			if failed {
				require.ErrorContains(t, err, "all Azure Monitor batch queries failed")
				return
			}
			require.NoError(t, err)
			_, ok := seriesValue(series, "virtual_machines.power_state{")
			require.False(t, ok, "state must still be valid when it is emitted")
			_, ok = seriesValue(series, "virtual_machines.percentage_cpu_average{")
			require.True(t, ok)
		})
	}
}

func TestCollector_ArbitraryResourceStateIDs(t *testing.T) {
	for _, id := range []string{"phase", "power", "lifecycle", "provisioning_state"} {
		t.Run(id, func(t *testing.T) {
			data, err := os.ReadFile("../../config/go.d/azure_monitor.profiles/default/virtual_machines.yaml")
			require.NoError(t, err)
			catalog := mustLoadStockCatalog(t, map[string]string{"vm_custom.yaml": strings.ReplaceAll(string(data), "power_state", id)})
			c := newTestCollectorWithMocks(&mockResourceGraph{resources: []map[string]any{testVirtualMachine("vm-a", "Standard_B2s", "Linux", "PowerState/running")}}, &mockMetricsClient{})
			c.loadProfileCatalog = func() (azureprofiles.Catalog, error) { return catalog, nil }
			c.Config = testConfig()
			c.Config.Profiles.ModeExact = testProfilesModeEntries("vm_custom")
			require.NoError(t, c.Init(context.Background()))
			require.NoError(t, c.Check(context.Background()))
			source, err := collectorapi.NewChartTemplateSource(c)
			require.NoError(t, err)
			_, err = source.Capture()
			require.NoError(t, err, "accepted profiles must compile through the production template adapter")
			series, err := collecttest.CollectScalarSeries(c, metrix.ReadFlatten())
			require.NoError(t, err)
			require.Len(t, series, 7)
			collecttest.AssertChartCoverage(t, c, collecttest.ChartCoverageExpectation{})
		})
	}
}

func TestCollector_InstrumentOwnershipRetiresHistory(t *testing.T) {
	for _, scenario := range []string{"metadata changes", "scope changes", "state metadata changes", "departed state resources"} {
		t.Run(scenario, func(t *testing.T) {
			now := time.Date(2026, 10, 26, 12, 0, 0, 0, time.UTC)
			rg, mx := &mockResourceGraph{}, &mockMetricsClient{}
			setResource := func(i int) {
				if scenario == "departed state resources" {
					if i >= 200 {
						rg.setResources(nil)
						return
					}
					rg.setResources([]map[string]any{testVirtualMachine(fmt.Sprintf("vm-%d", i), "Standard_B2s", "Linux", "PowerState/deallocated")})
					return
				}
				if i >= 200 {
					i = 199
				}
				if scenario == "state metadata changes" {
					rg.setResources([]map[string]any{testVirtualMachine("vm-a", fmt.Sprintf("Standard_%d", i), "Linux", "PowerState/running")})
					return
				}
				plan, scope := "plan-a", "worker"
				if scenario == "metadata changes" {
					plan = fmt.Sprintf("plan-%d", i)
				} else {
					scope = fmt.Sprintf("worker-%d", i)
				}
				rg.setResources([]map[string]any{testResource("Microsoft.Web/sites", "app-a", map[string]any{
					"nd_properties_serverFarmId": "/plans/" + plan,
					"tags":                       map[string]any{"workload": scope},
				})})
			}
			setResource(0)
			c := newTestCollectorWithMocks(rg, mx)
			c.Config = testConfig()
			c.Config.Discovery.RefreshEvery = 60
			c.Config.Profiles.ModeExact = testProfilesModeEntries("app_service")
			if scenario == "scope changes" {
				c.Config.VirtualNodes = &VirtualNodesConfig{ByResourceTag: "workload"}
			}
			if scenario == "departed state resources" || scenario == "state metadata changes" {
				c.Config.Profiles.ModeExact = testProfilesModeEntries("virtual_machines")
			}
			c.now = func() time.Time { return now }
			require.NoError(t, c.Init(context.Background()))
			require.NoError(t, c.Check(context.Background()))
			for i := range 400 {
				setResource(i)
				mx.queryResponse = appServiceResponse(now)
				scope := ""
				if scenario == "scope changes" {
					host, _ := workloadHostScope(resourceInfo{Tags: []resourceTag{{Key: "workload", Value: fmt.Sprintf("worker-%d", min(i, 199))}}}, "workload")
					scope = host.ScopeKey
				}
				series, err := collecttest.CollectScalarSeries(c, metrix.ReadFlatten(), metrix.ReadHostScope(scope))
				require.NoError(t, err)
				if scenario == "departed state resources" && i >= 200 {
					require.Empty(t, series)
				} else if scenario == "departed state resources" || scenario == "state metadata changes" {
					require.Len(t, series, 7)
					if scenario == "state metadata changes" {
						value, ok := seriesValue(series, "virtual_machines.power_state{", fmt.Sprintf(`vm_size="Standard_%d"`, min(i, 199)), `virtual_machines.power_state="running"`)
						require.True(t, ok)
						require.Equal(t, metrix.SampleValue(1), value)
					}
				} else {
					require.Len(t, series, 2)
					value, ok := seriesValue(series, "app_service.requests_total{")
					require.True(t, ok)
					total := (i + 1) * 10
					if scenario == "scope changes" {
						total = max(1, i-198) * 10
					}
					require.Equal(t, metrix.SampleValue(total), value)
				}
				now = now.Add(time.Minute)
			}
			want := 2
			if scenario == "departed state resources" {
				want = 0
			} else if scenario == "state metadata changes" {
				want = 1
			}
			require.Equal(t, want, retainedInstrumentHandles(c.runtime, c.observations), "handles must follow active membership, not historical labels/resources/scopes")
		})
	}
}

// Count actual retained handles, including handles hidden inside framework Vec caches.
// Stop at instruments and other framework objects so the store's independent retention
// does not contaminate the collector ownership measurement.
func retainedInstrumentHandles(roots ...any) int {
	seen := map[uintptr]bool{}
	count := 0
	var visit func(reflect.Value)
	visit = func(v reflect.Value) {
		if !v.IsValid() {
			return
		}
		if v.Kind() == reflect.Interface {
			if !v.IsNil() {
				visit(v.Elem())
			}
			return
		}
		if v.Kind() == reflect.Pointer {
			if v.IsNil() || seen[v.Pointer()] {
				return
			}
			seen[v.Pointer()] = true
			visit(v.Elem())
			return
		}
		if v.Type().PkgPath() == "github.com/netdata/netdata/go/plugins/pkg/metrix" {
			switch v.Type().Name() {
			case "snapshotGaugeInstrument", "snapshotCounterInstrument", "snapshotStateSetInstrument":
				count++
				return
			}
			if v.Kind() == reflect.Struct && (strings.HasSuffix(v.Type().Name(), "Vec") || strings.HasPrefix(v.Type().Name(), "vecCache[")) {
				visit(v.FieldByName("cache"))
			}
			return
		}
		switch v.Kind() {
		case reflect.Struct:
			for i := range v.NumField() {
				visit(v.Field(i))
			}
		case reflect.Map:
			iter := v.MapRange()
			for iter.Next() {
				visit(iter.Value())
			}
		case reflect.Slice, reflect.Array:
			for i := range v.Len() {
				visit(v.Index(i))
			}
		}
	}
	for _, root := range roots {
		visit(reflect.ValueOf(root))
	}
	return count
}
