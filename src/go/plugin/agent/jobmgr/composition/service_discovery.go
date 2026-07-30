// SPDX-License-Identifier: GPL-3.0-or-later

package composition

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"slices"
	"sync"

	"github.com/netdata/netdata/go/plugins/pkg/netdataapi"
	"github.com/netdata/netdata/go/plugins/plugin/agent/jobmgr"
	functionadapter "github.com/netdata/netdata/go/plugins/plugin/agent/jobmgr/functions"
	"github.com/netdata/netdata/go/plugins/plugin/agent/jobmgr/joboutput"
	"github.com/netdata/netdata/go/plugins/plugin/agent/jobmgr/lifecycle"
	"github.com/netdata/netdata/go/plugins/plugin/framework/dyncfg"
	frameworkfunctions "github.com/netdata/netdata/go/plugins/plugin/framework/functions"
)

const dynCfgServiceDiscoveryClaim = "dyncfg:service-discovery"

var errServiceDiscoveryNoTerminalResult = errors.New(
	"jobmgr composition: service discovery handler produced no terminal result",
)

type serviceDiscoveryBinding struct {
	mu sync.Mutex // guards handler/registered/active/dirty

	pluginName  string // owning plugin name
	epoch       uint64 // run generation
	attemptKey  string // one physical invocation owner per binding
	attempts    jobmgr.ProcessAttemptAuthority
	frames      *lifecycle.FrameOwner       // the one wire frame writer
	diagnostics jobmgr.DiagnosticObserver   // operational log sink
	handler     frameworkfunctions.Handler  // the registered service-discovery handler
	active      *serviceDiscoveryInvocation // current synchronous invocation
	registered  bool                        // the SD Function is registered
	dirty       error                       // sticky error (unexpected registration)
}

type serviceDiscoveryInvocation struct {
	uid                  string
	result               *dyncfg.Result
	captureNotifications bool
	notificationOverflow bool
	notifications        []byte
	err                  error
}

type preparedServiceDiscoveryTransaction struct {
	mu sync.Mutex

	binding  *serviceDiscoveryBinding
	handler  frameworkfunctions.Handler
	function frameworkfunctions.Function
	scope    lifecycle.ResourceTransactionScope
	consumed bool
}

func newServiceDiscoveryBinding(
	epoch uint64,
	pluginName string,
	attempts jobmgr.ProcessAttemptAuthority,
	frames *lifecycle.FrameOwner,
	diagnostics jobmgr.DiagnosticObserver,
) (*serviceDiscoveryBinding, error) {
	if epoch == 0 || pluginName == "" || attempts == nil || frames == nil {
		return nil, errors.New("jobmgr composition: invalid service discovery binding")
	}
	return &serviceDiscoveryBinding{
		pluginName: pluginName,
		epoch:      epoch,
		attemptKey: jobmgr.ProcessAttemptIdentityKey(
			"service-discovery-binding",
			fmt.Sprintf("%d", epoch),
			pluginName,
		),
		attempts:    attempts,
		frames:      frames,
		diagnostics: diagnostics,
	}, nil
}

func (sdb *serviceDiscoveryBinding) prefix() string {
	return sdb.pluginName + ":sd:"
}

func (sdb *serviceDiscoveryBinding) RegisterPrefix(
	name string,
	prefix string,
	fn frameworkfunctions.Handler,
) {
	if fn == nil {
		sdb.recordRegistrationError(errors.New("nil service discovery prefix Function"))
		return
	}
	sdb.mu.Lock()
	if sdb.dirty != nil {
		sdb.mu.Unlock()
		return
	}
	if name != joboutput.DynCfgFunctionName || prefix != sdb.prefix() || fn == nil || sdb.registered {
		sdb.setDirtyLocked(errors.New("jobmgr composition: invalid service discovery Function registration"))
		sdb.mu.Unlock()
		return
	}
	sdb.handler = fn
	sdb.registered = true
	sdb.mu.Unlock()
}

func (sdb *serviceDiscoveryBinding) UnregisterPrefix(name string, prefix string) {
	sdb.mu.Lock()
	if sdb.dirty != nil {
		sdb.mu.Unlock()
		return
	}
	if name != joboutput.DynCfgFunctionName || prefix != sdb.prefix() || !sdb.registered {
		sdb.setDirtyLocked(errors.New("jobmgr composition: invalid service discovery Function withdrawal"))
		sdb.mu.Unlock()
		return
	}
	sdb.handler = nil
	sdb.registered = false
	sdb.mu.Unlock()
}

