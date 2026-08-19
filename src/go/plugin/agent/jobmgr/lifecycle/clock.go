// SPDX-License-Identifier: GPL-3.0-or-later

package lifecycle

import "time"

const (
	TimerKindDeadline = "deadline"
	TimerKindShutdown = "shutdown"
)

type RealClock struct{}

func (RealClock) Now() time.Time {
	return time.Now()
}

func (RealClock) Arm(_ string, delay time.Duration) (<-chan time.Time, func()) {
	timer := newRealClockTimer()
	return timer.Arm(delay), timer.Stop
}

func (RealClock) NewTimer(string) ReusableTimer { return newRealClockTimer() }

type realClockTimer struct {
	timer *time.Timer
}

func newRealClockTimer() *realClockTimer {
	timer := time.NewTimer(time.Hour)
	if !timer.Stop() {
		<-timer.C
	}
	return &realClockTimer{
		timer: timer,
	}
}

func (rct *realClockTimer) Arm(delay time.Duration) <-chan time.Time {
	rct.Stop()
	rct.timer.Reset(delay)
	return rct.timer.C
}

func (rct *realClockTimer) Stop() {
	if rct == nil || rct.timer == nil {
		return
	}
	if !rct.timer.Stop() {
		select {
		case <-rct.timer.C:
		default:
		}
	}
}
