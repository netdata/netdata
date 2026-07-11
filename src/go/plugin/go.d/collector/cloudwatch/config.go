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
	defaultTimeout          = confopt.Duration(30 * time.Second)
)

// apiConcurrency bounds concurrent AWS API calls (discovery fan-out and
// GetMetricData query chunks); 5 stays well under CloudWatch's account-level
// request rates. metricsPerQuery is the GetMetricData batch size; 500 is the AWS
// hard maximum per call. Neither is an operator decision, so both are fixed
// constants rather than config.
const (
	apiConcurrency  = 5
	metricsPerQuery = 500
)

type Config struct {
	UpdateEvery        int                                 `yaml:"update_every,omitempty" json:"update_every,omitempty"`
	AutoDetectionRetry int                                 `yaml:"autodetection_retry,omitempty" json:"autodetection_retry,omitempty"`
	Vnode              string                              `yaml:"vnode,omitempty" json:"vnode"`
	Credentials        map[string]awsauth.CredentialConfig `yaml:"credentials" json:"credentials"`
	Targets            []TargetConfig                      `yaml:"targets" json:"targets"`
	Rules              []RuleConfig                        `yaml:"rules" json:"rules"`
	Discovery          DiscoveryConfig                     `yaml:"discovery" json:"discovery"`
	Tags               []TagConfig                         `yaml:"tags,omitempty" json:"tags,omitempty"`
	QueryOffset        int                                 `yaml:"query_offset,omitempty" json:"query_offset"`
	Timeout            confopt.Duration                    `yaml:"timeout,omitempty" json:"timeout"`
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
}

type ProfileSelectorConfig struct {
	Defaults *bool    `yaml:"defaults,omitempty" json:"defaults,omitempty"`
	Include  []string `yaml:"include,omitempty" json:"include,omitempty"`
	Exclude  []string `yaml:"exclude,omitempty" json:"exclude,omitempty"`
}

func (c *ProfileSelectorConfig) includesDefaults() bool {
	return c == nil || c.Defaults == nil || *c.Defaults
}

// TagConfig selects one AWS resource tag to emit as an additional label. Name is
// the AWS tag key (case-sensitive). Rename optionally sets the Netdata label name;
// without it the label is the sanitized tag key. An empty allowlist disables tag
// enrichment entirely (no RGTA calls, no extra IAM). Tag resolution -- sanitize,
// collision skip-and-warn, per-service ARN join -- is non-fatal and runs after
// profile selection, never in config validation.
type TagConfig struct {
	Name   string `yaml:"name" json:"name"`
	Rename string `yaml:"rename,omitempty" json:"rename,omitempty"`
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
	if err := validateConfigStructure(c); err != nil {
		errs = append(errs, err)
	}

	return errors.Join(errs...)
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
