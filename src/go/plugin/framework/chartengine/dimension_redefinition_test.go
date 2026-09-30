// SPDX-License-Identifier: GPL-3.0-or-later

package chartengine

import (
	"testing"

	"github.com/netdata/netdata/go/plugins/pkg/metrix"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestLiveDimensionDefinitionReconciliationIsTransactional(t *testing.T) {
	// Both routes render one dimension. Only one contributes in each cycle;
	// selecting the other changes the definition without any inactive cycle.
	const template = `
version: v1
groups:
  - family: Test
    metrics: [first, second]
    charts:
      - id: live
        title: Live dimension
        context: live
        units: value
        dimensions:
          - selector: first
            name_from_label: dimension
          - selector: second
            name_from_label: dimension
            options:
              multiplier: -1
              divisor: 1000
              hidden: true
`
	engine, err := New()
	require.NoError(t, err)
	require.NoError(t, engine.LoadYAML([]byte(template), 1))
	store := metrix.NewCollectorStore()
	cycle := mustCycleController(t, store)
	meter := store.Write().SnapshotMeter("")
	labels := meter.LabelSet(metrix.Label{Key: "dimension", Value: "value"})
	observe := func(second bool) {
		cycle.BeginCycle()
		if second {
			meter.Gauge("second", metrix.WithFloat(true)).Observe(7.5, labels)
		} else {
			meter.Gauge("first").Observe(7, labels)
		}
		require.NoError(t, cycle.CommitCycleSuccess())
	}
	observe(false)
	initial, err := buildPlan(engine, store.Read(metrix.ReadFlatten()))
	require.NoError(t, err)
	original := findOnlyCreateDimension(t, initial)
	require.False(t, original.Float)
	require.False(t, original.Hidden)

	observe(true)
	rejected, err := engine.PreparePlan(store.Read(metrix.ReadFlatten()))
	require.NoError(t, err)
	changed := findOnlyCreateDimension(t, rejected.Plan())
	assert.Equal(t, original.ChartID, changed.ChartID)
	assert.Equal(t, original.Name, changed.Name)
	assert.True(t, changed.Float)
	assert.True(t, changed.Hidden)
	assert.Equal(t, -1, changed.Multiplier)
	assert.Equal(t, 1000, changed.Divisor)
	rejected.Abort()

	observe(false)
	rolledBack, err := buildPlan(engine, store.Read(metrix.ReadFlatten()))
	require.NoError(t, err)
	require.Len(t, rolledBack.Actions, 1)
	assert.IsType(t, UpdateChartAction{}, rolledBack.Actions[0], "abort must retain the original definition")

	observe(true)
	retry, err := buildPlan(engine, store.Read(metrix.ReadFlatten()))
	require.NoError(t, err)
	assert.Equal(t, changed, findOnlyCreateDimension(t, retry))
	require.Len(t, retry.Actions, 2)
	assert.IsType(t, CreateDimensionAction{}, retry.Actions[0])
	assert.IsType(t, UpdateChartAction{}, retry.Actions[1])

	observe(true)
	stable, err := buildPlan(engine, store.Read(metrix.ReadFlatten()))
	require.NoError(t, err)
	require.Len(t, stable.Actions, 1)
	assert.IsType(t, UpdateChartAction{}, stable.Actions[0])
}
