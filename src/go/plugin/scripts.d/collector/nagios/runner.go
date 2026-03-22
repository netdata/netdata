// SPDX-License-Identifier: GPL-3.0-or-later

package nagios

import (
	"context"
	"time"

	"github.com/netdata/netdata/go/plugins/logger"
	"github.com/netdata/netdata/go/plugins/plugin/go.d/pkg/ndexec"
	outputpkg "github.com/netdata/netdata/go/plugins/plugin/scripts.d/collector/nagios/internal/output"
)

type checkRunner interface {
	Run(ctx context.Context, req checkRunRequest) (checkRunResult, error)
}

type checkRunRequest struct {
	Job        JobConfig
	Vnode      vnodeInfo
	MacroState macroState
	Now        time.Time
	Log        *logger.Logger
}

type checkRunResult struct {
	ExitCode     int
	ServiceState string
	JobState     string
	Parsed       outputpkg.ParsedOutput
	Duration     time.Duration
	Usage        ndexec.ResourceUsage
}

type systemCheckRunner struct{}

func (systemCheckRunner) Run(ctx context.Context, req checkRunRequest) (checkRunResult, error) {
	macros := buildMacroSet(req.Job, req.Vnode, req.MacroState, req.Now)
	args := macros.CommandArgs
	if len(args) == 0 {
		args = req.Job.Args
	}

	opts := ndexec.RunOptions{
		Env: buildRunEnv(req.Job.WorkingDirectory, req.Job.Environment, macros.Env),
		Dir: req.Job.WorkingDirectory,
	}
	timeout := req.Job.Timeout.Duration()

	execCtx := ctx
	cancel := func() {}
	if timeout > 0 {
		execCtx, cancel = context.WithTimeoutCause(ctx, timeout, errNagiosCheckTimeout)
	}
	defer cancel()

	startedAt := time.Now()
	// Temporary branch-specific direct execution path:
	// Nagios checks need job environment values and NAGIOS_* macros to reach the
	// real child process, but that does not currently survive the nd-run helper
	// boundary. The intended follow-up fix is to restore the helper path once
	// nd-run can preserve explicitly forwarded vars while still scrubbing the
	// ambient parent environment.
	output, _, usage, err := ndexec.RunDirectWithOptionsUsageContext(
		execCtx,
		req.Log,
		0,
		opts,
		req.Job.Plugin,
		args...,
	)

	result := checkRunResult{
		ExitCode: exitCodeFromError(err),
		Duration: time.Since(startedAt),
		Usage:    usage,
	}
	result.ServiceState = serviceStateFromExecution(result.ExitCode, err)
	result.JobState = jobStateFromExecution(result.ExitCode, err)
	result.Parsed = outputpkg.Parse(output)
	return result, err
}

type macroState struct {
	ServiceState       string
	ServiceAttempt     int
	ServiceMaxAttempts int
}

type vnodeInfo struct {
	Hostname string
	Labels   map[string]string
}
