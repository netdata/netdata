// SPDX-License-Identifier: GPL-3.0-or-later

package native

import (
	"encoding/json"
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
