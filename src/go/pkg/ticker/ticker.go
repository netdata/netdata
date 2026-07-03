// SPDX-License-Identifier: GPL-3.0-or-later

package ticker

import "time"

type (
	// Ticker holds a channel that delivers ticks of a clock at intervals.
	// The ticks are aligned to interval boundaries.
	Ticker struct {
		C        <-chan int
		done     chan struct{}
		loops    int
		interval time.Duration
	}
)

// New returns a new Ticker containing a channel that will send the time with a period specified by the duration argument.
// It adjusts the intervals or drops ticks to make up for slow receivers.
// The duration must be greater than zero; if not, New will panic. Stop the Ticker to release associated resources.
func New(interval time.Duration) *Ticker {
	ticker := &Ticker{
		interval: interval,
		done:     make(chan struct{}, 1),
	}
	ticker.start()
	return ticker
}

func (t *Ticker) start() {
	ch := make(chan int)
	t.C = ch
	go func() {
		for {
			now := time.Now()
			nextRun := now.Truncate(t.interval).Add(t.interval)

			// The wait must be interruptible: Stop releases this goroutine
			// promptly, not after the current interval elapses.
			timer := time.NewTimer(nextRun.Sub(now))
			select {
			case <-t.done:
				timer.Stop()
				close(ch)
				return
			case <-timer.C:
			}
			select {
			case <-t.done:
				close(ch)
				return
			case ch <- t.loops:
				t.loops++
			}
		}
	}()
}

// Stop turns off a Ticker and promptly releases its goroutine. After Stop,
// no more ticks are sent and the channel is closed once the goroutine
// observes the stop; do not read C after Stop.
func (t *Ticker) Stop() {
	t.done <- struct{}{}
}
