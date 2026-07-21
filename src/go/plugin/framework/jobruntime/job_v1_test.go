// SPDX-License-Identifier: GPL-3.0-or-later

package jobruntime

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"io"
	"sync"
	"testing"
	"time"

	"github.com/netdata/netdata/go/plugins/plugin/framework/collectorapi"
	"github.com/netdata/netdata/go/plugins/plugin/framework/vnodes"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

const (
	pluginName = "plugin"
	modName    = "module"
	jobName    = "job"
)

type retryableTestError struct {
	error
}

func (e retryableTestError) DyncfgRetryable() bool { return true }

type foreignRetryableTestError struct {
	error
}

func (e foreignRetryableTestError) Retryable() bool { return true }

func newTestJob() *Job {
	return NewJob(
		JobConfig{
			PluginName:      pluginName,
			Name:            jobName,
			ModuleName:      modName,
			FullName:        modName + "_" + jobName,
			Module:          nil,
			Out:             io.Discard,
			UpdateEvery:     0,
			AutoDetectEvery: 0,
			Priority:        0,
		},
	)
}

func TestNewJob(t *testing.T) {
	assert.IsType(t, (*Job)(nil), newTestJob())
}

func TestJob_FullName(t *testing.T) {
	job := newTestJob()

	assert.Equal(t, job.FullName(), fmt.Sprintf("%s_%s", modName, jobName))
}

func TestJob_ModuleName(t *testing.T) {
	job := newTestJob()

	assert.Equal(t, job.ModuleName(), modName)
}

func TestJob_Name(t *testing.T) {
	job := newTestJob()

	assert.Equal(t, job.Name(), jobName)
}

func TestJob_AutoDetectionEvery(t *testing.T) {
	job := newTestJob()

	assert.Equal(t, job.AutoDetectionEvery(), job.autoDetectEvery)
}

func TestJob_RetryAutoDetection(t *testing.T) {
	job := newTestJob()
	m := &collectorapi.MockCollectorV1{
		InitFunc: func(context.Context) error {
			return nil
		},
		CheckFunc: func(context.Context) error { return errors.New("check error") },
		ChartsFunc: func() *collectorapi.Charts {
			return &collectorapi.Charts{}
		},
	}
	job.module = m
	job.autoDetectEvery = 1

	assert.True(t, job.RetryAutoDetection())
	assert.Equal(t, infTries, job.autoDetectTries)
	for range 1000 {
		_ = job.check(context.Background())
	}
	assert.True(t, job.RetryAutoDetection())
	assert.Equal(t, infTries, job.autoDetectTries)

	job.autoDetectTries = 10
	for range 10 {
		_ = job.check(context.Background())
	}
	assert.False(t, job.RetryAutoDetection())
	assert.Equal(t, 0, job.autoDetectTries)
}

func TestJob_AutoDetection(t *testing.T) {
	job := newTestJob()
	var v int
	m := &collectorapi.MockCollectorV1{
		InitFunc: func(context.Context) error {
			v++
			return nil
		},
		CheckFunc: func(context.Context) error {
			v++
			return nil
		},
		ChartsFunc: func() *collectorapi.Charts {
			v++
			return &collectorapi.Charts{}
		},
	}
	job.module = m

	assert.NoError(t, job.AutoDetectionManaged(context.Background()))
	assert.Equal(t, 3, v)
}

func TestJob_AutoDetection_FailInit(t *testing.T) {
	job := newTestJob()
	job.autoDetectEvery = 1
	m := &collectorapi.MockCollectorV1{
		InitFunc: func(context.Context) error {
			return errors.New("init error")
		},
	}
	job.module = m

	assert.Error(t, job.AutoDetectionManaged(context.Background()))
	assert.False(t, job.RetryAutoDetection())
	assert.False(t, m.CleanupDone)
	job.CleanupRejected()
	assert.True(t, m.CleanupDone)
}

