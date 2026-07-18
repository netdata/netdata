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
	"github.com/netdata/netdata/go/plugins/plugin/agent/jobmgr"
	"github.com/netdata/netdata/go/plugins/plugin/agent/jobmgr/lifecycle"
	"github.com/netdata/netdata/go/plugins/plugin/framework/collectorapi"
	"github.com/netdata/netdata/go/plugins/plugin/framework/confgroup"
	frameworkfunctions "github.com/netdata/netdata/go/plugins/plugin/framework/functions"
	"github.com/netdata/netdata/go/plugins/plugin/framework/vnodes"
)

func TestProcessCoreServiceDiscoveryUsesCandidateFunctionTransaction(t *testing.T) {
	reader, writer := io.Pipe()
	output := newProcessSynchronizedBuffer()
	services := testRunServiceDiscoveryServices(t)
	process, err := newProcessCore(processCoreConfig{
		Input: reader, Output: output, FirstGeneration: 1,
		ShutdownTimeout: time.Second, Clock: lifecycle.RealClock{},
		Modules:   collectorapi.Registry{},
		Jobs:      testRunJobServices(t),
		Discovery: services,
		Planner: func(
			runPlannerCapabilities,
		) (jobmgr.Planner, jobmgr.RunFinalizer, error) {
			return runRejectingPlanner{},
				jobmgr.RunFinalizerFunc(
					func(context.Context, uint64) error { return nil },
				),
				nil
		},
	})
	if err != nil {
		t.Fatal(err)
	}
	commands := make(chan processControl, 1)
	done := make(chan error, 1)
	go func() {
		done <- process.run(context.Background(), commands)
	}()
	output.waitContains(
		t,
		"CONFIG go.d:sd:test create accepted template",
	)
	if _, err := io.WriteString(
		writer,
		"FUNCTION sd-get 30 \"config go.d:sd:test:job get\" 0xFFFF \"user=test\"\n",
	); err != nil {
		t.Fatal(err)
	}
	output.waitContains(
		t,
		"CONFIG go.d:sd:test:job status running",
	)
	wire := output.String()
	result := strings.Index(
		wire,
		"FUNCTION_RESULT_BEGIN sd-get 200 application/json",
	)
	notification := strings.Index(
		wire,
		"CONFIG go.d:sd:test:job status running",
	)
	if result < 0 || notification < 0 || result >= notification {
		t.Fatalf(
			"service discovery wire ordering differs: %q",
			wire,
		)
	}
	commands <- testProcessControl(processTerminate)
	select {
	case err := <-done:
		if err != nil {
			t.Fatal(err)
		}
	case <-time.After(3 * time.Second):
		t.Fatal("process did not terminate")
	}
	if err := writer.Close(); err != nil {
		t.Fatal(err)
	}
}

func TestProcessCoreVnodeDynCfgUsesCandidateTransaction(t *testing.T) {
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
		Input: reader, Output: output, FirstGeneration: 1,
		ShutdownTimeout: time.Second, Clock: lifecycle.RealClock{},
		Modules:   collectorapi.Registry{},
		Jobs:      jobs,
		Discovery: testRunDiscoveryServices(t),
		Planner: func(
			runPlannerCapabilities,
		) (jobmgr.Planner, jobmgr.RunFinalizer, error) {
			return runRejectingPlanner{},
				jobmgr.RunFinalizerFunc(
					func(context.Context, uint64) error { return nil },
				),
				nil
		},
	})
	if err != nil {
		t.Fatal(err)
	}
	commands := make(chan processControl, 1)
	done := make(chan error, 1)
	go func() {
		done <- process.run(context.Background(), commands)
	}()
	output.waitContains(
		t,
		"CONFIG go.d:vnode:initial create running job",
	)
	input := "" +
		"FUNCTION_PAYLOAD vnode-add 30 \"config go.d:vnode add db\" 0xFFFF \"user=test\" application/json\n" +
		"{\"guid\":\"22222222-2222-2222-2222-222222222222\"}\n" +
		"FUNCTION_PAYLOAD_END\n" +
		"FUNCTION vnode-get 30 \"config go.d:vnode:db get\" 0xFFFF \"user=test\"\n"
	if _, err := io.WriteString(writer, input); err != nil {
		t.Fatal(err)
	}
	output.waitContains(
		t,
		"FUNCTION_RESULT_BEGIN vnode-get 200 application/json",
	)
	wire := output.String()
	addResult := strings.Index(
		wire,
		"FUNCTION_RESULT_BEGIN vnode-add 202 application/json",
	)
	configCreate := strings.Index(
		wire,
		"CONFIG go.d:vnode:db create running job",
	)
	getResult := strings.Index(
		wire,
		"FUNCTION_RESULT_BEGIN vnode-get 200 application/json",
	)
	if addResult < 0 ||
		configCreate < 0 ||
		getResult < 0 ||
		addResult >= configCreate ||
		configCreate >= getResult ||
		!strings.Contains(
			wire[getResult:],
			`"name":"db","hostname":"db","guid":"22222222-2222-2222-2222-222222222222"`,
		) {
		t.Fatalf("vnode wire flow differs: %q", wire)
	}
	commands <- testProcessControl(processTerminate)
	select {
	case err := <-done:
		if err != nil {
			t.Fatal(err)
		}
	case <-time.After(3 * time.Second):
		t.Fatal("process did not terminate")
	}
	if err := writer.Close(); err != nil {
		t.Fatal(err)
	}
}

