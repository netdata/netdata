// SPDX-License-Identifier: GPL-3.0-or-later

package composition

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"io"
	"testing"
	"time"

	"github.com/netdata/netdata/go/plugins/plugin/agent/jobmgr"
	"github.com/netdata/netdata/go/plugins/plugin/agent/jobmgr/containment"
	functionadapter "github.com/netdata/netdata/go/plugins/plugin/agent/jobmgr/functions"
	"github.com/netdata/netdata/go/plugins/plugin/agent/jobmgr/lifecycle"
	"github.com/netdata/netdata/go/plugins/plugin/framework/dyncfg"
	frameworkfunctions "github.com/netdata/netdata/go/plugins/plugin/framework/functions"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestServiceDiscoveryBindingCapturesReadOnlyResult(t *testing.T) {
	tests := map[string]struct {
		emit              func(*serviceDiscoveryBinding)
		wantResult        string
		wantNotifications string
		wantError         string
	}{
		"result": {
			emit: func(binding *serviceDiscoveryBinding) {
				binding.FunctionResult(dyncfg.Result{
					UID:         "uid",
					Code:        200,
					ContentType: "application/json",
					Payload:     `{"ok":true}`,
				})
			},
			wantResult: "FUNCTION_RESULT_BEGIN result 200 application/json 1\n" +
				"{\"ok\":true}\nFUNCTION_RESULT_END\n\n",
		},
		"result and notification": {
			emit: func(binding *serviceDiscoveryBinding) {
				binding.FunctionResult(dyncfg.Result{
					UID:         "uid",
					Code:        204,
					ContentType: "application/json",
				})
				binding.ConfigStatus("go.d:sd:type:job", dyncfg.StatusRunning)
			},
			wantResult: "FUNCTION_RESULT_BEGIN result 204 application/json 1\n" +
				"FUNCTION_RESULT_END\n\n",
			wantNotifications: "CONFIG go.d:sd:type:job status running\n\n",
		},
		"missing result": {
			emit:      func(*serviceDiscoveryBinding) {},
			wantError: "produced no terminal result",
		},
		"multiple results": {
			emit: func(binding *serviceDiscoveryBinding) {
				result := dyncfg.Result{
					UID:         "uid",
					Code:        200,
					ContentType: "application/json",
				}
				binding.FunctionResult(result)
				binding.FunctionResult(result)
			},
			wantError: "produced multiple results",
		},
		"different result UID": {
			emit: func(binding *serviceDiscoveryBinding) {
				binding.FunctionResult(dyncfg.Result{
					UID:         "other",
					Code:        200,
					ContentType: "application/json",
				})
			},
			wantError: "result outside invocation",
		},
		"handler panic": {
			emit: func(*serviceDiscoveryBinding) {
				panic("failed")
			},
			wantError: "service discovery Function handler: failed",
		},
	}

	for name, test := range tests {
		t.Run(name, func(t *testing.T) {
			var notifications bytes.Buffer
			frames, err := lifecycle.NewFrameOwner(&notifications)
			require.NoError(t, err)
			binding := newServiceDiscoveryTestBinding(t, 1, frames, nil)

			result, cleanup, err := binding.invoke("uid", func() {
				test.emit(binding)
			})
			if test.wantError != "" {
				require.ErrorContains(t, err, test.wantError)
				assert.Nil(t, cleanup)
				return
			}
			require.NoError(t, err)
			require.NotNil(t, cleanup)
			require.NoError(t, cleanup())
			assert.Equal(t, test.wantNotifications, notifications.String())

			var encoded bytes.Buffer
			resultFrames, err := lifecycle.NewFrameOwner(&encoded)
			require.NoError(t, err)
			frame, err := lifecycle.PrepareFrame("result", result, 1)
			require.NoError(t, err)
			require.NoError(t, resultFrames.Commit(frame))
			assert.Equal(t, test.wantResult, encoded.String())
		})
	}
}

