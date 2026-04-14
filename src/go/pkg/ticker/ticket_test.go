// SPDX-License-Identifier: GPL-3.0-or-later

package ticker

import (
	"testing"
	"time"
)

// TODO: often fails Circle CI (~200-240)
var allowedDelta = 500 * time.Millisecond

func TestTickerParallel(t *testing.T) {
	for i := range 100 {
		go func() {
			time.Sleep(time.Second / 100 * time.Duration(i))
			TestTicker(t)
		}()
	}
	time.Sleep(4 * time.Second)
}

func TestTicker(t *testing.T) {
	tk := New(time.Second)
	defer tk.Stop()
	prev := time.Now()
	for i := range 3 {
		<-tk.C
		now := time.Now()
		diff := abs(now.Round(time.Second).Sub(now))
		if diff >= allowedDelta {
			t.Errorf("Ticker is not aligned: expect delta < %v but was: %v (%s)", allowedDelta, diff, now.Format(time.RFC3339Nano))
		}
		if i > 0 {
			dt := now.Sub(prev)
			if abs(dt-time.Second) >= allowedDelta {
				t.Errorf("Ticker interval: expect delta < %v ns but was: %v", allowedDelta, abs(dt-time.Second))
			}
		}
		prev = now
	}
}

func abs(a time.Duration) time.Duration {
	if a < 0 {
		return -a
	}
	return a
}
