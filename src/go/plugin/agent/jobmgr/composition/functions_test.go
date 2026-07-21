// SPDX-License-Identifier: GPL-3.0-or-later

package composition

import (
	"bytes"
	"context"
	"errors"
	"testing"
	"time"

	"github.com/netdata/netdata/go/plugins/pkg/funcapi"
	"github.com/netdata/netdata/go/plugins/plugin/agent/jobmgr"
	functionadapter "github.com/netdata/netdata/go/plugins/plugin/agent/jobmgr/functions"
	"github.com/netdata/netdata/go/plugins/plugin/agent/jobmgr/joboutput"
	"github.com/netdata/netdata/go/plugins/plugin/agent/jobmgr/lifecycle"
	"github.com/netdata/netdata/go/plugins/plugin/framework/collectorapi"
	"github.com/stretchr/testify/require"
)

// stopFunctionAssembly runs the same shutdown sequence the kernel drives in
// production: BeforeFunctionCatalogClose then FinalizeRun.
func stopFunctionAssembly(fa *FunctionAssembly, epoch uint64) error {
	if fa == nil {
		return nil
	}
	if err := fa.BeforeFunctionCatalogClose(
		context.Background(),
		epoch,
	); err != nil {
		return err
	}
	return fa.FinalizeRun(context.Background(), epoch)
}

func TestFunctionAssemblyLifecycle(t *testing.T) {
	var output bytes.Buffer
	frames, err := lifecycle.NewFrameOwner(&output)
	require.NoError(t, err)
	modules := collectorapi.Registry{
		"module": {
			AgentFunctions: func() []funcapi.FunctionConfig {
				return []funcapi.FunctionConfig{{ID: "method"}}
			},
			MethodHandler: func(collectorapi.RuntimeJob) funcapi.MethodHandler {
				return &assemblyTestHandler{}
			},
		},
	}
	assembly, err := NewFunctionAssembly(7, modules, frames)
	require.NoError(t, err)
	catalog, ok := assembly.Catalog().(*functionadapter.Catalog)
	require.True(t, ok)
	mutations := &assemblyMutationPort{catalog: catalog}

	require.NoError(t, assembly.Bind(mutations))

	require.NoError(t, assembly.Activate())

	const registration = "FUNCTION GLOBAL \"module:method\" 60 \"module method data function\" \"top\" 0x0000 100 3\n\n"

	require.EqualValues(t, registration, output.String())

	require.NoError(t, stopFunctionAssembly(assembly, 7))

	require.EqualValues(t, registration+"FUNCTION_DEL GLOBAL \"module:method\"\n\n", output.String())
}

func TestFunctionAssemblyStateGuards(t *testing.T) {
	tests := map[string]struct {
		run func(*FunctionAssembly, *assemblyMutationPort) error
	}{
		"activate before bind": {
			run: func(assembly *FunctionAssembly, _ *assemblyMutationPort) error {
				return assembly.Activate()
			},
		},
		"duplicate bind": {
			run: func(assembly *FunctionAssembly, mutations *assemblyMutationPort) error {
				if err := assembly.Bind(mutations); err != nil {
					return err
				}
				return assembly.Bind(mutations)
			},
		},
		"duplicate activation": {
			run: func(assembly *FunctionAssembly, mutations *assemblyMutationPort) error {
				if err := assembly.Bind(mutations); err != nil {
					return err
				}
				if err := assembly.Activate(); err != nil {
					return err
				}
				return assembly.Activate()
			},
		},
		"bind after stop": {
			run: func(assembly *FunctionAssembly, mutations *assemblyMutationPort) error {
				if err := assembly.Bind(mutations); err != nil {
					return err
				}
				if err := assembly.Activate(); err != nil {
					return err
				}
				if err := stopFunctionAssembly(assembly, 1); err != nil {
					return err
				}
				return assembly.Bind(mutations)
			},
		},
		"finalize before shutdown": {
			run: func(assembly *FunctionAssembly, mutations *assemblyMutationPort) error {
				if err := assembly.Bind(mutations); err != nil {
					return err
				}
				if err := assembly.Activate(); err != nil {
					return err
				}
				return assembly.FinalizeRun(context.Background(), 1)
			},
		},
		"shutdown from wrong generation": {
			run: func(assembly *FunctionAssembly, mutations *assemblyMutationPort) error {
				if err := assembly.Bind(mutations); err != nil {
					return err
				}
				if err := assembly.Activate(); err != nil {
					return err
				}
				return assembly.BeforeFunctionCatalogClose(context.Background(), 2)
			},
		},
	}
	for name, test := range tests {
		t.Run(name, func(t *testing.T) {
			frames, err := lifecycle.NewFrameOwner(&bytes.Buffer{})
			require.NoError(t, err)
			assembly, err := NewFunctionAssembly(1, collectorapi.Registry{}, frames)
			require.NoError(t, err)
			catalog := assembly.Catalog().(*functionadapter.Catalog)

			require.Error(t, test.run(assembly, &assemblyMutationPort{catalog: catalog}))
		})
	}
}

