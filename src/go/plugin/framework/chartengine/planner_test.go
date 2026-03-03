// SPDX-License-Identifier: GPL-3.0-or-later

package chartengine

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/netdata/netdata/go/plugins/pkg/metrix"
	metrixselector "github.com/netdata/netdata/go/plugins/pkg/metrix/selector"
	"github.com/netdata/netdata/go/plugins/plugin/framework/chartengine/internal/program"
)

func TestInferDimensionLabelKeyScenarios(t *testing.T) {
	tests := map[string]struct {
		metricName string
		meta       metrix.SeriesMeta
		wantKey    string
		wantOK     bool
		wantErr    bool
	}{
		"histogram bucket uses le label": {
			meta: metrix.SeriesMeta{
				FlattenRole: metrix.FlattenRoleHistogramBucket,
			},
			wantKey: "le",
			wantOK:  true,
		},
		"summary quantile uses quantile label": {
			meta: metrix.SeriesMeta{
				FlattenRole: metrix.FlattenRoleSummaryQuantile,
			},
			wantKey: "quantile",
			wantOK:  true,
		},
		"stateset uses metric family name label": {
			metricName: "system_status",
			meta: metrix.SeriesMeta{
				FlattenRole: metrix.FlattenRoleStateSetState,
			},
			wantKey: "system_status",
			wantOK:  true,
		},
		"histogram count does not infer dynamic key": {
			meta: metrix.SeriesMeta{
				FlattenRole: metrix.FlattenRoleHistogramCount,
			},
			wantOK: false,
		},
		"non flattened role is an inference error": {
			meta: metrix.SeriesMeta{
				FlattenRole: metrix.FlattenRoleNone,
			},
			wantErr: true,
		},
	}

	for name, tc := range tests {
		t.Run(name, func(t *testing.T) {
			key, ok, err := inferDimensionLabelKey(tc.metricName, tc.meta)
			if tc.wantErr {
				require.Error(t, err)
				return
			}
			require.NoError(t, err)
			assert.Equal(t, tc.wantKey, key)
			assert.Equal(t, tc.wantOK, ok)
		})
	}
}

func TestBuildPlanResolvesInferDimensionNames(t *testing.T) {
	tests := map[string]struct {
		yaml      string
		setup     func(t *testing.T, s metrix.CollectorStore)
		wantNames []string
		wantKinds []ActionKind
	}{
		"histogram bucket inference resolves bucket names from le": {
			yaml: `
version: v1
groups:
  - family: Latency
    metrics:
      - svc.latency_seconds_bucket
    charts:
      - title: Latency buckets
        context: latency_bucket
        units: observations
        dimensions:
          - selector: svc.latency_seconds_bucket
`,
			setup: func(t *testing.T, s metrix.CollectorStore) {
				t.Helper()
				cc := mustCycleController(t, s)
				h := s.Write().SnapshotMeter("svc").Histogram("latency_seconds", metrix.WithHistogramBounds(1, 2))
				cc.BeginCycle()
				h.ObservePoint(metrix.HistogramPoint{
					Count: 2,
					Sum:   3,
					Buckets: []metrix.BucketPoint{
						{UpperBound: 1, CumulativeCount: 1},
						{UpperBound: 2, CumulativeCount: 2},
					},
				})
				cc.CommitCycleSuccess()
			},
			wantNames: []string{"+Inf", "1", "2"},
			wantKinds: []ActionKind{ActionCreateChart, ActionCreateDimension, ActionCreateDimension, ActionCreateDimension, ActionUpdateChart},
		},
		"summary quantile inference resolves quantile labels": {
			yaml: `
version: v1
groups:
  - family: Latency
    metrics:
      - svc.request_time
    charts:
      - title: Request time quantiles
        context: request_time_quantile
        units: seconds
        dimensions:
          - selector: svc.request_time{quantile=~".+"}
`,
			setup: func(t *testing.T, s metrix.CollectorStore) {
				t.Helper()
				cc := mustCycleController(t, s)
				sm := s.Write().SnapshotMeter("svc")
				sum := sm.Summary("request_time", metrix.WithSummaryQuantiles(0.5, 0.9))

				cc.BeginCycle()
				sum.ObservePoint(metrix.SummaryPoint{
					Count: 10,
					Sum:   8.8,
					Quantiles: []metrix.QuantilePoint{
						{Quantile: 0.5, Value: 0.4},
						{Quantile: 0.9, Value: 1.2},
					},
				})
				cc.CommitCycleSuccess()
			},
			wantNames: []string{"0.5", "0.9"},
			wantKinds: []ActionKind{ActionCreateChart, ActionCreateDimension, ActionCreateDimension, ActionUpdateChart},
		},
		"stateset inference resolves state names from metric-family label key": {
			yaml: `
version: v1
groups:
  - family: Service
    metrics:
      - system.status
    charts:
      - title: System status
        context: system_status
        units: state
        dimensions:
          - selector: system.status
`,
			setup: func(t *testing.T, s metrix.CollectorStore) {
				t.Helper()
				cc := mustCycleController(t, s)
				ss := s.Write().SnapshotMeter("system").StateSet(
					"status",
					metrix.WithStateSetStates("ok", "failed"),
					metrix.WithStateSetMode(metrix.ModeEnum),
				)
				cc.BeginCycle()
				ss.Enable("ok")
				cc.CommitCycleSuccess()
			},
			wantNames: []string{"failed", "ok"},
			wantKinds: []ActionKind{ActionCreateChart, ActionCreateDimension, ActionCreateDimension, ActionUpdateChart},
		},
	}

	for name, tc := range tests {
		t.Run(name, func(t *testing.T) {
			e, err := New()
			require.NoError(t, err)
			require.NoError(t, e.LoadYAML([]byte(tc.yaml), 1))

			store := metrix.NewCollectorStore()
			tc.setup(t, store)

			plan, err := e.BuildPlan(store.Read(metrix.ReadFlatten()))
			require.NoError(t, err)

			got := make([]string, 0, len(plan.InferredDimensions))
			for _, dim := range plan.InferredDimensions {
				got = append(got, dim.Name)
			}
			assert.Equal(t, tc.wantNames, got)
			assert.Equal(t, tc.wantKinds, actionKinds(plan.Actions))
		})
	}
}

func TestBuildPlanLegacySingleScenarioCases(t *testing.T) {
	tests := map[string]struct {
		run func(t *testing.T)
	}{
		"BuildPlanRequiresFlattenedReaderForInference":                 {run: runTestBuildPlanRequiresFlattenedReaderForInference},
		"BuildPlanUsesRouteCacheReuse":                                 {run: runTestBuildPlanUsesRouteCacheReuse},
		"BuildPlanLifecycleDimensionExpiry":                            {run: runTestBuildPlanLifecycleDimensionExpiry},
		"BuildPlanLifecycleChartExpiry":                                {run: runTestBuildPlanLifecycleChartExpiry},
		"BuildPlanLifecycleNoRemovalOnFailedCycle":                     {run: runTestBuildPlanLifecycleNoRemovalOnFailedCycle},
		"BuildPlanRendersChartIDsFromInstances":                        {run: runTestBuildPlanRendersChartIDsFromInstances},
		"BuildPlanEnforcesMaxInstancesDeterministically":               {run: runTestBuildPlanEnforcesMaxInstancesDeterministically},
		"BuildPlanEnforcesMaxDimsDeterministically":                    {run: runTestBuildPlanEnforcesMaxDimsDeterministically},
		"BuildPlanComputesChartLabelsIntersectionAndExclusions":        {run: runTestBuildPlanComputesChartLabelsIntersectionAndExclusions},
		"BuildPlanAutogenDisabledSkipsUnmatchedSeries":                 {run: runTestBuildPlanAutogenDisabledSkipsUnmatchedSeries},
		"BuildPlanEnginePolicySelectorFiltersSeriesBeforeRouting":      {run: runTestBuildPlanEnginePolicySelectorFiltersSeriesBeforeRouting},
		"BuildPlanTemplateEnginePolicyControlsSelectorAndAutogen":      {run: runTestBuildPlanTemplateEnginePolicyControlsSelectorAndAutogen},
		"BuildPlanEnginePolicyOptionOverridesTemplatePolicy":           {run: runTestBuildPlanEnginePolicyOptionOverridesTemplatePolicy},
		"BuildPlanAutogenOptionKeepsTemplateSelector":                  {run: runTestBuildPlanAutogenOptionKeepsTemplateSelector},
		"BuildPlanAutogenCreatesChartForUnmatchedScalar":               {run: runTestBuildPlanAutogenCreatesChartForUnmatchedScalar},
		"BuildPlanAutogenUsesMetricMetadataForScalar":                  {run: runTestBuildPlanAutogenUsesMetricMetadataForScalar},
		"BuildPlanAutogenUsesMetricMetadataForHistogram":               {run: runTestBuildPlanAutogenUsesMetricMetadataForHistogram},
		"BuildPlanAutogenUsesMetricFloatMetadataForScalar":             {run: runTestBuildPlanAutogenUsesMetricFloatMetadataForScalar},
		"BuildPlanAutogenUsesMetricMetadataForSummaryWithoutQuantiles": {run: runTestBuildPlanAutogenUsesMetricMetadataForSummaryWithoutQuantiles},
		"BuildPlanTemplatePrecedenceOverAutogen":                       {run: runTestBuildPlanTemplatePrecedenceOverAutogen},
		"BuildPlanAutogenStrictOverflowDrop":                           {run: runTestBuildPlanAutogenStrictOverflowDrop},
		"BuildPlanAutogenUsesFlattenMetadataForHistogramBuckets":       {run: runTestBuildPlanAutogenUsesFlattenMetadataForHistogramBuckets},
		"BuildPlanAutogenCreatesChartForUnmatchedGauge":                {run: runTestBuildPlanAutogenCreatesChartForUnmatchedGauge},
		"BuildPlanAutogenCreatesChartForUnmatchedStateSet":             {run: runTestBuildPlanAutogenCreatesChartForUnmatchedStateSet},
		"BuildPlanAutogenKeepsStateSetUnitsWhenMetricMetaUnitIsSet":    {run: runTestBuildPlanAutogenKeepsStateSetUnitsWhenMetricMetaUnitIsSet},
		"BuildPlanTemplateWinsOnAutogenChartIDCollisionAcrossSeries":   {run: runTestBuildPlanTemplateWinsOnAutogenChartIDCollisionAcrossSeries},
		"BuildPlanAutogenRemovalLifecycleExpiry":                       {run: runTestBuildPlanAutogenRemovalLifecycleExpiry},
		"BuildPlanFirstWriterWinsAndAccumulatesRepeatedRoutes":         {run: runTestBuildPlanFirstWriterWinsAndAccumulatesRepeatedRoutes},
		"BuildPlanEmptyEmissionAndScratchReusePruneAcrossCycles":       {run: runTestBuildPlanEmptyEmissionAndScratchReusePruneAcrossCycles},
	}

	for name, tc := range tests {
		t.Run(name, tc.run)
	}
}

