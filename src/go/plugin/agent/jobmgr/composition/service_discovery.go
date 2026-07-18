// SPDX-License-Identifier: GPL-3.0-or-later

package composition

import (
	"context"
	"errors"
	"fmt"
	"sync"

	functionadapter "github.com/netdata/netdata/go/plugins/plugin/agent/jobmgr/functions"
	"github.com/netdata/netdata/go/plugins/plugin/agent/jobmgr/joboutput"
	"github.com/netdata/netdata/go/plugins/plugin/agent/jobmgr/lifecycle"
	"github.com/netdata/netdata/go/plugins/plugin/framework/dyncfg"
	frameworkfunctions "github.com/netdata/netdata/go/plugins/plugin/framework/functions"
)

const dynCfgServiceDiscoveryClaim = "dyncfg:service-discovery"

type serviceDiscoveryBinding struct {
	mu sync.Mutex

	epoch      uint64
	pluginName string
	capture    *legacyProtocolCapture
	handler    frameworkfunctions.Handler
	registered bool
	dirty      error
}

func newServiceDiscoveryBinding(
	epoch uint64,
	pluginName string,
	frames *lifecycle.FrameOwner,
) (*serviceDiscoveryBinding, error) {
	if epoch == 0 || pluginName == "" || frames == nil {
		return nil, errors.New(
			"jobmgr composition: invalid service discovery binding",
		)
	}
	capture, err := newLegacyProtocolCaptureWithDirectFrames(frames)
	if err != nil {
		return nil, err
	}
	return &serviceDiscoveryBinding{
		epoch: epoch, pluginName: pluginName, capture: capture,
	}, nil
}

func (binding *serviceDiscoveryBinding) prefix() string {
	return binding.pluginName + ":sd:"
}

func (binding *serviceDiscoveryBinding) Register(
	name string,
	fn func(frameworkfunctions.Function),
) {
	if fn == nil {
		binding.recordRegistrationError(
			errors.New("nil exact service discovery Function"),
		)
		return
	}
	binding.recordRegistrationError(
		fmt.Errorf(
			"unexpected exact service discovery Function registration %q",
			name,
		),
	)
}

func (binding *serviceDiscoveryBinding) Unregister(name string) {
	binding.recordRegistrationError(
		fmt.Errorf(
			"unexpected exact service discovery Function withdrawal %q",
			name,
		),
	)
}

func (binding *serviceDiscoveryBinding) RegisterWithContext(
	name string,
	fn frameworkfunctions.Handler,
) {
	if fn == nil {
		binding.recordRegistrationError(
			errors.New("nil exact service discovery Function"),
		)
		return
	}
	binding.recordRegistrationError(
		fmt.Errorf(
			"unexpected exact service discovery Function registration %q",
			name,
		),
	)
}

func (binding *serviceDiscoveryBinding) RegisterPrefix(
	name string,
	prefix string,
	fn func(frameworkfunctions.Function),
) {
	if fn == nil {
		binding.recordRegistrationError(
			errors.New("nil service discovery prefix Function"),
		)
		return
	}
	binding.registerPrefixWithContext(
		name,
		prefix,
		func(_ context.Context, input frameworkfunctions.Function) {
			fn(input)
		},
	)
}

func (binding *serviceDiscoveryBinding) RegisterPrefixWithContext(
	name string,
	prefix string,
	fn frameworkfunctions.Handler,
) {
	binding.registerPrefixWithContext(name, prefix, fn)
}

func (binding *serviceDiscoveryBinding) registerPrefixWithContext(
	name string,
	prefix string,
	fn frameworkfunctions.Handler,
) {
	binding.mu.Lock()
	defer binding.mu.Unlock()
	if binding.dirty != nil {
		return
	}
	if name != joboutput.DynCfgFunctionName ||
		prefix != binding.prefix() ||
		fn == nil ||
		binding.registered {
		binding.dirty = errors.New(
			"jobmgr composition: invalid service discovery Function registration",
		)
		return
	}
	binding.handler = fn
	binding.registered = true
}

