// SPDX-License-Identifier: GPL-3.0-or-later
package lighthouse

import (
	"context"
	"errors"
	"time"

	model "github.com/netdata/netdata/go/plugins/plugin/dem/synthetic"
)

func (c *Collector) Init(ctx context.Context) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	if c.deps.Executor == nil || c.deps.Hub == nil {
		return errors.New("missing synthetic executor or observation hub")
	}
	if c.UpdateEvery <= 0 {
		return errors.New("update_every must be positive")
	}
	request := model.Request{
		Kind:    model.Lighthouse,
		Name:    c.Name,
		URL:     c.URL,
		Timeout: time.Duration(c.Timeout),
		Capture: c.SaveReport,
	}
	if err := model.ValidateRequest(request); err != nil {
		return err
	}
	c.request = request
	c.initialized = true
	return nil
}
func (c *Collector) Check(ctx context.Context) error {
	if !c.initialized {
		return errors.New("collector is not initialized")
	}
	// Preparation has a fixed budget independent of the public attempt timeout.
	ctx, cancel := context.WithTimeout(ctx, 15*time.Second)
	defer cancel()
	return c.deps.Executor.Check(ctx, c.request.Kind)
}
func (c *Collector) Cleanup(context.Context) {}
