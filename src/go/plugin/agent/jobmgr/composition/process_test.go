// SPDX-License-Identifier: GPL-3.0-or-later

package composition

import (
	"bytes"
	"context"
	"errors"
	"io"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/netdata/netdata/go/plugins/pkg/funcapi"
	"github.com/netdata/netdata/go/plugins/pkg/multipath"
	"github.com/netdata/netdata/go/plugins/pkg/netdataapi"
	agentdiscovery "github.com/netdata/netdata/go/plugins/plugin/agent/discovery"
	"github.com/netdata/netdata/go/plugins/plugin/agent/jobmgr/lifecycle"
	"github.com/netdata/netdata/go/plugins/plugin/framework/collectorapi"
	"github.com/netdata/netdata/go/plugins/plugin/framework/confgroup"
	"github.com/netdata/netdata/go/plugins/plugin/framework/dyncfg"
	frameworkfunctions "github.com/netdata/netdata/go/plugins/plugin/framework/functions"
	"github.com/netdata/netdata/go/plugins/plugin/framework/vnodes"
	"github.com/stretchr/testify/require"
)

func TestProcessCoreServiceDiscoverySendsFunctionResultBeforeStatus(t *testing.T) {
	reader, writer := io.Pipe()
	output := newProcessSynchronizedBuffer()
	services := testRunServiceDiscoveryServices(t)
	process, err := newProcessCore(processCoreConfig{
		Input: reader, Output: output,
		ShutdownTimeout: time.Second,
		Modules:         collectorapi.Registry{},
		Jobs:            testRunJobServices(t),
		Discovery:       services,
	})
	require.NoError(t, err)
	commands := make(chan processControl, 1)
	done := make(chan error, 1)
	go func() {
		done <- process.run(context.Background(), commands)
	}()
	output.waitContains(t, "CONFIG go.d:sd:test create accepted template")

	_, writeStringErr := io.WriteString(
		writer,
		"FUNCTION sd-get 30 \"config go.d:sd:test:job get\" 0xFFFF \"user=test\"\n",
	)
	require.NoError(t, writeStringErr)

	output.waitContains(t, "CONFIG go.d:sd:test:job status running")
	wire := output.String()
	result := strings.Index(wire, "FUNCTION_RESULT_BEGIN sd-get 200 application/json")
	notification := strings.Index(wire, "CONFIG go.d:sd:test:job status running")
	require.False(t, result < 0 || notification < 0 || result >= notification)
	commands <- testProcessControl(processTerminate)
	select {
	case err := <-done:
		require.NoError(t, err)
	case <-time.After(3 * time.Second):
		require.FailNow(t, "test failed", "process did not terminate")
	}

	require.NoError(t, writer.Close())
}

func TestProcessCoreVnodeDynCfgOrdersAddCreateAndGet(t *testing.T) {
	reader, writer := io.Pipe()
	output := newProcessSynchronizedBuffer()
	jobs := testRunJobServices(t)
	jobs.InitialVnodes = map[string]*vnodes.VirtualNode{
		"initial": {
			Name: "initial", Hostname: "initial",
			GUID:       "11111111-1111-1111-1111-111111111111",
			Source:     "file=test",
			SourceType: confgroup.TypeUser,
		},
	}
	process, err := newProcessCore(processCoreConfig{
		Input: reader, Output: output,
		ShutdownTimeout: time.Second,
		Modules:         collectorapi.Registry{},
		Jobs:            jobs,
		Discovery:       testRunDiscoveryServices(t),
	})
	require.NoError(t, err)
	commands := make(chan processControl, 1)
	done := make(chan error, 1)
	go func() {
		done <- process.run(context.Background(), commands)
	}()
	output.waitContains(t, "CONFIG go.d:vnode:initial create running job")
	input := "" +
		"FUNCTION_PAYLOAD vnode-add 30 \"config go.d:vnode add db\" 0xFFFF \"user=test\" application/json\n" +
		"{\"guid\":\"22222222-2222-2222-2222-222222222222\"}\n" +
		"FUNCTION_PAYLOAD_END\n" +
		"FUNCTION vnode-get 30 \"config go.d:vnode:db get\" 0xFFFF \"user=test\"\n"

	_, writeStringErr := io.WriteString(writer, input)
	require.NoError(t, writeStringErr)

	output.waitContains(t, "FUNCTION_RESULT_BEGIN vnode-get 200 application/json")
	wire := output.String()
	addResult := strings.Index(wire, "FUNCTION_RESULT_BEGIN vnode-add 202 application/json")
	configCreate := strings.Index(wire, "CONFIG go.d:vnode:db create running job")
	getResult := strings.Index(wire, "FUNCTION_RESULT_BEGIN vnode-get 200 application/json")
	require.False(t, addResult < 0 ||
		configCreate < 0 ||
		getResult < 0 ||
		addResult >= configCreate ||
		configCreate >= getResult ||
		!strings.Contains(wire[getResult:], `"name":"db","hostname":"db","guid":"22222222-2222-2222-2222-222222222222"`))
	commands <- testProcessControl(processTerminate)
	select {
	case err := <-done:
		require.NoError(t, err)
	case <-time.After(3 * time.Second):
		require.FailNow(t, "test failed", "process did not terminate")
	}

	require.NoError(t, writer.Close())
}

