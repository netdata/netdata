// SPDX-License-Identifier: GPL-3.0-or-later

//go:build linux

package jetson

import (
	"context"
	"errors"
)

// collect publishes the latest record while it is fresh. Missing readings stay
// gaps; a stale record is never republished as current.
func (c *Collector) collect(ctx context.Context) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	s, ok := c.latestSample()
	if !ok {
		return errors.New("no fresh tegrastats sample")
	}
	if !s.hasReadings() {
		return errors.New("tegrastats sample contains no supported GPU or EMC readings")
	}
	c.writeMetrics(s)
	return nil
}

func (c *Collector) latestSample() (sample, bool) {
	if c.source == nil {
		return sample{}, false
	}
	return c.source.Latest()
}
