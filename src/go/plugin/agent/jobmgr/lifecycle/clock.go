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
	return &realClockTimer{timer: timer}
}

func (timer *realClockTimer) Arm(delay time.Duration) <-chan time.Time {
	timer.Stop()
	timer.timer.Reset(delay)
	return timer.timer.C
}

func (timer *realClockTimer) Stop() {
	if timer == nil || timer.timer == nil {
		return
	}
	if !timer.timer.Stop() {
		select {
		case <-timer.timer.C:
		default:
		}
	}
}
