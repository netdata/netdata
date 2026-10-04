// SPDX-License-Identifier: GPL-3.0-or-later

package receiver

import (
	"context"
)

func (c *Collector) Collect(context.Context) error {
	state := "unavailable"
	if c.hub.Availability().Serving {
		state = "serving"
	}
	c.metrics.state.Enable(state)
	for i, counter := range c.metrics.requests {
		counter.ObserveTotal(float64(c.requests[i].Load()))
	}
	return nil
}
