// SPDX-License-Identifier: GPL-3.0-or-later

package sd

import (
	"bytes"
	"context"
	"fmt"
	"testing"
	"time"

	"github.com/netdata/netdata/go/plugins/logger"
	"github.com/netdata/netdata/go/plugins/pkg/safewriter"
	"github.com/netdata/netdata/go/plugins/plugin/agent/discovery/sd/pipeline"
	"github.com/netdata/netdata/go/plugins/plugin/agent/policy"
	"github.com/netdata/netdata/go/plugins/plugin/framework/dyncfg"
	"github.com/netdata/netdata/go/plugins/plugin/framework/functions"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestServiceDiscovery_Run_WaitDecision(t *testing.T) {
	tests := map[string]struct {
		run func(t *testing.T, sd *ServiceDiscovery, confCh chan confFile, stop func())
	}{
		"enable command clears wait and starts pipeline": {
			run: func(t *testing.T, sd *ServiceDiscovery, confCh chan confFile, stop func()) {
				cfg := prepareConfigFile("/etc/netdata/sd.d/job1.conf", "job1")
				confCh <- cfg

				require.Eventually(t, sd.handler.WaitingForDecision, time.Second, 10*time.Millisecond)

				applyTestCommand(t, sd, dyncfg.NewFunction(t.Context(), functions.Function{
					UID:  "enable-job1",
					Args: []string{sd.dyncfgJobID(testDiscovererTypeNetListeners, "job1"), "enable"},
				}))

				require.Eventually(t, func() bool {
					return !sd.handler.WaitingForDecision() &&
						exposedExistsByKey(sd.exposed, testDiscovererTypeNetListeners+":job1") &&
						sd.mgr.IsRunning(pipelineKeyFromSource(cfg.source))
				}, time.Second, 10*time.Millisecond)

				stop()

				entry, ok := sd.exposed.LookupByKey(testDiscovererTypeNetListeners + ":job1")
				require.True(t, ok)
				assert.Equal(t, dyncfg.StatusRunning, entry.Status)
			},
		},
		"second config blocks while wait gate is open and proceeds after decision": {
			run: func(t *testing.T, sd *ServiceDiscovery, confCh chan confFile, stop func()) {
				// Without a wait-decision timeout, the run loop must stay in
				// WaitingForDecision until an explicit enable/disable for cfg1
				// arrives. Sending cfg2 in the meantime must block until the
				// gate clears. This guards against accidental gate-clearing or
				// a regression that lets new configs interleave.
				cfg1 := prepareConfigFile("/etc/netdata/sd.d/job1.conf", "job1")
				cfg2 := prepareConfigFile("/etc/netdata/sd.d/job2.conf", "job2")

				confCh <- cfg1
				require.Eventually(t, sd.handler.WaitingForDecision, time.Second, 10*time.Millisecond)

				secondSent := make(chan struct{})
				go func() {
					confCh <- cfg2
					close(secondSent)
				}()

				// Give the goroutine a chance to either block (expected) or
				// race ahead. We can't use require.Eventually for negative
				// "still blocked" assertions, but a short window is enough to
				// catch a regression that lets the second config flow through
				// while the wait gate is still open.
				select {
				case <-secondSent:
					t.Fatal("second config was processed while wait gate was open")
				case <-time.After(100 * time.Millisecond):
				}
				require.True(t, sd.handler.WaitingForDecision(), "wait gate should still be open before decision")

				// Send the matching enable for cfg1 — this clears the wait gate.
				applyTestCommand(t, sd, dyncfg.NewFunction(t.Context(), functions.Function{
					UID:  "enable-job1",
					Args: []string{sd.dyncfgJobID(testDiscovererTypeNetListeners, "job1"), "enable"},
				}))

				select {
				case <-secondSent:
				case <-time.After(2 * time.Second):
					t.Fatal("second config did not proceed after wait gate cleared")
				}

				require.Eventually(t, func() bool {
					return exposedExistsByKey(sd.exposed, testDiscovererTypeNetListeners+":job1") &&
						exposedExistsByKey(sd.exposed, testDiscovererTypeNetListeners+":job2")
				}, time.Second, 10*time.Millisecond)
			},
		},
	}

	for name, tc := range tests {
		t.Run(name, func(t *testing.T) {
			sd, confCh, cancel, done := newWaitTestServiceDiscovery(t)
			stopped := false
			stop := func() {
				if stopped {
					return
				}
				stopped = true
				stopWaitTestServiceDiscovery(t, sd, cancel, done)
			}
			defer stop()
			tc.run(t, sd, confCh, stop)
		})
	}
}

// A saved DynCfg job can be replayed after its same-named file is registered.
func TestServiceDiscoveryReplayTransfersPendingDecision(t *testing.T) {
	for _, command := range []string{"enable", "disable", "remove"} {
		t.Run(command, func(t *testing.T) {
			d, configs, cancel, done := newWaitTestServiceDiscovery(t)
			defer stopWaitTestServiceDiscovery(t, d, cancel, done)
			configs <- prepareConfigFile("/etc/netdata/sd.d/job.conf", "job")
			require.Eventually(t, d.handler.WaitingForDecision, time.Second, time.Millisecond)
			add := dyncfg.NewFunction(t.Context(), functions.Function{
				UID:    "replayed-add",
				Args:   []string{d.dyncfgTemplateID(testDiscovererTypeNetListeners), "add", "job"},
				Source: "user=test",
				Payload: []byte(
					`{"name":"job","discoverer":{"net_listeners":{}},"services":[{"id":"restored","match":"true"}]}`,
				),
			})
			require.Equal(t, 202, applyTestCommand(t, d, add).Result.Code)
			require.True(t, d.handler.WaitingForDecision(), "ADD stays passive until a decision")
			want := 200
			if command == "enable" {
				want = 202
			}
			require.Equal(t, want, applyTestCommand(t, d, actorFunction(d, t.Context(), "job", command)).Result.Code)
			select {
			case configs <- prepareConfigFile("/etc/netdata/sd.d/next.conf", "next"):
			case <-time.After(time.Second):
				t.Fatal("file discovery still blocked after replay adoption and decision")
			}
		})
	}
}

func newWaitTestServiceDiscovery(t *testing.T) (*ServiceDiscovery, chan confFile, context.CancelFunc, <-chan struct{}) {
	t.Helper()

	var out bytes.Buffer
	confProv := &mockConfigProvider{
		ch: make(chan confFile),
	}

	sd := &ServiceDiscovery{
		epoch:          1,
		attempts:       newTestAttemptAuthority(t),
		Logger:         logger.New(),
		confProv:       confProv,
		pluginName:     testPluginName,
		fnReg:          waitTestFunctionRegistry{},
		discoverers:    testDiscovererRegistry(),
		dyncfgApi:      dyncfg.NewResponder(dyncfg.NewProtocolOutput(safewriter.New(&out))),
		seen:           dyncfg.NewSeenCache[sdConfig](),
		exposed:        dyncfg.NewExposedCache[sdConfig](),
		actorCommands:  make(chan sdActorCommand),
		newPipeline:    newWaitTestPipeline,
		runModePolicy:  policy.RunModePolicy{},
		configDefaults: nil,
	}
	sd.sdCb = &sdCallbacks{
		sd: sd,
	}
	sd.handler = dyncfg.NewHandler(dyncfg.HandlerOpts[sdConfig]{
		API:       sd.dyncfgApi,
		Seen:      sd.seen,
		Exposed:   sd.exposed,
		Callbacks: sd.sdCb,
		WaitKey: func(cfg sdConfig) string {
			return cfg.PipelineKey()
		},

		Path: fmt.Sprintf(dyncfgSDPath, testPluginName),
		ConfigCommands: []dyncfg.Command{
			dyncfg.CommandSchema,
			dyncfg.CommandGet,
			dyncfg.CommandEnable,
			dyncfg.CommandDisable,
			dyncfg.CommandUpdate,
			dyncfg.CommandTest,
			dyncfg.CommandUserconfig,
		},
	})

	sd.mgr = NewPipelineManager(sd.Logger)

	ctx, cancel := context.WithCancel(context.Background())
	sd.ctx = ctx

	done := make(chan struct{})
	go func() {
		defer close(done)
		sd.run(ctx)
	}()

	return sd, confProv.ch, cancel, done
}

type waitTestFunctionRegistry struct{}

func (waitTestFunctionRegistry) RegisterPrefix(string, string, functions.Handler)               {}
func (waitTestFunctionRegistry) RegisterCommandPreparer(string, string, dyncfg.CommandPreparer) {}
func (waitTestFunctionRegistry) UnregisterPrefix(string, string)                                {}

func stopWaitTestServiceDiscovery(t *testing.T, sd *ServiceDiscovery, cancel context.CancelFunc, done <-chan struct{}) {
	t.Helper()

	cancel()

	select {
	case <-done:
	case <-time.After(5 * time.Second):
		t.Fatal("service discovery run loop did not stop")
	}

	sd.mgr.StopAll()
}

func newWaitTestPipeline(cfg pipeline.Config) (sdPipeline, error) {
	return newTestPipeline(cfg.Name), nil
}

func exposedExistsByKey(cache *dyncfg.ExposedCache[sdConfig], key string) bool {
	_, ok := cache.LookupByKey(key)
	return ok
}
