// SPDX-License-Identifier: GPL-3.0-or-later

package composition

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"io"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/netdata/netdata/go/plugins/pkg/funcapi"
	"github.com/netdata/netdata/go/plugins/pkg/multipath"
	"github.com/netdata/netdata/go/plugins/pkg/netdataapi"
	agentdiscovery "github.com/netdata/netdata/go/plugins/plugin/agent/discovery"
	"github.com/netdata/netdata/go/plugins/plugin/agent/jobmgr"
	functionadapter "github.com/netdata/netdata/go/plugins/plugin/agent/jobmgr/functions"
	"github.com/netdata/netdata/go/plugins/plugin/agent/jobmgr/lifecycle"
	"github.com/netdata/netdata/go/plugins/plugin/agent/secrets/secretstore"
	"github.com/netdata/netdata/go/plugins/plugin/framework/collectorapi"
	"github.com/netdata/netdata/go/plugins/plugin/framework/confgroup"
	"github.com/netdata/netdata/go/plugins/plugin/framework/dyncfg"
	frameworkfunctions "github.com/netdata/netdata/go/plugins/plugin/framework/functions"
	"github.com/netdata/netdata/go/plugins/plugin/framework/vnodes"
	"github.com/stretchr/testify/require"
)

func TestProcessCoreRestartSupersedesInitialStartup(t *testing.T) {
	process, entered, release := newStartupControlTestProcess(t, 1)
	controls := newTestProcessControls(2)
	done := make(chan error, 1)
	go func() {
		done <- process.run(context.Background(), controls)
	}()
	require.EqualValues(t, 1, waitStartupControlStore(t, entered))

	restart := testProcessControl()
	controls.sendRestart(restart)
	select {
	case err := <-restart.result:
		require.NoError(t, err)
	case <-time.After(time.Second):
		require.FailNow(t, "test failed", "restart was unavailable during initial startup")
	}

	terminate := testProcessControl()
	controls.sendTerminate(terminate)
	require.NoError(t, <-terminate.result)
	require.NoError(t, <-done)
	close(release)
}

func TestProcessCoreFencesCleanupOutputBeforeFinalizingOutput(t *testing.T) {
	var output bytes.Buffer
	process, err := newProcessCore(processCoreConfig{
		Input:           strings.NewReader(""),
		Output:          &output,
		ShutdownTimeout: time.Second,
		Modules:         collectorapi.Registry{},
		Jobs:            testRunJobServices(t),
		Discovery:       testRunDiscoveryServices(t),
		Diagnostics:     testProcessDiagnostics(),
	})
	require.NoError(t, err)

	var lateWriteErr error
	process.config.FinalizeOutput = func() {
		_, lateWriteErr = process.cleanupOut.Write([]byte("late cleanup\n"))
	}

	require.NoError(t, process.finalize(nil, nil))
	require.ErrorContains(t, lateWriteErr, "cleanup output is fenced")
	require.Empty(t, output.Bytes())
}

func TestProcessCoreRestartFencesInitialTargetAfterCanceledConstruction(t *testing.T) {
	reader, writer := io.Pipe()
	entered := make(chan struct{})
	release := make(chan struct{})
	var handlerCalls atomic.Int32
	process, err := newProcessCore(processCoreConfig{
		Input:           reader,
		Output:          newProcessSynchronizedBuffer(),
		ShutdownTimeout: time.Second,
		Modules: collectorapi.Registry{
			"module": {
				AgentFunctions: func() []funcapi.FunctionConfig {
					return []funcapi.FunctionConfig{{ID: "method"}}
				},
				MethodHandler: func(collectorapi.RuntimeJob) funcapi.MethodHandler {
					if handlerCalls.Add(1) == 1 {
						close(entered)
						<-release
					}
					return &runTestHandler{cleanup: func() {}}
				},
			},
		},
		Jobs:        testRunJobServices(t),
		Discovery:   testRunDiscoveryServices(t),
		Diagnostics: testProcessDiagnostics(),
	})
	require.NoError(t, err)
	t.Cleanup(func() {
		_ = writer.Close()
		select {
		case <-release:
		default:
			close(release)
		}
	})
	controls := newTestProcessControls(2)
	done := make(chan error, 1)
	go func() {
		done <- process.run(context.Background(), controls)
	}()
	select {
	case <-entered:
	case <-time.After(time.Second):
		require.FailNow(t, "test failed", "initial Function construction did not start")
	}

	restart := testProcessControl()
	controls.sendRestart(restart)
	close(release)
	require.NoError(t, <-restart.result)

	_, err = process.attempts.StartProcessAttempt(context.Background(), jobmgr.ProcessAttemptPlan{
		Identity: jobmgr.ProcessAttemptIdentity{
			Namespace: jobmgr.ProcessAttemptJob,
			Key:       "late-generation-one-work",
			Resource:  "late generation one work",
		},
		Target: 1,
		Work: func(context.Context, jobmgr.ProcessAttemptAdmission) error {
			return nil
		},
	})
	require.ErrorIs(t, err, jobmgr.ErrProcessAttemptRetired)

	terminate := testProcessControl()
	controls.sendTerminate(terminate)
	require.NoError(t, <-terminate.result)
	require.NoError(t, <-done)
}

