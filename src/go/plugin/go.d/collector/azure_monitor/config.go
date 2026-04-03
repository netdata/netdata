// SPDX-License-Identifier: GPL-3.0-or-later

package azure_monitor

import (
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/netdata/netdata/go/plugins/pkg/confopt"
	"github.com/netdata/netdata/go/plugins/plugin/go.d/pkg/cloudauth"
)

const (
	defaultUpdateEvery      = 60
	defaultAutoDetectRetry  = 0
	defaultCloud            = cloudPublic
	defaultDiscoveryEvery   = 300
	defaultDiscoveryMode    = discoveryModeFilters
	defaultProfilesMode     = profilesModeAuto
	defaultQueryOffset      = 180
	defaultTimeout          = confopt.Duration(30 * time.Second)
	defaultMaxConcurrency   = 4
	defaultMaxBatchResource = 50
	defaultMaxMetricsQuery  = 20
)

const (
	discoveryModeFilters = "filters"
	discoveryModeQuery   = "query"
)

const (
	profilesModeAuto     = "auto"
	profilesModeExact    = "exact"
	profilesModeCombined = "combined"
)

const (
	cloudPublic     = "public"
	cloudGovernment = "government"
	cloudChina      = "china"
)

type Config struct {
	Vnode              string                      `yaml:"vnode,omitempty" json:"vnode,omitempty"`
	UpdateEvery        int                         `yaml:"update_every,omitempty" json:"update_every,omitempty"`
	AutoDetectionRetry int                         `yaml:"autodetection_retry,omitempty" json:"autodetection_retry,omitempty"`
	SubscriptionIDs    []string                    `yaml:"subscription_ids" json:"subscription_ids"`
	Cloud              string                      `yaml:"cloud,omitempty" json:"cloud"`
	Discovery          DiscoveryConfig             `yaml:"discovery" json:"discovery"`
	Profiles           ProfilesConfig              `yaml:"profiles" json:"profiles"`
	QueryOffset        int                         `yaml:"query_offset,omitempty" json:"query_offset"`
	Timeout            confopt.Duration            `yaml:"timeout,omitempty" json:"timeout"`
	Limits             LimitsConfig                `yaml:"limits" json:"limits"`
	Auth               cloudauth.AzureADAuthConfig `yaml:"auth" json:"auth"`
}

type DiscoveryConfig struct {
	RefreshEvery int                    `yaml:"refresh_every,omitempty" json:"refresh_every"`
	Mode         string                 `yaml:"mode,omitempty" json:"mode"`
	ModeFilters  *ResourceFiltersConfig `yaml:"mode_filters,omitempty" json:"mode_filters,omitempty"`
	ModeQuery    *DiscoveryQueryConfig  `yaml:"mode_query,omitempty" json:"mode_query,omitempty"`
}

type ResourceFiltersConfig struct {
	ResourceGroups []string            `yaml:"resource_groups,omitempty" json:"resource_groups,omitempty"`
	Regions        []string            `yaml:"regions,omitempty" json:"regions,omitempty"`
	Tags           map[string][]string `yaml:"tags,omitempty" json:"tags,omitempty"`
}

type DiscoveryQueryConfig struct {
	KQL string `yaml:"kql" json:"kql"`
}

type ProfileEntryConfig struct {
	Name    string                 `yaml:"name" json:"name"`
	Filters *ResourceFiltersConfig `yaml:"filters,omitempty" json:"filters,omitempty"`
}

type ProfilesModeConfig struct {
	Entries []ProfileEntryConfig `yaml:"entries,omitempty" json:"entries,omitempty"`
}

type ProfilesConfig struct {
	Mode         string              `yaml:"mode,omitempty" json:"mode"`
	ModeAuto     *ProfilesModeConfig `yaml:"mode_auto,omitempty" json:"mode_auto,omitempty"`
	ModeExact    *ProfilesModeConfig `yaml:"mode_exact,omitempty" json:"mode_exact,omitempty"`
	ModeCombined *ProfilesModeConfig `yaml:"mode_combined,omitempty" json:"mode_combined,omitempty"`
}

