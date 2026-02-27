// SPDX-License-Identifier: GPL-3.0-or-later

package jobruntime

import (
	"bytes"
	"context"
	"errors"
	"testing"
	"time"

	"github.com/netdata/netdata/go/plugins/plugin/framework/collectorapi"
	"github.com/netdata/netdata/go/plugins/plugin/framework/runtimecomp"
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

	store    metrix.CollectorStore
	template string
	cleaned  bool
	vnode    *vnodes.VirtualNode
}

type mockRuntimeComponentService struct {
	registerErr  error
	registered   []runtimecomp.ComponentConfig
	unregistered []string
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
func (m *mockModuleV2) ChartTemplateYAML() string          { return m.template }

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
				require.NotNil(t, job.engine)
				_, err := job.engine.BuildPlan(job.store.Read(metrix.ReadFlatten()))
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
				assert.Contains(t, wire, "CHART 'module_job.workers_busy'")
				assert.Contains(t, wire, "CLABEL '_collect_job' 'job' '1'")
				assert.Contains(t, wire, "BEGIN 'module_job.workers_busy'")
				assert.Contains(t, wire, "SET 'busy' = 7")
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
				assert.Contains(t, wire, "CHART 'module_job.win_nic_traffic_eth0'")
				assert.Contains(t, wire, "CHART 'module_job.win_nic_traffic_eth1'")
				assert.Contains(t, wire, "BEGIN 'module_job.win_nic_traffic_eth0'")
				assert.Contains(t, wire, "SET 'received' = 100")
				assert.Contains(t, wire, "SET 'sent' = 80")
				assert.Contains(t, wire, "BEGIN 'module_job.win_nic_traffic_eth1'")
				assert.Contains(t, wire, "SET 'received' = 50")
				assert.Contains(t, wire, "SET 'sent' = 40")
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
				assert.Equal(t, job.engine.RuntimeStore(), cfg.Store)
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

				for i := 0; i < 3; i++ {
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