func TestProcessCoreRestartsOneInputAndMovesFrameAuthority(t *testing.T) {
	reader, writer := io.Pipe()
	defer writer.Close()
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
		Input: reader, Output: output, FirstGeneration: 1,
		ShutdownTimeout: time.Second, Clock: lifecycle.RealClock{},
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
		Planner: func(runPlannerCapabilities) (jobmgr.Planner, jobmgr.RunFinalizer, error) {
			return runRejectingPlanner{},
				jobmgr.RunFinalizerFunc(func(context.Context, uint64) error { return nil }),
				nil
		},
	})
	if err != nil {
		t.Fatal(err)
	}
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
		if err != nil {
			t.Fatal(err)
		}
	case <-time.After(3 * time.Second):
		t.Fatal("process did not terminate")
	}
	if census := process.ingress.Census(); census.ReaderStarts != 1 ||
		census.State != "contained" ||
		census.RunGeneration != 0 {
		t.Fatalf("process ingress census=%+v", census)
	}
	cleanupsMu.Lock()
	defer cleanupsMu.Unlock()
	if cleanups != 2 {
		t.Fatalf("handler cleanups=%d want=2", cleanups)
	}
}

func TestProcessCoreContainsConstructionFailures(t *testing.T) {
	cases := map[string]struct {
		failAt          int
		restart         bool
		wantCleanups    int
		wantReaderStart int
	}{
		"first generation": {
			failAt: 1, wantCleanups: 1,
		},
		"restart successor": {
			failAt: 2, restart: true,
			wantCleanups: 2, wantReaderStart: 1,
		},
	}
	for name, tc := range cases {
		t.Run(name, func(t *testing.T) {
			reader, writer := io.Pipe()
			events := make(chan string, 8)
			var cleanupsMu sync.Mutex
			cleanups := 0
			plannerCalls := 0
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
				FirstGeneration: 1, ShutdownTimeout: time.Second,
				Clock: lifecycle.RealClock{},
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
				Planner: func(runPlannerCapabilities) (jobmgr.Planner, jobmgr.RunFinalizer, error) {
					plannerCalls++
					if plannerCalls == tc.failAt {
						return nil, nil, errProcessPlannerConstruction
					}
					return runRejectingPlanner{},
						jobmgr.RunFinalizerFunc(func(context.Context, uint64) error { return nil }),
						nil
				},
			})
			if err != nil {
				t.Fatal(err)
			}
			commands := make(chan processControl, 1)
			done := make(chan error, 1)
			go func() {
				done <- process.run(context.Background(), commands)
			}()
			if tc.restart {
				waitProcessEvent(t, events, "publish")
				commands <- testProcessControl(processRestart)
				waitProcessEvent(t, events, "withdraw")
			}
			select {
			case err := <-done:
				if !errors.Is(err, errProcessPlannerConstruction) {
					t.Fatalf("process error=%v", err)
				}
			case <-time.After(3 * time.Second):
				t.Fatal("process did not contain construction failure")
			}
			if err := writer.Close(); err != nil {
				t.Fatal(err)
			}
			if len(events) != 0 {
				t.Fatalf("unexpected process events remain: %d", len(events))
			}
			census := process.ingress.Census()
			if census.State != "contained" ||
				census.RunGeneration != 0 ||
				census.ReaderStarts != tc.wantReaderStart {
				t.Fatalf("process ingress census=%+v", census)
			}
			if census := process.admission.Census(); census.Phase != "closed" {
				t.Fatalf("admission census=%+v", census)
			}
			cleanupsMu.Lock()
			defer cleanupsMu.Unlock()
			if cleanups != tc.wantCleanups {
				t.Fatalf("handler cleanups=%d want=%d", cleanups, tc.wantCleanups)
			}
		})
	}
}

