// SPDX-License-Identifier: GPL-3.0-or-later

package azureprofiles

import (
	"bytes"
	"errors"
	"fmt"
	"path/filepath"
	"runtime"
	"sort"
	"strings"

	"gopkg.in/yaml.v3"

	"github.com/netdata/netdata/go/plugins/logger"
	"github.com/netdata/netdata/go/plugins/pkg/executable"
	"github.com/netdata/netdata/go/plugins/pkg/pluginconfig"
	"github.com/netdata/netdata/go/plugins/plugin/go.d/pkg/profilecatalog"
)

const (
	profilesDirName = "azure_monitor.profiles"
)

var log = logger.New().With("component", "azure_monitor/azureprofiles")

// DirSpec is one profile search directory (re-exported so this package's API is
// self-contained for callers of LoadFromDirs).
type DirSpec = profilecatalog.DirSpec

// Catalog wraps the shared profile catalog with Azure-specific queries
// (resolution by basename and by resource type).
type Catalog struct {
	profilecatalog.Catalog[Profile]
}

type ResolvedProfile struct {
	Name   string
	Config Profile
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
	if len(catalog.StockNames()) == 0 {
		return Catalog{}, fmt.Errorf("no stock profiles found under %q", filepath.Join(pluginconfig.CollectorsStockDir(), profilesDirName, "default"))
	}
	return catalog, nil
}

func LoadFromDirs(specs []DirSpec) (Catalog, error) {
	core, err := profilecatalog.Load(specs, profilecatalog.Options[Profile]{
		Decode: func(ctx profilecatalog.FileContext, data []byte) (Profile, error) {
			return decodeProfile(ctx, data)
		},
		Log: log,
	})
	if err != nil {
		return Catalog{}, err
	}

	catalog := Catalog{Catalog: core}
	if catalog.Empty() {
		return Catalog{}, errors.New("no Azure Monitor profiles were loaded")
	}
	return catalog, nil
}

// decodeProfile decodes, normalizes, and validates one profile. The decoder is
// non-strict (unknown keys are ignored), so an older collector tolerates
// profiles that carry newer optional fields.
func decodeProfile(ctx profilecatalog.FileContext, data []byte) (Profile, error) {
	baseName := ctx.BaseName
	path := strings.TrimSpace(ctx.Path)
	if path == "" {
		path = baseName
	}

	var cfg Profile
	dec := yaml.NewDecoder(bytes.NewReader(data))
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
		prof, ok := c.Get(profileName)
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
	for _, n := range c.Sorted() {
		rt := normalizeKey(n.Profile.ResourceType)
		if rt == "" {
			continue
		}
		if _, ok := types[rt]; ok {
			names = append(names, n.Name)
		}
	}
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

func (c Catalog) ResourceTypes() []string {
	if c.Empty() {
		return nil
	}

	seen := make(map[string]struct{}, c.Len())
	types := make([]string, 0, c.Len())
	for _, n := range c.Sorted() {
		rt := strings.TrimSpace(n.Profile.ResourceType)
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
	if dir := filepath.Join(executable.Directory, "../config/go.d", profilesDirName, "default"); profilecatalog.DirExists(dir) {
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
	candidate := filepath.Join(base, "..", "..", "..", "config", "go.d", profilesDirName, "default")
	if !profilecatalog.DirExists(candidate) {
		return ""
	}
	abs, _ := filepath.Abs(candidate)
	return abs
}

func normalizeKey(v string) string {
	return strings.ToLower(strings.TrimSpace(v))
}
