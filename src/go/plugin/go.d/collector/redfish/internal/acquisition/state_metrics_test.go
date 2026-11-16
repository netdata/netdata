// SPDX-License-Identifier: GPL-3.0-or-later

package acquisition

import (
	"fmt"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"
)

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
			observations := measurementTestProject(t, client, node)
			for _, observation := range observations {
				if strings.HasPrefix(observation.Metric, "sensor_conditions_") {
					if counts == nil {
						counts = make(map[string]float64)
					}
					counts[observation.Metric] = observation.Value
				}
			}
			require.Equal(t, test.want, counts)
			readings := measurementTestReadings(observations)
			require.Len(t, readings, 1)
			require.Equal(t, float64(42), readings[0].Value, "valid unrelated source survives optional decode failures")
		})
	}
}
