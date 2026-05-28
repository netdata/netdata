// SPDX-License-Identifier: GPL-3.0-or-later

package jobruntime

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"strings"
	"testing"
	"time"

	"github.com/netdata/netdata/go/plugins/pkg/netdataapi"
	"github.com/netdata/netdata/go/plugins/plugin/framework/chartemit"
	"github.com/netdata/netdata/go/plugins/plugin/framework/chartengine"
	"github.com/netdata/netdata/go/plugins/plugin/framework/collectorapi"
	"github.com/netdata/netdata/go/plugins/plugin/framework/runtimecomp"
	"github.com/netdata/netdata/go/plugins/plugin/framework/vnoderegistry"
	"github.com/netdata/netdata/go/plugins/plugin/framework/vnodes"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/netdata/netdata/go/plugins/pkg/metrix"
)

type mockModuleV2 struct {
	collectorapi.Base

	initFunc    func(context.Context) error
	checkFunc   func(context.Context) error
	collectFunc func(context.Context) error
	cleanupFunc func(context.Context)

	store         metrix.CollectorStore
	template      string
	templateCalls int
	cleaned       bool
	vnode         *vnodes.VirtualNode
}

type mockRuntimeComponentService struct {
	registerErr  error
	registered   []runtimecomp.ComponentConfig
	unregistered []string
}

type writeFunc func([]byte) (int, error)

func (f writeFunc) Write(p []byte) (int, error) {
	return f(p)
}

func (m *mockRuntimeComponentService) RegisterComponent(cfg runtimecomp.ComponentConfig) error {
	if m.registerErr != nil {
		return m.registerErr
	}
	m.registered = append(m.registered, cfg)
	return nil
}

func (m *mockRuntimeComponentService) UnregisterComponent(name string) {
	m.unregistered = append(m.unregistered, name)
}

func (m *mockRuntimeComponentService) RegisterProducer(_ string, _ func() error) error {
	return nil
}

func (m *mockRuntimeComponentService) UnregisterProducer(_ string) {}

func (m *mockModuleV2) Init(ctx context.Context) error {
	if m.initFunc == nil {
		return nil
	}
	return m.initFunc(ctx)
}

func (m *mockModuleV2) Check(ctx context.Context) error {
	if m.checkFunc == nil {
		return nil
	}
	return m.checkFunc(ctx)
}

func (m *mockModuleV2) Collect(ctx context.Context) error {
	if m.collectFunc == nil {
		return nil
	}
	return m.collectFunc(ctx)
}

func (m *mockModuleV2) Cleanup(ctx context.Context) {
	if m.cleanupFunc != nil {
		m.cleanupFunc(ctx)
	}
	m.cleaned = true
}

func (m *mockModuleV2) Configuration() any                 { return nil }
func (m *mockModuleV2) VirtualNode() *vnodes.VirtualNode   { return m.vnode }
func (m *mockModuleV2) MetricStore() metrix.CollectorStore { return m.store }
func (m *mockModuleV2) ChartTemplateYAML() string {
	m.templateCalls++
	return m.template
}

func newTestJobV2(mod collectorapi.CollectorV2, out *bytes.Buffer) *JobV2 {
	return NewJobV2(JobV2Config{
		PluginName:  pluginName,
		Name:        jobName,
		ModuleName:  modName,
		FullName:    modName + "_" + jobName,
		Module:      mod,
		Out:         out,
		UpdateEvery: 1,
		Labels: map[string]string{
			"instance": "localhost",
		},
	})
}

func newTestJobV2WithVnode(mod collectorapi.CollectorV2, out *bytes.Buffer, vnode vnodes.VirtualNode) *JobV2 {
	return NewJobV2(JobV2Config{
		PluginName:  pluginName,
		Name:        jobName,
		ModuleName:  modName,
		FullName:    modName + "_" + jobName,
		Module:      mod,
		Out:         out,
		UpdateEvery: 1,
		Labels: map[string]string{
			"instance": "localhost",
		},
		Vnode: vnode,
	})
}

func newRegistryTestJobV2(t *testing.T, fullName string, registry *vnoderegistry.Registry, out *bytes.Buffer, vnode vnodes.VirtualNode) *JobV2 {
	t.Helper()
	store := metrix.NewCollectorStore()
	mod := &mockModuleV2{
		store:    store,
		template: chartTemplateV2(),
		collectFunc: func(context.Context) error {
			store.Write().SnapshotMeter("apache").Gauge("workers_busy").Observe(1)
			return nil
		},
	}
	job := NewJobV2(JobV2Config{
		PluginName:    pluginName,
		Name:          fullName,
		ModuleName:    modName,
		FullName:      fullName,
		Module:        mod,
		Out:           out,
		UpdateEvery:   1,
		Vnode:         vnode,
		VnodeRegistry: registry,
	})
	require.NoError(t, job.AutoDetection())
	return job
}

func requireDefaultScopeState(t *testing.T, job *JobV2) *jobV2ScopeState {
	t.Helper()
	state := job.scopeStates[defaultHostScopeKey]
	require.NotNil(t, state)
	return state
}

func chartTemplateV2() string {
	return `
version: v1
groups:
  - family: Workers
    metrics:
      - apache.workers_busy
    charts:
      - id: workers_busy
        title: Workers Busy
        context: workers_busy
        units: workers
        dimensions:
          - selector: apache.workers_busy
            name: busy
`
}

func chartTemplateV2ExpireAfterOne() string {
	return `
version: v1
groups:
  - family: Workers
    metrics:
      - apache.workers_busy
    charts:
      - id: workers_busy
        title: Workers Busy
        context: workers_busy
        units: workers
        lifecycle:
          expire_after_cycles: 1
        dimensions:
          - selector: apache.workers_busy
            name: busy
`
}

func chartTemplateV2Dynamic() string {
	return `
version: v1
groups:
  - family: Net
    metrics:
      - windows_net_bytes_received_total
      - windows_net_bytes_sent_total
    charts:
      - id: win_nic_traffic
        title: NIC traffic
        context: nic_traffic
        units: bytes/s
        instances:
          by_labels: [nic]
        dimensions:
          - selector: windows_net_bytes_received_total
            name: received
          - selector: windows_net_bytes_sent_total
            name: sent
`
}

