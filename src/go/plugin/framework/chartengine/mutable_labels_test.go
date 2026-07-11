// SPDX-License-Identifier: GPL-3.0-or-later

package chartengine

import (
	"fmt"
	"reflect"
	"testing"

	"github.com/netdata/netdata/go/plugins/pkg/metrix"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

const mutableLabelsTestTemplate = "version: v1\n" +
	"groups:\n" +
	"  - family: Service\n" +
	"    metrics: [service_value]\n" +
	"    charts:\n" +
	"      - id: service\n" +
	"        title: Service\n" +
	"        context: service.value\n" +
	"        units: value\n" +
	"        instances:\n" +
	"          by_labels: [instance]\n" +
	"        label_promotion: [owner, zone]\n" +
	"        dimensions:\n" +
	"          - selector: service_value\n" +
	"            name: value\n"

func TestMaterializedChartPresentationDoesNotOwnMutableLabelWorkspace(t *testing.T) {
	typ := reflect.TypeFor[materializedChartPresentation]()
	fields := make([]string, 0, typ.NumField())
	for i := range typ.NumField() {
		fields = append(fields, typ.Field(i).Name)
	}
	assert.Equal(t, []string{"orderedDims", "labelValues", "labelMembership"}, fields,
		"committed chart presentation must contain only semantic copy-on-write state")
}

func TestBuildPlanUpdatesMutableNonIdentityLabels(t *testing.T) {
	engine, store, cycle, meter, gauge := newMutableLabelsTestState(t)

	observeMutableLabels(t, cycle, meter, gauge, "node-1", "owner-a", "zone-a")
	initial, err := buildPlan(engine, store.Read(metrix.ReadFlatten()))
	require.NoError(t, err)
	create := findCreateChartAction(initial)
	require.NotNil(t, create)
	assert.Equal(t, map[string]string{
		"instance": "node-1",
		"owner":    "owner-a",
		"zone":     "zone-a",
	}, create.Labels)

	observeMutableLabels(t, cycle, meter, gauge, "node-1", "owner-b", "")
	changed, err := buildPlan(engine, store.Read(metrix.ReadFlatten()))
	require.NoError(t, err)
	assert.Equal(t, []ActionKind{ActionUpdateChartLabels, ActionUpdateChart}, actionKinds(changed.Actions))
	update := findUpdateChartLabelsAction(changed)
	require.NotNil(t, update)
	assert.Equal(t, create.ChartID, update.ChartID)
	assert.Equal(t, map[string]string{
		"instance": "node-1",
		"owner":    "owner-b",
	}, update.Labels)

	observeMutableLabels(t, cycle, meter, gauge, "node-1", "owner-b", "")
	unchanged, err := buildPlan(engine, store.Read(metrix.ReadFlatten()))
	require.NoError(t, err)
	assert.Equal(t, []ActionKind{ActionUpdateChart}, actionKinds(unchanged.Actions))
}

func TestBuildPlanIdentityLabelChangeCreatesNewChart(t *testing.T) {
	engine, store, cycle, meter, gauge := newMutableLabelsTestState(t)

	observeMutableLabels(t, cycle, meter, gauge, "node-1", "owner-a", "")
	initial, err := buildPlan(engine, store.Read(metrix.ReadFlatten()))
	require.NoError(t, err)
	initialCreate := findCreateChartAction(initial)
	require.NotNil(t, initialCreate)

	observeMutableLabels(t, cycle, meter, gauge, "node-2", "owner-a", "")
	changed, err := buildPlan(engine, store.Read(metrix.ReadFlatten()))
	require.NoError(t, err)
	assert.Equal(t, []ActionKind{ActionCreateChart, ActionCreateDimension, ActionUpdateChart}, actionKinds(changed.Actions))
	assert.Nil(t, findUpdateChartLabelsAction(changed))
	nextCreate := findCreateChartAction(changed)
	require.NotNil(t, nextCreate)
	assert.NotEqual(t, initialCreate.ChartID, nextCreate.ChartID)
}

func TestBuildPlanMutableLabelUpdateRetriesAfterAbort(t *testing.T) {
	engine, store, cycle, meter, gauge := newMutableLabelsTestState(t)

	observeMutableLabels(t, cycle, meter, gauge, "node-1", "owner-a", "")
	_, err := buildPlan(engine, store.Read(metrix.ReadFlatten()))
	require.NoError(t, err)

	observeMutableLabels(t, cycle, meter, gauge, "node-1", "owner-b", "")
	reader := store.Read(metrix.ReadFlatten())
	attempt, err := engine.PreparePlan(reader)
	require.NoError(t, err)
	require.NotNil(t, findUpdateChartLabelsAction(attempt.Plan()))
	assertCanonicalLabelMembership(t, engine, 1)
	attempt.Abort()

	retry, err := engine.PreparePlan(reader)
	require.NoError(t, err)
	require.NotNil(t, findUpdateChartLabelsAction(retry.Plan()))
	require.NoError(t, retry.Commit())

	observeMutableLabels(t, cycle, meter, gauge, "node-1", "owner-b", "")
	unchanged, err := buildPlan(engine, store.Read(metrix.ReadFlatten()))
	require.NoError(t, err)
	assert.Equal(t, []ActionKind{ActionUpdateChart}, actionKinds(unchanged.Actions))
}

func TestBuildPlanReconcilesLabelsWhenSeriesMembershipChanges(t *testing.T) {
	engine, store, cycle, meter, gauge := newMutableLabelsTestState(t)

	observeMutableLabelSeries(t, cycle, meter, gauge,
		mutableLabelSeries{instance: "node-1", owner: "owner-a", zone: "zone-a", shard: "shard-a"},
		mutableLabelSeries{instance: "node-1", owner: "owner-a", zone: "zone-b", shard: "shard-b"},
	)
	initial, err := buildPlan(engine, store.Read(metrix.ReadFlatten()))
	require.NoError(t, err)
	create := findCreateChartAction(initial)
	require.NotNil(t, create)
	assert.Equal(t, map[string]string{
		"instance": "node-1",
		"owner":    "owner-a",
	}, create.Labels)

	observeMutableLabelSeries(t, cycle, meter, gauge,
		mutableLabelSeries{instance: "node-1", owner: "owner-a", zone: "zone-a", shard: "shard-a"},
	)
	expanded, err := buildPlan(engine, store.Read(metrix.ReadFlatten()))
	require.NoError(t, err)
	update := findUpdateChartLabelsAction(expanded)
	require.NotNil(t, update)
	assert.Equal(t, map[string]string{
		"instance": "node-1",
		"owner":    "owner-a",
		"zone":     "zone-a",
	}, update.Labels)

	// The series identity changes, but the effective promoted labels do not.
	observeMutableLabelSeries(t, cycle, meter, gauge,
		mutableLabelSeries{instance: "node-1", owner: "owner-a", zone: "zone-a", shard: "shard-c"},
	)
	sameLabels, err := buildPlan(engine, store.Read(metrix.ReadFlatten()))
	require.NoError(t, err)
	assert.Equal(t, []ActionKind{ActionUpdateChart}, actionKinds(sameLabels.Actions))

	observeMutableLabelSeries(t, cycle, meter, gauge,
		mutableLabelSeries{instance: "node-1", owner: "owner-a", zone: "zone-a", shard: "shard-c"},
	)
	stable, err := buildPlan(engine, store.Read(metrix.ReadFlatten()))
	require.NoError(t, err)
	assert.Equal(t, []ActionKind{ActionUpdateChart}, actionKinds(stable.Actions))
}

func TestBuildPlanDimensionIdentityChangeCreatesNewDimension(t *testing.T) {
	engine, err := New(WithRuntimeStore(nil))
	require.NoError(t, err)
	require.NoError(t, engine.LoadYAML([]byte("version: v1\n"+
		"groups:\n"+
		"  - family: Service\n"+
		"    metrics: [service_value]\n"+
		"    charts:\n"+
		"      - id: service\n"+
		"        title: Service\n"+
		"        context: service.value\n"+
		"        units: value\n"+
		"        instances:\n"+
		"          by_labels: [instance]\n"+
		"        label_promotion: [owner]\n"+
		"        dimensions:\n"+
		"          - selector: service_value\n"+
		"            name_from_label: mode\n"), 1))

	store := metrix.NewCollectorStore()
	managed, ok := metrix.AsCycleManagedStore(store)
	require.True(t, ok)
	cycle := managed.CycleController()
	meter := store.Write().SnapshotMeter("")
	gauge := meter.Gauge("service_value")

	observeMutableDimension(t, cycle, meter, gauge, "read")
	initial, err := buildPlan(engine, store.Read(metrix.ReadFlatten()))
	require.NoError(t, err)
	create := findCreateChartAction(initial)
	require.NotNil(t, create)

	observeMutableDimension(t, cycle, meter, gauge, "write")
	changed, err := buildPlan(engine, store.Read(metrix.ReadFlatten()))
	require.NoError(t, err)
	assert.Equal(t, []ActionKind{ActionCreateDimension, ActionUpdateChart}, actionKinds(changed.Actions))
	assert.Nil(t, findUpdateChartLabelsAction(changed))
	assert.Nil(t, findCreateChartAction(changed))
	createDimension := changed.Actions[0].(CreateDimensionAction)
	assert.Equal(t, create.ChartID, createDimension.ChartID)
	assert.Equal(t, "write", createDimension.Name)
}

func TestBuildPlanMutableLabelsWithGenericReader(t *testing.T) {
	engine, store, cycle, meter, gauge := newMutableLabelsTestState(t)

	observeMutableLabels(t, cycle, meter, gauge, "node-1", "owner-a", "")
	_, err := buildPlan(engine, genericMutableLabelReader{Reader: store.Read(metrix.ReadFlatten())})
	require.NoError(t, err)

	observeMutableLabels(t, cycle, meter, gauge, "node-1", "owner-b", "")
	changed, err := buildPlan(engine, genericMutableLabelReader{Reader: store.Read(metrix.ReadFlatten())})
	require.NoError(t, err)
	update := findUpdateChartLabelsAction(changed)
	require.NotNil(t, update)
	assert.Equal(t, "owner-b", update.Labels["owner"])
}

func TestBuildPlanActionLabelsDoNotAliasCommittedState(t *testing.T) {
	engine, store, cycle, meter, gauge := newMutableLabelsTestState(t)

	observeMutableLabels(t, cycle, meter, gauge, "node-1", "owner-a", "")
	initial, err := buildPlan(engine, store.Read(metrix.ReadFlatten()))
	require.NoError(t, err)
	create := findCreateChartAction(initial)
	require.NotNil(t, create)
	create.Labels["owner"] = "owner-b"

	observeMutableLabels(t, cycle, meter, gauge, "node-1", "owner-b", "")
	changed, err := buildPlan(engine, store.Read(metrix.ReadFlatten()))
	require.NoError(t, err)
	update := findUpdateChartLabelsAction(changed)
	require.NotNil(t, update)
	assert.Equal(t, "owner-b", update.Labels["owner"])
	update.Labels["owner"] = "owner-c"

	observeMutableLabels(t, cycle, meter, gauge, "node-1", "owner-c", "")
	changedAgain, err := buildPlan(engine, store.Read(metrix.ReadFlatten()))
	require.NoError(t, err)
	updateAgain := findUpdateChartLabelsAction(changedAgain)
	require.NotNil(t, updateAgain)
	assert.Equal(t, "owner-c", updateAgain.Labels["owner"])
}

func TestBuildPlanCanonicalMembershipTracksCurrentCardinality(t *testing.T) {
	engine, store, cycle, meter, gauge := newMutableLabelsTestState(t)

	series := make([]mutableLabelSeries, 128)
	for i := range series {
		series[i] = mutableLabelSeries{
			instance: "node-1",
			owner:    "owner-a",
			zone:     "zone-a",
			shard:    fmt.Sprintf("shard-%03d", i),
		}
	}
	observeMutableLabelSeries(t, cycle, meter, gauge, series...)
	_, err := buildPlan(engine, store.Read(metrix.ReadFlatten()))
	require.NoError(t, err)
	assertCanonicalLabelMembership(t, engine, 128)

	observeMutableLabelSeries(t, cycle, meter, gauge, series[:100]...)
	_, err = buildPlan(engine, store.Read(metrix.ReadFlatten()))
	require.NoError(t, err)
	assertCanonicalLabelMembership(t, engine, 100)

	observeMutableLabelSeries(t, cycle, meter, gauge, series[0])
	_, err = buildPlan(engine, store.Read(metrix.ReadFlatten()))
	require.NoError(t, err)
	assertCanonicalLabelMembership(t, engine, 1)
}

func TestBuildPlanAbortedHighCardinalityProposalDoesNotEnterCommittedLabelState(t *testing.T) {
	engine, store, cycle, meter, gauge := newMutableLabelsTestState(t)

	series := make([]mutableLabelSeries, 128)
	for i := range series {
		series[i] = mutableLabelSeries{
			instance: "node-1",
			owner:    "owner-a",
			zone:     "zone-a",
			shard:    fmt.Sprintf("shard-%03d", i),
		}
	}
	observeMutableLabelSeries(t, cycle, meter, gauge, series[0])
	_, err := buildPlan(engine, store.Read(metrix.ReadFlatten()))
	require.NoError(t, err)
	assertCanonicalLabelMembership(t, engine, 1)

	observeMutableLabelSeries(t, cycle, meter, gauge, series...)
	attempt, err := engine.PreparePlan(store.Read(metrix.ReadFlatten()))
	require.NoError(t, err)
	assertCanonicalLabelMembership(t, engine, 1)
	staged := attempt.state.materialized.charts["service_node-1"]
	require.NotNil(t, staged)
	require.NotNil(t, staged.presentation)
	assert.Len(t, staged.presentation.labelMembership, 128)
	attempt.Abort()

	observeMutableLabelSeries(t, cycle, meter, gauge, series[0])
	retry, err := buildPlan(engine, store.Read(metrix.ReadFlatten()))
	require.NoError(t, err)
	assert.Equal(t, []ActionKind{ActionUpdateChart}, actionKinds(retry.Actions))
	assertCanonicalLabelMembership(t, engine, 1)
}

type mutableLabelSeries struct {
	instance string
	owner    string
	zone     string
	shard    string
}

type genericMutableLabelReader struct {
	metrix.Reader
}

func assertCanonicalLabelMembership(t *testing.T, engine *Engine, want int) {
	t.Helper()
	chart := engine.state.materialized.charts["service_node-1"]
	require.NotNil(t, chart)
	require.NotNil(t, chart.presentation)
	assert.Len(t, chart.presentation.labelMembership, want)
	assert.LessOrEqual(t, cap(chart.presentation.labelMembership), max(1, want*2))
	for _, membership := range chart.presentation.labelMembership[len(chart.presentation.labelMembership):cap(chart.presentation.labelMembership)] {
		assert.Empty(t, membership.seriesID)
		assert.Empty(t, membership.dimensionKeyLabel)
	}
}

func observeMutableLabelSeries(
	t *testing.T,
	cycle metrix.CycleController,
	meter metrix.SnapshotMeter,
	gauge metrix.SnapshotGauge,
	series ...mutableLabelSeries,
) {
	t.Helper()
	cycle.BeginCycle()
	for _, item := range series {
		gauge.Observe(1, meter.LabelSet(
			metrix.Label{Key: "instance", Value: item.instance},
			metrix.Label{Key: "owner", Value: item.owner},
			metrix.Label{Key: "zone", Value: item.zone},
			metrix.Label{Key: "shard", Value: item.shard},
		))
	}
	require.NoError(t, cycle.CommitCycleSuccess())
}

func observeMutableDimension(
	t *testing.T,
	cycle metrix.CycleController,
	meter metrix.SnapshotMeter,
	gauge metrix.SnapshotGauge,
	mode string,
) {
	t.Helper()
	cycle.BeginCycle()
	gauge.Observe(1, meter.LabelSet(
		metrix.Label{Key: "instance", Value: "node-1"},
		metrix.Label{Key: "owner", Value: "owner-a"},
		metrix.Label{Key: "mode", Value: mode},
	))
	require.NoError(t, cycle.CommitCycleSuccess())
}

func newMutableLabelsTestState(t *testing.T) (*Engine, metrix.CollectorStore, metrix.CycleController, metrix.SnapshotMeter, metrix.SnapshotGauge) {
	t.Helper()
	engine, err := New(WithRuntimeStore(nil))
	require.NoError(t, err)
	require.NoError(t, engine.LoadYAML([]byte(mutableLabelsTestTemplate), 1))

	store := metrix.NewCollectorStore()
	managed, ok := metrix.AsCycleManagedStore(store)
	require.True(t, ok)
	meter := store.Write().SnapshotMeter("")
	return engine, store, managed.CycleController(), meter, meter.Gauge("service_value")
}

func observeMutableLabels(t *testing.T, cycle metrix.CycleController, meter metrix.SnapshotMeter, gauge metrix.SnapshotGauge, instance, owner, zone string) {
	t.Helper()
	labels := []metrix.Label{
		{Key: "instance", Value: instance},
		{Key: "owner", Value: owner},
	}
	if zone != "" {
		labels = append(labels, metrix.Label{Key: "zone", Value: zone})
	}
	cycle.BeginCycle()
	gauge.Observe(1, meter.LabelSet(labels...))
	require.NoError(t, cycle.CommitCycleSuccess())
}

func findUpdateChartLabelsAction(plan Plan) *UpdateChartLabelsAction {
	for _, action := range plan.Actions {
		if update, ok := action.(UpdateChartLabelsAction); ok {
			return &update
		}
	}
	return nil
}
