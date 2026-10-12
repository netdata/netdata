// SPDX-License-Identifier: GPL-3.0-or-later

//go:build linux

package jetson

import (
	"bytes"
	"testing"

	"github.com/netdata/netdata/go/plugins/pkg/metrix"
	"github.com/netdata/netdata/go/plugins/pkg/netdataapi"
	"github.com/netdata/netdata/go/plugins/plugin/framework/chartemit"
	"github.com/netdata/netdata/go/plugins/plugin/framework/chartengine"
	"github.com/netdata/netdata/go/plugins/plugin/go.d/pkg/collecttest"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestTemperatureChartsSurviveUnavailableSensor(t *testing.T) {
	c, fake := newCollectorWithFake(t)
	startRun(t, c)
	conn := fake.Accept(t)
	engine, err := chartengine.New()
	require.NoError(t, err)
	require.NoError(t, engine.LoadYAML([]byte(c.ChartTemplateYAML()), 1))

	var gpuID string
	for phase, tc := range []struct {
		record string
		gpu    *float64
		cycles int
	}{
		{record: "cpu@39.218C gpu@34.562C", gpu: new(34.562), cycles: 1},
		// Continue successful CPU observations beyond both chart expiry and series retention.
		{record: "cpu@39.218C gpu@-256C", cycles: 25},
		{record: "gpu@35.593C cpu@39.218C", gpu: new(35.593), cycles: 1},
	} {
		sendRecord(t, conn, tc.record)
		waitSample(t, c, func(s sample) bool {
			gpu, ok := s.Temperatures["gpu"]
			return tc.gpu == nil && !ok || tc.gpu != nil && ok && gpu == *tc.gpu
		})
		for cycle := range tc.cycles {
			got, err := collecttest.CollectScalarSeries(c)
			require.NoError(t, err)
			want := map[string]metrix.SampleValue{`temperature{sensor="cpu"}`: 39.218}
			if tc.gpu != nil {
				want[`temperature{sensor="gpu"}`] = *tc.gpu
			}
			assert.Equal(t, want, got)

			attempt, err := engine.PreparePlan(c.MetricStore().Read(metrix.ReadRaw(), metrix.ReadFlatten()))
			require.NoError(t, err)
			var gpuUpdated bool
			for _, action := range attempt.Plan().Actions {
				switch action := action.(type) {
				case chartengine.CreateChartAction:
					if action.Labels["sensor"] == "gpu" {
						assert.Zero(t, phase, "GPU chart must not be recreated on recovery")
						gpuID = action.ChartID
					}
				case chartengine.RemoveChartAction:
					assert.NotEqual(t, gpuID, action.ChartID, "GPU chart retired at missing cycle %d", cycle+1)
				case chartengine.UpdateChartAction:
					if action.ChartID == gpuID {
						for _, value := range action.Values {
							if !value.IsEmpty {
								require.NotNil(t, tc.gpu, "unavailable sensor emitted a numeric value")
								assert.Equal(t, *tc.gpu, value.Float64)
								gpuUpdated = true
							}
						}
					}
				}
			}
			require.NotEmpty(t, gpuID)
			assert.Equal(t, tc.gpu != nil, gpuUpdated)
			var wire bytes.Buffer
			require.NoError(t, chartemit.ApplyPlan(netdataapi.New(&wire), attempt.Plan(), chartemit.EmitEnv{
				TypeID: "jetson_localhost", UpdateEvery: 1, Plugin: "go.d.plugin", Module: "jetson", JobName: "localhost", MSSinceLast: 1000,
			}))
			assert.NotContains(t, wire.String(), "obsolete")
			require.NoError(t, attempt.Commit())
		}
	}
}
