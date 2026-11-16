// SPDX-License-Identifier: GPL-3.0-or-later

package listen

import (
	"context"
)

// collect detaches one coherent batch, publishes newly activated profiles in
// the template set captured with that batch, then writes the batch and the
// receiver diagnostics. A failure after the cut loses the interval; there is no
// replay.
func (c *Collector) collect(ctx context.Context) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	if c.receiver == nil {
		return errNotInitialized
	}
	cut, err := c.receiver.cut(c.now(), c.published)
	if err != nil {
		return err
	}
	defer c.receiver.release(cut.batch)
	if cut.membership != nil {
		set, err := c.templateSet(cut.membership)
		if err != nil {
			return err
		}
		c.templates, c.published = set, cut.activated
	}
	for _, m := range cut.batch {
		if err := ctx.Err(); err != nil {
			return err
		}
		c.writeMeasurement(m)
	}
	c.diagnostics.write(cut.stats)
	return ctx.Err()
}
