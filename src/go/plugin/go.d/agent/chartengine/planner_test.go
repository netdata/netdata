// SPDX-License-Identifier: GPL-3.0-or-later

package chartengine

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/netdata/netdata/go/plugins/pkg/metrix"
	"github.com/netdata/netdata/go/plugins/plugin/go.d/agent/chartengine/internal/program"
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

			plan, err := e.BuildPlan(store.Read())
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

func TestBuildPlanUsesRouteCacheReuse(t *testing.T) {
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

	plan1, err := e.BuildPlan(store.Read())
	require.NoError(t, err)
	assert.Equal(t, []ActionKind{ActionCreateChart, ActionCreateDimension, ActionUpdateChart}, actionKinds(plan1.Actions))
	stats1 := e.Stats()
	assert.Equal(t, uint64(0), stats1.RouteCacheHits)
	assert.Equal(t, uint64(1), stats1.RouteCacheMisses)
	require.NotNil(t, findUpdateAction(plan1))
	assert.Equal(t, float64(10), findUpdateAction(plan1).Values[0].Float64)

	cc.BeginCycle()
	c.ObserveTotal(20)
	cc.CommitCycleSuccess()

	plan2, err := e.BuildPlan(store.Read())
	require.NoError(t, err)
	assert.Equal(t, []ActionKind{ActionUpdateChart}, actionKinds(plan2.Actions))
	stats2 := e.Stats()
	assert.Equal(t, uint64(1), stats2.RouteCacheHits)
	assert.Equal(t, uint64(1), stats2.RouteCacheMisses)
	require.NotNil(t, findUpdateAction(plan2))
	assert.Equal(t, float64(20), findUpdateAction(plan2).Values[0].Float64)
}

func TestBuildPlanLifecycleDimensionExpiry(t *testing.T) {
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

	plan1, err := e.BuildPlan(store.Read())
	require.NoError(t, err)
	assert.Equal(t, []ActionKind{ActionCreateChart, ActionCreateDimension, ActionCreateDimension, ActionUpdateChart}, actionKinds(plan1.Actions))

	cc.BeginCycle()
	total.Observe(101)
	cc.CommitCycleSuccess()

	plan2, err := e.BuildPlan(store.Read())
	require.NoError(t, err)
	assert.Equal(t, []ActionKind{ActionUpdateChart, ActionRemoveDimension}, actionKinds(plan2.Actions))
	removeDim := findRemoveDimensionAction(plan2)
	require.NotNil(t, removeDim)
	assert.Equal(t, "ok", removeDim.Name)
}

func TestBuildPlanLifecycleChartExpiry(t *testing.T) {
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

	plan1, err := e.BuildPlan(store.Read())
	require.NoError(t, err)
	assert.Equal(t, []ActionKind{ActionCreateChart, ActionCreateDimension, ActionUpdateChart}, actionKinds(plan1.Actions))

	cc.BeginCycle()
	cc.CommitCycleSuccess()

	plan2, err := e.BuildPlan(store.Read())
	require.NoError(t, err)
	assert.Equal(t, []ActionKind{ActionRemoveChart}, actionKinds(plan2.Actions))
}

func TestBuildPlanLifecycleNoRemovalOnFailedCycle(t *testing.T) {
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

	plan1, err := e.BuildPlan(store.Read())
	require.NoError(t, err)
	assert.Equal(t, []ActionKind{ActionCreateChart, ActionCreateDimension, ActionUpdateChart}, actionKinds(plan1.Actions))

	cc.BeginCycle()
	cc.AbortCycle()

	plan2, err := e.BuildPlan(store.Read())
	require.NoError(t, err)
	assert.Empty(t, plan2.Actions)

	cc.BeginCycle()
	cc.CommitCycleSuccess()

	plan3, err := e.BuildPlan(store.Read())
	require.NoError(t, err)
	assert.Equal(t, []ActionKind{ActionRemoveChart}, actionKinds(plan3.Actions))
}

func TestBuildPlanRendersChartIDsFromInstances(t *testing.T) {
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

	plan1, err := e.BuildPlan(store.Read())
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

	plan2, err := e.BuildPlan(store.Read())
	require.NoError(t, err)
	assert.Equal(t, []ActionKind{ActionUpdateChart, ActionUpdateChart}, actionKinds(plan2.Actions))
}

