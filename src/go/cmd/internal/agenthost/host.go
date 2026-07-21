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

	collectorapi.ObsoleteCharts(true)
	runDone := make(chan error, 1)
	go func() {
		runDone <- a.RunContext(context.Background())
	}()
	for {
		select {
		case sig := <-ch:
			var restartErr error
			if sig == syscall.SIGHUP {
				a.Infof("received %s signal (%d). Restarting running instance", sig, sig)
				ctx, cancel := context.WithTimeout(
					context.Background(),
					10*time.Second,
				)
				err := a.Restart(ctx)
				cancel()
				if err == nil {
					continue
				}
				restartErr = restartControlError(err)
				if restartErr != nil {
					a.Errorf("restarting the Agent failed: %v", err)
				}
			} else {
				a.Infof("received %s signal (%d). Terminating...", sig, sig)
			}
			collectorapi.ObsoleteCharts(false)
			ctx, cancel := context.WithTimeout(
				context.Background(),
				10*time.Second,
			)
			err := a.Terminate(ctx)
			cancel()
			if err != nil && !errors.Is(err, agent.ErrNotRunning) {
				a.Errorf("terminating the Agent failed: %v", err)
			}
			if errors.Is(err, agent.ErrNotRunning) {
				err = nil
			}
			runErr := waitForRun(runDone, 10*time.Second)
			if runErr != nil {
				a.Errorf("agent shutdown failed: %v", runErr)
			}
			return errors.Join(restartErr, err, runErr)
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
