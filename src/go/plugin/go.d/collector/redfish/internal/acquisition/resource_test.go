// SPDX-License-Identifier: GPL-3.0-or-later

package acquisition

import (
	"encoding/json"
	"net/url"
	"strings"
	"testing"

	"github.com/netdata/netdata/go/plugins/plugin/go.d/collector/redfish/internal/measurement"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestResourceMapDecodePreservesTypedJSONPolicy(t *testing.T) {
	for name, body := range map[string]string{
		"full":                      `{"@odata.id":"/resource","Id":"1","Name":"Resource","PowerState":"On","FailurePredicted":true,"Status":{"Health":"OK","HealthRollup":"Warning","State":"Enabled","Conditions":[{"Message":"x","MessageId":"M.1","MessageArgs":["a","b"],"Severity":"Critical","Timestamp":"now","OriginOfCondition":{"@odata.id":"/other"}}]}}`,
		"null":                      `{"Name":null,"FailurePredicted":null,"Status":{"Health":null,"Conditions":[null,{"MessageArgs":null,"Severity":null,"OriginOfCondition":null}]}}`,
		"malformed status":          `{"Id":"1","Name":"valid","Status":false,"PowerState":"On"}`,
		"malformed optional fields": `{"FailurePredicted":"bad","Name":12,"Status":{"Health":[],"State":"Absent","Conditions":[false,{"MessageArgs":["valid",false,null],"Severity":{},"OriginOfCondition":false}]}}`,
		"case variants":             `{"id":"1","name":"Name","powerstate":"On","status":{"health":"OK","conditions":[{"messageargs":["a"],"severity":"Critical","originofcondition":{"@ODATA.ID":"/r"}}]}}`,
		"duplicate casing":          `{"Name":"first","name":false,"Status":{"Health":"OK","Conditions":[{"Message":"first","MessageArgs":["first"]}]},"status":{"Health":null,"health":"Warning","conditions":[{"MessageArgs":[null],"Severity":"OK"}]}}`,
		"empty arrays":              `{"Status":{"Conditions":[]}}`,
	} {
		t.Run(name, func(t *testing.T) {
			var data map[string]any
			require.NoError(t, decodeJSONBytes([]byte(body), &data))
			sorted, err := json.Marshal(data)
			require.NoError(t, err)
			var expected measurement.Document
			expectedErr := json.Unmarshal(sorted, &expected)
			actual, actualErr := decodeGenericResource(data)
			assert.Equal(t, expected, actual)
			assert.Equal(t, expectedErr != nil, actualErr != nil)
		})
	}
}

func TestResourceValidationPreservesPartialDescendantPolicy(t *testing.T) {
	root, origin, err := NormalizeServiceRoot("https://bmc.example/redfish/v1/")
	require.NoError(t, err)
	client := &Client{
		root:   root,
		origin: origin,
	}
	target, err := url.Parse("https://bmc.example/redfish/v1/Sensors/1")
	require.NoError(t, err)
	body := []byte(
		`{"@odata.id":"/redfish/v1/Sensors/1","@odata.type":"#Sensor.v1_0_0.Sensor","Id":"1","Name":"Sensor","Status":{"Health":false,"State":"Enabled"},"Reading":0}`,
	)
	var data map[string]any
	require.NoError(t, decodeJSONBytes(body, &data))
	doc, err := client.validateResourceData("sensor", data, target)
	var decodeErr *resourceDecodeError
	require.ErrorAs(t, err, &decodeErr, "strict base acquisition rejects malformed optional typed fields")
	expected := measurement.Document{
		ODataID: "/redfish/v1/Sensors/1",
		ID:      "1",
		Name:    "Sensor",
		Status: measurement.Status{
			State: "Enabled",
		},
	}
	assert.Equal(t, expected, doc)
	node, err := client.graphNodeFromResponse("sensor", &responseData{
		url:  target,
		body: body,
	}, "")
	require.NoError(t, err, "descendant acquisition keeps usable fields as before")
	assert.Equal(t, expected, node.Doc)
	assert.Equal(t, json.Number("0"), node.Data["Reading"])
}

// The full response path measures parsing plus validation and graph construction.
// Arbitrary source properties stay in Data; only generic typed fields are decoded.
func BenchmarkGraphResourceDecode(b *testing.B) {
	root, origin, err := NormalizeServiceRoot("https://bmc.example/redfish/v1/")
	require.NoError(b, err)
	client := &Client{
		root:   root,
		origin: origin,
	}
	target, err := url.Parse("https://bmc.example/redfish/v1/Sensors/1")
	require.NoError(b, err)
	for name, extra := range map[string]string{"sensor": "", "extended": " ,\"Oem\":{\"Vendor\":{\"Payload\":\"" + strings.Repeat("value", 1600) + "\"}}"} {
		b.Run(name, func(b *testing.B) {
			body := []byte(
				`{"@odata.id":"/redfish/v1/Sensors/1","@odata.type":"#Sensor.v1_0_0.Sensor","Id":"1","Name":"Sensor","Status":{"Health":"OK","Conditions":[{"Severity":"Warning","MessageArgs":["a"]}]},"ReadingType":"Temperature","ReadingUnits":"Cel","Reading":42` + extra + `}`,
			)
			b.ReportAllocs()
			b.ResetTimer()
			for b.Loop() {
				_, err := client.graphNodeFromResponse(
					"sensor",
					&responseData{
						url:  target,
						body: body,
					},
					"",
				)
				if err != nil {
					b.Fatal(err)
				}
			}
		})
	}
}