func runTestBuildPlanRequiresFlattenedReaderForInference(t *testing.T) {
	e, err := New()
	require.NoError(t, err)

	yaml := `
version: v1
groups:
  - family: Service
    metrics:
      - system.status
    charts:
      - title: System status
        context: system_status
        units: state
        dimensions:
          - selector: system.status
`
	require.NoError(t, e.LoadYAML([]byte(yaml), 1))

	store := metrix.NewCollectorStore()
	cc := mustCycleController(t, store)
	ss := store.Write().SnapshotMeter("system").StateSet(
		"status",
		metrix.WithStateSetStates("ok", "failed"),
		metrix.WithStateSetMode(metrix.ModeEnum),
	)

	cc.BeginCycle()
	ss.Enable("ok")
	cc.CommitCycleSuccess()

	_, err = e.BuildPlan(store.Read())
	require.Error(t, err)
	assert.ErrorContains(t, err, "Read(metrix.ReadFlatten())")

	_, err = e.BuildPlan(store.Read(metrix.ReadFlatten()))
	require.NoError(t, err)
}

func runTestBuildPlanUsesRouteCacheReuse(t *testing.T) {
	e, err := New()
	require.NoError(t, err)

	yaml := `
version: v1
groups:
  - family: Service
    metrics:
      - svc.requests_total
    charts:
      - title: Requests
        context: requests
        units: requests/s
        dimensions:
          - selector: svc.requests_total
            name: total
`
	require.NoError(t, e.LoadYAML([]byte(yaml), 1))

	store := metrix.NewCollectorStore()
	cc := mustCycleController(t, store)
	c := store.Write().SnapshotMeter("svc").Counter("requests_total")

	cc.BeginCycle()
	c.ObserveTotal(10)
	cc.CommitCycleSuccess()

	plan1, err := e.BuildPlan(store.Read(metrix.ReadFlatten()))
	require.NoError(t, err)
	assert.Equal(t, []ActionKind{ActionCreateChart, ActionCreateDimension, ActionUpdateChart}, actionKinds(plan1.Actions))
	stats1 := e.stats()
	assert.Equal(t, uint64(0), stats1.RouteCacheHits)
	assert.Equal(t, uint64(1), stats1.RouteCacheMisses)
	require.NotNil(t, findUpdateAction(plan1))
	assert.Equal(t, float64(10), findUpdateAction(plan1).Values[0].Float64)

	cc.BeginCycle()
	c.ObserveTotal(20)
	cc.CommitCycleSuccess()

	plan2, err := e.BuildPlan(store.Read(metrix.ReadFlatten()))
	require.NoError(t, err)
	assert.Equal(t, []ActionKind{ActionUpdateChart}, actionKinds(plan2.Actions))
	stats2 := e.stats()
	assert.Equal(t, uint64(1), stats2.RouteCacheHits)
	assert.Equal(t, uint64(1), stats2.RouteCacheMisses)
	require.NotNil(t, findUpdateAction(plan2))
	assert.Equal(t, float64(20), findUpdateAction(plan2).Values[0].Float64)
}

func runTestBuildPlanLifecycleDimensionExpiry(t *testing.T) {
	e, err := New()
	require.NoError(t, err)

	yaml := `
version: v1
groups:
  - family: Service
    metrics:
      - svc.total
      - svc.mode_metric
    charts:
      - title: Service status
        context: service_status
        units: state
        lifecycle:
          dimensions:
            expire_after_cycles: 1
        dimensions:
          - selector: svc.total
            name: total
          - selector: svc.mode_metric
            name_from_label: mode
`
	require.NoError(t, e.LoadYAML([]byte(yaml), 1))

	store := metrix.NewCollectorStore()
	cc := mustCycleController(t, store)
	sm := store.Write().SnapshotMeter("svc")
	total := sm.Gauge("total")
	modeMetric := sm.Gauge("mode_metric")
	modeOK := sm.LabelSet(metrix.Label{Key: "mode", Value: "ok"})

	cc.BeginCycle()
	total.Observe(100)
	modeMetric.Observe(1, modeOK)
	cc.CommitCycleSuccess()

	plan1, err := e.BuildPlan(store.Read(metrix.ReadFlatten()))
	require.NoError(t, err)
	assert.Equal(t, []ActionKind{ActionCreateChart, ActionCreateDimension, ActionCreateDimension, ActionUpdateChart}, actionKinds(plan1.Actions))

	cc.BeginCycle()
	total.Observe(101)
	cc.CommitCycleSuccess()

	plan2, err := e.BuildPlan(store.Read(metrix.ReadFlatten()))
	require.NoError(t, err)
	assert.Equal(t, []ActionKind{ActionUpdateChart, ActionRemoveDimension}, actionKinds(plan2.Actions))
	removeDim := findRemoveDimensionAction(plan2)
	require.NotNil(t, removeDim)
	assert.Equal(t, "ok", removeDim.Name)
}

func runTestBuildPlanLifecycleChartExpiry(t *testing.T) {
	e, err := New()
	require.NoError(t, err)

	yaml := `
version: v1
groups:
  - family: Service
    metrics:
      - svc.requests_total
    charts:
      - title: Requests
        context: requests
        units: requests/s
        lifecycle:
          expire_after_cycles: 1
        dimensions:
          - selector: svc.requests_total
            name: total
`
	require.NoError(t, e.LoadYAML([]byte(yaml), 1))

	store := metrix.NewCollectorStore()
	cc := mustCycleController(t, store)
	c := store.Write().SnapshotMeter("svc").Counter("requests_total")

	cc.BeginCycle()
	c.ObserveTotal(10)
	cc.CommitCycleSuccess()

	plan1, err := e.BuildPlan(store.Read(metrix.ReadFlatten()))
	require.NoError(t, err)
	assert.Equal(t, []ActionKind{ActionCreateChart, ActionCreateDimension, ActionUpdateChart}, actionKinds(plan1.Actions))

	cc.BeginCycle()
	cc.CommitCycleSuccess()

	plan2, err := e.BuildPlan(store.Read(metrix.ReadFlatten()))
	require.NoError(t, err)
	assert.Equal(t, []ActionKind{ActionRemoveChart}, actionKinds(plan2.Actions))
}

func runTestBuildPlanLifecycleNoRemovalOnFailedCycle(t *testing.T) {
	e, err := New()
	require.NoError(t, err)

	yaml := `
version: v1
groups:
  - family: Service
    metrics:
      - svc.requests_total
    charts:
      - title: Requests
        context: requests
        units: requests/s
        lifecycle:
          expire_after_cycles: 1
        dimensions:
          - selector: svc.requests_total
            name: total
`
	require.NoError(t, e.LoadYAML([]byte(yaml), 1))

	store := metrix.NewCollectorStore()
	cc := mustCycleController(t, store)
	c := store.Write().SnapshotMeter("svc").Counter("requests_total")

	cc.BeginCycle()
	c.ObserveTotal(10)
	cc.CommitCycleSuccess()

	plan1, err := e.BuildPlan(store.Read(metrix.ReadFlatten()))
	require.NoError(t, err)
	assert.Equal(t, []ActionKind{ActionCreateChart, ActionCreateDimension, ActionUpdateChart}, actionKinds(plan1.Actions))

	cc.BeginCycle()
	cc.AbortCycle()

	plan2, err := e.BuildPlan(store.Read(metrix.ReadFlatten()))
	require.NoError(t, err)
	assert.Empty(t, plan2.Actions)

	cc.BeginCycle()
	cc.CommitCycleSuccess()

	plan3, err := e.BuildPlan(store.Read(metrix.ReadFlatten()))
	require.NoError(t, err)
	assert.Equal(t, []ActionKind{ActionRemoveChart}, actionKinds(plan3.Actions))
}

