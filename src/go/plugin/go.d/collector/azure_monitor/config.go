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
	RefreshEvery int                     `yaml:"refresh_every,omitempty" json:"refresh_every"`
	Mode         string                  `yaml:"mode,omitempty" json:"mode"`
	ModeFilters  *DiscoveryFiltersConfig `yaml:"mode_filters,omitempty" json:"mode_filters,omitempty"`
	ModeQuery    *DiscoveryQueryConfig   `yaml:"mode_query,omitempty" json:"mode_query,omitempty"`
}

type DiscoveryFiltersConfig struct {
	ResourceGroups []string            `yaml:"resource_groups,omitempty" json:"resource_groups,omitempty"`
	Regions        []string            `yaml:"regions,omitempty" json:"regions,omitempty"`
	Tags           map[string][]string `yaml:"tags,omitempty" json:"tags,omitempty"`
}

type DiscoveryQueryConfig struct {
	KQL string `yaml:"kql" json:"kql"`
}

type ProfilesConfig struct {
	Mode  string   `yaml:"mode,omitempty" json:"mode"`
	Names []string `yaml:"names,omitempty" json:"names,omitempty"`
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

	switch strings.ToLower(strings.TrimSpace(c.Cloud)) {
	case cloudPublic, cloudGovernment, cloudChina:
	default:
		errs = append(errs, fmt.Errorf("'cloud' must be one of: %s, %s, %s", cloudPublic, cloudGovernment, cloudChina))
	}

	if err := c.Auth.ValidateWithPath("auth"); err != nil {
		errs = append(errs, err)
	}

	switch strings.ToLower(strings.TrimSpace(c.Discovery.Mode)) {
	case discoveryModeFilters:
		if c.Discovery.ModeQuery != nil {
			errs = append(errs, errors.New("'discovery.mode_query' is only allowed when discovery.mode is 'query'"))
		}
		errs = append(errs, validateDiscoveryFilters(c.Discovery.ModeFilters)...)
	case discoveryModeQuery:
		if c.Discovery.ModeFilters != nil {
			errs = append(errs, errors.New("'discovery.mode_filters' is only allowed when discovery.mode is 'filters'"))
		}
		if c.Discovery.ModeQuery == nil || strings.TrimSpace(c.Discovery.ModeQuery.KQL) == "" {
			errs = append(errs, errors.New("'discovery.mode_query.kql' must not be empty when discovery.mode is 'query'"))
		}
	default:
		errs = append(errs, fmt.Errorf("'discovery.mode' must be one of: %s, %s", discoveryModeFilters, discoveryModeQuery))
	}

	switch strings.ToLower(strings.TrimSpace(c.Profiles.Mode)) {
	case profilesModeAuto:
		if len(c.Profiles.Names) > 0 {
			errs = append(errs, errors.New("'profiles.names' must be empty when profiles.mode is 'auto'"))
		}
	case profilesModeExact, profilesModeCombined:
		if len(c.Profiles.Names) == 0 {
			errs = append(errs, fmt.Errorf("'profiles.names' must not be empty when profiles.mode is '%s'", c.Profiles.Mode))
		} else {
			errs = append(errs, validateProfilesList(c.Profiles.Names)...)
		}
	default:
		errs = append(errs, fmt.Errorf("'profiles.mode' must be one of: %s, %s, %s",
			profilesModeAuto, profilesModeExact, profilesModeCombined))
	}

	return errors.Join(errs...)
}

func validateDiscoveryFilters(filters *DiscoveryFiltersConfig) []error {
	if filters == nil {
		return nil
	}

	var errs []error
	for _, v := range filters.ResourceGroups {
		if strings.TrimSpace(v) == "" {
			continue
		}
	}
	for _, v := range filters.Regions {
		if strings.TrimSpace(v) == "" {
			continue
		}
	}
	for key, values := range filters.Tags {
		if strings.TrimSpace(key) == "" {
			errs = append(errs, errors.New("'discovery.mode_filters.tags' contains an empty key"))
			continue
		}
		if len(values) == 0 {
			errs = append(errs, fmt.Errorf("'discovery.mode_filters.tags.%s' must contain at least one value", key))
			continue
		}
		for i, v := range values {
			if strings.TrimSpace(v) == "" {
				errs = append(errs, fmt.Errorf("'discovery.mode_filters.tags.%s[%d]' must not be empty", key, i))
			}
		}
	}
	return errs
}

func validateProfilesList(profiles []string) []error {
	var errs []error
	seen := map[string]struct{}{}
	for _, name := range profiles {
		n := strings.TrimSpace(name)
		if n == "" {
			errs = append(errs, errors.New("'profiles.names' contains an empty value"))
			continue
		}
		norm := stringsLowerTrim(n)
		if _, ok := seen[norm]; ok {
			errs = append(errs, fmt.Errorf("'profiles.names' contains duplicate value '%s'", n))
		}
		seen[norm] = struct{}{}
	}
	return errs
}

func (c Config) primarySubscriptionID() string {
	for _, id := range c.SubscriptionIDs {
		if v := strings.TrimSpace(id); v != "" {
			return v
		}
	}
	return ""
}

func (c Config) subscriptionIDs() []string {
	out := make([]string, 0, len(c.SubscriptionIDs))
	for _, id := range c.SubscriptionIDs {
		if v := strings.TrimSpace(id); v != "" {
			out = append(out, v)
		}
	}
	return out
}