func (sdb *serviceDiscoveryBinding) recordRegistrationError(err error) {
	sdb.mu.Lock()
	defer sdb.mu.Unlock()
	sdb.setDirtyLocked(err)
}

func (sdb *serviceDiscoveryBinding) prepare(
	ctx context.Context,
	input functionadapter.HandlerInput,
	current lifecycle.ReadyResource,
	scope lifecycle.ResourceTransactionScope,
	permit lifecycle.LongLivedPermit,
) (lifecycle.PreparedResourceTransaction, error) {
	if sdb == nil || ctx == nil || current != nil ||
		scope.Current.Valid() ||
		scope.Successor.Valid() ||
		permit.Valid() ||
		!scope.Valid() {
		return nil, errors.New("jobmgr composition: invalid service discovery transaction scope")
	}
	sdb.mu.Lock()
	handler, dirty := sdb.handler, sdb.dirty
	sdb.mu.Unlock()
	if dirty != nil {
		return nil, dirty
	}
	if handler == nil {
		return joboutput.PrepareNoopResourceTransaction(
			scope,
			nil,
			lifecycle.LongLivedPermit{},
			mustDynCfgMessage(503, "Service discovery configuration is not available."),
			func() error { return nil },
			nil,
		)
	}
	function := frameworkfunctions.Function{
		UID:         input.UID,
		Timeout:     input.Timeout,
		Name:        input.Method,
		Args:        slices.Clone(input.Args),
		Payload:     slices.Clone(input.Payload),
		Permissions: input.Permissions,
		Source:      input.CallerSource,
		ContentType: input.ContentType,
	}
	return &preparedServiceDiscoveryTransaction{
		binding:  sdb,
		handler:  handler,
		function: function,
		scope:    scope,
	}, nil
}

func (psdt *preparedServiceDiscoveryTransaction) Scope() lifecycle.ResourceTransactionScope {
	if psdt == nil {
		return lifecycle.ResourceTransactionScope{}
	}
	psdt.mu.Lock()
	defer psdt.mu.Unlock()
	if psdt.consumed {
		return lifecycle.ResourceTransactionScope{}
	}
	return psdt.scope
}

func (psdt *preparedServiceDiscoveryTransaction) Apply(
	ctx context.Context,
) (lifecycle.AppliedResourceTransaction, error) {
	binding, handler, function, scope, err := psdt.take()
	if err != nil {
		return lifecycle.AppliedResourceTransaction{}, err
	}
	if ctx == nil {
		return lifecycle.AppliedResourceTransaction{}, errors.New(
			"jobmgr composition: nil service discovery apply context",
		)
	}
	command := serviceDiscoveryCommand(function.Args)
	result, cleanup, err := binding.invokeContained(
		ctx,
		scope.ID,
		function,
		serviceDiscoveryMutationCommand(command),
		func(callCtx context.Context) {
			handler(callCtx, function)
		},
	)
	if err != nil {
		binding.observeCommand(command, scope.ID, 0, err)
		return lifecycle.AppliedResourceTransaction{}, err
	}
	applied, err := lifecycle.NewAppliedResourceTransaction(
		scope,
		lifecycle.ResourceTransactionUnchanged,
		nil,
		result,
		cleanup,
	)
	binding.observeCommand(command, scope.ID, applied.ResultStatus(), err)
	return applied, err
}

func (psdt *preparedServiceDiscoveryTransaction) Dispose(context.Context) (lifecycle.ReadyResource, error) {
	_, _, _, _, err := psdt.take()
	return nil, err
}

func (psdt *preparedServiceDiscoveryTransaction) take() (
	*serviceDiscoveryBinding,
	frameworkfunctions.Handler,
	frameworkfunctions.Function,
	lifecycle.ResourceTransactionScope,
	error,
) {
	if psdt == nil {
		return nil, nil, frameworkfunctions.Function{}, lifecycle.ResourceTransactionScope{},
			errors.New("jobmgr composition: nil service discovery transaction")
	}
	psdt.mu.Lock()
	defer psdt.mu.Unlock()
	if psdt.consumed {
		return nil, nil, frameworkfunctions.Function{}, lifecycle.ResourceTransactionScope{},
			errors.New("jobmgr composition: service discovery transaction consumed")
	}
	psdt.consumed = true
	binding, handler, function, scope := psdt.binding, psdt.handler, psdt.function, psdt.scope
	psdt.binding = nil
	psdt.handler = nil
	psdt.function = frameworkfunctions.Function{}
	psdt.scope = lifecycle.ResourceTransactionScope{}
	return binding, handler, function, scope, nil
}