func runTestBuildPlanRendersChartIDsFromInstances(t *testing.T) {
	e, err := New()
	require.NoError(t, err)

	yaml := `
version: v1
groups:
  - family: Net
    metrics:
      - windows_net_bytes_received_total
    charts:
      - id: win_nic_traffic
        title: NIC traffic
        context: nic_traffic
        units: bytes/s
        instances:
          by_labels: [nic]
        dimensions:
          - selector: windows_net_bytes_received_total
            name: received
`
	require.NoError(t, e.LoadYAML([]byte(yaml), 1))

	store := metrix.NewCollectorStore()
	cc := mustCycleController(t, store)
	sm := store.Write().SnapshotMeter("")
	rx := sm.Counter("windows_net_bytes_received_total")

	eth0 := sm.LabelSet(metrix.Label{Key: "nic", Value: "eth0"})
	eth1 := sm.LabelSet(metrix.Label{Key: "nic", Value: "eth1"})

	cc.BeginCycle()
	rx.ObserveTotal(10, eth1)
	rx.ObserveTotal(20, eth0)
	cc.CommitCycleSuccess()

	plan1, err := e.BuildPlan(store.Read(metrix.ReadFlatten()))
	require.NoError(t, err)
	assert.Equal(t, []ActionKind{
		ActionCreateChart, ActionCreateDimension, ActionUpdateChart,
		ActionCreateChart, ActionCreateDimension, ActionUpdateChart,
	}, actionKinds(plan1.Actions))

	createChartIDs := make([]string, 0, 2)
	createChartLabels := make(map[string]map[string]string, 2)
	updateChartIDs := make([]string, 0, 2)
	for _, action := range plan1.Actions {
		switch v := action.(type) {
		case CreateChartAction:
			createChartIDs = append(createChartIDs, v.ChartID)
			createChartLabels[v.ChartID] = v.Labels
		case UpdateChartAction:
			updateChartIDs = append(updateChartIDs, v.ChartID)
		}
	}
	assert.Equal(t, []string{"win_nic_traffic_eth0", "win_nic_traffic_eth1"}, createChartIDs)
	assert.Equal(t, "eth0", createChartLabels["win_nic_traffic_eth0"]["nic"])
	assert.Equal(t, "eth1", createChartLabels["win_nic_traffic_eth1"]["nic"])
	assert.Equal(t, []string{"win_nic_traffic_eth0", "win_nic_traffic_eth1"}, updateChartIDs)

	cc.BeginCycle()
	rx.ObserveTotal(21, eth0)
	rx.ObserveTotal(11, eth1)
	cc.CommitCycleSuccess()

	plan2, err := e.BuildPlan(store.Read(metrix.ReadFlatten()))
	require.NoError(t, err)
	assert.Equal(t, []ActionKind{ActionUpdateChart, ActionUpdateChart}, actionKinds(plan2.Actions))
}

func runTestBuildPlanEnforcesMaxInstancesDeterministically(t *testing.T) {
	e, err := New()
	require.NoError(t, err)

	yaml := `
version: v1
groups:
  - family: Net
    metrics:
      - windows_net_bytes_received_total
    charts:
      - id: win_nic_traffic
        title: NIC traffic
        context: nic_traffic
        units: bytes/s
        lifecycle:
          max_instances: 1
        instances:
          by_labels: [nic]
        dimensions:
          - selector: windows_net_bytes_received_total
            name: received
`
	require.NoError(t, e.LoadYAML([]byte(yaml), 1))

	store := metrix.NewCollectorStore()
	cc := mustCycleController(t, store)
	sm := store.Write().SnapshotMeter("")
	rx := sm.Counter("windows_net_bytes_received_total")

	eth0 := sm.LabelSet(metrix.Label{Key: "nic", Value: "eth0"})
	eth1 := sm.LabelSet(metrix.Label{Key: "nic", Value: "eth1"})

	cc.BeginCycle()
	rx.ObserveTotal(10, eth0)
	cc.CommitCycleSuccess()

	plan1, err := e.BuildPlan(store.Read(metrix.ReadFlatten()))
	require.NoError(t, err)
	assert.Equal(t, []ActionKind{ActionCreateChart, ActionCreateDimension, ActionUpdateChart}, actionKinds(plan1.Actions))

	cc.BeginCycle()
	rx.ObserveTotal(11, eth0)
	rx.ObserveTotal(20, eth1)
	cc.CommitCycleSuccess()

	plan2, err := e.BuildPlan(store.Read(metrix.ReadFlatten()))
	require.NoError(t, err)
	// eth0 exists and is seen, so eth1 is dropped under max_instances=1.
	assert.Equal(t, []ActionKind{ActionUpdateChart}, actionKinds(plan2.Actions))
	update2 := findUpdateAction(plan2)
	require.NotNil(t, update2)
	assert.Equal(t, "win_nic_traffic_eth0", update2.ChartID)

	cc.BeginCycle()
	rx.ObserveTotal(21, eth1)
	cc.CommitCycleSuccess()

	plan3, err := e.BuildPlan(store.Read(metrix.ReadFlatten()))
	require.NoError(t, err)
	assert.Equal(t, []ActionKind{
		ActionRemoveChart,
		ActionCreateChart,
		ActionCreateDimension,
		ActionUpdateChart,
	}, actionKinds(plan3.Actions))
}

func runTestBuildPlanEnforcesMaxDimsDeterministically(t *testing.T) {
	e, err := New()
	require.NoError(t, err)

	yaml := `
version: v1
groups:
  - family: Service
    metrics:
      - svc_mode
    charts:
      - id: service_mode
        title: Service mode
        context: service_mode
        units: state
        lifecycle:
          dimensions:
            max_dims: 2
        dimensions:
          - selector: svc_mode
            name_from_label: mode
`
	require.NoError(t, e.LoadYAML([]byte(yaml), 1))

	store := metrix.NewCollectorStore()
	cc := mustCycleController(t, store)
	sm := store.Write().SnapshotMeter("")
	g := sm.Gauge("svc_mode")

	modeA := sm.LabelSet(metrix.Label{Key: "mode", Value: "a"})
	modeB := sm.LabelSet(metrix.Label{Key: "mode", Value: "b"})
	modeC := sm.LabelSet(metrix.Label{Key: "mode", Value: "c"})

	cc.BeginCycle()
	g.Observe(1, modeA)
	g.Observe(1, modeB)
	cc.CommitCycleSuccess()

	plan1, err := e.BuildPlan(store.Read(metrix.ReadFlatten()))
	require.NoError(t, err)
	assert.Equal(t, []ActionKind{ActionCreateChart, ActionCreateDimension, ActionCreateDimension, ActionUpdateChart}, actionKinds(plan1.Actions))

	cc.BeginCycle()
	g.Observe(1, modeA)
	g.Observe(1, modeB)
	g.Observe(1, modeC)
	cc.CommitCycleSuccess()

	plan2, err := e.BuildPlan(store.Read(metrix.ReadFlatten()))
	require.NoError(t, err)
	// a,b seen; c is dropped under max_dims=2.
	assert.Equal(t, []ActionKind{ActionUpdateChart}, actionKinds(plan2.Actions))
	update2 := findUpdateAction(plan2)
	require.NotNil(t, update2)
	assert.Len(t, update2.Values, 2)

	cc.BeginCycle()
	g.Observe(1, modeB)
	g.Observe(1, modeC)
	cc.CommitCycleSuccess()

	plan3, err := e.BuildPlan(store.Read(metrix.ReadFlatten()))
	require.NoError(t, err)
	assert.Equal(t, []ActionKind{
		ActionRemoveDimension,
		ActionCreateDimension,
		ActionUpdateChart,
	}, actionKinds(plan3.Actions))
}

func runTestBuildPlanComputesChartLabelsIntersectionAndExclusions(t *testing.T) {
	e, err := New()
	require.NoError(t, err)

	yaml := `
version: v1
groups:
  - family: Net
    metrics:
      - windows_net_bytes
    charts:
      - id: win_nic_traffic
        title: NIC traffic
        context: nic_traffic
        units: bytes/s
        instances:
          by_labels: [nic]
        dimensions:
          - selector: windows_net_bytes{direction="in"}
            name: received
          - selector: windows_net_bytes{direction="out"}
            name: sent
`
	require.NoError(t, e.LoadYAML([]byte(yaml), 1))

	store := metrix.NewCollectorStore()
	cc := mustCycleController(t, store)
	sm := store.Write().SnapshotMeter("")
	m := sm.Counter("windows_net_bytes")
	in := sm.LabelSet(
		metrix.Label{Key: "nic", Value: "eth0"},
		metrix.Label{Key: "direction", Value: "in"},
		metrix.Label{Key: "interface_type", Value: "ethernet"},
	)
	out := sm.LabelSet(
		metrix.Label{Key: "nic", Value: "eth0"},
		metrix.Label{Key: "direction", Value: "out"},
		metrix.Label{Key: "interface_type", Value: "ethernet"},
	)

	cc.BeginCycle()
	m.ObserveTotal(10, in)
	m.ObserveTotal(20, out)
	cc.CommitCycleSuccess()

	plan, err := e.BuildPlan(store.Read(metrix.ReadFlatten()))
	require.NoError(t, err)

	var create *CreateChartAction
	for _, action := range plan.Actions {
		if v, ok := action.(CreateChartAction); ok {
			create = &v
			break
		}
	}
	require.NotNil(t, create)
	assert.Equal(t, "eth0", create.Labels["nic"])
	assert.Equal(t, "ethernet", create.Labels["interface_type"])
	_, hasDirection := create.Labels["direction"]
	assert.False(t, hasDirection)
}