func TestServiceDiscoveryBindingRoutesNotificationsOutsideInvocations(t *testing.T) {
	var output bytes.Buffer
	frames, err := lifecycle.NewFrameOwner(&output)
	require.NoError(t, err)
	binding := newServiceDiscoveryTestBinding(t, 1, frames, nil)

	binding.ConfigDelete("go.d:sd:type:gone")

	assert.Equal(t, "CONFIG go.d:sd:type:gone delete\n\n", output.String())
}

func TestServiceDiscoveryQuarantineReturnsConfigLocalUnavailableResult(t *testing.T) {
	binding := &serviceDiscoveryBinding{}

	result, cleanup, err := binding.serviceDiscoveryContainmentResult(
		jobmgr.ErrProcessAttemptQuarantined,
	)
	require.NoError(t, err)
	require.NotNil(t, cleanup)
	applied, err := lifecycle.NewAppliedResourceTransaction(
		lifecycle.ResourceTransactionScope{
			ID: "config",
			Current: lifecycle.ResourceIdentity{
				ID:         "config",
				Generation: 1,
			},
		},
		lifecycle.ResourceTransactionRemoved,
		nil,
		result,
		cleanup,
	)
	require.NoError(t, err)
	require.Equal(t, 503, applied.ResultStatus())
	require.NoError(t, cleanup())
}

func TestServiceDiscoveryHandlerPanicQuarantinesProductionInvocation(t *testing.T) {
	frames, err := lifecycle.NewFrameOwner(&bytes.Buffer{})
	require.NoError(t, err)
	binding := newServiceDiscoveryTestBinding(t, 1, frames, nil)
	attempts := binding.attempts.(*containment.Authority)

	requireServiceDiscoveryInvocationStatus(t, binding, "panicked", 503,
		func(context.Context) {
			panic("handler failed")
		},
	)

	var retryCalls int
	requireServiceDiscoveryInvocationStatus(t, binding, "retry", 503,
		func(context.Context) {
			retryCalls++
		},
	)
	require.Zero(t, retryCalls)
	require.Equal(t, containment.Census{
		Quarantined: 1,
	}, attempts.Census())
}

func TestServiceDiscoveryHandlerPanicAfterContainmentQuarantinesProductionInvocation(t *testing.T) {
	frames, err := lifecycle.NewFrameOwner(&bytes.Buffer{})
	require.NoError(t, err)
	binding := newServiceDiscoveryTestBinding(t, 1, frames, nil)
	attempts := binding.attempts.(*containment.Authority)
	entered := make(chan struct{})
	release := make(chan struct{})
	released := false
	t.Cleanup(func() {
		if !released {
			close(release)
		}
	})
	ctx, cancel := context.WithCancel(context.Background())
	firstDone := make(chan error, 1)
	go func() {
		_, _, err := binding.invokeContained(
			ctx,
			"go.d:sd:type:job",
			frameworkfunctions.Function{
				UID: "late-panic",
			},
			func(context.Context) {
				close(entered)
				<-release
				panic("handler failed after containment")
			},
		)
		firstDone <- err
	}()
	select {
	case <-entered:
	case <-time.After(time.Second):
		require.FailNow(t, "test failed", "service discovery handler was not entered")
	}
	cancel()
	select {
	case err := <-firstDone:
		require.ErrorIs(t, err, context.Canceled)
	case <-time.After(time.Second):
		require.FailNow(t, "test failed", "contained service discovery invocation did not settle")
	}

	close(release)
	released = true
	require.Eventually(t, func() bool {
		return attempts.Census() == (containment.Census{
			Quarantined: 1,
		})
	}, time.Second, time.Millisecond)

	var retryCalls int
	requireServiceDiscoveryInvocationStatus(t, binding, "retry-after-late-panic", 503,
		func(context.Context) {
			retryCalls++
		},
	)
	require.Zero(t, retryCalls)
}

