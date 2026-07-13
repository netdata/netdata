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
	MaxBuckets              = 1440
	maxReliableHorizon      = 14 * 24 * time.Hour
)

// Config defines optional CloudWatch query defaults. Callers compose profile,
// metric, job-default, and rule values field by field through Resolve.
type Config struct {
	Period           *confopt.LongDuration `yaml:"period,omitempty" json:"period,omitempty"`
	Lookback         *confopt.LongDuration `yaml:"lookback,omitempty" json:"lookback,omitempty"`
	PublicationDelay *confopt.LongDuration `yaml:"publication_delay,omitempty" json:"publication_delay,omitempty"`
}

// Source associates query values with their user-facing configuration path.
type Source struct {
	Config *Config
	Path   string
}

// Resolution defines the low-to-high query precedence sources for one series.
type Resolution struct {
	Path         string
	Profile      Source
	Metric       Source
	RuleDefaults Source
	Rule         Source
}

// Policy is the fully resolved timing contract for one exported series.
type Policy struct {
	Period           time.Duration
	Lookback         time.Duration
	PublicationDelay time.Duration
}

// BucketCount returns the number of period buckets in the lookback window.
func (p Policy) BucketCount() int {
	return int(p.Lookback / p.Period)
}

// Horizon returns the full age range needed for discovery freshness decisions.
func (p Policy) Horizon() time.Duration {
	return p.PublicationDelay + p.Lookback + p.Period
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
func Resolve(input Resolution) (Policy, error) {
	if input.Profile.Config == nil || input.Profile.Config.Period == nil {
		return Policy{}, fmt.Errorf("%s.period is required", sourcePath(input.Profile, "profile query"))
	}
	sources := []Source{input.Profile, input.Metric, input.RuleDefaults, input.Rule}
	period, periodPath, _ := resolveDuration(sources, "period", func(cfg *Config) *confopt.LongDuration { return cfg.Period })

	lookback, lookbackPath := period, periodPath
	if value, path, ok := resolveDuration(sources, "lookback", func(cfg *Config) *confopt.LongDuration { return cfg.Lookback }); ok {
		lookback, lookbackPath = value, path
	}

	delay, delayPath := DefaultPublicationDelay, "built-in publication delay"
	if value, path, ok := resolveDuration(sources, "publication_delay", func(cfg *Config) *confopt.LongDuration { return cfg.PublicationDelay }); ok {
		delay, delayPath = value, path
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
	if buckets > MaxBuckets {
		return Policy{}, fmt.Errorf("%s spans %d buckets; maximum is %d", lookbackPath, buckets, MaxBuckets)
	}
	if delay > maxReliableHorizon || lookback > maxReliableHorizon || period > maxReliableHorizon-delay-lookback {
		return Policy{}, fmt.Errorf("%s query horizon (publication_delay + lookback + period) exceeds %s", input.Path, maxReliableHorizon)
	}
	return Policy{Period: period, Lookback: lookback, PublicationDelay: delay}, nil
}

func resolveDuration(sources []Source, fieldName string, field func(*Config) *confopt.LongDuration) (time.Duration, string, bool) {
	var (
		value time.Duration
		path  string
		ok    bool
	)
	for _, source := range sources {
		if source.Config == nil {
			continue
		}
		if raw := field(source.Config); raw != nil {
			value = raw.Duration()
			path = sourcePath(source, "query") + "." + fieldName
			ok = true
		}
	}
	return value, path, ok
}

func sourcePath(source Source, fallback string) string {
	if source.Path != "" {
		return source.Path
	}
	return fallback
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