func TestJobV2Scenarios(t *testing.T) {
	tests := map[string]struct {
		run func(t *testing.T)
	}{
		"auto detection succeeds with valid store/template": {
			run: func(t *testing.T) {
				mod := &mockModuleV2{
					store:    metrix.NewCollectorStore(),
					template: chartTemplateV2(),
				}
				job := newTestJobV2(mod, &bytes.Buffer{})
				require.NoError(t, job.AutoDetection())
				require.NotNil(t, job.store)
				require.NotNil(t, job.cycle)
				state, err := job.ensureScopeState(metrix.HostScope{})
				require.NoError(t, err)
				require.NotNil(t, state.engine)
				attempt, err := state.engine.PreparePlan(job.store.Read(metrix.ReadFlatten()))
				require.NoError(t, err)
				defer attempt.Abort()
				err = attempt.Commit()
				require.NoError(t, err)
			},
		},
		"auto detection fails when metric store is nil": {
			run: func(t *testing.T) {
				mod := &mockModuleV2{
					store:    nil,
					template: chartTemplateV2(),
				}
				job := newTestJobV2(mod, &bytes.Buffer{})
				require.ErrorContains(t, job.AutoDetection(), "nil metric store")
			},
		},
		"runOnce collects and emits chart actions": {
			run: func(t *testing.T) {
				store := metrix.NewCollectorStore()
				mod := &mockModuleV2{
					store:    store,
					template: chartTemplateV2(),
					collectFunc: func(context.Context) error {
						store.Write().SnapshotMeter("apache").Gauge("workers_busy").Observe(7)
						return nil
					},
				}

				var out bytes.Buffer
				job := newTestJobV2(mod, &out)
				require.NoError(t, job.AutoDetection())
				job.runOnce()

				wire := out.String()
				assert.Contains(t, wire, fmt.Sprintf(`HOST ''

CHART 'module_job.workers_busy' '' 'Workers Busy' 'workers' 'Workers' 'workers_busy' 'line' '%d' '1' '' 'plugin' 'module'
CLABEL 'instance' 'localhost' '2'
CLABEL '_collect_job' 'job' '1'
CLABEL_COMMIT
DIMENSION 'busy' 'busy' 'absolute' '1' '1' ''
BEGIN 'module_job.workers_busy'
SET 'busy' = 7
END`, chartengine.Priority))
				assert.False(t, job.Panicked())
			},
		},
		"collect error aborts cycle and emits nothing": {
			run: func(t *testing.T) {
				store := metrix.NewCollectorStore()
				mod := &mockModuleV2{
					store:    store,
					template: chartTemplateV2(),
					collectFunc: func(context.Context) error {
						return errors.New("collect failed")
					},
				}

				var out bytes.Buffer
				job := newTestJobV2(mod, &out)
				require.NoError(t, job.AutoDetection())
				job.runOnce()

				assert.Equal(t, "", out.String())
				assert.Equal(t, metrix.CollectStatusFailed, store.Read(metrix.ReadRaw()).CollectMeta().LastAttemptStatus)
			},
		},
		"panic in collect aborts active cycle and next cycle still succeeds": {
			run: func(t *testing.T) {
				store := metrix.NewCollectorStore()
				collectCalls := 0
				mod := &mockModuleV2{
					store:    store,
					template: chartTemplateV2(),
					collectFunc: func(context.Context) error {
						collectCalls++
						if collectCalls == 1 {
							panic("boom")
						}
						store.Write().SnapshotMeter("apache").Gauge("workers_busy").Observe(11)
						return nil
					},
				}

				var out bytes.Buffer
				job := newTestJobV2(mod, &out)
				require.NoError(t, job.AutoDetection())

				job.runOnce()
				assert.True(t, job.Panicked())
				assert.Equal(t, metrix.CollectStatusFailed, store.Read(metrix.ReadRaw()).CollectMeta().LastAttemptStatus)
				out.Reset()

				job.runOnce()
				assert.False(t, job.Panicked())
				assert.Equal(t, metrix.CollectStatusSuccess, store.Read(metrix.ReadRaw()).CollectMeta().LastAttemptStatus)
				assert.Contains(t, out.String(), "SET 'busy' = 11")
			},
		},
		"runOnce materializes dynamic chart instances from labels": {
			run: func(t *testing.T) {
				store := metrix.NewCollectorStore()
				mod := &mockModuleV2{
					store:    store,
					template: chartTemplateV2Dynamic(),
					collectFunc: func(context.Context) error {
						sm := store.Write().SnapshotMeter("")
						rx := sm.Counter("windows_net_bytes_received_total")
						tx := sm.Counter("windows_net_bytes_sent_total")

						eth0 := sm.LabelSet(metrix.Label{Key: "nic", Value: "eth0"})
						eth1 := sm.LabelSet(metrix.Label{Key: "nic", Value: "eth1"})

						rx.ObserveTotal(100, eth0)
						tx.ObserveTotal(80, eth0)
						rx.ObserveTotal(50, eth1)
						tx.ObserveTotal(40, eth1)
						return nil
					},
				}

				var out bytes.Buffer
				job := newTestJobV2(mod, &out)
				require.NoError(t, job.AutoDetection())
				job.runOnce()

				wire := out.String()
				assert.Contains(t, wire, fmt.Sprintf(`CHART 'module_job.win_nic_traffic_eth0' '' 'NIC traffic' 'bytes/s' 'Net' 'nic_traffic' 'line' '%d' '1' '' 'plugin' 'module'
CLABEL 'instance' 'localhost' '2'
CLABEL 'nic' 'eth0' '1'
CLABEL '_collect_job' 'job' '1'
CLABEL_COMMIT
DIMENSION 'received' 'received' 'incremental' '1' '1' ''
DIMENSION 'sent' 'sent' 'incremental' '1' '1' ''
CHART 'module_job.win_nic_traffic_eth1' '' 'NIC traffic' 'bytes/s' 'Net' 'nic_traffic' 'line' '%d' '1' '' 'plugin' 'module'
CLABEL 'instance' 'localhost' '2'
CLABEL 'nic' 'eth1' '1'
CLABEL '_collect_job' 'job' '1'
CLABEL_COMMIT
DIMENSION 'received' 'received' 'incremental' '1' '1' ''
DIMENSION 'sent' 'sent' 'incremental' '1' '1' ''
BEGIN 'module_job.win_nic_traffic_eth0'
SET 'received' = 100
SET 'sent' = 80
END

BEGIN 'module_job.win_nic_traffic_eth1'
SET 'received' = 50
SET 'sent' = 40
END`, chartengine.Priority, chartengine.Priority))
			},
		},
		"runtime component registers on successful autodetection": {
			run: func(t *testing.T) {
				store := metrix.NewCollectorStore()
				runtimeSvc := &mockRuntimeComponentService{}
				mod := &mockModuleV2{
					store:    store,
					template: chartTemplateV2(),
				}

				job := NewJobV2(JobV2Config{
					PluginName:     pluginName,
					Name:           jobName,
					ModuleName:     modName,
					FullName:       modName + "_" + jobName,
					Module:         mod,
					Out:            &bytes.Buffer{},
					UpdateEvery:    3,
					RuntimeService: runtimeSvc,
				})

				require.NoError(t, job.AutoDetection())
				require.Len(t, runtimeSvc.registered, 1)
				cfg := runtimeSvc.registered[0]
				assert.Equal(t, job.runtimeComponentName, cfg.Name)
				assert.True(t, cfg.Autogen.Enabled)
				assert.Equal(t, 3, cfg.UpdateEvery)
				assert.Equal(t, pluginName, cfg.Plugin)
				assert.Equal(t, "chartengine", cfg.Module)
				assert.Equal(t, jobName, cfg.JobName)
				assert.Equal(t, modName, cfg.JobLabels["_collect_module"])
				assert.NotContains(t, cfg.JobLabels, "source")
				assert.NotContains(t, cfg.JobLabels, "collector_module")
				require.NotNil(t, cfg.Store)
				assert.Equal(t, job.runtimeStore, cfg.Store)
			},
		},
		"module context carries runtime component service when available": {
			run: func(t *testing.T) {
				store := metrix.NewCollectorStore()
				runtimeSvc := &mockRuntimeComponentService{}
				mod := &mockModuleV2{
					store:    store,
					template: chartTemplateV2(),
					initFunc: func(ctx context.Context) error {
						got, ok := runtimecomp.ServiceFromContext(ctx)
						require.True(t, ok)
						require.NotNil(t, got)
						assert.Same(t, runtimeSvc, got)
						return nil
					},
				}

				job := NewJobV2(JobV2Config{
					PluginName:     pluginName,
					Name:           jobName,
					ModuleName:     modName,
					FullName:       modName + "_" + jobName,
					Module:         mod,
					Out:            &bytes.Buffer{},
					UpdateEvery:    1,
					RuntimeService: runtimeSvc,
				})

				require.NoError(t, job.AutoDetection())
			},
		},
		"runtime registration failure is non-fatal for autodetection": {
			run: func(t *testing.T) {
				store := metrix.NewCollectorStore()
				runtimeSvc := &mockRuntimeComponentService{registerErr: errors.New("register failed")}
				mod := &mockModuleV2{
					store:    store,
					template: chartTemplateV2(),
				}
				job := NewJobV2(JobV2Config{
					PluginName:     pluginName,
					Name:           jobName,
					ModuleName:     modName,
					FullName:       modName + "_" + jobName,
					Module:         mod,
					Out:            &bytes.Buffer{},
					UpdateEvery:    1,
					RuntimeService: runtimeSvc,
				})

				require.NoError(t, job.AutoDetection())
				assert.False(t, job.runtimeComponentRegistered)
				assert.Empty(t, runtimeSvc.registered)
			},
		},
		"cleanup unregisters runtime component": {
			run: func(t *testing.T) {
				store := metrix.NewCollectorStore()
				runtimeSvc := &mockRuntimeComponentService{}
				mod := &mockModuleV2{
					store:    store,
					template: chartTemplateV2(),
				}
				job := NewJobV2(JobV2Config{
					PluginName:     pluginName,
					Name:           jobName,
					ModuleName:     modName,
					FullName:       modName + "_" + jobName,
					Module:         mod,
					Out:            &bytes.Buffer{},
					UpdateEvery:    1,
					RuntimeService: runtimeSvc,
				})

				require.NoError(t, job.AutoDetection())
				require.True(t, job.runtimeComponentRegistered)
				componentName := job.runtimeComponentName

				job.Cleanup()
				assert.False(t, job.runtimeComponentRegistered)
				assert.Contains(t, runtimeSvc.unregistered, componentName)
			},
		},
		"panic cycle drops buffered partial output": {
			run: func(t *testing.T) {
				store := metrix.NewCollectorStore()
				mod := &mockModuleV2{
					store:    store,
					template: chartTemplateV2(),
					collectFunc: func(context.Context) error {
						panic("boom")
					},
				}

				var out bytes.Buffer
				job := newTestJobV2(mod, &out)
				require.NoError(t, job.AutoDetection())

				// Simulate partial protocol bytes already present in the cycle buffer.
				_, err := job.buf.WriteString("BEGIN 'broken'\nSET 'x' = 1\n")
				require.NoError(t, err)

				job.runOnce()
				assert.True(t, job.Panicked())
				assert.Equal(t, "", out.String())
				assert.Zero(t, job.buf.Len())
			},
		},
		"module-owned vnode is not overridden by queued job vnode updates": {
			run: func(t *testing.T) {
				store := metrix.NewCollectorStore()
				mod := &mockModuleV2{
					store:    store,
					template: chartTemplateV2(),
					vnode: &vnodes.VirtualNode{
						Name:     "old",
						Hostname: "old-host",
						GUID:     "old-guid",
					},
				}

				collectStarted := make(chan struct{})
				collectRelease := make(chan struct{})
				firstCollectDone := make(chan struct{})
				collectCalls := 0

				mod.collectFunc = func(context.Context) error {
					collectCalls++
					if collectCalls == 1 {
						close(collectStarted)
						<-collectRelease
						assert.Equal(t, "old-guid", mod.vnode.GUID)
						close(firstCollectDone)
					} else {
						assert.Equal(t, "old-guid", mod.vnode.GUID)
					}
					store.Write().SnapshotMeter("apache").Gauge("workers_busy").Observe(1)
					return nil
				}

				job := newTestJobV2WithVnode(mod, &bytes.Buffer{}, *mod.vnode.Copy())
				require.NoError(t, job.AutoDetection())

				runDone := make(chan struct{})
				go func() {
					job.runOnce()
					close(runDone)
				}()

				<-collectStarted
				job.UpdateVnode(&vnodes.VirtualNode{
					Name:     "new",
					Hostname: "new-host",
					GUID:     "new-guid",
				})
				assert.Equal(t, "old-guid", mod.vnode.GUID)

				close(collectRelease)
				<-firstCollectDone
				<-runDone

				job.runOnce()
				assert.Equal(t, "old-guid", mod.vnode.GUID)
				assert.Equal(t, "old-guid", job.Vnode().GUID)
			},
		},
		"stop cancels in-flight collect context": {
			run: func(t *testing.T) {
				store := metrix.NewCollectorStore()
				collectCtxCh := make(chan context.Context, 1)
				mod := &mockModuleV2{
					store:    store,
					template: chartTemplateV2(),
					collectFunc: func(ctx context.Context) error {
						collectCtxCh <- ctx
						<-ctx.Done()
						return ctx.Err()
					},
				}

				job := newTestJobV2(mod, &bytes.Buffer{})
				require.NoError(t, job.AutoDetection())

				startDone := make(chan struct{})
				go func() {
					job.Start()
					close(startDone)
				}()

				var collectCtx context.Context
				deadline := time.After(2 * time.Second)
			WAIT_COLLECT:
				for {
					job.Tick(1)
					select {
					case collectCtx = <-collectCtxCh:
						break WAIT_COLLECT
					case <-time.After(10 * time.Millisecond):
					case <-deadline:
						t.Fatal("collect did not start")
					}
				}

				select {
				case <-collectCtx.Done():
					t.Fatal("collect context canceled before stop")
				default:
				}

				stopDone := make(chan struct{})
				go func() {
					job.Stop()
					close(stopDone)
				}()

				select {
				case <-collectCtx.Done():
				case <-time.After(time.Second):
					t.Fatal("collect context was not canceled on stop")
				}

				select {
				case <-stopDone:
				case <-time.After(time.Second):
					t.Fatal("stop did not finish")
				}

				select {
				case <-startDone:
				case <-time.After(time.Second):
					t.Fatal("job start loop did not exit")
				}
			},
		},
		"autodetection init failure disables retry": {
			run: func(t *testing.T) {
				mod := &mockModuleV2{
					initFunc: func(context.Context) error { return errors.New("init failed") },
				}
				job := NewJobV2(JobV2Config{
					PluginName:      pluginName,
					Name:            jobName,
					ModuleName:      modName,
					FullName:        modName + "_" + jobName,
					Module:          mod,
					Out:             &bytes.Buffer{},
					UpdateEvery:     1,
					AutoDetectEvery: 1,
				})

				require.Error(t, job.AutoDetection())
				assert.False(t, job.RetryAutoDetection())
			},
		},
		"autodetection panic disables retry": {
			run: func(t *testing.T) {
				mod := &mockModuleV2{
					initFunc: func(context.Context) error { panic("boom") },
				}
				job := NewJobV2(JobV2Config{
					PluginName:      pluginName,
					Name:            jobName,
					ModuleName:      modName,
					FullName:        modName + "_" + jobName,
					Module:          mod,
					Out:             &bytes.Buffer{},
					UpdateEvery:     1,
					AutoDetectEvery: 1,
				})

				require.Error(t, job.AutoDetection())
				assert.False(t, job.RetryAutoDetection())
			},
		},
		"function-only mode skips collect loop": {
			run: func(t *testing.T) {
				collectCalls := 0
				mod := &mockModuleV2{
					collectFunc: func(context.Context) error {
						collectCalls++
						return nil
					},
				}
				job := NewJobV2(JobV2Config{
					PluginName:      pluginName,
					Name:            jobName,
					ModuleName:      modName,
					FullName:        modName + "_" + jobName,
					Module:          mod,
					Out:             &bytes.Buffer{},
					UpdateEvery:     1,
					AutoDetectEvery: 1,
					FunctionOnly:    true,
				})

				require.NoError(t, job.AutoDetection())

				done := make(chan struct{})
				go func() {
					job.Start()
					close(done)
				}()

				for i := range 3 {
					job.Tick(i + 1)
					time.Sleep(10 * time.Millisecond)
				}
				job.Stop()

				select {
				case <-done:
				case <-time.After(time.Second):
					t.Fatal("job did not stop")
				}

				assert.Equal(t, 0, collectCalls)
			},
		},
	}

	for name, tc := range tests {
		t.Run(name, tc.run)
	}
}