func TestBuildPlanEnforcesMaxInstancesDeterministically(t *testing.T) {
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

	plan1, err := e.BuildPlan(store.Read())
	require.NoError(t, err)
	assert.Equal(t, []ActionKind{ActionCreateChart, ActionCreateDimension, ActionUpdateChart}, actionKinds(plan1.Actions))

	cc.BeginCycle()
	rx.ObserveTotal(11, eth0)
	rx.ObserveTotal(20, eth1)
	cc.CommitCycleSuccess()

	plan2, err := e.BuildPlan(store.Read())
	require.NoError(t, err)
	// eth0 exists and is seen, so eth1 is dropped under max_instances=1.
	assert.Equal(t, []ActionKind{ActionUpdateChart}, actionKinds(plan2.Actions))
	update2 := findUpdateAction(plan2)
	require.NotNil(t, update2)
	assert.Equal(t, "win_nic_traffic_eth0", update2.ChartID)

	cc.BeginCycle()
	rx.ObserveTotal(21, eth1)
	cc.CommitCycleSuccess()

	plan3, err := e.BuildPlan(store.Read())
	require.NoError(t, err)
	assert.Equal(t, []ActionKind{
		ActionRemoveChart,
		ActionCreateChart,
		ActionCreateDimension,
		ActionUpdateChart,
	}, actionKinds(plan3.Actions))
}

func TestBuildPlanEnforcesMaxDimsDeterministically(t *testing.T) {
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

	plan1, err := e.BuildPlan(store.Read())
	require.NoError(t, err)
	assert.Equal(t, []ActionKind{ActionCreateChart, ActionCreateDimension, ActionCreateDimension, ActionUpdateChart}, actionKinds(plan1.Actions))

	cc.BeginCycle()
	g.Observe(1, modeA)
	g.Observe(1, modeB)
	g.Observe(1, modeC)
	cc.CommitCycleSuccess()

	plan2, err := e.BuildPlan(store.Read())
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

	plan3, err := e.BuildPlan(store.Read())
	require.NoError(t, err)
	assert.Equal(t, []ActionKind{
		ActionRemoveDimension,
		ActionCreateDimension,
		ActionUpdateChart,
	}, actionKinds(plan3.Actions))
}