type LimitsConfig struct {
	MaxConcurrency     int `yaml:"max_concurrency,omitempty" json:"max_concurrency"`
	MaxBatchResources  int `yaml:"max_batch_resources,omitempty" json:"max_batch_resources"`
	MaxMetricsPerQuery int `yaml:"max_metrics_per_query,omitempty" json:"max_metrics_per_query"`
}

func (c *Config) applyDefaults() {
	if c.UpdateEvery <= 0 {
		c.UpdateEvery = defaultUpdateEvery
	}
	if c.AutoDetectionRetry < 0 {
		c.AutoDetectionRetry = defaultAutoDetectRetry
	}
	if strings.TrimSpace(c.Cloud) == "" {
		c.Cloud = defaultCloud
	}
	if c.Discovery.RefreshEvery < 0 {
		c.Discovery.RefreshEvery = defaultDiscoveryEvery
	}
	if strings.TrimSpace(c.Discovery.Mode) == "" {
		c.Discovery.Mode = defaultDiscoveryMode
	}
	if strings.TrimSpace(c.Profiles.Mode) == "" {
		c.Profiles.Mode = defaultProfilesMode
	}
	if c.QueryOffset <= 0 {
		c.QueryOffset = defaultQueryOffset
	}
	if c.Timeout.Duration() == 0 {
		c.Timeout = defaultTimeout
	}
	if c.Limits.MaxConcurrency <= 0 {
		c.Limits.MaxConcurrency = defaultMaxConcurrency
	}
	if c.Limits.MaxBatchResources <= 0 {
		c.Limits.MaxBatchResources = defaultMaxBatchResource
	}
	if c.Limits.MaxMetricsPerQuery <= 0 {
		c.Limits.MaxMetricsPerQuery = defaultMaxMetricsQuery
	}
}

func (c Config) validate() error {
	var errs []error

	if len(c.SubscriptionIDs) == 0 {
		errs = append(errs, errors.New("'subscription_ids' must contain at least one value"))
	} else {
		for i, v := range c.SubscriptionIDs {
			if strings.TrimSpace(v) == "" {
				errs = append(errs, fmt.Errorf("'subscription_ids[%d]' must not be empty", i))
			}
		}
	}
	if c.UpdateEvery < 60 {
		errs = append(errs, errors.New("'update_every' must be >= 60 seconds"))
	}
	if c.Discovery.RefreshEvery < 0 || (c.Discovery.RefreshEvery > 0 && c.Discovery.RefreshEvery < 60) {
		errs = append(errs, errors.New("'discovery.refresh_every' must be 0 or >= 60 seconds"))
	}
	if c.QueryOffset < 60 {
		errs = append(errs, errors.New("'query_offset' must be >= 60 seconds"))
	}
	if c.Timeout.Duration() < 0 {
		errs = append(errs, errors.New("'timeout' cannot be negative"))
	}
	if c.Limits.MaxConcurrency < 1 || c.Limits.MaxConcurrency > 64 {
		errs = append(errs, errors.New("'limits.max_concurrency' must be between 1 and 64"))
	}
	if c.Limits.MaxBatchResources < 1 || c.Limits.MaxBatchResources > 50 {
		errs = append(errs, errors.New("'limits.max_batch_resources' must be between 1 and 50"))
	}
	if c.Limits.MaxMetricsPerQuery < 1 || c.Limits.MaxMetricsPerQuery > 20 {
		errs = append(errs, errors.New("'limits.max_metrics_per_query' must be between 1 and 20"))
	}

	switch stringsLowerTrim(c.Cloud) {
	case cloudPublic, cloudGovernment, cloudChina:
	default:
		errs = append(errs, fmt.Errorf("'cloud' must be one of: %s, %s, %s", cloudPublic, cloudGovernment, cloudChina))
	}

	if err := c.Auth.ValidateWithPath("auth"); err != nil {
		errs = append(errs, err)
	}

	validateProfileTags := false
	switch stringsLowerTrim(c.Discovery.Mode) {
	case discoveryModeFilters:
		validateProfileTags = true
		errs = append(errs, validateResourceFilters("discovery.mode_filters", c.Discovery.ModeFilters, true)...)
	case discoveryModeQuery:
		if c.Discovery.ModeQuery == nil || strings.TrimSpace(c.Discovery.ModeQuery.KQL) == "" {
			errs = append(errs, errors.New("'discovery.mode_query.kql' must not be empty when discovery.mode is 'query'"))
		}
	default:
		errs = append(errs, fmt.Errorf("'discovery.mode' must be one of: %s, %s", discoveryModeFilters, discoveryModeQuery))
	}

	switch stringsLowerTrim(c.Profiles.Mode) {
	case profilesModeAuto:
		errs = append(errs, validateProfileEntries("profiles.mode_auto.entries", modeEntries(c.Profiles.ModeAuto), validateProfileTags)...)
	case profilesModeExact:
		entries := modeEntries(c.Profiles.ModeExact)
		if len(entries) == 0 {
			errs = append(errs, fmt.Errorf("'profiles.mode_exact.entries' must not be empty when profiles.mode is '%s'", c.Profiles.Mode))
		} else {
			errs = append(errs, validateProfileEntries("profiles.mode_exact.entries", entries, validateProfileTags)...)
		}
	case profilesModeCombined:
		entries := modeEntries(c.Profiles.ModeCombined)
		if len(entries) == 0 {
			errs = append(errs, fmt.Errorf("'profiles.mode_combined.entries' must not be empty when profiles.mode is '%s'", c.Profiles.Mode))
		} else {
			errs = append(errs, validateProfileEntries("profiles.mode_combined.entries", entries, validateProfileTags)...)
		}
	default:
		errs = append(errs, fmt.Errorf("'profiles.mode' must be one of: %s, %s, %s",
			profilesModeAuto, profilesModeExact, profilesModeCombined))
	}

	return errors.Join(errs...)
}