func TestJobV2_StartMarksNotRunningBeforeCleanup(t *testing.T) {
	cleanupStarted := make(chan struct{})
	cleanupRelease := make(chan struct{})
	cleanupEntered := make(chan struct{}, 1)

	mod := &mockModuleV2{
		store:    metrix.NewCollectorStore(),
		template: chartTemplateV2(),
		cleanupFunc: func(context.Context) {
			select {
			case cleanupEntered <- struct{}{}:
			default:
			}
			close(cleanupStarted)
			<-cleanupRelease
		},
	}

	var out bytes.Buffer
	job := newTestJobV2(mod, &out)
	require.NoError(t, job.AutoDetection())

	startDone := make(chan struct{})
	go func() {
		job.Start()
		close(startDone)
	}()

	require.Eventually(t, job.IsRunning, time.Second, 10*time.Millisecond)

	stopDone := make(chan struct{})
	go func() {
		job.Stop()
		close(stopDone)
	}()

	select {
	case <-cleanupStarted:
	case <-time.After(2 * time.Second):
		t.Fatal("timeout waiting for cleanup to start")
	}

	assert.False(t, job.IsRunning(), "job must report not running while cleanup is in progress")

	close(cleanupRelease)

	select {
	case <-stopDone:
	case <-time.After(2 * time.Second):
		t.Fatal("timeout waiting for stop to finish")
	}

	select {
	case <-startDone:
	case <-time.After(2 * time.Second):
		t.Fatal("timeout waiting for start loop to exit")
	}

	select {
	case <-cleanupEntered:
	default:
		t.Fatal("cleanup function was not entered")
	}
}

