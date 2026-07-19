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
	secretresolver "github.com/netdata/netdata/go/plugins/plugin/agent/secrets/resolver"
	"github.com/netdata/netdata/go/plugins/plugin/agent/secrets/secretstore"
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

func TestProcessCoreRejectsSuccessorAfterUnquiescedPredecessor(
	t *testing.T,
) {
	reader, writer := io.Pipe()
	defer writer.Close()
	events := make(chan string, 8)
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
	jobs := testRunJobServices(t)
	var err error
	jobs.StoreCreators, err = secretstore.NewCreatorCatalog(
		[]secretstore.Creator{{
			Kind:        secretstore.KindVault,
			DisplayName: "Vault",
			Schema:      `{}`,
			Create: func() secretstore.Store {
				return &processSecretStore{}
			},
		}},
	)
	if err != nil {
		t.Fatal(err)
	}
	initialStore := secretstore.Config{
		"name": "main", "kind": string(secretstore.KindVault),
		"value":           "initial",
		"__source__":      confgroup.TypeUser,
		"__source_type__": confgroup.TypeUser,
	}
	plannerCalls := 0
	var storeScope func(
		[]string,
	) (secretresolver.AtomicScope, error)
	var storeCensus func() secretstore.SecretStoreCensus
	process, err := newProcessCore(processCoreConfig{
		Input: reader, Output: output, FirstGeneration: 1,
		ShutdownTimeout: time.Second, Clock: lifecycle.RealClock{},
		Modules: collectorapi.Registry{
			"module": {
				AgentFunctions: func() []funcapi.FunctionConfig {
					return []funcapi.FunctionConfig{{ID: "method"}}
				},
				MethodHandler: func(
					collectorapi.RuntimeJob,
				) funcapi.MethodHandler {
					return &runTestHandler{
						cleanup: func() {},
					}
				},
			},
		},
		Jobs: jobs,
		Secrets: runSecretServices{
			Initial: []secretstore.Config{initialStore},
		},
		Discovery: testRunDiscoveryServices(t),
		Planner: func(
			capabilities runPlannerCapabilities,
		) (jobmgr.Planner, jobmgr.RunFinalizer, error) {
			plannerCalls++
			storeScope = capabilities.StoreScope
			storeCensus = capabilities.StoreCensus
			return runRejectingPlanner{},
				jobmgr.RunFinalizerFunc(
					func(context.Context, uint64) error {
						return nil
					},
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
	waitProcessEvent(t, events, "publish")
	if storeScope == nil || storeCensus == nil {
		t.Fatal("planner did not receive Store observation capabilities")
	}
	key := secretstore.StoreKey(secretstore.KindVault, "main")
	var retained secretresolver.AtomicScope
	deadline := time.Now().Add(3 * time.Second)
	for retained == nil {
		retained, err = storeScope([]string{key})
		if err == nil {
			break
		}
		if time.Now().After(deadline) {
			t.Fatalf("initial Store was not published: %v", err)
		}
		time.Sleep(time.Millisecond)
	}
	defer func() {
		if retained != nil {
			if err := retained.Release(context.Background()); err != nil {
				t.Errorf("release retained Store scope: %v", err)
			}
		}
	}()
	if census := storeCensus(); census.Current != 1 ||
		census.Generations != 1 ||
		census.Readers != 1 ||
		census.Scopes != 1 {
		t.Fatalf("retained predecessor Store census=%+v", census)
	}
	control := testProcessControl(processRestart)
	commands <- control
	waitProcessEvent(t, events, "withdraw")
	select {
	case err := <-control.result:
		if err == nil ||
			!strings.Contains(
				err.Error(),
				"secretstore: close with retained ownership",
			) ||
			!strings.Contains(
				err.Error(),
				"jobmgr composition: run did not quiesce",
			) {
			t.Fatalf("restart error=%v", err)
		}
	case <-time.After(3 * time.Second):
		t.Fatal("restart did not reject unquiesced predecessor")
	}
	select {
	case err := <-done:
		if err == nil ||
			!strings.Contains(
				err.Error(),
				"secretstore: close with retained ownership",
			) {
			t.Fatalf("process error=%v", err)
		}
	case <-time.After(3 * time.Second):
		t.Fatal("process did not exit after rejected publication")
	}
	if plannerCalls != 1 {
		t.Fatalf(
			"successor construction reached planner: calls=%d",
			plannerCalls,
		)
	}
	select {
	case event := <-events:
		t.Fatalf("successor produced event %q", event)
	default:
	}
	if census := process.ingress.Census(); census.State != "contained" ||
		census.RunGeneration != 0 {
		t.Fatalf("process ingress census=%+v", census)
	}
	if census := storeCensus(); census.Current != 0 ||
		census.Retiring != 1 ||
		census.Generations != 1 ||
		census.Readers != 1 ||
		census.Scopes != 1 ||
		!census.Closing {
		t.Fatalf("unquiesced predecessor Store census=%+v", census)
	}
	if err := retained.Release(context.Background()); err != nil {
		t.Fatal(err)
	}
	retained = nil
	if census := storeCensus(); census != (secretstore.SecretStoreCensus{
		Closing: true,
	}) {
		t.Fatalf("released predecessor Store census=%+v", census)
	}
}

func TestProcessCoreRejectsSuccessorAfterDiscoveryProviderMissesJoin(
	t *testing.T,
) {
	reader, writer := io.Pipe()
	defer writer.Close()
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
	if err != nil {
		t.Fatal(err)
	}
	plannerCalls := 0
	process, err := newProcessCore(processCoreConfig{
		Input: reader, Output: newProcessSynchronizedBuffer(),
		FirstGeneration: 1, ShutdownTimeout: 100 * time.Millisecond,
		Clock: lifecycle.RealClock{}, Modules: collectorapi.Registry{},
		Jobs: testRunJobServices(t),
		Discovery: runDiscoveryServices{
			BuildContext: agentdiscovery.BuildContext{
				Registry: confgroup.Registry{"test": {}},
			},
			Providers: catalog,
		},
		Planner: func(
			runPlannerCapabilities,
		) (jobmgr.Planner, jobmgr.RunFinalizer, error) {
			plannerCalls++
			return runRejectingPlanner{},
				jobmgr.RunFinalizerFunc(
					func(context.Context, uint64) error {
						return nil
					},
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
	select {
	case <-started:
	case <-time.After(time.Second):
		t.Fatal("discovery provider did not start")
	}
	control := testProcessControl(processRestart)
	commands <- control
	select {
	case err := <-control.result:
		if err == nil ||
			!strings.Contains(
				err.Error(),
				"jobmgr composition: run did not quiesce",
			) ||
			!errors.Is(
				err,
				context.DeadlineExceeded,
			) {
			t.Fatalf("restart error=%v", err)
		}
	case <-time.After(time.Second):
		t.Fatal("restart did not reject retained discovery provider")
	}
	select {
	case err := <-done:
		if err == nil ||
			!strings.Contains(
				err.Error(),
				"jobmgr composition: run did not quiesce",
			) {
			t.Fatalf("process error=%v", err)
		}
	case <-time.After(time.Second):
		t.Fatal("process did not exit after retained discovery provider")
	}
	if plannerCalls != 1 {
		t.Fatalf(
			"successor construction reached planner: calls=%d",
			plannerCalls,
		)
	}
}

func TestProcessCoreContainsConstructionFailures(t *testing.T) {
	cases := map[string]struct {
		failAt             int
		restart            bool
		providerBuildPanic bool
		wantErr            error
		wantErrorText      string
		wantCleanups       int
		wantReaderStart    int
	}{
		"first generation": {
			failAt: 1, wantErr: errProcessPlannerConstruction,
			wantCleanups: 1,
		},
		"restart successor": {
			failAt: 2, restart: true,
			wantErr:      errProcessPlannerConstruction,
			wantCleanups: 2, wantReaderStart: 1,
		},
		"provider build panic": {
			providerBuildPanic: true,
			wantErrorText:      "provider factory panic",
			wantCleanups:       1,
		},
	}
	for name, tc := range cases {
		t.Run(name, func(t *testing.T) {
			reader, writer := io.Pipe()
			events := make(chan string, 8)
			var cleanupsMu sync.Mutex
			cleanups := 0
			plannerCalls := 0
			discovery := testRunDiscoveryServices(t)
			if tc.providerBuildPanic {
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
				if err != nil {
					t.Fatal(err)
				}
				discovery = runDiscoveryServices{
					BuildContext: agentdiscovery.BuildContext{
						Registry: confgroup.Registry{"module": {}},
					},
					Providers: catalog,
				}
			}
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
				Discovery: discovery,
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
			if tc.providerBuildPanic {
				waitProcessEvent(t, events, "publish")
				waitProcessEvent(t, events, "withdraw")
			} else if tc.restart {
				waitProcessEvent(t, events, "publish")
				commands <- testProcessControl(processRestart)
				waitProcessEvent(t, events, "withdraw")
			}
			select {
			case err := <-done:
				if tc.wantErr != nil &&
					!errors.Is(err, tc.wantErr) {
					t.Fatalf("process error=%v", err)
				}
				if tc.wantErrorText != "" &&
					!strings.Contains(err.Error(), tc.wantErrorText) {
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
			t.Fatalf(
				"process output does not contain %q: %q",
				want,
				psb.String(),
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
	api := netdataapi.New(psd.output)
	psd.registry.RegisterPrefix(
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
	psd.registry.UnregisterPrefix("config", "go.d:sd:")
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
