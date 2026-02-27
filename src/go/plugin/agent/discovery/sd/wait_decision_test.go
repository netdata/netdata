// SPDX-License-Identifier: GPL-3.0-or-later

package sd

import (
	"bytes"
	"context"
	"fmt"
	"testing"
	"time"

	"github.com/netdata/netdata/go/plugins/logger"
	"github.com/netdata/netdata/go/plugins/pkg/netdataapi"
	"github.com/netdata/netdata/go/plugins/pkg/safewriter"
	"github.com/netdata/netdata/go/plugins/plugin/agent/discovery/sd/pipeline"
	"github.com/netdata/netdata/go/plugins/plugin/agent/policy"
	"github.com/netdata/netdata/go/plugins/plugin/framework/confgroup"
	"github.com/netdata/netdata/go/plugins/plugin/framework/dyncfg"
	"github.com/netdata/netdata/go/plugins/plugin/framework/functions"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestServiceDiscovery_Run_WaitDecision(t *testing.T) {
	tests := map[string]struct {
		waitTimeout time.Duration
		run         func(t *testing.T, sd *ServiceDiscovery, confCh chan confFile)
	}{
		"timeout clears wait gate and keeps accepted state": {
			waitTimeout: 40 * time.Millisecond,
			run: func(t *testing.T, sd *ServiceDiscovery, confCh chan confFile) {
				cfg := prepareConfigFile("/etc/netdata/sd.d/job1.conf", "job1")
				confCh <- cfg

				require.Eventually(t, sd.handler.WaitingForDecision, time.Second, 10*time.Millisecond)
				require.Eventually(t, func() bool { return !sd.handler.WaitingForDecision() }, time.Second, 10*time.Millisecond)

				status, ok := exposedStatusByKey(sd.exposed, testDiscovererTypeNetListeners+":job1")
				require.True(t, ok, "expected discovered config to stay exposed after wait timeout")
				assert.Equal(t, dyncfg.StatusAccepted, status)
				assert.False(t, sd.mgr.IsRunning(pipelineKeyFromSource(cfg.source)))
			},
		},
		"enable command before timeout clears wait and starts pipeline": {
			waitTimeout: 750 * time.Millisecond,
			run: func(t *testing.T, sd *ServiceDiscovery, confCh chan confFile) {
				cfg := prepareConfigFile("/etc/netdata/sd.d/job1.conf", "job1")
				confCh <- cfg

				require.Eventually(t, sd.handler.WaitingForDecision, time.Second, 10*time.Millisecond)

				sd.dyncfgCh <- dyncfg.NewFunction(functions.Function{
					UID:  "enable-job1",
					Args: []string{sd.dyncfgJobID(testDiscovererTypeNetListeners, "job1"), "enable"},
				})

				require.Eventually(t, func() bool {
					status, ok := exposedStatusByKey(sd.exposed, testDiscovererTypeNetListeners+":job1")
					if !ok {
						return false
					}
					return !sd.handler.WaitingForDecision() &&
						status == dyncfg.StatusRunning &&
						sd.mgr.IsRunning(pipelineKeyFromSource(cfg.source))
				}, time.Second, 10*time.Millisecond)
			},
		},
		"timeout unblocks and next config is processed": {
			waitTimeout: 40 * time.Millisecond,
			run: func(t *testing.T, sd *ServiceDiscovery, confCh chan confFile) {
				cfg1 := prepareConfigFile("/etc/netdata/sd.d/job1.conf", "job1")
				cfg2 := prepareConfigFile("/etc/netdata/sd.d/job2.conf", "job2")

				confCh <- cfg1
				require.Eventually(t, sd.handler.WaitingForDecision, time.Second, 10*time.Millisecond)

				secondSent := make(chan struct{})
				go func() {
					confCh <- cfg2
					close(secondSent)
				}()

				select {
				case <-secondSent:
					t.Fatalf("second config should block while wait gate is active")
				case <-time.After(20 * time.Millisecond):
				}

				require.Eventually(t, func() bool { return !sd.handler.WaitingForDecision() }, time.Second, 10*time.Millisecond)
				require.Eventually(t, func() bool {
					select {
					case <-secondSent:
						return true
					default:
						return false
					}
				}, time.Second, 10*time.Millisecond)

				require.Eventually(t, func() bool {
					_, ok1 := exposedStatusByKey(sd.exposed, testDiscovererTypeNetListeners+":job1")
					_, ok2 := exposedStatusByKey(sd.exposed, testDiscovererTypeNetListeners+":job2")
					return ok1 && ok2
				}, time.Second, 10*time.Millisecond)

				status1, ok := exposedStatusByKey(sd.exposed, testDiscovererTypeNetListeners+":job1")
				require.True(t, ok)
				assert.Equal(t, dyncfg.StatusAccepted, status1)
				status2, ok := exposedStatusByKey(sd.exposed, testDiscovererTypeNetListeners+":job2")
				require.True(t, ok)
				assert.Equal(t, dyncfg.StatusAccepted, status2)
			},
		},
	}

	for name, tc := range tests {
		t.Run(name, func(t *testing.T) {
			sd, confCh, cancel, done := newWaitTestServiceDiscovery(t, tc.waitTimeout)
			defer stopWaitTestServiceDiscovery(t, sd, cancel, done)
			tc.run(t, sd, confCh)
		})
	}
}

func newWaitTestServiceDiscovery(t *testing.T, waitTimeout time.Duration) (*ServiceDiscovery, chan confFile, context.CancelFunc, <-chan struct{}) {
	t.Helper()

	var out bytes.Buffer
	confProv := &mockConfigProvider{ch: make(chan confFile)}

	sd := &ServiceDiscovery{
		Logger:         logger.New(),
		confProv:       confProv,
		pluginName:     testPluginName,
		fnReg:          functions.NewManager(),
		discoverers:    testDiscovererRegistry(),
		dyncfgApi:      dyncfg.NewResponder(netdataapi.New(safewriter.New(&out))),
		seen:           dyncfg.NewSeenCache[sdConfig](),
		exposed:        dyncfg.NewExposedCache[sdConfig](),
		dyncfgCh:       make(chan dyncfg.Function, 1),
		newPipeline:    newWaitTestPipeline,
		runModePolicy:  policy.RunModePolicy{},
		configDefaults: nil,
	}
	sd.sdCb = &sdCallbacks{sd: sd}
	sd.handler = dyncfg.NewHandler(dyncfg.HandlerOpts[sdConfig]{
		Logger:    sd.Logger,
		API:       sd.dyncfgApi,
		Seen:      sd.seen,
		Exposed:   sd.exposed,
		Callbacks: sd.sdCb,
		WaitKey: func(cfg sdConfig) string {
			return cfg.PipelineKey()
		},
		WaitTimeout: waitTimeout,

		Path:           fmt.Sprintf(dyncfgSDPath, testPluginName),
		EnableFailCode: 422,
		JobCommands: []dyncfg.Command{
			dyncfg.CommandSchema,
			dyncfg.CommandGet,
			dyncfg.CommandEnable,
			dyncfg.CommandDisable,
			dyncfg.CommandUpdate,
			dyncfg.CommandTest,
			dyncfg.CommandUserconfig,
		},
	})

	send := func(context.Context, []*confgroup.Group) {}
	sd.mgr = NewPipelineManager(sd.Logger, sd.newPipeline, send)

	ctx, cancel := context.WithCancel(context.Background())
	sd.ctx = ctx

	done := make(chan struct{})
	go func() {
		defer close(done)
		sd.run(ctx)
	}()

	return sd, confProv.ch, cancel, done
}

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

func exposedStatusByKey(cache *dyncfg.ExposedCache[sdConfig], key string) (dyncfg.Status, bool) {
	var status dyncfg.Status
	var found bool

	cache.ForEach(func(k string, entry *dyncfg.Entry[sdConfig]) bool {
		if k != key {
			return true
		}
		status = entry.Status
		found = true
		return false
	})

	return status, found
}