func TestJob_AutoDetection_RetryableFailInitKeepsRetry(t *testing.T) {
	job := newTestJob()
	job.autoDetectEvery = 1
	m := &collectorapi.MockCollectorV1{
		InitFunc: func(context.Context) error {
			return retryableTestError{error: errors.New("init error")}
		},
	}
	job.module = m

	assert.Error(t, job.AutoDetectionManaged(context.Background()))
	assert.True(t, job.RetryAutoDetection())
	assert.False(t, m.CleanupDone)
	job.CleanupRejected()
	assert.True(t, m.CleanupDone)
}

func TestJob_AutoDetection_ForeignRetryableFailInitDisablesRetry(t *testing.T) {
	job := newTestJob()
	job.autoDetectEvery = 1
	m := &collectorapi.MockCollectorV1{
		InitFunc: func(context.Context) error {
			return foreignRetryableTestError{error: errors.New("init error")}
		},
	}
	job.module = m

	assert.Error(t, job.AutoDetectionManaged(context.Background()))
	assert.False(t, job.RetryAutoDetection())
	assert.False(t, m.CleanupDone)
	job.CleanupRejected()
	assert.True(t, m.CleanupDone)
}

func TestJob_AutoDetection_FailCheck(t *testing.T) {
	job := newTestJob()
	m := &collectorapi.MockCollectorV1{
		InitFunc: func(context.Context) error {
			return nil
		},
		CheckFunc: func(context.Context) error {
			return errors.New("check error")
		},
	}
	job.module = m

	assert.Error(t, job.AutoDetectionManaged(context.Background()))
	assert.False(t, m.CleanupDone)
	job.CleanupRejected()
	assert.True(t, m.CleanupDone)
}

func TestJob_AutoDetection_FailPostCheck(t *testing.T) {
	job := newTestJob()
	m := &collectorapi.MockCollectorV1{
		InitFunc: func(context.Context) error {
			return nil
		},
		CheckFunc: func(context.Context) error {
			return nil
		},
		ChartsFunc: func() *collectorapi.Charts {
			return nil
		},
	}
	job.module = m

	assert.Error(t, job.AutoDetectionManaged(context.Background()))
	assert.False(t, m.CleanupDone)
	job.CleanupRejected()
	assert.True(t, m.CleanupDone)
}

func TestJob_AutoDetection_PanicInit(t *testing.T) {
	job := newTestJob()
	m := &collectorapi.MockCollectorV1{
		InitFunc: func(context.Context) error {
			panic("panic in Init")
		},
	}
	job.module = m

	assert.Error(t, job.AutoDetectionManaged(context.Background()))
	assert.False(t, m.CleanupDone)
	job.CleanupRejected()
	assert.True(t, m.CleanupDone)
}

func TestJob_AutoDetection_PanicCheck(t *testing.T) {
	job := newTestJob()
	m := &collectorapi.MockCollectorV1{
		InitFunc: func(context.Context) error {
			return nil
		},
		CheckFunc: func(context.Context) error {
			panic("panic in Check")
		},
	}
	job.module = m

	assert.Error(t, job.AutoDetectionManaged(context.Background()))
	assert.False(t, m.CleanupDone)
	job.CleanupRejected()
	assert.True(t, m.CleanupDone)
}

func TestJob_AutoDetection_PanicPostCheck(t *testing.T) {
	job := newTestJob()
	m := &collectorapi.MockCollectorV1{
		InitFunc: func(context.Context) error {
			return nil
		},
		CheckFunc: func(context.Context) error {
			return nil
		},
		ChartsFunc: func() *collectorapi.Charts {
			panic("panic in PostCheck")
		},
	}
	job.module = m

	assert.Error(t, job.AutoDetectionManaged(context.Background()))
	assert.False(t, m.CleanupDone)
	job.CleanupRejected()
	assert.True(t, m.CleanupDone)
}

