// SPDX-License-Identifier: GPL-3.0-or-later

package nagios

import (
	"math/rand"
	"time"

	"github.com/netdata/netdata/go/plugins/plugin/scripts.d/pkg/timeperiod"
)

func (s *collectState) scheduleRegular(now, intervalBase time.Time, interval, jitter time.Duration) {
	s.nextAnniversary = advanceAnniversary(intervalBase, interval, now)
	s.nextDue = applyJitter(s.nextAnniversary, jitter, s.jitterRand)
}

func (s *collectState) scheduleRetry(now time.Time, interval, jitter time.Duration) {
	s.nextAnniversary = advanceAnniversary(now, interval, now)
	s.nextDue = applyJitter(s.nextAnniversary, jitter, s.jitterRand)
}

func (s *collectState) scheduleNextAllowed(now time.Time, interval, jitter time.Duration, period *timeperiod.Period) {
	next := time.Time{}
	if period != nil {
		next = period.NextAllowed(now)
	}
	if next.IsZero() {
		next = now.Add(intervalOrDefault(interval))
	}
	s.nextAnniversary = next
	s.nextDue = applyJitter(next, jitter, s.jitterRand)
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

func applyJitter(base time.Time, jitter time.Duration, rng *rand.Rand) time.Time {
	if jitter <= 0 || rng == nil {
		return base
	}
	val := rng.Float64()
	if val <= 0 {
		return base
	}
	return base.Add(time.Duration(val * float64(jitter)))
}

func intervalOrDefault(d time.Duration) time.Duration {
	if d <= 0 {
		return time.Minute
	}
	return d
}
