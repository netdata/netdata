// SPDX-License-Identifier: GPL-3.0-or-later

package native

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"maps"
	"os"
	"path/filepath"
	"regexp"
	"slices"

	"github.com/netdata/netdata/go/plugins/pkg/funcapi"
	"github.com/netdata/netdata/go/plugins/plugin/framework/collectorapi"
	"github.com/netdata/netdata/go/plugins/plugin/go.d/pkg/pathvalidate"
)

const packageModulePrefix = "native-"

var rePackageName = regexp.MustCompile(`^[a-z][a-z0-9]*(?:-[a-z0-9]+)*$`)

// packageInventory is scripts.d.packages.yaml.
type packageInventory struct {
	Version  string           `yaml:"version"`
	Packages []inventoryEntry `yaml:"packages"`
}

// inventoryEntry registers one package from exactly one source: a manifest
// file, or a self-contained command that describes itself.
type inventoryEntry struct {
	Name     string   `yaml:"name"`
	Manifest string   `yaml:"manifest"`
	Command  []string `yaml:"command"`
}

// LoadPackages returns a startup registry. Command entries execute describe once;
// file entries and creator construction remain passive. The caller owns enablement.
func LoadPackages(ctx context.Context, path string, base collectorapi.Registry) (collectorapi.Registry, error) {
	return loadPackages(ctx, path, base, pathvalidate.ValidateBinaryPath)
}

func loadPackages(
	ctx context.Context,
	path string,
	base collectorapi.Registry,
	validateExecutable func(string) (string, error),
) (collectorapi.Registry, error) {
	inventory, err := readInventory(path)
	if err != nil {
		return nil, err
	}
	// Build a new registry so a failure never partially mutates base.
	registry := make(collectorapi.Registry, len(base)+len(inventory.Packages))
	maps.Copy(registry, base)
	for _, entry := range inventory.Packages {
		if !rePackageName.MatchString(entry.Name) {
			return nil, errors.New("package name must contain lowercase words separated by hyphens")
		}
		name := packageModulePrefix + entry.Name
		if _, exists := registry[name]; exists {
			return nil, fmt.Errorf("duplicate package module %q", name)
		}
		if err := ctx.Err(); err != nil {
			return nil, err
		}
		definition, err := entry.load(ctx, validateExecutable)
		if err != nil {
			return nil, fmt.Errorf("package %s: %w", name, err)
		}
		creator, err := packageCreator(name, definition)
		if err != nil {
			return nil, err
		}
		registry.Register(name, creator)
	}
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	return registry, nil
}

func readInventory(path string) (packageInventory, error) {
	var inventory packageInventory
	data, err := os.ReadFile(path)
	if err != nil {
		return inventory, fmt.Errorf("read package inventory: %w", err)
	}
	if err := decodeYAMLDocument(data, &inventory); err != nil {
		return inventory, fmt.Errorf("package inventory %w", err)
	}
	if inventory.Version != "v1" {
		return inventory, errors.New("unsupported package inventory version")
	}
	return inventory, nil
}

func (e inventoryEntry) load(
	ctx context.Context,
	validateExecutable func(string) (string, error),
) (packageDefinition, error) {
	if (e.Manifest == "") == (e.Command == nil) {
		return packageDefinition{}, errors.New("requires exactly one manifest or command")
	}
	if e.Command == nil {
		return loadManifest(e.Manifest, validateExecutable)
	}
	command, err := validatedCommand(e.Command, validateExecutable)
	if err != nil {
		return packageDefinition{}, err
	}
	return loadDescribedPackage(ctx, command)
}

// validatedCommand shares the executable policy of direct and registered jobs.
func validatedCommand(command []string, validateExecutable func(string) (string, error)) ([]string, error) {
	if len(command) == 0 || !filepath.IsAbs(command[0]) {
		return nil, errors.New("command requires an absolute executable")
	}
	command = slices.Clone(command)
	var err error
	if command[0], err = validateExecutable(command[0]); err != nil {
		return nil, err
	}
	return command, nil
}

// packageCreator registers a package as its own module. Every job receives the
// startup definition; package files are never reread.
func packageCreator(name string, definition packageDefinition) (collectorapi.Creator, error) {
	schema, err := packageForm(name, definition)
	if err != nil {
		return collectorapi.Creator{}, err
	}
	creator := collectorapi.Creator{
		Defaults: collectorapi.Defaults{
			UpdateEvery: defaultUpdateEvery,
		},
		JobConfigSchema: schema,
		CreateV2: func() collectorapi.CollectorV2 {
			c := New()
			c.registered = true
			c.definition = definition
			c.ScriptConfig = definition.effectiveSettings(nil)
			return c
		},
		Config: func() any {
			return &Config{
				ScriptConfig: definition.effectiveSettings(nil),
			}
		},
	}
	if len(definition.methods) > 0 {
		creator.FunctionOnly = definition.functionOnly()
		creator.SharedFunctions = func() []funcapi.FunctionConfig { return slices.Clone(definition.methods) }
		creator.MethodHandler = functionHandler
	}
	return creator, nil
}

// packageForm derives a package's job form from the generic native form: no
// source or mode options, and the package configuration form embedded as config.
func packageForm(name string, definition packageDefinition) (string, error) {
	var form map[string]any
	if json.Unmarshal([]byte(configSchema), &form) != nil {
		return "", errors.New("invalid native form")
	}
	schema := form["jsonSchema"].(map[string]any)
	schema["title"] = name + " collector configuration."
	delete(schema, "if")
	delete(schema, "then")
	delete(schema, "else")
	properties := schema["properties"].(map[string]any)
	delete(properties, "manifest")
	delete(properties, "command")
	delete(properties, "mode")
	delete(properties, "snapshot_format")
	delete(properties, "config")
	ui := form["uiSchema"].(map[string]any)
	delete(ui, "manifest")
	delete(ui, "command")
	delete(ui, "mode")
	delete(ui, "snapshot_format")
	delete(ui, "config")
	if definition.functionOnly() {
		delete(properties, "update_every")
		if definition.Mode == modePersistent {
			properties["timeout"].(map[string]any)["description"] = "Timeout in seconds for persistent startup " +
				"and for draining a Function reply after caller cancellation. Function callers use their own deadline."
		} else {
			delete(properties, "timeout")
		}
	}
	if definition.form != nil {
		properties["config"], ui["config"] = definition.form.Embed("#/properties/config")
	}
	data, err := json.Marshal(form)
	if err != nil {
		return "", errors.New("cannot encode package form")
	}
	return string(data), nil
}
