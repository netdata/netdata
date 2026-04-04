// SPDX-License-Identifier: GPL-3.0-or-later

package azureprofiles

import (
	"bytes"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"runtime"
	"sort"
	"strings"

	"gopkg.in/yaml.v3"

	"github.com/netdata/netdata/go/plugins/pkg/executable"
	"github.com/netdata/netdata/go/plugins/pkg/pluginconfig"
)

const (
	profilesDirName = "azure_monitor.profiles"
)

type Catalog struct {
	byBaseName            map[string]Profile
	stockProfileBaseNames map[string]struct{}
}

type ResolvedProfile struct {
	Name   string
	Config Profile
}

type catalogEntry struct {
	Config   Profile
	Path     string
	BaseName string
	IsStock  bool
}

type DirSpec struct {
	Path    string
	IsStock bool
}

func LoadFromDefaultDirs() (Catalog, error) {
	specs := defaultDirSpecs()
	if len(specs) == 0 {
		return Catalog{}, errors.New("no profile search directories configured")
	}

	catalog, err := LoadFromDirs(specs)
	if err != nil {
		return Catalog{}, err
	}
	if len(catalog.stockProfileBaseNames) == 0 {
		return Catalog{}, fmt.Errorf("no stock profiles found under %q", filepath.Join(pluginconfig.CollectorsStockDir(), profilesDirName, "default"))
	}
	return catalog, nil
}

func LoadFromDirs(specs []DirSpec) (Catalog, error) {
	catalog := Catalog{
		byBaseName:            make(map[string]Profile),
		stockProfileBaseNames: make(map[string]struct{}),
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

			baseName := strings.TrimSpace(profileBaseName(path))
			if !IsValidProfileName(baseName) {
				return fmt.Errorf("profile %q: basename must match %q", path, reIdentityID.String())
			}

			cfg, err := loadProfileFile(path, baseName)
			if err != nil {
				return err
			}
			if spec.IsStock {
				catalog.stockProfileBaseNames[baseName] = struct{}{}
			}

			prev, exists := seen[baseName]
			if !exists {
				seen[baseName] = catalogEntry{Config: cfg, Path: path, BaseName: baseName, IsStock: spec.IsStock}
				return nil
			}

			switch {
			case prev.IsStock == spec.IsStock:
				scope := "user"
				if spec.IsStock {
					scope = "stock"
				}
				return fmt.Errorf("duplicate %s profile basename %q in %q and %q", scope, baseName, prev.Path, path)
			case prev.IsStock && !spec.IsStock:
				seen[baseName] = catalogEntry{Config: cfg, Path: path, BaseName: baseName, IsStock: false}
			case !prev.IsStock && spec.IsStock:
				// User overrides stock. Keep the existing user profile.
			}

			return nil
		})
		if err != nil {
			return Catalog{}, err
		}
	}

	if len(seen) == 0 {
		return Catalog{}, errors.New("no Azure Monitor profiles were loaded")
	}

	for baseName, entry := range seen {
		catalog.byBaseName[baseName] = entry.Config
	}

	return catalog, nil
}

func loadProfileFile(path, baseName string) (Profile, error) {
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

	if err := cfg.Normalize(baseName); err != nil {
		return Profile{}, fmt.Errorf("normalize profile %q: %w", path, err)
	}
	if err := cfg.Validate(fmt.Sprintf("profile %q", baseName), baseName); err != nil {
		return Profile{}, fmt.Errorf("validate profile %q: %w", path, err)
	}

	return cfg, nil
}

func (c Catalog) Resolve(profileNames []string) ([]ResolvedProfile, error) {
	if len(profileNames) == 0 {
		return nil, errors.New("no Azure Monitor profiles selected")
	}

	profiles := make([]ResolvedProfile, 0, len(profileNames))
	for _, name := range profileNames {
		profileName := strings.TrimSpace(name)
		prof, ok := c.byBaseName[profileName]
		if !ok {
			return nil, fmt.Errorf("unknown profile %q", name)
		}
		profiles = append(profiles, ResolvedProfile{Name: profileName, Config: prof})
	}
	return profiles, nil
}

