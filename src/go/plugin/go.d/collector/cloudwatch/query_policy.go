// SPDX-License-Identifier: GPL-3.0-or-later

package cloudwatch

import (
	"time"

	"github.com/netdata/netdata/go/plugins/plugin/go.d/collector/cloudwatch/internal/cwquery"
)

const (
	defaultPublicationDelay = cwquery.DefaultPublicationDelay
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

func resolveQueryPolicy(path string, rule, defaults, metric *cwquery.Config, profile cwquery.Config) (queryPolicy, error) {
	resolved, err := cwquery.Resolve(path, rule, defaults, metric, profile)
	if err != nil {
		return queryPolicy{}, err
	}
	return queryPolicy{
		period: resolved.Period, lookback: resolved.Lookback, publicationDelay: resolved.PublicationDelay,
	}, nil
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
