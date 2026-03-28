// SPDX-License-Identifier: GPL-3.0-or-later

package promprofiles

import (
	"bytes"
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

const profilesDirName = "prometheus.profiles"

type Catalog struct {
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

func DefaultCatalog() (Catalog, error) {
	return LoadFromDirs(defaultDirSpecs())
}

func LoadFromDirs(specs []DirSpec) (Catalog, error) {
	catalog := Catalog{
		byID:            make(map[string]Profile),
		stockProfileIDs: make(map[string]struct{}),
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
				// User override wins.
			}

			return nil
		})
		if err != nil {
			return Catalog{}, err
		}
	}

	for id, entry := range seen {
		catalog.byID[id] = entry.Config
	}

	return catalog, nil
}

func (c Catalog) Empty() bool { return len(c.byID) == 0 }

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
		return nil, fmt.Errorf("no Prometheus profiles selected")
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
	if dir := filepath.Join(pluginconfig.CollectorsStockDir(), profilesDirName, "default"); dir != "" {
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

func normalizeKey(v string) string {
	return strings.ToLower(strings.TrimSpace(v))
}