func TestJob_Start(t *testing.T) {
	m := &collectorapi.MockCollectorV1{
		ChartsFunc: func() *collectorapi.Charts {
			return &collectorapi.Charts{
				&collectorapi.Chart{
					ID:    "id",
					Title: "title",
					Units: "units",
					Dims: collectorapi.Dims{
						{ID: "id1"},
						{ID: "id2"},
					},
				},
			}
		},
		CollectFunc: func(context.Context) map[string]int64 {
			return map[string]int64{
				"id1": 1,
				"id2": 2,
			}
		},
	}
	job := newTestJob()
	job.module = m
	job.charts = job.module.Charts()
	job.updateEvery = 1

	go func() {
		for i := 1; i < 3; i++ {
			job.Tick(i)
			time.Sleep(time.Second)
		}
		job.Stop()
	}()

	job.StartManaged(make(chan struct{}))

	assert.False(t, m.CleanupDone)
	job.Cleanup()
	assert.True(t, m.CleanupDone)
}

func TestJob_StopBeforeStartDoesNotBlock(t *testing.T) {
	job := newTestJob()

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

func TestJob_MainLoop_Panic(t *testing.T) {
	m := &collectorapi.MockCollectorV1{
		CollectFunc: func(context.Context) map[string]int64 {
			panic("panic in Collect")
		},
	}
	job := newTestJob()
	job.module = m
	job.updateEvery = 1

	go func() {
		for i := 1; i < 3; i++ {
			time.Sleep(time.Second)
			job.Tick(i)
		}
		job.Stop()
	}()

	job.StartManaged(make(chan struct{}))

	assert.True(t, job.panicked.Load())
	assert.False(t, m.CleanupDone)
	job.Cleanup()
	assert.True(t, m.CleanupDone)
}

func TestJob_OutputFailureDoesNotReportCollectorPanic(t *testing.T) {
	mod := &collectorapi.MockCollectorV1{
		ChartsFunc: func() *collectorapi.Charts {
			return &collectorapi.Charts{
				&collectorapi.Chart{
					ID: "id", Title: "title", Units: "units",
					Dims: collectorapi.Dims{&collectorapi.Dim{ID: "value"}},
				},
			}
		},
		CollectFunc: func(context.Context) map[string]int64 {
			return map[string]int64{"value": 1}
		},
	}
	job := NewJob(JobConfig{
		PluginName: pluginName, Name: jobName, ModuleName: modName,
		FullName: modName + "_" + jobName, Module: mod,
		Out: writeFunc(func([]byte) (int, error) {
			return 0, errors.New("write failed")
		}),
		UpdateEvery: 1,
	})
	require.NoError(t, job.AutoDetectionManaged(context.Background()))

	job.runOnce()

	assert.False(t, job.panicked.Load())
}

func TestJob_Tick(t *testing.T) {
	job := newTestJob()
	for i := range 3 {
		job.Tick(i)
	}
}

func TestJob_PullVnodeUpdateDuringCollectAppliesBeforeSameCycleEmission(t *testing.T) {
	var out bytes.Buffer
	current := newSnapshotHolder(VnodeSnapshot{
		Vnode:            &vnodes.VirtualNode{Name: "db", Hostname: "host-one", GUID: "node-guid"},
		Revision:         1,
		MetadataRevision: 1,
	})
	collectStarted := make(chan struct{})
	collectRelease := make(chan struct{})

	mod := &collectorapi.MockCollectorV1{
		ChartsFunc: func() *collectorapi.Charts {
			return &collectorapi.Charts{&collectorapi.Chart{ID: "id", Title: "t", Units: "u", Dims: collectorapi.Dims{{ID: "d1"}}}}
		},
		CollectFunc: func(context.Context) map[string]int64 {
			close(collectStarted)
			<-collectRelease
			return map[string]int64{"d1": 1}
		},
	}
	job := NewJob(JobConfig{
		PluginName:            pluginName,
		Name:                  jobName,
		ModuleName:            modName,
		FullName:              modName + "_" + jobName,
		Module:                mod,
		Out:                   &out,
		Vnode:                 *current.snapshot().Vnode.Copy(),
		VnodeName:             "db",
		VnodeRevision:         1,
		VnodeMetadataRevision: 1,
		VnodeLookup:           current.lookup,
	})
	require.NoError(t, job.AutoDetectionManaged(context.Background()))

	done := make(chan struct{})
	go func() {
		defer close(done)
		job.runOnce()
	}()
	<-collectStarted
	current.set(VnodeSnapshot{
		Vnode:            &vnodes.VirtualNode{Name: "db", Hostname: "host-two", GUID: "node-guid"},
		Revision:         2,
		MetadataRevision: 2,
	})
	close(collectRelease)
	<-done

	assert.Contains(t, out.String(), "HOST_DEFINE 'node-guid' 'host-two'")
	assert.NotContains(t, out.String(), "HOST_DEFINE 'node-guid' 'host-one'")
}

func TestJob_PullSourceOnlyUpdateDoesNotResendHostInfo(t *testing.T) {
	var out bytes.Buffer
	current := newSnapshotHolder(VnodeSnapshot{
		Vnode:            &vnodes.VirtualNode{Name: "db", Hostname: "host-one", GUID: "node-guid", SourceType: "user"},
		Revision:         1,
		MetadataRevision: 1,
	})
	mod := &collectorapi.MockCollectorV1{
		ChartsFunc: func() *collectorapi.Charts {
			return &collectorapi.Charts{&collectorapi.Chart{ID: "id", Title: "t", Units: "u", Dims: collectorapi.Dims{{ID: "d1"}}}}
		},
		CollectFunc: func(context.Context) map[string]int64 { return map[string]int64{"d1": 1} },
	}
	job := NewJob(JobConfig{
		PluginName:            pluginName,
		Name:                  jobName,
		ModuleName:            modName,
		FullName:              modName + "_" + jobName,
		Module:                mod,
		Out:                   &out,
		Vnode:                 *current.snapshot().Vnode.Copy(),
		VnodeName:             "db",
		VnodeRevision:         1,
		VnodeMetadataRevision: 1,
		VnodeLookup:           current.lookup,
	})
	require.NoError(t, job.AutoDetectionManaged(context.Background()))
	job.runOnce()

	out.Reset()
	current.set(VnodeSnapshot{
		Vnode:            &vnodes.VirtualNode{Name: "db", Hostname: "host-one", GUID: "node-guid", SourceType: "dyncfg"},
		Revision:         2,
		MetadataRevision: 1,
	})
	job.runOnce()

	assert.NotContains(t, out.String(), "HOST_DEFINE 'node-guid' 'host-one'")
}

func TestJob_ModuleOwnedVnodeDoesNotOverrideConfiguredJobVnode(t *testing.T) {
	var out bytes.Buffer
	mod := &v1ModuleOwnedVnodeCollector{
		MockCollectorV1: collectorapi.MockCollectorV1{
			ChartsFunc: func() *collectorapi.Charts {
				return &collectorapi.Charts{&collectorapi.Chart{ID: "id", Title: "t", Units: "u", Dims: collectorapi.Dims{{ID: "d1"}}}}
			},
			CollectFunc: func(context.Context) map[string]int64 { return map[string]int64{"d1": 1} },
		},
		vnode: &vnodes.VirtualNode{Name: "module", Hostname: "module-host", GUID: "module-guid"},
	}
	current := newSnapshotHolder(VnodeSnapshot{
		Vnode:            &vnodes.VirtualNode{Name: "db", Hostname: "pulled-host", GUID: "pulled-guid"},
		Revision:         2,
		MetadataRevision: 2,
	})
	job := NewJob(JobConfig{
		PluginName:            pluginName,
		Name:                  jobName,
		ModuleName:            modName,
		FullName:              modName + "_" + jobName,
		Module:                mod,
		Out:                   &out,
		Vnode:                 vnodes.VirtualNode{Name: "db", Hostname: "job-host", GUID: "job-guid"},
		VnodeName:             "db",
		VnodeRevision:         1,
		VnodeMetadataRevision: 1,
		VnodeLookup:           current.lookup,
	})
	require.NoError(t, job.AutoDetectionManaged(context.Background()))

	job.runOnce()

	assert.Contains(t, out.String(), "HOST_DEFINE 'job-guid' 'job-host'")
	assert.NotContains(t, out.String(), "module-host")
	assert.NotContains(t, out.String(), "pulled-host")
}

type v1ModuleOwnedVnodeCollector struct {
	collectorapi.MockCollectorV1
	vnode *vnodes.VirtualNode
}

func (c *v1ModuleOwnedVnodeCollector) VirtualNode() *vnodes.VirtualNode {
	return c.vnode
}

type vnodeSnapshotHolder struct {
	mu      sync.Mutex
	current VnodeSnapshot
}

func newSnapshotHolder(snapshot VnodeSnapshot) *vnodeSnapshotHolder {
	return &vnodeSnapshotHolder{current: snapshot.Copy()}
}

func (h *vnodeSnapshotHolder) set(snapshot VnodeSnapshot) {
	h.mu.Lock()
	defer h.mu.Unlock()
	h.current = snapshot.Copy()
}

func (h *vnodeSnapshotHolder) snapshot() VnodeSnapshot {
	h.mu.Lock()
	defer h.mu.Unlock()
	return h.current.Copy()
}

func (h *vnodeSnapshotHolder) lookup(string) (VnodeSnapshot, bool) {
	return h.snapshot(), true
}

func newTestFunctionOnlyJob() *Job {
	return NewJob(
		JobConfig{
			PluginName:      pluginName,
			Name:            jobName,
			ModuleName:      modName,
			FullName:        modName + "_" + jobName,
			Module:          nil,
			Out:             io.Discard,
			UpdateEvery:     0,
			AutoDetectEvery: 0,
			Priority:        0,
			FunctionOnly:    true,
		},
	)
}

func TestJob_AutoDetection_FunctionOnly_NilCharts(t *testing.T) {
	job := newTestFunctionOnlyJob()
	m := &collectorapi.MockCollectorV1{
		InitFunc: func(context.Context) error {
			return nil
		},
		CheckFunc: func(context.Context) error {
			return nil
		},
		ChartsFunc: func() *collectorapi.Charts {
			return nil
		},
	}
	job.module = m

	assert.NoError(t, job.AutoDetectionManaged(context.Background()))
}

func TestJob_AutoDetection_FunctionOnlyFailCheckCleansUp(t *testing.T) {
	job := newTestFunctionOnlyJob()
	cleanupCalls := 0
	m := &collectorapi.MockCollectorV1{
		InitFunc: func(context.Context) error {
			return nil
		},
		CheckFunc: func(context.Context) error {
			return errors.New("check error")
		},
		CleanupFunc: func(context.Context) {
			cleanupCalls++
		},
	}
	job.module = m

	assert.Error(t, job.AutoDetectionManaged(context.Background()))
	assert.False(t, m.CleanupDone)
	job.CleanupRejected()
	assert.True(t, m.CleanupDone)
	assert.Equal(t, 1, cleanupCalls)
}

func TestJob_Start_FunctionOnly(t *testing.T) {
	collectCalled := false
	m := &collectorapi.MockCollectorV1{
		ChartsFunc: func() *collectorapi.Charts {
			return nil
		},
		CollectFunc: func(context.Context) map[string]int64 {
			collectCalled = true
			return map[string]int64{"id1": 1}
		},
	}
	job := newTestFunctionOnlyJob()
	job.module = m
	job.updateEvery = 1

	go func() {
		for i := 1; i < 3; i++ {
			job.Tick(i)
			time.Sleep(time.Second)
		}
		job.Stop()
	}()

	job.StartManaged(make(chan struct{}))

	assert.False(t, collectCalled, "Collect should not be called for function-only jobs")
	assert.False(t, m.CleanupDone)
	job.Cleanup()
	assert.True(t, m.CleanupDone)
}
