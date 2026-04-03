// SPDX-License-Identifier: GPL-3.0-or-later

package promprofiles

import (
	"bytes"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"runtime"
	"strings"

	"gopkg.in/yaml.v3"

	"github.com/netdata/netdata/go/plugins/pkg/executable"
	"github.com/netdata/netdata/go/plugins/pkg/pluginconfig"
)

const profilesDirName = "prometheus.profiles"

type Catalog struct {
	byName      map[string]Profile
	orderedKeys []string
}

type catalogEntry struct {
	config  Profile
	path    string
	isStock bool
}

type DirSpec struct {
	Path    string
	IsStock bool
}

func loadFromDefaultDirs() (Catalog, error) {
	return LoadFromDirs(defaultDirSpecs())
}

func LoadFromDirs(specs []DirSpec) (Catalog, error) {
	catalog := Catalog{
		byName: make(map[string]Profile),
	}
	if len(specs) == 0 {
		return catalog, nil
	}

	seen := make(map[string]catalogEntry)
	for _, spec := range specs {
		if strings.TrimSpace(spec.Path) == "" {
			continue
		}
		if !isDirExists(spec.Path) {
			if spec.IsStock {
				return Catalog{}, fmt.Errorf("stock profiles directory does not exist: %s", spec.Path)
			}
			continue
		}

		err := filepath.WalkDir(spec.Path, func(path string, d fs.DirEntry, err error) error {
			if err != nil {
				return err
			}
			if d.IsDir() {
				return nil
			}

			name := d.Name()
			ext := strings.ToLower(filepath.Ext(name))
			if ext != ".yaml" && ext != ".yml" {
				return nil
			}
			if strings.HasPrefix(name, "_") {
				return nil
			}

			cfg, err := loadProfileFile(path)
			if err != nil {
				return err
			}

			key := normalizeKey(cfg.Name)
			if key == "" {
				return fmt.Errorf("profile %q: decoded empty name", path)
			}

			prev, exists := seen[key]
			if !exists {
				seen[key] = catalogEntry{config: cfg, path: path, isStock: spec.IsStock}
				catalog.orderedKeys = append(catalog.orderedKeys, key)
				return nil
			}

			switch {
			case prev.isStock == spec.IsStock:
				scope := "user"
				if spec.IsStock {
					scope = "stock"
				}
				return fmt.Errorf("duplicate %s profile name %q in %q and %q", scope, cfg.Name, prev.path, path)
			case prev.isStock && !spec.IsStock:
				seen[key] = catalogEntry{config: cfg, path: path, isStock: false}
			case !prev.isStock && spec.IsStock:
				// User override wins.
			}

			return nil
		})
		if err != nil {
			return Catalog{}, err
		}
	}

	for _, key := range catalog.orderedKeys {
		catalog.byName[key] = seen[key].config
	}

	return catalog, nil
}

func (c Catalog) Empty() bool { return len(c.byName) == 0 }

func (c Catalog) OrderedProfiles() []Profile {
	if len(c.orderedKeys) == 0 {
		return nil
	}

	profiles := make([]Profile, 0, len(c.orderedKeys))
	for _, key := range c.orderedKeys {
		profiles = append(profiles, c.byName[key])
	}
	return profiles
}

func (c Catalog) Resolve(profileNames []string) ([]Profile, error) {
	if len(profileNames) == 0 {
		return nil, fmt.Errorf("no Prometheus profiles selected")
	}

	profiles := make([]Profile, 0, len(profileNames))
	for _, name := range profileNames {
		normalizedName := normalizeKey(name)
		prof, ok := c.byName[normalizedName]
		if !ok {
			return nil, fmt.Errorf("unknown profile %q", name)
		}
		profiles = append(profiles, prof)
	}
	return profiles, nil
}

func loadProfileFile(path string) (Profile, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return Profile{}, err
	}

	var cfg Profile
	dec := yaml.NewDecoder(bytes.NewReader(data))
	dec.KnownFields(true)
	if err := dec.Decode(&cfg); err != nil {
		return Profile{}, fmt.Errorf("unmarshal profile %q: %w", path, err)
	}

	profileName := strings.TrimSpace(cfg.Name)
	if profileName == "" {
		return Profile{}, fmt.Errorf("validate profile %q: missing required field 'name'", path)
	}
	if err := cfg.validate(fmt.Sprintf("profile %q", profileName)); err != nil {
		return Profile{}, fmt.Errorf("validate profile %q: %w", path, err)
	}

	return cfg, nil
}

func defaultDirSpecs() []DirSpec {
	if executable.Name == "test" {
		if dir := profilesDirFromThisFile(); dir != "" {
			return []DirSpec{{Path: dir, IsStock: true}}
		}
		return nil
	}

	if dir := filepath.Join(executable.Directory, "../config/go.d", profilesDirName, "default"); isDirExists(dir) {
		return []DirSpec{{Path: dir, IsStock: true}}
	}

	specs := make([]DirSpec, 0, len(pluginconfig.CollectorsUserDirs())+1)
	for _, dir := range pluginconfig.CollectorsUserDirs() {
		specs = append(specs, DirSpec{
			Path:    filepath.Join(dir, profilesDirName),
			IsStock: false,
		})
	}
	if dir := filepath.Join(pluginconfig.CollectorsStockDir(), profilesDirName, "default"); dir != "" && isDirExists(dir) {
		specs = append(specs, DirSpec{
			Path:    dir,
			IsStock: true,
		})
	}

	return specs
}

func profilesDirFromThisFile() string {
	_, thisFile, _, ok := runtime.Caller(0)
	if !ok {
		return ""
	}

	base := filepath.Dir(thisFile)
	candidate := filepath.Join(base, "..", "..", "..", "..", "config", "go.d", profilesDirName, "default")
	if !isDirExists(candidate) {
		return ""
	}

	abs, _ := filepath.Abs(candidate)
	return abs
}

func isDirExists(dir string) bool {
	fi, err := os.Stat(dir)
	if err != nil {
		return false
	}
	return fi.Mode().IsDir()
}

func NormalizeProfileKey(v string) string { return strings.ToLower(strings.TrimSpace(v)) }

func normalizeKey(v string) string { return NormalizeProfileKey(v) }