func (c Catalog) ResolveBaseNames(profileBaseNames []string) ([]Profile, error) {
	resolved, err := c.Resolve(profileBaseNames)
	if err != nil {
		return nil, err
	}

	profiles := make([]Profile, 0, len(resolved))
	for _, profile := range resolved {
		profiles = append(profiles, profile.Config)
	}
	return profiles, nil
}

func (c Catalog) ProfilesForResourceTypes(types map[string]struct{}) []string {
	if len(types) == 0 {
		return nil
	}

	var names []string
	for name, prof := range c.byBaseName {
		rt := normalizeKey(prof.ResourceType)
		if rt == "" {
			continue
		}
		if _, ok := types[rt]; ok {
			names = append(names, name)
		}
	}
	sort.Strings(names)
	return names
}

func (c Catalog) ResourceTypesForProfileBaseNames(profileBaseNames []string) ([]string, error) {
	resolved, err := c.Resolve(profileBaseNames)
	if err != nil {
		return nil, err
	}

	seen := make(map[string]struct{}, len(resolved))
	types := make([]string, 0, len(resolved))
	for _, resolvedProfile := range resolved {
		rt := strings.TrimSpace(resolvedProfile.Config.ResourceType)
		key := normalizeKey(rt)
		if key == "" {
			continue
		}
		if _, ok := seen[key]; ok {
			continue
		}
		seen[key] = struct{}{}
		types = append(types, rt)
	}

	sort.Strings(types)
	return types, nil
}

func (c Catalog) defaultProfileBaseNames() []string {
	if len(c.stockProfileBaseNames) == 0 {
		return nil
	}

	names := make([]string, 0, len(c.stockProfileBaseNames))
	for name := range c.stockProfileBaseNames {
		names = append(names, name)
	}
	sort.Strings(names)
	return names
}

func profileBaseName(path string) string {
	name := filepath.Base(path)
	ext := filepath.Ext(name)
	return strings.TrimSuffix(name, ext)
}

func (c Catalog) ResourceTypes() []string {
	if len(c.byBaseName) == 0 {
		return nil
	}

	seen := make(map[string]struct{}, len(c.byBaseName))
	types := make([]string, 0, len(c.byBaseName))
	for _, prof := range c.byBaseName {
		rt := strings.TrimSpace(prof.ResourceType)
		key := normalizeKey(rt)
		if key == "" {
			continue
		}
		if _, ok := seen[key]; ok {
			continue
		}
		seen[key] = struct{}{}
		types = append(types, rt)
	}

	sort.Strings(types)
	return types
}

func defaultDirSpecs() []DirSpec {
	if executable.Name == "test" {
		if dir := azureProfilesDirFromThisFile(); dir != "" {
			return []DirSpec{{Path: dir, IsStock: true}}
		}
		return nil
	}

	// Keep the adjacent stock-dir fast path for local development runs where the
	// built binary lives under src/go/plugin/go.d/bin and should use the source-tree
	// stock profiles without depending on installed pluginconfig paths.
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
	specs = append(specs, DirSpec{
		Path:    filepath.Join(pluginconfig.CollectorsStockDir(), profilesDirName, "default"),
		IsStock: true,
	})

	return specs
}

func azureProfilesDirFromThisFile() string {
	_, thisFile, _, ok := runtime.Caller(0)
	if !ok {
		return ""
	}

	base := filepath.Dir(thisFile)
	candidates := []string{
		filepath.Join(base, "..", "..", "..", "config", "go.d", profilesDirName, "default"),
	}

	for _, candidate := range candidates {
		if !isDirExists(candidate) {
			continue
		}
		abs, _ := filepath.Abs(candidate)
		return abs
	}
	return ""
}

func isDirExists(dir string) bool {
	fi, err := os.Stat(dir)
	if err != nil {
		return false
	}
	return fi.Mode().IsDir()
}

func normalizeKey(v string) string {
	return strings.ToLower(strings.TrimSpace(v))
}
