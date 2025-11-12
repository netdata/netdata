// SPDX-License-Identifier: GPL-3.0-or-later

package config

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/bmatcuk/doublestar/v4"

	"github.com/netdata/netdata/go/plugins/plugin/scripts.d/pkg/spec"
)

// DirectoryConfig describes how to turn every executable inside a directory into
// a Nagios job definition.
type DirectoryConfig struct {
	NamePrefix  string            `yaml:"name_prefix,omitempty" json:"name_prefix"`
	Path        string            `yaml:"path" json:"path"`
	Recursive   bool              `yaml:"recursive,omitempty" json:"recursive"`
	Include     []string          `yaml:"include,omitempty" json:"include"`
	Exclude     []string          `yaml:"exclude,omitempty" json:"exclude"`
	Args        []string          `yaml:"args,omitempty" json:"args"`
	ArgValues   []string          `yaml:"arg_values,omitempty" json:"arg_values"`
	Environment map[string]string `yaml:"environment,omitempty" json:"environment"`
	CustomVars  map[string]string `yaml:"custom_vars,omitempty" json:"custom_vars"`
	Defaults    Defaults          `yaml:"defaults,omitempty" json:"defaults"`
}

var (
	errEmptyDirectoryPath = errors.New("directory path is required")
)

// Expand walks the configured directory and produces per-script job configs.
func (d DirectoryConfig) Expand(base Defaults) ([]spec.JobConfig, error) {
	if d.Path == "" {
		return nil, errEmptyDirectoryPath
	}

	patterns := d.Include
	if len(patterns) == 0 {
		patterns = []string{"*"}
	}

	matcher := func(name string, pats []string) bool {
		for _, p := range pats {
			if ok, _ := doublestar.Match(p, name); ok {
				return true
			}
		}
		return false
	}

	matchesExclude := func(name string) bool {
		if len(d.Exclude) == 0 {
			return false
		}
		for _, p := range d.Exclude {
			if ok, _ := doublestar.Match(p, name); ok {
				return true
			}
		}
		return false
	}

	entries := make([]spec.JobConfig, 0)
	effectiveDefaults := base.Merge(d.Defaults)
	visit := func(path string, dirEntry os.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if dirEntry.IsDir() {
			if strings.EqualFold(path, d.Path) {
				return nil
			}
			if !d.Recursive {
				return filepath.SkipDir
			}
			return nil
		}

		if !dirEntry.Type().IsRegular() {
			return nil
		}
		info, err := dirEntry.Info()
		if err != nil {
			return err
		}
		if info.Mode()&0o111 == 0 {
			return nil
		}

		basename := dirEntry.Name()
		if matchesExclude(basename) {
			return nil
		}
		if !matcher(basename, patterns) {
			return nil
		}

		job := spec.JobConfig{
			Name:        d.composeName(basename),
			Plugin:      path,
			Args:        append([]string{}, d.Args...),
			ArgValues:   append([]string{}, d.ArgValues...),
			Environment: cloneMap(d.Environment),
			CustomVars:  cloneMap(d.CustomVars),
		}

		effectiveDefaults.Apply(&job)
		entries = append(entries, job)
		return nil
	}

	if d.Recursive {
		if err := filepath.WalkDir(d.Path, visit); err != nil {
			return nil, err
		}
	} else {
		dirEntries, err := os.ReadDir(d.Path)
		if err != nil {
			return nil, err
		}
		for _, entry := range dirEntries {
			if err := visit(filepath.Join(d.Path, entry.Name()), entry, nil); err != nil {
				return nil, err
			}
		}
	}

	return entries, nil
}

func (d DirectoryConfig) composeName(base string) string {
	name := strings.TrimSuffix(base, filepath.Ext(base))
	if d.NamePrefix != "" {
		return fmt.Sprintf("%s%s", d.NamePrefix, name)
	}
	return name
}

func cloneMap(m map[string]string) map[string]string {
	if len(m) == 0 {
		return map[string]string{}
	}
	res := make(map[string]string, len(m))
	for k, v := range m {
		res[k] = v
	}
	return res
}