func TestProcessCoreTerminateInterruptsInitialStartup(t *testing.T) {
	process, entered, release := newStartupControlTestProcess(t, 1)
	controls := newTestProcessControls(2)
	done := make(chan error, 1)
	go func() {
		done <- process.run(context.Background(), controls)
	}()
	require.EqualValues(t, 1, waitStartupControlStore(t, entered))

	terminate := testProcessControl()
	controls.sendTerminate(terminate)
	select {
	case err := <-terminate.result:
		require.NoError(t, err)
	case <-time.After(time.Second):
		require.FailNow(t, "test failed", "termination was unavailable during initial startup")
	}
	require.NoError(t, <-done)
	close(release)
}

func TestProcessCoreTerminateInterruptsSuccessorStartup(t *testing.T) {
	process, entered, release := newStartupControlTestProcess(t, 2)
	controls := newTestProcessControls(2)
	done := make(chan error, 1)
	go func() {
		done <- process.run(context.Background(), controls)
	}()
	require.EqualValues(t, 1, waitStartupControlStore(t, entered))

	restart := testProcessControl()
	controls.sendRestart(restart)
	require.EqualValues(t, 2, waitStartupControlStore(t, entered))

	terminate := testProcessControl()
	controls.sendTerminate(terminate)
	select {
	case err := <-terminate.result:
		require.NoError(t, err)
	case <-time.After(time.Second):
		require.FailNow(t, "test failed", "termination waited behind successor startup")
	}
	require.ErrorIs(t, <-restart.result, ErrProcessStopped)
	require.NoError(t, <-done)
	close(release)
}

func TestProcessCorePublishesKnownRestartFailureBeforeFinalization(t *testing.T) {
	reader, writer := io.Pipe()
	started := make(chan struct{})
	failureKnown := make(chan struct{})
	var builds atomic.Int32
	factory := agentdiscovery.NewProviderFactory(
		"failing-successor",
		func(build agentdiscovery.BuildContext) (agentdiscovery.Discoverer, bool, error) {
			if builds.Add(1) == 2 {
				close(failureKnown)
				panic("successor construction failed")
			}
			return processSignalingDiscovery{started: started}, true, nil
		},
	)
	catalog, err := agentdiscovery.NewProviderCatalog([]agentdiscovery.ProviderFactory{factory})
	require.NoError(t, err)
	process, err := newProcessCore(processCoreConfig{
		Input:           reader,
		Output:          newProcessSynchronizedBuffer(),
		ShutdownTimeout: time.Second,
		Modules:         collectorapi.Registry{},
		Jobs:            testRunJobServices(t),
		Discovery: runDiscoveryServices{
			BuildContext: agentdiscovery.BuildContext{
				Registry: confgroup.Registry{"test": {}},
			},
			Providers: catalog,
		},
		Diagnostics: testProcessDiagnostics(),
	})
	require.NoError(t, err)
	finalizeEntered := make(chan struct{})
	releaseFinalize := make(chan struct{})
	process.config.FinalizeOutput = func() {
		close(finalizeEntered)
		<-releaseFinalize
	}
	t.Cleanup(func() {
		_ = writer.Close()
		select {
		case <-releaseFinalize:
		default:
			close(releaseFinalize)
		}
	})

	controls := newTestProcessControls(1)
	done := make(chan error, 1)
	go func() {
		done <- process.run(context.Background(), controls)
	}()
	select {
	case <-started:
	case <-time.After(time.Second):
		require.FailNow(t, "test failed", "initial discovery provider did not start")
	}

	restart := testProcessControl()
	controls.sendRestart(restart)
	select {
	case <-failureKnown:
	case <-time.After(time.Second):
		require.FailNow(t, "test failed", "successor failure was not reached")
	}
	select {
	case err := <-restart.result:
		require.ErrorContains(t, err, "provider factory panic")
	case <-time.After(100 * time.Millisecond):
		require.FailNow(t, "test failed", "known restart failure was withheld behind finalization")
	}
	select {
	case <-finalizeEntered:
	case <-time.After(time.Second):
		require.FailNow(t, "test failed", "process finalization did not start")
	}
	select {
	case err := <-done:
		require.FailNowf(t, "test failed", "process stopped before finalization was released: %v", err)
	default:
	}
	close(releaseFinalize)
	require.ErrorContains(t, <-done, "provider factory panic")
}