func TestFunctionAssemblyJobHookCapturesExactHandle(t *testing.T) {
	frames, err := lifecycle.NewFrameOwner(&bytes.Buffer{})
	require.NoError(t, err)
	assembly, err := NewFunctionAssembly(
		1,
		collectorapi.Registry{"module": {}},
		frames,
	)
	require.NoError(t, err)
	job := &assemblyTestJob{}
	handle, err := assembly.JobHooks().Prepare(joboutput.PublishedJob{
		Identity: lifecycle.ResourceIdentity{ID: job.FullName(), Generation: 3},
		Job:      job,
	})
	require.NoError(t, err)
	require.NotNil(t, handle)

	require.NoError(t, handle.CloseAndDrain(context.Background()))

	require.NoError(t, handle.Cleanup(context.Background()))
}

func TestFunctionAssemblyCommitsProtectedTransactionAfterShutdownCut(
	t *testing.T,
) {
	harness := newShutdownFunctionHarness(t)
	applyEntered := make(chan struct{})
	applyRelease := make(chan struct{})
	submitted := make(chan error, 1)
	go func() {
		submitted <- harness.kernel.SubmitPreparedAndWait(
			context.Background(),
			jobmgr.Request{
				UID:     "shutdown-function-transaction",
				LaneKey: harness.job.FullName(),
				Source:  lifecycle.SourceJobManager,
				Route:   "internal/test",
			},
			jobmgr.WorkPlan{
				NoResponse: true,
				Transaction: &jobmgr.ResourceTransactionPlan{
					ID:                harness.job.FullName(),
					AllocateSuccessor: true,
					Permit:            harness.permit,
					Prepare: func(
						_ context.Context,
						_ lifecycle.ReadyResource,
						scope lifecycle.ResourceTransactionScope,
						permit lifecycle.LongLivedPermit,
					) (lifecycle.PreparedResourceTransaction, error) {
						return &shutdownFunctionPreparedTransaction{
							scope:   scope,
							permit:  permit,
							handle:  harness.handle,
							entered: applyEntered,
							release: applyRelease,
						}, nil
					},
				},
			},
		)
	}()

	waitShutdownFunctionGate(t, applyEntered, "transaction apply")
	harness.kernel.Stop()
	waitShutdownFunctionStart(t, harness.kernel)
	close(applyRelease)
	require.NoError(t, waitShutdownFunctionResult(
		t,
		submitted,
		"protected transaction",
	))
	waitCtx, cancelWait := context.WithTimeout(
		context.Background(),
		time.Second,
	)
	defer cancelWait()
	require.NoError(t, harness.kernel.Wait(waitCtx))
	require.NoError(t, harness.run.DirtyCause())
	requireFunctionPublicationCycle(t, harness.output)
	harness.requireDrained(t)
}

