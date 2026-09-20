// SPDX-License-Identifier: GPL-3.0-or-later

package smbios_memory

import (
	"encoding/json"
	"os"
	"strings"
	"testing"

	"gopkg.in/yaml.v3"

	"github.com/netdata/netdata/go/plugins/plugin/framework/chartengine"
	"github.com/netdata/netdata/go/plugins/plugin/framework/charttpl"
	"github.com/netdata/netdata/go/plugins/plugin/go.d/pkg/collecttest"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestArtifacts(t *testing.T) {
	read := func(path string) []byte { data, err := os.ReadFile(path); require.NoError(t, err); return data }
	collecttest.AssertChartTemplateSchema(t, chartTemplateYAML)
	spec, err := charttpl.DecodeYAML([]byte(chartTemplateYAML))
	require.NoError(t, err)
	_, err = chartengine.Compile(spec, 60)
	require.NoError(t, err)
	metadata := read("metadata.yaml")
	health := read("../../../../../health/health.d/smbios_memory.conf")
	collecttest.AssertMetadataDocumentsChartTemplate(t, metadata, chartTemplateYAML, map[string][]string{
		"inventory_status": inventoryStates, "comparison_status": comparisonStates, "confirmed_loss_status": lossStates,
	})
	collecttest.AssertHealthAlertsTargetChartTemplate(t, health, chartTemplateYAML)
	collecttest.AssertHealthAlertsMatchMetadata(t, health, metadata)
	collecttest.AssertMetadataAlertsMatchHealthConfig(t, metadata, health)
	// This one-option form intentionally has no tabs; the tabbed-form helper does not apply.
	var form struct {
		JSONSchema struct {
			Properties map[string]struct {
				Description string
				Default     int
			}
		}
	}
	var doc struct {
		Modules []struct {
			Setup struct {
				Configuration struct {
					Options struct {
						List []struct {
							Name        string
							Description string
							Default     int `yaml:"default_value"`
						}
					}
				}
			}
		}
	}
	require.NoError(t, json.Unmarshal([]byte(configSchema), &form))
	require.NoError(t, yaml.Unmarshal(metadata, &doc))
	require.Len(t, doc.Modules[0].Setup.Configuration.Options.List, 1)
	option := doc.Modules[0].Setup.Configuration.Options.List[0]
	assert.Equal(t, form.JSONSchema.Properties[option.Name].Description, option.Description)
	assert.Equal(t, form.JSONSchema.Properties[option.Name].Default, option.Default)
}

// This is a policy regression check, not an implementation of the query engine.
// The existing source counters remain cumulative and uncorrectable alerts retain
// their lifetime-count semantics; only the correctable health lookup changes.
func TestExistingECCAlertPolicy(t *testing.T) {
	data, err := os.ReadFile("../../../../../health/health.d/memory.conf")
	require.NoError(t, err)
	templates := make(map[string]string)
	for _, part := range strings.Split(string(data), "   template: ")[1:] {
		name, _, _ := strings.Cut(part, "\n")
		templates[name] = part
	}
	for name, selector := range map[string]string{"ecc_memory_mc_correctable": "correctable,correctable_noinfo", "ecc_memory_dimm_correctable": "correctable"} {
		text := templates[name]
		require.NotEmpty(t, text)
		assert.Contains(t, text, "lookup: incremental-sum -2m unaligned of "+selector+"\n")
		assert.NotContains(t, text, "calc:")
		assert.Contains(t, text, "every: 1m")
		assert.Contains(t, text, "warn: ($this == nan or $this == inf) ? (nan) : ($this > 0)")
	}
	for name, calc := range map[string]string{"ecc_memory_mc_uncorrectable": "$uncorrectable + $uncorrectable_noinfo", "ecc_memory_dimm_uncorrectable": "$uncorrectable"} {
		text := templates[name]
		require.NotEmpty(t, text)
		assert.Contains(t, text, "calc: "+calc+"\n")
		assert.Contains(t, text, "crit: $this > 0")
		assert.NotContains(t, text, "lookup:")
	}
}
