// SPDX-License-Identifier: GPL-3.0-or-later

package cloudwatch

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
	defaultDiscoveryRefresh = 300
	defaultQueryOffset      = 600
	defaultTimeout          = confopt.Duration(30 * time.Second)

	profilesModeAuto     = "auto"
	profilesModeExact    = "exact"
	profilesModeCombined = "combined"
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
	UpdateEvery        int                     `yaml:"update_every,omitempty" json:"update_every,omitempty"`
	AutoDetectionRetry int                     `yaml:"autodetection_retry,omitempty" json:"autodetection_retry,omitempty"`
	Vnode              string                  `yaml:"vnode,omitempty" json:"vnode"`
	Regions            []string                `yaml:"regions" json:"regions"`
	Auth               cloudauth.AWSAuthConfig `yaml:"auth" json:"auth"`
	Profiles           ProfilesConfig          `yaml:"profiles" json:"profiles"`
	Discovery          DiscoveryConfig         `yaml:"discovery" json:"discovery"`
	QueryOffset        int                     `yaml:"query_offset,omitempty" json:"query_offset"`
	Timeout            confopt.Duration        `yaml:"timeout,omitempty" json:"timeout"`
}

type ProfilesConfig struct {
	Mode      string               `yaml:"mode,omitempty" json:"mode"`
	ModeExact *ProfilesExactConfig `yaml:"mode_exact,omitempty" json:"mode_exact,omitempty"`
}

type ProfilesExactConfig struct {
	Entries []ProfileEntry `yaml:"entries,omitempty" json:"entries,omitempty"`
}

// ProfileEntry names one profile (by basename) to collect in exact mode.
type ProfileEntry struct {
	Name string `yaml:"name" json:"name"`
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
	if strings.TrimSpace(c.Profiles.Mode) == "" {
		c.Profiles.Mode = profilesModeAuto
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

	regions := c.regions()
	if len(regions) == 0 {
		errs = append(errs, errors.New("'regions' must contain at least one value"))
	}

	// All regions must share one AWS partition: account_id is resolved once from
	// the first region's STS endpoint and stamped on every series, so a
	// mixed-partition job (e.g. us-east-1 + cn-north-1) would mislabel metrics.
	partitions := make(map[string]struct{})
	for _, r := range regions {
		partitions[regionPartition(r)] = struct{}{}
	}
	if len(partitions) > 1 {
		errs = append(errs, fmt.Errorf("all 'regions' must be in one AWS partition; got multiple partitions across %v", regions))
	}

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

	switch strings.ToLower(strings.TrimSpace(c.Profiles.Mode)) {
	case profilesModeAuto:
	case profilesModeCombined:
	case profilesModeExact:
		if c.Profiles.ModeExact == nil || len(c.Profiles.ModeExact.Entries) == 0 {
			errs = append(errs, fmt.Errorf("'profiles.mode_exact.entries' must not be empty when profiles.mode is %q", profilesModeExact))
		} else {
			for i, e := range c.Profiles.ModeExact.Entries {
				if strings.TrimSpace(e.Name) == "" {
					errs = append(errs, fmt.Errorf("'profiles.mode_exact.entries[%d].name' must not be empty", i))
				}
			}
		}
	default:
		errs = append(errs, fmt.Errorf("'profiles.mode' must be one of: %s, %s, %s", profilesModeAuto, profilesModeExact, profilesModeCombined))
	}

	if err := c.Auth.ValidateWithPath("auth"); err != nil {
		errs = append(errs, err)
	}

	return errors.Join(errs...)
}

// regionPartition maps an AWS region to its partition. account_id and the STS
// endpoint are partition-scoped, so a job's regions must not span partitions.
func regionPartition(region string) string {
	switch {
	case strings.HasPrefix(region, "us-gov-"):
		return "aws-us-gov"
	case strings.HasPrefix(region, "cn-"):
		return "aws-cn"
	case strings.HasPrefix(region, "us-isob-"):
		return "aws-iso-b"
	case strings.HasPrefix(region, "us-isof-"):
		return "aws-iso-f"
	case strings.HasPrefix(region, "us-iso-"):
		return "aws-iso"
	case strings.HasPrefix(region, "eu-isoe-"):
		return "aws-iso-e"
	case strings.HasPrefix(region, "eusc-"):
		return "aws-eusc"
	default:
		// Unrecognized regions fall back to the standard partition; extend this
		// switch as AWS ships new partitions. A wrong guess here is never worse
		// than this fallback, which is the pre-existing behavior.
		return "aws"
	}
}

func (c Config) regions() []string {
	out := make([]string, 0, len(c.Regions))
	seen := make(map[string]struct{}, len(c.Regions))
	for _, r := range c.Regions {
		// AWS region codes are canonically lowercase, so lowercasing is loss-less and
		// makes both dedupe and partition detection (regionPartition) case-insensitive.
		v := strings.ToLower(strings.TrimSpace(r))
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
