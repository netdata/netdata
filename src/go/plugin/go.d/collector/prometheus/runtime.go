// SPDX-License-Identifier: GPL-3.0-or-later

package prometheus

import (
	"fmt"
	"sort"
	"strings"

	"github.com/netdata/netdata/go/plugins/pkg/matcher"
	"github.com/netdata/netdata/go/plugins/pkg/prometheus"
	"github.com/netdata/netdata/go/plugins/plugin/go.d/collector/prometheus/promprofiles"
)

// promRuntime is the per-job state built once at Check and reused afterwards:
// the selected profiles and the chart template served via ChartTemplateYAML
// (autogen, or autogen merged with the selected profiles' curated groups).
type promRuntime struct {
	profiles      []promprofiles.Profile
	chartTemplate string
}

// ensureChartTemplate selects the profiles that apply to this job against the
// scraped families and builds the per-job chart template, caching both on the
// runtime on the first Check. Later cycles reuse it; Cleanup (job restart)
// clears it so the next Check rebuilds. Building at Check (not Init) is what
// lets selection depend on what the endpoint actually exposes.
func (c *Collector) ensureChartTemplate(mfs prometheus.MetricFamilies) error {
	if c.runtime != nil {
		return nil
	}

	profiles, err := c.selectProfiles(mfs)
	if err != nil {
		return err
	}

	tmpl, err := buildMergedChartTemplate(c.resolveApp(profiles), profiles)
	if err != nil {
		return err
	}

	c.runtime = &promRuntime{profiles: profiles, chartTemplate: tmpl}
	return nil
}

// resolveApp is the chart-context "app" segment — the per-job identity the UI
// turns into an Applications section. Precedence: the configured app, else the
// first selected profile that declares one, else the job name. If selected
// profiles declare different apps, the first (in selection order) wins and the
// rest are logged (set the job's app to disambiguate).
func (c *Collector) resolveApp(profiles []promprofiles.Profile) string {
	if c.Application != "" {
		return c.Application
	}
	app := ""
	for _, p := range profiles {
		switch {
		case p.App == "":
			continue
		case app == "":
			app = p.App
		case p.App != app:
			c.Warningf("profiles: selected profiles declare different apps; using %q, ignoring %q (set the job's 'app' to disambiguate)", app, p.App)
		}
	}
	if app != "" {
		return app
	}
	return c.Name
}

// selectProfiles resolves the profiles for this job per profiles.mode, matching
// each profile's pattern against the scraped family base names (the metric's
// declared "# TYPE" name; the 2B selection surface). mode "none" selects nothing
// and skips the catalog entirely so a broken stock profile cannot break a job
// that opted out of profiles.
func (c *Collector) selectProfiles(mfs prometheus.MetricFamilies) ([]promprofiles.Profile, error) {
	mode := c.Profiles.effectiveMode()
	if mode == profilesModeNone {
		return nil, nil
	}

	catalog, err := c.loadProfileCatalog()
	if err != nil {
		return nil, fmt.Errorf("loading profiles catalog: %w", err)
	}

	var selected []promprofiles.Profile
	switch mode {
	case profilesModeAuto:
		selected, err = autoSelectProfiles(catalog.OrderedProfiles(), mfs)
	case profilesModeExact:
		selected, err = namedSelectProfiles(catalog, entryNames(c.Profiles.ModeExact), mfs)
	case profilesModeCombined:
		selected, err = combinedSelectProfiles(catalog, entryNames(c.Profiles.ModeCombined), mfs)
	}
	if err != nil {
		return nil, err
	}

	if len(selected) == 0 {
		c.Infof("profiles: mode %q selected no profiles; using generic autogen charts", mode)
	} else {
		c.Infof("profiles: mode %q selected %d profile(s): %s", mode, len(selected), profileNamesList(selected))
	}
	return selected, nil
}

// autoSelectProfiles returns every catalog profile whose match hits at least one
// scraped family.
func autoSelectProfiles(profiles []promprofiles.Profile, mfs prometheus.MetricFamilies) ([]promprofiles.Profile, error) {
	var out []promprofiles.Profile
	for _, p := range profiles {
		ok, err := profileMatchesFamilies(p, mfs)
		if err != nil {
			return nil, err
		}
		if ok {
			out = append(out, p)
		}
	}
	return out, nil
}

// namedSelectProfiles resolves the named profiles (each must exist) and requires
// each to match at least one scraped family, else the job fails its check.
func namedSelectProfiles(catalog promprofiles.Catalog, names []string, mfs prometheus.MetricFamilies) ([]promprofiles.Profile, error) {
	profiles, err := catalog.Resolve(names)
	if err != nil {
		return nil, err
	}
	for _, p := range profiles {
		ok, err := profileMatchesFamilies(p, mfs)
		if err != nil {
			return nil, err
		}
		if !ok {
			return nil, fmt.Errorf("profile %q matches no scraped metric (pattern %q)", p.Name, p.Match)
		}
	}
	return profiles, nil
}

// combinedSelectProfiles selects the auto-matched profiles plus the named ones
// (each named must match), deduplicated by name so a profile picked by both
// parts is merged once.
func combinedSelectProfiles(catalog promprofiles.Catalog, names []string, mfs prometheus.MetricFamilies) ([]promprofiles.Profile, error) {
	auto, err := autoSelectProfiles(catalog.OrderedProfiles(), mfs)
	if err != nil {
		return nil, err
	}
	named, err := namedSelectProfiles(catalog, names, mfs)
	if err != nil {
		return nil, err
	}

	seen := make(map[string]struct{}, len(auto)+len(named))
	out := make([]promprofiles.Profile, 0, len(auto)+len(named))
	for _, p := range auto {
		seen[promprofiles.NormalizeProfileKey(p.Name)] = struct{}{}
		out = append(out, p)
	}
	for _, p := range named {
		if _, ok := seen[promprofiles.NormalizeProfileKey(p.Name)]; ok {
			continue
		}
		out = append(out, p)
	}
	return out, nil
}

// profileMatchesFamilies reports whether the profile's match pattern hits at
// least one scraped family base name.
func profileMatchesFamilies(p promprofiles.Profile, mfs prometheus.MetricFamilies) (bool, error) {
	m, err := matcher.NewSimplePatternsMatcher(p.Match)
	if err != nil {
		return false, fmt.Errorf("profile %q: invalid match %q: %w", p.Name, p.Match, err)
	}
	for name := range mfs {
		if m.MatchString(name) {
			return true, nil
		}
	}
	return false, nil
}

// entryNames extracts the non-empty profile names from a mode block.
func entryNames(m *ProfilesModeConfig) []string {
	if m == nil {
		return nil
	}
	names := make([]string, 0, len(m.Entries))
	for _, e := range m.Entries {
		if n := strings.TrimSpace(e.Name); n != "" {
			names = append(names, n)
		}
	}
	return names
}

func profileNamesList(profiles []promprofiles.Profile) string {
	names := make([]string, 0, len(profiles))
	for _, p := range profiles {
		names = append(names, p.Name)
	}
	sort.Strings(names)
	return strings.Join(names, ", ")
}