func TestServiceDiscoveryCooperativeCancellationDoesNotQuarantineProductionInvocation(t *testing.T) {
	frames, err := lifecycle.NewFrameOwner(&bytes.Buffer{})
	require.NoError(t, err)
	binding := newServiceDiscoveryTestBinding(t, 1, frames, nil)
	attempts := binding.attempts.(*containment.Authority)
	entered := make(chan struct{})
	ctx, cancel := context.WithCancel(context.Background())
	firstDone := make(chan error, 1)
	go func() {
		_, _, err := binding.invokeContained(
			ctx,
			"go.d:sd:type:job",
			frameworkfunctions.Function{
				UID: "cooperative-cancel",
			},
			func(attemptCtx context.Context) {
				close(entered)
				<-attemptCtx.Done()
			},
		)
		firstDone <- err
	}()
	select {
	case <-entered:
	case <-time.After(time.Second):
		require.FailNow(t, "test failed", "service discovery handler was not entered")
	}
	cancel()
	select {
	case err := <-firstDone:
		require.ErrorIs(t, err, context.Canceled)
	case <-time.After(time.Second):
		require.FailNow(t, "test failed", "contained service discovery invocation did not settle")
	}
	require.Eventually(t, func() bool {
		return attempts.Census() == (containment.Census{})
	}, time.Second, time.Millisecond)

	var retryCalls int
	requireServiceDiscoveryInvocationStatus(t, binding, "retry-after-cooperative-cancel", 200,
		func(context.Context) {
			retryCalls++
			binding.FunctionResult(dyncfg.Result{
				UID:         "retry-after-cooperative-cancel",
				Code:        200,
				ContentType: "application/json",
			})
		},
	)
	require.EqualValues(t, 1, retryCalls)
}

func TestServiceDiscoveryCanceledReadRetainsOnlyItsResource(t *testing.T) {
	frames, err := lifecycle.NewFrameOwner(&bytes.Buffer{})
	require.NoError(t, err)
	binding := newServiceDiscoveryTestBinding(t, 1, frames, nil)
	attempts := binding.attempts.(*containment.Authority)
	entered, release := make(chan struct{}), make(chan struct{})
	released := false
	t.Cleanup(func() {
		if !released {
			close(release)
		}
	})
	ctx, cancel := context.WithCancel(t.Context())
	defer cancel()
	settled := make(chan error, 1)
	go func() {
		_, _, err := binding.invokeContained(
			ctx,
			"job-a",
			frameworkfunctions.Function{
				UID: "old-a",
			},
			func(context.Context) {
				close(entered)
				<-release
				binding.FunctionResult(
					dyncfg.Result{
						UID:         "old-a",
						Code:        200,
						ContentType: "application/json",
						Payload:     `{"owner":"old-a"}`,
					},
				)
			},
		)
		settled <- err
	}()
	select {
	case <-entered:
	case <-time.After(time.Second):
		t.Fatal("read did not enter")
	}
	cancel()
	select {
	case err := <-settled:
		require.ErrorIs(t, err, context.Canceled)
	case <-time.After(time.Second):
		t.Fatal("read cancellation did not settle")
	}
	read := func(resource, uid string, want int, wantCalled bool) {
		t.Helper()
		called := false
		result, cleanup, err := binding.invokeContained(
			t.Context(),
			resource,
			frameworkfunctions.Function{
				UID: uid,
			},
			func(context.Context) {
				called = true
				binding.FunctionResult(
					dyncfg.Result{
						UID:         uid,
						Code:        200,
						ContentType: "application/json",
						Payload:     `{"owner":"` + uid + `"}`,
					},
				)
			},
		)
		require.NoError(t, err)
		require.Equal(t, wantCalled, called)
		require.NoError(t, cleanup())
		frame, err := lifecycle.PrepareFrame(uid, result, 1)
		require.NoError(t, err)
		var wire bytes.Buffer
		writer, err := lifecycle.NewFrameOwner(&wire)
		require.NoError(t, err)
		require.NoError(t, writer.Commit(frame))
		require.Contains(t, wire.String(), fmt.Sprintf("FUNCTION_RESULT_BEGIN %s %d ", uid, want))
		if wantCalled {
			require.Contains(t, wire.String(), `{"owner":"`+uid+`"}`)
		}
	}
	read("job-a", "blocked-a", 503, false)
	read("job-b", "independent-b", 200, true)
	close(release)
	released = true
	require.Eventually(
		t,
		func() bool { return attempts.Census() == (containment.Census{}) },
		time.Second,
		time.Millisecond,
	)
	read("job-a", "new-a", 200, true)
}