func runTestBuildPlanAutogenDisabledSkipsUnmatchedSeries(t *testing.T) {
	e, err := New()
	require.NoError(t, err)

	yaml := `
version: v1
groups:
  - family: Service
    metrics:
      - svc.requests_total
    charts:
      - title: Requests
        context: requests
        units: requests/s
        dimensions:
          - selector: svc.requests_total
            name: total
`
	require.NoError(t, e.LoadYAML([]byte(yaml), 1))

	store := metrix.NewCollectorStore()
	cc := mustCycleController(t, store)
	unmatched := store.Write().SnapshotMeter("svc").Counter("errors_total")

	cc.BeginCycle()
	unmatched.ObserveTotal(10)
	cc.CommitCycleSuccess()

	plan, err := e.BuildPlan(store.Read(metrix.ReadFlatten()))
	require.NoError(t, err)
	assert.Empty(t, plan.Actions)
}

func runTestBuildPlanEnginePolicySelectorFiltersSeriesBeforeRouting(t *testing.T) {
	selectorExpr := metrixselector.Expr{
		Allow: []string{`svc.errors_total{method="GET"}`},
	}
	e, err := New(WithEnginePolicy(EnginePolicy{
		Selector: &selectorExpr,
		Autogen:  &AutogenPolicy{Enabled: true},
	}))
	require.NoError(t, err)

	yaml := `
version: v1
groups:
  - family: Service
    metrics:
      - svc.requests_total
    charts:
      - title: Requests
        context: requests
        units: requests/s
        dimensions:
          - selector: svc.requests_total
            name: total
`
	require.NoError(t, e.LoadYAML([]byte(yaml), 1))

	store := metrix.NewCollectorStore()
	cc := mustCycleController(t, store)
	sm := store.Write().SnapshotMeter("svc")
	unmatched := sm.Counter("errors_total")
	methodGET := sm.LabelSet(metrix.Label{Key: "method", Value: "GET"})
	methodPOST := sm.LabelSet(metrix.Label{Key: "method", Value: "POST"})

	cc.BeginCycle()
	unmatched.ObserveTotal(10, methodGET)
	unmatched.ObserveTotal(20, methodPOST)
	cc.CommitCycleSuccess()

	plan, err := e.BuildPlan(store.Read(metrix.ReadFlatten()))
	require.NoError(t, err)

	assert.Equal(t, []ActionKind{ActionCreateChart, ActionCreateDimension, ActionUpdateChart}, actionKinds(plan.Actions))
	create := findCreateChartAction(plan)
	require.NotNil(t, create)
	assert.Equal(t, "svc.errors_total-method=GET", create.ChartID)
	assert.Equal(t, "GET", create.Labels["method"])

	update := findUpdateAction(plan)
	require.NotNil(t, update)
	require.Len(t, update.Values, 1)
	assert.Equal(t, float64(10), update.Values[0].Float64)
}

func runTestBuildPlanTemplateEnginePolicyControlsSelectorAndAutogen(t *testing.T) {
	e, err := New()
	require.NoError(t, err)

	yaml := `
version: v1
engine:
  selector:
    allow:
      - svc.errors_total{method="GET"}
  autogen:
    enabled: true
groups:
  - family: Service
`
	require.NoError(t, e.LoadYAML([]byte(yaml), 1))

	store := metrix.NewCollectorStore()
	cc := mustCycleController(t, store)
	sm := store.Write().SnapshotMeter("svc")
	unmatched := sm.Counter("errors_total")
	methodGET := sm.LabelSet(metrix.Label{Key: "method", Value: "GET"})
	methodPOST := sm.LabelSet(metrix.Label{Key: "method", Value: "POST"})

	cc.BeginCycle()
	unmatched.ObserveTotal(10, methodGET)
	unmatched.ObserveTotal(20, methodPOST)
	cc.CommitCycleSuccess()

	plan, err := e.BuildPlan(store.Read(metrix.ReadFlatten()))
	require.NoError(t, err)

	assert.Equal(t, []ActionKind{ActionCreateChart, ActionCreateDimension, ActionUpdateChart}, actionKinds(plan.Actions))
	create := findCreateChartAction(plan)
	require.NotNil(t, create)
	assert.Equal(t, "svc.errors_total-method=GET", create.ChartID)
}

func runTestBuildPlanEnginePolicyOptionOverridesTemplatePolicy(t *testing.T) {
	overrideSelector := metrixselector.Expr{
		Allow: []string{`svc.errors_total{method="POST"}`},
	}
	e, err := New(WithEnginePolicy(EnginePolicy{
		Selector: &overrideSelector,
		Autogen:  &AutogenPolicy{Enabled: true},
	}))
	require.NoError(t, err)

	yaml := `
version: v1
engine:
  selector:
    allow:
      - svc.errors_total{method="GET"}
  autogen:
    enabled: false
groups:
  - family: Service
`
	require.NoError(t, e.LoadYAML([]byte(yaml), 1))

	store := metrix.NewCollectorStore()
	cc := mustCycleController(t, store)
	sm := store.Write().SnapshotMeter("svc")
	unmatched := sm.Counter("errors_total")
	methodGET := sm.LabelSet(metrix.Label{Key: "method", Value: "GET"})
	methodPOST := sm.LabelSet(metrix.Label{Key: "method", Value: "POST"})

	cc.BeginCycle()
	unmatched.ObserveTotal(10, methodGET)
	unmatched.ObserveTotal(20, methodPOST)
	cc.CommitCycleSuccess()

	plan, err := e.BuildPlan(store.Read(metrix.ReadFlatten()))
	require.NoError(t, err)

	assert.Equal(t, []ActionKind{ActionCreateChart, ActionCreateDimension, ActionUpdateChart}, actionKinds(plan.Actions))
	create := findCreateChartAction(plan)
	require.NotNil(t, create)
	assert.Equal(t, "svc.errors_total-method=POST", create.ChartID)
}

func runTestBuildPlanAutogenOptionKeepsTemplateSelector(t *testing.T) {
	e, err := New(WithEnginePolicy(EnginePolicy{Autogen: &AutogenPolicy{Enabled: true}}))
	require.NoError(t, err)

	yaml := `
version: v1
engine:
  selector:
    allow:
      - svc.errors_total{method="GET"}
  autogen:
    enabled: false
groups:
  - family: Service
`
	require.NoError(t, e.LoadYAML([]byte(yaml), 1))

	store := metrix.NewCollectorStore()
	cc := mustCycleController(t, store)
	sm := store.Write().SnapshotMeter("svc")
	unmatched := sm.Counter("errors_total")
	methodGET := sm.LabelSet(metrix.Label{Key: "method", Value: "GET"})
	methodPOST := sm.LabelSet(metrix.Label{Key: "method", Value: "POST"})

	cc.BeginCycle()
	unmatched.ObserveTotal(10, methodGET)
	unmatched.ObserveTotal(20, methodPOST)
	cc.CommitCycleSuccess()

	plan, err := e.BuildPlan(store.Read(metrix.ReadFlatten()))
	require.NoError(t, err)

	assert.Equal(t, []ActionKind{ActionCreateChart, ActionCreateDimension, ActionUpdateChart}, actionKinds(plan.Actions))
	create := findCreateChartAction(plan)
	require.NotNil(t, create)
	assert.Equal(t, "svc.errors_total-method=GET", create.ChartID)
}

func runTestBuildPlanAutogenCreatesChartForUnmatchedScalar(t *testing.T) {
	e, err := New(WithEnginePolicy(EnginePolicy{Autogen: &AutogenPolicy{Enabled: true}}))
	require.NoError(t, err)

	yaml := `
version: v1
groups:
  - family: Service
    metrics:
      - svc.requests_total
    charts:
      - title: Requests
        context: requests
        units: requests/s
        dimensions:
          - selector: svc.requests_total
            name: total
`
	require.NoError(t, e.LoadYAML([]byte(yaml), 1))

	store := metrix.NewCollectorStore()
	cc := mustCycleController(t, store)
	sm := store.Write().SnapshotMeter("svc")
	unmatched := sm.Counter("errors_total")
	methodGET := sm.LabelSet(metrix.Label{Key: "method", Value: "GET"})

	cc.BeginCycle()
	unmatched.ObserveTotal(10, methodGET)
	cc.CommitCycleSuccess()

	plan, err := e.BuildPlan(store.Read(metrix.ReadFlatten()))
	require.NoError(t, err)

	assert.Equal(t, []ActionKind{ActionCreateChart, ActionCreateDimension, ActionUpdateChart}, actionKinds(plan.Actions))
	create := findCreateChartAction(plan)
	require.NotNil(t, create)
	assert.Equal(t, "svc.errors_total-method=GET", create.ChartID)
	assert.Equal(t, "svc.errors_total", create.Meta.Context)
	assert.Equal(t, "events/s", create.Meta.Units)
	assert.Equal(t, "GET", create.Labels["method"])
	update := findUpdateAction(plan)
	require.NotNil(t, update)
	assert.Equal(t, "svc.errors_total-method=GET", update.ChartID)
	require.Len(t, update.Values, 1)
	assert.Equal(t, "errors_total", update.Values[0].Name)
	assert.Equal(t, float64(10), update.Values[0].Float64)
}

