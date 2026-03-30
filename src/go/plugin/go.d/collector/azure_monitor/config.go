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
	defaultQueryOffset      = 180
	defaultTimeout          = confopt.Duration(30 * time.Second)
	defaultMaxConcurrency   = 4
	defaultMaxBatchResource = 50
	defaultMaxMetricsQuery  = 20
)

const (
	profileSelectionModeAuto     = "auto"
	profileSelectionModeExact    = "exact"
	profileSelectionModeCombined = "combined"
)

const (
	cloudPublic     = "public"
	cloudGovernment = "government"
	cloudChina      = "china"
)

type Config struct {
	Vnode                        string                              `yaml:"vnode,omitempty" json:"vnode"`
	UpdateEvery                  int                                 `yaml:"update_every,omitempty" json:"update_every"`
	AutoDetectionRetry           int                                 `yaml:"autodetection_retry,omitempty" json:"autodetection_retry"`
	SubscriptionID               string                              `yaml:"subscription_id" json:"subscription_id"`
	Cloud                        string                              `yaml:"cloud,omitempty" json:"cloud"`
	DiscoveryEvery               int                                 `yaml:"discovery_every,omitempty" json:"discovery_every"`
	QueryOffset                  int                                 `yaml:"query_offset,omitempty" json:"query_offset"`
	Timeout                      confopt.Duration                    `yaml:"timeout,omitempty" json:"timeout"`
	MaxConcurrency               int                                 `yaml:"max_concurrency,omitempty" json:"max_concurrency"`
	MaxBatchResources            int                                 `yaml:"max_batch_resources,omitempty" json:"max_batch_resources"`
	MaxMetricsPerQuery           int                                 `yaml:"max_metrics_per_query,omitempty" json:"max_metrics_per_query"`
	ResourceGroups               []string                            `yaml:"resource_groups,omitempty" json:"resource_groups"`
	ProfileSelectionMode         string                              `yaml:"profile_selection_mode,omitempty" json:"profile_selection_mode"`
	ProfileSelectionModeExact    *ProfileSelectionModeExactConfig    `yaml:"profile_selection_mode_exact,omitempty" json:"profile_selection_mode_exact,omitempty"`
	ProfileSelectionModeCombined *ProfileSelectionModeCombinedConfig `yaml:"profile_selection_mode_combined,omitempty" json:"profile_selection_mode_combined,omitempty"`
	Auth                         cloudauth.AzureADAuthConfig         `yaml:"auth" json:"auth"`
}

type ProfileSelectionModeExactConfig struct {
	Profiles []string `yaml:"profiles" json:"profiles"`
}

type ProfileSelectionModeCombinedConfig struct {
	Profiles []string `yaml:"profiles" json:"profiles"`
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
	if c.DiscoveryEvery <= 0 {
		c.DiscoveryEvery = defaultDiscoveryEvery
	}
	if c.QueryOffset <= 0 {
		c.QueryOffset = defaultQueryOffset
	}
	if c.Timeout.Duration() == 0 {
		c.Timeout = defaultTimeout
	}
	if c.MaxConcurrency <= 0 {
		c.MaxConcurrency = defaultMaxConcurrency
	}
	if c.MaxBatchResources <= 0 {
		c.MaxBatchResources = defaultMaxBatchResource
	}
	if c.MaxMetricsPerQuery <= 0 {
		c.MaxMetricsPerQuery = defaultMaxMetricsQuery
	}
	if strings.TrimSpace(c.ProfileSelectionMode) == "" {
		c.ProfileSelectionMode = profileSelectionModeAuto
	}
}

func (c Config) validate() error {
	var errs []error

	if strings.TrimSpace(c.SubscriptionID) == "" {
		errs = append(errs, errors.New("'subscription_id' is required"))
	}
	if c.UpdateEvery < 60 {
		errs = append(errs, errors.New("'update_every' must be >= 60 seconds"))
	}
	if c.DiscoveryEvery < 60 {
		errs = append(errs, errors.New("'discovery_every' must be >= 60 seconds"))
	}
	if c.QueryOffset < 60 {
		errs = append(errs, errors.New("'query_offset' must be >= 60 seconds"))
	}
	if c.Timeout.Duration() < 0 {
		errs = append(errs, errors.New("'timeout' cannot be negative"))
	}
	if c.MaxConcurrency < 1 || c.MaxConcurrency > 64 {
		errs = append(errs, errors.New("'max_concurrency' must be between 1 and 64"))
	}
	if c.MaxBatchResources < 1 || c.MaxBatchResources > 50 {
		errs = append(errs, errors.New("'max_batch_resources' must be between 1 and 50"))
	}
	if c.MaxMetricsPerQuery < 1 || c.MaxMetricsPerQuery > 20 {
		errs = append(errs, errors.New("'max_metrics_per_query' must be between 1 and 20"))
	}

	switch strings.ToLower(strings.TrimSpace(c.Cloud)) {
	case cloudPublic, cloudGovernment, cloudChina:
	default:
		errs = append(errs, fmt.Errorf("'cloud' must be one of: %s, %s, %s", cloudPublic, cloudGovernment, cloudChina))
	}

	if err := c.Auth.ValidateWithPath("auth"); err != nil {
		errs = append(errs, err)
	}

	switch strings.ToLower(strings.TrimSpace(c.ProfileSelectionMode)) {
	case profileSelectionModeAuto:
	case profileSelectionModeExact:
		if c.ProfileSelectionModeExact == nil || len(c.ProfileSelectionModeExact.Profiles) == 0 {
			errs = append(errs, errors.New("'profile_selection_mode_exact.profiles' must not be empty when mode is 'exact'"))
		} else {
			errs = append(errs, validateProfilesList(c.ProfileSelectionModeExact.Profiles)...)
		}
	case profileSelectionModeCombined:
		if c.ProfileSelectionModeCombined == nil || len(c.ProfileSelectionModeCombined.Profiles) == 0 {
			errs = append(errs, errors.New("'profile_selection_mode_combined.profiles' must not be empty when mode is 'combined'"))
		} else {
			errs = append(errs, validateProfilesList(c.ProfileSelectionModeCombined.Profiles)...)
		}
	default:
		errs = append(errs, fmt.Errorf("'profile_selection_mode' must be one of: %s, %s, %s",
			profileSelectionModeAuto, profileSelectionModeExact, profileSelectionModeCombined))
	}

	return errors.Join(errs...)
}

func validateProfilesList(profiles []string) []error {
	var errs []error
	seen := map[string]struct{}{}
	for _, name := range profiles {
		n := strings.TrimSpace(name)
		if n == "" {
			errs = append(errs, errors.New("'profiles' contains an empty value"))
			continue
		}
		norm := stringsLowerTrim(n)
		if _, ok := seen[norm]; ok {
			errs = append(errs, fmt.Errorf("'profiles' contains duplicate value '%s'", n))
		}
		seen[norm] = struct{}{}
	}
	return errs
}
