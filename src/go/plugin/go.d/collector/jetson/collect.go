// SPDX-License-Identifier: GPL-3.0-or-later

package jetson

import (
	"context"
	"errors"
	"time"
)

func (c *Collector) collect(ctx context.Context) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	latest := c.latest.Load()
	if latest == nil || time.Since(latest.at) > c.timing.freshFor {
		return errors.New("no fresh tegrastats sample")
	}
	if !latest.sample.hasMetrics() {
		return errors.New("tegrastats sample contains no supported GPU or EMC readings")
	}
	c.writeMetrics(latest.sample)
	return nil
}
