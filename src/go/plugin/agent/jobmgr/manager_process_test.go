// SPDX-License-Identifier: GPL-3.0-or-later

package jobmgr

import (
	"bytes"
	"context"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/netdata/netdata/go/plugins/pkg/funcapi"
	"github.com/netdata/netdata/go/plugins/plugin/agent/secrets/secretstore"
	"github.com/netdata/netdata/go/plugins/plugin/framework/collectorapi"
	"github.com/netdata/netdata/go/plugins/plugin/framework/confgroup"
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

func TestRun_DoesNotRegisterModuleMethodsBeforeAnyJobStarts(t *testing.T) {
	fnReg := &recordingFunctionRegistry{}
	mgr := New(Config{PluginName: testPluginName, FnReg: fnReg})

	mgr.modules = collectorapi.Registry{
		"mod": collectorapi.Creator{
			Methods: func() []funcapi.MethodConfig {
				return []funcapi.MethodConfig{{ID: "a"}}
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

func TestStartRunningJob_RegistersModuleMethodsOnFirstStartedJob(t *testing.T) {
	fnReg := &recordingFunctionRegistry{}
	mgr := New(Config{PluginName: testPluginName, FnReg: fnReg})
	creator := collectorapi.Creator{
		Methods: func() []funcapi.MethodConfig {
			return []funcapi.MethodConfig{{ID: "a"}, {ID: "b"}}
		},
	}
	mgr.modules = collectorapi.Registry{"mod": creator}
	mgr.funcCtl.RegisterModules(mgr.modules)

	job := &lockProbeJob{fullName: "mod_job1", moduleName: "mod", name: "job1"}
	mgr.startRunningJob(job)

	assert.ElementsMatch(t, []string{"mod:a", "mod:b"}, fnReg.registeredNames())
}

func TestStartRunningJob_DoesNotReregisterModuleMethods(t *testing.T) {
	fnReg := &recordingFunctionRegistry{}
	mgr := New(Config{PluginName: testPluginName, FnReg: fnReg})
	creator := collectorapi.Creator{
		Methods: func() []funcapi.MethodConfig {
			return []funcapi.MethodConfig{{ID: "a"}, {ID: "b"}}
		},
	}
	mgr.modules = collectorapi.Registry{"mod": creator}
	mgr.funcCtl.RegisterModules(mgr.modules)

	job1 := &lockProbeJob{fullName: "mod_job1", moduleName: "mod", name: "job1"}
	mgr.startRunningJob(job1)

	job2 := &lockProbeJob{fullName: "mod_job2", moduleName: "mod", name: "job2"}
	mgr.startRunningJob(job2)

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
	prefixes   []registeredPrefix
}

func (r *recordingFunctionRegistry) Register(name string, _ func(functions.Function)) {
	r.mu.Lock()
	r.registered = append(r.registered, name)
	r.mu.Unlock()
}

func (r *recordingFunctionRegistry) Unregister(string) {}
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

func (r *recordingFunctionRegistry) registeredPrefixes() []registeredPrefix {
	r.mu.Lock()
	defer r.mu.Unlock()
	out := make([]registeredPrefix, len(r.prefixes))
	copy(out, r.prefixes)
	return out
}

type registeredPrefix struct {
	name   string
	prefix string
}
