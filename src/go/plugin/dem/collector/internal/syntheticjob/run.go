// SPDX-License-Identifier: GPL-3.0-or-later

package syntheticjob

import (
	"context"
	"time"

	"github.com/netdata/netdata/go/plugins/plugin/dem/internal/redact"
	"github.com/netdata/netdata/go/plugins/plugin/dem/synthetic/monitor"
)

// Run registers the job's monitor session, runs it and retires the registration after the session joins its
// executor.
func (j *Job) Run(ctx context.Context, ready func()) error {
	if !j.initialized {
		return errNotInitialized
	}
	if err := ctx.Err(); err != nil {
		return err
	}
	session, err := monitor.New(j.request, time.Duration(j.checkEvery)*time.Second, j.deps.Executor)
	if err != nil {
		return err
	}
	// Only URL-targeted kinds have a target; Functions show it without credentials.
	retire, err := j.deps.Registry.Register(session, redact.NewRedactor().ApplyURL(j.request.URL))
	if err != nil {
		return err
	}
	defer retire()
	j.session = session
	ready()
	return session.Run(ctx)
}
