// SPDX-License-Identifier: GPL-3.0-or-later

//go:build linux

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
		healthy, err := c.follow(ctx, proc)
		// A source that kept producing records for a stall period restarts without
		// accumulated backoff; one that fails sooner keeps backing off.
		if healthy >= c.timing.stallTimeout {
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

// follow publishes the records of proc until it exits, stalls or ctx is canceled,
// then withdraws the latest record and closes proc. It reports how long records
// kept arriving: the time from the start of following to the last record.
func (c *Collector) follow(ctx context.Context, proc *tegrastatsProcess) (healthy time.Duration, err error) {
	defer func() {
		c.latest.Store(nil)
		proc.close()
	}()

	// The end of output is not a failure of its own: the process exit reports the
	// status, and a process that keeps running without output is a stall.
	began := time.Now()
	stall := time.NewTimer(c.timing.stallTimeout)
	defer stall.Stop()
	for {
		select {
		case <-ctx.Done():
			return healthy, ctx.Err()
		case <-proc.exited:
			return healthy, proc.exitError()
		case obs := <-proc.observations:
			healthy = obs.at.Sub(began)
			c.latest.Store(&obs)
			stall.Reset(c.timing.stallTimeout)
		case <-stall.C:
			return healthy, errors.New("tegrastats stopped producing records")
		}
	}
}
