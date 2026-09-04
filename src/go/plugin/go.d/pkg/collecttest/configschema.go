// SPDX-License-Identifier: GPL-3.0-or-later

package collecttest

import (
	"encoding/json"
	"maps"
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

		node, ok := schema.node(option.Name)
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
}

type schemaTab struct {
	Title  string   `json:"title"`
	Fields []string `json:"fields"`
}

// configSchemaDocument is a loaded config_schema.json.
type configSchemaDocument struct {
	path   string
	tabs   []schemaTab
	schema map[string]any // the jsonSchema member
}

func readConfigSchema(t testing.TB, schemaPath string) configSchemaDocument {
	t.Helper()

	data, err := os.ReadFile(schemaPath)
	require.NoError(t, err)
	var doc struct {
		JSONSchema map[string]any `json:"jsonSchema"`
		UISchema   struct {
			Options struct {
				Tabs []schemaTab `json:"tabs"`
			} `json:"ui:options"`
		} `json:"uiSchema"`
	}
	require.NoError(t, json.Unmarshal(data, &doc))
	require.NotEmptyf(t, doc.UISchema.Options.Tabs, "%s declares no uiSchema tabs", schemaPath)
	return configSchemaDocument{path: schemaPath, tabs: doc.UISchema.Options.Tabs, schema: doc.JSONSchema}
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

// node resolves a documented option name ("rules[].query.period", "mode_filters.regions") to its
// schema object, descending through properties, the properties every dependencies branch reveals,
// and array items, with local $ref and allOf composition applied at each step.
func (d configSchemaDocument) node(option string) (map[string]any, bool) {
	node := d.resolve(d.schema)
	for segment := range strings.SplitSeq(option, ".") {
		name := strings.TrimSuffix(segment, "[]")
		child, ok := d.properties(node)[name]
		if !ok {
			return nil, false
		}
		node = child
		if strings.HasSuffix(segment, "[]") {
			items, _ := node["items"].(map[string]any)
			if items == nil {
				return nil, false
			}
			node = d.resolve(items)
		}
	}
	return node, true
}

// properties returns a node's properties plus the properties its dependencies branches reveal.
func (d configSchemaDocument) properties(node map[string]any) map[string]map[string]any {
	out := map[string]map[string]any{}
	if props, ok := node["properties"].(map[string]any); ok {
		for name, child := range props {
			if obj, ok := child.(map[string]any); ok {
				out[name] = d.resolve(obj)
			}
		}
	}
	deps, _ := node["dependencies"].(map[string]any)
	for discriminator, raw := range deps {
		dep, ok := raw.(map[string]any)
		if !ok {
			continue // a property dependency (list of names) reveals nothing
		}
		branches := []any{dep}
		for _, keyword := range []string{"oneOf", "anyOf"} {
			if list, ok := dep[keyword].([]any); ok {
				branches = list
				break
			}
		}
		for _, raw := range branches {
			branch, _ := raw.(map[string]any)
			props, _ := d.resolve(branch)["properties"].(map[string]any)
			for name, child := range props {
				obj, ok := child.(map[string]any)
				if !ok || name == discriminator {
					continue
				}
				if _, seen := out[name]; !seen {
					out[name] = d.resolve(obj)
				}
			}
		}
	}
	return out
}

// resolve follows a local $ref and folds allOf members into one object, so callers see the
// properties a composed node has. Properties merge; every other keyword is last-writer-wins, with
// the node's own keywords winning over the composed ones.
func (d configSchemaDocument) resolve(node map[string]any) map[string]any {
	if node == nil {
		return nil
	}
	out := map[string]any{}
	merge := func(src map[string]any) {
		for key, value := range src {
			existing, _ := out[key].(map[string]any)
			incoming, _ := value.(map[string]any)
			if key == "properties" && existing != nil && incoming != nil {
				merged := make(map[string]any, len(existing)+len(incoming))
				maps.Copy(merged, existing)
				maps.Copy(merged, incoming)
				out[key] = merged
				continue
			}
			out[key] = value
		}
	}
	if ref, ok := node["$ref"].(string); ok && strings.HasPrefix(ref, "#/") {
		var target any = d.schema
		for part := range strings.SplitSeq(ref[2:], "/") {
			obj, _ := target.(map[string]any)
			target = obj[part]
		}
		if obj, ok := target.(map[string]any); ok {
			merge(d.resolve(obj))
		}
	}
	if members, ok := node["allOf"].([]any); ok {
		for _, member := range members {
			if obj, ok := member.(map[string]any); ok {
				merge(d.resolve(obj))
			}
		}
	}
	own := make(map[string]any, len(node))
	for key, value := range node {
		if key != "$ref" && key != "allOf" {
			own[key] = value
		}
	}
	merge(own)
	return out
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
