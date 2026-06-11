// SPDX-License-Identifier: GPL-3.0-or-later

package promprofiles

import (
	"bytes"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"regexp"
	"runtime"
	"strings"

	"gopkg.in/yaml.v3"

	"github.com/netdata/netdata/go/plugins/logger"
	"github.com/netdata/netdata/go/plugins/pkg/executable"
	"github.com/netdata/netdata/go/plugins/pkg/pluginconfig"
)

const profilesDirName = "prometheus.profiles"

var log = logger.New().With("component", "prometheus/promprofiles")

// validProfileName constrains a profile's identity, which is its file basename.
var validProfileName = regexp.MustCompile(`^[a-z][a-z0-9_]*$`)

// IsValidProfileName reports whether name satisfies the profile-identity
// constraint (the same pattern applied to profile file basenames). Callers that
// reference a profile by name — config entries, for example — use it to reject
// names no catalog profile can ever have.
func IsValidProfileName(name string) bool {
	return validProfileName.MatchString(name)
}

// Catalog holds the resolved set of profiles, keyed by normalized name and kept
// in discovery order so OrderedProfiles is deterministic.
type Catalog struct {
	byName      map[string]Profile
	orderedKeys []string
}

type catalogEntry struct {
	config  Profile
	path    string
	isStock bool
}

// DirSpec is one profile search directory. Stock directories are authoritative:
// an invalid stock profile is fatal, while an invalid user profile is skipped.
type DirSpec struct {
	Path    string
	IsStock bool
}

func loadFromDefaultDirs() (Catalog, error) {
	return LoadFromDirs(defaultDirSpecs())
}

// LoadFromDirs builds a Catalog from the given directories. A profile's identity
// is its file basename. User profiles override stock profiles of the same name.
// Invalid or duplicate user profiles are skipped with a warning; invalid or
// duplicate stock profiles are fatal.
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

			baseName := strings.TrimSuffix(name, filepath.Ext(name))
			if !validProfileName.MatchString(baseName) {
				return handleProfileLoadError(spec, path, fmt.Errorf("profile file %q: basename %q must match %s", path, baseName, validProfileName.String()))
			}

			cfg, err := loadProfileFile(path, baseName)
			if err != nil {
				return handleProfileLoadError(spec, path, err)
			}

			key := normalizeKey(baseName)

			prev, exists := seen[key]
			if !exists {
				seen[key] = catalogEntry{config: cfg, path: path, isStock: spec.IsStock}
				catalog.orderedKeys = append(catalog.orderedKeys, key)
				return nil
			}

			switch {
			case prev.isStock == spec.IsStock:
				if !spec.IsStock {
					log.Warningf("ignoring duplicate user profile %q in %q; already loaded from %q", cfg.Name, path, prev.path)
					return nil
				}
				return fmt.Errorf("duplicate stock profile name %q in %q and %q", cfg.Name, prev.path, path)
			case prev.isStock && !spec.IsStock:
				// User profile overrides the stock one.
				seen[key] = catalogEntry{config: cfg, path: path, isStock: false}
			case !prev.isStock && spec.IsStock:
				// User profile already loaded; stock does not override it.
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

// Empty reports whether the catalog holds no profiles.
func (c Catalog) Empty() bool { return len(c.byName) == 0 }

// OrderedProfiles returns all profiles in deterministic discovery order.
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

// Resolve returns the profiles for the given names, in the requested order.
// Names are matched case-insensitively. An unknown name is an error.
func (c Catalog) Resolve(profileNames []string) ([]Profile, error) {
	if len(profileNames) == 0 {
		return nil, fmt.Errorf("no Prometheus profiles selected")
	}

	profiles := make([]Profile, 0, len(profileNames))
	for _, name := range profileNames {
		prof, ok := c.byName[normalizeKey(name)]
		if !ok {
			return nil, fmt.Errorf("unknown profile %q", name)
		}
		profiles = append(profiles, prof)
	}
	return profiles, nil
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
	cfg.Name = baseName

	if err := cfg.validate(fmt.Sprintf("profile %q", path)); err != nil {
		return Profile{}, err
	}

	return cfg, nil
}

// handleProfileLoadError makes a stock-profile error fatal while downgrading a
// user-profile error to a skip-with-warning, so one bad user file cannot stop
// the agent from loading the rest of the catalog.
func handleProfileLoadError(spec DirSpec, path string, err error) error {
	if spec.IsStock {
		return err
	}

	log.Warningf("ignoring invalid user profile %q: %v", path, err)
	return nil
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
	if dir := filepath.Join(pluginconfig.CollectorsStockDir(), profilesDirName, "default"); isDirExists(dir) {
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
	// This file sits at plugin/go.d/collector/prometheus/promprofiles; three levels
	// up is plugin/go.d, under which config/go.d/<profilesDirName>/default lives.
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

// NormalizeProfileKey is the canonical-name transform shared with callers that
// need to compare profile names against the catalog (case-insensitive).
func NormalizeProfileKey(v string) string { return strings.ToLower(strings.TrimSpace(v)) }

func normalizeKey(v string) string { return NormalizeProfileKey(v) }