func TestProcessCoreRotationRetainsOldStoreScopeOutsideRun(t *testing.T) {
	reader, writer := io.Pipe()
	output := newProcessSynchronizedBuffer()
	creators, err := secretstore.NewCreatorCatalog([]secretstore.Creator{{
		Kind:   secretstore.KindVault,
		Schema: `{}`,
		Create: func() secretstore.Store {
			return &processSecretStore{}
		},
	}})
	require.NoError(t, err)
	jobs := testRunJobServices(t)
	jobs.StoreCreators = creators
	process, err := newProcessCore(processCoreConfig{
		Input:           reader,
		Output:          output,
		ShutdownTimeout: time.Second,
		Modules:         collectorapi.Registry{},
		Jobs:            jobs,
		Secrets: runSecretServices{
			Initial: []secretstore.Config{{
				"name":            "main",
				"kind":            string(secretstore.KindVault),
				"value":           "old",
				"__source__":      confgroup.TypeUser,
				"__source_type__": confgroup.TypeUser,
			}},
		},
		Discovery:   testRunDiscoveryServices(t),
		Diagnostics: testProcessDiagnostics(),
	})
	require.NoError(t, err)
	controls := newTestProcessControls(2)
	done := make(chan error, 1)
	go func() {
		done <- process.run(context.Background(), controls)
	}()
	t.Cleanup(func() {
		_ = writer.Close()
	})
	select {
	case err := <-done:
		require.NoError(t, err)
		require.FailNow(t, "test failed", "process stopped before publishing its Store configuration")
	case <-time.After(25 * time.Millisecond):
	}

	output.waitContains(t, "CONFIG go.d:secretstore:vault:main create running job")
	oldEpoch, ok := process.storeEpochs.testLookup(1)
	require.True(t, ok)
	key := secretstore.StoreKey(secretstore.KindVault, "main")
	scope, err := oldEpoch.acquireScope([]string{key})
	require.NoError(t, err)

	restart := testProcessControl()
	controls.sendRestart(restart)
	select {
	case err := <-restart.result:
		require.NoError(t, err)
	case <-time.After(2 * time.Second):
		require.FailNow(t, "test failed", "restart waited for a process-owned Store scope")
	}

	require.True(t, oldEpoch.store.Census().Closing)
	select {
	case <-oldEpoch.done():
		require.FailNow(t, "test failed", "old Store epoch closed while its scope remained live")
	default:
	}
	value, err := scope.Resolve(t.Context(), key, "key")
	require.NoError(t, err)
	require.Equal(t, "old", string(value))
	_, ok = process.storeEpochs.testLookup(2)
	require.True(t, ok)

	require.NoError(t, scope.Release(t.Context()))
	select {
	case <-oldEpoch.done():
	case <-time.After(time.Second):
		require.FailNow(t, "test failed", "old Store epoch was retained after its scope drained")
	}

	terminate := testProcessControl()
	controls.sendTerminate(terminate)
	require.NoError(t, <-terminate.result)
	require.NoError(t, <-done)
}