func runTestBuildPlanAutogenUsesMetricMetadataForScalar(t *testing.T) {
	e, err := New(WithEnginePolicy(EnginePolicy{Autogen: &AutogenPolicy{Enabled: true}}))
	require.NoError(t, err)

	yaml := `
version: v1
groups:
  - family: Service
    metrics:
      - svc.requests_total
    charts:
      - title: Requests
        context: requests
        units: requests/s
        dimensions:
          - selector: svc.requests_total
            name: total
`
	require.NoError(t, e.LoadYAML([]byte(yaml), 1))

	store := metrix.NewCollectorStore()
	cc := mustCycleController(t, store)
	unmatched := store.Write().SnapshotMeter("svc").Counter(
		"bytes_total",
		metrix.WithDescription("HTTP traffic"),
		metrix.WithChartFamily("Traffic"),
		metrix.WithUnit("bytes"),
	)

	cc.BeginCycle()
	unmatched.ObserveTotal(10)
	cc.CommitCycleSuccess()

	plan, err := e.BuildPlan(store.Read(metrix.ReadFlatten()))
	require.NoError(t, err)

	create := findCreateChartAction(plan)
	require.NotNil(t, create)
	assert.Equal(t, "HTTP traffic", create.Meta.Title)
	assert.Equal(t, "Traffic", create.Meta.Family)
	assert.Equal(t, "bytes/s", create.Meta.Units)
}

func runTestBuildPlanAutogenUsesMetricMetadataForHistogram(t *testing.T) {
	e, err := New(WithEnginePolicy(EnginePolicy{Autogen: &AutogenPolicy{Enabled: true}}))
	require.NoError(t, err)

	yaml := `
version: v1
groups:
  - family: Service
    metrics:
      - svc.requests_total
    charts:
      - title: Requests
        context: requests
        units: requests/s
        dimensions:
          - selector: svc.requests_total
            name: total
`
	require.NoError(t, e.LoadYAML([]byte(yaml), 1))

	store := metrix.NewCollectorStore()
	cc := mustCycleController(t, store)
	h := store.Write().SnapshotMeter("svc").Histogram(
		"request_duration_ms",
		metrix.WithHistogramBounds(1, 2),
		metrix.WithDescription("Request duration"),
		metrix.WithChartFamily("Latency"),
		metrix.WithUnit("ms"),
	)

	cc.BeginCycle()
	h.ObservePoint(metrix.HistogramPoint{
		Count: 3,
		Sum:   5,
		Buckets: []metrix.BucketPoint{
			{UpperBound: 1, CumulativeCount: 1},
			{UpperBound: 2, CumulativeCount: 3},
		},
	})
	cc.CommitCycleSuccess()

	plan, err := e.BuildPlan(store.Read(metrix.ReadFlatten()))
	require.NoError(t, err)

	buckets := findCreateChartActionByID(plan, "svc.request_duration_ms")
	require.NotNil(t, buckets)
	assert.Equal(t, "Request duration", buckets.Meta.Title)
	assert.Equal(t, "Latency", buckets.Meta.Family)
	assert.Equal(t, "observations/s", buckets.Meta.Units)

	sum := findCreateChartActionByID(plan, "svc.request_duration_ms_sum")
	require.NotNil(t, sum)
	assert.Equal(t, "Request duration", sum.Meta.Title)
	assert.Equal(t, "Latency", sum.Meta.Family)
	assert.Equal(t, "ms/s", sum.Meta.Units)
}

func runTestBuildPlanAutogenUsesMetricFloatMetadataForScalar(t *testing.T) {
	e, err := New(WithEnginePolicy(EnginePolicy{Autogen: &AutogenPolicy{Enabled: true}}))
	require.NoError(t, err)

	yaml := `
version: v1
groups:
  - family: Service
    metrics:
      - svc.requests_total
    charts:
      - title: Requests
        context: requests
        units: requests/s
        dimensions:
          - selector: svc.requests_total
            name: total
`
	require.NoError(t, e.LoadYAML([]byte(yaml), 1))

	store := metrix.NewCollectorStore()
	cc := mustCycleController(t, store)
	unmatched := store.Write().SnapshotMeter("svc").Gauge(
		"temperature_celsius",
		metrix.WithFloat(true),
	)

	cc.BeginCycle()
	unmatched.Observe(10.5)
	cc.CommitCycleSuccess()

	plan, err := e.BuildPlan(store.Read(metrix.ReadFlatten()))
	require.NoError(t, err)

	var created *CreateDimensionAction
	for _, action := range plan.Actions {
		dim, ok := action.(CreateDimensionAction)
		if !ok || dim.ChartID != "svc.temperature_celsius" {
			continue
		}
		created = &dim
		break
	}
	require.NotNil(t, created)
	assert.True(t, created.Float)
	update := findUpdateAction(plan)
	require.NotNil(t, update)
	require.Len(t, update.Values, 1)
	assert.True(t, update.Values[0].IsFloat)
	assert.Equal(t, float64(10.5), update.Values[0].Float64)
}

func runTestBuildPlanAutogenUsesMetricMetadataForSummaryWithoutQuantiles(t *testing.T) {
	e, err := New(WithEnginePolicy(EnginePolicy{Autogen: &AutogenPolicy{Enabled: true}}))
	require.NoError(t, err)

	yaml := `
version: v1
groups:
  - family: Service
    metrics:
      - svc.requests_total
    charts:
      - title: Requests
        context: requests
        units: requests/s
        dimensions:
          - selector: svc.requests_total
            name: total
`
	require.NoError(t, e.LoadYAML([]byte(yaml), 1))

	store := metrix.NewCollectorStore()
	cc := mustCycleController(t, store)
	s := store.Write().SnapshotMeter("svc").Summary(
		"query_duration_ms",
		metrix.WithDescription("Query duration"),
		metrix.WithChartFamily("Latency"),
		metrix.WithUnit("ms"),
	)

	cc.BeginCycle()
	s.ObservePoint(metrix.SummaryPoint{
		Count: 4,
		Sum:   8,
	})
	cc.CommitCycleSuccess()

	plan, err := e.BuildPlan(store.Read(metrix.ReadFlatten()))
	require.NoError(t, err)

	sum := findCreateChartActionByID(plan, "svc.query_duration_ms_sum")
	require.NotNil(t, sum)
	assert.Equal(t, "Query duration", sum.Meta.Title)
	assert.Equal(t, "Latency", sum.Meta.Family)
	assert.Equal(t, "ms/s", sum.Meta.Units)
}

func runTestBuildPlanTemplatePrecedenceOverAutogen(t *testing.T) {
	e, err := New(WithEnginePolicy(EnginePolicy{Autogen: &AutogenPolicy{Enabled: true}}))
	require.NoError(t, err)

	yaml := `
version: v1
groups:
  - family: Service
    metrics:
      - svc.requests_total
    charts:
      - id: svc_requests
        title: Requests
        context: requests
        units: requests/s
        dimensions:
          - selector: svc.requests_total
            name: total
`
	require.NoError(t, e.LoadYAML([]byte(yaml), 1))

	store := metrix.NewCollectorStore()
	cc := mustCycleController(t, store)
	sm := store.Write().SnapshotMeter("svc")
	m := sm.Counter("requests_total", metrix.WithFloat(true))
	methodGET := sm.LabelSet(metrix.Label{Key: "method", Value: "GET"})

	cc.BeginCycle()
	m.ObserveTotal(10, methodGET)
	cc.CommitCycleSuccess()

	plan, err := e.BuildPlan(store.Read(metrix.ReadFlatten()))
	require.NoError(t, err)
	assert.Equal(t, []ActionKind{ActionCreateChart, ActionCreateDimension, ActionUpdateChart}, actionKinds(plan.Actions))
	create := findCreateChartAction(plan)
	require.NotNil(t, create)
	assert.Equal(t, "svc_requests", create.ChartID)
	assert.NotEqual(t, "svc.requests_total-method=GET", create.ChartID)
	var createdDim *CreateDimensionAction
	for _, action := range plan.Actions {
		dim, ok := action.(CreateDimensionAction)
		if !ok || dim.ChartID != "svc_requests" {
			continue
		}
		createdDim = &dim
		break
	}
	require.NotNil(t, createdDim)
	assert.False(t, createdDim.Float)
	update := findUpdateAction(plan)
	require.NotNil(t, update)
	require.Len(t, update.Values, 1)
	assert.False(t, update.Values[0].IsFloat)
	assert.Equal(t, int64(10), update.Values[0].Int64)
}

func runTestBuildPlanAutogenStrictOverflowDrop(t *testing.T) {
	e, err := New(
		WithEmitTypeIDBudgetPrefix("collector.job"),
		WithEnginePolicy(EnginePolicy{Autogen: &AutogenPolicy{
			Enabled:      true,
			MaxTypeIDLen: 32,
		}}),
	)
	require.NoError(t, err)

	yaml := `
version: v1
groups:
  - family: Service
    metrics:
      - svc.requests_total
    charts:
      - title: Requests
        context: requests
        units: requests/s
        dimensions:
          - selector: svc.requests_total
            name: total
`
	require.NoError(t, e.LoadYAML([]byte(yaml), 1))

	store := metrix.NewCollectorStore()
	cc := mustCycleController(t, store)
	sm := store.Write().SnapshotMeter("svc")
	metric := sm.Counter("this_metric_name_is_long_total")
	ls := sm.LabelSet(metrix.Label{Key: "tenant", Value: "a_very_long_tenant_name"})

	cc.BeginCycle()
	metric.ObserveTotal(10, ls)
	cc.CommitCycleSuccess()

	plan, err := e.BuildPlan(store.Read(metrix.ReadFlatten()))
	require.NoError(t, err)
	assert.Empty(t, plan.Actions)
}

