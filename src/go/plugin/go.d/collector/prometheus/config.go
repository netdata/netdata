// SPDX-License-Identifier: GPL-3.0-or-later

package prometheus

import (
	"errors"
	"fmt"
	"strings"

	"github.com/netdata/netdata/go/plugins/pkg/prometheus/selector"
	"github.com/netdata/netdata/go/plugins/pkg/web"
	"github.com/netdata/netdata/go/plugins/plugin/go.d/collector/prometheus/promprofiles"
	"github.com/netdata/netdata/go/plugins/plugin/go.d/collector/prometheus/relabel"
)

type Config struct {
	Vnode              string `yaml:"vnode,omitempty" json:"vnode"`
	UpdateEvery        int    `yaml:"update_every,omitempty" json:"update_every"`
	AutoDetectionRetry int    `yaml:"autodetection_retry,omitempty" json:"autodetection_retry"`
	web.HTTPConfig     `yaml:",inline" json:""`
	Name               string         `yaml:"name,omitempty" json:"name"`
	Application        string         `yaml:"app,omitempty" json:"app"`
	Selector           selector.Expr  `yaml:"selector,omitempty" json:"selector"`
	Relabeling         []RelabelBlock `yaml:"relabeling,omitempty" json:"relabeling,omitempty"`
	Profiles           ProfilesConfig `yaml:"profiles" json:"profiles"`
	ExpectedPrefix     string         `yaml:"expected_prefix,omitempty" json:"expected_prefix"`
	MaxTS              int            `yaml:"max_time_series" json:"max_time_series"`
	MaxTSPerMetric     int            `yaml:"max_time_series_per_metric" json:"max_time_series_per_metric"`
	FallbackType       struct {
		Gauge   []string `yaml:"gauge,omitempty" json:"gauge"`
		Counter []string `yaml:"counter,omitempty" json:"counter"`
	} `yaml:"fallback_type,omitempty" json:"fallback_type"`
}

// RelabelBlock is one job-level metric-relabeling block: a required metric-name
// match that scopes a list of Prometheus relabel rules to a metric-name subset.
// Use match "*" to target every metric. Rules run pre-assembly on the sample
// stream, in block order.
type RelabelBlock struct {
	Match                string           `yaml:"match" json:"match"`
	MetricRelabelConfigs []relabel.Config `yaml:"metric_relabel_configs" json:"metric_relabel_configs"`
}

const (
	profilesModeNone     = "none"
	profilesModeAuto     = "auto"
	profilesModeExact    = "exact"
	profilesModeCombined = "combined"
)

// ProfilesConfig selects which curated profiles apply to the job:
//   - "auto" (default): every profile whose match hits at least one scraped metric family;
//   - "exact": only the named profiles, each of which must match or Check fails;
//   - "combined": "auto" plus the named profiles, deduplicated;
//   - "none": no profiles (pure autogen, the pre-profile behavior).
//
// Only "exact" and "combined" carry entries; "auto" and "none" take none.
type ProfilesConfig struct {
	Mode         string              `yaml:"mode,omitempty" json:"mode"`
	ModeExact    *ProfilesModeConfig `yaml:"mode_exact,omitempty" json:"mode_exact,omitempty"`
	ModeCombined *ProfilesModeConfig `yaml:"mode_combined,omitempty" json:"mode_combined,omitempty"`
}

type ProfilesModeConfig struct {
	Entries []ProfileEntryConfig `yaml:"entries,omitempty" json:"entries,omitempty"`
}

// ProfileEntryConfig names a profile by its catalog identity (the profile file's
// basename), matched case-insensitively.
type ProfileEntryConfig struct {
	Name string `yaml:"name" json:"name"`
}

// effectiveMode normalizes an empty mode to the default ("auto").
func (p ProfilesConfig) effectiveMode() string {
	if m := strings.ToLower(strings.TrimSpace(p.Mode)); m != "" {
		return m
	}
	return profilesModeAuto
}

func (p ProfilesConfig) validate() error {
	switch p.effectiveMode() {
	case profilesModeNone, profilesModeAuto:
		return nil
	case profilesModeExact:
		return validateProfileEntries("profiles.mode_exact.entries", modeEntries(p.ModeExact))
	case profilesModeCombined:
		return validateProfileEntries("profiles.mode_combined.entries", modeEntries(p.ModeCombined))
	default:
		return fmt.Errorf("'profiles.mode' must be one of: %s, %s, %s, %s",
			profilesModeNone, profilesModeAuto, profilesModeExact, profilesModeCombined)
	}
}

func modeEntries(m *ProfilesModeConfig) []ProfileEntryConfig {
	if m == nil {
		return nil
	}
	return m.Entries
}

// validateProfileEntries enforces a non-empty entry list with non-empty,
// case-insensitively-unique names. Names are matched against the catalog the
// same (case-insensitive) way, so duplicates that differ only in case would
// select the same profile twice and collide during the template merge.
func validateProfileEntries(path string, entries []ProfileEntryConfig) error {
	if len(entries) == 0 {
		return fmt.Errorf("'%s' must not be empty for the selected profiles mode", path)
	}

	var errs []error
	seen := make(map[string]struct{}, len(entries))
	for i, e := range entries {
		name := strings.TrimSpace(e.Name)
		if name == "" {
			errs = append(errs, fmt.Errorf("'%s[%d].name' must not be empty", path, i))
			continue
		}
		if !promprofiles.IsValidProfileName(name) {
			errs = append(errs, fmt.Errorf("'%s[%d].name' (%q) must be lowercase letters, digits, or underscores and start with a letter", path, i, name))
			continue
		}
		key := promprofiles.NormalizeProfileKey(name)
		if _, ok := seen[key]; ok {
			errs = append(errs, fmt.Errorf("'%s' contains duplicate profile name %q", path, name))
			continue
		}
		seen[key] = struct{}{}
	}
	return errors.Join(errs...)
}