func TestProcessCoreRestartsOneInputAndMovesFrameAuthority(t *testing.T) {
	reader, writer := io.Pipe()
	defer func() { require.NoError(t, writer.Close()) }()
	events := make(chan string, 16)
	var cleanupsMu sync.Mutex
	cleanups := 0
	output := processRecordingWriter{
		record: func(payload []byte) {
			switch {
			case bytes.HasPrefix(payload, []byte("FUNCTION GLOBAL")) &&
				bytes.Contains(payload, []byte(`"module:method"`)):
				events <- "publish"
			case bytes.HasPrefix(payload, []byte("FUNCTION_DEL")) &&
				bytes.Contains(payload, []byte(`"module:method"`)):
				events <- "withdraw"
			}
		},
	}
	process, err := newProcessCore(processCoreConfig{
		Input: reader, Output: output,
		ShutdownTimeout: time.Second,
		Modules: collectorapi.Registry{
			"module": {
				AgentFunctions: func() []funcapi.FunctionConfig {
					return []funcapi.FunctionConfig{{ID: "method"}}
				},
				MethodHandler: func(collectorapi.RuntimeJob) funcapi.MethodHandler {
					return &runTestHandler{cleanup: func() {
						cleanupsMu.Lock()
						cleanups++
						cleanupsMu.Unlock()
					}}
				},
			},
		},
		Jobs:      testRunJobServices(t),
		Discovery: testRunDiscoveryServices(t),
	})
	require.NoError(t, err)
	commands := make(chan processControl, 2)
	done := make(chan error, 1)
	go func() {
		done <- process.run(context.Background(), commands)
	}()
	waitProcessEvent(t, events, "publish")
	commands <- testProcessControl(processRestart)
	waitProcessEvent(t, events, "withdraw")
	waitProcessEvent(t, events, "publish")
	commands <- testProcessControl(processTerminate)
	waitProcessEvent(t, events, "withdraw")
	select {
	case err := <-done:
		require.NoError(t, err)
	case <-time.After(3 * time.Second):
		require.FailNow(t, "test failed", "process did not terminate")
	}

	census := process.ingress.Census()
	require.False(t, census.ReaderStarts != 1 || census.State != "contained" || census.RunGeneration != 0)

	cleanupsMu.Lock()
	defer cleanupsMu.Unlock()
	require.EqualValues(t, 2, cleanups)
}