type serviceDiscoveryInvocationResult struct {
	result  lifecycle.SealedResult
	cleanup lifecycle.TaskCleanup
	err     error
}

func (sdb *serviceDiscoveryBinding) invokeContained(
	ctx context.Context,
	resource string,
	function frameworkfunctions.Function,
	captureNotifications bool,
	call func(context.Context),
) (lifecycle.SealedResult, lifecycle.TaskCleanup, error) {
	if sdb == nil || ctx == nil || sdb.attempts == nil ||
		lifecycle.ValidateUID(function.UID) != nil || call == nil {
		return lifecycle.SealedResult{}, nil,
			errors.New("jobmgr composition: invalid contained service discovery invocation")
	}
	if cause := context.Cause(ctx); cause != nil {
		return lifecycle.SealedResult{}, nil, cause
	}
	resultCh := make(chan serviceDiscoveryInvocationResult, 1)
	attempt, err := sdb.attempts.StartProcessAttempt(ctx, jobmgr.ProcessAttemptPlan{
		Identity: jobmgr.ProcessAttemptIdentity{
			Namespace: jobmgr.ProcessAttemptServiceDiscovery,
			Key:       sdb.attemptKey,
			Resource:  serviceDiscoveryDiagnosticResource(resource),
		},
		Target: sdb.epoch,
		Work: func(
			attemptCtx context.Context,
			admission jobmgr.ProcessAttemptAdmission,
		) error {
			result, cleanup, invokeErr := sdb.invoke(
				function.UID,
				captureNotifications,
				func() { call(attemptCtx) },
			)
			// The opaque handler stays fuse-bounded. Its terminal result is
			// admitted only after the callback returns, so handler or
			// protocol failures quarantine the identity.
			// A started command may finish state private to its generation.
			// The identity prevents re-entry and admission gates publication.
			if admitErr := admission.Admit(); admitErr != nil {
				if invokeErr != nil &&
					!errors.Is(invokeErr, errServiceDiscoveryNoTerminalResult) {
					// Containment already settled the caller, so preserve the
					// obscured terminal failure for physical-release quarantine.
					return lifecycle.RetainOwnership(errors.Join(admitErr, invokeErr))
				}
				return admitErr
			}
			resultCh <- serviceDiscoveryInvocationResult{
				result:  result,
				cleanup: cleanup,
				err:     invokeErr,
			}
			return invokeErr
		},
	})
	if err != nil {
		return sdb.serviceDiscoveryContainmentResult(err)
	}
	if err := attempt.Await(ctx); err != nil {
		return sdb.serviceDiscoveryContainmentResult(err)
	}
	select {
	case result := <-resultCh:
		return result.result, result.cleanup, result.err
	default:
		return lifecycle.SealedResult{}, nil,
			errors.New("jobmgr composition: service discovery invocation settled without a result")
	}
}

func (sdb *serviceDiscoveryBinding) serviceDiscoveryContainmentResult(
	err error,
) (lifecycle.SealedResult, lifecycle.TaskCleanup, error) {
	if jobmgr.ContainsOnlyErrorLeaves(
		err,
		jobmgr.ErrProcessAttemptRetired,
		jobmgr.ErrProcessAttemptStopped,
	) {
		return mustDynCfgMessage(
				503,
				"Service discovery configuration is unavailable while the plugin is stopping.",
			),
			func() error { return nil },
			nil
	}
	if errors.Is(err, jobmgr.ErrProcessAttemptBusy) ||
		errors.Is(err, jobmgr.ErrProcessAttemptDeadline) ||
		errors.Is(err, jobmgr.ErrProcessAttemptQuarantined) {
		message := "Service discovery configuration is busy; retry the command."
		if errors.Is(err, jobmgr.ErrProcessAttemptQuarantined) {
			message = "Service discovery configuration is unavailable until the plugin restarts."
		}
		return mustDynCfgMessage(
				503,
				message,
			),
			func() error { return nil },
			nil
	}
	return lifecycle.SealedResult{}, nil, err
}

func serviceDiscoveryDiagnosticResource(resource string) string {
	return jobmgr.ProcessAttemptDiagnosticResource(
		resource,
		"service discovery configuration",
	)
}