type shutdownFunctionHarness struct {
	kernel    *jobmgr.CommandKernel
	run       *lifecycle.RunSupervisor
	admission *lifecycle.AdmissionLedger
	uids      *lifecycle.UIDLedger
	tasks     *lifecycle.TaskSupervisor
	output    *bytes.Buffer
	job       *assemblyTestJob
	handle    joboutput.HandlerLifecycle
	permit    lifecycle.LongLivedPlan
}

func newShutdownFunctionHarness(t *testing.T) shutdownFunctionHarness {
	t.Helper()
	output := &bytes.Buffer{}
	frames, err := lifecycle.NewFrameOwner(output)
	require.NoError(t, err)
	modules := collectorapi.Registry{
		"module": {
			SharedFunctions: func() []funcapi.FunctionConfig {
				return []funcapi.FunctionConfig{{ID: "method"}}
			},
			MethodHandler: func(
				collectorapi.RuntimeJob,
			) funcapi.MethodHandler {
				return &assemblyTestHandler{}
			},
		},
	}
	assembly, err := NewFunctionAssembly(1, modules, frames)
	require.NoError(t, err)
	clock := lifecycle.RealClock{}
	run, err := lifecycle.NewRunSupervisor(1, clock, time.Second)
	require.NoError(t, err)
	t.Cleanup(func() { _ = run.FinishShutdown() })
	admission := lifecycle.NewAdmissionLedger()
	uids := lifecycle.NewUIDLedger()
	tasks, err := lifecycle.NewTaskSupervisor(frames)
	require.NoError(t, err)
	kernel, err := jobmgr.NewCommandKernel(
		run,
		admission,
		uids,
		tasks,
		frames,
		clock,
		make(chan lifecycle.AdmissionGrant, 1),
		assembly,
		assembly,
		assembly.Catalog(),
	)
	require.NoError(t, err)
	require.NoError(t, assembly.Bind(kernel))
	require.NoError(t, run.OpenAdmission())
	require.NoError(t, kernel.Start(t.Context()))
	require.NoError(t, assembly.Activate())

	job := &assemblyTestJob{}
	handle, err := assembly.JobHooks().Prepare(joboutput.PublishedJob{
		Identity: lifecycle.ResourceIdentity{
			ID:         job.FullName(),
			Generation: 1,
		},
		Job: job,
	})
	require.NoError(t, err)
	permit := lifecycle.NewJobLongLivedPlan()
	return shutdownFunctionHarness{
		kernel: kernel, run: run, admission: admission, uids: uids,
		tasks: tasks, output: output, job: job, handle: handle,
		permit: permit,
	}
}

func (sfh shutdownFunctionHarness) requireDrained(t *testing.T) {
	t.Helper()
	require.Equal(t, lifecycle.LongLivedCensus{}, sfh.tasks.LongLivedCensus())
	require.NoError(t, sfh.admission.CloseDrained(sfh.run.Generation()))
	closeRunTestUIDs(t, sfh.uids)
}

func waitShutdownFunctionGate(
	t *testing.T,
	entered <-chan struct{},
	name string,
) {
	t.Helper()
	select {
	case <-entered:
	case <-time.After(time.Second):
		require.FailNowf(t, "test failed", "%s did not reach the shutdown gate", name)
	}
}

func waitShutdownFunctionStart(
	t *testing.T,
	kernel *jobmgr.CommandKernel,
) {
	t.Helper()
	ctx, cancel := context.WithTimeout(context.Background(), time.Second)
	defer cancel()
	require.NoError(t, kernel.WaitShutdownStarted(ctx))
}

func waitShutdownFunctionResult(
	t *testing.T,
	result <-chan error,
	name string,
) error {
	t.Helper()
	select {
	case err := <-result:
		return err
	case <-time.After(time.Second):
		require.FailNowf(t, "test failed", "%s did not settle", name)
		return nil
	}
}

