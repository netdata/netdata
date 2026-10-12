// SPDX-License-Identifier: GPL-3.0-or-later

package chartengine_test

import (
	"bytes"
	"fmt"
	"testing"

	"github.com/netdata/netdata/go/plugins/pkg/metrix"
	"github.com/netdata/netdata/go/plugins/pkg/netdataapi"
	"github.com/netdata/netdata/go/plugins/plugin/framework/chartemit"
	"github.com/netdata/netdata/go/plugins/plugin/framework/chartengine"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestAuthoredChartExpiryAcrossMissingMeasurements(t *testing.T) {
	for name, tc := range map[string]struct {
		lifecycle string
		expiry    int
	}{
		"omitted lifecycle":    {expiry: 5},
		"empty lifecycle":      {lifecycle: "lifecycle: {}", expiry: 5},
		"omitted chart expiry": {lifecycle: "lifecycle: {dimensions: {max_dims: 2}}", expiry: 5},
		"disabled expiry":      {lifecycle: "lifecycle: {expire_after_cycles: 0}"},
		"positive expiry":      {lifecycle: "lifecycle: {expire_after_cycles: 2}", expiry: 2},
	} {
		for _, partial := range []bool{false, true} {
			t.Run(fmt.Sprintf("%s/partial=%t", name, partial), func(t *testing.T) {
				engine, err := chartengine.New()
				require.NoError(t, err)
				require.NoError(t, engine.LoadYAML([]byte(fmt.Sprintf(`
version: v1
context_namespace: test
groups:
  - family: Sensors
    metrics: [temperature]
    charts:
      - title: Temperature
        context: temperature
        units: Celsius
        instances:
          by_labels: [sensor]
        %s
        dimensions:
          - selector: temperature
            name: temperature
`, tc.lifecycle)), 1))
				store := metrix.NewCollectorStore()
				managed, ok := metrix.AsCycleManagedStore(store)
				require.True(t, ok)
				cycles := managed.CycleController()
				temperature := store.Write().SnapshotMeter("").Vec("sensor").Gauge("temperature", metrix.WithFloat(true))
				const recoveryCycle = 26 // Beyond the default series retention and descriptor grace.
				var gpuID string
				for cycle := 0; cycle <= recoveryCycle; cycle++ {
					observed := cycle == 0 || cycle == recoveryCycle
					cycles.BeginCycle()
					if partial {
						temperature.WithLabelValues("cpu").Observe(40)
					}
					if observed {
						temperature.WithLabelValues("gpu").Observe(35 + float64(cycle))
					}
					require.NoError(t, cycles.CommitCycleSuccess())
					reader := store.Read(metrix.ReadRaw(), metrix.ReadFlatten())
					if cycle == recoveryCycle-1 && !partial {
						require.Positive(t, reader.CollectMeta().EvictedDescriptors)
					}
					attempt, err := engine.PreparePlan(reader)
					require.NoError(t, err)
					var created, removed, updated bool
					for _, action := range attempt.Plan().Actions {
						switch action := action.(type) {
						case chartengine.CreateChartAction:
							if action.Labels["sensor"] == "gpu" {
								if cycle == 0 {
									gpuID = action.ChartID
								}
								assert.Equal(t, gpuID, action.ChartID)
								created = true
							}
						case chartengine.RemoveChartAction:
							removed = removed || action.ChartID == gpuID
						case chartengine.UpdateChartAction:
							if action.ChartID == gpuID {
								for _, value := range action.Values {
									if !value.IsEmpty {
										updated = true
										assert.Equal(t, 35+float64(cycle), value.Float64)
									}
								}
							}
						}
					}
					require.NotEmpty(t, gpuID)
					assert.Equal(t, observed, updated, "numeric update at cycle %d", cycle)
					assert.Equal(t, cycle == 0 || cycle == recoveryCycle && tc.expiry > 0, created, "creation at cycle %d", cycle)
					wantRemoval := tc.expiry > 0 && cycle == tc.expiry
					assert.Equal(t, wantRemoval, removed, "removal at cycle %d", cycle)
					var wire bytes.Buffer
					require.NoError(t, chartemit.ApplyPlan(netdataapi.New(&wire), attempt.Plan(), chartemit.EmitEnv{
						TypeID: "test_localhost", UpdateEvery: 1, Plugin: "test", Module: "test", JobName: "localhost", MSSinceLast: 1000,
					}))
					if wantRemoval {
						assert.Contains(t, wire.String(), "obsolete")
					} else {
						assert.NotContains(t, wire.String(), "obsolete", "cycle %d", cycle)
					}
					require.NoError(t, attempt.Commit())
				}
			})
		}
	}
}
