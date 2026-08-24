// SPDX-License-Identifier: GPL-3.0-or-later

package corpus

import (
	"testing"
	"time"
)

func TestC016PhaseBudgetGuards(t *testing.T) {
	const budget = c016ScanPhaseBudget

	for name, tc := range map[string]struct {
		monotonic time.Duration
		wall      time.Duration
		want      bool
	}{
		"inside":             {monotonic: budget - time.Nanosecond, wall: budget - time.Nanosecond, want: true},
		"monotonic equal":    {monotonic: budget, wall: budget - time.Nanosecond, want: false},
		"wall equal":         {monotonic: budget - time.Nanosecond, wall: budget, want: false},
		"negative monotonic": {monotonic: -time.Nanosecond, wall: time.Second, want: false},
		"negative wall":      {monotonic: time.Second, wall: -time.Nanosecond, want: false},
	} {
		t.Run(name, func(t *testing.T) {
			if got := c016DurationsWithinBudget(tc.monotonic, tc.wall, budget); got != tc.want {
				t.Fatalf(
					"c016DurationsWithinBudget(%s, %s, %s) = %v, want %v",
					tc.monotonic, tc.wall, budget, got, tc.want)
			}
		})
	}
	if c016DurationsWithinBudget(0, 0, 0) {
		t.Fatal("phase budget accepted a non-positive budget")
	}

	start := time.Now()
	if !c016PhaseWithinBudget(start, start.Add(budget-time.Nanosecond), budget) {
		t.Fatal("phase budget rejected a timestamp strictly inside both elapsed limits")
	}
	if c016PhaseWithinBudget(start, start.Add(budget), budget) {
		t.Fatal("phase budget accepted its exclusive upper bound")
	}
	if c016PhaseWithinBudget(start, start.Add(-time.Nanosecond), budget) {
		t.Fatal("phase budget accepted a negative elapsed time")
	}
	if c016PhaseWithinBudget(time.Time{}, start, budget) {
		t.Fatal("phase budget accepted a missing launch timestamp")
	}
}