func TestProcessRetirementPreservesRunDirtyCause(t *testing.T) {
	run, err := lifecycle.NewRunSupervisor(
		1,
		lifecycle.RealClock{},
		time.Second,
	)
	if err != nil {
		t.Fatal(err)
	}
	cause := errors.New("discovery shutdown failed")
	if err := run.Dirty(cause); err != nil {
		t.Fatal(err)
	}
	err = (&processCore{}).retireRun(
		context.Background(),
		&runGeneration{run: run},
	)
	if !errors.Is(err, cause) ||
		!strings.Contains(err.Error(), "run did not quiesce") {
		t.Fatalf("retirement lost run failure: %v", err)
	}
}

var errProcessPlannerConstruction = errors.New("process planner construction failed")

type processRecordingWriter struct {
	record func([]byte)
}

func (writer processRecordingWriter) Write(payload []byte) (int, error) {
	writer.record(payload)
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

func (buffer *processSynchronizedBuffer) Write(payload []byte) (int, error) {
	buffer.mu.Lock()
	count, err := buffer.buffer.Write(payload)
	buffer.mu.Unlock()
	select {
	case buffer.writes <- struct{}{}:
	default:
	}
	return count, err
}

func (buffer *processSynchronizedBuffer) String() string {
	buffer.mu.Lock()
	defer buffer.mu.Unlock()
	return buffer.buffer.String()
}

func (buffer *processSynchronizedBuffer) waitContains(
	t *testing.T,
	want string,
) {
	t.Helper()
	timeout := time.NewTimer(3 * time.Second)
	defer timeout.Stop()
	for {
		if strings.Contains(buffer.String(), want) {
			return
		}
		select {
		case <-buffer.writes:
		case <-timeout.C:
			t.Fatalf(
				"process output does not contain %q: %q",
				want,
				buffer.String(),
			)
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
				output:   build.Out,
			}, true, nil
		},
	)
	catalog, err := agentdiscovery.NewProviderCatalog(
		[]agentdiscovery.ProviderFactory{factory},
	)
	if err != nil {
		t.Fatal(err)
	}
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
	output   io.Writer
}

func (discovery processServiceDiscovery) Run(
	ctx context.Context,
	_ chan<- []*confgroup.Group,
) {
	api := netdataapi.New(discovery.output)
	discovery.registry.RegisterPrefix(
		"config",
		"go.d:sd:",
		func(function frameworkfunctions.Function) {
			api.FUNCRESULT(netdataapi.FunctionResult{
				UID: function.UID, Code: "200",
				ContentType:     "application/json",
				ExpireTimestamp: "0",
				Payload:         `{"status":200}`,
			})
			api.CONFIGSTATUS(
				"go.d:sd:test:job",
				"running",
			)
		},
	)
	api.CONFIGCREATE(netdataapi.ConfigOpts{
		ID: "go.d:sd:test", Status: "accepted",
		ConfigType: "template",
		Path:       "/collectors/go.d/ServiceDiscovery",
		SourceType: "internal", Source: "internal",
		SupportedCommands: "get",
	})
	<-ctx.Done()
	discovery.registry.UnregisterPrefix("config", "go.d:sd:")
}

func waitProcessEvent(t *testing.T, events <-chan string, want string) {
	t.Helper()
	select {
	case got := <-events:
		if got != want {
			t.Fatalf("process event=%q want=%q", got, want)
		}
	case <-time.After(2 * time.Second):
		t.Fatalf("timed out waiting for process event %q", want)
	}
}

func testProcessControl(command processCommand) processControl {
	return processControl{
		command: command,
		result:  make(chan error, 1),
	}
}
