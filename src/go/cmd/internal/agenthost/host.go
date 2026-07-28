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

// Run hosts one process-lifetime Agent and forwards acknowledged lifecycle
// controls from operating-system signals.
func Run(a *agent.Agent) error {
	ch := make(chan os.Signal, 1)
	signal.Notify(ch, syscall.SIGHUP, syscall.SIGINT, syscall.SIGTERM)
	defer signal.Stop(ch)
	signal.Ignore(syscall.SIGPIPE)

	return runSignals(a, ch)
}

const hostedControlTimeout = 10 * time.Second

type hostedAgent interface {
	RunContext(context.Context) error
	Restart(context.Context) error
	Terminate(context.Context) error
	Info(...any)
	Infof(string, ...any)
	Errorf(string, ...any)
}

func runSignals(a hostedAgent, signals <-chan os.Signal) error {
	return runSignalsWithTimeout(a, signals, hostedControlTimeout)
}

func runSignalsWithTimeout(a hostedAgent, signals <-chan os.Signal, timeout time.Duration) error {
	if a == nil || signals == nil || timeout <= 0 {
		return errors.New("agent host: invalid signal loop")
	}
	collectorapi.ObsoleteCharts(true)
	runDone := make(chan error, 1)
	go func() {
		runDone <- a.RunContext(context.Background())
	}()
	var restartDone <-chan error
	var restartCancel context.CancelFunc
	restartPending := false
	startRestart := func(sig os.Signal) {
		a.Infof("received %s signal (%d). Restarting running instance", sig, sig)
		ctx, cancel := context.WithTimeout(context.Background(), timeout)
		done := make(chan error, 2)
		restartDone = done
		restartCancel = cancel
		go func() {
			done <- a.Restart(ctx)
		}()
		go func() {
			<-ctx.Done()
			done <- context.Cause(ctx)
		}()
	}
	terminate := func(restartErr error) error {
		if restartCancel != nil {
			restartCancel()
			restartCancel = nil
		}
		collectorapi.ObsoleteCharts(false)
		ctx, cancel := context.WithTimeout(context.Background(), timeout)
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
		cancel()
		if err != nil && !errors.Is(err, agent.ErrNotRunning) {
			a.Errorf("terminating the Agent failed: %v", err)
		}
		if errors.Is(err, agent.ErrNotRunning) {
			err = nil
		}
		runErr := waitForRun(runDone, timeout)
		if runErr != nil {
			a.Errorf("agent shutdown failed: %v", runErr)
		}
		return errors.Join(restartErr, err, runErr)
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
			restartCancel()
			restartCancel = nil
			if err == nil {
				if restartPending {
					restartPending = false
					startRestart(syscall.SIGHUP)
				}
				continue
			}
			restartErr := restartControlError(err)
			if restartErr != nil {
				a.Errorf("restarting the Agent failed: %v", err)
			}
			return terminate(restartErr)
		case err := <-runDone:
			a.Info("agent run loop stopped. Terminating...")
			collectorapi.ObsoleteCharts(false)
			if err != nil {
				a.Errorf("agent run loop failed: %v", err)
			}
			return err
		}
	}
}

func restartControlError(err error) error {
	if err == nil || errors.Is(err, agent.ErrNotRunning) {
		return nil
	}
	return fmt.Errorf("restart Agent: %w", err)
}

func waitForRun(done <-chan error, timeout time.Duration) error {
	timer := time.NewTimer(timeout)
	defer timer.Stop()
	select {
	case err := <-done:
		return err
	case <-timer.C:
		return errors.New("agent shutdown timed out; process exit will contain remaining work")
	}
}
