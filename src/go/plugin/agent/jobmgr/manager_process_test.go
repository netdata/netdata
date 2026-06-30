// SPDX-License-Identifier: GPL-3.0-or-later

package jobmgr

import (
	"bytes"
	"context"
	"fmt"
	"slices"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/netdata/netdata/go/plugins/pkg/funcapi"
	"github.com/netdata/netdata/go/plugins/plugin/agent/secrets/secretstore"
	"github.com/netdata/netdata/go/plugins/plugin/framework/collectorapi"
	"github.com/netdata/netdata/go/plugins/plugin/framework/confgroup"
	"github.com/netdata/netdata/go/plugins/plugin/framework/dyncfg"
	"github.com/netdata/netdata/go/plugins/plugin/framework/functions"
	"github.com/netdata/netdata/go/plugins/plugin/framework/vnodes"
)

func TestRunProcessConfGroups_ChannelCloseDoesNotSpin(t *testing.T) {
	tests := map[string]struct {
		closeInput   bool
		cancelBefore bool
		wantExit     bool
	}{
		"closed channel exits loop": {
			closeInput: true,
			wantExit:   true,
		},
		"canceled context exits loop": {
			cancelBefore: true,
			wantExit:     true,
		},
	}

	for name, tc := range tests {
		t.Run(name, func(t *testing.T) {
			mgr := New(Config{PluginName: testPluginName})
			ctx, cancel := context.WithCancel(context.Background())
			t.Cleanup(cancel)
			mgr.ctx = ctx

			in := make(chan []*confgroup.Group)
			if tc.closeInput {
				close(in)
			}
			if tc.cancelBefore {
				cancel()
			}

			done := make(chan struct{})
			go func() {
				mgr.runProcessConfGroups(in)
				close(done)
			}()

			select {
			case <-done:
				assert.True(t, tc.wantExit)
			case <-time.After(200 * time.Millisecond):
				t.Fatal("runProcessConfGroups did not exit")
			}
		})
	}
}

func TestRunNotifyRunningJobs_TickOutsideLock(t *testing.T) {
	mgr := New(Config{PluginName: testPluginName})

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	mgr.ctx = ctx

	job := &lockProbeJob{
		fullName:    "success_lockprobe",
		moduleName:  "success",
		name:        "lockprobe",
		tickStarted: make(chan struct{}),
		tickRelease: make(chan struct{}),
	}

	mgr.runningJobs.lock()
	mgr.runningJobs.add(job.FullName(), job)
	mgr.runningJobs.unlock()

	done := make(chan struct{})
	go func() {
		mgr.runNotifyRunningJobs()
		close(done)
	}()

	select {
	case <-job.tickStarted:
	case <-time.After(2 * time.Second):
		t.Fatal("tick did not start")
	}

	stopDone := make(chan struct{})
	go func() {
		mgr.stopRunningJob(job.FullName())
		close(stopDone)
	}()

	select {
	case <-stopDone:
	case <-time.After(300 * time.Millisecond):
		t.Fatal("stopRunningJob blocked while Tick was in progress")
	}

	close(job.tickRelease)
	cancel()

	select {
	case <-done:
	case <-time.After(2 * time.Second):
		t.Fatal("runNotifyRunningJobs did not stop")
	}
}

func TestRunNotifyRunningJobs_ReconcilesLateAvailableModuleMethods(t *testing.T) {
	reg := &recordingFunctionRegistry{}
	var out bytes.Buffer
	var available atomic.Bool
	availability := managerFunctionAvailability{fn: func(string) bool { return available.Load() }}
	mgr := New(Config{
		PluginName: testPluginName,
		Out:        &out,
		FnReg:      reg,
		Modules: collectorapi.Registry{
			"mod": collectorapi.Creator{
				SharedFunctions: func() []funcapi.FunctionConfig {
					return []funcapi.FunctionConfig{{ID: "late"}}
				},
			},
		},
	})

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	mgr.ctx = ctx
	mgr.funcCtl.Init(ctx)
	mgr.funcCtl.RegisterModules(mgr.modules)
	reconcileDone := make(chan struct{})
	go func() {
		mgr.runFunctionReconciler()
		close(reconcileDone)
	}()

	job := &tickProbeJob{
		fullName:   "mod_job1",
		moduleName: "mod",
		name:       "job1",
		collector:  availability,
	}
	mgr.funcCtl.OnJobStart(job)

	mgr.runningJobs.lock()
	mgr.runningJobs.add(job.FullName(), job)
	mgr.runningJobs.unlock()

	available.Store(true)
	done := make(chan struct{})
	go func() {
		mgr.runNotifyRunningJobs()
		close(done)
	}()

	require.Eventually(t, func() bool {
		return slices.Contains(reg.registeredNames(), "mod:late")
	}, 2*time.Second, 10*time.Millisecond)

	cancel()
	select {
	case <-reconcileDone:
	case <-time.After(2 * time.Second):
		t.Fatal("runFunctionReconciler did not stop")
	}
	assert.NotContains(t, out.String(), "FUNCTION_DEL")
	select {
	case <-done:
	case <-time.After(2 * time.Second):
		t.Fatal("runNotifyRunningJobs did not stop")
	}
}

