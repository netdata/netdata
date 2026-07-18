// SPDX-License-Identifier: GPL-3.0-or-later

package cloudwatch

import (
	"time"

	"github.com/netdata/netdata/go/plugins/plugin/go.d/collector/cloudwatch/internal/cwquery"
)

const recentlyActiveHorizon = 3 * time.Hour

func queryPolicyUsesRecentlyActive(policy cwquery.Policy) bool {
	return policy.Horizon() <= recentlyActiveHorizon
}

// queryWindow returns the period-aligned rolling window whose newest bucket has
// had publicationDelay time to settle. CloudWatch treats EndTime as exclusive.
func queryWindow(now time.Time, policy cwquery.Policy) (start, end time.Time) {
	periodSeconds := int64(policy.Period / time.Second)
	endSeconds := now.Unix() - int64(policy.PublicationDelay/time.Second)
	endSeconds -= endSeconds % periodSeconds
	end = time.Unix(endSeconds, 0).UTC()
	return end.Add(-policy.Lookback), end
}
