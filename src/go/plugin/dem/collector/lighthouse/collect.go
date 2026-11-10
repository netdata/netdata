// SPDX-License-Identifier: GPL-3.0-or-later
package lighthouse

import (
	"context"
	"errors"
	"time"
)

func (c *Collector) Collect(ctx context.Context) error {
	if c.session == nil {
		return errors.New("synthetic job is not active")
	}
	if err := ctx.Err(); err != nil {
		return err
	}
	snapshot := c.session.Snapshot(time.Now())
	c.attempt.Write(snapshot)
	if snapshot.Fresh {
		c.metrics.write(snapshot.Latest)
	}
	return nil
}