func TestServiceDiscoveryRetirementReturnsConfigLocalUnavailableResult(t *testing.T) {
	binding := &serviceDiscoveryBinding{}

	result, cleanup, err := binding.serviceDiscoveryContainmentResult(
		jobmgr.ErrProcessAttemptRetired,
	)
	require.NoError(t, err)
	require.NotNil(t, cleanup)
	applied, err := lifecycle.NewAppliedResourceTransaction(
		lifecycle.ResourceTransactionScope{
			ID: "config",
		},
		lifecycle.ResourceTransactionUnchanged,
		nil,
		result,
		cleanup,
	)
	require.NoError(t, err)
	require.Equal(t, 503, applied.ResultStatus())
	require.NoError(t, cleanup())
}

func TestServiceDiscoveryRetirementDoesNotHideMixedFailure(t *testing.T) {
	binding := &serviceDiscoveryBinding{}
	unexpected := errors.New("unexpected")

	_, cleanup, err := binding.serviceDiscoveryContainmentResult(
		errors.Join(jobmgr.ErrProcessAttemptRetired, unexpected),
	)
	require.ErrorIs(t, err, unexpected)
	require.Nil(t, cleanup)
}

func TestServiceDiscoveryReadOnlyInvocationDoesNotCaptureConfigNotifications(t *testing.T) {
	output := newProcessSynchronizedBuffer()
	frames, err := lifecycle.NewFrameOwner(output)
	require.NoError(t, err)
	binding := newServiceDiscoveryTestBinding(t, 1, frames, nil)
	entered, release := make(chan struct{}), make(chan struct{})
	binding.RegisterPrefix("config", "go.d:sd:", func(_ context.Context, fn frameworkfunctions.Function) {
		close(entered)
		<-release
		binding.FunctionResult(dyncfg.Result{
			UID:         fn.UID,
			Code:        200,
			ContentType: "application/json",
		})
	})
	done := make(chan error, 1)
	go func() {
		transaction, err := binding.prepareUnclaimed(t.Context(), functionadapter.HandlerInput{
			UID:    "read-only",
			Method: "config",
			Args:   []string{"go.d:sd:type:job", "get"},
		}, nil, lifecycle.ResourceTransactionScope{
			ID: "go.d:sd:type:job",
		}, lifecycle.LongLivedPermit{})
		if err == nil {
			_, err = transaction.Apply(t.Context())
		}
		done <- err
	}()
	<-entered
	binding.ConfigDelete("go.d:sd:type:unrelated")
	require.Equal(t, "CONFIG go.d:sd:type:unrelated delete\n\n", output.String())
	close(release)
	require.NoError(t, <-done)
}

func TestServiceDiscoveryBindingRejectsResultOutsideInvocation(t *testing.T) {
	frames, err := lifecycle.NewFrameOwner(&bytes.Buffer{})
	require.NoError(t, err)
	binding := newServiceDiscoveryTestBinding(t, 1, frames, nil)

	binding.FunctionResult(dyncfg.Result{
		UID:         "late",
		Code:        200,
		ContentType: "application/json",
	})
	_, _, err = binding.invoke("next", func() {})

	require.ErrorContains(t, err, "result outside invocation")
}

func TestServiceDiscoveryMutationCommand(t *testing.T) {
	tests := map[string]struct {
		command dyncfg.Command
		want    bool
	}{
		"add": {
			command: dyncfg.CommandAdd,
			want:    true,
		},
		"enable": {
			command: dyncfg.CommandEnable,
			want:    true,
		},
		"disable": {
			command: dyncfg.CommandDisable,
			want:    true,
		},
		"update": {
			command: dyncfg.CommandUpdate,
			want:    true,
		},
		"remove": {
			command: dyncfg.CommandRemove,
			want:    true,
		},
		"restart is unsupported": {
			command: dyncfg.CommandRestart,
		},
		"read-only": {
			command: dyncfg.CommandGet,
		},
		"schema is read-only": {
			command: dyncfg.CommandSchema,
		},
		"test is read-only": {
			command: dyncfg.CommandTest,
		},
		"userconfig is read-only": {
			command: dyncfg.CommandUserconfig,
		},
	}
	for name, test := range tests {
		t.Run(name, func(t *testing.T) {
			assert.Equal(t, test.want, serviceDiscoveryMutationCommand(test.command))
		})
	}
}