func requireFunctionPublicationCycle(
	t *testing.T,
	output *bytes.Buffer,
) {
	t.Helper()
	publishedAt := bytes.Index(
		output.Bytes(),
		[]byte(`FUNCTION GLOBAL "module:method"`),
	)
	withdrawnAt := bytes.Index(
		output.Bytes(),
		[]byte(`FUNCTION_DEL GLOBAL "module:method"`),
	)
	require.GreaterOrEqual(t, publishedAt, 0)
	require.Greater(t, withdrawnAt, publishedAt)
}

type assemblyMutationPort struct {
	catalog *functionadapter.Catalog
}

func (amp *assemblyMutationPort) QuiesceFunctions(
	_ context.Context,
	mutation jobmgr.FunctionCatalogMutation,
) error {
	if amp == nil || amp.catalog == nil {
		return errors.New("nil mutation port")
	}
	if err := amp.catalog.BeginMutation(mutation); err != nil {
		return err
	}
	for {
		progress, err := amp.catalog.AdvanceMutationQuiesce(jobmgr.MaximumFunctionMutationQuantum)
		if err != nil {
			return err
		}
		if progress.Quiesced {
			return nil
		}
	}
}

func (amp *assemblyMutationPort) CommitFunctions(
	_ context.Context,
	mutation jobmgr.FunctionCatalogMutation,
) (uint64, error) {
	if amp == nil || amp.catalog == nil {
		return 0, errors.New("nil mutation port")
	}
	if err := amp.catalog.ResumeMutation(mutation); err != nil {
		return 0, err
	}
	for {
		progress, cleanups, err := amp.catalog.AdvanceMutation(jobmgr.MaximumFunctionMutationQuantum)
		if err != nil {
			return 0, err
		}
		for _, cleanup := range cleanups {
			_, cleanupErr := cleanup.Work(context.Background())
			if err := amp.catalog.CompleteCleanup(cleanup.Ref); err != nil {
				return 0, errors.Join(cleanupErr, err)
			}
		}
		if progress.Done {
			return progress.Version, nil
		}
	}
}

func (amp *assemblyMutationPort) AbortFunctions(
	_ context.Context,
	mutation jobmgr.FunctionCatalogMutation,
) error {
	if amp == nil || amp.catalog == nil {
		return errors.New("nil mutation port")
	}
	return amp.catalog.AbortMutation(mutation)
}

type assemblyTestHandler struct{}

func (*assemblyTestHandler) MethodParams(context.Context, string) ([]funcapi.ParamConfig, error) {
	return nil, nil
}

func (*assemblyTestHandler) Handle(
	context.Context,
	string,
	funcapi.ResolvedParams,
) *funcapi.FunctionResponse {
	return &funcapi.FunctionResponse{Status: 200}
}

func (*assemblyTestHandler) Cleanup(context.Context) {}

type assemblyTestJob struct{}

func (*assemblyTestJob) FullName() string                           { return "module_job" }
func (*assemblyTestJob) ModuleName() string                         { return "module" }
func (*assemblyTestJob) Name() string                               { return "job" }
func (*assemblyTestJob) IsRunning() bool                            { return true }
func (*assemblyTestJob) Collector() any                             { return nil }
func (*assemblyTestJob) AutoDetectionManaged(context.Context) error { return nil }
func (*assemblyTestJob) AutoDetectionEvery() int                    { return 0 }
func (*assemblyTestJob) RetryAutoDetection() bool                   { return false }
func (*assemblyTestJob) CleanupRejected()                           {}
func (*assemblyTestJob) Tick(int)                                   {}
func (*assemblyTestJob) Cleanup()                                   {}
func (*assemblyTestJob) Stop()                                      {}
func (*assemblyTestJob) StartManaged(ready chan<- struct{}) {
	close(ready)
}

