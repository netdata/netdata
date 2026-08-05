// SPDX-License-Identifier: GPL-3.0-or-later

package promprofiles

import (
	"bytes"
	"fmt"
	"path/filepath"
	"regexp"
	"runtime"
	"strings"

	"gopkg.in/yaml.v3"

	"github.com/netdata/netdata/go/plugins/logger"
	"github.com/netdata/netdata/go/plugins/pkg/executable"
	"github.com/netdata/netdata/go/plugins/pkg/pluginconfig"
	"github.com/netdata/netdata/go/plugins/plugin/go.d/pkg/profilecatalog"
)

const profilesDirName = "prometheus.profiles"

var log = logger.New().With("component", "prometheus/promprofiles")

// validProfileName constrains a profile's identity (its file basename) and the
// optional app field.
var validProfileName = regexp.MustCompile(`^[a-z][a-z0-9_]*$`)

// IsValidProfileName reports whether name satisfies the profile-identity
// constraint. Callers that reference a profile by name (config entries, for
// example) use it to reject names no catalog profile can ever have.
func IsValidProfileName(name string) bool {
	return validProfileName.MatchString(name)
}

// NormalizeProfileKey is the canonical (case-insensitive) name transform shared
// with callers that compare profile names against the catalog.
func NormalizeProfileKey(v string) string { return strings.ToLower(strings.TrimSpace(v)) }

// DirSpec is one profile search directory (re-exported so this package's API is
// self-contained for callers of LoadFromDirs).
type DirSpec = profilecatalog.DirSpec

// Catalog wraps the shared profile catalog with Prometheus-specific queries:
// case-insensitive resolution and discovery-ordered listing.
type Catalog struct {
	core profilecatalog.Catalog[Profile]
}

// LoadFromDefaultDirs builds the catalog from the stock and user directories.
func LoadFromDefaultDirs() (Catalog, error) {
	return LoadFromDirs(defaultDirSpecs())
}

// LoadFromDirs builds a Catalog from the given directories via the shared loader.
// A profile's identity is its file basename (case-insensitive). User profiles
// override stock profiles of the same name; invalid or duplicate user profiles
// are skipped with a warning, invalid or duplicate stock profiles are fatal.
func LoadFromDirs(specs []DirSpec) (Catalog, error) {
	core, err := profilecatalog.Load(specs, profilecatalog.Options[Profile]{
		Decode: func(ctx profilecatalog.FileContext, data []byte) (Profile, error) {
			return decodeProfile(data, ctx.BaseName, ctx.IsStock)
		},
		NormalizeKey: NormalizeProfileKey,
		ValidName:    IsValidProfileName,
		Log:          log,
	})
	if err != nil {
		return Catalog{}, err
	}
	return Catalog{core: core}, nil
}

// decodeProfile strict-decodes the profile header (match, app, and the template
// as an un-decoded node), retains the raw bytes for lazy hydration, and
// validates the header. A user profile additionally validates its template at
// load, so a broken user profile is skipped (the stock profile of the same name
// survives); stock template validation is deferred to first use.
func decodeProfile(data []byte, baseName string, isStock bool) (Profile, error) {
	var hdr profileHeader
	dec := yaml.NewDecoder(bytes.NewReader(data))
	dec.KnownFields(true)
	if err := dec.Decode(&hdr); err != nil {
		return Profile{}, fmt.Errorf("unmarshal profile %q: %w", baseName, err)
	}

	p := Profile{
		Name:            baseName,
		Match:           hdr.Match,
		App:             hdr.App,
		autogenSelector: nil,
		lazy:            &lazyTemplate{raw: data},
	}
	if hdr.Autogen != nil {
		if hdr.Autogen.Selector == nil {
			return Profile{}, fmt.Errorf("profile %q: 'autogen.selector' is required when 'autogen' is set", baseName)
		}
		p.autogenSelector = cloneSelectorExpr(hdr.Autogen.Selector)
	}
	if err := p.validateHeader(); err != nil {
		return Profile{}, err
	}
	if !isStock {
		if _, err := p.Template(); err != nil {
			return Profile{}, err
		}
	}
	return p, nil
}

// Get returns an ownership-safe profile copy.
func (c Catalog) Get(name string) (Profile, bool) {
	profile, ok := c.core.Get(name)
	if !ok {
		return Profile{}, false
	}
	return profile.clone(), true
}

// OrderedProfiles returns all profiles in deterministic discovery order.
func (c Catalog) OrderedProfiles() []Profile {
	named := c.core.InOrder()
	out := make([]Profile, 0, len(named))
	for _, n := range named {
		out = append(out, n.Profile.clone())
	}
	return out
}

// Resolve returns the profiles for the given names, in the requested order.
// Names are matched case-insensitively. An unknown name is an error.
func (c Catalog) Resolve(profileNames []string) ([]Profile, error) {
	if len(profileNames) == 0 {
		return nil, fmt.Errorf("no Prometheus profiles selected")
	}

	profiles := make([]Profile, 0, len(profileNames))
	for _, name := range profileNames {
		prof, ok := c.Get(name)
		if !ok {
			return nil, fmt.Errorf("unknown profile %q", name)
		}
		profiles = append(profiles, prof)
	}
	return profiles, nil
}

func defaultDirSpecs() []DirSpec {
	if executable.Name == "test" {
		if dir := profilesDirFromThisFile(); dir != "" {
			return []DirSpec{{Path: dir, IsStock: true}}
		}
		return nil
	}

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

func profilesDirFromThisFile() string {
	_, thisFile, _, ok := runtime.Caller(0)
	if !ok {
		return ""
	}

	base := filepath.Dir(thisFile)
	// This file sits at plugin/go.d/collector/prometheus/promprofiles; three levels
	// up is plugin/go.d, under which config/go.d/<profilesDirName>/default lives.
	candidate := filepath.Join(base, "..", "..", "..", "config", "go.d", profilesDirName, "default")
	if !profilecatalog.DirExists(candidate) {
		return ""
	}

	abs, _ := filepath.Abs(candidate)
	return abs
}
