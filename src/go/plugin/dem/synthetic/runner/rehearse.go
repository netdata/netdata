// SPDX-License-Identifier: GPL-3.0-or-later

package runner

import (
	"context"
	"errors"
	"fmt"

	"github.com/netdata/netdata/go/plugins/plugin/dem/synthetic"
	"github.com/netdata/netdata/go/plugins/plugin/dem/synthetic/artifacts"
	"github.com/netdata/netdata/go/plugins/plugin/go.d/pkg/ndexec"
)

// Rehearse executes a draft through the process's ordinary admission and runtime.
// It publishes no monitoring history or captures. Unverified drainage preserves work
// and triggers fail-stop through the shared owner.
func (e *Engine) Rehearse(ctx context.Context, request synthetic.Request) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	captures, err := artifacts.Open("")
	if err != nil {
		return fmt.Errorf("prepare rehearsal workspace: %w", err)
	}
	request.Capture = false
	execution := e.unrecordedView(e.source, captures).Execute(ctx, request, nil)
	if !execution.Drained {
		// The shared fatal owner stops the process; uncertain live work must remain.
		return ndexec.ErrTreeNotDrained
	}
	cleanupErr := captures.Close()
	if err := ctx.Err(); err != nil {
		return errors.Join(err, cleanupErr)
	}
	if execution.Run.Outcome != synthetic.Success || execution.Run.Error != "" {
		err := fmt.Errorf("rehearsal %s: %s", execution.Run.Outcome, execution.Run.Error)
		if execution.Run.Outcome == synthetic.Timeout {
			err = errors.Join(err, context.DeadlineExceeded)
		}
		return errors.Join(err, cleanupErr)
	}
	return cleanupErr
}
