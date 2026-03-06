// SPDX-License-Identifier: GPL-3.0-or-later

package azure_monitor

import (
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
	profilesDirName      = "azure_monitor.profiles"
	profilesStockSubdir  = "default"
	profilesStockRelPath = "../../config/go.d/azure_monitor.profiles/default"
)

type profileCatalog struct {
	byName          map[string]ProfileConfig
	stockProfileSet map[string]struct{}
}

type profileDirSpec struct {
	Path    string
	IsStock bool
}

func loadProfileCatalog() (profileCatalog, error) {
	specs := defaultProfileDirSpecs()
	if len(specs) == 0 {
		return profileCatalog{}, errors.New("no profile search directories configured")
	}

	catalog, err := loadProfileCatalogFromDirs(specs)
	if err != nil {
		return profileCatalog{}, err
	}
	if len(catalog.stockProfileSet) == 0 {
		return profileCatalog{}, fmt.Errorf("no stock profiles found under %q", filepath.Join(pluginconfig.CollectorsStockDir(), profilesDirName, profilesStockSubdir))
	}
	return catalog, nil
}

func loadProfileCatalogFromDirs(specs []profileDirSpec) (profileCatalog, error) {
	catalog := profileCatalog{
		byName:          make(map[string]ProfileConfig),
		stockProfileSet: make(map[string]struct{}),
	}

	for _, spec := range specs {
		if stringsTrim(spec.Path) == "" {
			continue
		}
		if !isDirExists(spec.Path) {
			if spec.IsStock {
				return profileCatalog{}, fmt.Errorf("stock profiles directory does not exist: %s", spec.Path)
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

			key := profileKeyFromFilename(name)
			if key == "" {
				return fmt.Errorf("invalid profile filename %q", name)
			}
			if spec.IsStock {
				catalog.stockProfileSet[key] = struct{}{}
			}
			if _, exists := catalog.byName[key]; exists {
				// First wins: user directories are scanned before stock directories.
				return nil
			}

			cfg, err := loadProfileFile(path, key)
			if err != nil {
				return err
			}
			catalog.byName[key] = cfg
			return nil
		})
		if err != nil {
			return profileCatalog{}, err
		}
	}

	if len(catalog.byName) == 0 {
		return profileCatalog{}, errors.New("no Azure Monitor profiles were loaded")
	}
	return catalog, nil
}

func loadProfileFile(path, key string) (ProfileConfig, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return ProfileConfig{}, err
	}

	var cfg ProfileConfig
	if err := yaml.Unmarshal(data, &cfg); err != nil {
		return ProfileConfig{}, fmt.Errorf("unmarshal profile %q: %w", path, err)
	}

	if err := cfg.validate(fmt.Sprintf("profile %q", key)); err != nil {
		return ProfileConfig{}, fmt.Errorf("validate profile %q: %w", path, err)
	}

	return cfg, nil
}

func (c profileCatalog) defaultProfileNames() []string {
	if len(c.stockProfileSet) == 0 {
		return nil
	}

	names := make([]string, 0, len(c.stockProfileSet))
	for name := range c.stockProfileSet {
		names = append(names, name)
	}
	sort.Strings(names)
	return names
}

func (c profileCatalog) resolve(profileNames []string) ([]ProfileConfig, error) {
	selected := profileNames
	if len(selected) == 0 {
		selected = c.defaultProfileNames()
	}
	if len(selected) == 0 {
		return nil, errors.New("no Azure Monitor profiles selected")
	}

	profiles := make([]ProfileConfig, 0, len(selected))
	for _, name := range selected {
		key := stringsLowerTrim(name)
		prof, ok := c.byName[key]
		if !ok {
			return nil, fmt.Errorf("unknown profile %q", name)
		}
		profiles = append(profiles, prof)
	}
	return profiles, nil
}

func profileKeyFromFilename(name string) string {
	base := strings.TrimSuffix(name, filepath.Ext(name))
	return stringsLowerTrim(base)
}

func defaultProfileDirSpecs() []profileDirSpec {
	if executable.Name == "test" {
		if dir := stockProfilesDirFromThisFile(); dir != "" {
			return []profileDirSpec{{Path: dir, IsStock: true}}
		}
		return nil
	}

	if dir := filepath.Join(executable.Directory, "../config/go.d", profilesDirName, profilesStockSubdir); isDirExists(dir) {
		return []profileDirSpec{{Path: dir, IsStock: true}}
	}

	specs := make([]profileDirSpec, 0, len(pluginconfig.CollectorsUserDirs())+1)
	for _, dir := range pluginconfig.CollectorsUserDirs() {
		specs = append(specs, profileDirSpec{
			Path:    filepath.Join(dir, profilesDirName),
			IsStock: false,
		})
	}
	specs = append(specs, profileDirSpec{
		Path:    filepath.Join(pluginconfig.CollectorsStockDir(), profilesDirName, profilesStockSubdir),
		IsStock: true,
	})

	return specs
}

func stockProfilesDirFromThisFile() string {
	_, thisFile, _, ok := runtime.Caller(0)
	if !ok {
		return ""
	}

	base := filepath.Dir(thisFile)
	candidate := filepath.Join(base, profilesStockRelPath)
	if !isDirExists(candidate) {
		return ""
	}
	abs, _ := filepath.Abs(candidate)
	return abs
}

func isDirExists(dir string) bool {
	fi, err := os.Stat(dir)
	if err != nil {
		return !errors.Is(err, fs.ErrNotExist)
	}
	return fi.Mode().IsDir()
}