func runTestBuildPlanAutogenUsesFlattenMetadataForHistogramBuckets(t *testing.T) {
	e, err := New(WithEnginePolicy(EnginePolicy{Autogen: &AutogenPolicy{Enabled: true}}))
	require.NoError(t, err)

	yaml := `
version: v1
groups:
  - family: Service
    metrics:
      - svc.requests_total
    charts:
      - title: Requests
        context: requests
        units: requests/s
        dimensions:
          - selector: svc.requests_total
            name: total
`
	require.NoError(t, e.LoadYAML([]byte(yaml), 1))

	store := metrix.NewCollectorStore()
	cc := mustCycleController(t, store)
	sm := store.Write().SnapshotMeter("svc")
	h := sm.Histogram("latency_seconds", metrix.WithHistogramBounds(1, 2))
	method := sm.LabelSet(metrix.Label{Key: "method", Value: "GET"})

	cc.BeginCycle()
	h.ObservePoint(metrix.HistogramPoint{
		Count: 3,
		Sum:   4,
		Buckets: []metrix.BucketPoint{
			{UpperBound: 1, CumulativeCount: 1},
			{UpperBound: 2, CumulativeCount: 3},
		},
	}, method)
	cc.CommitCycleSuccess()

	plan, err := e.BuildPlan(store.Read(metrix.ReadFlatten()))
	require.NoError(t, err)

	var bucketChart *CreateChartAction
	for _, action := range plan.Actions {
		create, ok := action.(CreateChartAction)
		if !ok {
			continue
		}
		if create.ChartID == "svc.latency_seconds-method=GET" {
			bucketChart = &create
			break
		}
	}
	require.NotNil(t, bucketChart)
	assert.Equal(t, "GET", bucketChart.Labels["method"])
	_, hasLE := bucketChart.Labels["le"]
	assert.False(t, hasLE)

	dims := map[string]struct{}{}
	for _, action := range plan.Actions {
		create, ok := action.(CreateDimensionAction)
		if !ok || create.ChartID != "svc.latency_seconds-method=GET" {
			continue
		}
		dims[create.Name] = struct{}{}
	}
	assert.Contains(t, dims, "bucket_1")
	assert.Contains(t, dims, "bucket_2")
	assert.Contains(t, dims, "bucket_+Inf")
}

func runTestBuildPlanAutogenCreatesChartForUnmatchedGauge(t *testing.T) {
	e, err := New(WithEnginePolicy(EnginePolicy{Autogen: &AutogenPolicy{Enabled: true}}))
	require.NoError(t, err)

	yaml := `
version: v1
groups:
  - family: Service
    metrics:
      - svc.requests_total
    charts:
      - title: Requests
        context: requests
        units: requests/s
        dimensions:
          - selector: svc.requests_total
            name: total
`
	require.NoError(t, e.LoadYAML([]byte(yaml), 1))

	store := metrix.NewCollectorStore()
	cc := mustCycleController(t, store)
	sm := store.Write().SnapshotMeter("svc")
	g := sm.Gauge("queue_depth")
	queueMain := sm.LabelSet(metrix.Label{Key: "queue", Value: "main"})

	cc.BeginCycle()
	g.Observe(7, queueMain)
	cc.CommitCycleSuccess()

	plan, err := e.BuildPlan(store.Read(metrix.ReadFlatten()))
	require.NoError(t, err)

	assert.Equal(t, []ActionKind{ActionCreateChart, ActionCreateDimension, ActionUpdateChart}, actionKinds(plan.Actions))
	create := findCreateChartAction(plan)
	require.NotNil(t, create)
	assert.Equal(t, "svc.queue_depth-queue=main", create.ChartID)
	assert.Equal(t, "svc.queue_depth", create.Meta.Context)
	assert.Equal(t, "depth", create.Meta.Units)
	assert.Equal(t, program.AlgorithmAbsolute, create.Meta.Algorithm)

	update := findUpdateAction(plan)
	require.NotNil(t, update)
	require.Len(t, update.Values, 1)
	assert.Equal(t, "queue_depth", update.Values[0].Name)
	assert.Equal(t, float64(7), update.Values[0].Float64)
}

func runTestBuildPlanAutogenCreatesChartForUnmatchedStateSet(t *testing.T) {
	e, err := New(WithEnginePolicy(EnginePolicy{Autogen: &AutogenPolicy{Enabled: true}}))
	require.NoError(t, err)

	yaml := `
version: v1
groups:
  - family: Service
    metrics:
      - svc.requests_total
    charts:
      - title: Requests
        context: requests
        units: requests/s
        dimensions:
          - selector: svc.requests_total
            name: total
`
	require.NoError(t, e.LoadYAML([]byte(yaml), 1))

	store := metrix.NewCollectorStore()
	cc := mustCycleController(t, store)
	sm := store.Write().SnapshotMeter("svc")
	ss := sm.StateSet("service_mode",
		metrix.WithStateSetStates("maintenance", "operational"),
		metrix.WithStateSetMode(metrix.ModeEnum),
	)

	cc.BeginCycle()
	ss.Enable("operational")
	cc.CommitCycleSuccess()

	plan, err := e.BuildPlan(store.Read(metrix.ReadFlatten()))
	require.NoError(t, err)

	assert.Equal(t, []ActionKind{
		ActionCreateChart,
		ActionCreateDimension,
		ActionCreateDimension,
		ActionUpdateChart,
	}, actionKinds(plan.Actions))
	create := findCreateChartAction(plan)
	require.NotNil(t, create)
	assert.Equal(t, "svc.service_mode", create.ChartID)
	assert.Equal(t, "svc.service_mode", create.Meta.Context)
	assert.Equal(t, "state", create.Meta.Units)
	_, hasStateLabel := create.Labels["svc.service_mode"]
	assert.False(t, hasStateLabel)

	dims := map[string]struct{}{}
	for _, action := range plan.Actions {
		dim, ok := action.(CreateDimensionAction)
		if !ok || dim.ChartID != "svc.service_mode" {
			continue
		}
		dims[dim.Name] = struct{}{}
	}
	assert.Contains(t, dims, "maintenance")
	assert.Contains(t, dims, "operational")
}

func runTestBuildPlanAutogenKeepsStateSetUnitsWhenMetricMetaUnitIsSet(t *testing.T) {
	e, err := New(WithEnginePolicy(EnginePolicy{Autogen: &AutogenPolicy{Enabled: true}}))
	require.NoError(t, err)

	yaml := `
version: v1
groups:
  - family: Service
    metrics:
      - svc.requests_total
    charts:
      - title: Requests
        context: requests
        units: requests/s
        dimensions:
          - selector: svc.requests_total
            name: total
`
	require.NoError(t, e.LoadYAML([]byte(yaml), 1))

	store := metrix.NewCollectorStore()
	cc := mustCycleController(t, store)
	ss := store.Write().SnapshotMeter("svc").StateSet(
		"service_mode",
		metrix.WithStateSetStates("maintenance", "operational"),
		metrix.WithDescription("Service mode"),
		metrix.WithChartFamily("Service"),
		metrix.WithUnit("watts"),
	)

	cc.BeginCycle()
	ss.Enable("operational")
	cc.CommitCycleSuccess()

	plan, err := e.BuildPlan(store.Read(metrix.ReadFlatten()))
	require.NoError(t, err)

	create := findCreateChartAction(plan)
	require.NotNil(t, create)
	assert.Equal(t, "svc.service_mode", create.ChartID)
	assert.Equal(t, "Service mode", create.Meta.Title)
	assert.Equal(t, "Service", create.Meta.Family)
	assert.Equal(t, "state", create.Meta.Units)
}

func runTestBuildPlanTemplateWinsOnAutogenChartIDCollisionAcrossSeries(t *testing.T) {
	e, err := New(WithEnginePolicy(EnginePolicy{Autogen: &AutogenPolicy{Enabled: true}}))
	require.NoError(t, err)

	yaml := `
version: v1
groups:
  - family: Service
    metrics:
      - svc.foo_total
    charts:
      - id: svc.errors_total-method=GET
        title: Foo requests
        context: foo_requests
        units: requests/s
        dimensions:
          - selector: svc.foo_total{method="GET"}
            name: total
`
	require.NoError(t, e.LoadYAML([]byte(yaml), 1))

	store := metrix.NewCollectorStore()
	cc := mustCycleController(t, store)
	sm := store.Write().SnapshotMeter("svc")
	errorsTotal := sm.Counter("errors_total")
	fooTotal := sm.Counter("foo_total")
	methodGET := sm.LabelSet(metrix.Label{Key: "method", Value: "GET"})

	cc.BeginCycle()
	errorsTotal.ObserveTotal(10, methodGET)
	fooTotal.ObserveTotal(7, methodGET)
	cc.CommitCycleSuccess()

	plan, err := e.BuildPlan(store.Read(metrix.ReadFlatten()))
	require.NoError(t, err)

	assert.Equal(t, []ActionKind{ActionCreateChart, ActionCreateDimension, ActionUpdateChart}, actionKinds(plan.Actions))
	create := findCreateChartAction(plan)
	require.NotNil(t, create)
	assert.Equal(t, "svc.errors_total-method=GET", create.ChartID)
	assert.Equal(t, "foo_requests", create.Meta.Context)

	update := findUpdateAction(plan)
	require.NotNil(t, update)
	require.Len(t, update.Values, 1)
	assert.Equal(t, "total", update.Values[0].Name)
	assert.Equal(t, float64(7), update.Values[0].Float64)
}

