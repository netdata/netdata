// SPDX-License-Identifier: GPL-3.0-or-later

package lifecycle

import (
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

func TestRealClockReusableTimerDoesNotAllocatePerArm(t *testing.T) {
	timer := (RealClock{}).NewTimer(TimerKindDeadline)
	defer timer.Stop()

	allocations := testing.AllocsPerRun(1000, func() {
		timer.Arm(time.Hour)
		timer.Stop()
	})
	require.EqualValues(t, 0, allocations)
}
