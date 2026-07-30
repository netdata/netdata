// SPDX-License-Identifier: GPL-3.0-or-later

package collecttest

import (
	"encoding/json"
	"os"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gopkg.in/yaml.v3"
)

// AssertConfigSchemaMatchesMetadata checks that the DynCfg form and the generated integration doc
// group a collector's options the same way, so an operator who reads about an option in the doc can
// find where to set it in the UI.
//
// metadata.yaml `group` becomes the doc's Group column, and config_schema.json
// `uiSchema.ui:options.tabs[].title` becomes the UI tab. For every documented option, the tab
// listing that option's root property must be the first segment of its group, and every tab must be
// named by at least one group. Tabs can only list whole top-level properties, so a group that
// refines a nested concern takes the "Tab / Subgroup" form and matches on its first segment.
//
// Neither artifact is asserted against itself: each supplies the other's expected value.
//
// This is opt-in per collector because most collectors still group the two artifacts independently.
// Call it from a collector whose artifacts agree, to keep them that way.
func AssertConfigSchemaMatchesMetadata(t testing.TB, schemaPath, metadataPath string) {
	t.Helper()

	tabOfProperty, tabs := readSchemaTabs(t, schemaPath)
	options := readMetadataOptionGroups(t, metadataPath)

	for _, option := range options {
		root := rootProperty(option.Name)
		tab, ok := tabOfProperty[root]
		if !assert.Truef(t, ok, "%s: option %q has group %q, but no tab lists its %q property",
			metadataPath, option.Name, option.Group, root) {
			continue
		}
		group, _, _ := strings.Cut(option.Group, " / ")
		assert.Equalf(t, tab, group, "%s: option %q is in group %q, but %q is on the %q tab",
			metadataPath, option.Name, option.Group, root, tab)
		tabs[tab] = true
	}

	for tab, named := range tabs {
		assert.Truef(t, named, "%s: tab %q is named by no option group, so the doc cannot point at it",
			schemaPath, tab)
	}
}

// readSchemaTabs returns the tab title owning each top-level property, plus a per-tab "was named by
// a group" ledger for the caller to fill in.
func readSchemaTabs(t testing.TB, schemaPath string) (map[string]string, map[string]bool) {
	t.Helper()

	data, err := os.ReadFile(schemaPath)
	require.NoError(t, err)
	var doc struct {
		UISchema struct {
			Options struct {
				Tabs []struct {
					Title  string   `json:"title"`
					Fields []string `json:"fields"`
				} `json:"tabs"`
			} `json:"ui:options"`
		} `json:"uiSchema"`
	}
	require.NoError(t, json.Unmarshal(data, &doc))
	require.NotEmptyf(t, doc.UISchema.Options.Tabs, "%s declares no uiSchema tabs", schemaPath)

	tabOfProperty := make(map[string]string)
	tabs := make(map[string]bool, len(doc.UISchema.Options.Tabs))
	for _, tab := range doc.UISchema.Options.Tabs {
		require.NotEmptyf(t, tab.Title, "%s has a tab with no title", schemaPath)
		tabs[tab.Title] = false
		for _, field := range tab.Fields {
			require.NotContainsf(t, tabOfProperty, field, "%s lists property %q on more than one tab", schemaPath, field)
			tabOfProperty[field] = tab.Title
		}
	}
	return tabOfProperty, tabs
}

type metadataOption struct {
	Name  string `yaml:"name"`
	Group string `yaml:"group"`
}

func readMetadataOptionGroups(t testing.TB, metadataPath string) []metadataOption {
	t.Helper()

	data, err := os.ReadFile(metadataPath)
	require.NoError(t, err)
	var doc struct {
		Modules []struct {
			Setup struct {
				Configuration struct {
					Options struct {
						List []metadataOption `yaml:"list"`
					} `yaml:"options"`
				} `yaml:"configuration"`
			} `yaml:"setup"`
		} `yaml:"modules"`
	}
	require.NoError(t, yaml.Unmarshal(data, &doc))
	require.NotEmptyf(t, doc.Modules, "%s describes no modules", metadataPath)

	// A collector with several documented integrations repeats the option list per module, and they
	// all share the one config_schema.json, so every module's groups must map to the same tabs.
	var options []metadataOption
	for _, module := range doc.Modules {
		options = append(options, module.Setup.Configuration.Options.List...)
	}
	require.NotEmptyf(t, options, "%s documents no config options", metadataPath)
	for _, option := range options {
		require.NotEmptyf(t, option.Group, "%s: option %q has no group", metadataPath, option.Name)
	}
	return options
}

// rootProperty reduces a documented option name to the top-level property a tab can list:
// "rules[].query.period" and "limits.max_instances" become "rules" and "limits".
func rootProperty(option string) string {
	if i := strings.IndexAny(option, ".["); i >= 0 {
		return option[:i]
	}
	return option
}