func TestRunNotifyRunningJobs_DeduplicatesFunctionReconcileByModule(t *testing.T) {
	reg := &recordingFunctionRegistry{}
	var availableCalls atomic.Int32
	var available atomic.Bool
	availability := managerFunctionAvailability{fn: func(string) bool {
		if available.Load() {
			availableCalls.Add(1)
		}
		return available.Load()
	}}
	mgr := New(Config{
		PluginName: testPluginName,
		FnReg:      reg,
		Modules: collectorapi.Registry{
			"mod": collectorapi.Creator{
				SharedFunctions: func() []funcapi.FunctionConfig {
					return []funcapi.FunctionConfig{{ID: "late"}}
				},
			},
		},
	})

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	mgr.ctx = ctx
	mgr.funcCtl.Init(ctx)
	mgr.funcCtl.RegisterModules(mgr.modules)
	reconcileDone := make(chan struct{})
	go func() {
		mgr.runFunctionReconciler()
		close(reconcileDone)
	}()

	job1 := &tickProbeJob{fullName: "mod_job1", moduleName: "mod", name: "job1", collector: availability}
	job2 := &tickProbeJob{fullName: "mod_job2", moduleName: "mod", name: "job2", collector: availability}
	mgr.funcCtl.OnJobStart(job1)
	mgr.funcCtl.OnJobStart(job2)
	mgr.runningJobs.lock()
	mgr.runningJobs.add(job1.FullName(), job1)
	mgr.runningJobs.add(job2.FullName(), job2)
	mgr.runningJobs.unlock()

	availableCalls.Store(0)
	available.Store(true)
	notifyDone := make(chan struct{})
	go func() {
		mgr.runNotifyRunningJobs()
		close(notifyDone)
	}()

	require.Eventually(t, func() bool {
		return slices.Contains(reg.registeredNames(), "mod:late")
	}, 2*time.Second, 10*time.Millisecond)
	assert.Equal(t, int32(2), availableCalls.Load())

	cancel()
	select {
	case <-reconcileDone:
	case <-time.After(2 * time.Second):
		t.Fatal("runFunctionReconciler did not stop")
	}
	select {
	case <-notifyDone:
	case <-time.After(2 * time.Second):
		t.Fatal("runNotifyRunningJobs did not stop")
	}
}

func TestRunNotifyRunningJobs_DoesNotPublishDirectly(t *testing.T) {
	reg := &recordingFunctionRegistry{}
	var available atomic.Bool
	availability := managerFunctionAvailability{fn: func(string) bool { return available.Load() }}
	mgr := New(Config{
		PluginName: testPluginName,
		FnReg:      reg,
		Modules: collectorapi.Registry{
			"mod": collectorapi.Creator{
				SharedFunctions: func() []funcapi.FunctionConfig {
					return []funcapi.FunctionConfig{{ID: "late"}}
				},
			},
		},
	})

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	mgr.ctx = ctx
	mgr.funcCtl.Init(ctx)
	mgr.funcCtl.RegisterModules(mgr.modules)

	job := &lockProbeJob{
		fullName:    "mod_job1",
		moduleName:  "mod",
		name:        "job1",
		collector:   availability,
		tickStarted: make(chan struct{}),
		tickRelease: make(chan struct{}),
	}
	mgr.funcCtl.OnJobStart(job)
	mgr.runningJobs.lock()
	mgr.runningJobs.add(job.FullName(), job)
	mgr.runningJobs.unlock()

	done := make(chan struct{})
	go func() {
		mgr.runNotifyRunningJobs()
		close(done)
	}()

	select {
	case <-job.tickStarted:
	case <-time.After(2 * time.Second):
		t.Fatal("tick did not start")
	}
	available.Store(true)
	close(job.tickRelease)

	time.Sleep(50 * time.Millisecond)
	assert.Empty(t, reg.registeredNames())

	cancel()
	select {
	case <-done:
	case <-time.After(2 * time.Second):
		t.Fatal("runNotifyRunningJobs did not stop")
	}
}