func TestProcessCoreRejectsSuccessorAfterDiscoveryProviderMissesJoin(
	t *testing.T,
) {
	reader, writer := io.Pipe()
	defer func() { require.NoError(t, writer.Close()) }()
	started := make(chan struct{})
	release := make(chan struct{})
	defer close(release)
	factory := agentdiscovery.NewProviderFactory(
		"noncooperative",
		func(agentdiscovery.BuildContext) (
			agentdiscovery.Discoverer,
			bool,
			error,
		) {
			return processNoncooperativeDiscovery{
				started: started,
				release: release,
			}, true, nil
		},
	)
	catalog, err := agentdiscovery.NewProviderCatalog(
		[]agentdiscovery.ProviderFactory{factory},
	)
	require.NoError(t, err)
	process, err := newProcessCore(processCoreConfig{
		Input: reader, Output: newProcessSynchronizedBuffer(),
		ShutdownTimeout: 100 * time.Millisecond,
		Modules:         collectorapi.Registry{},
		Jobs:            testRunJobServices(t),
		Discovery: runDiscoveryServices{
			BuildContext: agentdiscovery.BuildContext{
				Registry: confgroup.Registry{"test": {}},
			},
			Providers: catalog,
		},
	})
	require.NoError(t, err)
	commands := make(chan processControl, 1)
	done := make(chan error, 1)
	go func() {
		done <- process.run(context.Background(), commands)
	}()
	select {
	case <-started:
	case <-time.After(time.Second):
		require.FailNow(t, "test failed", "discovery provider did not start")
	}
	control := testProcessControl(processRestart)
	commands <- control
	select {
	case err := <-control.result:
		require.False(t, err == nil ||
			!strings.Contains(err.Error(), "jobmgr composition: run did not quiesce") ||
			!errors.Is(err, context.DeadlineExceeded))
	case <-time.After(time.Second):
		require.FailNow(t, "test failed", "restart did not reject retained discovery provider")
	}
	select {
	case err := <-done:
		require.False(t, err == nil || !strings.Contains(err.Error(), "jobmgr composition: run did not quiesce"))
	case <-time.After(time.Second):
		require.FailNow(t, "test failed", "process did not exit after retained discovery provider")
	}
}

func TestProcessCoreContainsProviderConstructionPanic(t *testing.T) {
	reader, writer := io.Pipe()
	events := make(chan string, 8)
	var cleanupsMu sync.Mutex
	cleanups := 0
	factory := agentdiscovery.NewProviderFactory(
		"panicked",
		func(agentdiscovery.BuildContext) (
			agentdiscovery.Discoverer,
			bool,
			error,
		) {
			panic("provider construction")
		},
	)
	catalog, err := agentdiscovery.NewProviderCatalog(
		[]agentdiscovery.ProviderFactory{factory},
	)
	require.NoError(t, err)
	process, err := newProcessCore(processCoreConfig{
		Input: reader,
		Output: processRecordingWriter{record: func(payload []byte) {
			switch {
			case bytes.HasPrefix(payload, []byte("FUNCTION GLOBAL")) &&
				bytes.Contains(payload, []byte(`"module:method"`)):
				events <- "publish"
			case bytes.HasPrefix(payload, []byte("FUNCTION_DEL")) &&
				bytes.Contains(payload, []byte(`"module:method"`)):
				events <- "withdraw"
			}
		}},
		ShutdownTimeout: time.Second,
		Modules: collectorapi.Registry{
			"module": {
				AgentFunctions: func() []funcapi.FunctionConfig {
					return []funcapi.FunctionConfig{{ID: "method"}}
				},
				MethodHandler: func(collectorapi.RuntimeJob) funcapi.MethodHandler {
					return &runTestHandler{cleanup: func() {
						cleanupsMu.Lock()
						cleanups++
						cleanupsMu.Unlock()
					}}
				},
			},
		},
		Jobs: testRunJobServices(t),
		Discovery: runDiscoveryServices{
			BuildContext: agentdiscovery.BuildContext{
				Registry: confgroup.Registry{"module": {}},
			},
			Providers: catalog,
		},
	})
	require.NoError(t, err)
	done := make(chan error, 1)
	go func() {
		done <- process.run(context.Background(), make(chan processControl, 1))
	}()
	waitProcessEvent(t, events, "publish")
	waitProcessEvent(t, events, "withdraw")
	select {
	case err := <-done:
		require.ErrorContains(t, err, "provider factory panic")
	case <-time.After(3 * time.Second):
		require.FailNow(t, "test failed", "process did not contain construction failure")
	}
	require.NoError(t, writer.Close())
	require.Empty(t, events)
	census := process.ingress.Census()
	require.False(t, census.State != "contained" ||
		census.RunGeneration != 0 || census.ReaderStarts != 0)
	cleanupsMu.Lock()
	defer cleanupsMu.Unlock()
	require.EqualValues(t, 1, cleanups)
}

func TestProcessRetirementPreservesRunDirtyCause(t *testing.T) {
	run, err := lifecycle.NewRunSupervisor(
		1,
		lifecycle.RealClock{},
		time.Second,
	)
	require.NoError(t, err)
	cause := errors.New("discovery shutdown failed")

	run.Dirty(cause)

	err = (&processCore{}).retireRun(
		context.Background(),
		&runGeneration{run: run},
	)
	require.False(t, !errors.Is(err, cause) || !strings.Contains(err.Error(), "run did not quiesce"))
}