func TestJobV2VnodeEmissionLifecycle(t *testing.T) {
	store := metrix.NewCollectorStore()
	current := 1.0
	mod := &mockModuleV2{
		store:    store,
		template: chartTemplateV2(),
		collectFunc: func(context.Context) error {
			store.Write().SnapshotMeter("apache").Gauge("workers_busy").Observe(current)
			return nil
		},
	}

	var out bytes.Buffer
	job := newTestJobV2WithVnode(mod, &out, vnodes.VirtualNode{
		Hostname: "node-host",
		GUID:     "node-guid",
		Labels: map[string]string{
			"region": "eu'\n",
		},
	})
	require.NoError(t, job.AutoDetection())

	job.runOnce()
	wire := out.String()
	assert.Contains(t, wire, `HOST_DEFINE 'node-guid' 'node-host'
HOST_LABEL '_hostname' 'node-host'
HOST_LABEL 'region' 'eu '
HOST_DEFINE_END

HOST 'node-guid'

CHART 'module_job.workers_busy'`)
	assert.NotContains(t, wire, "HOST ''")

	out.Reset()
	current = 2
	job.runOnce()
	wire = out.String()
	assert.Contains(t, wire, `HOST 'node-guid'

BEGIN 'module_job.workers_busy'`)
	assert.NotContains(t, wire, "HOST_DEFINE 'node-guid' 'node-host'")

	out.Reset()
	job.UpdateVnode(&vnodes.VirtualNode{
		Hostname: "node-host-2",
		GUID:     "node-guid-2",
	})
	current = 3
	job.runOnce()
	wire = out.String()
	assert.Contains(t, wire, `HOST_DEFINE 'node-guid-2' 'node-host-2'
HOST_LABEL '_hostname' 'node-host-2'
HOST_DEFINE_END

HOST 'node-guid-2'

CHART 'module_job.workers_busy'`)
}

func TestJobV2ModuleOwnedVnodeSameGUIDMetadataRefresh(t *testing.T) {
	cases := map[string]struct {
		mutate         func(*vnodes.VirtualNode)
		wantDefine     bool
		wantDefineWire string
		wantInfo       netdataapi.HostInfo
	}{
		"unchanged metadata does not redefine": {
			mutate:     func(*vnodes.VirtualNode) {},
			wantDefine: false,
			wantInfo: netdataapi.HostInfo{
				GUID:     "node-guid",
				Hostname: "node-host-a",
				Labels: map[string]string{
					"_hostname": "node-host-a",
					"region":    "eu",
				},
			},
		},
		"hostname change redefines same guid": {
			mutate: func(vnode *vnodes.VirtualNode) {
				vnode.Hostname = "node-host-b"
			},
			wantDefine: true,
			wantDefineWire: `HOST_DEFINE 'node-guid' 'node-host-b'
HOST_LABEL '_hostname' 'node-host-b'
HOST_LABEL 'region' 'eu'
HOST_DEFINE_END

HOST 'node-guid'

BEGIN 'module_job.workers_busy'`,
			wantInfo: netdataapi.HostInfo{
				GUID:     "node-guid",
				Hostname: "node-host-b",
				Labels: map[string]string{
					"_hostname": "node-host-b",
					"region":    "eu",
				},
			},
		},
		"label change redefines same guid": {
			mutate: func(vnode *vnodes.VirtualNode) {
				vnode.Labels["region"] = "us"
			},
			wantDefine: true,
			wantDefineWire: `HOST_DEFINE 'node-guid' 'node-host-a'
HOST_LABEL '_hostname' 'node-host-a'
HOST_LABEL 'region' 'us'
HOST_DEFINE_END

HOST 'node-guid'

BEGIN 'module_job.workers_busy'`,
			wantInfo: netdataapi.HostInfo{
				GUID:     "node-guid",
				Hostname: "node-host-a",
				Labels: map[string]string{
					"_hostname": "node-host-a",
					"region":    "us",
				},
			},
		},
	}

	for name, tc := range cases {
		t.Run(name, func(t *testing.T) {
			store := metrix.NewCollectorStore()
			current := 1.0
			modVnode := &vnodes.VirtualNode{
				Hostname: "node-host-a",
				GUID:     "node-guid",
				Labels: map[string]string{
					"region": "eu",
				},
			}
			mod := &mockModuleV2{
				store:    store,
				template: chartTemplateV2(),
				vnode:    modVnode,
				collectFunc: func(context.Context) error {
					store.Write().SnapshotMeter("apache").Gauge("workers_busy").Observe(current)
					return nil
				},
			}

			var out bytes.Buffer
			job := newTestJobV2WithVnode(mod, &out, *modVnode.Copy())
			require.NoError(t, job.AutoDetection())

			initialInfo, err := chartemit.PrepareHostInfo(netdataapi.HostInfo{
				GUID:     "node-guid",
				Hostname: "node-host-a",
				Labels: map[string]string{
					"region": "eu",
				},
			})
			require.NoError(t, err)

			job.runOnce()
			require.Equal(t, initialInfo, requireDefaultScopeState(t, job).host.definedInfo)

			out.Reset()
			tc.mutate(modVnode)
			current = 2

			job.runOnce()
			wire := out.String()
			if tc.wantDefine {
				assert.Contains(t, wire, tc.wantDefineWire)
			} else {
				assert.NotContains(t, wire, `HOST_DEFINE 'node-guid'`)
				assert.Contains(t, wire, `HOST 'node-guid'

BEGIN 'module_job.workers_busy'`)
			}

			expectedInfo, err := chartemit.PrepareHostInfo(tc.wantInfo)
			require.NoError(t, err)
			assert.Equal(t, expectedInfo, requireDefaultScopeState(t, job).host.definedInfo)
		})
	}
}