func TestRequestFunctionReconcile_CoalescesPendingModules(t *testing.T) {
	mgr := New(Config{PluginName: testPluginName})
	ctx := t.Context()
	mgr.ctx = ctx

	mgr.requestFunctionReconcile("bbb")
	mgr.requestFunctionReconcile("aaa")
	mgr.requestFunctionReconcile("aaa")

	select {
	case <-mgr.funcReconWake:
	case <-time.After(200 * time.Millisecond):
		t.Fatal("requestFunctionReconcile did not wake reconciler")
	}

	assert.Equal(t, []string{"aaa", "bbb"}, mgr.takePendingFunctionReconcileModules())
	assert.Empty(t, mgr.takePendingFunctionReconcileModules())

	mgr.requestFunctionReconcile("mod")
	select {
	case <-mgr.funcReconWake:
	case <-time.After(200 * time.Millisecond):
		t.Fatal("requestFunctionReconcile did not wake reconciler")
	}

	requestDone := make(chan struct{})
	go func() {
		for range 100 {
			mgr.requestFunctionReconcile("mod")
		}
		close(requestDone)
	}()

	select {
	case <-requestDone:
	case <-time.After(200 * time.Millisecond):
		t.Fatal("requestFunctionReconcile blocked while wake was pending")
	}

	assert.Equal(t, []string{"mod"}, mgr.takePendingFunctionReconcileModules())
}

func TestRunWaitDecisionStep(t *testing.T) {
	tests := map[string]struct {
		run func(t *testing.T)
	}{
		"executes dyncfg command while waiting": {
			run: func(t *testing.T) {
				var out bytes.Buffer
				mgr := New(Config{PluginName: testPluginName, Out: &out})
				ctx, cancel := context.WithCancel(context.Background())
				defer cancel()
				mgr.ctx = ctx
				mgr.collectorHandler.WaitForDecision(prepareUserCfg("mod", "job"))
				mgr.dyncfgCh <- dyncfg.NewFunction(functions.Function{
					UID:  "unknown",
					Name: "config",
					Args: []string{"unknown", string(dyncfg.CommandSchema)},
				})

				require.True(t, mgr.runWaitDecisionStep())
				assert.Contains(t, out.String(), "unknown function")
			},
		},
		"expires wait decision": {
			run: func(t *testing.T) {
				mgr := New(Config{PluginName: testPluginName})
				ctx, cancel := context.WithCancel(context.Background())
				defer cancel()
				mgr.ctx = ctx
				mgr.collectorHandler = newTestCollectorHandlerWithWaitTimeout(mgr, time.Nanosecond)
				mgr.collectorHandler.WaitForDecision(prepareUserCfg("mod", "job"))

				require.True(t, mgr.runWaitDecisionStep())
				assert.False(t, mgr.collectorHandler.WaitingForDecision())
			},
		},
		"context cancel returns false": {
			run: func(t *testing.T) {
				mgr := New(Config{PluginName: testPluginName})
				ctx, cancel := context.WithCancel(context.Background())
				mgr.ctx = ctx
				mgr.collectorHandler.WaitForDecision(prepareUserCfg("mod", "job"))
				cancel()

				assert.False(t, mgr.runWaitDecisionStep())
			},
		},
	}

	for name, tc := range tests {
		t.Run(name, tc.run)
	}
}

