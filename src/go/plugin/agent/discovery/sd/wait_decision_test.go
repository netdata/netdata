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
		run func(t *testing.T, sd *ServiceDiscovery, confCh chan confFile, stop func())
	}{
		"enable command clears wait and starts pipeline": {
			run: func(t *testing.T, sd *ServiceDiscovery, confCh chan confFile, stop func()) {
				cfg := prepareConfigFile("/etc/netdata/sd.d/job1.conf", "job1")
				confCh <- cfg

				require.Eventually(t, sd.handler.WaitingForDecision, time.Second, 10*time.Millisecond)

				sd.dyncfgCh <- dyncfg.NewFunction(functions.Function{
					UID:  "enable-job1",
					Args: []string{sd.dyncfgJobID(testDiscovererTypeNetListeners, "job1"), "enable"},
				})

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

func newWaitTestServiceDiscovery(t *testing.T) (*ServiceDiscovery, chan confFile, context.CancelFunc, <-chan struct{}) {
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

func exposedExistsByKey(cache *dyncfg.ExposedCache[sdConfig], key string) bool {
	_, ok := cache.LookupByKey(key)
	return ok
}