func TestServiceDiscoveryHandlerPanicIsClassifiedAsTaskPanic(t *testing.T) {
	err := callServiceDiscoveryHandler(func() {
		panic("handler failed")
	})
	require.ErrorIs(t, err, lifecycle.ErrTaskPanic)
}

func TestServiceDiscoveryTransactionDisposeDoesNotAdopt(t *testing.T) {
	frames, err := lifecycle.NewFrameOwner(&bytes.Buffer{})
	require.NoError(t, err)
	binding := newServiceDiscoveryTestBinding(t, 1, frames, nil)
	applied, disposed := false, false
	registerPreparedSDTestCommand(binding, func(fn dyncfg.Function) (dyncfg.PreparedCommand, error) {
		return &preparedSDTestCommand{
			apply: func(context.Context) (dyncfg.AppliedCommand, error) {
				applied = true
				return dyncfg.AppliedCommand{}, nil
			},
			dispose: func() { disposed = true },
		}, nil
	})
	transaction, err := prepareSDTestTransaction(t.Context(), binding, "cancelled")
	require.NoError(t, err)
	require.False(t, applied)
	current, err := transaction.Dispose(t.Context())
	require.NoError(t, err)
	require.Nil(t, current)
	require.False(t, applied)
	require.True(t, disposed)
}

func TestServiceDiscoveryInvocationDoesNotStartAfterCallerCancellation(t *testing.T) {
	frames, err := lifecycle.NewFrameOwner(&bytes.Buffer{})
	require.NoError(t, err)
	delegate, err := containment.NewAuthority(nil)
	require.NoError(t, err)
	t.Cleanup(func() {
		delegate.BeginShutdown()
		require.NoError(t, delegate.Shutdown(context.Background()))
	})
	attempts := &countingProcessAttemptAuthority{
		delegate: delegate,
	}
	binding, err := newServiceDiscoveryBinding(1, "go.d", attempts, frames, nil)
	require.NoError(t, err)
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	var calls int

	_, _, err = binding.invokeContained(
		ctx,
		"go.d:sd:type:job",
		frameworkfunctions.Function{
			UID: "canceled",
		},
		func(context.Context) { calls++ },
	)

	require.ErrorIs(t, err, context.Canceled)
	require.Zero(t, attempts.started)
	require.Zero(t, calls)
}

func TestServiceDiscoveryPreparationContainsLateCandidateWithoutAdoption(t *testing.T) {
	frames, err := lifecycle.NewFrameOwner(&bytes.Buffer{})
	require.NoError(t, err)
	binding := newServiceDiscoveryTestBinding(t, 1, frames, nil)
	entered, release, disposed := make(chan struct{}), make(chan struct{}), make(chan struct{})
	registerPreparedSDTestCommand(binding, func(fn dyncfg.Function) (dyncfg.PreparedCommand, error) {
		close(entered)
		<-release
		return &preparedSDTestCommand{
			apply:   func(context.Context) (dyncfg.AppliedCommand, error) { panic("late candidate adopted") },
			dispose: func() { close(disposed) },
		}, nil
	})
	done := make(chan lifecycle.PreparedResourceTransaction, 1)
	errs := make(chan error, 1)
	go func() {
		transaction, err := prepareSDTestTransaction(t.Context(), binding, "blocked")
		done <- transaction
		errs <- err
	}()
	<-entered
	require.True(
		t,
		binding.attempts.CutProcessAttempt(
			binding.commandIdentity(frameworkfunctions.Function{
				Args: []string{"go.d:sd:type:job", "update"},
			}),
			jobmgr.ErrProcessAttemptDeadline,
		),
	)
	transaction := <-done
	require.NoError(t, <-errs)
	applied, err := transaction.Apply(t.Context())
	require.NoError(t, err)
	require.Equal(t, 503, applied.ResultStatus())
	busy, err := prepareSDTestTransaction(t.Context(), binding, "busy")
	require.NoError(t, err)
	busyResult, err := busy.Apply(t.Context())
	require.NoError(t, err)
	require.Equal(t, 503, busyResult.ResultStatus())
	close(release)
	select {
	case <-disposed:
	case <-time.After(time.Second):
		t.Fatal("late candidate not disposed")
	}
}