func TestRunFunctionReconciler_DoesNotBlockManagerControlLoop(t *testing.T) {
	reg := &recordingFunctionRegistry{}
	var out simOutput
	availabilityStarted := make(chan struct{})
	availabilityRelease := make(chan struct{})
	availability := managerFunctionAvailability{fn: func(string) bool {
		close(availabilityStarted)
		<-availabilityRelease
		return true
	}}
	mgr := New(Config{
		PluginName: testPluginName,
		Out:        &out,
		FnReg:      reg,
		Modules: collectorapi.Registry{
			"mod": collectorapi.Creator{
				SharedFunctions: func() []funcapi.FunctionConfig {
					return []funcapi.FunctionConfig{{ID: "late"}}
				},
			},
		},
	})

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	mgr.ctx = ctx
	mgr.funcCtl.Init(ctx)
	mgr.funcCtl.RegisterModules(mgr.modules)
	mgr.funcCtl.OnJobStart(&tickProbeJob{fullName: "mod_job1", moduleName: "mod", name: "job1", collector: availability})

	reconcileDone := make(chan struct{})
	go func() {
		mgr.runFunctionReconciler()
		close(reconcileDone)
	}()
	mgr.requestFunctionReconcile("mod")

	select {
	case <-availabilityStarted:
	case <-time.After(2 * time.Second):
		t.Fatal("function availability check did not start")
	}

	runDone := make(chan struct{})
	go func() {
		mgr.run()
		close(runDone)
	}()
	mgr.dyncfgCh <- dyncfg.NewFunction(functions.Function{
		UID:  "unknown",
		Name: "config",
		Args: []string{"unknown", string(dyncfg.CommandSchema)},
	})

	require.Eventually(t, func() bool {
		return strings.Contains(out.String(), "unknown function")
	}, 2*time.Second, 10*time.Millisecond)

	close(availabilityRelease)
	cancel()
	select {
	case <-reconcileDone:
	case <-time.After(2 * time.Second):
		t.Fatal("runFunctionReconciler did not stop")
	}
	select {
	case <-runDone:
	case <-time.After(2 * time.Second):
		t.Fatal("run did not stop")
	}
}

func TestFunctionReconcileFunnelConcurrentLifecycle(t *testing.T) {
	reg := &recordingFunctionRegistry{}
	var available atomic.Bool
	availability := managerFunctionAvailability{fn: func(string) bool { return available.Load() }}
	mgr := New(Config{
		PluginName: testPluginName,
		FnReg:      reg,
		Modules: collectorapi.Registry{
			"mod": collectorapi.Creator{
				SharedFunctions: func() []funcapi.FunctionConfig {
					return []funcapi.FunctionConfig{{ID: "late"}}
				},
			},
		},
	})

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	mgr.ctx = ctx
	mgr.funcCtl.Init(ctx)
	mgr.funcCtl.RegisterModules(mgr.modules)

	reconcileDone := make(chan struct{})
	go func() {
		mgr.runFunctionReconciler()
		close(reconcileDone)
	}()
	notifyDone := make(chan struct{})
	go func() {
		mgr.runNotifyRunningJobs()
		close(notifyDone)
	}()

	stable := &tickProbeJob{fullName: "mod_stable", moduleName: "mod", name: "stable", collector: availability}
	mgr.startRunningJob(stable)
	available.Store(true)

	var wg sync.WaitGroup
	for worker := range 4 {
		wg.Add(1)
		go func(worker int) {
			defer wg.Done()
			for i := range 20 {
				name := fmt.Sprintf("churn-%d-%d", worker, i)
				fullName := fmt.Sprintf("mod_%s", name)
				job := &tickProbeJob{fullName: fullName, moduleName: "mod", name: name, collector: availability}
				mgr.startRunningJob(job)
				mgr.stopRunningJob(fullName)
			}
		}(worker)
	}

	require.Eventually(t, func() bool {
		return slices.Contains(reg.registeredNames(), "mod:late")
	}, 3*time.Second, 10*time.Millisecond)
	wg.Wait()

	cancel()
	select {
	case <-reconcileDone:
	case <-time.After(2 * time.Second):
		t.Fatal("runFunctionReconciler did not stop")
	}
	select {
	case <-notifyDone:
	case <-time.After(2 * time.Second):
		t.Fatal("runNotifyRunningJobs did not stop")
	}
}

func TestFunctionReconcileFunnelWithdrawsStoppedInstanceAfterInFlightPublish(t *testing.T) {
	reg := &recordingFunctionRegistry{}
	availabilityEntered := make(chan struct{})
	releaseAvailability := make(chan struct{})
	var enterOnce sync.Once
	availability := managerFunctionAvailability{fn: func(string) bool {
		enterOnce.Do(func() { close(availabilityEntered) })
		<-releaseAvailability
		return true
	}}
	mgr := New(Config{
		PluginName: testPluginName,
		FnReg:      reg,
		Modules: collectorapi.Registry{
			"mod": collectorapi.Creator{
				InstanceFunctions: func(collectorapi.RuntimeJob) []funcapi.FunctionConfig {
					return []funcapi.FunctionConfig{{ID: "details"}}
				},
			},
		},
	})

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	mgr.ctx = ctx
	mgr.funcCtl.Init(ctx)
	mgr.funcCtl.RegisterModules(mgr.modules)

	reconcileDone := make(chan struct{})
	go func() {
		mgr.runFunctionReconciler()
		close(reconcileDone)
	}()

	job := &tickProbeJob{
		fullName:   "mod_job1",
		moduleName: "mod",
		name:       "job1",
		collector:  availability,
	}
	mgr.startRunningJob(job)
	mgr.requestFunctionReconcile("mod")

	select {
	case <-availabilityEntered:
	case <-time.After(2 * time.Second):
		t.Fatal("reconcile did not check instance Function availability")
	}

	mgr.stopRunningJob(job.FullName())
	close(releaseAvailability)

	require.Eventually(t, func() bool {
		return slices.Contains(reg.registeredNames(), "mod:details")
	}, 2*time.Second, 10*time.Millisecond)
	require.Eventually(t, func() bool {
		return slices.Contains(reg.unregisteredNames(), "mod:details")
	}, 2*time.Second, 10*time.Millisecond)

	cancel()
	select {
	case <-reconcileDone:
	case <-time.After(2 * time.Second):
		t.Fatal("runFunctionReconciler did not stop")
	}
}

