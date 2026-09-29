// SPDX-License-Identifier: GPL-3.0-or-later

package native

import (
	"encoding/json"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

const testDescription = `version: v1
mode: persistent
metrics:
  - {name: depth, type: gauge, unit: jobs}
checks:
  - {id: ready, title: Ready}
functions:
  - {id: items, name: Items, help: Show queue items.}
charts: |
  version: v1
  context_namespace: fixture
  groups:
    - family: Queue
      metrics: [depth]
      charts:
        - id: depth
          title: Queue depth
          context: depth
          units: jobs
          dimensions: [{selector: depth, name: depth}]
config_schema:
  jsonSchema:
    $schema: http://json-schema.org/draft-07/schema#
    title: Queue configuration.
    description: Synthetic queue configuration.
    type: object
    additionalProperties: false
    properties:
      count:
        title: Count
        description: Synthetic queue depth in jobs.
        type: integer
        default: 17
  uiSchema: {}
`

func TestParseDescription(t *testing.T) {
	command := []string{"/configured/program", "fixed-argument"}
	m, charts, err := parseDescription([]byte(testDescription), command)
	require.NoError(t, err)
	require.NotNil(t, charts)
	assert.Equal(t, command, m.Command)
	assert.Equal(t, modePersistent, m.Mode)
	require.Len(t, m.methods, 1)
	assert.Equal(t, "items", m.methods[0].ID)
	assert.Equal(t, float64(17), m.config.effective(nil)["count"])
	form, err := packageForm("native-fixture", m)
	require.NoError(t, err)
	assert.Contains(t, form, "Synthetic queue depth")
	var described map[string]any
	require.NoError(
		t,
		json.Unmarshal(
			[]byte(`{"version":"v1","functions":[{"id":"items","name":"Items","help":"Show items."}]}`),
			&described,
		),
	)
	data, err := json.Marshal(described)
	require.NoError(t, err)
	m, charts, err = parseDescription(data, command)
	require.NoError(t, err)
	assert.True(t, m.functionOnly())
	assert.Nil(t, charts)
}

func TestDescriptionRejectsInvalidMetadata(t *testing.T) {
	for name, body := range map[string]string{
		"command override":       "version: v1\ncommand: [/private-command]\nchecks: [{id: ready, title: Ready}]\n",
		"empty command override": "version: v1\ncommand: []\nchecks: [{id: ready, title: Ready}]\n",
		"unknown field":          testDescription + "private-field: value\n",
		"duplicate":              testDescription + "version: v1\n",
		"trailing document":      testDescription + "---\nversion: v1\n",
		"bad version":            "version: private-version\nchecks: [{id: ready, title: Ready}]\n",
		"bad metric":             "version: v1\nmetrics: [{name: private-name, type: bad, unit: jobs}]\n",
		"schema path":            "version: v1\nchecks: [{id: ready, title: Ready}]\nconfig_schema: /private-file\n",
		"chart path":             "version: v1\nmetrics: [{name: depth, type: gauge, unit: jobs}]\ncharts: /private-file\n",
		"function-only chart":    "version: v1\nfunctions: [{id: items, name: Items, help: Show items.}]\ncharts: 'version: v1'\n",
		"duplicate schema key":   "version: v1\nchecks: [{id: ready, title: Ready}]\nconfig_schema:\n  jsonSchema: {}\n  jsonSchema: {}\n  uiSchema: {}\n",
		"external schema ref":    "version: v1\nchecks: [{id: ready, title: Ready}]\nconfig_schema:\n  jsonSchema: {$schema: 'http://json-schema.org/draft-07/schema#', type: object, $ref: 'file:///private-file'}\n  uiSchema: {}\n",
	} {
		t.Run(name, func(t *testing.T) {
			_, _, err := parseDescription([]byte(body), []string{"/configured/program"})
			require.Error(t, err)
			assert.NotContains(t, err.Error(), "private-")
		})
	}
}

func TestDescriptionJSONUnicode(t *testing.T) {
	// Python json.dumps() uses surrogate-pair escapes by default for non-BMP Unicode.
	const escaped = `{
  "version": "v1",
  "mode": "persistent",
  "metrics": [{"name": "depth", "type": "gauge", "unit": "jobs"}],
  "checks": [{"id": "ready", "title": "Ready \ud83d\ude00", "by_labels": ["queue"]}],
  "functions": [{
    "id": "items", "name": "Items \ud83d\ude00", "help": "Show items \ud83d\ude00.",
    "update_every": 37, "response_type": "table", "has_history": true,
    "accepted_params": ["queue"],
    "required_params": [{
      "id": "queue", "name": "Queue", "help": "Choose a queue.", "type": "multiselect", "unique_view": true,
      "options": [{"id": "mail", "name": "Mail \ud83d\ude00", "defaultSelected": true, "disabled": true}]
    }]
  }],
  "charts": "version: v1\ncontext_namespace: fixture\ngroups:\n  - family: Queue\n    metrics: [depth]\n    charts:\n      - title: Queue \ud83d\ude00\n        context: depth\n        units: jobs\n        dimensions: [{selector: depth, name: depth}]\n",
  "config_schema": {
    "jsonSchema": {
      "$schema": "http://json-schema.org/draft-07/schema#",
      "title": "Queue \ud83d\ude00", "description": "Synthetic queue configuration.",
      "type": "object", "additionalProperties": false,
      "properties": {"text": {"title": "Text", "description": "Display text.", "type": "string", "default": "Queue \ud83d\ude00"}}
    },
    "uiSchema": {}
  }
}`
	for name, body := range map[string]string{
		"escaped": escaped,
		"literal": strings.ReplaceAll(escaped, `\ud83d\ude00`, "😀"),
	} {
		t.Run(name, func(t *testing.T) {
			require.True(t, json.Valid([]byte(body)))
			m, charts, err := parseDescription([]byte(body), []string{"/configured/program"})
			require.NoError(t, err)
			require.NotNil(t, charts)
			assert.Equal(t, modePersistent, m.Mode)
			require.Len(t, m.Metrics, 1)
			assert.Equal(t, "jobs", m.Metrics[0].Unit)
			require.Len(t, m.Checks, 1)
			assert.Equal(t, "Ready 😀", m.Checks[0].Title)
			assert.Equal(t, []string{"queue"}, m.Checks[0].ByLabels)
			require.Len(t, m.methods, 1)
			method := m.methods[0]
			assert.Equal(t, "Items 😀", method.Name)
			assert.Equal(t, "Show items 😀.", method.Help)
			assert.Equal(t, 37, method.UpdateEvery)
			assert.Equal(t, "table", method.ResponseType)
			assert.True(t, method.HasHistory)
			assert.Equal(t, []string{"queue"}, method.AcceptedParams)
			require.Len(t, method.RequiredParams, 1)
			param := method.RequiredParams[0]
			assert.True(t, param.UniqueView)
			require.Len(t, param.Options, 1)
			assert.Equal(t, "Mail 😀", param.Options[0].Name)
			assert.True(t, param.Options[0].Default)
			assert.True(t, param.Options[0].Disabled)
			assert.Equal(t, "Queue 😀", m.config.effective(nil)["text"])
		})
	}
}

func TestDescriptionYAMLAndNullDefaults(t *testing.T) {
	for name, body := range map[string]string{
		"YAML block": "version: v1\nchecks:\n  - id: ready\n    title: Ready 😀\n",
		"YAML flow":  `{version: v1, checks: [{id: ready, title: Ready 😀}]}`,
		"JSON nulls": `{"version":"v1","mode":null,"metrics":null,"checks":[{"id":"ready","title":"Ready 😀","by_labels":null}],"functions":null,"charts":null,"config_schema":null}`,
	} {
		t.Run(name, func(t *testing.T) {
			m, _, err := parseDescription([]byte(body), []string{"/configured/program"})
			require.NoError(t, err)
			assert.Equal(t, modeOneshot, m.Mode)
			require.Len(t, m.Checks, 1)
			assert.Equal(t, "Ready 😀", m.Checks[0].Title)
		})
	}
}

func TestDescriptionRejectsInvalidJSON(t *testing.T) {
	for name, body := range map[string]string{
		"command override":       `{"version":"v1","checks":[{"id":"ready","title":"Ready"}],"command":["/private-command"]}`,
		"unknown field":          `{"version":"v1","checks":[{"id":"ready","title":"Ready"}],"private-field":0}`,
		"wrong case":             `{"Version":"v1","checks":[{"id":"ready","title":"Ready"}]}`,
		"wrong metric case":      `{"version":"v1","metrics":[{"Name":"depth","type":"gauge","unit":"jobs"}]}`,
		"wrong check case":       `{"version":"v1","checks":[{"ID":"ready","title":"Ready"}]}`,
		"unknown function field": `{"version":"v1","functions":[{"id":"items","name":"Items","help":"Show items.","private-field":true}]}`,
		"wrong parameter case":   `{"version":"v1","functions":[{"id":"items","name":"Items","help":"Show items.","required_params":[{"ID":"queue","name":"Queue"}]}]}`,
		"wrong option case":      `{"version":"v1","functions":[{"id":"items","name":"Items","help":"Show items.","required_params":[{"id":"queue","name":"Queue","options":[{"id":"mail","name":"Mail","DefaultSelected":true}]}]}]}`,
		"duplicate":              `{"version":"v1","checks":[{"id":"ready","title":"Ready"}],"version":"v1"}`,
		"escaped duplicate":      `{"version":"v1","checks":[{"id":"ready","title":"Ready","\u0074itle":"private-title"}]}`,
		"duplicate schema key":   `{"version":"v1","checks":[{"id":"ready","title":"Ready"}],"config_schema":{"jsonSchema":{"type":"object","type":"object"},"uiSchema":{}}}`,
		"trailing document":      `{"version":"v1","checks":[{"id":"ready","title":"Ready"}]} {}`,
		"array":                  `[{"version":"v1","checks":[{"id":"ready","title":"Ready"}]}]`,
		"invalid UTF-8":          "{\"version\":\"v1\",\"checks\":[{\"id\":\"ready\",\"title\":\"\xff\"}]}",
	} {
		t.Run(name, func(t *testing.T) {
			_, _, err := parseDescription([]byte(body), []string{"/configured/program"})
			require.Error(t, err)
			assert.NotContains(t, err.Error(), "private-")
		})
	}
}