func TestProcessCoreRotationRejectsUnownedStoreEpoch(t *testing.T) {
	run, err := lifecycle.NewRunSupervisor(1, lifecycle.RealClock{}, time.Second)
	require.NoError(t, err)
	process := &processCore{
		storeEpochs: &processSecretEpochs{
			diagnostics: testProcessDiagnostics(),
			epochs:      make(map[uint64]*processSecretEpoch),
		},
	}
	current := &runGeneration{
		run:         run,
		secretEpoch: &processSecretEpoch{generation: 1},
	}

	err = process.retireForSuccessor(t.Context(), current, 1, 2)

	require.ErrorContains(t, err, "secret Store epoch is not process-owned")
}

func TestProcessTransitionCancellationClassificationRejectsMixedFailures(t *testing.T) {
	cleanupErr := errors.New("cleanup failed")
	tests := map[string]struct {
		err  error
		want bool
	}{
		"none": {},
		"transition cut": {
			err:  errProcessTransitionInterrupted,
			want: true,
		},
		"context cancellation": {
			err:  context.Canceled,
			want: true,
		},
		"joined expected cuts": {
			err:  errors.Join(errProcessTransitionInterrupted, context.Canceled),
			want: true,
		},
		"mixed cleanup failure": {
			err: errors.Join(errProcessTransitionInterrupted, context.Canceled, cleanupErr),
		},
		"deadline is not a transition cut": {
			err: context.DeadlineExceeded,
		},
	}
	for name, test := range tests {
		t.Run(name, func(t *testing.T) {
			require.Equal(t, test.want, processTransitionCancellationOnly(test.err))
		})
	}
}

func TestProcessControlErrorClassificationChecksEveryLeaf(t *testing.T) {
	unexpected := errors.New("unexpected")
	tests := map[string]struct {
		err     error
		allowed []error
		want    bool
	}{
		"nil": {
			allowed: []error{context.Canceled},
		},
		"single": {
			err:     context.Canceled,
			allowed: []error{context.Canceled},
			want:    true,
		},
		"wrapped": {
			err:     fmt.Errorf("wrapped: %w", context.Canceled),
			allowed: []error{context.Canceled},
			want:    true,
		},
		"joined allowed": {
			err:     errors.Join(context.Canceled, context.DeadlineExceeded),
			allowed: []error{context.Canceled, context.DeadlineExceeded},
			want:    true,
		},
		"mixed": {
			err:     errors.Join(context.Canceled, unexpected),
			allowed: []error{context.Canceled},
		},
	}
	for name, test := range tests {
		t.Run(name, func(t *testing.T) {
			require.Equal(t, test.want, ContainsOnlyProcessControlErrors(test.err, test.allowed...))
		})
	}
}

func TestProcessRestartRecoveryDispositionRejectsMixedFailures(t *testing.T) {
	unexpected := errors.New("unexpected")
	tests := map[string]struct {
		err  error
		want error
	}{
		"deadline": {
			err:  context.DeadlineExceeded,
			want: context.DeadlineExceeded,
		},
		"restart required": {
			err:  ErrProcessRestartRequired,
			want: ErrProcessRestartRequired,
		},
		"initial cancellation then deadline": {
			err: errors.Join(
				errProcessTransitionInterrupted,
				context.Canceled,
				context.DeadlineExceeded,
			),
			want: context.DeadlineExceeded,
		},
		"deadline-generated nonquiescence": {
			err: errors.Join(
				context.DeadlineExceeded,
				jobmgr.ErrShutdownDeadlineExceeded,
				lifecycle.ErrRunTerminalNonQuiescent,
				errRunDidNotQuiesce,
			),
			want: context.DeadlineExceeded,
		},
		"deadline plus restart required": {
			err:  errors.Join(context.DeadlineExceeded, ErrProcessRestartRequired),
			want: context.DeadlineExceeded,
		},
		"cancellation only": {
			err: errors.Join(errProcessTransitionInterrupted, context.Canceled),
		},
		"nonquiescence without deadline": {
			err: errors.Join(
				jobmgr.ErrShutdownDeadlineExceeded,
				lifecycle.ErrRunTerminalNonQuiescent,
				errRunDidNotQuiesce,
			),
		},
		"deadline plus unexpected": {
			err: errors.Join(context.DeadlineExceeded, unexpected),
		},
		"deadline-generated nonquiescence plus unexpected": {
			err: errors.Join(
				context.DeadlineExceeded,
				jobmgr.ErrShutdownDeadlineExceeded,
				lifecycle.ErrRunTerminalNonQuiescent,
				errRunDidNotQuiesce,
				unexpected,
			),
		},
		"restart required plus unexpected": {
			err: errors.Join(ErrProcessRestartRequired, unexpected),
		},
	}
	for name, test := range tests {
		t.Run(name, func(t *testing.T) {
			disposition := processRestartRecoveryDisposition(test.err)
			if test.want == nil {
				require.NoError(t, disposition)
				return
			}
			require.ErrorIs(t, disposition, test.want)
		})
	}
}

