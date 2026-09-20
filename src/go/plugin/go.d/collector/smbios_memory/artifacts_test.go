// SPDX-License-Identifier: GPL-3.0-or-later

package smbios_memory

import (
	"encoding/json"
	"os"
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
	type option struct {
		Description string
		Default     int `json:"default" yaml:"default_value"`
	}
	var form struct {
		JSONSchema struct {
			Properties map[string]option
		}
	}
	var doc struct {
		Modules []struct {
			Setup struct {
				Configuration struct {
					Options struct {
						List []struct {
							Name   string
							option `yaml:",inline"`
						}
					}
				}
			}
		}
	}
	require.NoError(t, json.Unmarshal([]byte(configSchema), &form))
	require.NoError(t, yaml.Unmarshal(metadata, &doc))
	require.NotEmpty(t, doc.Modules)
	documented := make(map[string]option)
	for _, item := range doc.Modules[0].Setup.Configuration.Options.List {
		documented[item.Name] = item.option
	}
	assert.Equal(t, form.JSONSchema.Properties, documented)
}

// The Live Data section mirrors the Function: ids, parameters and returned columns.
func TestMetadataDocumentsFunctions(t *testing.T) {
	metadata, err := os.ReadFile("metadata.yaml")
	require.NoError(t, err)
	h := newFixtureHost(t)
	c := h.collector(t)
	cycle(t, c)
	collecttest.AssertMetadataDocumentsFunctions(t, metadata, collecttest.MetadataFunctionsCheck{
		Context: t.Context(),
		Module:  "smbios_memory",
		Methods: smbiosMethods(),
		Handler: c.funcRouter,
	})
}
