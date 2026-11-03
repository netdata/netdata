// SPDX-License-Identifier: GPL-3.0-or-later
package lighthouse

import (
	"context"
	"errors"

	redact "github.com/netdata/netdata/go/plugins/plugin/dem/internal/redact"
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
		Kind:           c.request.Kind,
		Name:           c.request.Name,
		CadenceSeconds: c.UpdateEvery,
		TimeoutSeconds: c.request.Timeout.Seconds(),
		Target:         redact.NewRedactor().ApplyURL(c.request.URL),
	})
	if err != nil {
		return err
	}
	c.registration = registration
	defer registration.Retire()
	ready()
	select {
	case <-ctx.Done():
		return nil
	case err := <-c.terminal:
		return err
	}
}
