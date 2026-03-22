// SPDX-License-Identifier: GPL-3.0-or-later

package nagios

import (
	"time"

	"github.com/netdata/netdata/go/plugins/plugin/scripts.d/pkg/timeperiod"
)

func (s *collectState) scheduleRegular(now, intervalBase time.Time, interval time.Duration) {
	s.nextAnniversary = advanceAnniversary(intervalBase, interval, now)
	s.nextDue = s.nextAnniversary
}

func (s *collectState) scheduleRetry(now time.Time, interval time.Duration) {
	s.nextAnniversary = advanceAnniversary(now, interval, now)
	s.nextDue = s.nextAnniversary
}

func (s *collectState) scheduleNextAllowed(now time.Time, interval time.Duration, period *timeperiod.Period) {
	next := time.Time{}
	if period != nil {
		next = period.NextAllowed(now)
	}
	if next.IsZero() {
		next = now.Add(intervalOrDefault(interval))
	}
	s.nextAnniversary = next
	s.nextDue = next
}

func advanceAnniversary(current time.Time, interval time.Duration, now time.Time) time.Time {
	interval = intervalOrDefault(interval)
	if current.IsZero() {
		current = now
	}
	next := current.Add(interval)
	if !next.After(now) {
		diff := now.Sub(next)
		steps := diff/interval + 1
		next = next.Add(time.Duration(steps) * interval)
	}
	return next
}

func intervalOrDefault(d time.Duration) time.Duration {
	if d <= 0 {
		return time.Minute
	}
	return d
}
