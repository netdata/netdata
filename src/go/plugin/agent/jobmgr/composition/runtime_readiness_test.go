// SPDX-License-Identifier: GPL-3.0-or-later

package composition

import (
	"context"
	"errors"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/netdata/netdata/go/plugins/pkg/funcapi"
	"github.com/netdata/netdata/go/plugins/pkg/metrix"
	"github.com/netdata/netdata/go/plugins/plugin/agent/jobmgr/lifecycle"
	"github.com/netdata/netdata/go/plugins/plugin/framework/collectorapi"
	"github.com/netdata/netdata/go/plugins/plugin/framework/confgroup"
	"github.com/netdata/netdata/go/plugins/plugin/framework/dyncfg"
	"github.com/stretchr/testify/require"
)

type readinessIntegrationCollector struct {
	collectorapi.Base
	store    metrix.CollectorStore
	terminal string
}

func (*readinessIntegrationCollector) Init(context.Context) error           { return nil }
func (*readinessIntegrationCollector) Check(context.Context) error          { return nil }
func (*readinessIntegrationCollector) Collect(context.Context) error        { return nil }
func (*readinessIntegrationCollector) Cleanup(context.Context)              {}
func (*readinessIntegrationCollector) Configuration() any                   { return &collectorapi.MockConfiguration{} }
func (c *readinessIntegrationCollector) MetricStore() metrix.CollectorStore { return c.store }
func (*readinessIntegrationCollector) ChartTemplateYAML() string {
	return `version: v1
groups:
  - family: readiness
    metrics: [value]
    charts:
      - context: readiness.value
        title: Value
        units: value
        dimensions:
          - selector: value
            name: value
`
}
func (c *readinessIntegrationCollector) Run(ctx context.Context, ready func()) error {
	if c.terminal == "startup" {
		return errors.New("listen 127.0.0.1:8125: bind failed")
	}
	ready()
	switch c.terminal {
	case "error":
		return errors.New("required reader failed")
	case "panic":
		panic("required reader panic")
	default:
		return nil
	}
}

func TestRunGenerationCollectorFailureDoesNotDirtyManager(t *testing.T) {
	for _, terminal := range []string{"startup", "error", "panic", "nil"} {
		t.Run(terminal, func(t *testing.T) {
			output := newProcessSynchronizedBuffer()
			frames, err := lifecycle.NewFrameOwner(output)
			require.NoError(t, err)
			cfg := confgroup.Config{"module": "module", "name": "receiver", "update_every": 1}.
				SetSourceType(confgroup.TypeUser).SetProvider(confgroup.TypeUser).SetSource("file=test")
			modules := collectorapi.Registry{"module": {
				CreateV2: func() collectorapi.CollectorV2 {
					return &readinessIntegrationCollector{store: metrix.NewCollectorStore(), terminal: terminal}
				},
				Config: func() any { return &collectorapi.MockConfiguration{} },
			}}
			uids := lifecycle.NewUIDLedger()
			generation, err := newTestRunGeneration(t, runGenerationConfig{
				Secrets:    testRunSecrets(t),
				Generation: 1, ShutdownTimeout: time.Second, UIDs: uids, Frames: frames,
				Modules: modules, Jobs: testRunJobServices(t), Discovery: testRunDiscoveryServices(t, cfg),
			})
			require.NoError(t, err)
			t.Cleanup(func() {
				generation.Stop()
				require.NoError(t, generation.Wait(context.Background()))
				closeRunTestUIDs(t, uids)
			})
			require.NoError(t, generation.start(context.Background()))
			require.Eventually(t, func() bool {
				record, exists := generation.vnodes.graph.Lookup(cfg.FullName())
				return exists && record.Status == dyncfg.StatusFailed.String()
			}, 3*time.Second, time.Millisecond)
			require.NoError(t, generation.run.DirtyCause())
			require.Eventually(t, func() bool { return generation.tasks.LongLivedCensus().Active == 1 }, time.Second, time.Millisecond,
				"only the discovery pipeline should remain active")
			output.waitContains(t, "failed")
		})
	}
}

type startupCapabilityCollector struct {
	readinessIntegrationCollector
	cached atomic.Bool
}

func (c *startupCapabilityCollector) Check(context.Context) error {
	c.cached.Store(true)
	return nil
}

func (c *startupCapabilityCollector) Run(ctx context.Context, ready func()) error {
	c.cached.Store(false)
	ready()
	<-ctx.Done()
	return ctx.Err()
}

func (c *startupCapabilityCollector) FunctionAvailable(method string) bool {
	if method == "live" {
		return !c.cached.Load()
	}
	return c.cached.Load()
}

func TestFunctionPublicationUsesRunningJobReadiness(t *testing.T) {
	output := newProcessSynchronizedBuffer()
	frames, err := lifecycle.NewFrameOwner(output)
	require.NoError(t, err)
	modules := collectorapi.Registry{}
	modules.Register("module", collectorapi.Creator{
		CreateV2: func() collectorapi.CollectorV2 {
			return &startupCapabilityCollector{
				readinessIntegrationCollector: readinessIntegrationCollector{store: metrix.NewCollectorStore()},
			}
		},
		Config: func() any { return &collectorapi.MockConfiguration{} },
		SharedFunctions: func() []funcapi.FunctionConfig {
			return []funcapi.FunctionConfig{{ID: "cached", Aliases: []string{"alias"}}, {ID: "live"}}
		},
		MethodHandler: func(collectorapi.RuntimeJob) funcapi.MethodHandler { return &assemblyTestHandler{} },
	})
	cfg := confgroup.Config{"module": "module", "name": "receiver", "update_every": 1}.
		SetSourceType(confgroup.TypeUser).SetProvider(confgroup.TypeUser).SetSource("file=test")
	uids := lifecycle.NewUIDLedger()
	generation, err := newTestRunGeneration(t, runGenerationConfig{
		Secrets: testRunSecrets(t), Generation: 1, ShutdownTimeout: time.Second, UIDs: uids, Frames: frames,
		Modules: modules, Jobs: testRunJobServices(t), Discovery: testRunDiscoveryServices(t, cfg),
	})
	require.NoError(t, err)
	t.Cleanup(func() {
		generation.Stop()
		require.NoError(t, generation.Wait(context.Background()))
		closeRunTestUIDs(t, uids)
	})
	require.NoError(t, generation.start(context.Background()))
	require.Eventually(t, func() bool {
		record, ok := generation.vnodes.graph.Lookup(cfg.FullName())
		return ok && record.Status == dyncfg.StatusRunning.String()
	}, time.Second, time.Millisecond)
	require.NoError(t, generation.tick(t.Context(), 1))
	require.Eventually(t, func() bool {
		return strings.Contains(output.String(), `FUNCTION GLOBAL "module:live"`)
	}, time.Second, time.Millisecond)
	wire := output.String()
	require.NotContains(t, wire, `FUNCTION GLOBAL "module:cached"`)
	require.NotContains(t, wire, `FUNCTION GLOBAL "alias"`)
}
