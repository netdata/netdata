// SPDX-License-Identifier: GPL-3.0-or-later

package composition

import (
	"bytes"
	"context"
	"errors"
	"fmt"
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

func TestServiceDiscoveryBindingCapturesTypedInvocationOutput(t *testing.T) {
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
			wantError: "result UID differs from invocation",
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

			result, cleanup, err := binding.invoke("uid", true, func() {
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
			ID:      "config",
			Current: lifecycle.ResourceIdentity{ID: "config", Generation: 1},
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
	require.Equal(t, containment.Census{Quarantined: 1}, attempts.Census())
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
			frameworkfunctions.Function{UID: "late-panic"},
			false,
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
		return attempts.Census() == (containment.Census{Quarantined: 1})
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
			frameworkfunctions.Function{UID: "cooperative-cancel"},
			false,
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

func TestServiceDiscoveryRetirementReturnsConfigLocalUnavailableResult(t *testing.T) {
	binding := &serviceDiscoveryBinding{}

	result, cleanup, err := binding.serviceDiscoveryContainmentResult(
		jobmgr.ErrProcessAttemptRetired,
	)
	require.NoError(t, err)
	require.NotNil(t, cleanup)
	applied, err := lifecycle.NewAppliedResourceTransaction(
		lifecycle.ResourceTransactionScope{ID: "config"},
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
	var output bytes.Buffer
	frames, err := lifecycle.NewFrameOwner(&output)
	require.NoError(t, err)
	binding := newServiceDiscoveryTestBinding(t, 1, frames, nil)
	entered := make(chan struct{})
	release := make(chan struct{})
	released := false
	defer func() {
		if !released {
			close(release)
		}
	}()
	binding.RegisterPrefix("config", "go.d:sd:", func(_ context.Context, function frameworkfunctions.Function) {
		close(entered)
		<-release
		binding.FunctionResult(dyncfg.Result{
			UID:         function.UID,
			Code:        200,
			ContentType: "application/json",
		})
	})
	transaction, err := binding.prepare(
		t.Context(),
		functionadapter.HandlerInput{
			UID:    "read-only",
			Method: "config",
			Args:   []string{"go.d:sd:type:job", string(dyncfg.CommandGet)},
		},
		nil,
		lifecycle.ResourceTransactionScope{ID: "go.d:sd:type:job"},
		lifecycle.LongLivedPermit{},
	)
	require.NoError(t, err)
	applied := make(chan error, 1)
	go func() {
		_, err := transaction.Apply(t.Context())
		applied <- err
	}()
	select {
	case <-entered:
	case <-time.After(time.Second):
		require.FailNow(t, "test failed", "read-only handler was not entered")
	}

	binding.ConfigDelete("go.d:sd:type:unrelated")
	require.Equal(t, "CONFIG go.d:sd:type:unrelated delete\n\n", output.String())

	close(release)
	released = true
	require.NoError(t, <-applied)
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
	_, _, err = binding.invoke("next", true, func() {})

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

func TestServiceDiscoveryTransactionDisposeDoesNotInvokeHandler(t *testing.T) {
	var output bytes.Buffer
	frames, err := lifecycle.NewFrameOwner(&output)
	require.NoError(t, err)
	binding := newServiceDiscoveryTestBinding(t, 1, frames, nil)

	invoked := false
	binding.RegisterPrefix("config", "go.d:sd:", func(_ context.Context, function frameworkfunctions.Function) {
		invoked = true
		binding.FunctionResult(dyncfg.Result{
			UID:         function.UID,
			Code:        200,
			ContentType: "application/json",
		})
		binding.ConfigStatus("go.d:sd:type:job", dyncfg.StatusRunning)
	})
	transaction, err := binding.prepare(
		context.Background(),
		functionadapter.HandlerInput{
			UID:    "cancelled",
			Method: "config",
			Args:   []string{"go.d:sd:type:job", "enable"},
		},
		nil,
		lifecycle.ResourceTransactionScope{
			ID: "go.d:sd:type:job",
		},
		lifecycle.LongLivedPermit{},
	)
	require.NoError(t, err)

	current, err := transaction.Dispose(context.Background())
	require.NoError(t, err)
	require.Nil(t, current)
	require.False(t, invoked)
	require.Empty(t, output.String())
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
	attempts := &countingProcessAttemptAuthority{delegate: delegate}
	binding, err := newServiceDiscoveryBinding(1, "go.d", attempts, frames, nil)
	require.NoError(t, err)
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	var calls int

	_, _, err = binding.invokeContained(
		ctx,
		"go.d:sd:type:job",
		frameworkfunctions.Function{UID: "canceled"},
		false,
		func(context.Context) { calls++ },
	)

	require.ErrorIs(t, err, context.Canceled)
	require.Zero(t, attempts.started)
	require.Zero(t, calls)
}

func TestServiceDiscoveryTransactionContainsNonCooperativeHandler(t *testing.T) {
	var output bytes.Buffer
	frames, err := lifecycle.NewFrameOwner(&output)
	require.NoError(t, err)
	binding := newServiceDiscoveryTestBinding(t, 1, frames, nil)
	attempts := binding.attempts.(*containment.Authority)
	entered := make(chan struct{})
	release := make(chan struct{})
	binding.RegisterPrefix("config", "go.d:sd:", func(_ context.Context, function frameworkfunctions.Function) {
		close(entered)
		<-release
		binding.FunctionResult(dyncfg.Result{
			UID:         function.UID,
			Code:        200,
			ContentType: "application/json",
		})
		binding.ConfigStatus("go.d:sd:type:job", dyncfg.StatusRunning)
	})

	transaction, err := binding.prepare(
		context.Background(),
		functionadapter.HandlerInput{
			UID:    "blocked",
			Method: "config",
			Args:   []string{"go.d:sd:type:job", "enable"},
		},
		nil,
		lifecycle.ResourceTransactionScope{ID: "go.d:sd:type:job"},
		lifecycle.LongLivedPermit{},
	)
	require.NoError(t, err)
	ctx, cancel := context.WithCancel(context.Background())
	firstDone := make(chan error, 1)
	go func() {
		_, err := transaction.Apply(ctx)
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
		require.FailNow(t, "test failed", "canceled service discovery command did not settle")
	}

	busy, err := binding.prepare(
		context.Background(),
		functionadapter.HandlerInput{
			UID:    "busy",
			Method: "config",
			Args:   []string{"go.d:sd:type:job", "enable"},
		},
		nil,
		lifecycle.ResourceTransactionScope{ID: "go.d:sd:type:job"},
		lifecycle.LongLivedPermit{},
	)
	require.NoError(t, err)
	applied, err := busy.Apply(context.Background())
	require.NoError(t, err)
	require.Equal(t, 503, applied.ResultStatus())

	close(release)
	require.Eventually(t, func() bool {
		return attempts.Census() == (containment.Census{})
	}, time.Second, time.Millisecond)
	require.Empty(t, output.String())
}

func TestServiceDiscoveryDiagnosticsFollowAppliedCommandWithoutPayload(t *testing.T) {
	const payloadSentinel = "service-discovery-payload-must-not-appear"
	diagnostics := &recordingCompositionDiagnosticObserver{}
	frames, err := lifecycle.NewFrameOwner(&bytes.Buffer{})
	require.NoError(t, err)
	binding := newServiceDiscoveryTestBinding(t, 3, frames, diagnostics)
	binding.RegisterPrefix("config", "go.d:sd:", func(_ context.Context, function frameworkfunctions.Function) {
		binding.FunctionResult(dyncfg.Result{
			UID:         function.UID,
			Code:        200,
			ContentType: "application/json",
		})
		binding.ConfigStatus("go.d:sd:type:job", dyncfg.StatusRunning)
	})
	transaction, err := binding.prepare(
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
	require.Equal(t, 200, completed.ResultStatus)
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
		frameworkfunctions.Function{UID: uid},
		false,
		call,
	)
	require.NoError(t, err)
	require.NotNil(t, cleanup)
	applied, err := lifecycle.NewAppliedResourceTransaction(
		lifecycle.ResourceTransactionScope{ID: "go.d:sd:type:job"},
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