func TestBuildPlanComputesChartLabelsIntersectionAndExclusions(t *testing.T) {
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

	plan, err := e.BuildPlan(store.Read())
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

func TestBuildPlanAutogenDisabledSkipsUnmatchedSeries(t *testing.T) {
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

	plan, err := e.BuildPlan(store.Read())
	require.NoError(t, err)
	assert.Empty(t, plan.Actions)
}

func TestBuildPlanAutogenCreatesChartForUnmatchedScalar(t *testing.T) {
	e, err := New(WithAutogenPolicy(AutogenPolicy{Enabled: true}))
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

	plan, err := e.BuildPlan(store.Read())
	require.NoError(t, err)

	assert.Equal(t, []ActionKind{ActionCreateChart, ActionCreateDimension, ActionUpdateChart}, actionKinds(plan.Actions))
	create := findCreateChartAction(plan)
	require.NotNil(t, create)
	assert.Equal(t, "svc.errors_total-method=GET", create.ChartID)
	assert.Equal(t, "autogen.svc.errors_total", create.Meta.Context)
	assert.Equal(t, "events/s", create.Meta.Units)
	assert.Equal(t, "GET", create.Labels["method"])
	update := findUpdateAction(plan)
	require.NotNil(t, update)
	assert.Equal(t, "svc.errors_total-method=GET", update.ChartID)
	require.Len(t, update.Values, 1)
	assert.Equal(t, "svc.errors_total", update.Values[0].Name)
	assert.Equal(t, float64(10), update.Values[0].Float64)
}

func TestBuildPlanTemplatePrecedenceOverAutogen(t *testing.T) {
	e, err := New(WithAutogenPolicy(AutogenPolicy{Enabled: true}))
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
	m := sm.Counter("requests_total")
	methodGET := sm.LabelSet(metrix.Label{Key: "method", Value: "GET"})

	cc.BeginCycle()
	m.ObserveTotal(10, methodGET)
	cc.CommitCycleSuccess()

	plan, err := e.BuildPlan(store.Read())
	require.NoError(t, err)
	assert.Equal(t, []ActionKind{ActionCreateChart, ActionCreateDimension, ActionUpdateChart}, actionKinds(plan.Actions))
	create := findCreateChartAction(plan)
	require.NotNil(t, create)
	assert.Equal(t, "svc_requests", create.ChartID)
	assert.NotEqual(t, "svc.requests_total-method=GET", create.ChartID)
}

func TestBuildPlanAutogenStrictOverflowDrop(t *testing.T) {
	e, err := New(WithAutogenPolicy(AutogenPolicy{
		Enabled:      true,
		TypeID:       "collector.job",
		MaxTypeIDLen: 32,
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
	metric := sm.Counter("this_metric_name_is_long_total")
	ls := sm.LabelSet(metrix.Label{Key: "tenant", Value: "a_very_long_tenant_name"})

	cc.BeginCycle()
	metric.ObserveTotal(10, ls)
	cc.CommitCycleSuccess()

	plan, err := e.BuildPlan(store.Read())
	require.NoError(t, err)
	assert.Empty(t, plan.Actions)
}

func TestBuildPlanAutogenUsesFlattenMetadataForHistogramBuckets(t *testing.T) {
	e, err := New(WithAutogenPolicy(AutogenPolicy{Enabled: true}))
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

	plan, err := e.BuildPlan(store.Read())
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

func TestBuildPlanAutogenCreatesChartForUnmatchedGauge(t *testing.T) {
	e, err := New(WithAutogenPolicy(AutogenPolicy{Enabled: true}))
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

	plan, err := e.BuildPlan(store.Read())
	require.NoError(t, err)

	assert.Equal(t, []ActionKind{ActionCreateChart, ActionCreateDimension, ActionUpdateChart}, actionKinds(plan.Actions))
	create := findCreateChartAction(plan)
	require.NotNil(t, create)
	assert.Equal(t, "svc.queue_depth-queue=main", create.ChartID)
	assert.Equal(t, "autogen.svc.queue_depth", create.Meta.Context)
	assert.Equal(t, "depth", create.Meta.Units)
	assert.Equal(t, program.AlgorithmAbsolute, create.Meta.Algorithm)

	update := findUpdateAction(plan)
	require.NotNil(t, update)
	require.Len(t, update.Values, 1)
	assert.Equal(t, "svc.queue_depth", update.Values[0].Name)
	assert.Equal(t, float64(7), update.Values[0].Float64)
}

func TestBuildPlanAutogenCreatesChartForUnmatchedStateSet(t *testing.T) {
	e, err := New(WithAutogenPolicy(AutogenPolicy{Enabled: true}))
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

	plan, err := e.BuildPlan(store.Read())
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
	assert.Equal(t, "autogen.svc.service_mode", create.Meta.Context)
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

func TestBuildPlanTemplateWinsOnAutogenChartIDCollisionAcrossSeries(t *testing.T) {
	e, err := New(WithAutogenPolicy(AutogenPolicy{Enabled: true}))
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

	plan, err := e.BuildPlan(store.Read())
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

func TestBuildPlanAutogenRemovalLifecycleExpiry(t *testing.T) {
	e, err := New(WithAutogenPolicy(AutogenPolicy{
		Enabled:                  true,
		ExpireAfterSuccessCycles: 1,
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
	c := store.Write().SnapshotMeter("svc").Counter("errors_total")

	cc.BeginCycle()
	c.ObserveTotal(10)
	cc.CommitCycleSuccess()

	plan1, err := e.BuildPlan(store.Read())
	require.NoError(t, err)
	assert.Equal(t, []ActionKind{ActionCreateChart, ActionCreateDimension, ActionUpdateChart}, actionKinds(plan1.Actions))

	cc.BeginCycle()
	cc.CommitCycleSuccess()

	plan2, err := e.BuildPlan(store.Read())
	require.NoError(t, err)
	assert.Equal(t, []ActionKind{ActionRemoveChart}, actionKinds(plan2.Actions))
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
