// SPDX-License-Identifier: GPL-3.0-or-later

package receiver

import (
	"context"
)

func (c *Collector) Collect(context.Context) error {
	c.publishGeoIP()
	state := "unavailable"
	if c.registry.Availability().Serving {
		state = "serving"
	}
	c.metrics.state.Enable(state)
	c.metrics.success.ObserveTotal(float64(c.requests.success.Load()))
	c.metrics.clientErrors.ObserveTotal(float64(c.requests.clientErrors.Load()))
	c.metrics.serverErrors.ObserveTotal(float64(c.requests.serverErrors.Load()))
	return nil
}