func (binding *serviceDiscoveryBinding) UnregisterPrefix(
	name string,
	prefix string,
) {
	binding.mu.Lock()
	defer binding.mu.Unlock()
	if binding.dirty != nil {
		return
	}
	if name != joboutput.DynCfgFunctionName ||
		prefix != binding.prefix() ||
		!binding.registered {
		binding.dirty = errors.New(
			"jobmgr composition: invalid service discovery Function withdrawal",
		)
		return
	}
	binding.handler = nil
	binding.registered = false
}

func (binding *serviceDiscoveryBinding) TerminalFinalizer() frameworkfunctions.TerminalFinalizer {
	return frameworkfunctions.DirectTerminalFinalizer
}

func (binding *serviceDiscoveryBinding) recordRegistrationError(err error) {
	binding.mu.Lock()
	defer binding.mu.Unlock()
	if binding.dirty == nil {
		binding.dirty = err
	}
}

func (binding *serviceDiscoveryBinding) prepare(
	ctx context.Context,
	input functionadapter.HandlerInput,
	current lifecycle.ReadyResource,
	scope lifecycle.ResourceTransactionScope,
	permit lifecycle.LongLivedPermit,
) (lifecycle.PreparedResourceTransaction, error) {
	if binding == nil || ctx == nil || current != nil ||
		scope.Current.Valid() ||
		scope.Successor.Valid() ||
		permit.Valid() ||
		!scope.Valid() {
		return nil, errors.New(
			"jobmgr composition: invalid service discovery transaction scope",
		)
	}
	binding.mu.Lock()
	handler, dirty := binding.handler, binding.dirty
	binding.mu.Unlock()
	if dirty != nil {
		return nil, dirty
	}
	if handler == nil {
		return joboutput.PrepareNoopResourceTransaction(
			scope,
			nil,
			lifecycle.LongLivedPermit{},
			mustVNodeMessage(
				503,
				"Service discovery configuration is not available.",
			),
			func() error { return nil },
		)
	}
	function := frameworkfunctions.Function{
		UID: input.UID, Timeout: input.Timeout,
		Name: input.Method, Args: append([]string(nil), input.Args...),
		Payload:     append([]byte(nil), input.Payload...),
		Permissions: input.Permissions, Source: input.CallerSource,
		ContentType: input.ContentType,
	}
	result, cleanup, err := binding.capture.invoke(
		input.UID,
		func() { handler(ctx, function) },
	)
	if err != nil {
		return nil, err
	}
	return joboutput.PrepareNoopResourceTransaction(
		scope,
		nil,
		lifecycle.LongLivedPermit{},
		result,
		cleanup,
	)
}

func newServiceDiscoveryInitialRoute(
	epoch uint64,
	binding *serviceDiscoveryBinding,
) (functionadapter.InitialRoute, error) {
	if epoch == 0 || binding == nil {
		return functionadapter.InitialRoute{},
			errors.New(
				"jobmgr composition: invalid service discovery route",
			)
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
				ID: fmt.Sprintf(
					"dyncfg/service-discovery/%d",
					epoch,
				),
				Handler: func(
					context.Context,
					functionadapter.HandlerInput,
				) (lifecycle.SealedResult, error) {
					return mustVNodeMessage(
						501,
						"Service discovery command is not implemented.",
					), nil
				},
			},
			Transaction: &functionadapter.ResourceTransactionDeclaration{
				Prepare:         binding.prepare,
				CommandArgument: 1,
				GlobalClaim:     dynCfgServiceDiscoveryClaim,
				Commands:        commands,
			},
			PublicName: joboutput.DynCfgFunctionName,
			Prefix:     binding.prefix(),
			Resource: functionadapter.ScopedDynCfgJobResource(
				0,
				binding.prefix(),
				"sd:",
			),
			CooperativeCancel:   true,
			CooperativeDeadline: true,
			RawPayload:          true,
		},
		Publication: dynCfgPublication(epoch),
	}, nil
}

var (
	_ frameworkfunctions.Registry        = (*serviceDiscoveryBinding)(nil)
	_ frameworkfunctions.ContextRegistry = (*serviceDiscoveryBinding)(nil)
)