func validateProfileEntries(path string, entries []ProfileEntryConfig, validateTags bool) []error {
	if len(entries) == 0 {
		return nil
	}

	var errs []error
	seen := map[string]struct{}{}
	for i, entry := range entries {
		entryPath := fmt.Sprintf("%s[%d]", path, i)
		name := stringsTrim(entry.Name)
		if !isValidProfileName(name) {
			errs = append(errs, fmt.Errorf("'%s.name' must match %q", entryPath, profileNamePattern))
		} else {
			if _, ok := seen[name]; ok {
				errs = append(errs, fmt.Errorf("'%s' contains duplicate entry name '%s'", path, name))
			}
			seen[name] = struct{}{}
		}
		errs = append(errs, validateResourceFilters(entryPath+".filters", entry.Filters, validateTags)...)
	}
	return errs
}

func validateResourceFilters(path string, filters *ResourceFiltersConfig, validateTags bool) []error {
	if filters == nil {
		return nil
	}

	var errs []error
	for i, v := range filters.ResourceGroups {
		if stringsTrim(v) == "" {
			errs = append(errs, fmt.Errorf("'%s.resource_groups[%d]' must not be empty", path, i))
		}
	}
	for i, v := range filters.Regions {
		if stringsTrim(v) == "" {
			errs = append(errs, fmt.Errorf("'%s.regions[%d]' must not be empty", path, i))
		}
	}
	if !validateTags && len(filters.Tags) > 0 {
		return errs
	}
	for key, values := range filters.Tags {
		if stringsTrim(key) == "" {
			errs = append(errs, fmt.Errorf("'%s.tags' contains an empty key", path))
			continue
		}
		if len(values) == 0 {
			errs = append(errs, fmt.Errorf("'%s.tags.%s' must contain at least one value", path, key))
			continue
		}
		for i, v := range values {
			if stringsTrim(v) == "" {
				errs = append(errs, fmt.Errorf("'%s.tags.%s[%d]' must not be empty", path, key, i))
			}
		}
	}
	return errs
}

