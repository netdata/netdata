// SPDX-License-Identifier: GPL-3.0-or-later

package runner

import (
	"context"
	"errors"
	"fmt"
	"strings"

	"github.com/netdata/netdata/go/plugins/plugin/dem/synthetic"
	"github.com/netdata/netdata/go/plugins/plugin/go.d/pkg/ndexec"
)

// Rehearse executes a draft through the process's ordinary admission and runtime.
// It publishes no monitoring history or captures. Unverified drainage preserves work
// and triggers fail-stop through the shared owner.
func (e *Engine) Rehearse(ctx context.Context, request synthetic.Request) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	view, finish, err := e.temporaryView(e.source)
	if err != nil {
		return fmt.Errorf("prepare rehearsal workspace: %w", err)
	}
	request.Capture = false
	request.Trace = false
	return finish(rehearsalResult(ctx, view.execute(ctx, request)))
}

// rehearsalResult reports an undrained, cancelled or unsuccessful rehearsal.
func rehearsalResult(ctx context.Context, execution synthetic.Execution) error {
	if !execution.Drained {
		// The shared fatal owner stops the process; uncertain live work must remain.
		return ndexec.ErrTreeNotDrained
	}
	if err := ctx.Err(); err != nil {
		return err
	}
	run := execution.Run
	if run.Outcome == synthetic.Success && run.Error == "" {
		return nil
	}
	outcome := string(run.Outcome)
	if run.TerminationReason != "" {
		outcome += " (" + string(run.TerminationReason) + ")"
	}
	err := fmt.Errorf("rehearsal %s: %s", outcome, rehearsalFailureText(run))
	if run.Outcome == synthetic.Timeout {
		err = errors.Join(err, context.DeadlineExceeded)
	}
	return err
}

// rehearsalFailureText preserves the core error and adds the same diagnostic
// summaries shown in run history without assigning them outcome authority.
func rehearsalFailureText(run synthetic.Run) string {
	var lines []string
	// The contextual first-failure summary already carries an identical core error.
	if e := run.FailureEvidence; run.Error != "" &&
		(e == nil || e.FirstFailure == nil || e.FirstFailure.Message != run.Error) {
		lines = append(lines, run.Error)
	}
	for _, d := range run.Diagnostics() {
		lines = append(lines, d.Label+": "+d.Text)
	}
	return strings.Join(lines, "\n")
}
