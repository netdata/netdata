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
	functionadapter "github.com/netdata/netdata/go/plugins/plugin/agent/jobmgr/functions"
	"github.com/netdata/netdata/go/plugins/plugin/agent/jobmgr/joboutput"
	"github.com/netdata/netdata/go/plugins/plugin/agent/jobmgr/lifecycle"
	"github.com/netdata/netdata/go/plugins/plugin/framework/dyncfg"
	frameworkfunctions "github.com/netdata/netdata/go/plugins/plugin/framework/functions"
)

const dynCfgServiceDiscoveryClaim = "dyncfg:service-discovery"

type serviceDiscoveryBinding struct {
	mu sync.Mutex // guards handler/registered/active/dirty

	pluginName string                      // owning plugin name
	frames     *lifecycle.FrameOwner       // the one wire frame writer
	handler    frameworkfunctions.Handler  // the registered service-discovery handler
	active     *serviceDiscoveryInvocation // current synchronous invocation
	registered bool                        // the SD Function is registered
	dirty      error                       // sticky error (unexpected registration)
}

type serviceDiscoveryInvocation struct {
	uid           string
	result        *dyncfg.Result
	notifications []byte
	err           error
}

func newServiceDiscoveryBinding(pluginName string, frames *lifecycle.FrameOwner) (*serviceDiscoveryBinding, error) {
	if pluginName == "" || frames == nil {
		return nil, errors.New("jobmgr composition: invalid service discovery binding")
	}
	return &serviceDiscoveryBinding{pluginName: pluginName, frames: frames}, nil
}

func (sdb *serviceDiscoveryBinding) prefix() string {
	return sdb.pluginName + ":sd:"
}

func (sdb *serviceDiscoveryBinding) RegisterPrefix(
	name string,
	prefix string,
	fn func(frameworkfunctions.Function),
) {
	if fn == nil {
		sdb.recordRegistrationError(errors.New("nil service discovery prefix Function"))
		return
	}
	sdb.registerPrefix(
		name,
		prefix,
		func(_ context.Context, input frameworkfunctions.Function) {
			fn(input)
		},
	)
}

func (sdb *serviceDiscoveryBinding) registerPrefix(name string, prefix string, fn frameworkfunctions.Handler) {
	sdb.mu.Lock()
	defer sdb.mu.Unlock()
	if sdb.dirty != nil {
		return
	}
	if name != joboutput.DynCfgFunctionName || prefix != sdb.prefix() || fn == nil || sdb.registered {
		sdb.setDirtyLocked(errors.New("jobmgr composition: invalid service discovery Function registration"))
		return
	}
	sdb.handler = fn
	sdb.registered = true
}

func (sdb *serviceDiscoveryBinding) UnregisterPrefix(name string, prefix string) {
	sdb.mu.Lock()
	defer sdb.mu.Unlock()
	if sdb.dirty != nil {
		return
	}
	if name != joboutput.DynCfgFunctionName || prefix != sdb.prefix() || !sdb.registered {
		sdb.setDirtyLocked(errors.New("jobmgr composition: invalid service discovery Function withdrawal"))
		return
	}
	sdb.handler = nil
	sdb.registered = false
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
		UID: input.UID, Timeout: input.Timeout,
		Name: input.Method, Args: slices.Clone(input.Args),
		Payload:     slices.Clone(input.Payload),
		Permissions: input.Permissions, Source: input.CallerSource,
		ContentType: input.ContentType,
	}
	result, cleanup, err := sdb.invoke(
		input.UID,
		func() { handler(ctx, function) },
	)
	if err != nil {
		return nil, err
	}
	return joboutput.PrepareNoopResourceTransaction(scope, nil, lifecycle.LongLivedPermit{}, result, cleanup, nil)
}

func (sdb *serviceDiscoveryBinding) invoke(
	uid string,
	call func(),
) (lifecycle.SealedResult, lifecycle.TaskCleanup, error) {
	if sdb == nil || lifecycle.ValidateUID(uid) != nil || call == nil {
		return lifecycle.SealedResult{}, nil, errors.New("jobmgr composition: invalid service discovery invocation")
	}

	invocation := &serviceDiscoveryInvocation{uid: uid}
	sdb.mu.Lock()
	if sdb.dirty != nil || sdb.active != nil {
		err := errors.Join(sdb.dirty, errors.New("jobmgr composition: service discovery invocation unavailable"))
		sdb.mu.Unlock()
		return lifecycle.SealedResult{}, nil, err
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
		return lifecycle.SealedResult{}, nil,
			errors.New("jobmgr composition: service discovery handler produced no terminal result")
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
	defer sdb.mu.Unlock()
	if sdb.active == nil {
		if err := sdb.frames.CommitBorrowedProtocolFrame(payload); err != nil {
			sdb.setDirtyLocked(err)
		}
		return
	}
	if len(payload) > lifecycle.MaximumOtherFrameBytes-len(sdb.active.notifications) {
		sdb.active.err = errors.Join(
			sdb.active.err,
			errors.New("jobmgr composition: service discovery notifications exceed frame bounds"),
		)
		return
	}
	sdb.active.notifications = append(sdb.active.notifications, payload...)
}

func (sdb *serviceDiscoveryBinding) setDirtyLocked(err error) {
	if sdb.dirty == nil {
		sdb.dirty = err
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
		Publication: dynCfgPublication(epoch),
	}, nil
}

var _ frameworkfunctions.Registry = (*serviceDiscoveryBinding)(nil)
var _ dyncfg.Output = (*serviceDiscoveryBinding)(nil)
