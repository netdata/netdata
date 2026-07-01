// SPDX-License-Identifier: GPL-3.0-or-later

package cwprofiles

import (
	"bytes"
	"errors"
	"fmt"
	"io/fs"
	"maps"
	"os"
	"path/filepath"
	"runtime"
	"slices"
	"strings"

	"gopkg.in/yaml.v3"

	"github.com/netdata/netdata/go/plugins/logger"
	"github.com/netdata/netdata/go/plugins/pkg/executable"
	"github.com/netdata/netdata/go/plugins/pkg/pluginconfig"
)

const profilesDirName = "cloudwatch.profiles"

var log = logger.New().With("component", "cloudwatch/cwprofiles")

// Catalog holds the loaded profiles keyed by basename. Stock profiles are
// tracked separately so auto-mode can default to the stock set.
type Catalog struct {
	byBaseName            map[string]Profile
	stockProfileBaseNames map[string]struct{}
}

// ResolvedProfile pairs a profile with its basename (the series-name prefix).
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

// DirSpec is one profile search directory.
type DirSpec struct {
	Path    string
	IsStock bool
}

// LoadFromDefaultDirs loads stock profiles plus any user overrides from the
// standard go.d config locations.
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

// LoadFromDirs loads profiles from the given directories. A user profile
// overrides a stock profile with the same basename. Invalid stock profiles are
// fatal; invalid user profiles are logged and skipped.
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
				return handleProfileLoadError(spec, path, err)
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
				return handleProfileLoadError(spec, path, fmt.Errorf("profile %q: basename must match %q", path, reIdentityID.String()))
			}

			cfg, err := loadProfileFile(path, baseName)
			if err != nil {
				return handleProfileLoadError(spec, path, err)
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
				if !spec.IsStock {
					log.Warningf("ignoring duplicate user profile basename %q in %q; already loaded from %q", baseName, path, prev.Path)
					return nil
				}
				return fmt.Errorf("duplicate stock profile basename %q in %q and %q", baseName, prev.Path, path)
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
		return Catalog{}, errors.New("no CloudWatch profiles were loaded")
	}

	for baseName, entry := range seen {
		catalog.byBaseName[baseName] = entry.Config
	}

	if err := catalog.validateUniqueChartIDs(); err != nil {
		return Catalog{}, err
	}

	return catalog, nil
}

func loadProfileFile(path, baseName string) (Profile, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return Profile{}, err
	}
	return decodeProfileBytes(data, baseName)
}

// decodeProfileBytes decodes, normalizes, and validates one profile. It is the
// single decode path shared by file loading and tests. The decoder is non-strict
// (unknown keys are ignored), so an older collector tolerates profiles that carry
// newer optional fields.
func decodeProfileBytes(data []byte, baseName string) (Profile, error) {
	var cfg Profile
	dec := yaml.NewDecoder(bytes.NewReader(data))
	if err := dec.Decode(&cfg); err != nil {
		return Profile{}, fmt.Errorf("unmarshal profile %q: %w", baseName, err)
	}

	if err := cfg.Normalize(baseName); err != nil {
		return Profile{}, fmt.Errorf("normalize profile %q: %w", baseName, err)
	}
	if err := cfg.Validate(fmt.Sprintf("profile %q", baseName), baseName); err != nil {
		return Profile{}, fmt.Errorf("validate profile %q: %w", baseName, err)
	}

	return cfg, nil
}

func handleProfileLoadError(spec DirSpec, path string, err error) error {
	if spec.IsStock {
		return err
	}
	log.Warningf("ignoring invalid user profile %q: %v", path, err)
	return nil
}

// AllProfiles returns every loaded profile (stock + user), sorted by basename.
// This is the candidate set for namespaces.mode=auto.
func (c Catalog) AllProfiles() []ResolvedProfile {
	out := make([]ResolvedProfile, 0, len(c.byBaseName))
	for _, name := range c.sortedBaseNames() {
		out = append(out, ResolvedProfile{Name: name, Config: c.byBaseName[name]})
	}
	return out
}

// ProfilesForNamespaces returns the loaded profiles whose namespace matches any
// of the given namespaces (case-sensitive, e.g. "AWS/EC2"), sorted by basename.
// This is the candidate set for namespaces.mode=exact.
func (c Catalog) ProfilesForNamespaces(namespaces []string) []ResolvedProfile {
	want := make(map[string]struct{}, len(namespaces))
	for _, ns := range namespaces {
		if ns = strings.TrimSpace(ns); ns != "" {
			want[ns] = struct{}{}
		}
	}
	if len(want) == 0 {
		return nil
	}

	var out []ResolvedProfile
	for _, name := range c.sortedBaseNames() {
		prof := c.byBaseName[name]
		if _, ok := want[strings.TrimSpace(prof.Namespace)]; ok {
			out = append(out, ResolvedProfile{Name: name, Config: prof})
		}
	}
	return out
}

func (c Catalog) sortedBaseNames() []string {
	return slices.Sorted(maps.Keys(c.byBaseName))
}

// validateUniqueChartIDs ensures no two loaded profiles render a chart with the
// same id. chartengine keys charts by id and, on a cross-template id collision,
// silently keeps the first and drops the rest, so a colliding chart would simply
// vanish (e.g. in namespaces.mode combined, where every profile is active). A
// collision between two stock profiles is a packaging bug and is fatal; a
// collision involving a user profile is logged (its colliding chart is dropped).
func (c Catalog) validateUniqueChartIDs() error {
	type owner struct {
		base  string
		stock bool
	}
	seen := make(map[string]owner)
	var errs []error
	for _, base := range c.sortedBaseNames() {
		_, isStock := c.stockProfileBaseNames[base]
		for _, id := range chartIDs(c.byBaseName[base].Template) {
			prev, ok := seen[id]
			if !ok {
				seen[id] = owner{base: base, stock: isStock}
				continue
			}
			if prev.stock && isStock {
				errs = append(errs, fmt.Errorf("duplicate chart id %q in stock profiles %q and %q", id, prev.base, base))
			} else {
				log.Warningf("chart id %q in profile %q collides with profile %q; chartengine will render only one", id, base, prev.base)
			}
		}
	}
	return errors.Join(errs...)
}

func profileBaseName(path string) string {
	name := filepath.Base(path)
	ext := filepath.Ext(name)
	return strings.TrimSuffix(name, ext)
}

func defaultDirSpecs() []DirSpec {
	if executable.Name == "test" {
		if dir := cwProfilesDirFromThisFile(); dir != "" {
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

func cwProfilesDirFromThisFile() string {
	_, thisFile, _, ok := runtime.Caller(0)
	if !ok {
		return ""
	}

	base := filepath.Dir(thisFile)
	candidate := filepath.Join(base, "..", "..", "..", "config", "go.d", profilesDirName, "default")
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