func runTestBuildPlanAutogenRemovalLifecycleExpiry(t *testing.T) {
	e, err := New(WithEnginePolicy(EnginePolicy{Autogen: &AutogenPolicy{
		Enabled:                  true,
		ExpireAfterSuccessCycles: 1,
	}}))
	require.NoError(t, err)

	yaml := `
version: v1
groups:
  - family: Service
    metrics:
      - svc.requests_total
    charts:
      - title: Requests
        context: requests
        units: requests/s
        dimensions:
          - selector: svc.requests_total
            name: total
`
	require.NoError(t, e.LoadYAML([]byte(yaml), 1))

	store := metrix.NewCollectorStore()
	cc := mustCycleController(t, store)
	c := store.Write().SnapshotMeter("svc").Counter("errors_total")

	cc.BeginCycle()
	c.ObserveTotal(10)
	cc.CommitCycleSuccess()

	plan1, err := e.BuildPlan(store.Read(metrix.ReadFlatten()))
	require.NoError(t, err)
	assert.Equal(t, []ActionKind{ActionCreateChart, ActionCreateDimension, ActionUpdateChart}, actionKinds(plan1.Actions))

	cc.BeginCycle()
	cc.CommitCycleSuccess()

	plan2, err := e.BuildPlan(store.Read(metrix.ReadFlatten()))
	require.NoError(t, err)
	assert.Equal(t, []ActionKind{ActionRemoveChart}, actionKinds(plan2.Actions))
}

func runTestBuildPlanFirstWriterWinsAndAccumulatesRepeatedRoutes(t *testing.T) {
	e, err := New()
	require.NoError(t, err)

	yaml := `
version: v1
groups:
  - family: Service
    metrics:
      - m_a
      - m_b
    charts:
      - id: conflict_total
        title: Conflict total
        context: conflict_total
        units: value
        dimensions:
          - selector: m_a
            name_from_label: mode
            options:
              hidden: true
              float: true
          - selector: m_b
            name_from_label: mode
            options:
              hidden: false
              float: false
`
	require.NoError(t, e.LoadYAML([]byte(yaml), 1))

	store := metrix.NewCollectorStore()
	cc := mustCycleController(t, store)
	m := store.Write().SnapshotMeter("")
	a := m.Gauge("m_a")
	b := m.Gauge("m_b")
	total := m.LabelSet(metrix.Label{Key: "mode", Value: "total"})

	cc.BeginCycle()
	a.Observe(5, total)
	b.Observe(3, total)
	cc.CommitCycleSuccess()

	plan, err := e.BuildPlan(store.Read())
	require.NoError(t, err)

	assert.Equal(t, []ActionKind{
		ActionCreateChart,
		ActionCreateDimension,
		ActionUpdateChart,
	}, actionKinds(plan.Actions))

	var created *CreateDimensionAction
	for _, action := range plan.Actions {
		dim, ok := action.(CreateDimensionAction)
		if !ok {
			continue
		}
		created = &dim
		break
	}
	require.NotNil(t, created)
	assert.Equal(t, "total", created.Name)
	assert.True(t, created.Hidden)
	assert.True(t, created.Float)

	update := findUpdateAction(plan)
	require.NotNil(t, update)
	require.Len(t, update.Values, 1)
	assert.True(t, update.Values[0].IsFloat)
	assert.Equal(t, "total", update.Values[0].Name)
	assert.Equal(t, float64(8), update.Values[0].Float64)
}

func runTestBuildPlanEmptyEmissionAndScratchReusePruneAcrossCycles(t *testing.T) {
	e, err := New()
	require.NoError(t, err)

	yaml := `
version: v1
groups:
  - family: Service
    metrics:
      - svc_mode
    charts:
      - id: service_mode
        title: Service mode
        context: service_mode
        units: state
        lifecycle:
          dimensions:
            expire_after_cycles: 1
        dimensions:
          - selector: svc_mode
            name_from_label: mode
`
	require.NoError(t, e.LoadYAML([]byte(yaml), 1))

	store := metrix.NewCollectorStore()
	cc := mustCycleController(t, store)
	m := store.Write().SnapshotMeter("")
	mode := m.Gauge("svc_mode")
	okSet := m.LabelSet(metrix.Label{Key: "mode", Value: "ok"})
	warnSet := m.LabelSet(metrix.Label{Key: "mode", Value: "warn"})

	cc.BeginCycle()
	mode.Observe(1, okSet)
	mode.Observe(2, warnSet)
	cc.CommitCycleSuccess()

	plan1, err := e.BuildPlan(store.Read())
	require.NoError(t, err)
	require.NotNil(t, findUpdateAction(plan1))

	matChart := e.state.materialized.charts["service_mode"]
	require.NotNil(t, matChart)
	require.Contains(t, matChart.scratchEntries, "ok")
	require.Contains(t, matChart.scratchEntries, "warn")
	okEntryPtr := matChart.scratchEntries["ok"]
	require.NotNil(t, okEntryPtr)

	cc.BeginCycle()
	mode.Observe(3, okSet)
	cc.CommitCycleSuccess()

	plan2, err := e.BuildPlan(store.Read())
	require.NoError(t, err)

	update2 := findUpdateAction(plan2)
	require.NotNil(t, update2)
	got := make(map[string]UpdateDimensionValue, len(update2.Values))
	for _, v := range update2.Values {
		got[v.Name] = v
	}
	require.Contains(t, got, "ok")
	require.Contains(t, got, "warn")
	assert.Equal(t, float64(3), got["ok"].Float64)
	assert.True(t, got["warn"].IsEmpty)
	require.NotNil(t, findRemoveDimensionAction(plan2))

	matChart = e.state.materialized.charts["service_mode"]
	require.NotNil(t, matChart)
	assert.NotContains(t, matChart.dimensions, "warn")
	require.Contains(t, matChart.scratchEntries, "warn")
	require.Contains(t, matChart.scratchEntries, "ok")
	assert.Equal(t, okEntryPtr, matChart.scratchEntries["ok"])

	cc.BeginCycle()
	mode.Observe(4, okSet)
	cc.CommitCycleSuccess()

	plan3, err := e.BuildPlan(store.Read())
	require.NoError(t, err)
	update3 := findUpdateAction(plan3)
	require.NotNil(t, update3)
	require.Len(t, update3.Values, 1)
	assert.Equal(t, "ok", update3.Values[0].Name)

	matChart = e.state.materialized.charts["service_mode"]
	require.NotNil(t, matChart)
	assert.NotContains(t, matChart.scratchEntries, "warn")
	require.Contains(t, matChart.scratchEntries, "ok")
}

func TestBuildPlanSequenceModeScenarios(t *testing.T) {
	tests := map[string]struct {
		run func(t *testing.T)
	}{
		"collector mode keeps static success-seq dedupe semantics": {
			run: func(t *testing.T) {
				e, err := New()
				require.NoError(t, err)
				require.NoError(t, e.LoadYAML([]byte(`
version: v1
groups:
  - family: Service
    metrics:
      - component.load
    charts:
      - id: component_load
        title: Component Load
        context: component_load
        units: load
        dimensions:
          - selector: component.load
            name: value
`), 1))

				store := metrix.NewCollectorStore()
				cc := mustCycleController(t, store)
				g := store.Write().SnapshotMeter("component").Gauge("load")

				cc.BeginCycle()
				g.Observe(5)
				cc.CommitCycleSuccess()

				plan1, err := e.BuildPlan(store.Read())
				require.NoError(t, err)
				require.NotNil(t, findUpdateAction(plan1))

				plan2, err := e.BuildPlan(store.Read())
				require.NoError(t, err)
				assert.Empty(t, plan2.Actions)
			},
		},
		"runtime mode re-emits updates on no-write ticks and keeps scratch entries": {
			run: func(t *testing.T) {
				e, err := New(WithSeriesSelectionAllVisible(), WithRuntimePlannerMode())
				require.NoError(t, err)
				require.NoError(t, e.LoadYAML([]byte(`
version: v1
groups:
  - family: Runtime
    metrics:
      - component.load
    charts:
      - id: component_load
        title: Component Load
        context: component_load
        units: load
        dimensions:
          - selector: component.load
            name_from_label: id
`), 1))

				store := metrix.NewRuntimeStore()
				vec := store.Write().StatefulMeter("component").Vec("id").Gauge("load")
				vec.WithLabelValues("ok").Set(1)
				vec.WithLabelValues("warn").Set(2)

				reader := store.Read(metrix.ReadRaw(), metrix.ReadFlatten())
				plan1, err := e.BuildPlan(reader)
				require.NoError(t, err)
				require.NotNil(t, findUpdateAction(plan1))

				matChart := e.state.materialized.charts["component_load"]
				require.NotNil(t, matChart)
				require.Contains(t, matChart.scratchEntries, "ok")
				require.Contains(t, matChart.scratchEntries, "warn")
				okEntry := matChart.scratchEntries["ok"]
				require.NotNil(t, okEntry)

				plan2, err := e.BuildPlan(reader)
				require.NoError(t, err)
				assert.Equal(t, []ActionKind{ActionUpdateChart}, actionKinds(plan2.Actions))
				require.NotNil(t, findUpdateAction(plan2))
				metricsReader := e.RuntimeStore().Read(metrix.ReadRaw())
				cacheHits, ok := metricsReader.Value("netdata.go.plugin.framework.chartengine.route_cache_hits_total", nil)
				require.True(t, ok)
				assert.GreaterOrEqual(t, cacheHits, float64(1))
				fullDrops, fullDropsSeen := metricsReader.Value("netdata.go.plugin.framework.chartengine.route_cache_full_drops_total", nil)
				require.True(t, fullDropsSeen)
				assert.Equal(t, float64(0), fullDrops)

				matChart = e.state.materialized.charts["component_load"]
				require.NotNil(t, matChart)
				require.Contains(t, matChart.scratchEntries, "ok")
				require.Contains(t, matChart.scratchEntries, "warn")
				assert.Equal(t, okEntry, matChart.scratchEntries["ok"])
			},
		},
	}

	for name, tc := range tests {
		t.Run(name, tc.run)
	}
}

