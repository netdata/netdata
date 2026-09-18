// SPDX-License-Identifier: GPL-3.0-or-later

package measurement_test

import (
	"encoding/json"
	"math"
	"testing"
	"time"

	"github.com/netdata/netdata/go/plugins/plugin/go.d/collector/redfish/internal/measurement"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestProjectInventoryNumbers(t *testing.T) {
	for name, tc := range map[string]struct {
		raw  any
		want *float64
	}{
		"zero":             {raw: json.Number("0"), want: new(float64(0))},
		"integer":          {raw: json.Number("32"), want: new(float64(32))},
		"exponent":         {raw: json.Number("3.2e1"), want: new(float64(32))},
		"floating integer": {raw: float64(32), want: new(float64(32))},
		"int":              {raw: 32, want: new(float64(32))},
		"null":             {},
		"negative":         {raw: json.Number("-1")},
		"fraction":         {raw: json.Number("1.5")},
		"numeric string":   {raw: "32"},
		"bool":             {raw: true},
		"object":           {raw: map[string]any{"value": 32}},
		"infinity":         {raw: math.Inf(1)},
		"NaN":              {raw: math.NaN()},
	} {
		t.Run(name, func(t *testing.T) {
			p := measurement.New("https://bmc.example.test", "test", nil)
			nodes := []*measurement.Resource{
				{
					Kind:             "processor",
					Key:              "cpu",
					AcquisitionState: "readable",
					Data:             map[string]any{"TotalCores": tc.raw},
				},
				{
					Kind:             "memory",
					Key:              "dimm",
					AcquisitionState: "readable",
					Data:             map[string]any{"CapacityMiB": tc.raw},
				},
				{
					Kind:             "drive",
					Key:              "drive",
					AcquisitionState: "readable",
					Data:             map[string]any{"CapacityBytes": tc.raw},
				},
				{
					Kind:             "volume",
					Key:              "volume",
					AcquisitionState: "readable",
					Data:             map[string]any{"CapacityBytes": tc.raw},
				},
			}
			result, err := p.Project(nodes, true, time.Unix(100, 0))
			require.NoError(t, err)
			require.Len(t, result.Components, 4)
			assert.Equal(t, tc.want, result.Components[0].TotalCores)
			assert.Nil(t, result.Components[0].EnabledCores, "absent is not zero")
			wantCapacity := tc.want
			if tc.want != nil {
				wantCapacity = new(*tc.want * 1048576)
			}
			assert.Equal(t, wantCapacity, result.Components[1].CapacityBytes)
			assert.Equal(t, tc.want, result.Components[2].CapacityBytes)
			assert.Equal(t, tc.want, result.Components[3].CapacityBytes)
			for _, node := range nodes {
				node.Data = map[string]any{}
			}
			without, err := p.Project(nodes, true, time.Unix(110, 0))
			require.NoError(t, err)
			assert.Equal(t, without.Observations, result.Observations, "inventory details do not add metrics")
			assert.Equal(t, tc.want, result.Components[0].TotalCores, "copied values survive source changes")
		})
	}
}

func TestProjectInventoryMemoryScaleOverflow(t *testing.T) {
	p := measurement.New("https://bmc.example.test", "test", nil)
	result, err := p.Project([]*measurement.Resource{{
		Kind: "memory", Key: "dimm", AcquisitionState: "readable", Data: map[string]any{"CapacityMiB": math.MaxFloat64},
	}}, true, time.Unix(100, 0))
	require.NoError(t, err)
	require.Len(t, result.Components, 1)
	assert.Nil(t, result.Components[0].CapacityBytes)
}