func TestProcessRestartRequiresProcessExitOnlyForContainmentFailures(t *testing.T) {
	unexpected := errors.New("unexpected")
	tests := map[string]struct {
		err  error
		want bool
	}{
		"quarantined identity": {
			err:  jobmgr.ErrProcessAttemptQuarantined,
			want: true,
		},
		"quarantined worker panic": {
			err: errors.Join(
				jobmgr.ErrProcessAttemptQuarantined,
				jobmgr.ErrProcessAttemptWorkerPanic,
			),
			want: true,
		},
		"quarantined fence panic": {
			err: errors.Join(
				jobmgr.ErrProcessAttemptQuarantined,
				jobmgr.ErrProcessAttemptFencePanic,
			),
			want: true,
		},
		"unexpected failure": {
			err: unexpected,
		},
		"quarantine plus unexpected failure": {
			err: errors.Join(
				jobmgr.ErrProcessAttemptQuarantined,
				unexpected,
			),
		},
	}
	for name, test := range tests {
		t.Run(name, func(t *testing.T) {
			require.Equal(t, test.want, processRestartRequiresProcessExit(test.err))
		})
	}
}

func TestProcessCoreServiceDiscoveryMutationSendsFunctionResultBeforeStatus(t *testing.T) {
	reader, writer := io.Pipe()
	output := newProcessSynchronizedBuffer()
	services := testRunServiceDiscoveryServices(t)
	process, err := newProcessCore(processCoreConfig{
		Input:           reader,
		Output:          output,
		ShutdownTimeout: time.Second,
		Modules:         collectorapi.Registry{},
		Jobs:            testRunJobServices(t),
		Discovery:       services,
		Diagnostics:     testProcessDiagnostics(),
	})
	require.NoError(t, err)
	controls := newTestProcessControls(1)
	done := make(chan error, 1)
	go func() {
		done <- process.run(context.Background(), controls)
	}()
	output.waitContains(t, "CONFIG go.d:sd:test create accepted template")

	_, writeStringErr := io.WriteString(
		writer,
		"FUNCTION sd-enable 30 \"config go.d:sd:test:job enable\" 0xFFFF \"user=test\"\n",
	)
	require.NoError(t, writeStringErr)

	output.waitContains(t, "CONFIG go.d:sd:test:job status running")
	wire := output.String()
	result := strings.Index(wire, "FUNCTION_RESULT_BEGIN sd-enable 200 application/json")
	notification := strings.Index(wire, "CONFIG go.d:sd:test:job status running")
	require.False(t, result < 0 || notification < 0 || result >= notification)
	controls.sendTerminate(testProcessControl())
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
		Input:           reader,
		Output:          output,
		ShutdownTimeout: time.Second,
		Modules:         collectorapi.Registry{},
		Jobs:            jobs,
		Discovery:       testRunDiscoveryServices(t),
		Diagnostics:     testProcessDiagnostics(),
	})
	require.NoError(t, err)
	controls := newTestProcessControls(1)
	done := make(chan error, 1)
	go func() {
		done <- process.run(context.Background(), controls)
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
		!strings.Contains(
			wire[getResult:],
			`"name":"db","hostname":"db","guid":"22222222-2222-2222-2222-222222222222"`,
		))
	controls.sendTerminate(testProcessControl())
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
	diagnostics := &recordingCompositionDiagnosticObserver{}
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
		Input:           reader,
		Output:          output,
		ShutdownTimeout: time.Second,
		Modules: collectorapi.Registry{
			"module": {
				AgentFunctions: func() []funcapi.FunctionConfig {
					return []funcapi.FunctionConfig{{ID: "method"}}
				},
				MethodHandler: func(collectorapi.RuntimeJob) funcapi.MethodHandler {
					return &runTestHandler{
						cleanup: func() {
							cleanupsMu.Lock()
							cleanups++
							cleanupsMu.Unlock()
						},
					}
				},
			},
		},
		Jobs:        testRunJobServices(t),
		Discovery:   testRunDiscoveryServices(t),
		Diagnostics: diagnostics,
	})
	require.NoError(t, err)
	controls := newTestProcessControls(2)
	done := make(chan error, 1)
	go func() {
		done <- process.run(context.Background(), controls)
	}()
	waitProcessEvent(t, events, "publish")
	require.Eventually(t, func() bool {
		for _, event := range diagnostics.snapshot() {
			if event.Name == "job manager generation started" && event.Generation == 1 {
				return true
			}
		}
		return false
	}, time.Second, time.Millisecond)
	restart := testProcessControl()
	controls.sendRestart(restart)
	waitProcessEvent(t, events, "withdraw")
	waitProcessEvent(t, events, "publish")
	require.NoError(t, <-restart.result)
	controls.sendTerminate(testProcessControl())
	waitProcessEvent(t, events, "withdraw")
	select {
	case err := <-done:
		require.NoError(t, err)
	case <-time.After(3 * time.Second):
		require.FailNow(t, "test failed", "process did not terminate")
	}

	require.Equal(t, functionadapter.ProcessIngressContained, process.ingress.State())

	cleanupsMu.Lock()
	defer cleanupsMu.Unlock()
	require.EqualValues(t, 2, cleanups)
	diagnosticEvents := diagnostics.snapshot()
	diagnosticNames := make([]string, 0, len(diagnosticEvents))
	for _, event := range diagnosticEvents {
		diagnosticNames = append(diagnosticNames, event.Name)
	}
	require.Contains(t, diagnosticNames, "job manager generation rotation started")
	require.Contains(t, diagnosticNames, "job manager generation rotation completed")
	require.Contains(t, diagnosticNames, "job manager process stopped")
}