type shutdownFunctionReadyResource struct {
	identity    lifecycle.ResourceIdentity
	permit      lifecycle.LongLivedPermit
	handle      joboutput.HandlerLifecycle
	stopEntered chan<- struct{}
	stopRelease <-chan struct{}
}

func (sfrr *shutdownFunctionReadyResource) Identity() lifecycle.ResourceIdentity {
	return sfrr.identity
}

func (sfrr *shutdownFunctionReadyResource) Publish() error {
	return sfrr.handle.Publish()
}

func (sfrr *shutdownFunctionReadyResource) AbortReady(
	ctx context.Context,
) error {
	return errors.Join(
		sfrr.handle.CloseAndDrain(ctx),
		sfrr.handle.Cleanup(ctx),
		sfrr.permit.ReleaseExternal(
			lifecycle.LongLivedEJobResources,
		),
		sfrr.permit.ReleaseBytes(),
		sfrr.permit.Return(),
	)
}

func (sfrr *shutdownFunctionReadyResource) Stop(
	ctx context.Context,
) error {
	if sfrr.stopEntered != nil {
		close(sfrr.stopEntered)
		<-sfrr.stopRelease
	}
	return errors.Join(
		sfrr.handle.CloseAndDrain(ctx),
		sfrr.handle.Cleanup(ctx),
		sfrr.permit.ReleaseExternal(
			lifecycle.LongLivedEJobResources,
		),
		sfrr.permit.ReleaseBytes(),
	)
}

func (sfrr *shutdownFunctionReadyResource) Finalize() error {
	return sfrr.permit.Return()
}

type shutdownFunctionPreparedTransaction struct {
	scope   lifecycle.ResourceTransactionScope
	permit  lifecycle.LongLivedPermit
	handle  joboutput.HandlerLifecycle
	entered chan<- struct{}
	release <-chan struct{}
}

func (sfpt *shutdownFunctionPreparedTransaction) Scope() lifecycle.ResourceTransactionScope {
	return sfpt.scope
}

func (sfpt *shutdownFunctionPreparedTransaction) Apply(
	ctx context.Context,
) (lifecycle.AppliedResourceTransaction, error) {
	close(sfpt.entered)
	<-sfpt.release
	if err := sfpt.permit.ActivateExternal(
		lifecycle.LongLivedEJobResources,
	); err != nil {
		_, disposeErr := sfpt.Dispose(ctx)
		return lifecycle.AppliedResourceTransaction{}, errors.Join(
			err,
			disposeErr,
		)
	}
	ready := &shutdownFunctionReadyResource{
		identity: sfpt.scope.Successor,
		permit:   sfpt.permit,
		handle:   sfpt.handle,
	}
	if err := ready.Publish(); err != nil {
		return lifecycle.AppliedResourceTransaction{}, errors.Join(
			err,
			ready.AbortReady(ctx),
		)
	}
	result, err := lifecycle.NewSealedResult(204, "text/plain", nil)
	if err != nil {
		return lifecycle.AppliedResourceTransaction{}, errors.Join(
			err,
			ready.AbortReady(ctx),
		)
	}
	applied, err := lifecycle.NewAppliedResourceTransaction(
		sfpt.scope,
		lifecycle.ResourceTransactionInstalled,
		ready,
		result,
		func() error { return nil },
	)
	if err != nil {
		return lifecycle.AppliedResourceTransaction{}, errors.Join(
			err,
			ready.AbortReady(ctx),
		)
	}
	return applied, nil
}

func (sfpt *shutdownFunctionPreparedTransaction) Dispose(
	ctx context.Context,
) (lifecycle.ReadyResource, error) {
	return nil, errors.Join(
		sfpt.handle.CloseAndDrain(ctx),
		sfpt.handle.Cleanup(ctx),
		sfpt.permit.AbortUnused(),
	)
}
