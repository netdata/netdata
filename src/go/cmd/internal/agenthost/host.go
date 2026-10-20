// SPDX-License-Identifier: GPL-3.0-or-later

package agenthost

import (
	"context"
	"errors"
	"fmt"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/netdata/netdata/go/plugins/plugin/agent"
	"github.com/netdata/netdata/go/plugins/plugin/framework/collectorapi"
)

// Result distinguishes a joined Agent run from an early process-exit disposition.
// Shared resources may be closed only when Err is nil and ExitRequired is false.
type Result struct {
	// Err reports a host, control or Agent failure. A failed run may retain work.
	Err error
	// ExitRequired means the host returned without observing RunContext complete.
	// The command must exit immediately, without deferred resource cleanup. Use
	// status 1 for Err != nil and status 0 otherwise so the daemon can recover.
	ExitRequired bool
}

// Run hosts one process-lifetime Agent and forwards acknowledged lifecycle
// controls from operating-system signals. The command owns process exit and
// must inspect both result fields before releasing shared resources.
func Run(a *agent.Agent) Result {
	ch := make(chan os.Signal, 1)
	signal.Notify(ch, syscall.SIGHUP, syscall.SIGINT, syscall.SIGTERM)
	defer signal.Stop(ch)
	signal.Ignore(syscall.SIGPIPE)

	return runSignals(a, ch)
}

const (
	hostedRestartTimeout     = 30 * time.Second
	hostedTerminationTimeout = 10 * time.Second
)

type hostedAgent interface {
	RunContext(context.Context) error
	Restart(context.Context) error
	Terminate(context.Context) error
	Info(...any)
	Infof(string, ...any)
	Errorf(string, ...any)
}

func runSignals(a hostedAgent, signals <-chan os.Signal) Result {
	return runSignalsWithTimeout(a, signals, hostedRestartTimeout, hostedTerminationTimeout)
}

func runSignalsWithTimeout(
	a hostedAgent,
	signals <-chan os.Signal,
	restartTimeout time.Duration,
	terminationTimeout time.Duration,
) Result {
	if a == nil || signals == nil || restartTimeout <= 0 || terminationTimeout <= 0 {
		return Result{
			Err: errors.New("agent host: invalid signal loop"),
		}
	}
	collectorapi.ObsoleteCharts(true)
	runDone := make(chan error, 1)
	go func() {
		runDone <- a.RunContext(context.Background())
	}()
	var restartDone <-chan error
	var restartDeadline <-chan struct{}
	var restartCancel context.CancelFunc
	restartPending := false
	startRestart := func(sig os.Signal) {
		a.Infof("received %s signal (%d). Restarting running instance", sig, sig)
		ctx, cancel := context.WithTimeout(context.Background(), restartTimeout)
		done := make(chan error, 1)
		restartDone = done
		restartDeadline = ctx.Done()
		restartCancel = cancel
		go func() {
			done <- a.Restart(ctx)
		}()
	}
	terminate := func(restartErr error) Result {
		if restartCancel != nil {
			restartCancel()
			restartCancel = nil
		}
		collectorapi.ObsoleteCharts(false)
		ctx, cancel := context.WithTimeout(context.Background(), terminationTimeout)
		defer cancel()
		done := make(chan error, 1)
		go func() {
			done <- a.Terminate(ctx)
		}()
		var err error
		select {
		case err = <-done:
		case <-ctx.Done():
			err = context.Cause(ctx)
		}
		if err != nil &&
			!agent.ContainsOnlyProcessControlErrors(err, agent.ErrNotRunning) {
			a.Errorf("terminating the Agent failed: %v", err)
		}
		if agent.ContainsOnlyProcessControlErrors(err, agent.ErrNotRunning) {
			err = nil
		}
		result := waitForRun(runDone, ctx)
		if result.Err != nil {
			a.Errorf("agent shutdown failed: %v", result.Err)
		}
		result.Err = errors.Join(restartErr, err, result.Err)
		return result
	}
	for {
		select {
		case sig := <-signals:
			if sig == syscall.SIGHUP {
				if restartDone != nil {
					restartPending = true
				} else {
					startRestart(sig)
				}
				continue
			}
			a.Infof("received %s signal (%d). Terminating...", sig, sig)
			return terminate(nil)
		case err := <-restartDone:
			restartDone = nil
			restartDeadline = nil
			restartCancel()
			restartCancel = nil
			if err == nil {
				if restartPending {
					restartPending = false
					startRestart(syscall.SIGHUP)
				}
				continue
			}
			if restartRecoveryRequired(err) {
				return recoveryResult(runDone)
			}
			restartErr := restartControlError(err)
			if restartErr != nil {
				a.Errorf("restarting the Agent failed: %v", err)
			}
			return terminate(restartErr)
		case <-restartDeadline:
			if err, ready := completedRunResult(restartDone); ready {
				restartDone = nil
				restartDeadline = nil
				restartCancel()
				restartCancel = nil
				if err == nil {
					if restartPending {
						restartPending = false
						startRestart(syscall.SIGHUP)
					}
					continue
				}
				if !restartRecoveryRequired(err) {
					restartErr := restartControlError(err)
					if restartErr != nil {
						a.Errorf("restarting the Agent failed: %v", err)
					}
					return terminate(restartErr)
				}
			}
			return recoveryResult(runDone)
		case err := <-runDone:
			a.Info("agent run loop stopped. Terminating...")
			collectorapi.ObsoleteCharts(false)
			if err != nil {
				a.Errorf("agent run loop failed: %v", err)
			}
			return Result{
				Err: err,
			}
		}
	}
}

func recoveryResult(done <-chan error) Result {
	collectorapi.ObsoleteCharts(false)
	err, completed := completedRunResult(done)
	return Result{
		Err:          err,
		ExitRequired: !completed,
	}
}

func restartControlError(err error) error {
	if err == nil ||
		agent.ContainsOnlyProcessControlErrors(err, agent.ErrNotRunning) {
		return nil
	}
	return fmt.Errorf("restart Agent: %w", err)
}

func restartRecoveryRequired(err error) bool {
	// Status 0 is reserved for the dispositions the daemon can recover by
	// starting a fresh plugin process.
	return agent.ContainsOnlyProcessControlErrors(
		err,
		context.DeadlineExceeded,
		agent.ErrProcessRestartRequired,
	)
}

func completedRunResult(done <-chan error) (error, bool) {
	select {
	case err := <-done:
		return err, true
	default:
		return nil, false
	}
}

func waitForRun(done <-chan error, ctx context.Context) Result {
	if err, ready := completedRunResult(done); ready {
		return Result{
			Err: err,
		}
	}
	select {
	case err := <-done:
		return Result{
			Err: err,
		}
	case <-ctx.Done():
		if err, ready := completedRunResult(done); ready {
			return Result{
				Err: err,
			}
		}
		return Result{
			Err: errors.Join(
				context.Cause(ctx),
				errors.New("agent shutdown timed out; process exit will contain remaining work"),
			),
			ExitRequired: true,
		}
	}
}
