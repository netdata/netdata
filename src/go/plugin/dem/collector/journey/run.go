// SPDX-License-Identifier: GPL-3.0-or-later
package journey

import (
	"context"
	"errors"
	"time"

	"github.com/netdata/netdata/go/plugins/plugin/dem/synthetic/monitor"

	syntheticregistry "github.com/netdata/netdata/go/plugins/plugin/dem/synthetic/registry"
)

func (c *Collector) Run(ctx context.Context, ready func()) error {
	if !c.initialized {
		return errors.New("collector is not initialized")
	}
	if err := ctx.Err(); err != nil {
		return err
	}
	registration, err := c.deps.Registry.Register(syntheticregistry.Job{
		JobID:          c.request.JobID(),
		VnodeGUID:      c.request.VnodeGUID,
		Kind:           c.request.Kind,
		Name:           c.request.Name,
		CadenceSeconds: c.CheckEvery,
		TimeoutSeconds: c.request.Timeout.Seconds(),
	})
	if err != nil {
		return err
	}
	session, err := monitor.New(c.request, time.Duration(c.CheckEvery)*time.Second, c.deps.Executor, registration)
	if err != nil {
		registration.Retire()
		return err
	}
	c.session = session
	defer registration.Retire()
	ready()
	return session.Run(ctx)
}
