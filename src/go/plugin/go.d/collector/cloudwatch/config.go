// SPDX-License-Identifier: GPL-3.0-or-later

package cloudwatch

import (
	"errors"
	"time"

	"github.com/netdata/netdata/go/plugins/pkg/confopt"
	"github.com/netdata/netdata/go/plugins/plugin/go.d/collector/cloudwatch/internal/awsauth"
	"github.com/netdata/netdata/go/plugins/plugin/go.d/collector/cloudwatch/internal/awsregion"
)

const (
	defaultUpdateEvery      = 60
	defaultAutoDetectRetry  = 0
	defaultDiscoveryRefresh = 300
	defaultQueryOffset      = 600
	defaultMaxInstances     = 1000
	defaultTimeout          = confopt.Duration(30 * time.Second)
)

// apiConcurrency bounds concurrent AWS API calls (ListMetrics discovery,
// resource-tag lookup, and GetMetricData query chunks). metricsPerQuery is the
// GetMetricData batch size; 500 is the AWS hard maximum per call. Neither is an
// operator decision, so both are fixed constants rather than config.
const (
	apiConcurrency  = 5
	metricsPerQuery = 500
)

type Config struct {
	UpdateEvery        int                      `yaml:"update_every,omitempty" json:"update_every,omitempty"`
	AutoDetectionRetry int                      `yaml:"autodetection_retry,omitempty" json:"autodetection_retry,omitempty"`
	Vnode              string                   `yaml:"vnode,omitempty" json:"vnode"`
	Credentials        []CredentialSourceConfig `yaml:"credentials" json:"credentials"`
	Targets            []TargetConfig           `yaml:"targets" json:"targets"`
	Rules              []RuleConfig             `yaml:"rules" json:"rules"`
	Defaults           DefaultsConfig           `yaml:"defaults,omitempty" json:"defaults,omitempty"`
	Labels             LabelsConfig             `yaml:"labels,omitempty" json:"labels,omitempty"`
	Limits             LimitsConfig             `yaml:"limits,omitempty" json:"limits,omitempty"`
	Discovery          DiscoveryConfig          `yaml:"discovery" json:"discovery"`
	QueryOffset        int                      `yaml:"query_offset,omitempty" json:"query_offset"`
	Timeout            confopt.Duration         `yaml:"timeout,omitempty" json:"timeout"`
}

// CredentialSourceConfig names one reusable base-credential configuration.
type CredentialSourceConfig struct {
	Name                     string `yaml:"name" json:"name"`
	awsauth.CredentialConfig `yaml:",inline"`
}

type TargetConfig struct {
	Name        string                    `yaml:"name" json:"name"`
	Credentials string                    `yaml:"credentials" json:"credentials"`
	AssumeRole  *awsauth.AssumeRoleConfig `yaml:"assume_role,omitempty" json:"assume_role,omitempty"`
}

type RuleConfig struct {
	Name     string                 `yaml:"name" json:"name"`
	Targets  []string               `yaml:"targets" json:"targets"`
	Profiles *ProfileSelectorConfig `yaml:"profiles,omitempty" json:"profiles,omitempty"`
	Regions  []string               `yaml:"regions" json:"regions"`
	Filters  *RuleFiltersConfig     `yaml:"filters,omitempty" json:"filters,omitempty"`
}

type DefaultsConfig struct {
	Filters DefaultFiltersConfig `yaml:"filters,omitempty" json:"filters,omitempty"`
}

type DefaultFiltersConfig struct {
	ResourceTags []ResourceTagFilterConfig `yaml:"resource_tags,omitempty" json:"resource_tags,omitempty"`
}

// RuleFiltersConfig distinguishes an omitted resource_tags field (inherit the
// per-job default) from an explicitly empty list (disable it for this rule).
type RuleFiltersConfig struct {
	ResourceTags *[]ResourceTagFilterConfig `yaml:"resource_tags,omitempty" json:"resource_tags,omitempty"`
}

type ResourceTagFilterConfig struct {
	Key    string   `yaml:"key" json:"key"`
	Values []string `yaml:"values" json:"values"`
}

