// SPDX-License-Identifier: GPL-3.0-or-later

package profilemetrics

import (
	"errors"
	"fmt"
	"slices"
	"strings"

	"github.com/netdata/netdata/go/plugins/plugin/go.d/collector/snmp_traps/internal/attribution"
	"github.com/netdata/netdata/go/plugins/plugin/go.d/collector/snmp_traps/internal/catalog"
)

const (
	defaultProfileMetricMaxRules              = 500
	defaultProfileMetricMaxSources            = 2000
	defaultProfileMetricMaxResourcesPerSource = 512
	defaultProfileMetricMaxInstancesPerJob    = 50000
)

type profileMetricIdentityPolicy struct {
	Device attribution.DeviceMode
}

type profileMetricLimitsPolicy struct {
	MaxRules              int
	MaxSources            int
	MaxResourcesPerSource int
	MaxInstancesPerJob    int
}

type profileMetricRule = catalog.MetricRule
type profileMetricChart = catalog.MetricChart
type profileMetricPredicate = catalog.MetricPredicate

// Policy is the normalized, immutable per-job profile-metric policy.
type Policy struct {
	enabled  bool
	include  []string
	identity profileMetricIdentityPolicy
	limits   profileMetricLimitsPolicy
}

// Normalize validates and copies the public job configuration.
func Normalize(enabled bool, names []string) (Policy, error) {
	seen := make(map[string]bool, len(names))
	include := make([]string, 0, len(names))
	for i, name := range names {
		name = strings.TrimSpace(name)
		if name == "" {
			return Policy{}, fmt.Errorf("profile_metrics.include[%d] is empty", i)
		}
		if seen[name] {
			return Policy{}, fmt.Errorf("profile_metrics.include[%d]: duplicate rule %q", i, name)
		}
		seen[name] = true
		include = append(include, name)
	}
	if enabled && len(include) == 0 {
		return Policy{}, errors.New("profile_metrics.include must contain at least one rule when enabled")
	}

	return Policy{
		enabled:  enabled,
		include:  include,
		identity: defaultProfileMetricIdentityPolicy(),
		limits:   defaultProfileMetricLimitsPolicy(),
	}, nil
}

func (p Policy) Enabled() bool { return p.enabled }

func defaultProfileMetricIdentityPolicy() profileMetricIdentityPolicy {
	return profileMetricIdentityPolicy{
		Device: attribution.DeviceSource,
	}
}

func defaultProfileMetricLimitsPolicy() profileMetricLimitsPolicy {
	return profileMetricLimitsPolicy{
		MaxRules:              defaultProfileMetricMaxRules,
		MaxSources:            defaultProfileMetricMaxSources,
		MaxResourcesPerSource: defaultProfileMetricMaxResourcesPerSource,
		MaxInstancesPerJob:    defaultProfileMetricMaxInstancesPerJob,
	}
}

type profileMetricCatalog struct {
	rulesByName map[string]*profileMetricRule
	chartsByID  map[string]*profileMetricChart
}

func selectProfileMetricRules(cfg Policy, cat profileMetricCatalog) ([]*profileMetricRule, error) {
	if !cfg.enabled {
		return nil, nil
	}
	selected := make([]*profileMetricRule, 0, len(cfg.include))
	for _, name := range cfg.include {
		rule := cat.rulesByName[name]
		if rule == nil {
			return nil, fmt.Errorf("profile_metrics.include rule %q not found", name)
		}
		if rule.Disabled() {
			return nil, fmt.Errorf("profile_metrics.include rule %q is disabled by profile", name)
		}
		selected = append(selected, rule)
	}
	if len(selected) > cfg.limits.MaxRules {
		return nil, fmt.Errorf("profile_metrics selected %d rules, above fixed maximum %d", len(selected), cfg.limits.MaxRules)
	}
	slices.SortFunc(selected, func(a, b *profileMetricRule) int {
		return strings.Compare(a.Name, b.Name)
	})
	return selected, nil
}
