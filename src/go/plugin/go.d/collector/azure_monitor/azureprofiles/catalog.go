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
	byName          map[string]Profile
	byID            map[string]Profile
	stockProfileIDs map[string]struct{}
}

type catalogEntry struct {
	Config  Profile
	Path    string
	IsStock bool
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
	if len(catalog.stockProfileIDs) == 0 {
		return Catalog{}, fmt.Errorf("no stock profiles found under %q", filepath.Join(pluginconfig.CollectorsStockDir(), profilesDirName, "default"))
	}
	return catalog, nil
}

func LoadFromDirs(specs []DirSpec) (Catalog, error) {
	catalog := Catalog{
		byName:          make(map[string]Profile),
		byID:            make(map[string]Profile),
		stockProfileIDs: make(map[string]struct{}),
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

			id := normalizeKey(cfg.ID)
			if id == "" {
				return fmt.Errorf("profile %q: decoded empty id", path)
			}
			if spec.IsStock {
				catalog.stockProfileIDs[id] = struct{}{}
			}

			prev, exists := seen[id]
			if !exists {
				seen[id] = catalogEntry{Config: cfg, Path: path, IsStock: spec.IsStock}
				return nil
			}

			switch {
			case prev.IsStock == spec.IsStock:
				scope := "user"
				if spec.IsStock {
					scope = "stock"
				}
				return fmt.Errorf("duplicate %s profile id %q in %q and %q", scope, id, prev.Path, path)
			case prev.IsStock && !spec.IsStock:
				seen[id] = catalogEntry{Config: cfg, Path: path, IsStock: false}
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

	for id, entry := range seen {
		catalog.byID[id] = entry.Config

		name := strings.TrimSpace(entry.Config.Name)
		nameKey := normalizeKey(name)
		if nameKey == "" {
			return Catalog{}, fmt.Errorf("profile %q: decoded empty name", entry.Path)
		}
		if prev, ok := catalog.byName[nameKey]; ok {
			return Catalog{}, fmt.Errorf("duplicate profile name %q for ids %q and %q", name, prev.ID, entry.Config.ID)
		}
		catalog.byName[nameKey] = entry.Config
	}

	return catalog, nil
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

	profileID := strings.TrimSpace(cfg.ID)
	if profileID == "" {
		return Profile{}, fmt.Errorf("validate profile %q: missing required field 'id'", path)
	}
	if err := cfg.Validate(fmt.Sprintf("profile %q", profileID)); err != nil {
		return Profile{}, fmt.Errorf("validate profile %q: %w", path, err)
	}

	return cfg, nil
}

func (c Catalog) DefaultProfileIDs() []string {
	if len(c.stockProfileIDs) == 0 {
		return nil
	}

	ids := make([]string, 0, len(c.stockProfileIDs))
	for id := range c.stockProfileIDs {
		ids = append(ids, id)
	}
	sort.Strings(ids)
	return ids
}

func (c Catalog) Resolve(profileIDs []string) ([]Profile, error) {
	if len(profileIDs) == 0 {
		return nil, errors.New("no Azure Monitor profiles selected")
	}

	profiles := make([]Profile, 0, len(profileIDs))
	for _, id := range profileIDs {
		normalizedID := normalizeKey(id)
		prof, ok := c.byID[normalizedID]
		if !ok {
			return nil, fmt.Errorf("unknown profile %q", id)
		}
		profiles = append(profiles, prof)
	}
	return profiles, nil
}

func (c Catalog) ResolveNames(profileNames []string) ([]Profile, error) {
	if len(profileNames) == 0 {
		return nil, errors.New("no Azure Monitor profiles selected")
	}

	profiles := make([]Profile, 0, len(profileNames))
	for _, name := range profileNames {
		normalizedName := normalizeKey(name)
		prof, ok := c.byName[normalizedName]
		if !ok {
			return nil, fmt.Errorf("unknown profile name %q", name)
		}
		profiles = append(profiles, prof)
	}
	return profiles, nil
}

func (c Catalog) ProfilesForResourceTypes(types map[string]struct{}) []string {
	if len(types) == 0 {
		return nil
	}

	var ids []string
	for id, prof := range c.byID {
		rt := normalizeKey(prof.ResourceType)
		if rt == "" {
			continue
		}
		if _, ok := types[rt]; ok {
			ids = append(ids, id)
		}
	}
	sort.Strings(ids)
	return ids
}

func (c Catalog) ResourceTypesForProfileIDs(profileIDs []string) ([]string, error) {
	if len(profileIDs) == 0 {
		return nil, nil
	}

	seen := make(map[string]struct{}, len(profileIDs))
	types := make([]string, 0, len(profileIDs))
	for _, id := range profileIDs {
		prof, ok := c.byID[normalizeKey(id)]
		if !ok {
			return nil, fmt.Errorf("unknown profile %q", id)
		}

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
	return types, nil
}

func (c Catalog) ProfileIDsForNames(profileNames []string) ([]string, error) {
	profiles, err := c.ResolveNames(profileNames)
	if err != nil {
		return nil, err
	}

	ids := make([]string, 0, len(profiles))
	for _, profile := range profiles {
		ids = append(ids, profile.ID)
	}
	return ids, nil
}

func (c Catalog) ResourceTypesForProfileNames(profileNames []string) ([]string, error) {
	profiles, err := c.ResolveNames(profileNames)
	if err != nil {
		return nil, err
	}

	seen := make(map[string]struct{}, len(profiles))
	types := make([]string, 0, len(profiles))
	for _, profile := range profiles {
		rt := strings.TrimSpace(profile.ResourceType)
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

func (c Catalog) ResourceTypes() []string {
	if len(c.byID) == 0 {
		return nil
	}

	seen := make(map[string]struct{}, len(c.byID))
	types := make([]string, 0, len(c.byID))
	for _, prof := range c.byID {
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