type LabelsConfig struct {
	ResourceTags []ResourceTagLabelConfig `yaml:"resource_tags,omitempty" json:"resource_tags,omitempty"`
}

// ResourceTagLabelConfig maps one exact AWS tag key to a Netdata label. Label
// defaults to the sanitized AWS key when omitted.
type ResourceTagLabelConfig struct {
	Key   string `yaml:"key" json:"key"`
	Label string `yaml:"label,omitempty" json:"label,omitempty"`
}

type LimitsConfig struct {
	MaxInstances int `yaml:"max_instances,omitempty" json:"max_instances"`
}

type ProfileSelectorConfig struct {
	Defaults *bool    `yaml:"defaults,omitempty" json:"defaults,omitempty"`
	Include  []string `yaml:"include,omitempty" json:"include,omitempty"`
	Exclude  []string `yaml:"exclude,omitempty" json:"exclude,omitempty"`
}

func (c *ProfileSelectorConfig) includesDefaults() bool {
	return c == nil || c.Defaults == nil || *c.Defaults
}

type DiscoveryConfig struct {
	RefreshEvery int `yaml:"refresh_every,omitempty" json:"refresh_every"`
	// RecentlyActiveOnly is period-aware: when enabled, ListMetrics uses
	// RecentlyActive=PT3H only for metrics whose period is <= 3h. It is a
	// pointer so the (true) default can be distinguished from an explicit false.
	RecentlyActiveOnly *bool `yaml:"recently_active_only,omitempty" json:"recently_active_only,omitempty"`
}

func (c *Config) applyDefaults() {
	if c.UpdateEvery <= 0 {
		c.UpdateEvery = defaultUpdateEvery
	}
	if c.AutoDetectionRetry < 0 {
		c.AutoDetectionRetry = defaultAutoDetectRetry
	}
	if c.Discovery.RefreshEvery <= 0 {
		c.Discovery.RefreshEvery = defaultDiscoveryRefresh
	}
	if c.Discovery.RecentlyActiveOnly == nil {
		v := true
		c.Discovery.RecentlyActiveOnly = &v
	}
	if c.QueryOffset <= 0 {
		c.QueryOffset = defaultQueryOffset
	}
	if c.Limits.MaxInstances == 0 {
		c.Limits.MaxInstances = defaultMaxInstances
	}
	if c.Timeout.Duration() == 0 {
		c.Timeout = defaultTimeout
	}
}

func (c Config) validate() error {
	var errs []error

	if c.UpdateEvery < 60 {
		errs = append(errs, errors.New("'update_every' must be >= 60 seconds (CloudWatch minimum period)"))
	}
	if c.Discovery.RefreshEvery < 60 {
		errs = append(errs, errors.New("'discovery.refresh_every' must be >= 60 seconds"))
	}
	if c.QueryOffset < 0 {
		errs = append(errs, errors.New("'query_offset' cannot be negative"))
	}
	if c.Timeout.Duration() < 0 {
		errs = append(errs, errors.New("'timeout' cannot be negative"))
	}
	if c.Limits.MaxInstances < 0 {
		errs = append(errs, errors.New("'limits.max_instances' must be >= 1"))
	}
	if err := validateConfigStructure(c); err != nil {
		errs = append(errs, err)
	}

	return errors.Join(errs...)
}

func (r RuleConfig) effectiveResourceTagFilters(defaults []ResourceTagFilterConfig) []ResourceTagFilterConfig {
	if r.Filters == nil || r.Filters.ResourceTags == nil {
		return defaults
	}
	return *r.Filters.ResourceTags
}

func normalizeRegions(regions []string) []string {
	out := make([]string, 0, len(regions))
	seen := make(map[string]struct{}, len(regions))
	for _, r := range regions {
		v := awsregion.Normalize(r)
		if v == "" {
			continue
		}
		if _, dup := seen[v]; dup {
			continue // a duplicate region would double discovery/query cost
		}
		seen[v] = struct{}{}
		out = append(out, v)
	}
	return out
}

func (c Config) recentlyActiveOnly() bool {
	return c.Discovery.RecentlyActiveOnly == nil || *c.Discovery.RecentlyActiveOnly
}
