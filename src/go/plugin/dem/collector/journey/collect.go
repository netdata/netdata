// SPDX-License-Identifier: GPL-3.0-or-later
package journey

import (
	"context"
	"errors"
)

func (c *Collector) Collect(ctx context.Context) error {
	registration := c.registration
	if registration == nil {
		return errors.New("synthetic job is not active")
	}
	if err := ctx.Err(); err != nil {
		return err
	}
	execution := c.deps.Executor.Execute(ctx, c.request, registration.SetState)
	if !execution.Drained {
		err := errors.New("synthetic execution tree completion is unverified")
		select {
		case c.terminal <- err:
		default:
		}
		return err
	}
	registration.Complete(execution.Run)
	// Execution inability is an observed outcome, not a discarded collection cycle.
	c.attempt.Write(execution.Run.Outcome, execution.Run.DurationMS)
	c.metrics.write(execution.Run)
	return nil
}
