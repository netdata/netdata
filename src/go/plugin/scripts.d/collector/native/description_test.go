// SPDX-License-Identifier: GPL-3.0-or-later

package native

import (
	"strings"
	"testing"

	"github.com/netdata/netdata/go/plugins/plugin/scripts.d/collector/native/nativefunc"
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

// unicodeDescription is a JSON description with non-BMP text. Tests also derive
// its escaped spelling with jsonEscape.
const unicodeDescription = `{
  "version": "v1",
  "mode": "persistent",
  "metrics": [{"name": "depth", "type": "gauge", "unit": "jobs"}],
  "checks": [{"id": "ready", "title": "Ready 😀", "by_labels": ["queue"]}],
  "functions": [{
    "id": "items", "name": "Items 😀", "help": "Show items 😀.",
    "update_every": 37, "response_type": "table", "has_history": true,
    "accepted_params": ["queue"],
    "required_params": [{
      "id": "queue", "name": "Queue", "help": "Choose a queue.", "type": "multiselect", "unique_view": true,
      "options": [{"id": "mail", "name": "Mail 😀", "defaultSelected": true, "disabled": true}]
    }]
  }],
  "charts": "version: v1\ncontext_namespace: fixture\ngroups:\n  - family: Queue\n    metrics: [depth]\n    charts:\n      - title: Queue 😀\n        context: depth\n        units: jobs\n        dimensions: [{selector: depth, name: depth}]\n",
  "config_schema": {
    "jsonSchema": {
      "$schema": "http://json-schema.org/draft-07/schema#",
      "title": "Queue 😀", "description": "Synthetic queue configuration.",
      "type": "object", "additionalProperties": false,
      "properties": {"text": {"title": "Text", "description": "Display text.", "type": "string", "default": "Queue 😀"}}
    },
    "uiSchema": {}
  }
}`

func TestParseDescription(t *testing.T) {
	unicodeSpec := packageSpec{
		Version: "v1",
		Mode:    modePersistent,
		Metrics: []metricDefinition{{Name: "depth", Type: metricGauge, Unit: "jobs"}},
		Checks:  []checkDefinition{{ID: "ready", Title: "Ready 😀", ByLabels: []string{"queue"}}},
		Functions: []nativefunc.Definition{{
			ID:             "items",
			Name:           "Items 😀",
			Help:           "Show items 😀.",
			UpdateEvery:    37,
			ResponseType:   "table",
			HasHistory:     true,
			AcceptedParams: []string{"queue"},
			RequiredParams: []nativefunc.Parameter{{
				ID:         "queue",
				Name:       "Queue",
				Help:       "Choose a queue.",
				Type:       "multiselect",
				UniqueView: true,
				Options:    []nativefunc.Option{{ID: "mail", Name: "Mail 😀", Default: true, Disabled: true}},
			}},
		}},
	}
	escapedDescription := strings.ReplaceAll(unicodeDescription, "😀", jsonEscape("😀"))
	require.Contains(t, escapedDescription, `\`+"ud83d", "the fixture must spell surrogate-pair escapes")
	require.NotContains(t, escapedDescription, "😀")
	readyCheck := packageSpec{
		Version: "v1",
		Mode:    modeOneshot,
		Checks:  []checkDefinition{{ID: "ready", Title: "Ready 😀"}},
	}
	tests := map[string]struct {
		data         string
		wantSpec     packageSpec
		wantCharts   bool
		wantSettings Settings // effective defaults of an omitted job config
		wantFormText string
	}{
		"YAML with inline assets": {
			data: testDescription,
			wantSpec: packageSpec{
				Version:   "v1",
				Mode:      modePersistent,
				Metrics:   []metricDefinition{{Name: "depth", Type: metricGauge, Unit: "jobs"}},
				Checks:    []checkDefinition{{ID: "ready", Title: "Ready"}},
				Functions: []nativefunc.Definition{{ID: "items", Name: "Items", Help: "Show queue items."}},
			},
			wantCharts: true,
			wantSettings: Settings{
				"count": float64(17),
			},
			wantFormText: "Synthetic queue depth",
		},
		"JSON function-only": {
			data: `{"version":"v1","functions":[{"id":"items","name":"Items","help":"Show items."}]}`,
			wantSpec: packageSpec{
				Version:   "v1",
				Mode:      modeOneshot,
				Functions: []nativefunc.Definition{{ID: "items", Name: "Items", Help: "Show items."}},
			},
		},
		// Python json.dumps() uses surrogate-pair escapes by default for non-BMP Unicode.
		"JSON surrogate-pair escapes": {
			data:       escapedDescription,
			wantSpec:   unicodeSpec,
			wantCharts: true,
			wantSettings: Settings{
				"text": "Queue 😀",
			},
		},
		"JSON literal Unicode": {
			data:       unicodeDescription,
			wantSpec:   unicodeSpec,
			wantCharts: true,
			wantSettings: Settings{
				"text": "Queue 😀",
			},
		},
		"YAML block": {
			data:       "version: v1\nchecks:\n  - id: ready\n    title: Ready 😀\n",
			wantSpec:   readyCheck,
			wantCharts: true,
		},
		"YAML flow": {
			data:       `{version: v1, checks: [{id: ready, title: Ready 😀}]}`,
			wantSpec:   readyCheck,
			wantCharts: true,
		},
		"JSON nulls select defaults": {
			data:       `{"version":"v1","mode":null,"metrics":null,"checks":[{"id":"ready","title":"Ready 😀","by_labels":null}],"functions":null,"charts":null,"config_schema":null}`,
			wantSpec:   readyCheck,
			wantCharts: true,
		},
	}
	for name, tc := range tests {
		t.Run(name, func(t *testing.T) {
			command := []string{"/configured/program", "fixed-argument"}
			definition, err := parseDescription([]byte(tc.data), command)
			require.NoError(t, err)
			assert.Equal(t, tc.wantSpec, definition.packageSpec)
			assert.Equal(t, command, definition.command)
			assert.Len(t, definition.methods, len(tc.wantSpec.Functions))
			assert.Equal(t, tc.wantCharts, definition.templates != nil)
			assert.Equal(t, tc.wantSettings, definition.effectiveSettings(nil))
			if tc.wantFormText != "" {
				form, err := packageForm("native-fixture", definition)
				require.NoError(t, err)
				assert.Contains(t, form, tc.wantFormText)
			}
		})
	}
}

func TestParseDescription_Invalid(t *testing.T) {
	tests := map[string]string{
		"YAML command override":       "version: v1\ncommand: [/private-command]\nchecks: [{id: ready, title: Ready}]\n",
		"YAML empty command override": "version: v1\ncommand: []\nchecks: [{id: ready, title: Ready}]\n",
		"YAML unknown field":          testDescription + "private-field: value\n",
		"YAML duplicate":              testDescription + "version: v1\n",
		"YAML trailing document":      testDescription + "---\nversion: v1\n",
		"YAML bad version":            "version: private-version\nchecks: [{id: ready, title: Ready}]\n",
		"YAML bad metric":             "version: v1\nmetrics: [{name: private-name, type: bad, unit: jobs}]\n",
		"YAML schema path":            "version: v1\nchecks: [{id: ready, title: Ready}]\nconfig_schema: /private-file\n",
		"YAML chart path":             "version: v1\nmetrics: [{name: depth, type: gauge, unit: jobs}]\ncharts: /private-file\n",
		"YAML function-only chart":    "version: v1\nfunctions: [{id: items, name: Items, help: Show items.}]\ncharts: 'version: v1'\n",
		"YAML duplicate schema key":   "version: v1\nchecks: [{id: ready, title: Ready}]\nconfig_schema:\n  jsonSchema: {}\n  jsonSchema: {}\n  uiSchema: {}\n",
		"YAML external schema ref":    "version: v1\nchecks: [{id: ready, title: Ready}]\nconfig_schema:\n  jsonSchema: {$schema: 'http://json-schema.org/draft-07/schema#', type: object, $ref: 'file:///private-file'}\n  uiSchema: {}\n",
		"JSON command override":       `{"version":"v1","checks":[{"id":"ready","title":"Ready"}],"command":["/private-command"]}`,
		"JSON unknown field":          `{"version":"v1","checks":[{"id":"ready","title":"Ready"}],"private-field":0}`,
		"JSON wrong case":             `{"Version":"v1","checks":[{"id":"ready","title":"Ready"}]}`,
		"JSON wrong metric case":      `{"version":"v1","metrics":[{"Name":"depth","type":"gauge","unit":"jobs"}]}`,
		"JSON wrong check case":       `{"version":"v1","checks":[{"ID":"ready","title":"Ready"}]}`,
		"JSON unknown function field": `{"version":"v1","functions":[{"id":"items","name":"Items","help":"Show items.","private-field":true}]}`,
		"JSON wrong parameter case":   `{"version":"v1","functions":[{"id":"items","name":"Items","help":"Show items.","required_params":[{"ID":"queue","name":"Queue"}]}]}`,
		"JSON wrong option case":      `{"version":"v1","functions":[{"id":"items","name":"Items","help":"Show items.","required_params":[{"id":"queue","name":"Queue","options":[{"id":"mail","name":"Mail","DefaultSelected":true}]}]}]}`,
		"JSON duplicate":              `{"version":"v1","checks":[{"id":"ready","title":"Ready"}],"version":"v1"}`,
		"JSON escaped duplicate": `{"version":"v1","checks":[{"id":"ready","title":"Ready","` + jsonEscape("t") +
			`itle":"private-title"}]}`,
		"JSON duplicate schema key":   `{"version":"v1","checks":[{"id":"ready","title":"Ready"}],"config_schema":{"jsonSchema":{"type":"object","type":"object"},"uiSchema":{}}}`,
		"JSON case-folded form field": `{"version":"v1","checks":[{"id":"ready","title":"Ready"}],"config_schema":{"JsonSchema":{"$schema":"http://json-schema.org/draft-07/schema#","type":"object"},"uiSchema":{}}}`,
		"JSON trailing document":      `{"version":"v1","checks":[{"id":"ready","title":"Ready"}]} {}`,
		"JSON array":                  `[{"version":"v1","checks":[{"id":"ready","title":"Ready"}]}]`,
		"invalid UTF-8":               "{\"version\":\"v1\",\"checks\":[{\"id\":\"ready\",\"title\":\"\xff\"}]}",
	}
	for name, data := range tests {
		t.Run(name, func(t *testing.T) {
			_, err := parseDescription([]byte(data), []string{"/configured/program"})
			require.Error(t, err)
			assert.NotContains(t, err.Error(), "private-", "errors never echo script-produced values")
		})
	}
}