func TestServiceDiscoveryDiagnosticsFollowAppliedCommandWithoutPayload(t *testing.T) {
	const payloadSentinel = "service-discovery-payload-must-not-appear"
	diagnostics := &recordingCompositionDiagnosticObserver{}
	frames, err := lifecycle.NewFrameOwner(&bytes.Buffer{})
	require.NoError(t, err)
	binding := newServiceDiscoveryTestBinding(t, 3, frames, diagnostics)

	registerPreparedSDTestCommand(binding, func(fn dyncfg.Function) (dyncfg.PreparedCommand, error) {
		return &preparedSDTestCommand{
			apply: func(context.Context) (dyncfg.AppliedCommand, error) {
				return dyncfg.AppliedCommand{
					Result: dyncfg.Result{
						UID:         fn.UID(),
						Code:        202,
						ContentType: "application/json",
					},
				}, nil
			},
		}, nil
	})
	transaction, err := binding.prepareUnclaimed(
		context.Background(),
		functionadapter.HandlerInput{
			UID:        "diagnostic-enable",
			Method:     "config",
			Args:       []string{"go.d:sd:type:job", string(dyncfg.CommandEnable)},
			Payload:    []byte(payloadSentinel),
			HasPayload: true,
		},
		nil,
		lifecycle.ResourceTransactionScope{
			ID: "go.d:sd:type:job",
		},
		lifecycle.LongLivedPermit{},
	)
	require.NoError(t, err)
	_, err = transaction.Apply(context.Background())
	require.NoError(t, err)

	events := diagnostics.snapshot()
	var completed *jobmgr.DiagnosticEvent
	for _, event := range events {
		if event.Name == "service discovery configuration command completed" {
			completed = &event
			break
		}
	}
	require.NotNil(t, completed)
	require.Equal(t, jobmgr.DiagnosticInfo, completed.Level)
	require.Equal(t, "go.d:sd:type:job", completed.Resource)
	require.Equal(t, string(dyncfg.CommandEnable), completed.Command)
	require.EqualValues(t, 3, completed.Generation)
	require.Equal(t, 202, completed.ResultStatus)
	require.NotContains(t, fmt.Sprintf("%+v", events), payloadSentinel)
}

func newServiceDiscoveryTestBinding(
	t *testing.T,
	epoch uint64,
	frames *lifecycle.FrameOwner,
	diagnostics jobmgr.DiagnosticObserver,
) *serviceDiscoveryBinding {
	t.Helper()
	attempts, err := containment.NewAuthority(diagnostics)
	require.NoError(t, err)
	t.Cleanup(func() {
		ctx, cancel := context.WithTimeout(context.Background(), time.Second)
		defer cancel()
		require.NoError(t, attempts.Shutdown(ctx))
	})
	binding, err := newServiceDiscoveryBinding(epoch, "go.d", attempts, frames, diagnostics)
	require.NoError(t, err)
	return binding
}

func requireServiceDiscoveryInvocationStatus(
	t *testing.T,
	binding *serviceDiscoveryBinding,
	uid string,
	want int,
	call func(context.Context),
) {
	t.Helper()
	result, cleanup, err := binding.invokeContained(
		t.Context(),
		"go.d:sd:type:job",
		frameworkfunctions.Function{
			UID: uid,
		},
		call,
	)
	require.NoError(t, err)
	require.NotNil(t, cleanup)
	applied, err := lifecycle.NewAppliedResourceTransaction(
		lifecycle.ResourceTransactionScope{
			ID: "go.d:sd:type:job",
		},
		lifecycle.ResourceTransactionUnchanged,
		nil,
		result,
		cleanup,
	)
	require.NoError(t, err)
	require.Equal(t, want, applied.ResultStatus())
}

type countingProcessAttemptAuthority struct {
	delegate jobmgr.ProcessAttemptAuthority
	started  int
}