func TestManagerAddConfigSingleInstancePolicy(t *testing.T) {
	tests := map[string]struct {
		run func(t *testing.T, mgr *Manager)
	}{
		"non-canonical config is rejected before exposure": {
			run: func(t *testing.T, mgr *Manager) {
				cfg := prepareUserCfg("single", "custom")

				mgr.addConfig(cfg)

				assert.Equal(t, 0, mgr.collectorSeen.Count())
				_, exposed := mgr.collectorExposed.LookupByKey(cfg.FullName())
				assert.False(t, exposed)
			},
		},
		"defaulted empty name is accepted as canonical": {
			run: func(t *testing.T, mgr *Manager) {
				cfg := prepareUserCfg("single", "")
				cfg.ApplyDefaults(confgroup.Default{})

				mgr.addConfig(cfg)

				entry, ok := mgr.collectorExposed.LookupByKey("single")
				require.True(t, ok)
				assert.Equal(t, "single", entry.Cfg.Name())
			},
		},
		"canonical higher-priority config replaces lower-priority config": {
			run: func(t *testing.T, mgr *Manager) {
				stockCfg := prepareStockCfg("single", "single")
				userCfg := prepareUserCfg("single", "single")

				mgr.addConfig(stockCfg)
				entry, ok := mgr.collectorExposed.LookupByKey("single")
				require.True(t, ok)
				assert.Equal(t, confgroup.TypeStock, entry.Cfg.SourceType())

				mgr.addConfig(userCfg)
				entry, ok = mgr.collectorExposed.LookupByKey("single")
				require.True(t, ok)
				assert.Equal(t, confgroup.TypeUser, entry.Cfg.SourceType())
			},
		},
	}

	for name, tc := range tests {
		t.Run(name, func(t *testing.T) {
			mgr := New(Config{PluginName: testPluginName})
			mgr.modules = collectorapi.Registry{
				"single": {
					InstancePolicy: collectorapi.InstancePolicySingle,
					Create:         func() collectorapi.CollectorV1 { return &collectorapi.MockCollectorV1{} },
				},
			}

			tc.run(t, mgr)
		})
	}
}

func TestManagerAddConfigSingleInstancePolicyPublishesDyncfgSingle(t *testing.T) {
	var buf bytes.Buffer
	mgr := New(Config{
		PluginName: testPluginName,
		Out:        &buf,
		Modules: collectorapi.Registry{
			"single": {
				InstancePolicy: collectorapi.InstancePolicySingle,
				Create:         func() collectorapi.CollectorV1 { return &collectorapi.MockCollectorV1{} },
			},
		},
	})

	cfg := prepareUserCfg("single", "single")
	mgr.addConfig(cfg)

	out := buf.String()
	assert.Contains(t, out, "CONFIG "+mgr.dyncfgModID("single")+" create accepted single /collectors/"+testPluginName+"/Jobs")
	assert.Contains(t, out, "'schema get enable disable update restart test userconfig'")
	assert.NotContains(t, out, " remove")
}

