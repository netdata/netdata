// SPDX-License-Identifier: GPL-3.0-or-later

package jobmgr

import (
	"context"
	"sync"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/netdata/netdata/go/plugins/pkg/funcapi"
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

func TestRun_WaitTimeoutClearsGateAndKeepsAccepted(t *testing.T) {
	mgr := New(Config{PluginName: testPluginName})
	mgr.modules = prepareMockRegistry()

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	mgr.ctx = ctx

	done := make(chan struct{})
	go func() {
		mgr.run()
		close(done)
	}()
	defer func() {
		cancel()
		select {
		case <-done:
		case <-time.After(2 * time.Second):
			t.Fatal("run did not stop after cancel")
		}
	}()

	cfg1 := prepareStockCfg("success", "wait1")
	cfg2 := prepareStockCfg("success", "wait2")

	mgr.addCh <- cfg1
	require.Eventually(t, mgr.handler.WaitingForDecision, time.Second, 10*time.Millisecond)

	secondSent := make(chan struct{})
	go func() {
		mgr.addCh <- cfg2
		close(secondSent)
	}()

	select {
	case <-secondSent:
		t.Fatal("second add was processed before wait timeout")
	case <-time.After(500 * time.Millisecond):
	}

	select {
	case <-secondSent:
	case <-time.After(7 * time.Second):
		t.Fatal("second add did not progress after wait timeout")
	}

	entry1, ok := mgr.exposed.LookupByKey(cfg1.ExposedKey())
	require.True(t, ok, "first config must stay exposed after timeout")
	assert.Equal(t, dyncfg.StatusAccepted, entry1.Status)
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

func TestRegisterJobMethods_FailFastOnCollisionWithStaticMethod(t *testing.T) {
	fnReg := &recordingFunctionRegistry{}
	mgr := New(Config{PluginName: testPluginName, FnReg: fnReg})
	mgr.moduleFuncs.registerModule("mod", collectorapi.Creator{
		Methods: func() []funcapi.MethodConfig {
			return []funcapi.MethodConfig{{ID: "dup"}}
		},
	})

	job := &lockProbeJob{fullName: "mod_job1", moduleName: "mod", name: "job1"}
	mgr.registerJobMethods(job, []funcapi.MethodConfig{{ID: "dup"}})

	assert.Empty(t, fnReg.registeredNames())
	assert.Empty(t, mgr.moduleFuncs.getJobMethods("mod", "job1"))
}

func TestRegisterJobMethods_FailFastOnCollisionWithOtherJob(t *testing.T) {
	fnReg := &recordingFunctionRegistry{}
	mgr := New(Config{PluginName: testPluginName, FnReg: fnReg})
	mgr.moduleFuncs.registerModule("mod", collectorapi.Creator{})
	mgr.moduleFuncs.registerJobMethods("mod", "jobA", []funcapi.MethodConfig{{ID: "dup"}})

	job := &lockProbeJob{fullName: "mod_jobB", moduleName: "mod", name: "jobB"}
	mgr.registerJobMethods(job, []funcapi.MethodConfig{{ID: "dup"}})

	assert.Empty(t, fnReg.registeredNames())
	assert.Empty(t, mgr.moduleFuncs.getJobMethods("mod", "jobB"))
}

func TestRegisterJobMethods_FailFastOnDuplicateWithinBatch(t *testing.T) {
	fnReg := &recordingFunctionRegistry{}
	mgr := New(Config{PluginName: testPluginName, FnReg: fnReg})
	mgr.moduleFuncs.registerModule("mod", collectorapi.Creator{})

	job := &lockProbeJob{fullName: "mod_job1", moduleName: "mod", name: "job1"}
	mgr.registerJobMethods(job, []funcapi.MethodConfig{
		{ID: "dup"},
		{ID: "dup"},
	})

	assert.Empty(t, fnReg.registeredNames())
	assert.Empty(t, mgr.moduleFuncs.getJobMethods("mod", "job1"))
}

func TestRegisterJobMethods_SuccessCommitsAllMethods(t *testing.T) {
	fnReg := &recordingFunctionRegistry{}
	mgr := New(Config{PluginName: testPluginName, FnReg: fnReg})
	mgr.moduleFuncs.registerModule("mod", collectorapi.Creator{})

	job := &lockProbeJob{fullName: "mod_job1", moduleName: "mod", name: "job1"}
	mgr.registerJobMethods(job, []funcapi.MethodConfig{
		{ID: "a"},
		{ID: "b"},
	})

	assert.ElementsMatch(t, []string{"mod:a", "mod:b"}, fnReg.registeredNames())
	assert.Len(t, mgr.moduleFuncs.getJobMethods("mod", "job1"), 2)
}

type lockProbeJob struct {
	fullName   string
	moduleName string
	name       string

	tickOnce    sync.Once
	stopOnce    sync.Once
	tickStarted chan struct{}
	tickRelease chan struct{}
}

func (j *lockProbeJob) FullName() string   { return j.fullName }
func (j *lockProbeJob) ModuleName() string { return j.moduleName }
func (j *lockProbeJob) Name() string       { return j.name }
func (j *lockProbeJob) Collector() any     { return nil }
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

type recordingFunctionRegistry struct {
	mu         sync.Mutex
	registered []string
}

func (r *recordingFunctionRegistry) Register(name string, _ func(functions.Function)) {
	r.mu.Lock()
	r.registered = append(r.registered, name)
	r.mu.Unlock()
}

func (r *recordingFunctionRegistry) Unregister(string)                                       {}
func (r *recordingFunctionRegistry) RegisterPrefix(string, string, func(functions.Function)) {}
func (r *recordingFunctionRegistry) UnregisterPrefix(string, string)                         {}

func (r *recordingFunctionRegistry) registeredNames() []string {
	r.mu.Lock()
	defer r.mu.Unlock()
	out := make([]string, len(r.registered))
	copy(out, r.registered)
	return out
}
