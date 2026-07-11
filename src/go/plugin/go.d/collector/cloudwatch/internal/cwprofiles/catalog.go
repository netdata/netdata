// SPDX-License-Identifier: GPL-3.0-or-later

package cwprofiles

import (
	"bytes"
	"errors"
	"fmt"
	"path/filepath"
	"runtime"

	"gopkg.in/yaml.v3"

	"github.com/netdata/netdata/go/plugins/logger"
	"github.com/netdata/netdata/go/plugins/pkg/executable"
	"github.com/netdata/netdata/go/plugins/pkg/pluginconfig"
	"github.com/netdata/netdata/go/plugins/plugin/go.d/pkg/profilecatalog"
)

const profilesDirName = "cloudwatch.profiles"

var log = logger.New().With("component", "cloudwatch/cwprofiles")

// Catalog wraps the shared profile catalog with CloudWatch-specific queries.
// Stock/user tracking and lookups come from the embedded shared catalog.
type Catalog struct {
	profilecatalog.Catalog[Profile]
}

// ResolvedProfile pairs a profile with its basename (the series-name prefix).
type ResolvedProfile struct {
	Name   string
	Config Profile
}

// LoadFromDefaultDirs loads stock profiles plus any user overrides from the
// standard go.d config locations.
func LoadFromDefaultDirs() (Catalog, error) {
	specs := defaultDirSpecs()
	if len(specs) == 0 {
		return Catalog{}, errors.New("no profile search directories configured")
	}

	cat, err := loadFromDirs(specs)
	if err != nil {
		return Catalog{}, err
	}
	if len(cat.StockNames()) == 0 {
		return Catalog{}, fmt.Errorf("no stock profiles found under %q", filepath.Join(pluginconfig.CollectorsStockDir(), profilesDirName, "default"))
	}
	return cat, nil
}

// loadFromDirs loads profiles from the given directories via the shared loader,
// then enforces the CloudWatch-specific cross-profile chart-id uniqueness rule.
func loadFromDirs(specs []profilecatalog.DirSpec) (Catalog, error) {
	core, err := profilecatalog.Load(specs, profilecatalog.Options[Profile]{
		Decode: func(ctx profilecatalog.FileContext, data []byte) (Profile, error) {
			return decodeProfileBytes(data, ctx.BaseName)
		},
		Log: log,
	})
	if err != nil {
		return Catalog{}, err
	}

	cat := Catalog{Catalog: core}
	if cat.Empty() {
		return Catalog{}, errors.New("no CloudWatch profiles were loaded")
	}
	if err := validateUniqueChartIDs(cat.Sorted(), cat.EffectiveIsStock); err != nil {
		return Catalog{}, err
	}
	return cat, nil
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

// AllProfiles returns every loaded profile (stock + user), sorted by basename.
// The collection-rule compiler selects from this catalog.
func (c Catalog) AllProfiles() []ResolvedProfile {
	named := c.Sorted()
	out := make([]ResolvedProfile, 0, len(named))
	for _, n := range named {
		out = append(out, ResolvedProfile{Name: n.Name, Config: n.Profile})
	}
	return out
}

// validateUniqueChartIDs ensures no two loaded profiles render a chart with the
// same id. chartengine keys charts by id and, on a cross-template id collision,
// silently keeps the first and drops the rest, so a colliding chart would simply
// vanish when overlapping profiles are selected together. A
// collision between two stock profiles is a packaging bug and is fatal; a
// collision involving a user profile is logged (its colliding chart is dropped).
// isStock reports the effective origin of a profile by basename (a user override
// of a stock profile is NOT stock).
func validateUniqueChartIDs(profiles []profilecatalog.Named[Profile], isStock func(string) bool) error {
	type owner struct {
		base  string
		stock bool
	}
	seen := make(map[string]owner)
	var errs []error
	for _, p := range profiles {
		stock := isStock(p.Name)
		for _, id := range chartIDs(p.Profile.Template) {
			prev, ok := seen[id]
			if !ok {
				seen[id] = owner{base: p.Name, stock: stock}
				continue
			}
			if prev.stock && stock {
				errs = append(errs, fmt.Errorf("duplicate chart id %q in stock profiles %q and %q", id, prev.base, p.Name))
			} else {
				log.Warningf("chart id %q in profile %q collides with profile %q; chartengine will render only one", id, p.Name, prev.base)
			}
		}
	}
	return errors.Join(errs...)
}

func defaultDirSpecs() []profilecatalog.DirSpec {
	if executable.Name == "test" {
		if dir := cwProfilesDirFromThisFile(); dir != "" {
			return []profilecatalog.DirSpec{{Path: dir, IsStock: true}}
		}
		return nil
	}

	// Keep the adjacent stock-dir fast path for local development runs where the
	// built binary lives under src/go/plugin/go.d/bin and should use the source-tree
	// stock profiles without depending on installed pluginconfig paths.
	if dir := filepath.Join(executable.Directory, "../config/go.d", profilesDirName, "default"); profilecatalog.DirExists(dir) {
		return []profilecatalog.DirSpec{{Path: dir, IsStock: true}}
	}

	specs := make([]profilecatalog.DirSpec, 0, len(pluginconfig.CollectorsUserDirs())+1)
	for _, dir := range pluginconfig.CollectorsUserDirs() {
		specs = append(specs, profilecatalog.DirSpec{
			Path:    filepath.Join(dir, profilesDirName),
			IsStock: false,
		})
	}
	specs = append(specs, profilecatalog.DirSpec{
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
	// base is .../collector/cloudwatch/internal/cwprofiles → climb 4 levels to plugin/go.d,
	// then into config/go.d/<profiles>/default. Update the count if this file moves.
	candidate := filepath.Join(base, "..", "..", "..", "..", "config", "go.d", profilesDirName, "default")
	if !profilecatalog.DirExists(candidate) {
		return ""
	}
	abs, _ := filepath.Abs(candidate)
	return abs
}