func TestJobV2VnodeRegistryScenarios(t *testing.T) {
	cases := map[string]struct {
		run func(t *testing.T)
	}{
		"shared registry suppresses duplicate and updates changed metadata": {
			run: func(t *testing.T) {
				registry := vnoderegistry.New()
				jobAOut := &bytes.Buffer{}
				jobBOut := &bytes.Buffer{}

				jobA := newRegistryTestJobV2(t, "module_job_a", registry, jobAOut, vnodes.VirtualNode{
					Hostname: "node-host-a",
					GUID:     "node-guid",
					Labels: map[string]string{
						"region": "eu",
					},
				})
				jobB := newRegistryTestJobV2(t, "module_job_b", registry, jobBOut, vnodes.VirtualNode{
					Hostname: "node-host-b",
					GUID:     "node-guid",
					Labels: map[string]string{
						"region": "us",
					},
				})

				jobA.runOnce()
				assert.Contains(t, jobAOut.String(), `HOST_DEFINE 'node-guid' 'node-host-a'`)
				assert.Contains(t, jobAOut.String(), `HOST 'node-guid'`)

				jobB.runOnce()
				assert.Contains(t, jobBOut.String(), `HOST_DEFINE 'node-guid' 'node-host-b'`)
				assert.Contains(t, jobBOut.String(), `HOST 'node-guid'`)

				info, ok := registry.Lookup("node-guid")
				require.True(t, ok)
				assert.Equal(t, "node-host-b", info.Hostname)

				jobAOut.Reset()
				jobA.runOnce()
				assert.Contains(t, jobAOut.String(), `HOST_DEFINE 'node-guid' 'node-host-a'`)
				info, ok = registry.Lookup("node-guid")
				require.True(t, ok)
				assert.Equal(t, "node-host-a", info.Hostname)

				assert.Equal(t, []vnoderegistry.Owner{
					vnoderegistry.Owner("module_job_a\xffjob\xffnode-guid"),
					vnoderegistry.Owner("module_job_b\xffjob\xffnode-guid"),
				}, registry.Owners("node-guid"))

				jobA.Cleanup()
				assert.Equal(t, []vnoderegistry.Owner{vnoderegistry.Owner("module_job_b\xffjob\xffnode-guid")}, registry.Owners("node-guid"))
				jobB.Cleanup()
				assert.Equal(t, 0, registry.Len())
			},
		},
		"rollback on apply failure": {
			run: func(t *testing.T) {
				registry := vnoderegistry.New()
				_, err := registry.Register("other", netdataapi.HostInfo{
					GUID:     "node-guid",
					Hostname: "node-host-a",
				})
				require.NoError(t, err)

				store := metrix.NewCollectorStore()
				mod := &mockModuleV2{
					store:    store,
					template: chartTemplateV2(),
					collectFunc: func(context.Context) error {
						store.Write().SnapshotMeter("apache").Gauge("workers_busy").Observe(1)
						return nil
					},
				}

				var out bytes.Buffer
				job := NewJobV2(JobV2Config{
					PluginName:    pluginName,
					Name:          jobName,
					ModuleName:    modName,
					FullName:      strings.Repeat("a", 1200),
					Module:        mod,
					Out:           &out,
					UpdateEvery:   1,
					VnodeRegistry: registry,
					Vnode: vnodes.VirtualNode{
						Hostname: "node-host-b",
						GUID:     "node-guid",
					},
				})
				require.NoError(t, job.AutoDetection())

				job.runOnce()

				assert.Empty(t, out.String())
				info, ok := registry.Lookup("node-guid")
				require.True(t, ok)
				assert.Equal(t, "node-host-a", info.Hostname)
				assert.Equal(t, []vnoderegistry.Owner{vnoderegistry.Owner("other")}, registry.Owners("node-guid"))
			},
		},
		"rollback on commit failure emits nothing and next cycle recovers": {
			run: func(t *testing.T) {
				registry := vnoderegistry.New()
				store := metrix.NewCollectorStore()
				current := 1.0
				mod := &mockModuleV2{
					store:    store,
					template: chartTemplateV2(),
					collectFunc: func(context.Context) error {
						store.Write().SnapshotMeter("apache").Gauge("workers_busy").Observe(current)
						return nil
					},
				}

				var out bytes.Buffer
				job := NewJobV2(JobV2Config{
					PluginName:    pluginName,
					Name:          jobName,
					ModuleName:    modName,
					FullName:      modName + "_" + jobName,
					Module:        mod,
					Out:           &out,
					UpdateEvery:   1,
					VnodeRegistry: registry,
					Vnode: vnodes.VirtualNode{
						Hostname: "node-host",
						GUID:     "node-guid",
					},
				})
				require.NoError(t, job.AutoDetection())

				prepared, ok := job.collectAndEmit(0)
				require.True(t, ok)
				require.Len(t, prepared.scopes, 1)
				require.NotEmpty(t, prepared.scopes[0].output)
				assert.NotEmpty(t, registry.Owners("node-guid"))

				requireDefaultScopeState(t, job).engine.ResetMaterialized()
				require.ErrorIs(t, job.finishPreparedEmission(prepared), chartengine.ErrStalePlanAttempt)
				assert.Empty(t, out.String())
				assert.Empty(t, registry.Owners("node-guid"))

				current = 2
				job.runOnce()
				assert.Contains(t, out.String(), `HOST_DEFINE 'node-guid' 'node-host'`)
				assert.Contains(t, out.String(), "SET 'busy' = 2")
				assert.Equal(t, []vnoderegistry.Owner{vnoderegistry.Owner("module_job\xffjob\xffnode-guid")}, registry.Owners("node-guid"))
			},
		},
		"guid change releases superseded owner": {
			run: func(t *testing.T) {
				registry := vnoderegistry.New()
				store := metrix.NewCollectorStore()
				current := 1.0
				mod := &mockModuleV2{
					store:    store,
					template: chartTemplateV2(),
					collectFunc: func(context.Context) error {
						store.Write().SnapshotMeter("apache").Gauge("workers_busy").Observe(current)
						return nil
					},
				}

				var out bytes.Buffer
				job := NewJobV2(JobV2Config{
					PluginName:    pluginName,
					Name:          jobName,
					ModuleName:    modName,
					FullName:      modName + "_" + jobName,
					Module:        mod,
					Out:           &out,
					UpdateEvery:   1,
					VnodeRegistry: registry,
					Vnode: vnodes.VirtualNode{
						Hostname: "node-host-a",
						GUID:     "node-guid-a",
					},
				})
				require.NoError(t, job.AutoDetection())

				job.runOnce()
				assert.Equal(t, []vnoderegistry.Owner{vnoderegistry.Owner("module_job\xffjob\xffnode-guid-a")}, registry.Owners("node-guid-a"))

				out.Reset()
				current = 2
				job.UpdateVnode(&vnodes.VirtualNode{
					Hostname: "node-host-b",
					GUID:     "node-guid-b",
				})
				job.runOnce()

				assert.Empty(t, registry.Owners("node-guid-a"))
				assert.Equal(t, []vnoderegistry.Owner{vnoderegistry.Owner("module_job\xffjob\xffnode-guid-b")}, registry.Owners("node-guid-b"))
			},
		},
		"vnode to global switch releases superseded owner": {
			run: func(t *testing.T) {
				registry := vnoderegistry.New()
				store := metrix.NewCollectorStore()
				current := 1.0
				mod := &mockModuleV2{
					store:    store,
					template: chartTemplateV2(),
					collectFunc: func(context.Context) error {
						store.Write().SnapshotMeter("apache").Gauge("workers_busy").Observe(current)
						return nil
					},
				}

				var out bytes.Buffer
				job := NewJobV2(JobV2Config{
					PluginName:    pluginName,
					Name:          jobName,
					ModuleName:    modName,
					FullName:      modName + "_" + jobName,
					Module:        mod,
					Out:           &out,
					UpdateEvery:   1,
					VnodeRegistry: registry,
					Vnode: vnodes.VirtualNode{
						Hostname: "node-host",
						GUID:     "node-guid",
					},
				})
				require.NoError(t, job.AutoDetection())

				job.runOnce()
				require.Equal(t, []vnoderegistry.Owner{vnoderegistry.Owner("module_job\xffjob\xffnode-guid")}, registry.Owners("node-guid"))

				out.Reset()
				current = 2
				job.UpdateVnode(&vnodes.VirtualNode{})
				job.runOnce()

				assert.Empty(t, registry.Owners("node-guid"))
				assert.Contains(t, out.String(), `HOST ''`)
				assert.Contains(t, out.String(), "SET 'busy' = 2")
			},
		},
		"cleanup emits obsoletes before releasing owner": {
			run: func(t *testing.T) {
				registry := vnoderegistry.New()
				var out bytes.Buffer
				job := newRegistryTestJobV2(t, "module_job", registry, &out, vnodes.VirtualNode{
					Hostname: "node-host",
					GUID:     "node-guid",
				})

				job.runOnce()
				require.Equal(t, []vnoderegistry.Owner{vnoderegistry.Owner("module_job\xffjob\xffnode-guid")}, registry.Owners("node-guid"))

				ownerPresentDuringWrite := false
				job.out = writeFunc(func(p []byte) (int, error) {
					ownerPresentDuringWrite = assert.Contains(t, registry.Owners("node-guid"), vnoderegistry.Owner("module_job\xffjob\xffnode-guid"))
					return len(p), nil
				})

				job.Cleanup()

				assert.True(t, ownerPresentDuringWrite)
				assert.Empty(t, registry.Owners("node-guid"))
			},
		},
		"cleanup with obsolete disabled releases owners and clears state": {
			run: func(t *testing.T) {
				registry := vnoderegistry.New()
				var out bytes.Buffer
				job := newRegistryTestJobV2(t, "module_job", registry, &out, vnodes.VirtualNode{
					Hostname: "node-host",
					GUID:     "node-guid",
				})

				job.runOnce()
				require.Equal(t, []vnoderegistry.Owner{vnoderegistry.Owner("module_job\xffjob\xffnode-guid")}, registry.Owners("node-guid"))
				require.NotEmpty(t, job.scopeStates)

				out.Reset()
				collectorapi.ObsoleteCharts(false)
				defer collectorapi.ObsoleteCharts(true)
				job.Cleanup()

				assert.Empty(t, out.String())
				assert.Empty(t, registry.Owners("node-guid"))
				assert.Empty(t, job.scopeStates)
			},
		},
		"bad hostname aborts cycle without owner leak": {
			run: func(t *testing.T) {
				registry := vnoderegistry.New()
				store := metrix.NewCollectorStore()
				mod := &mockModuleV2{
					store:    store,
					template: chartTemplateV2(),
					collectFunc: func(context.Context) error {
						store.Write().SnapshotMeter("apache").Gauge("workers_busy").Observe(1)
						return nil
					},
				}

				var out bytes.Buffer
				job := NewJobV2(JobV2Config{
					PluginName:    pluginName,
					Name:          jobName,
					ModuleName:    modName,
					FullName:      modName + "_" + jobName,
					Module:        mod,
					Out:           &out,
					UpdateEvery:   1,
					VnodeRegistry: registry,
					Vnode: vnodes.VirtualNode{
						Hostname: "bad\nhost",
						GUID:     "node-guid",
					},
				})
				require.NoError(t, job.AutoDetection())

				job.runOnce()

				assert.Empty(t, out.String())
				assert.Empty(t, registry.Owners("node-guid"))
			},
		},
		"empty plan does not reserve registry": {
			run: func(t *testing.T) {
				store := metrix.NewCollectorStore()
				registry := vnoderegistry.New()
				emitValue := false
				mod := &mockModuleV2{
					store:    store,
					template: chartTemplateV2(),
					collectFunc: func(context.Context) error {
						if emitValue {
							store.Write().SnapshotMeter("apache").Gauge("workers_busy").Observe(1)
						}
						return nil
					},
				}

				var out bytes.Buffer
				job := NewJobV2(JobV2Config{
					PluginName:    pluginName,
					Name:          jobName,
					ModuleName:    modName,
					FullName:      modName + "_" + jobName,
					Module:        mod,
					Out:           &out,
					UpdateEvery:   1,
					VnodeRegistry: registry,
					Vnode: vnodes.VirtualNode{
						Hostname: "node-host",
						GUID:     "node-guid",
					},
				})
				require.NoError(t, job.AutoDetection())

				job.runOnce()
				assert.Equal(t, "", out.String())
				assert.Equal(t, 0, registry.Len())

				emitValue = true
				job.runOnce()
				assert.Contains(t, out.String(), `HOST_DEFINE 'node-guid' 'node-host'`)
				assert.Equal(t, 1, registry.Len())
			},
		},
		"empty plan does not mark vnode defined": {
			run: func(t *testing.T) {
				store := metrix.NewCollectorStore()
				emitValue := false
				mod := &mockModuleV2{
					store:    store,
					template: chartTemplateV2(),
					collectFunc: func(context.Context) error {
						if emitValue {
							store.Write().SnapshotMeter("apache").Gauge("workers_busy").Observe(1)
						}
						return nil
					},
				}

				var out bytes.Buffer
				job := newTestJobV2WithVnode(mod, &out, vnodes.VirtualNode{
					Hostname: "node-host",
					GUID:     "node-guid",
				})
				require.NoError(t, job.AutoDetection())

				job.runOnce()
				assert.Equal(t, "", out.String())
				assert.Nil(t, job.scopeStates[defaultHostScopeKey])

				emitValue = true
				job.runOnce()
				assert.Contains(t, out.String(), `HOST_DEFINE 'node-guid' 'node-host'`)
				assert.Equal(t, jobV2HostRef{kind: jobV2HostVnode, guid: "node-guid"}, requireDefaultScopeState(t, job).host.definedHost)
			},
		},
	}

	for name, tc := range cases {
		t.Run(name, tc.run)
	}
}

