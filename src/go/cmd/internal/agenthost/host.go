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

// Run hosts an agent process lifecycle (signals, restart, quit, dump timer).
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

	var dumpTimer *time.Timer
	var dumpTimerCh <-chan time.Time
	if mode := a.DumpModeDuration(); mode > 0 {
		dumpTimer = time.NewTimer(mode)
		dumpTimerCh = dumpTimer.C
		defer dumpTimer.Stop()
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
			}
		case <-a.QuitCh():
			a.Infof("received QUIT command. Terminating...")
			exit = true
		case <-dumpTimerCh:
			a.Infof("dump mode duration expired, collecting analysis...")
			a.TriggerDumpAnalysis()
			exit = true
		case <-keepAliveErr:
			a.Info("too many keepAlive errors. Terminating...")
			exit = true
		case <-runDone:
			a.Info("agent run loop stopped. Terminating...")
			exit = true
		}

		if exit {
			collectorapi.ObsoleteCharts(false)
		}

		cancel()

		func() {
			timeout := time.Second * 10
			t := time.NewTimer(timeout)
			defer t.Stop()
			done := make(chan struct{})

			go func() { wg.Wait(); close(done) }()

			select {
			case <-t.C:
				a.Errorf("stopping all goroutines timed out after %s. Exiting...", timeout)
				os.Exit(0)
			case <-done:
			}
		}()

		if exit {
			os.Exit(0)
		}

		time.Sleep(time.Second)
	}
}
