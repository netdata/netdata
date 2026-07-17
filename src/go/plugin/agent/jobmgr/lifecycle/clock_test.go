// SPDX-License-Identifier: GPL-3.0-or-later

package lifecycle

import (
	"testing"
	"time"
)

func TestRealClockReusableTimerDoesNotAllocatePerArm(t *testing.T) {
	timer := (RealClock{}).NewTimer(TimerKindDeadline)
	defer timer.Stop()
	if allocations := testing.AllocsPerRun(1000, func() {
		timer.Arm(time.Hour)
		timer.Stop()
	}); allocations != 0 {
		t.Fatalf("reusable deadline timer allocations per arm=%f", allocations)
	}
}
