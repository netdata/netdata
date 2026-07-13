// SPDX-License-Identifier: GPL-3.0-or-later

package cwquery

import (
	"fmt"
	"time"

	"github.com/netdata/netdata/go/plugins/pkg/confopt"
)

const (
	minPeriod               = time.Minute
	maxPeriod               = 24 * time.Hour
	DefaultPublicationDelay = 10 * time.Minute
	maxReliableHorizon      = 14 * 24 * time.Hour
	maxBuckets              = 1440
)

// Config defines optional CloudWatch query defaults. Callers compose profile,
// metric, job-default, and rule values field by field through Resolve.
type Config struct {
	Period           *confopt.LongDuration `yaml:"period,omitempty" json:"period,omitempty"`
	Lookback         *confopt.LongDuration `yaml:"lookback,omitempty" json:"lookback,omitempty"`
	PublicationDelay *confopt.LongDuration `yaml:"publication_delay,omitempty" json:"publication_delay,omitempty"`
}

// Policy is the fully resolved timing contract for one exported series.
type Policy struct {
	Period           time.Duration
	Lookback         time.Duration
	PublicationDelay time.Duration
}

// Validate checks the raw fields present in cfg. Cross-field constraints are
// checked by Resolve after inheritance has produced an effective policy.
func Validate(path string, cfg *Config) error {
	if cfg == nil {
		return nil
	}
	if cfg.Period != nil {
		if err := validatePeriod(path+".period", cfg.Period.Duration()); err != nil {
			return err
		}
	}
	if cfg.Lookback != nil {
		if err := validateWholeSeconds(path+".lookback", cfg.Lookback.Duration(), false); err != nil {
			return err
		}
	}
	if cfg.PublicationDelay != nil {
		if err := validateWholeSeconds(path+".publication_delay", cfg.PublicationDelay.Duration(), true); err != nil {
			return err
		}
	}
	return nil
}

// ValidateProfile checks the required profile-level query defaults.
func ValidateProfile(path string, cfg Config) error {
	if cfg.Period == nil {
		return fmt.Errorf("%s.period is required", path)
	}
	return Validate(path, &cfg)
}

// Resolve applies field precedence rule > rule defaults > metric > profile,
// then validates the effective policy. Lookback falls back to the resolved
// period and publication delay to the collector-wide default.
func Resolve(path string, rule, defaults, metric *Config, profile Config) (Policy, error) {
	if profile.Period == nil {
		return Policy{}, fmt.Errorf("profile query.period is required")
	}
	period, periodPath := profile.Period.Duration(), "profile query.period"
	if metric != nil && metric.Period != nil {
		period, periodPath = metric.Period.Duration(), "profile metric query.period"
	}
	if defaults != nil && defaults.Period != nil {
		period, periodPath = defaults.Period.Duration(), "rule_defaults.query.period"
	}
	if rule != nil && rule.Period != nil {
		period, periodPath = rule.Period.Duration(), path+".query.period"
	}

	lookback, lookbackPath := period, periodPath
	if profile.Lookback != nil {
		lookback, lookbackPath = profile.Lookback.Duration(), "profile query.lookback"
	}
	if metric != nil && metric.Lookback != nil {
		lookback, lookbackPath = metric.Lookback.Duration(), "profile metric query.lookback"
	}
	if defaults != nil && defaults.Lookback != nil {
		lookback, lookbackPath = defaults.Lookback.Duration(), "rule_defaults.query.lookback"
	}
	if rule != nil && rule.Lookback != nil {
		lookback, lookbackPath = rule.Lookback.Duration(), path+".query.lookback"
	}

	delay, delayPath := DefaultPublicationDelay, "built-in publication delay"
	if profile.PublicationDelay != nil {
		delay, delayPath = profile.PublicationDelay.Duration(), "profile query.publication_delay"
	}
	if metric != nil && metric.PublicationDelay != nil {
		delay, delayPath = metric.PublicationDelay.Duration(), "profile metric query.publication_delay"
	}
	if defaults != nil && defaults.PublicationDelay != nil {
		delay, delayPath = defaults.PublicationDelay.Duration(), "rule_defaults.query.publication_delay"
	}
	if rule != nil && rule.PublicationDelay != nil {
		delay, delayPath = rule.PublicationDelay.Duration(), path+".query.publication_delay"
	}

	if err := validatePeriod(periodPath, period); err != nil {
		return Policy{}, err
	}
	if err := validateWholeSeconds(lookbackPath, lookback, false); err != nil {
		return Policy{}, err
	}
	if err := validateWholeSeconds(delayPath, delay, true); err != nil {
		return Policy{}, err
	}
	if lookback < period {
		return Policy{}, fmt.Errorf("%s must be at least the effective period %s", lookbackPath, period)
	}
	if lookback%period != 0 {
		return Policy{}, fmt.Errorf("%s must be an exact multiple of the effective period %s", lookbackPath, period)
	}
	buckets := lookback / period
	if buckets > maxBuckets {
		return Policy{}, fmt.Errorf("%s spans %d buckets; maximum is %d", lookbackPath, buckets, maxBuckets)
	}
	if delay > maxReliableHorizon || lookback > maxReliableHorizon || period > maxReliableHorizon-delay-lookback {
		return Policy{}, fmt.Errorf("%s query horizon (publication_delay + lookback + period) exceeds %s", path, maxReliableHorizon)
	}
	return Policy{Period: period, Lookback: lookback, PublicationDelay: delay}, nil
}

func validatePeriod(path string, value time.Duration) error {
	if err := validateWholeSeconds(path, value, false); err != nil {
		return err
	}
	if value < minPeriod || value > maxPeriod || value%minPeriod != 0 {
		return fmt.Errorf("%s must be between %s and %s and an exact multiple of %s", path, minPeriod, maxPeriod, minPeriod)
	}
	return nil
}

func validateWholeSeconds(path string, value time.Duration, allowZero bool) error {
	if value < 0 || (!allowZero && value == 0) {
		if allowZero {
			return fmt.Errorf("%s must not be negative", path)
		}
		return fmt.Errorf("%s must be positive", path)
	}
	if value%time.Second != 0 {
		return fmt.Errorf("%s must use whole seconds", path)
	}
	return nil
}