func TestJobV2HostScopeScenarios(t *testing.T) {
	scopeA := metrix.HostScope{ScopeKey: "scope-a", GUID: "guid-a", Hostname: "host-a", Labels: map[string]string{"workload": "a"}}
	scopeB := metrix.HostScope{ScopeKey: "scope-b", GUID: "guid-b", Hostname: "host-b", Labels: map[string]string{"workload": "b"}}

	cases := map[string]struct {
		run func(t *testing.T)
	}{
		"mixed default and explicit scopes emit deterministic host batches": {
			run: func(t *testing.T) {
				store := metrix.NewCollectorStore()
				mod := &mockModuleV2{
					store:    store,
					template: chartTemplateV2(),
					collectFunc: func(context.Context) error {
						meter := store.Write().SnapshotMeter("apache")
						meter.Gauge("workers_busy").Observe(1)
						meter.WithHostScope(scopeB).Gauge("workers_busy").Observe(2)
						meter.WithHostScope(scopeA).Gauge("workers_busy").Observe(3)
						return nil
					},
				}

				var out bytes.Buffer
				registry := vnoderegistry.New()
				job := NewJobV2(JobV2Config{
					PluginName:    pluginName,
					Name:          jobName,
					ModuleName:    modName,
					FullName:      modName + "_" + jobName,
					Module:        mod,
					Out:           &out,
					UpdateEvery:   1,
					VnodeRegistry: registry,
				})
				require.NoError(t, job.AutoDetection())

				job.runOnce()

				wire := out.String()
				assert.Contains(t, wire, `HOST ''

CHART 'module_job.workers_busy'`)
				assert.Contains(t, wire, `HOST_DEFINE 'guid-a' 'host-a'`)
				assert.Contains(t, wire, `HOST 'guid-a'

CHART 'module_job.workers_busy'`)
				assert.Contains(t, wire, `HOST_DEFINE 'guid-b' 'host-b'`)
				assert.Contains(t, wire, `HOST 'guid-b'

CHART 'module_job.workers_busy'`)
				assertContainsInOrder(t, wire, "HOST ''", "HOST_DEFINE 'guid-a'", "HOST_DEFINE 'guid-b'")
				assert.Equal(t, []vnoderegistry.Owner{vnoderegistry.Owner("module_job\xffscope\xffscope-a\xffguid-a")}, registry.Owners("guid-a"))
				assert.Equal(t, []vnoderegistry.Owner{vnoderegistry.Owner("module_job\xffscope\xffscope-b\xffguid-b")}, registry.Owners("guid-b"))
			},
		},
		"bad explicit scope does not block default scope": {
			run: func(t *testing.T) {
				store := metrix.NewCollectorStore()
				badScope := metrix.HostScope{ScopeKey: "bad", GUID: "bad-guid", Hostname: "bad\nhost"}
				mod := &mockModuleV2{
					store:    store,
					template: chartTemplateV2(),
					collectFunc: func(context.Context) error {
						meter := store.Write().SnapshotMeter("apache")
						meter.Gauge("workers_busy").Observe(1)
						meter.WithHostScope(badScope).Gauge("workers_busy").Observe(2)
						return nil
					},
				}

				var out bytes.Buffer
				registry := vnoderegistry.New()
				job := NewJobV2(JobV2Config{
					PluginName:    pluginName,
					Name:          jobName,
					ModuleName:    modName,
					FullName:      modName + "_" + jobName,
					Module:        mod,
					Out:           &out,
					UpdateEvery:   1,
					VnodeRegistry: registry,
				})
				require.NoError(t, job.AutoDetection())

				job.runOnce()

				wire := out.String()
				assert.Contains(t, wire, `HOST ''

CHART 'module_job.workers_busy'`)
				assert.Contains(t, wire, "SET 'busy' = 1")
				assert.NotContains(t, wire, "bad-guid")
				assert.Empty(t, registry.Owners("bad-guid"))
				assert.Equal(t, int64(0), job.retries.Load())
			},
		},
		"disappeared scope is removed through chartengine lifecycle and releases owner": {
			run: func(t *testing.T) {
				store := metrix.NewCollectorStore()
				emitScope := true
				mod := &mockModuleV2{
					store:    store,
					template: chartTemplateV2ExpireAfterOne(),
					collectFunc: func(context.Context) error {
						if emitScope {
							store.Write().SnapshotMeter("apache").WithHostScope(scopeA).Gauge("workers_busy").Observe(7)
						}
						return nil
					},
				}

				var out bytes.Buffer
				registry := vnoderegistry.New()
				job := NewJobV2(JobV2Config{
					PluginName:    pluginName,
					Name:          jobName,
					ModuleName:    modName,
					FullName:      modName + "_" + jobName,
					Module:        mod,
					Out:           &out,
					UpdateEvery:   1,
					VnodeRegistry: registry,
				})
				require.NoError(t, job.AutoDetection())

				job.runOnce()
				require.Contains(t, out.String(), `HOST_DEFINE 'guid-a' 'host-a'`)
				require.NotEmpty(t, registry.Owners("guid-a"))
				require.NotNil(t, job.scopeStates["scope-a"])

				out.Reset()
				emitScope = false
				job.runOnce()

				wire := out.String()
				assert.Contains(t, wire, `HOST 'guid-a'`)
				assert.Contains(t, wire, "obsolete")
				assert.Empty(t, registry.Owners("guid-a"))
				assert.Nil(t, job.scopeStates["scope-a"])
			},
		},
		"disappeared zero-action scope is removed without registry owner": {
			run: func(t *testing.T) {
				store := metrix.NewCollectorStore()
				emitScope := true
				mod := &mockModuleV2{
					store:    store,
					template: chartTemplateV2(),
					collectFunc: func(context.Context) error {
						if emitScope {
							store.Write().SnapshotMeter("apache").WithHostScope(scopeA).Gauge("workers_idle").Observe(7)
						}
						return nil
					},
				}

				var out bytes.Buffer
				registry := vnoderegistry.New()
				job := NewJobV2(JobV2Config{
					PluginName:    pluginName,
					Name:          jobName,
					ModuleName:    modName,
					FullName:      modName + "_" + jobName,
					Module:        mod,
					Out:           &out,
					UpdateEvery:   1,
					VnodeRegistry: registry,
				})
				require.NoError(t, job.AutoDetection())

				job.runOnce()
				assert.Empty(t, out.String())
				assert.Empty(t, registry.Owners("guid-a"))
				require.NotNil(t, job.scopeStates["scope-a"])
				assert.Empty(t, job.scopeStates["scope-a"].host.cleanupCharts)

				emitScope = false
				job.runOnce()

				assert.Empty(t, out.String())
				assert.Empty(t, registry.Owners("guid-a"))
				assert.Nil(t, job.scopeStates["scope-a"])
			},
		},
		"default scope removal runs while explicit scope remains": {
			run: func(t *testing.T) {
				store := metrix.NewCollectorStore()
				emitDefault := true
				mod := &mockModuleV2{
					store:    store,
					template: chartTemplateV2ExpireAfterOne(),
					collectFunc: func(context.Context) error {
						meter := store.Write().SnapshotMeter("apache")
						if emitDefault {
							meter.Gauge("workers_busy").Observe(1)
						}
						meter.WithHostScope(scopeA).Gauge("workers_busy").Observe(7)
						return nil
					},
				}

				var out bytes.Buffer
				job := newTestJobV2(mod, &out)
				require.NoError(t, job.AutoDetection())

				job.runOnce()
				require.NotNil(t, job.scopeStates[defaultHostScopeKey])
				require.NotNil(t, job.scopeStates["scope-a"])

				out.Reset()
				emitDefault = false
				job.runOnce()

				wire := out.String()
				assert.Contains(t, wire, `HOST ''`)
				assert.Contains(t, wire, "obsolete")
				assert.Nil(t, job.scopeStates[defaultHostScopeKey])
				assert.NotNil(t, job.scopeStates["scope-a"])
			},
		},
		"per-scope commit failure does not block peer scope": {
			run: func(t *testing.T) {
				store := metrix.NewCollectorStore()
				mod := &mockModuleV2{
					store:    store,
					template: chartTemplateV2(),
					collectFunc: func(context.Context) error {
						meter := store.Write().SnapshotMeter("apache")
						meter.Gauge("workers_busy").Observe(1)
						meter.WithHostScope(scopeA).Gauge("workers_busy").Observe(7)
						return nil
					},
				}

				var out bytes.Buffer
				registry := vnoderegistry.New()
				job := NewJobV2(JobV2Config{
					PluginName:    pluginName,
					Name:          jobName,
					ModuleName:    modName,
					FullName:      modName + "_" + jobName,
					Module:        mod,
					Out:           &out,
					UpdateEvery:   1,
					VnodeRegistry: registry,
				})
				require.NoError(t, job.AutoDetection())

				prepared, ok := job.collectAndEmit(0)
				require.True(t, ok)
				require.Len(t, prepared.scopes, 2)
				for _, scope := range prepared.scopes {
					if scope.scope.scopeKey == "scope-a" {
						scope.scope.engine.ResetMaterialized()
					}
				}

				require.NoError(t, job.finishPreparedEmission(prepared))

				wire := out.String()
				assert.Contains(t, wire, `HOST ''`)
				assert.Contains(t, wire, "SET 'busy' = 1")
				assert.NotContains(t, wire, "guid-a")
				assert.Empty(t, registry.Owners("guid-a"))
				assert.NotNil(t, job.scopeStates[defaultHostScopeKey])
			},
		},
		"cleanup apply failure on one scope does not block peer cleanup": {
			run: func(t *testing.T) {
				store := metrix.NewCollectorStore()
				mod := &mockModuleV2{store: store, template: chartTemplateV2()}
				var out bytes.Buffer
				job := newTestJobV2(mod, &out)
				require.NoError(t, job.AutoDetection())

				okMeta := chartengine.ChartMeta{
					Title:    "OK",
					Family:   "Workers",
					Context:  "workers_ok",
					Units:    "workers",
					Type:     chartengine.ChartTypeLine,
					Priority: chartengine.Priority,
				}
				badMeta := okMeta
				badMeta.Title = "Bad"
				job.scopeStates = map[string]*jobV2ScopeState{
					defaultHostScopeKey: {
						scopeKey: defaultHostScopeKey,
						host: jobV2HostState{
							cleanupOwner: jobV2HostRef{kind: jobV2HostGlobal},
							cleanupCharts: map[string]chartengine.ChartMeta{
								"workers_ok": okMeta,
							},
						},
					},
					"bad": {
						scopeKey: "bad",
						scope:    scopeA,
						host: jobV2HostState{
							cleanupOwner: jobV2HostRef{kind: jobV2HostGlobal},
							cleanupCharts: map[string]chartengine.ChartMeta{
								strings.Repeat("x", 1300): badMeta,
							},
						},
					},
				}

				job.Cleanup()

				assert.Contains(t, out.String(), "workers_ok")
				assert.Contains(t, out.String(), "obsolete")
				assert.Empty(t, job.scopeStates)
			},
		},
		"panic after scoped runtime samples resets aggregator and in-flight scope": {
			run: func(t *testing.T) {
				store := metrix.NewCollectorStore()
				mod := &mockModuleV2{
					store:    store,
					template: chartTemplateV2(),
					collectFunc: func(context.Context) error {
						meter := store.Write().SnapshotMeter("apache")
						meter.Gauge("workers_busy").Observe(1)
						meter.WithHostScope(scopeA).Gauge("workers_busy").Observe(7)
						return nil
					},
				}

				var out bytes.Buffer
				registry := vnoderegistry.New()
				job := NewJobV2(JobV2Config{
					PluginName:    pluginName,
					Name:          jobName,
					ModuleName:    modName,
					FullName:      modName + "_" + jobName,
					Module:        mod,
					Out:           &out,
					UpdateEvery:   1,
					VnodeRegistry: registry,
				})
				require.NoError(t, job.AutoDetection())
				job.api = netdataapi.New(writeFunc(func(p []byte) (int, error) {
					if bytes.Contains(p, []byte("HOST_DEFINE 'guid-a'")) {
						panic("boom")
					}
					return job.buf.Write(p)
				}))

				job.runOnce()

				assert.True(t, job.Panicked())
				assert.Empty(t, out.String())
				assert.Empty(t, registry.Owners(scopeA.GUID))
				value, ok := job.runtimeStore.Read(metrix.ReadRaw()).Value("netdata.go.plugin.framework.chartengine.build_success_total", nil)
				if ok {
					assert.Zero(t, value)
				}

				job.api = netdataapi.New(job.buf)
				out.Reset()
				job.runOnce()

				assert.False(t, job.Panicked())
				assert.Contains(t, out.String(), `HOST_DEFINE 'guid-a' 'host-a'`)
				assert.NotEmpty(t, registry.Owners(scopeA.GUID))
			},
		},
	}

	for name, tc := range cases {
		t.Run(name, tc.run)
	}
}