func TestPlannerStageBoundaries(t *testing.T) {
	tests := map[string]func(t *testing.T){
		"scan stage accumulates per-chart state": func(t *testing.T) {
			e, err := New()
			require.NoError(t, err)

			yaml := `
version: v1
groups:
  - family: Service
    metrics:
      - svc_mode
    charts:
      - id: service_mode
        title: Service mode
        context: service_mode
        units: state
        dimensions:
          - selector: svc_mode
            name_from_label: mode
`
			require.NoError(t, e.LoadYAML([]byte(yaml), 1))

			store := metrix.NewCollectorStore()
			cc := mustCycleController(t, store)
			sm := store.Write().SnapshotMeter("")
			mode := sm.Gauge("svc_mode")
			a := sm.LabelSet(metrix.Label{Key: "mode", Value: "a"})
			b := sm.LabelSet(metrix.Label{Key: "mode", Value: "b"})

			cc.BeginCycle()
			mode.Observe(1, a)
			mode.Observe(2, b)
			cc.CommitCycleSuccess()

			out := Plan{
				Actions:            make([]EngineAction, 0),
				InferredDimensions: make([]InferredDimension, 0),
			}
			reader := store.Read()
			meta := reader.CollectMeta()
			ctx, err := e.preparePlanBuildContext(reader, &out, meta, meta.LastSuccessSeq)
			require.NoError(t, err)
			require.NoError(t, e.scanPlanSeries(ctx))

			require.Len(t, ctx.chartsByID, 1)
			cs, ok := ctx.chartsByID["service_mode"]
			require.True(t, ok)
			require.NotNil(t, cs.entries["a"])
			require.NotNil(t, cs.entries["b"])
			assert.Equal(t, metrix.SampleValue(1), cs.entries["a"].value)
			assert.Equal(t, metrix.SampleValue(2), cs.entries["b"].value)
			assert.Empty(t, out.InferredDimensions)
		},
		"materialize stage emits create and update actions": func(t *testing.T) {
			e, err := New()
			require.NoError(t, err)

			yaml := `
version: v1
groups:
  - family: Service
    metrics:
      - svc_mode
    charts:
      - id: service_mode
        title: Service mode
        context: service_mode
        units: state
        dimensions:
          - selector: svc_mode
            name_from_label: mode
`
			require.NoError(t, e.LoadYAML([]byte(yaml), 1))

			store := metrix.NewCollectorStore()
			cc := mustCycleController(t, store)
			sm := store.Write().SnapshotMeter("")
			mode := sm.Gauge("svc_mode")
			okLabel := sm.LabelSet(metrix.Label{Key: "mode", Value: "ok"})

			cc.BeginCycle()
			mode.Observe(1, okLabel)
			cc.CommitCycleSuccess()

			out := Plan{
				Actions:            make([]EngineAction, 0),
				InferredDimensions: make([]InferredDimension, 0),
			}
			reader := store.Read()
			meta := reader.CollectMeta()
			ctx, err := e.preparePlanBuildContext(reader, &out, meta, meta.LastSuccessSeq)
			require.NoError(t, err)
			require.NoError(t, e.scanPlanSeries(ctx))
			require.NoError(t, e.materializePlanCharts(ctx))

			assert.Equal(t, []ActionKind{
				ActionCreateChart,
				ActionCreateDimension,
				ActionUpdateChart,
			}, actionKinds(out.Actions))

			update := findUpdateAction(out)
			require.NotNil(t, update)
			require.Len(t, update.Values, 1)
			assert.Equal(t, "ok", update.Values[0].Name)
			assert.Equal(t, float64(1), update.Values[0].Float64)
		},
		"caps stage evicts deterministically": func(t *testing.T) {
			lifecycle := program.LifecyclePolicy{
				MaxInstances: 1,
			}
			meta := program.ChartMeta{
				Title:     "Requests",
				Context:   "requests",
				Family:    "Service",
				Units:     "requests/s",
				Algorithm: program.AlgorithmIncremental,
				Type:      program.ChartTypeLine,
			}
			chartsByID := map[string]*chartState{
				"svc_a": {
					templateID:      "tpl.requests",
					chartID:         "svc_a",
					meta:            meta,
					lifecycle:       lifecycle,
					currentBuildSeq: 2,
					observedCount:   1,
					entries: map[string]*dimBuildEntry{
						"total": {
							seenSeq: 2,
							value:   10,
							dimensionState: dimensionState{
								static: true,
								order:  0,
							},
						},
					},
				},
				"svc_b": {
					templateID:      "tpl.requests",
					chartID:         "svc_b",
					meta:            meta,
					lifecycle:       lifecycle,
					currentBuildSeq: 2,
					observedCount:   1,
					entries: map[string]*dimBuildEntry{
						"total": {
							seenSeq: 2,
							value:   20,
							dimensionState: dimensionState{
								static: true,
								order:  0,
							},
						},
					},
				},
			}

			state := newMaterializedState()
			oldChart, created := state.ensureChart("svc_old", "tpl.requests", meta, lifecycle)
			require.True(t, created)
			oldChart.lastSeenSuccessSeq = 1

			removeDims, removeCharts := enforceLifecycleCaps(2, chartsByID, &state)
			assert.Empty(t, removeDims)
			require.Len(t, removeCharts, 1)
			assert.Equal(t, "svc_old", removeCharts[0].ChartID)

			assert.Contains(t, chartsByID, "svc_a")
			assert.NotContains(t, chartsByID, "svc_b")
		},
		"expiry stage removes stale dimensions and charts": func(t *testing.T) {
			state := newMaterializedState()

			liveMeta := program.ChartMeta{
				Title:   "Service mode",
				Context: "service_mode",
			}
			liveChart, created := state.ensureChart("svc_mode", "tpl.mode", liveMeta, program.LifecyclePolicy{
				Dimensions: program.DimensionLifecyclePolicy{ExpireAfterCycles: 1},
			})
			require.True(t, created)
			liveChart.lastSeenSuccessSeq = 3
			liveDim, dimCreated := liveChart.ensureDimension("stale_mode", dimensionState{
				static:     false,
				order:      1,
				algorithm:  program.AlgorithmAbsolute,
				multiplier: 1,
				divisor:    1,
			})
			require.True(t, dimCreated)
			liveDim.lastSeenSuccessSeq = 1

			oldMeta := program.ChartMeta{
				Title:   "Old chart",
				Context: "old_chart",
			}
			oldChart, oldCreated := state.ensureChart("old_chart", "tpl.old", oldMeta, program.LifecyclePolicy{
				ExpireAfterCycles: 1,
			})
			require.True(t, oldCreated)
			oldChart.lastSeenSuccessSeq = 1

			removeDims, removeCharts := collectExpiryRemovals(3, &state)
			require.Len(t, removeDims, 1)
			assert.Equal(t, "svc_mode", removeDims[0].ChartID)
			assert.Equal(t, "stale_mode", removeDims[0].Name)

			require.Len(t, removeCharts, 1)
			assert.Equal(t, "old_chart", removeCharts[0].ChartID)
		},
	}

	for name, run := range tests {
		t.Run(name, run)
	}
}

func actionKinds(actions []EngineAction) []ActionKind {
	out := make([]ActionKind, 0, len(actions))
	for _, action := range actions {
		out = append(out, action.Kind())
	}
	return out
}

func findUpdateAction(plan Plan) *UpdateChartAction {
	for _, action := range plan.Actions {
		if update, ok := action.(UpdateChartAction); ok {
			return &update
		}
	}
	return nil
}

func findCreateChartAction(plan Plan) *CreateChartAction {
	for _, action := range plan.Actions {
		if create, ok := action.(CreateChartAction); ok {
			return &create
		}
	}
	return nil
}

func findCreateChartActionByID(plan Plan, chartID string) *CreateChartAction {
	for _, action := range plan.Actions {
		create, ok := action.(CreateChartAction)
		if !ok || create.ChartID != chartID {
			continue
		}
		return &create
	}
	return nil
}

func findRemoveDimensionAction(plan Plan) *RemoveDimensionAction {
	for _, action := range plan.Actions {
		if remove, ok := action.(RemoveDimensionAction); ok {
			return &remove
		}
	}
	return nil
}

func mustCycleController(t *testing.T, s metrix.CollectorStore) metrix.CycleController {
	t.Helper()
	managed, ok := metrix.AsCycleManagedStore(s)
	require.True(t, ok)
	return managed.CycleController()
}