func TestRun_DoesNotRegisterJobBoundModuleMethodsBeforeAnyJobStarts(t *testing.T) {
	fnReg := &recordingFunctionRegistry{}
	mgr := New(Config{PluginName: testPluginName, FnReg: fnReg})

	mgr.modules = collectorapi.Registry{
		"mod": collectorapi.Creator{
			SharedFunctions: func() []funcapi.FunctionConfig {
				return []funcapi.FunctionConfig{{ID: "a"}}
			},
		},
	}

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	in := make(chan []*confgroup.Group)
	done := make(chan struct{})
	go func() {
		mgr.Run(ctx, in)
		close(done)
	}()

	waitCtx, waitCancel := context.WithTimeout(context.Background(), time.Second)
	defer waitCancel()
	require.True(t, mgr.WaitStarted(waitCtx), "manager did not report started")

	cancel()
	close(in)

	select {
	case <-done:
	case <-time.After(2 * time.Second):
		t.Fatal("manager did not stop after cancel")
	}

	assert.Empty(t, fnReg.registeredNames(), "static methods must not be registered before first started job")
}

func TestRun_RegistersAgentScopeModuleMethodsBeforeAnyJobStarts(t *testing.T) {
	fnReg := &recordingFunctionRegistry{}
	mgr := New(Config{PluginName: testPluginName, FnReg: fnReg})

	mgr.modules = collectorapi.Registry{
		"mod": collectorapi.Creator{
			AgentFunctions: func() []funcapi.FunctionConfig {
				return []funcapi.FunctionConfig{{ID: "a"}}
			},
		},
	}

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	in := make(chan []*confgroup.Group)
	done := make(chan struct{})
	go func() {
		mgr.Run(ctx, in)
		close(done)
	}()

	waitCtx, waitCancel := context.WithTimeout(context.Background(), time.Second)
	defer waitCancel()
	require.True(t, mgr.WaitStarted(waitCtx), "manager did not report started")

	cancel()
	close(in)

	select {
	case <-done:
	case <-time.After(2 * time.Second):
		t.Fatal("manager did not stop after cancel")
	}

	assert.Equal(t, []string{"mod:a"}, fnReg.registeredNames())
}

func TestStartRunningJob_RegistersModuleMethodsAfterReconcile(t *testing.T) {
	fnReg := &recordingFunctionRegistry{}
	mgr := New(Config{PluginName: testPluginName, FnReg: fnReg})
	creator := collectorapi.Creator{
		SharedFunctions: func() []funcapi.FunctionConfig {
			return []funcapi.FunctionConfig{{ID: "a"}, {ID: "b"}}
		},
	}
	mgr.modules = collectorapi.Registry{"mod": creator}
	mgr.funcCtl.RegisterModules(mgr.modules)

	job := &lockProbeJob{fullName: "mod_job1", moduleName: "mod", name: "job1"}
	mgr.startRunningJob(job)

	assert.Empty(t, fnReg.registeredNames())

	mgr.funcCtl.ReconcileModuleMethods("mod")

	assert.ElementsMatch(t, []string{"mod:a", "mod:b"}, fnReg.registeredNames())
}

func TestStartRunningJob_ReconcileDoesNotReregisterModuleMethods(t *testing.T) {
	fnReg := &recordingFunctionRegistry{}
	mgr := New(Config{PluginName: testPluginName, FnReg: fnReg})
	creator := collectorapi.Creator{
		SharedFunctions: func() []funcapi.FunctionConfig {
			return []funcapi.FunctionConfig{{ID: "a"}, {ID: "b"}}
		},
	}
	mgr.modules = collectorapi.Registry{"mod": creator}
	mgr.funcCtl.RegisterModules(mgr.modules)

	job1 := &lockProbeJob{fullName: "mod_job1", moduleName: "mod", name: "job1"}
	mgr.startRunningJob(job1)
	mgr.funcCtl.ReconcileModuleMethods("mod")

	job2 := &lockProbeJob{fullName: "mod_job2", moduleName: "mod", name: "job2"}
	mgr.startRunningJob(job2)
	mgr.funcCtl.ReconcileModuleMethods("mod")

	registered := fnReg.registeredNames()
	assert.Len(t, registered, 2)
	assert.ElementsMatch(t, []string{"mod:a", "mod:b"}, registered)
}

func TestRun_RegistersDyncfgConfigPrefixes(t *testing.T) {
	fnReg := &recordingFunctionRegistry{}
	mgr := New(Config{PluginName: testPluginName, FnReg: fnReg})

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	in := make(chan []*confgroup.Group)
	done := make(chan struct{})
	go func() {
		mgr.Run(ctx, in)
		close(done)
	}()

	waitCtx, waitCancel := context.WithTimeout(context.Background(), time.Second)
	defer waitCancel()
	require.True(t, mgr.WaitStarted(waitCtx), "manager did not report started")

	cancel()
	close(in)

	select {
	case <-done:
	case <-time.After(2 * time.Second):
		t.Fatal("manager did not stop after cancel")
	}

	assert.Equal(t, []registeredPrefix{
		{name: "config", prefix: mgr.dyncfgVnodePrefixValue()},
		{name: "config", prefix: mgr.dyncfgSecretStorePrefixValue()},
		{name: "config", prefix: mgr.dyncfgCollectorPrefixValue()},
	}, fnReg.registeredPrefixes())
}

