// SPDX-License-Identifier: GPL-3.0-or-later

package agenthost

import (
	"context"
	"errors"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/netdata/netdata/go/plugins/plugin/agent"
	"github.com/netdata/netdata/go/plugins/plugin/framework/collectorapi"
)

// Run hosts one process-lifetime Agent and forwards acknowledged lifecycle
// controls from operating-system signals.
func Run(a *agent.Agent) {
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
				a.Errorf("restarting the Agent failed: %v", err)
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
			waitForRun(a, runDone)
			return
		case err := <-runDone:
			a.Info("agent run loop stopped. Terminating...")
			collectorapi.ObsoleteCharts(false)
			if err != nil {
				a.Errorf("agent run loop failed: %v", err)
			}
			return
		}
	}
}

func waitForRun(a *agent.Agent, done <-chan error) {
	timer := time.NewTimer(10 * time.Second)
	defer timer.Stop()
	select {
	case err := <-done:
		if err != nil {
			a.Errorf("agent shutdown failed: %v", err)
		}
	case <-timer.C:
		a.Error("agent shutdown timed out; process exit will contain remaining work")
	}
}