func (sdb *serviceDiscoveryBinding) invoke(
	uid string,
	captureNotifications bool,
	call func(),
) (lifecycle.SealedResult, lifecycle.TaskCleanup, error) {
	if sdb == nil || lifecycle.ValidateUID(uid) != nil || call == nil {
		return lifecycle.SealedResult{}, nil, errors.New("jobmgr composition: invalid service discovery invocation")
	}

	invocation := &serviceDiscoveryInvocation{
		uid:                  uid,
		captureNotifications: captureNotifications,
	}
	sdb.mu.Lock()
	if sdb.dirty != nil {
		err := sdb.dirty
		sdb.mu.Unlock()
		return lifecycle.SealedResult{}, nil, err
	}
	if sdb.active != nil {
		sdb.mu.Unlock()
		return lifecycle.SealedResult{}, nil, errors.New(
			"jobmgr composition: concurrent service discovery invocation escaped containment",
		)
	}
	sdb.active = invocation
	sdb.mu.Unlock()

	callErr := callServiceDiscoveryHandler(call)

	sdb.mu.Lock()
	if sdb.active != invocation {
		sdb.mu.Unlock()
		return lifecycle.SealedResult{}, nil,
			errors.Join(callErr, errors.New("jobmgr composition: service discovery invocation changed"))
	}
	sdb.active = nil
	result := invocation.result
	notifications := invocation.notifications
	invocationErr := invocation.err
	sdb.mu.Unlock()

	if err := errors.Join(callErr, invocationErr); err != nil {
		return lifecycle.SealedResult{}, nil, err
	}
	if result == nil {
		return lifecycle.SealedResult{}, nil, errServiceDiscoveryNoTerminalResult
	}
	sealed, err := lifecycle.NewSealedResult(result.Code, result.ContentType, []byte(result.Payload))
	if err != nil {
		return lifecycle.SealedResult{}, nil, err
	}
	cleanup := lifecycle.TaskCleanup(func() error { return nil })
	if len(notifications) != 0 {
		prepared, err := lifecycle.PrepareProtocolFrame(notifications)
		if err != nil {
			return lifecycle.SealedResult{}, nil, err
		}
		cleanup = func() error {
			return sdb.frames.CommitPreparedProtocolFrame(prepared)
		}
	}
	return sealed, cleanup, nil
}

func callServiceDiscoveryHandler(call func()) (err error) {
	defer func() {
		if recovered := recover(); recovered != nil {
			err = fmt.Errorf("%w in service discovery Function handler: %v", lifecycle.ErrTaskPanic, recovered)
		}
	}()
	call()
	return nil
}

func (sdb *serviceDiscoveryBinding) FunctionResult(result dyncfg.Result) {
	sdb.mu.Lock()
	defer sdb.mu.Unlock()

	if sdb.active == nil {
		sdb.setDirtyLocked(errors.New("jobmgr composition: service discovery result outside invocation"))
		return
	}
	if sdb.active.result != nil {
		sdb.active.err = errors.Join(
			sdb.active.err,
			errors.New("jobmgr composition: service discovery handler produced multiple results"),
		)
		return
	}
	if result.UID != sdb.active.uid {
		sdb.active.err = errors.Join(
			sdb.active.err,
			errors.New("jobmgr composition: service discovery result UID differs from invocation"),
		)
		return
	}
	sdb.active.result = &result
}

func (sdb *serviceDiscoveryBinding) ConfigCreate(opts netdataapi.ConfigOpts) {
	sdb.emitNotification(func(output dyncfg.Output) {
		output.ConfigCreate(opts)
	})
}

func (sdb *serviceDiscoveryBinding) ConfigStatus(id string, status dyncfg.Status) {
	sdb.emitNotification(func(output dyncfg.Output) {
		output.ConfigStatus(id, status)
	})
}

func (sdb *serviceDiscoveryBinding) ConfigDelete(id string) {
	sdb.emitNotification(func(output dyncfg.Output) {
		output.ConfigDelete(id)
	})
}