type processRecordingWriter struct {
	record func([]byte)
}

func (prw processRecordingWriter) Write(payload []byte) (int, error) {
	prw.record(payload)
	return len(payload), nil
}

type processSynchronizedBuffer struct {
	mu     sync.Mutex
	buffer bytes.Buffer
	writes chan struct{}
}

func newProcessSynchronizedBuffer() *processSynchronizedBuffer {
	return &processSynchronizedBuffer{
		writes: make(chan struct{}, 32),
	}
}

func (psb *processSynchronizedBuffer) Write(payload []byte) (int, error) {
	psb.mu.Lock()
	count, err := psb.buffer.Write(payload)
	psb.mu.Unlock()
	select {
	case psb.writes <- struct{}{}:
	default:
	}
	return count, err
}

func (psb *processSynchronizedBuffer) String() string {
	psb.mu.Lock()
	defer psb.mu.Unlock()
	return psb.buffer.String()
}

func (psb *processSynchronizedBuffer) waitContains(
	t *testing.T,
	want string,
) {
	t.Helper()
	timeout := time.NewTimer(3 * time.Second)
	defer timeout.Stop()
	for {
		if strings.Contains(psb.String(), want) {
			return
		}
		select {
		case <-psb.writes:
		case <-timeout.C:
			require.FailNowf(t, "test failed", "process output does not contain %q: %q", want, psb.String())
		}
	}
}

func testRunServiceDiscoveryServices(
	t testing.TB,
) runDiscoveryServices {
	t.Helper()
	factory := agentdiscovery.NewProviderFactory(
		"service-discovery-test",
		func(build agentdiscovery.BuildContext) (
			agentdiscovery.Discoverer,
			bool,
			error,
		) {
			return processServiceDiscovery{
				registry: build.FnReg,
				output:   build.DyncfgOutput,
			}, true, nil
		},
	)
	catalog, err := agentdiscovery.NewProviderCatalog(
		[]agentdiscovery.ProviderFactory{factory},
	)
	require.NoError(t, err)
	return runDiscoveryServices{
		BuildContext: agentdiscovery.BuildContext{
			Registry: confgroup.Registry{"test": {}},
			Paths: agentdiscovery.PathsConfig{
				ServiceDiscoveryConfigDir: multipath.MultiPath{"enabled"},
			},
		},
		Providers: catalog,
	}
}

type processServiceDiscovery struct {
	registry frameworkfunctions.Registry
	output   dyncfg.Output
}

type processNoncooperativeDiscovery struct {
	started chan<- struct{}
	release <-chan struct{}
}

func (pnd processNoncooperativeDiscovery) Run(
	context.Context,
	chan<- []*confgroup.Group,
) {
	close(pnd.started)
	<-pnd.release
}

func (psd processServiceDiscovery) Run(
	ctx context.Context,
	_ chan<- []*confgroup.Group,
) {
	psd.registry.RegisterPrefix(
		"config",
		"go.d:sd:",
		func(function frameworkfunctions.Function) {
			psd.output.FunctionResult(dyncfg.Result{
				UID: function.UID, Code: 200,
				ContentType: "application/json",
				Payload:     `{"status":200}`,
			})
			psd.output.ConfigStatus("go.d:sd:test:job", dyncfg.StatusRunning)
		},
	)
	psd.output.ConfigCreate(netdataapi.ConfigOpts{
		ID: "go.d:sd:test", Status: "accepted",
		ConfigType: "template",
		Path:       "/collectors/go.d/ServiceDiscovery",
		SourceType: "internal", Source: "internal",
		SupportedCommands: "get",
	})
	<-ctx.Done()
	psd.registry.UnregisterPrefix("config", "go.d:sd:")
}

func waitProcessEvent(t *testing.T, events <-chan string, want string) {
	t.Helper()
	select {
	case got := <-events:
		require.EqualValues(t, want, got)
	case <-time.After(2 * time.Second):
		require.FailNowf(t, "test failed", "timed out waiting for process event %q", want)
	}
}

func testProcessControl(command processCommand) processControl {
	return processControl{
		command: command,
		result:  make(chan error, 1),
	}
}
