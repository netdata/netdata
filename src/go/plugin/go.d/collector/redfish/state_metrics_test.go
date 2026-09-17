// SPDX-License-Identifier: GPL-3.0-or-later

package redfish

import (
	"fmt"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

func TestRedundancyModeSourceEnums(t *testing.T) {
	for name, test := range map[string]struct {
		data map[string]any
		want string
	}{
		"legacy n plus m":    {map[string]any{"Mode": "N+m"}, "n_plus_m"},
		"modern n plus m":    {map[string]any{"RedundancyType": "NPlusM"}, "n_plus_m"},
		"modern wins":        {map[string]any{"RedundancyType": "NPlusM", "Mode": "Failover"}, "n_plus_m"},
		"modern null legacy": {map[string]any{"RedundancyType": nil, "Mode": "N+m"}, "n_plus_m"},
		"legacy failover":    {map[string]any{"Mode": "Failover"}, "failover"},
		"modern sharing":     {map[string]any{"RedundancyType": "Sharing"}, "sharing"},
		"modern malformed":   {map[string]any{"RedundancyType": false, "Mode": "Failover"}, "unknown"},
		"unsupported mode":   {map[string]any{"Mode": "VendorMode"}, "unknown"},
		"absent":             {map[string]any{}, ""},
	} {
		t.Run(name, func(t *testing.T) {
			observations := fixtureClient().additionalStateObservations(&graphNode{
				Kind: "redundancy",
				Data: test.data,
			}, nil)
			if test.want == "" {
				require.Empty(t, observations)
				return
			}
			require.Len(t, observations, 1)
			require.Equal(t, "redundancy_mode", observations[0].Metric)
			require.Equal(t, test.want, observations[0].State)
		})
	}
}

func TestMalformedConditionMembersDoNotPublishHealthyCounts(t *testing.T) {
	for name, test := range map[string]struct {
		conditions string
		want       map[string]float64
	}{
		"boolean member":          {`[false]`, nil},
		"null member":             {`[null]`, nil},
		"array member":            {`[[]]`, nil},
		"mixed malformed":         {`[{"Severity":"Critical"},false]`, nil},
		"wrong container":         {`false`, nil},
		"null":                    {`null`, nil},
		"empty":                   {`[]`, map[string]float64{"sensor_conditions_ok": 0, "sensor_conditions_warning": 0, "sensor_conditions_critical": 0, "sensor_conditions_unknown": 0}},
		"critical and unexpected": {`[{"Severity":"Critical"},{"Severity":false}]`, map[string]float64{"sensor_conditions_ok": 0, "sensor_conditions_warning": 0, "sensor_conditions_critical": 1, "sensor_conditions_unknown": 1}},
	} {
		t.Run(name, func(t *testing.T) {
			client := fixtureClient()
			response := &responseData{
				body: []byte(
					fmt.Sprintf(
						`{"@odata.id":"/redfish/v1/Sensors/1","@odata.type":"#Sensor.v1_0_0.Sensor","Id":"1","Name":"Sensor","ReadingType":"Temperature","ReadingUnits":"Cel","Reading":42,"Status":{"Conditions":%s}}`,
						test.conditions,
					),
				),
			}
			response.url, _ = client.resolveURI(client.root, "/redfish/v1/Sensors/1", false)
			node, err := client.graphNodeFromResponse("sensor", response, "resource")
			require.NoError(t, err)
			var counts map[string]float64
			for _, observation := range client.statusObservations(node) {
				if observation.State == "" {
					if counts == nil {
						counts = make(map[string]float64)
					}
					counts[observation.Metric] = observation.Value
				}
			}
			require.Equal(t, test.want, counts)
			readings := client.readingsForNode(node, time.Unix(10, 0))
			require.Len(t, readings, 1)
			require.True(t, readings[0].Valid)
			require.Equal(t, float64(42), readings[0].Value, "valid unrelated source survives optional decode failures")
		})
	}
}

// Counting stays O(condition records); ns/op is a local-machine trend indicator.
func BenchmarkConditionCounts(b *testing.B) {
	data := map[string]any{
		"Status": map[string]any{
			"Conditions": []any{map[string]any{"Severity": "Critical"}, map[string]any{"Severity": "Warning"}},
		},
	}
	doc, err := decodeGenericResource(data)
	if err != nil {
		b.Fatal(err)
	}
	node := &graphNode{
		Data: data,
		Doc:  doc,
	}
	b.ReportAllocs()
	for b.Loop() {
		_, _, readable := conditionCountsForNode(node)
		if !readable {
			b.Fatal("valid conditions unreadable")
		}
	}
}
