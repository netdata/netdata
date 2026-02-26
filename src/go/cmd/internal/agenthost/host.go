// SPDX-License-Identifier: GPL-3.0-or-later

package agenthost

import (
	"context"
	"os"
	"os/signal"
	"sync"
	"syscall"
	"time"

	"github.com/netdata/netdata/go/plugins/plugin/agent"
	"github.com/netdata/netdata/go/plugins/plugin/framework/collectorapi"
)

// Run hosts an agent process lifecycle (signals, restart, quit, metrics-audit timer).
func Run(a *agent.Agent) {
	ch := make(chan os.Signal, 1)
	signal.Notify(ch, syscall.SIGHUP, syscall.SIGINT, syscall.SIGTERM)
	signal.Ignore(syscall.SIGPIPE)

	var keepAliveErr <-chan error
	if !a.IsTerminalMode() {
		ch := make(chan error, 1)
		keepAliveErr = ch
		go func() {
			if err := a.RunKeepAlive(context.Background()); err != nil {
				select {
				case ch <- err:
				default:
				}
			}
		}()
	}

	var wg sync.WaitGroup
	var exit bool
	var finalizeReason string

	var auditTimer *time.Timer
	var auditTimerCh <-chan time.Time
	if mode := a.AuditDuration(); mode > 0 {
		auditTimer = time.NewTimer(mode)
		auditTimerCh = auditTimer.C
		defer auditTimer.Stop()
	}

	for {
		collectorapi.ObsoleteCharts(true)

		ctx, cancel := context.WithCancel(context.Background())
		runDone := make(chan struct{})
		wg.Add(1)
		go func() {
			defer wg.Done()
			defer close(runDone)
			a.RunContext(ctx)
		}()

		select {
		case sig := <-ch:
			switch sig {
			case syscall.SIGHUP:
				a.Infof("received %s signal (%d). Restarting running instance", sig, sig)
			default:
				a.Infof("received %s signal (%d). Terminating...", sig, sig)
				exit = true
				finalizeReason = sig.String()
			}
		case <-a.QuitCh():
			a.Infof("received QUIT command. Terminating...")
			exit = true
			finalizeReason = "quit"
		case <-auditTimerCh:
			a.Infof("metrics-audit duration expired, finalizing metrics audit...")
			exit = true
			finalizeReason = "audit timer expired"
		case <-keepAliveErr:
			a.Info("too many keepAlive errors. Terminating...")
			exit = true
			finalizeReason = "keepalive error"
		case <-runDone:
			a.Info("agent run loop stopped. Terminating...")
			exit = true
			finalizeReason = "run loop stopped"
		}

		if exit {
			collectorapi.ObsoleteCharts(false)
		}

		cancel()

		stopped := func() bool {
			timeout := time.Second * 10
			t := time.NewTimer(timeout)
			defer t.Stop()
			done := make(chan struct{})

			go func() { wg.Wait(); close(done) }()

			select {
			case <-t.C:
				a.Errorf("stopping all goroutines timed out after %s. Exiting...", timeout)
				return false
			case <-done:
				return true
			}
		}()

		if !stopped {
			if exit {
				a.FinalizeMetricsAudit(finalizeReason + ", forced shutdown")
			}
			os.Exit(0)
		}

		if exit {
			a.FinalizeMetricsAudit(finalizeReason)
			os.Exit(0)
		}

		time.Sleep(time.Second)
	}
}
