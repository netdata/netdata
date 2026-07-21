// SPDX-License-Identifier: GPL-3.0-or-later

package composition

import (
	"context"
	"errors"
	"fmt"
	"slices"
	"sync"

	functionadapter "github.com/netdata/netdata/go/plugins/plugin/agent/jobmgr/functions"
	"github.com/netdata/netdata/go/plugins/plugin/agent/jobmgr/joboutput"
	"github.com/netdata/netdata/go/plugins/plugin/agent/jobmgr/lifecycle"
	"github.com/netdata/netdata/go/plugins/plugin/framework/dyncfg"
	frameworkfunctions "github.com/netdata/netdata/go/plugins/plugin/framework/functions"
)

const dynCfgServiceDiscoveryClaim = "dyncfg:service-discovery"

type serviceDiscoveryBinding struct {
	mu sync.Mutex // guards handler/registered/dirty

	epoch      uint64                     // run generation
	pluginName string                     // owning plugin name
	capture    *functionProtocolCapture   // protocol capture wrapping the SD responder
	handler    frameworkfunctions.Handler // the registered service-discovery handler
	registered bool                       // the SD Function is registered
	dirty      error                      // sticky error (unexpected registration)
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
	capture, err := newFunctionProtocolCapture(frames)
	if err != nil {
		return nil, err
	}
	return &serviceDiscoveryBinding{
		epoch: epoch, pluginName: pluginName, capture: capture,
	}, nil
}

func (sdb *serviceDiscoveryBinding) prefix() string {
	return sdb.pluginName + ":sd:"
}

func (sdb *serviceDiscoveryBinding) Register(
	name string,
	fn func(frameworkfunctions.Function),
) {
	if fn == nil {
		sdb.recordRegistrationError(
			errors.New("nil exact service discovery Function"),
		)
		return
	}
	sdb.recordRegistrationError(
		fmt.Errorf(
			"unexpected exact service discovery Function registration %q",
			name,
		),
	)
}

func (sdb *serviceDiscoveryBinding) Unregister(name string) {
	sdb.recordRegistrationError(
		fmt.Errorf(
			"unexpected exact service discovery Function withdrawal %q",
			name,
		),
	)
}

func (sdb *serviceDiscoveryBinding) RegisterWithContext(
	name string,
	fn frameworkfunctions.Handler,
) {
	if fn == nil {
		sdb.recordRegistrationError(
			errors.New("nil exact service discovery Function"),
		)
		return
	}
	sdb.recordRegistrationError(
		fmt.Errorf(
			"unexpected exact service discovery Function registration %q",
			name,
		),
	)
}

func (sdb *serviceDiscoveryBinding) RegisterPrefix(
	name string,
	prefix string,
	fn func(frameworkfunctions.Function),
) {
	if fn == nil {
		sdb.recordRegistrationError(
			errors.New("nil service discovery prefix Function"),
		)
		return
	}
	sdb.registerPrefixWithContext(
		name,
		prefix,
		func(_ context.Context, input frameworkfunctions.Function) {
			fn(input)
		},
	)
}

func (sdb *serviceDiscoveryBinding) RegisterPrefixWithContext(
	name string,
	prefix string,
	fn frameworkfunctions.Handler,
) {
	sdb.registerPrefixWithContext(name, prefix, fn)
}

func (sdb *serviceDiscoveryBinding) registerPrefixWithContext(
	name string,
	prefix string,
	fn frameworkfunctions.Handler,
) {
	sdb.mu.Lock()
	defer sdb.mu.Unlock()
	if sdb.dirty != nil {
		return
	}
	if name != joboutput.DynCfgFunctionName ||
		prefix != sdb.prefix() ||
		fn == nil ||
		sdb.registered {
		sdb.dirty = errors.New(
			"jobmgr composition: invalid service discovery Function registration",
		)
		return
	}
	sdb.handler = fn
	sdb.registered = true
}

func (sdb *serviceDiscoveryBinding) UnregisterPrefix(
	name string,
	prefix string,
) {
	sdb.mu.Lock()
	defer sdb.mu.Unlock()
	if sdb.dirty != nil {
		return
	}
	if name != joboutput.DynCfgFunctionName ||
		prefix != sdb.prefix() ||
		!sdb.registered {
		sdb.dirty = errors.New(
			"jobmgr composition: invalid service discovery Function withdrawal",
		)
		return
	}
	sdb.handler = nil
	sdb.registered = false
}

func (sdb *serviceDiscoveryBinding) TerminalFinalizer() frameworkfunctions.TerminalFinalizer {
	return frameworkfunctions.DirectTerminalFinalizer
}

func (sdb *serviceDiscoveryBinding) recordRegistrationError(err error) {
	sdb.mu.Lock()
	defer sdb.mu.Unlock()
	if sdb.dirty == nil {
		sdb.dirty = err
	}
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
		return nil, errors.New(
			"jobmgr composition: invalid service discovery transaction scope",
		)
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
	result, cleanup, err := sdb.capture.invoke(
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
		nil,
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
					return mustDynCfgMessage(501, "Service discovery command is not implemented."), nil
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