func (sdb *serviceDiscoveryBinding) emitNotification(emit func(dyncfg.Output)) {
	var encoded bytes.Buffer
	emit(dyncfg.NewProtocolOutput(&encoded))
	payload := encoded.Bytes()

	sdb.mu.Lock()
	// Keep the lock through a direct commit to linearize it with invocation-captured notifications.
	// Supported output failures return as errors; this binding is not a panic-recovery boundary.
	if sdb.active == nil || !sdb.active.captureNotifications {
		commitErr := sdb.frames.CommitBorrowedProtocolFrame(payload)
		if commitErr != nil {
			sdb.setDirtyLocked(commitErr)
		}
		sdb.mu.Unlock()
		return
	}
	if sdb.active.notificationOverflow {
		sdb.mu.Unlock()
		return
	}
	if len(payload) > lifecycle.MaximumOtherFrameBytes-len(sdb.active.notifications) {
		boundErr := errors.New("jobmgr composition: service discovery notifications exceed frame bounds")
		sdb.active.notificationOverflow = true
		sdb.active.err = errors.Join(
			sdb.active.err,
			boundErr,
		)
		sdb.mu.Unlock()
		return
	}
	sdb.active.notifications = append(sdb.active.notifications, payload...)
	sdb.mu.Unlock()
}

func (sdb *serviceDiscoveryBinding) setDirtyLocked(err error) {
	if sdb.dirty == nil {
		sdb.dirty = err
	}
}

func (sdb *serviceDiscoveryBinding) observeCommand(
	command dyncfg.Command,
	resource string,
	status int,
	err error,
) {
	if !serviceDiscoveryMutationCommand(command) {
		return
	}
	level := jobmgr.DiagnosticInfo
	name := "service discovery configuration command completed"
	if err != nil || !jobmgr.DiagnosticResultSucceeded(status) {
		level = jobmgr.DiagnosticWarning
		name = "service discovery configuration command failed"
	}
	jobmgr.ObserveDiagnostic(sdb.diagnostics, jobmgr.DiagnosticEvent{
		Level:        level,
		Name:         name,
		Command:      string(command),
		Resource:     resource,
		Generation:   sdb.epoch,
		ResultStatus: status,
		Err:          err,
	})
}

func serviceDiscoveryCommand(args []string) dyncfg.Command {
	return dyncfg.CommandFromArgs(args)
}

func serviceDiscoveryMutationCommand(command dyncfg.Command) bool {
	switch command {
	case dyncfg.CommandAdd,
		dyncfg.CommandEnable,
		dyncfg.CommandDisable,
		dyncfg.CommandUpdate,
		dyncfg.CommandRemove:
		return true
	default:
		return false
	}
}

func newServiceDiscoveryInitialRoute(
	epoch uint64,
	binding *serviceDiscoveryBinding,
) (functionadapter.InitialRoute, error) {
	if epoch == 0 || binding == nil {
		return functionadapter.InitialRoute{}, errors.New("jobmgr composition: invalid service discovery route")
	}
	commands := []functionadapter.ResourceTransactionCommand{
		{Name: string(dyncfg.CommandAdd)},
		{Name: string(dyncfg.CommandSchema)},
		{Name: string(dyncfg.CommandGet)},
		{Name: string(dyncfg.CommandEnable)},
		{Name: string(dyncfg.CommandDisable)},
		{Name: string(dyncfg.CommandUpdate)},
		{Name: string(dyncfg.CommandTest)},
		{Name: string(dyncfg.CommandUserconfig)},
		{Name: string(dyncfg.CommandRemove)},
	}
	return functionadapter.InitialRoute{
		Declaration: functionadapter.Declaration{
			ID: "dyncfg/service-discovery",
			Generation: &functionadapter.HandlerGenerationDeclaration{
				ID: fmt.Sprintf("dyncfg/service-discovery/%d", epoch),
				Handler: func(context.Context, functionadapter.HandlerInput) (lifecycle.SealedResult, error) {
					return mustDynCfgMessage(501, "Service discovery command is not implemented."), nil
				},
			},
			Transaction: &functionadapter.ResourceTransactionDeclaration{
				Prepare:         binding.prepare,
				CommandArgument: 1,
				GlobalClaim:     dynCfgServiceDiscoveryClaim,
				Commands:        commands,
			},
			PublicName:          joboutput.DynCfgFunctionName,
			Prefix:              binding.prefix(),
			Resource:            functionadapter.ScopedDynCfgJobResource(0, binding.prefix(), "sd:"),
			CooperativeCancel:   true,
			CooperativeDeadline: true,
			RawPayload:          true,
		},
	}, nil
}

var _ frameworkfunctions.Registry = (*serviceDiscoveryBinding)(nil)
var _ dyncfg.Output = (*serviceDiscoveryBinding)(nil)