func TestProcessCoreRejectsSuccessorAfterDiscoveryProviderMissesJoin(t *testing.T) {
	reader, writer := io.Pipe()
	defer func() { require.NoError(t, writer.Close()) }()
	started := make(chan struct{})
	release := make(chan struct{})
	defer close(release)
	factory := agentdiscovery.NewProviderFactory(
		"noncooperative",
		func(agentdiscovery.BuildContext) (agentdiscovery.Discoverer, bool, error) {
			return processNoncooperativeDiscovery{
				started: started,
				release: release,
			}, true, nil
		},
	)
	catalog, err := agentdiscovery.NewProviderCatalog([]agentdiscovery.ProviderFactory{factory})
	require.NoError(t, err)
	process, err := newProcessCore(processCoreConfig{
		Input:           reader,
		Output:          newProcessSynchronizedBuffer(),
		ShutdownTimeout: 100 * time.Millisecond,
		Modules:         collectorapi.Registry{},
		Jobs:            testRunJobServices(t),
		Discovery: runDiscoveryServices{
			BuildContext: agentdiscovery.BuildContext{
				Registry: confgroup.Registry{
					"test": {},
				},
			},
			Providers: catalog,
		},
		Diagnostics: testProcessDiagnostics(),
	})
	require.NoError(t, err)
	controls := newTestProcessControls(1)
	done := make(chan error, 1)
	go func() {
		done <- process.run(context.Background(), controls)
	}()
	select {
	case <-started:
	case <-time.After(time.Second):
		require.FailNow(t, "test failed", "discovery provider did not start")
	}
	control := testProcessControl()
	controls.sendRestart(control)
	select {
	case err := <-control.result:
		require.ErrorIs(t, err, context.DeadlineExceeded)
		require.ErrorIs(t, processRestartRecoveryDisposition(err), context.DeadlineExceeded)
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

func TestProcessCoreRequestsFreshProcessForQuarantinedModule(t *testing.T) {
	reader, writer := io.Pipe()
	defer func() { require.NoError(t, writer.Close()) }()
	diagnostics := &recordingCompositionDiagnosticObserver{}
	process, err := newProcessCore(processCoreConfig{
		Input:           reader,
		Output:          newProcessSynchronizedBuffer(),
		ShutdownTimeout: time.Second,
		Modules: collectorapi.Registry{
			"module": {
				AgentFunctions: func() []funcapi.FunctionConfig {
					return []funcapi.FunctionConfig{{ID: "method"}}
				},
				MethodHandler: func(collectorapi.RuntimeJob) funcapi.MethodHandler {
					return &runTestHandler{
						cleanup: func() { panic("cleanup failed") },
					}
				},
			},
		},
		Jobs:        testRunJobServices(t),
		Discovery:   testRunDiscoveryServices(t),
		Diagnostics: diagnostics,
	})
	require.NoError(t, err)
	controls := newTestProcessControls(1)
	done := make(chan error, 1)
	go func() {
		done <- process.run(context.Background(), controls)
	}()
	require.Eventually(t, func() bool {
		for _, event := range diagnostics.snapshot() {
			if event.Name == "job manager generation started" && event.Generation == 1 {
				return true
			}
		}
		return false
	}, time.Second, time.Millisecond)

	ctx, cancel := context.WithTimeout(context.Background(), time.Second)
	defer cancel()
	control := testProcessControl()
	control.ctx = ctx
	controls.sendRestart(control)
	require.ErrorIs(t, <-control.result, ErrProcessRestartRequired)
	require.Error(t, <-done)
}

func TestProcessCoreContainsProviderConstructionPanic(t *testing.T) {
	reader, writer := io.Pipe()
	events := make(chan string, 8)
	var cleanupsMu sync.Mutex
	cleanups := 0
	factory := agentdiscovery.NewProviderFactory(
		"panicked",
		func(agentdiscovery.BuildContext) (agentdiscovery.Discoverer, bool, error) {
			panic("provider construction")
		},
	)
	catalog, err := agentdiscovery.NewProviderCatalog([]agentdiscovery.ProviderFactory{factory})
	require.NoError(t, err)
	process, err := newProcessCore(processCoreConfig{
		Input: reader,
		Output: processRecordingWriter{
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
		},
		ShutdownTimeout: time.Second,
		Modules: collectorapi.Registry{
			"module": {
				AgentFunctions: func() []funcapi.FunctionConfig {
					return []funcapi.FunctionConfig{{ID: "method"}}
				},
				MethodHandler: func(collectorapi.RuntimeJob) funcapi.MethodHandler {
					return &runTestHandler{
						cleanup: func() {
							cleanupsMu.Lock()
							cleanups++
							cleanupsMu.Unlock()
						},
					}
				},
			},
		},
		Jobs: testRunJobServices(t),
		Discovery: runDiscoveryServices{
			BuildContext: agentdiscovery.BuildContext{
				Registry: confgroup.Registry{
					"module": {},
				},
			},
			Providers: catalog,
		},
		Diagnostics: testProcessDiagnostics(),
	})
	require.NoError(t, err)
	done := make(chan error, 1)
	go func() {
		done <- process.run(context.Background(), newTestProcessControls(1))
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
	require.Equal(t, functionadapter.ProcessIngressContained, process.ingress.State())
	cleanupsMu.Lock()
	defer cleanupsMu.Unlock()
	require.EqualValues(t, 1, cleanups)
}

func TestProcessRetirementPreservesRunDirtyCause(t *testing.T) {
	run, err := lifecycle.NewRunSupervisor(1, lifecycle.RealClock{}, time.Second)
	require.NoError(t, err)
	cause := errors.New("discovery shutdown failed")

	run.Dirty(cause)

	err = (&processCore{}).retireRun(context.Background(), &runGeneration{
		run: run,
	})
	require.False(t, !errors.Is(err, cause) || !strings.Contains(err.Error(), "run did not quiesce"))
}

func newStartupControlTestProcess(
	t *testing.T,
	blockStart int,
) (*processCore, <-chan int, chan struct{}) {
	t.Helper()
	entered := make(chan int, 4)
	release := make(chan struct{})
	var starts int
	var startsMu sync.Mutex
	creators, err := secretstore.NewCreatorCatalog([]secretstore.Creator{{
		Kind:   secretstore.KindVault,
		Schema: `{}`,
		Create: func() secretstore.Store {
			return &processStartupControlStore{
				blockStart: blockStart,
				entered:    entered,
				release:    release,
				nextStart: func() int {
					startsMu.Lock()
					defer startsMu.Unlock()
					starts++
					return starts
				},
			}
		},
	}})
	require.NoError(t, err)
	jobs := testRunJobServices(t)
	jobs.StoreCreators = creators
	reader, writer := io.Pipe()
	process, err := newProcessCore(processCoreConfig{
		Input:           reader,
		Output:          newProcessSynchronizedBuffer(),
		ShutdownTimeout: time.Second,
		Modules:         collectorapi.Registry{},
		Jobs:            jobs,
		Secrets: runSecretServices{
			Initial: []secretstore.Config{{
				"name":            "main",
				"kind":            string(secretstore.KindVault),
				"__source__":      confgroup.TypeUser,
				"__source_type__": confgroup.TypeUser,
			}},
		},
		Discovery:   testRunDiscoveryServices(t),
		Diagnostics: testProcessDiagnostics(),
	})
	require.NoError(t, err)
	t.Cleanup(func() {
		_ = writer.Close()
		select {
		case <-release:
		default:
			close(release)
		}
	})
	return process, entered, release
}

func waitStartupControlStore(t *testing.T, entered <-chan int) int {
	t.Helper()
	select {
	case start := <-entered:
		return start
	case <-time.After(time.Second):
		require.FailNow(t, "test failed", "initial Store materialization did not start")
		return 0
	}
}

type processStartupControlStore struct {
	blockStart int
	entered    chan<- int
	release    <-chan struct{}
	nextStart  func() int
}

func (*processStartupControlStore) Configuration() any {
	return &struct{}{}
}

func (pscs *processStartupControlStore) Init(ctx context.Context) error {
	start := pscs.nextStart()
	pscs.entered <- start
	if start != pscs.blockStart {
		return nil
	}
	select {
	case <-ctx.Done():
		return ctx.Err()
	case <-pscs.release:
		return nil
	}
}

func (*processStartupControlStore) Publish() secretstore.PublishedStore {
	return processPublishedSecret("test")
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

func (psb *processSynchronizedBuffer) waitContains(t *testing.T, want string) {
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

func testRunServiceDiscoveryServices(t testing.TB) runDiscoveryServices {
	t.Helper()
	factory := agentdiscovery.NewProviderFactory(
		"service-discovery-test",
		func(build agentdiscovery.BuildContext) (agentdiscovery.Discoverer, bool, error) {
			return processServiceDiscovery{
				registry: build.FnReg,
				output:   build.DyncfgOutput,
			}, true, nil
		},
	)
	catalog, err := agentdiscovery.NewProviderCatalog([]agentdiscovery.ProviderFactory{factory})
	require.NoError(t, err)
	return runDiscoveryServices{
		BuildContext: agentdiscovery.BuildContext{
			Registry: confgroup.Registry{
				"test": {},
			},
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

type processSignalingDiscovery struct {
	started chan<- struct{}
}

func (psd processSignalingDiscovery) Run(ctx context.Context, _ chan<- []*confgroup.Group) {
	close(psd.started)
	<-ctx.Done()
}

func (pnd processNoncooperativeDiscovery) Run(context.Context, chan<- []*confgroup.Group) {
	close(pnd.started)
	<-pnd.release
}

func (psd processServiceDiscovery) Run(ctx context.Context, _ chan<- []*confgroup.Group) {
	psd.registry.RegisterPrefix(
		"config",
		"go.d:sd:",
		func(_ context.Context, function frameworkfunctions.Function) {
			psd.output.FunctionResult(dyncfg.Result{
				UID:         function.UID,
				Code:        200,
				ContentType: "application/json",
				Payload:     `{"status":200}`,
			})
			psd.output.ConfigStatus("go.d:sd:test:job", dyncfg.StatusRunning)
		},
	)
	psd.output.ConfigCreate(netdataapi.ConfigOpts{
		ID:                "go.d:sd:test",
		Status:            "accepted",
		ConfigType:        "template",
		Path:              "/collectors/go.d/ServiceDiscovery",
		SourceType:        "internal",
		Source:            "internal",
		SupportedCommands: "enable",
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

func testProcessControl() processControl {
	return processControl{
		ctx:    context.Background(),
		result: make(chan error, 1),
	}
}

func newTestProcessControls(capacity int) processControls {
	return processControls{
		restart:   make(chan processControl, capacity),
		terminate: make(chan processControl, capacity),
	}
}

func (pc processControls) sendRestart(control processControl) {
	pc.restart <- control
}

func (pc processControls) sendTerminate(control processControl) {
	pc.terminate <- control
}