func (a *countingProcessAttemptAuthority) StartProcessAttempt(
	ctx context.Context,
	plan jobmgr.ProcessAttemptPlan,
) (jobmgr.ProcessAttempt, error) {
	a.started++
	return a.delegate.StartProcessAttempt(ctx, plan)
}

func (a *countingProcessAttemptAuthority) SupersedeProcessAttempt(
	ctx context.Context,
	identity jobmgr.ProcessAttemptIdentity,
) error {
	return a.delegate.SupersedeProcessAttempt(ctx, identity)
}

func (a *countingProcessAttemptAuthority) CutProcessAttempt(
	identity jobmgr.ProcessAttemptIdentity,
	cause error,
) bool {
	return a.delegate.CutProcessAttempt(identity, cause)
}

func (a *countingProcessAttemptAuthority) ProcessAttemptReleased(
	identity jobmgr.ProcessAttemptIdentity,
) (<-chan struct{}, bool) {
	return a.delegate.ProcessAttemptReleased(identity)
}

type preparedSDTestCommand struct {
	apply   func(context.Context) (dyncfg.AppliedCommand, error)
	dispose func()
}

func (c *preparedSDTestCommand) Apply(ctx context.Context) (dyncfg.AppliedCommand, error) {
	return c.apply(ctx)
}
func (c *preparedSDTestCommand) Dispose(context.Context) error {
	if c.dispose != nil {
		c.dispose()
	}
	return nil
}
func registerPreparedSDTestCommand(binding *serviceDiscoveryBinding, prepare dyncfg.CommandPreparer) {
	binding.RegisterPrefix("config", "go.d:sd:", func(context.Context, frameworkfunctions.Function) {})
	binding.RegisterCommandPreparer("config", "go.d:sd:", prepare)
}

func prepareSDTestTransaction(
	ctx context.Context,
	binding *serviceDiscoveryBinding,
	uid string,
) (lifecycle.PreparedResourceTransaction, error) {
	return binding.prepareUnclaimed(ctx, functionadapter.HandlerInput{
		UID:    uid,
		Method: "config",
		Args:   []string{"go.d:sd:type:job", "update"},
	}, nil, lifecycle.ResourceTransactionScope{
		ID: "go.d:sd:type:job",
	}, lifecycle.LongLivedPermit{})
}

func TestServiceDiscoveryCommandOutputPublishesBeforeActivation(t *testing.T) {
	for _, failWrite := range []bool{false, true} {
		t.Run(fmt.Sprintf("write-fails=%t", failWrite), func(t *testing.T) {
			var output bytes.Buffer
			var writer io.Writer = &output
			if failWrite {
				writer = sdFailingWriter{}
			}
			frames, err := lifecycle.NewFrameOwner(writer)
			require.NoError(t, err)
			binding := newServiceDiscoveryTestBinding(t, 1, frames, nil)
			published := false
			result, cleanup, err := binding.prepareCommandOutput("accepted", dyncfg.AppliedCommand{
				Result: dyncfg.Result{
					UID:         "accepted",
					Code:        202,
					ContentType: "application/json",
				},
				Notifications: []dyncfg.Notification{
					{Kind: dyncfg.NotificationStatus, ID: "go.d:sd:type:job", Status: dyncfg.StatusAccepted},
				},
				Published: func() {
					published = true
					binding.ConfigStatus("go.d:sd:type:job", dyncfg.StatusRunning)
				},
			})
			require.NoError(t, err)
			require.False(t, published)
			require.Empty(t, output.String())
			require.NotEqual(t, lifecycle.SealedResult{}, result)
			err = cleanup()
			if failWrite {
				require.Error(t, err)
				require.False(t, published)
				return
			}
			require.NoError(t, err)
			require.True(t, published)
			require.Equal(
				t,
				"CONFIG go.d:sd:type:job status accepted\n\nCONFIG go.d:sd:type:job status running\n\n",
				output.String(),
			)
		})
	}
}

type sdFailingWriter struct{}

func (sdFailingWriter) Write([]byte) (int, error) { return 0, errors.New("test publication failure") }
