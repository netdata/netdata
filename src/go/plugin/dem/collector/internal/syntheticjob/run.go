// SPDX-License-Identifier: GPL-3.0-or-later

package syntheticjob

import (
	"context"
	"time"

	"github.com/netdata/netdata/go/plugins/plugin/dem/internal/redact"
	"github.com/netdata/netdata/go/plugins/plugin/dem/synthetic/monitor"
	"github.com/netdata/netdata/go/plugins/plugin/dem/synthetic/registry"
)

// Run registers the active job, runs its monitor session and retires the registration after the session joins its
// executor.
func (j *Job) Run(ctx context.Context, ready func()) error {
	if !j.initialized {
		return errNotInitialized
	}
	if err := ctx.Err(); err != nil {
		return err
	}
	registration, err := j.deps.Registry.Register(registry.Job{
		JobID:          j.request.JobID(),
		VnodeGUID:      j.request.VnodeGUID,
		Kind:           j.request.Kind,
		Name:           j.request.Name,
		CadenceSeconds: j.checkEvery,
		TimeoutSeconds: j.request.Timeout.Seconds(),
		// Only URL-targeted kinds have a target; Functions show it without credentials.
		Target: redact.NewRedactor().ApplyURL(j.request.URL),
	})
	if err != nil {
		return err
	}
	session, err := monitor.New(j.request, time.Duration(j.checkEvery)*time.Second, j.deps.Executor, registration)
	if err != nil {
		registration.Retire()
		return err
	}
	j.session = session
	defer registration.Retire()
	ready()
	return session.Run(ctx)
}
