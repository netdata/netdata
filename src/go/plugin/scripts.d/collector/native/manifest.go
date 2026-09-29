// SPDX-License-Identifier: GPL-3.0-or-later

package native

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"

	"github.com/netdata/netdata/go/plugins/plugin/scripts.d/collector/native/internal/configform"
)

// manifestFile is the file-backed package format. Relative paths resolve
// against the manifest directory.
type manifestFile struct {
	packageSpec  `yaml:",inline"`
	Command      []string `yaml:"command"`
	Charts       string   `yaml:"charts"`
	ConfigSchema string   `yaml:"config_schema"`
}

// loadManifest reads and validates a file-backed package without running it.
func loadManifest(path string, validateExecutable func(string) (string, error)) (packageDefinition, error) {
	if !filepath.IsAbs(path) {
		return packageDefinition{}, errors.New("manifest must be an absolute path")
	}
	data, err := os.ReadFile(path)
	if err != nil {
		return packageDefinition{}, fmt.Errorf("read manifest: %w", err)
	}
	var file manifestFile
	if err := decodeYAMLDocument(data, &file); err != nil {
		return packageDefinition{}, fmt.Errorf("manifest %w", err)
	}
	if len(file.Command) == 0 || file.Command[0] == "" {
		return packageDefinition{}, errors.New("manifest command is required")
	}
	dir := filepath.Dir(path)
	command := file.Command
	if command[0], err = validateExecutable(resolvePath(dir, command[0])); err != nil {
		return packageDefinition{}, fmt.Errorf("validate executable: %w", err)
	}
	def, err := newPackageDefinition(file.packageSpec, command)
	if err != nil {
		return packageDefinition{}, err
	}
	if file.ConfigSchema != "" {
		data, err := os.ReadFile(resolvePath(dir, file.ConfigSchema))
		if err != nil {
			return packageDefinition{}, fmt.Errorf("read package config schema: %w", err)
		}
		if def.form, err = configform.Parse(data); err != nil {
			return packageDefinition{}, err
		}
	}
	var charts []byte
	if file.Charts != "" {
		if charts, err = os.ReadFile(resolvePath(dir, file.Charts)); err != nil {
			return packageDefinition{}, fmt.Errorf("read charts: %w", err)
		}
	}
	if def.templates, err = def.chartTemplates(charts); err != nil {
		return packageDefinition{}, err
	}
	return def, nil
}

func resolvePath(dir, path string) string {
	if filepath.IsAbs(path) {
		return path
	}
	return filepath.Join(dir, path)
}