func sanitizeIgnoredProfileTagFilters(cfg Config) (Config, []string) {
	if stringsLowerTrim(cfg.Discovery.Mode) != discoveryModeQuery {
		return cfg, nil
	}

	switch stringsLowerTrim(cfg.Profiles.Mode) {
	case profilesModeAuto:
		cfg.Profiles.ModeAuto = cloneProfilesModeConfig(cfg.Profiles.ModeAuto)
		warnings := stripIgnoredProfileTagFilters("profiles.mode_auto.entries", cfg.Profiles.ModeAuto)
		return cfg, warnings
	case profilesModeExact:
		cfg.Profiles.ModeExact = cloneProfilesModeConfig(cfg.Profiles.ModeExact)
		warnings := stripIgnoredProfileTagFilters("profiles.mode_exact.entries", cfg.Profiles.ModeExact)
		return cfg, warnings
	case profilesModeCombined:
		cfg.Profiles.ModeCombined = cloneProfilesModeConfig(cfg.Profiles.ModeCombined)
		warnings := stripIgnoredProfileTagFilters("profiles.mode_combined.entries", cfg.Profiles.ModeCombined)
		return cfg, warnings
	default:
		return cfg, nil
	}
}

func cloneProfilesModeConfig(src *ProfilesModeConfig) *ProfilesModeConfig {
	if src == nil {
		return nil
	}

	out := &ProfilesModeConfig{
		Entries: make([]ProfileEntryConfig, len(src.Entries)),
	}
	for i, entry := range src.Entries {
		out.Entries[i] = ProfileEntryConfig{
			Name:    entry.Name,
			Filters: cloneResourceFilters(entry.Filters),
		}
	}
	return out
}

func stripIgnoredProfileTagFilters(path string, cfg *ProfilesModeConfig) []string {
	if cfg == nil {
		return nil
	}

	var warnings []string
	for i := range cfg.Entries {
		filters := cfg.Entries[i].Filters
		if filters == nil || len(filters.Tags) == 0 {
			continue
		}

		warnings = append(warnings, fmt.Sprintf("%s[%d].filters.tags", path, i))
		filters.Tags = nil
		if len(filters.ResourceGroups) == 0 && len(filters.Regions) == 0 {
			cfg.Entries[i].Filters = nil
		}
	}
	return warnings
}

func modeEntries(cfg *ProfilesModeConfig) []ProfileEntryConfig {
	if cfg == nil {
		return nil
	}
	return cfg.Entries
}

func entryNames(entries []ProfileEntryConfig) []string {
	if len(entries) == 0 {
		return nil
	}

	names := make([]string, 0, len(entries))
	for _, entry := range entries {
		if name := stringsTrim(entry.Name); name != "" {
			names = append(names, name)
		}
	}
	return names
}

func entryMap(entries []ProfileEntryConfig) map[string]ProfileEntryConfig {
	if len(entries) == 0 {
		return nil
	}

	out := make(map[string]ProfileEntryConfig, len(entries))
	for _, entry := range entries {
		name := stringsTrim(entry.Name)
		if name == "" {
			continue
		}
		out[name] = ProfileEntryConfig{
			Name:    name,
			Filters: cloneResourceFilters(entry.Filters),
		}
	}
	return out
}

func cloneResourceFilters(src *ResourceFiltersConfig) *ResourceFiltersConfig {
	if src == nil {
		return nil
	}

	dst := &ResourceFiltersConfig{
		ResourceGroups: append([]string(nil), src.ResourceGroups...),
		Regions:        append([]string(nil), src.Regions...),
	}
	if len(src.Tags) > 0 {
		dst.Tags = make(map[string][]string, len(src.Tags))
		for key, values := range src.Tags {
			dst.Tags[key] = append([]string(nil), values...)
		}
	}
	return dst
}

func (c Config) primarySubscriptionID() string {
	for _, id := range c.SubscriptionIDs {
		if v := stringsTrim(id); v != "" {
			return v
		}
	}
	return ""
}

func (c Config) subscriptionIDs() []string {
	out := make([]string, 0, len(c.SubscriptionIDs))
	for _, id := range c.SubscriptionIDs {
		if v := stringsTrim(id); v != "" {
			out = append(out, v)
		}
	}
	return out
}