func assertContainsInOrder(t *testing.T, s string, parts ...string) {
	t.Helper()
	offset := 0
	for _, part := range parts {
		idx := strings.Index(s[offset:], part)
		require.NotEqualf(t, -1, idx, "expected %q after offset %d", part, offset)
		offset += idx + len(part)
	}
}

func TestJobV2CleanupUsesLastSuccessfulHostAfterFailedHostSwitch(t *testing.T) {
	store := metrix.NewCollectorStore()
	current := 1.0
	failCollect := false
	mod := &mockModuleV2{
		store:    store,
		template: chartTemplateV2(),
		collectFunc: func(context.Context) error {
			if failCollect {
				return errors.New("collect failed")
			}
			store.Write().SnapshotMeter("apache").Gauge("workers_busy").Observe(current)
			return nil
		},
	}

	var out bytes.Buffer
	job := newTestJobV2WithVnode(mod, &out, vnodes.VirtualNode{
		Hostname: "node-host-a",
		GUID:     "node-guid-a",
	})
	require.NoError(t, job.AutoDetection())

	job.runOnce()
	out.Reset()

	failCollect = true
	job.UpdateVnode(&vnodes.VirtualNode{
		Hostname: "node-host-b",
		GUID:     "node-guid-b",
	})
	job.runOnce()
	assert.Equal(t, "", out.String())

	job.Cleanup()

	wire := out.String()
	assert.Contains(t, wire, fmt.Sprintf(`HOST 'node-guid-a'

CHART 'module_job.workers_busy' '' 'Workers Busy' 'workers' 'Workers' 'workers_busy' 'line' '%d' '1' 'obsolete' 'plugin' 'module'`, chartengine.Priority))
	assert.NotContains(t, wire, "HOST 'node-guid-b'")
	assert.Empty(t, job.scopeStates)
}

