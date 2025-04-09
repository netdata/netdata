// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2023-present Datadog, Inc.

package profiledefinition

import (
	"encoding/json"
	"github.com/invopop/jsonschema"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"testing"
)

type ExampleStruct struct {
	Mapping ListMap[string] `json:"mapping"`
}

var example = ExampleStruct{
	Mapping: map[string]string{
		"1": "aaa",
		"2": "bbb",
	},
}

var exampleJSON = `{
	"mapping": [
		{"key": "2", "value": "bbb"},
		{"key": "1", "value": "aaa"}
	]
}`

func TestListMap_MarshalJSON(t *testing.T) {
	bytes, err := json.Marshal(example)
	require.NoError(t, err)

	var expectedExample ExampleStruct
	err = json.Unmarshal(bytes, &expectedExample)
	require.NoError(t, err)

	assert.Equal(t, example.Mapping, expectedExample.Mapping)
}

func TestListMap_UnmarshalJSON(t *testing.T) {
	var expectedExample ExampleStruct
	err := json.Unmarshal([]byte(exampleJSON), &expectedExample)
	require.NoError(t, err)

	assert.Equal(t, example.Mapping, expectedExample.Mapping)
}

func TestListMap_JSONSchema(t *testing.T) {
	reflector := jsonschema.Reflector{
		AllowAdditionalProperties: false,
	}
	schema := reflector.Reflect(&ExampleStruct{})
	schema.Version = "" // no version, to make the test more future-proof
	schemaJSON, err := json.MarshalIndent(schema, "", "  ")
	require.NoError(t, err)

	expectedSchema := `
{
  "$id": "https://github.com/DataDog/datadog-agent/pkg/networkdevice/profile/profiledefinition/example-struct",
  "$ref": "#/$defs/ExampleStruct",
  "$defs": {
    "ExampleStruct": {
      "properties": {
        "mapping": {
          "$ref": "#/$defs/ListMap[string]"
        }
      },
      "additionalProperties": false,
      "type": "object",
      "required": [
        "mapping"
      ]
    },
    "ListMap[string]": {
      "items": {
        "properties": {
          "key": {
            "type": "string"
          },
          "value": {
            "type": "string"
          }
        },
        "additionalProperties": false,
        "type": "object",
        "required": [
          "key",
          "value"
        ]
      },
      "type": "array"
    }
  }
}
`
	assert.JSONEq(t, expectedSchema, string(schemaJSON))
}
