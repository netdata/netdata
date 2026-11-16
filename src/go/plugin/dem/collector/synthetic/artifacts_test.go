// SPDX-License-Identifier: GPL-3.0-or-later

package synthetic

import (
	"encoding/json"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"

	"github.com/netdata/netdata/go/plugins/plugin/dem/collector/internal/syntheticjob"
	model "github.com/netdata/netdata/go/plugins/plugin/dem/synthetic"
	"github.com/netdata/netdata/go/plugins/plugin/framework/charttpl"
	"github.com/netdata/netdata/go/plugins/plugin/go.d/pkg/collecttest"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gopkg.in/yaml.v3"
)

// shippedModules pairs each module with its kind, integration card, form and constructor defaults.
var shippedModules = map[string]struct {
	kind          model.Kind
	card          string
	schema        string
	configuration func() any
}{
	"journey": {
		kind:          model.Journey,
		card:          "collector-dem.plugin-journey",
		schema:        "config_schema_journey.json",
		configuration: func() any { return NewJourney(Dependencies{}).Configuration() },
	},
	"lighthouse": {
		kind:          model.Lighthouse,
		card:          "collector-dem.plugin-lighthouse",
		schema:        "config_schema_lighthouse.json",
		configuration: func() any { return NewLighthouse(Dependencies{}).Configuration() },
	},
}

// The modules share the chart template and the metadata file; each card documents the charts, alerts and form of its
// own module.
func TestShippedArtifacts(t *testing.T) {
	collecttest.AssertChartTemplateSchema(t, charts)
	metadata, err := os.ReadFile("metadata.yaml")
	require.NoError(t, err)
	health, err := os.ReadFile("../../../../../health/health.d/dem.conf")
	require.NoError(t, err)
	collecttest.AssertHealthAlertsTargetChartTemplateWith(t, health, charts, collecttest.HealthAlertsCheck{
		ContextPrefix: "dem_synthetic.",
	})
	templateCharts, err := collecttest.ChartTemplateCharts(charts)
	require.NoError(t, err)
	states := map[string][]string{"execution_state": syntheticjob.ExecutionStates}
	for name, module := range shippedModules {
		t.Run(name, func(t *testing.T) {
			card, err := collecttest.DecodeMetadataModule(metadata, module.card)
			require.NoError(t, err)
			require.NoError(
				t,
				collecttest.CheckMetadataMetricsMatchCharts(
					card.Metrics,
					kindCharts(templateCharts, module.kind),
					states,
				),
			)
			collecttest.AssertHealthAlertsMatchMetadataWith(t, health, metadata, collecttest.HealthAlertsCheck{
				ContextPrefix: "dem_synthetic.",
				ModuleID:      module.card,
			})
			collecttest.AssertConfigSchemaMatchesMetadataWith(t, module.schema, cardMetadata(t, metadata, module.card),
				collecttest.ConfigSchemaCheck{
					Defaults: true,
				})
			assertFormDefaultsMatchConstructor(t, module.schema, module.configuration())
		})
	}
}

// kindCharts returns the template charts a job of kind can create: the shared charts directly under the collector
// namespace and the charts under the kind's own namespace.
func kindCharts(all map[string]charttpl.Chart, kind model.Kind) map[string]charttpl.Chart {
	out := map[string]charttpl.Chart{}
	for context, chart := range all {
		_, leaf, _ := strings.Cut(context, ".")
		namespace, _, nested := strings.Cut(leaf, ".")
		if !nested || model.Kind(namespace) == kind {
			out[context] = chart
		}
	}
	return out
}

// cardMetadata writes the metadata file reduced to one card. The shared form check compares every module of a file
// with one form, and this file holds two modules with their own forms.
func cardMetadata(t *testing.T, metadata []byte, id string) string {
	t.Helper()
	var doc map[string]any
	require.NoError(t, yaml.Unmarshal(metadata, &doc))
	doc["modules"] = slices.DeleteFunc(doc["modules"].([]any), func(module any) bool {
		return module.(map[string]any)["meta"].(map[string]any)["id"] != id
	})
	require.Len(t, doc["modules"], 1)
	raw, err := yaml.Marshal(doc)
	require.NoError(t, err)
	path := filepath.Join(t.TempDir(), "metadata.yaml")
	require.NoError(t, os.WriteFile(path, raw, 0o600))
	return path
}

// assertFormDefaultsMatchConstructor requires every form default to equal the constructor's effective configuration,
// because the form materializes its defaults into every submitted job.
func assertFormDefaultsMatchConstructor(t *testing.T, schemaPath string, configuration any) {
	t.Helper()
	raw, err := os.ReadFile(schemaPath)
	require.NoError(t, err)
	var form struct {
		JSONSchema struct {
			Properties map[string]map[string]any `json:"properties"`
		} `json:"jsonSchema"`
	}
	require.NoError(t, json.Unmarshal(raw, &form))
	raw, err = json.Marshal(configuration)
	require.NoError(t, err)
	var effective map[string]any
	require.NoError(t, json.Unmarshal(raw, &effective))
	defaults := 0
	for name, property := range form.JSONSchema.Properties {
		if value, ok := property["default"]; ok {
			defaults++
			assert.Equalf(t, value, effective[name], "%s: form default and constructor default", name)
		}
	}
	assert.Positive(t, defaults)
}
