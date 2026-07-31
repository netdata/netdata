// SPDX-License-Identifier: GPL-3.0-or-later

package corpus

import (
	"testing"
	"time"
)

func TestC016PhaseBudgetGuards(t *testing.T) {
	const budget = c016ScanPhaseBudget

	for _, tc := range []struct {
		name      string
		monotonic time.Duration
		wall      time.Duration
		want      bool
	}{
		{name: "inside", monotonic: budget - time.Nanosecond, wall: budget - time.Nanosecond, want: true},
		{name: "monotonic equal", monotonic: budget, wall: budget - time.Nanosecond},
		{name: "wall equal", monotonic: budget - time.Nanosecond, wall: budget},
		{name: "negative monotonic", monotonic: -time.Nanosecond, wall: time.Second},
		{name: "negative wall", monotonic: time.Second, wall: -time.Nanosecond},
	} {
		t.Run(tc.name, func(t *testing.T) {
			if got := c016DurationsWithinBudget(tc.monotonic, tc.wall, budget); got != tc.want {
				t.Fatalf(
					"c016DurationsWithinBudget(%s, %s, %s) = %v, want %v",
					tc.monotonic, tc.wall, budget, got, tc.want)
			}
		})
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