func TestRun_PublishesVnodesAndSecretstoresBeforeCollectorTemplates(t *testing.T) {
	fnReg := &recordingFunctionRegistry{}
	var buf bytes.Buffer

	mgr := New(Config{
		PluginName: testPluginName,
		FnReg:      fnReg,
		Out:        &buf,
		Modules: collectorapi.Registry{
			"mod": collectorapi.Creator{},
		},
	})

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	in := make(chan []*confgroup.Group)
	done := make(chan struct{})
	go func() {
		mgr.Run(ctx, in)
		close(done)
	}()

	waitCtx, waitCancel := context.WithTimeout(context.Background(), time.Second)
	defer waitCancel()
	require.True(t, mgr.WaitStarted(waitCtx), "manager did not report started")

	cancel()
	close(in)

	select {
	case <-done:
	case <-time.After(2 * time.Second):
		t.Fatal("manager did not stop after cancel")
	}

	output := buf.String()

	vnodeIdx := strings.Index(output, "CONFIG "+mgr.dyncfgVnodePrefixValue()+" create accepted template /collectors/"+testPluginName+"/Vnodes")
	secretIdx := strings.Index(output, "CONFIG "+mgr.dyncfgSecretStoreID(string(secretstore.KindVault))+" create accepted template /collectors/"+testPluginName+"/SecretStores")
	collectorIdx := strings.Index(output, "CONFIG "+mgr.dyncfgModID("mod")+" create accepted template /collectors/"+testPluginName+"/Jobs")

	require.NotEqual(t, -1, vnodeIdx, "vnode template publication not found")
	require.NotEqual(t, -1, secretIdx, "secretstore template publication not found")
	require.NotEqual(t, -1, collectorIdx, "collector template publication not found")
	assert.Less(t, vnodeIdx, collectorIdx, "vnode publication must happen before collector template publication")
	assert.Less(t, secretIdx, collectorIdx, "secretstore publication must happen before collector template publication")
}

func TestRun_DoesNotPublishCollectorTemplateForSingleInstanceModules(t *testing.T) {
	var buf bytes.Buffer

	mgr := New(Config{
		PluginName: testPluginName,
		Out:        &buf,
		Modules: collectorapi.Registry{
			"single": collectorapi.Creator{
				InstancePolicy: collectorapi.InstancePolicySingle,
			},
			"perjob": collectorapi.Creator{},
		},
	})

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	in := make(chan []*confgroup.Group)
	done := make(chan struct{})
	go func() {
		mgr.Run(ctx, in)
		close(done)
	}()

	waitCtx, waitCancel := context.WithTimeout(context.Background(), time.Second)
	defer waitCancel()
	require.True(t, mgr.WaitStarted(waitCtx), "manager did not report started")

	cancel()
	close(in)

	select {
	case <-done:
	case <-time.After(2 * time.Second):
		t.Fatal("manager did not stop after cancel")
	}

	output := buf.String()
	assert.Contains(t, output, "CONFIG "+mgr.dyncfgModID("perjob")+" create accepted template /collectors/"+testPluginName+"/Jobs")
	assert.NotContains(t, output, "CONFIG "+mgr.dyncfgModID("single")+" create accepted template /collectors/"+testPluginName+"/Jobs")
}

type lockProbeJob struct {
	fullName   string
	moduleName string
	name       string
	collector  any

	tickOnce    sync.Once
	stopOnce    sync.Once
	tickStarted chan struct{}
	tickRelease chan struct{}
}

func (j *lockProbeJob) FullName() string   { return j.fullName }
func (j *lockProbeJob) ModuleName() string { return j.moduleName }
func (j *lockProbeJob) Name() string       { return j.name }
func (j *lockProbeJob) Collector() any     { return j.collector }
func (j *lockProbeJob) Start()             {}
func (j *lockProbeJob) Stop()              { j.stopOnce.Do(func() {}) }
func (j *lockProbeJob) Tick(_ int) {
	j.tickOnce.Do(func() {
		close(j.tickStarted)
		<-j.tickRelease
	})
}
func (j *lockProbeJob) AutoDetection() error              { return nil }
func (j *lockProbeJob) AutoDetectionEvery() int           { return 0 }
func (j *lockProbeJob) RetryAutoDetection() bool          { return false }
func (j *lockProbeJob) Cleanup()                          {}
func (j *lockProbeJob) IsRunning() bool                   { return true }
func (j *lockProbeJob) Panicked() bool                    { return false }
func (j *lockProbeJob) Vnode() vnodes.VirtualNode         { return vnodes.VirtualNode{} }
func (j *lockProbeJob) UpdateVnode(_ *vnodes.VirtualNode) {}

