// SPDX-License-Identifier: GPL-3.0-or-later

package cloudwatch

import (
	"fmt"
	"time"

	"github.com/netdata/netdata/go/plugins/pkg/confopt"
)

const (
	minQueryPeriod          = time.Minute
	maxQueryPeriod          = 24 * time.Hour
	defaultPublicationDelay = 10 * time.Minute
	maxReliableHorizon      = 14 * 24 * time.Hour
	recentlyActiveHorizon   = 3 * time.Hour
)

// queryPolicy is the fully resolved timing contract for one exported series.
type queryPolicy struct {
	period           time.Duration
	lookback         time.Duration
	publicationDelay time.Duration
}

func (p queryPolicy) bucketCount() int {
	return int(p.lookback / p.period)
}

func (p queryPolicy) horizon() time.Duration {
	return p.publicationDelay + p.lookback + p.period
}

func (p queryPolicy) useRecentlyActive() bool {
	return p.horizon() <= recentlyActiveHorizon
}

func validateQueryPolicyConfig(path string, cfg *QueryPolicyConfig) error {
	if cfg == nil {
		return nil
	}
	if cfg.Period != nil {
		if err := validateQueryPeriod(path+".period", cfg.Period.Duration()); err != nil {
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

func resolveQueryPolicy(path string, rule, defaults *QueryPolicyConfig, profilePeriod int, profileDelay *confopt.LongDuration) (queryPolicy, error) {
	period, periodPath := time.Duration(profilePeriod)*time.Second, "profile period"
	if defaults != nil && defaults.Period != nil {
		period, periodPath = defaults.Period.Duration(), "rule_defaults.query.period"
	}
	if rule != nil && rule.Period != nil {
		period, periodPath = rule.Period.Duration(), path+".query.period"
	}

	lookback, lookbackPath := period, periodPath
	if defaults != nil && defaults.Lookback != nil {
		lookback, lookbackPath = defaults.Lookback.Duration(), "rule_defaults.query.lookback"
	}
	if rule != nil && rule.Lookback != nil {
		lookback, lookbackPath = rule.Lookback.Duration(), path+".query.lookback"
	}

	delay, delayPath := defaultPublicationDelay, "built-in publication delay"
	if profileDelay != nil {
		delay, delayPath = profileDelay.Duration(), "profile publication_delay"
	}
	if defaults != nil && defaults.PublicationDelay != nil {
		delay, delayPath = defaults.PublicationDelay.Duration(), "rule_defaults.query.publication_delay"
	}
	if rule != nil && rule.PublicationDelay != nil {
		delay, delayPath = rule.PublicationDelay.Duration(), path+".query.publication_delay"
	}

	if err := validateQueryPeriod(periodPath, period); err != nil {
		return queryPolicy{}, err
	}
	if err := validateWholeSeconds(lookbackPath, lookback, false); err != nil {
		return queryPolicy{}, err
	}
	if err := validateWholeSeconds(delayPath, delay, true); err != nil {
		return queryPolicy{}, err
	}
	if lookback < period {
		return queryPolicy{}, fmt.Errorf("%s must be at least the effective period %s", lookbackPath, period)
	}
	if lookback%period != 0 {
		return queryPolicy{}, fmt.Errorf("%s must be an exact multiple of the effective period %s", lookbackPath, period)
	}
	buckets := lookback / period
	if buckets > maxQueryBuckets {
		return queryPolicy{}, fmt.Errorf("%s spans %d buckets; maximum is %d", lookbackPath, buckets, maxQueryBuckets)
	}
	if delay > maxReliableHorizon || lookback > maxReliableHorizon || period > maxReliableHorizon-delay-lookback {
		return queryPolicy{}, fmt.Errorf("%s query horizon (publication_delay + lookback + period) exceeds %s", path, maxReliableHorizon)
	}
	return queryPolicy{period: period, lookback: lookback, publicationDelay: delay}, nil
}

func validateQueryPeriod(path string, value time.Duration) error {
	if err := validateWholeSeconds(path, value, false); err != nil {
		return err
	}
	if value < minQueryPeriod || value > maxQueryPeriod || value%minQueryPeriod != 0 {
		return fmt.Errorf("%s must be between %s and %s and an exact multiple of %s", path, minQueryPeriod, maxQueryPeriod, minQueryPeriod)
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

// queryWindow returns the period-aligned rolling window whose newest bucket has
// had publicationDelay time to settle. CloudWatch treats EndTime as exclusive.
func queryWindow(now time.Time, policy queryPolicy) (start, end time.Time) {
	periodSeconds := int64(policy.period / time.Second)
	endSeconds := now.Unix() - int64(policy.publicationDelay/time.Second)
	endSeconds -= endSeconds % periodSeconds
	end = time.Unix(endSeconds, 0).UTC()
	return end.Add(-policy.lookback), end
}
