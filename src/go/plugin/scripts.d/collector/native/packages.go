// SPDX-License-Identifier: GPL-3.0-or-later

package native

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"regexp"
	"strings"

	"github.com/netdata/netdata/go/plugins/pkg/funcapi"
	"github.com/netdata/netdata/go/plugins/plugin/framework/collectorapi"
	"github.com/netdata/netdata/go/plugins/plugin/go.d/pkg/pathvalidate"
	"github.com/netdata/netdata/go/plugins/plugin/scripts.d/collector/native/nativefunc"
	"gopkg.in/yaml.v2"
)

var packageName = regexp.MustCompile(`^[a-z][a-z0-9]*(?:-[a-z0-9]+)*$`)

// LoadPackages returns a startup registry. Neither inventory loading nor creator
// construction executes a script. The caller owns discovery and enablement.
func LoadPackages(path string, base collectorapi.Registry) (collectorapi.Registry, error) {
	return loadPackages(path, base, pathvalidate.ValidateBinaryPath)
}

func loadPackages(
	path string,
	base collectorapi.Registry,
	validate func(string) (string, error),
) (collectorapi.Registry, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("read package inventory: %w", err)
	}
	var inventory struct {
		Version  string `yaml:"version"`
		Packages []struct {
			Name     string `yaml:"name"`
			Manifest string `yaml:"manifest"`
		} `yaml:"packages"`
	}
	decoder := yaml.NewDecoder(bytes.NewReader(data))
	decoder.SetStrict(true)
	if decoder.Decode(&inventory) != nil {
		return nil, fmt.Errorf("invalid package inventory")
	}
	var extra any
	if decoder.Decode(&extra) != io.EOF || inventory.Version != "v1" {
		return nil, fmt.Errorf("package inventory requires one v1 document")
	}
	registry := make(collectorapi.Registry, len(base)+len(inventory.Packages))
	for name, creator := range base {
		registry[name] = creator
	}
	for _, entry := range inventory.Packages {
		if !packageName.MatchString(entry.Name) {
			return nil, fmt.Errorf("package name must contain lowercase words separated by hyphens")
		}
		name := "native-" + entry.Name
		if _, exists := registry[name]; exists {
			return nil, fmt.Errorf("duplicate package module %q", name)
		}
		definition, templates, err := loadManifest(entry.Manifest, validate)
		if err != nil {
			return nil, fmt.Errorf("package %s: %w", name, err)
		}
		schema, err := packageForm(name, definition)
		if err != nil {
			return nil, err
		}
		creator := collectorapi.Creator{
			Defaults: collectorapi.Defaults{
				UpdateEvery: 10,
			},
			JobConfigSchema: schema,
			CreateV2: func() collectorapi.CollectorV2 {
				c := New()
				c.bound = true
				c.definition = definition
				c.templates = templates
				c.ScriptConfig = definition.config.effective(nil)
				return c
			},
			Config: func() any {
				return &Config{
					ScriptConfig: definition.config.effective(nil),
				}
			},
		}
		if len(definition.methods) > 0 {
			creator.FunctionOnly = definition.functionOnly()
			creator.SharedFunctions = func() []funcapi.FunctionConfig { return append([]funcapi.FunctionConfig(nil), definition.methods...) }
			creator.MethodHandler = func(job collectorapi.RuntimeJob) funcapi.MethodHandler {
				c, ok := job.Collector().(*Collector)
				if !ok {
					return nil
				}
				return nativefunc.New(c, c.definition.methods)
			}
		}
		registry.Register(name, creator)
	}
	return registry, nil
}

func packageForm(name string, definition manifest) (string, error) {
	config := definition.config
	var form map[string]any
	if json.Unmarshal([]byte(configSchema), &form) != nil {
		return "", fmt.Errorf("invalid native form")
	}
	schema := form["jsonSchema"].(map[string]any)
	schema["title"] = name + " collector configuration."
	properties := schema["properties"].(map[string]any)
	delete(properties, "manifest")
	delete(properties, "config")
	delete(schema, "required")
	ui := form["uiSchema"].(map[string]any)
	delete(ui, "manifest")
	if definition.functionOnly() {
		delete(properties, "update_every")
		if definition.Mode == modePersistent {
			properties["timeout"].(map[string]any)["description"] = "Timeout in seconds for persistent startup and for draining a Function reply after caller cancellation. Function callers use their own deadline."
		} else {
			delete(properties, "timeout")
		}
	}
	if config != nil {
		cloned, _ := jsonValue(config.document)
		child := cloned.(map[string]any)
		_ = walkSchema(child, "", true, func(value any, _ string, _ bool) error {
			node, ok := value.(map[string]any)
			if !ok {
				return nil
			}
			if ref, ok := node["$ref"].(string); ok {
				node["$ref"] = "#/properties/config" + strings.TrimPrefix(ref, "#")
			}
			return nil
		})
		// The returned job form embeds the package schema and rebases its local refs.
		properties["config"] = child
		ui["config"] = config.ui
	}
	data, err := json.Marshal(form)
	if err != nil {
		return "", fmt.Errorf("cannot encode package form")
	}
	return string(data), nil
}