func TestJobV2EmptyHostSwitchDoesNotKeepReloadingEngine(t *testing.T) {
	store := metrix.NewCollectorStore()
	emitValue := true
	mod := &mockModuleV2{
		store:    store,
		template: chartTemplateV2(),
		collectFunc: func(context.Context) error {
			if emitValue {
				store.Write().SnapshotMeter("apache").Gauge("workers_busy").Observe(1)
			}
			return nil
		},
	}

	var out bytes.Buffer
	job := newTestJobV2WithVnode(mod, &out, vnodes.VirtualNode{
		Hostname: "node-host-a",
		GUID:     "node-guid-a",
	})
	require.NoError(t, job.AutoDetection())
	require.Equal(t, 1, mod.templateCalls)

	job.runOnce()
	require.Equal(t, 1, mod.templateCalls)
	require.Equal(t, jobV2HostRef{kind: jobV2HostVnode, guid: "node-guid-a"}, requireDefaultScopeState(t, job).host.engineHost)
	require.Equal(t, jobV2HostRef{kind: jobV2HostVnode, guid: "node-guid-a"}, requireDefaultScopeState(t, job).host.cleanupOwner)

	out.Reset()
	emitValue = false
	job.UpdateVnode(&vnodes.VirtualNode{
		Hostname: "node-host-b",
		GUID:     "node-guid-b",
	})
	job.runOnce()
	assert.Equal(t, "", out.String())
	require.Equal(t, 1, mod.templateCalls)
	require.Equal(t, jobV2HostRef{kind: jobV2HostVnode, guid: "node-guid-b"}, requireDefaultScopeState(t, job).host.engineHost)
	require.Equal(t, jobV2HostRef{kind: jobV2HostVnode, guid: "node-guid-a"}, requireDefaultScopeState(t, job).host.cleanupOwner)

	out.Reset()
	job.runOnce()
	assert.Equal(t, "", out.String())
	assert.Equal(t, 1, mod.templateCalls)
	assert.Equal(t, jobV2HostRef{kind: jobV2HostVnode, guid: "node-guid-b"}, requireDefaultScopeState(t, job).host.engineHost)
	assert.Equal(t, jobV2HostRef{kind: jobV2HostVnode, guid: "node-guid-a"}, requireDefaultScopeState(t, job).host.cleanupOwner)
}

func TestJobV2CleanupDoesNotSuppressGlobalCleanupForDifferentStaleVnode(t *testing.T) {
	store := metrix.NewCollectorStore()
	failCollect := false
	mod := &mockModuleV2{
		store:    store,
		template: chartTemplateV2(),
		collectFunc: func(context.Context) error {
			if failCollect {
				return errors.New("collect failed")
			}
			store.Write().SnapshotMeter("apache").Gauge("workers_busy").Observe(1)
			return nil
		},
	}

	var out bytes.Buffer
	job := newTestJobV2(mod, &out)
	require.NoError(t, job.AutoDetection())

	job.runOnce()
	out.Reset()

	failCollect = true
	job.UpdateVnode(&vnodes.VirtualNode{
		Hostname: "node-host-b",
		GUID:     "node-guid-b",
		Labels: map[string]string{
			"_node_stale_after_seconds": "60",
		},
	})
	job.runOnce()
	assert.Equal(t, "", out.String())

	job.Cleanup()

	wire := out.String()
	assert.Contains(t, wire, fmt.Sprintf(`HOST ''

CHART 'module_job.workers_busy' '' 'Workers Busy' 'workers' 'Workers' 'workers_busy' 'line' '%d' '1' 'obsolete' 'plugin' 'module'`, chartengine.Priority))
	assert.NotContains(t, wire, "HOST 'node-guid-b'")
}

func TestJobV2CleanupUsesPreModuleCleanupSnapshotForStaleSuppression(t *testing.T) {
	store := metrix.NewCollectorStore()
	modVnode := &vnodes.VirtualNode{
		Hostname: "node-host-a",
		GUID:     "node-guid-a",
	}
	mod := &mockModuleV2{
		store:    store,
		template: chartTemplateV2(),
		vnode:    modVnode,
		collectFunc: func(context.Context) error {
			store.Write().SnapshotMeter("apache").Gauge("workers_busy").Observe(1)
			return nil
		},
	}
	mod.cleanupFunc = func(context.Context) {
		*modVnode = vnodes.VirtualNode{
			Hostname: "node-host-b",
			GUID:     "node-guid-b",
		}
	}

	var out bytes.Buffer
	job := newTestJobV2WithVnode(mod, &out, *modVnode.Copy())
	require.NoError(t, job.AutoDetection())

	job.runOnce()
	out.Reset()

	modVnode.Labels = map[string]string{
		"_node_stale_after_seconds": "60",
	}

	job.Cleanup()

	assert.Equal(t, "", out.String())
	assert.True(t, mod.cleaned)
}

func TestJobV2CleanupDoesNotSuppressExplicitScopeForStaleJobVnode(t *testing.T) {
	mod := &mockModuleV2{
		store:    metrix.NewCollectorStore(),
		template: chartTemplateV2(),
	}

	var out bytes.Buffer
	job := newTestJobV2WithVnode(mod, &out, vnodes.VirtualNode{
		Hostname: "node-host",
		GUID:     "node-guid",
		Labels: map[string]string{
			"_node_stale_after_seconds": "60",
		},
	})
	require.NoError(t, job.AutoDetection())

	job.scopeStates = map[string]*jobV2ScopeState{
		"scope-a": {
			scopeKey: "scope-a",
			scope: metrix.HostScope{
				ScopeKey: "scope-a",
				GUID:     "node-guid",
				Hostname: "scoped-host",
			},
			host: jobV2HostState{
				cleanupOwner: jobV2HostRef{kind: jobV2HostVnode, guid: "node-guid"},
				cleanupCharts: map[string]chartengine.ChartMeta{
					"workers_busy": {
						Title:    "Workers Busy",
						Family:   "Workers",
						Context:  "workers_busy",
						Units:    "workers",
						Type:     chartengine.ChartTypeLine,
						Priority: chartengine.Priority,
					},
				},
			},
		},
	}

	job.Cleanup()

	assert.Contains(t, out.String(), `HOST 'node-guid'`)
	assert.Contains(t, out.String(), "obsolete")
}

func TestJobV2CleanupNoSuccessfulEmissionsIsNoOp(t *testing.T) {
	mod := &mockModuleV2{
		store:    metrix.NewCollectorStore(),
		template: chartTemplateV2(),
	}

	var out bytes.Buffer
	job := newTestJobV2(mod, &out)
	require.NoError(t, job.AutoDetection())

	job.Cleanup()

	assert.Equal(t, "", out.String())
	assert.True(t, mod.cleaned)
}

func TestJobV2CleanupTrackerUsesEffectiveEmittedChartSet(t *testing.T) {
	meta := chartengine.ChartMeta{
		Title:   "Workers Busy",
		Family:  "Workers",
		Context: "workers_busy",
		Units:   "workers",
		Type:    chartengine.ChartTypeLine,
	}

	job := &JobV2{scopeStates: map[string]*jobV2ScopeState{
		defaultHostScopeKey: {scopeKey: defaultHostScopeKey},
	}}
	state := requireDefaultScopeState(t, job)
	decision := jobV2EmissionDecision{targetHost: jobV2HostRef{kind: jobV2HostGlobal}}
	state.host.commitSuccessfulEmission(chartengine.Plan{
		Actions: []chartengine.EngineAction{
			chartengine.CreateDimensionAction{
				ChartID:   "workers_busy",
				ChartMeta: meta,
				Name:      "busy",
			},
		},
	}, decision)

	require.Len(t, state.host.cleanupCharts, 1)
	assert.Equal(t, meta, state.host.cleanupCharts["workers_busy"])
	assert.Equal(t, jobV2HostRef{kind: jobV2HostGlobal}, state.host.cleanupOwner)

	state.host.commitSuccessfulEmission(chartengine.Plan{
		Actions: []chartengine.EngineAction{
			chartengine.RemoveChartAction{
				ChartID: "workers_busy",
				Meta:    meta,
			},
		},
	}, decision)

	assert.Empty(t, state.host.cleanupCharts)
}

func TestJobV2StopBeforeStartDoesNotBlock(t *testing.T) {
	job := NewJobV2(JobV2Config{
		PluginName:      pluginName,
		Name:            jobName,
		ModuleName:      modName,
		FullName:        modName + "_" + jobName,
		Out:             &bytes.Buffer{},
		UpdateEvery:     1,
		AutoDetectEvery: 1,
	})

	done := make(chan struct{})
	go func() {
		job.Stop()
		close(done)
	}()

	select {
	case <-done:
	case <-time.After(200 * time.Millisecond):
		t.Fatal("stop blocked before start")
	}
}
