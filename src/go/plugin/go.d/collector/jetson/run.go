// SPDX-License-Identifier: GPL-3.0-or-later

package jetson

import (
	"context"
	"errors"
	"time"
)

// sourceTiming holds the tegrastats supervision intervals; tests shorten them.
type sourceTiming struct {
	// maxSampleAge is how long a record stays publishable.
	maxSampleAge time.Duration
	// stallTimeout restarts tegrastats when no record arrives for this long.
	stallTimeout time.Duration
	// restartDelayMin and restartDelayMax bound the exponential restart backoff.
	restartDelayMin time.Duration
	restartDelayMax time.Duration
}

func (c *Collector) run(ctx context.Context, ready func()) error {
	if c.tegrastatsPath == "" {
		return errors.New("tegrastats executable has not been resolved")
	}

	// A startup failure is returned to the managed runtime and its retry policy.
	proc, err := startTegrastats(ctx, c.tegrastatsPath)
	if err != nil {
		if ctx.Err() != nil {
			return nil
		}
		return err
	}
	ready()

	// After readiness, returning would permanently fail the job, so source
	// failures are recovered here.
	delay := c.timing.restartDelayMin
	for {
		began := time.Now()
		observed, err := c.follow(ctx, proc)
		// A source that stayed healthy for a while restarts without accumulated backoff.
		if observed && time.Since(began) >= c.timing.stallTimeout {
			delay = c.timing.restartDelayMin
		}
		for {
			if ctx.Err() != nil {
				return nil
			}
			c.Limit("jetson:tegrastats", 1, time.Minute).Warningf("tegrastats source unavailable: %v", err)
			select {
			case <-ctx.Done():
				return nil
			case <-time.After(delay):
			}
			delay = min(delay*2, c.timing.restartDelayMax)
			if proc, err = startTegrastats(ctx, c.tegrastatsPath); err == nil {
				break
			}
		}
	}
}

// follow publishes the records of proc until it fails or ctx is canceled, then
// withdraws the latest record and closes proc. It reports whether any record arrived.
func (c *Collector) follow(ctx context.Context, proc *tegrastatsProcess) (observed bool, err error) {
	defer func() {
		c.latest.Store(nil)
		proc.close()
	}()

	stall := time.NewTimer(c.timing.stallTimeout)
	defer stall.Stop()
	for {
		select {
		case <-ctx.Done():
			return observed, ctx.Err()
		case <-proc.exited:
			return observed, proc.exitError()
		case <-proc.readDone:
			return observed, proc.readError()
		case obs := <-proc.records:
			observed = true
			c.latest.Store(&obs)
			stall.Reset(c.timing.stallTimeout)
		case <-stall.C:
			return observed, errors.New("tegrastats stopped producing recognizable samples")
		}
	}
}
