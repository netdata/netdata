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
// describe a collector's options the same way, so an operator who reads about an option in the doc
// can find it in the UI and recognizes it there.
//
// metadata.yaml `group` becomes the doc's Group column, and config_schema.json
// `uiSchema.ui:options.tabs[].title` becomes the UI tab. For every documented option, the tab
// listing that option's root property must be the first segment of its group, and every tab must be
// named by at least one group. Tabs can only list whole top-level properties, so a group that
// refines a nested concern takes the "Tab / Subgroup" form and matches on its first segment.
//
// metadata.yaml `description` becomes the doc table's Description column and the schema's
// `description` renders under the field, so the two must be the same sentence: depth that only the
// form needs goes in `ui:help`, depth that only the doc needs goes in `detailed_description`. A
// documented option the schema does not declare is drift too: the doc promises a field the form
// does not have.
//
// Neither artifact is asserted against itself: each supplies the other's expected value.
//
// This is opt-in per collector because most collectors still author the two artifacts independently.
// Call it from a collector whose artifacts agree, to keep them that way.
func AssertConfigSchemaMatchesMetadata(t testing.TB, schemaPath, metadataPath string) {
	t.Helper()

	schema := readConfigSchema(t, schemaPath)
	tabOfProperty, tabs := schema.tabOwners(t)
	options := readMetadataOptions(t, metadataPath)

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

		node, ok := schema.Node(option.Name)
		if !assert.Truef(t, ok, "%s: option %q is documented, but %s declares no such property",
			metadataPath, option.Name, schemaPath) {
			continue
		}
		description, _ := node["description"].(string)
		assert.Equalf(t, strings.TrimSpace(option.Description), strings.TrimSpace(description),
			"%s: option %q is described differently in the doc (%s) and the form (%s)",
			metadataPath, option.Name, metadataPath, schemaPath)
	}

	for tab, named := range tabs {
		assert.Truef(t, named, "%s: tab %q is named by no option group, so the doc cannot point at it",
			schemaPath, tab)
	}

	documented := map[string]bool{}
	for _, option := range options {
		documented[rootProperty(option.Name)] = true
	}
	for name := range schema.visibleTopLevel() {
		assert.Truef(t, documented[name], "%s: property %q is on the form but %s documents no option under it",
			schemaPath, name, metadataPath)
	}
}

type schemaTab struct {
	Title  string   `json:"title"`
	Fields []string `json:"fields"`
}

// configSchemaDocument is a loaded config_schema.json.
type configSchemaDocument struct {
	SchemaResolver
	path string
	tabs []schemaTab
	ui   map[string]any // the uiSchema member
}

// visibleTopLevel returns the top-level properties the form renders: everything the schema declares
// or a dependencies branch reveals, minus hidden ones.
func (d configSchemaDocument) visibleTopLevel() map[string]map[string]any {
	out := d.Properties(d.Root())
	for name := range out {
		ui, _ := d.ui[name].(map[string]any)
		if ui != nil && ui["ui:widget"] == "hidden" {
			delete(out, name)
		}
	}
	return out
}

func readConfigSchema(t testing.TB, schemaPath string) configSchemaDocument {
	t.Helper()

	data, err := os.ReadFile(schemaPath)
	require.NoError(t, err)
	var doc struct {
		JSONSchema map[string]any `json:"jsonSchema"`
		UISchema   map[string]any `json:"uiSchema"`
	}
	require.NoError(t, json.Unmarshal(data, &doc))
	var tabs []schemaTab
	if options, ok := doc.UISchema["ui:options"].(map[string]any); ok {
		raw, err := json.Marshal(options["tabs"])
		require.NoError(t, err)
		require.NoError(t, json.Unmarshal(raw, &tabs))
	}
	require.NotEmptyf(t, tabs, "%s declares no uiSchema tabs", schemaPath)
	return configSchemaDocument{SchemaResolver: NewSchemaResolver(doc.JSONSchema), path: schemaPath, tabs: tabs, ui: doc.UISchema}
}

// tabOwners returns the tab title owning each top-level property, plus a per-tab "was named by a
// group" ledger for the caller to fill in.
func (d configSchemaDocument) tabOwners(t testing.TB) (map[string]string, map[string]bool) {
	t.Helper()

	tabOfProperty := make(map[string]string)
	tabs := make(map[string]bool, len(d.tabs))
	for _, tab := range d.tabs {
		require.NotEmptyf(t, tab.Title, "%s has a tab with no title", d.path)
		tabs[tab.Title] = false
		for _, field := range tab.Fields {
			require.NotContainsf(t, tabOfProperty, field, "%s lists property %q on more than one tab", d.path, field)
			tabOfProperty[field] = tab.Title
		}
	}
	return tabOfProperty, tabs
}

type metadataOption struct {
	Name        string `yaml:"name"`
	Group       string `yaml:"group"`
	Description string `yaml:"description"`
}

func readMetadataOptions(t testing.TB, metadataPath string) []metadataOption {
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
