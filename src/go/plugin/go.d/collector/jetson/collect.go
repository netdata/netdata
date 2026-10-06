// SPDX-License-Identifier: GPL-3.0-or-later

package jetson

import (
	"context"
	"errors"
	"time"
)

// collect publishes the latest record while it is fresh. Missing readings stay
// gaps; a stale record is never republished as current.
func (c *Collector) collect(ctx context.Context) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	obs := c.latest.Load()
	if obs == nil || time.Since(obs.at) > c.timing.maxSampleAge {
		return errors.New("no fresh tegrastats sample")
	}
	if !obs.sample.hasReadings() {
		return errors.New("tegrastats sample contains no supported GPU or EMC readings")
	}
	c.writeMetrics(obs.sample)
	return nil
}