type tickProbeJob struct {
	fullName   string
	moduleName string
	name       string
	collector  any
}

func (j *tickProbeJob) FullName() string                  { return j.fullName }
func (j *tickProbeJob) ModuleName() string                { return j.moduleName }
func (j *tickProbeJob) Name() string                      { return j.name }
func (j *tickProbeJob) Collector() any                    { return j.collector }
func (j *tickProbeJob) Start()                            {}
func (j *tickProbeJob) Stop()                             {}
func (j *tickProbeJob) Tick(int)                          {}
func (j *tickProbeJob) AutoDetection() error              { return nil }
func (j *tickProbeJob) AutoDetectionEvery() int           { return 0 }
func (j *tickProbeJob) RetryAutoDetection() bool          { return false }
func (j *tickProbeJob) Cleanup()                          {}
func (j *tickProbeJob) IsRunning() bool                   { return true }
func (j *tickProbeJob) Panicked() bool                    { return false }
func (j *tickProbeJob) Vnode() vnodes.VirtualNode         { return vnodes.VirtualNode{} }
func (j *tickProbeJob) UpdateVnode(_ *vnodes.VirtualNode) {}

type managerFunctionAvailability struct {
	fn func(string) bool
}

func (a managerFunctionAvailability) FunctionAvailable(functionID string) bool {
	if a.fn == nil {
		return true
	}
	return a.fn(functionID)
}

type recordingFunctionRegistry struct {
	mu           sync.Mutex
	registered   []string
	unregistered []string
	prefixes     []registeredPrefix
}

func (r *recordingFunctionRegistry) Register(name string, _ func(functions.Function)) {
	r.mu.Lock()
	r.registered = append(r.registered, name)
	r.mu.Unlock()
}

func (r *recordingFunctionRegistry) Unregister(name string) {
	r.mu.Lock()
	r.unregistered = append(r.unregistered, name)
	r.mu.Unlock()
}
func (r *recordingFunctionRegistry) RegisterPrefix(name, prefix string, _ func(functions.Function)) {
	r.mu.Lock()
	r.prefixes = append(r.prefixes, registeredPrefix{name: name, prefix: prefix})
	r.mu.Unlock()
}
func (r *recordingFunctionRegistry) UnregisterPrefix(string, string) {}

func (r *recordingFunctionRegistry) registeredNames() []string {
	r.mu.Lock()
	defer r.mu.Unlock()
	out := make([]string, len(r.registered))
	copy(out, r.registered)
	return out
}

func (r *recordingFunctionRegistry) unregisteredNames() []string {
	r.mu.Lock()
	defer r.mu.Unlock()
	out := make([]string, len(r.unregistered))
	copy(out, r.unregistered)
	return out
}

func (r *recordingFunctionRegistry) registeredPrefixes() []registeredPrefix {
	r.mu.Lock()
	defer r.mu.Unlock()
	out := make([]registeredPrefix, len(r.prefixes))
	copy(out, r.prefixes)
	return out
}

func newTestCollectorHandlerWithWaitTimeout(mgr *Manager, timeout time.Duration) *dyncfg.Handler[confgroup.Config] {
	return dyncfg.NewHandler(dyncfg.HandlerOpts[confgroup.Config]{
		Logger:    mgr.Logger,
		API:       mgr.dyncfgResponder,
		Seen:      dyncfg.NewSeenCache[confgroup.Config](),
		Exposed:   dyncfg.NewExposedCache[confgroup.Config](),
		Callbacks: mgr.collectorCallbacks,
		WaitKey: func(cfg confgroup.Config) string {
			return cfg.FullName()
		},

		Path:                    fmt.Sprintf(dyncfgCollectorPath, testPluginName),
		EnableFailCode:          200,
		RemoveStockOnEnableFail: true,
		ConfigCommands:          dyncfgCollectorConfigCmds(),
		WaitTimeout:             timeout,
	})
}

type registeredPrefix struct {
	name   string
	prefix string
}
