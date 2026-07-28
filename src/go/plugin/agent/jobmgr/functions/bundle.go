// SPDX-License-Identifier: GPL-3.0-or-later

package functions

import (
	"context"
	"errors"
	"fmt"
	"sync"

	"github.com/netdata/netdata/go/plugins/pkg/funcapi"
	"github.com/netdata/netdata/go/plugins/plugin/agent/jobmgr"
	"github.com/netdata/netdata/go/plugins/plugin/agent/jobmgr/lifecycle"
	"github.com/netdata/netdata/go/plugins/plugin/framework/collectorapi"
)

type functionBundleKind uint8

const (
	functionBundleAgent functionBundleKind = iota + 1
	functionBundleJob
)

// functionBundle owns one collector-created handler. Catalog generations hold
// cheap references; only the bundle's process owner runs physical cleanup.
type functionBundle struct {
	mu sync.Mutex

	kind         functionBundleKind
	module       string
	job          collectorapi.RuntimeJob
	handler      funcapi.MethodHandler
	methods      []funcapi.FunctionConfig
	availability map[string]bool
	pollable     bool
	references   int
	retired      bool
	cleanupStart bool
	cleanupErr   error
	cleanupDone  chan struct{}
	attempts     jobmgr.ProcessAttemptAuthority
	identityKey  string
	resource     string
	target       uint64
	invocationID uint64
	activeCalls  int
	quarantined  bool
	idExhausted  bool
}

type functionAvailabilityPoll struct {
	attempt jobmgr.ProcessAttempt
	result  <-chan functionAvailabilityResult
	bundle  *functionBundle
}

type functionAvailabilityResult struct {
	availability map[string]bool
	err          error
}

type functionInvocation struct {
	bundle    *functionBundle
	completed bool
}

type functionInvocationResult struct {
	result lifecycle.SealedResult
	err    error
}

func (bundle *functionBundle) bindContainment(
	attempts jobmgr.ProcessAttemptAuthority,
	target uint64,
	key string,
	resource string,
) error {
	if bundle == nil ||
		attempts == nil ||
		target == 0 ||
		key == "" ||
		resource == "" {
		return errors.New("jobmgr Function bundle: invalid containment binding")
	}
	bundle.mu.Lock()
	defer bundle.mu.Unlock()
	if bundle.attempts != nil || bundle.identityKey != "" || bundle.target != 0 {
		return errors.New("jobmgr Function bundle: duplicate containment binding")
	}
	bundle.attempts = attempts
	bundle.identityKey = key
	bundle.resource = resource
	bundle.target = target
	return nil
}

func (bundle *functionBundle) startAvailabilityPoll() (functionAvailabilityPoll, error) {
	if bundle == nil {
		return functionAvailabilityPoll{}, errors.New("jobmgr Function bundle: nil availability poll")
	}
	bundle.mu.Lock()
	if bundle.retired || !bundle.pollable {
		bundle.mu.Unlock()
		return functionAvailabilityPoll{}, nil
	}
	bundle.references++
	attempts := bundle.attempts
	key := bundle.identityKey
	resource := bundle.resource
	target := bundle.target
	bundle.mu.Unlock()
	if attempts == nil {
		bundle.release()
		return functionAvailabilityPoll{}, errors.New("jobmgr Function bundle: availability containment is not bound")
	}
	workerResult := make(chan functionAvailabilityResult, 1)
	attempt, err := attempts.StartProcessAttempt(jobmgr.ProcessAttemptPlan{
		Identity: jobmgr.ProcessAttemptIdentity{
			Namespace: jobmgr.ProcessAttemptFunctionPoll,
			Key:       key,
			Resource:  resource,
		},
		Target: target,
		Work: func(ctx context.Context, _ jobmgr.ProcessAttemptAdmission) error {
			availability, pollErr := bundle.evaluateAvailability()
			if cause := context.Cause(ctx); cause != nil {
				return cause
			}
			workerResult <- functionAvailabilityResult{
				availability: availability,
				err:          pollErr,
			}
			return nil
		},
	})
	if err != nil {
		bundle.release()
		return functionAvailabilityPoll{}, err
	}
	result := make(chan functionAvailabilityResult, 1)
	go func() {
		settledErr := attempt.Await(context.Background())
		<-attempt.Released()
		if settledErr != nil {
			result <- functionAvailabilityResult{err: settledErr}
			return
		}
		result <- <-workerResult
	}()
	return functionAvailabilityPoll{
		attempt: attempt,
		result:  result,
		bundle:  bundle,
	}, nil
}

func (bundle *functionBundle) invoke(
	ctx context.Context,
	call func(context.Context) (lifecycle.SealedResult, error),
) (lifecycle.SealedResult, error) {
	if bundle == nil || ctx == nil || call == nil {
		return functionErrorResult(503, "Function handler is unavailable")
	}
	bundle.mu.Lock()
	if bundle.retired || bundle.quarantined {
		bundle.mu.Unlock()
		return functionErrorResult(503, "Function handler is unavailable")
	}
	bundle.invocationID++
	if bundle.invocationID == 0 {
		bundle.quarantined = true
		bundle.idExhausted = true
		bundle.mu.Unlock()
		return functionErrorResult(503, "Function handler is unavailable")
	}
	invocationID := bundle.invocationID
	bundle.activeCalls++
	bundle.references++
	attempts := bundle.attempts
	key := bundle.identityKey
	resource := bundle.resource
	target := bundle.target
	invocation := &functionInvocation{bundle: bundle}
	bundle.mu.Unlock()

	result := make(chan functionInvocationResult, 1)
	attempt, err := attempts.StartProcessAttempt(jobmgr.ProcessAttemptPlan{
		Identity: jobmgr.ProcessAttemptIdentity{
			Namespace: jobmgr.ProcessAttemptFunctionInvocation,
			Key:       fmt.Sprintf("%s/invocation/%d", key, invocationID),
			Resource:  resource,
		},
		Target: target,
		Work: func(
			attemptCtx context.Context,
			_ jobmgr.ProcessAttemptAdmission,
		) error {
			defer invocation.complete()
			callCtx, cancel := context.WithCancelCause(attemptCtx)
			stop := context.AfterFunc(ctx, func() {
				cancel(context.Cause(ctx))
			})
			sealed, callErr := call(callCtx)
			stop()
			cancel(nil)
			result <- functionInvocationResult{
				result: sealed,
				err:    callErr,
			}
			return nil
		},
	})
	if err != nil {
		invocation.complete()
		return functionErrorResult(503, "Function handler is unavailable")
	}
	if err := attempt.Await(ctx); err != nil {
		select {
		case <-attempt.Released():
		default:
			invocation.quarantineIfActive()
		}
		if ctx.Err() != nil || errors.Is(err, jobmgr.ErrProcessAttemptDeadline) {
			return functionErrorResult(503, "Function handler did not complete before its deadline")
		}
		return functionErrorResult(503, "Function handler is unavailable")
	}
	response := <-result
	return response.result, response.err
}

func (invocation *functionInvocation) quarantineIfActive() {
	if invocation == nil || invocation.bundle == nil {
		return
	}
	bundle := invocation.bundle
	bundle.mu.Lock()
	if !invocation.completed {
		bundle.quarantined = true
	}
	bundle.mu.Unlock()
}

func (invocation *functionInvocation) complete() {
	if invocation == nil || invocation.bundle == nil {
		return
	}
	bundle := invocation.bundle
	bundle.mu.Lock()
	if invocation.completed {
		bundle.mu.Unlock()
		return
	}
	invocation.completed = true
	if bundle.activeCalls <= 0 {
		bundle.mu.Unlock()
		panic("jobmgr Function bundle: active invocation underflow")
	}
	bundle.activeCalls--
	// A sibling can cross its logical deadline before its caller records
	// retained ownership. Once established, quarantine therefore stays closed
	// until every callback admitted before it has physically returned.
	if bundle.quarantined &&
		bundle.activeCalls == 0 &&
		!bundle.retired &&
		!bundle.idExhausted {
		bundle.quarantined = false
	}
	if bundle.references <= 0 {
		bundle.mu.Unlock()
		panic("jobmgr Function bundle: invocation reference underflow")
	}
	bundle.references--
	start := bundle.startCleanupLocked()
	bundle.mu.Unlock()
	if start {
		go bundle.cleanup()
	}
}

func newAgentFunctionBundle(
	module string,
	creator collectorapi.Creator,
	methods []funcapi.FunctionConfig,
) (*functionBundle, error) {
	return newFunctionBundle(functionBundleAgent, module, creator, nil, methods)
}

func newJobFunctionBundle(
	module string,
	creator collectorapi.Creator,
	job collectorapi.RuntimeJob,
	methods []funcapi.FunctionConfig,
) (*functionBundle, error) {
	return newFunctionBundle(functionBundleJob, module, creator, job, methods)
}

func newFunctionBundle(
	kind functionBundleKind,
	module string,
	creator collectorapi.Creator,
	job collectorapi.RuntimeJob,
	methods []funcapi.FunctionConfig,
) (bundle *functionBundle, resultErr error) {
	if module == "" ||
		(kind != functionBundleAgent && kind != functionBundleJob) ||
		kind == functionBundleAgent && job != nil ||
		kind == functionBundleJob && (job == nil || job.ModuleName() != module) {
		return nil, errors.New("jobmgr Function bundle: invalid construction")
	}
	if len(methods) != 0 && creator.MethodHandler == nil {
		return nil, errors.New("jobmgr Function bundle: collector has no method handler")
	}
	var handler funcapi.MethodHandler
	if len(methods) != 0 {
		var err error
		handler, err = callMethodHandler(creator, job)
		if err != nil {
			return nil, err
		}
		if handler == nil {
			return nil, errors.New("jobmgr Function bundle: collector returned a nil method handler")
		}
	}
	bundle = &functionBundle{
		kind:         kind,
		module:       module,
		job:          job,
		handler:      handler,
		methods:      append([]funcapi.FunctionConfig(nil), methods...),
		availability: make(map[string]bool, len(methods)),
		references:   1,
		cleanupDone:  make(chan struct{}),
	}
	if kind == functionBundleAgent {
		for _, method := range methods {
			bundle.pollable = bundle.pollable || method.Available != nil
		}
	} else if callback, ok := job.Collector().(collectorapi.FunctionAvailability); ok && callback != nil {
		bundle.pollable = true
	}
	transferred := false
	defer func() {
		if transferred {
			return
		}
		bundle.retire()
		resultErr = errors.Join(resultErr, bundle.wait(context.Background()))
		bundle = nil
	}()
	if _, err := bundle.refreshAvailability(); err != nil {
		return nil, err
	}
	transferred = true
	return bundle, nil
}

func callMethodHandler(
	creator collectorapi.Creator,
	job collectorapi.RuntimeJob,
) (handler funcapi.MethodHandler, err error) {
	defer func() {
		if recovered := recover(); recovered != nil {
			handler = nil
			err = lifecycle.RetainOwnership(fmt.Errorf(
				"%w in Function MethodHandler: %v",
				lifecycle.ErrTaskPanic,
				recovered,
			))
		}
	}()
	return creator.MethodHandler(job), nil
}

func callInstanceFunctions(
	creator collectorapi.Creator,
	job collectorapi.RuntimeJob,
) (methods []funcapi.FunctionConfig, err error) {
	if creator.InstanceFunctions == nil {
		return nil, nil
	}
	defer func() {
		if recovered := recover(); recovered != nil {
			methods = nil
			err = lifecycle.RetainOwnership(fmt.Errorf(
				"%w in Function InstanceFunctions: %v",
				lifecycle.ErrTaskPanic,
				recovered,
			))
		}
	}()
	return creator.InstanceFunctions(job), nil
}

func callModuleFunctions(
	name string,
	callback func() []funcapi.FunctionConfig,
) (methods []funcapi.FunctionConfig, err error) {
	if callback == nil {
		return nil, nil
	}
	defer func() {
		if recovered := recover(); recovered != nil {
			methods = nil
			err = lifecycle.RetainOwnership(fmt.Errorf(
				"%w in Function %s: %v",
				lifecycle.ErrTaskPanic,
				name,
				recovered,
			))
		}
	}()
	return callback(), nil
}

func callFunctionAvailability(callback func() bool) (available bool, err error) {
	defer func() {
		if recovered := recover(); recovered != nil {
			err = fmt.Errorf("%w in Function availability: %v", lifecycle.ErrTaskPanic, recovered)
		}
	}()
	return callback(), nil
}

func (bundle *functionBundle) acquire() error {
	if bundle == nil {
		return errors.New("jobmgr Function bundle: nil acquire")
	}
	bundle.mu.Lock()
	defer bundle.mu.Unlock()
	if bundle.retired || bundle.references <= 0 {
		return errors.New("jobmgr Function bundle: acquire after retirement")
	}
	bundle.references++
	return nil
}

func (bundle *functionBundle) release() {
	if bundle == nil {
		return
	}
	bundle.mu.Lock()
	if bundle.references <= 0 {
		bundle.mu.Unlock()
		panic("jobmgr Function bundle: reference underflow")
	}
	bundle.references--
	start := bundle.startCleanupLocked()
	bundle.mu.Unlock()
	if start {
		go bundle.cleanup()
	}
}

func (bundle *functionBundle) retire() {
	if bundle == nil {
		return
	}
	bundle.mu.Lock()
	if bundle.retired {
		bundle.mu.Unlock()
		return
	}
	bundle.retired = true
	bundle.references--
	start := bundle.startCleanupLocked()
	bundle.mu.Unlock()
	if start {
		go bundle.cleanup()
	}
}

func (bundle *functionBundle) startCleanupLocked() bool {
	if !bundle.retired || bundle.references != 0 || bundle.cleanupStart {
		return false
	}
	bundle.cleanupStart = true
	return true
}

func (bundle *functionBundle) cleanup() {
	if bundle.handler != nil {
		bundle.cleanupErr = callMethodCleanup(context.Background(), bundle.handler)
	}
	close(bundle.cleanupDone)
}

func (bundle *functionBundle) wait(ctx context.Context) error {
	if bundle == nil {
		return nil
	}
	select {
	case <-bundle.cleanupDone:
		bundle.mu.Lock()
		defer bundle.mu.Unlock()
		return bundle.cleanupErr
	case <-ctx.Done():
		return ctx.Err()
	}
}

func (bundle *functionBundle) refreshAvailability() (bool, error) {
	next, err := bundle.evaluateAvailability()
	if err != nil {
		return false, err
	}
	return bundle.commitAvailability(next), nil
}

func (bundle *functionBundle) evaluateAvailability() (map[string]bool, error) {
	if bundle == nil {
		return nil, errors.New("jobmgr Function bundle: nil availability refresh")
	}
	next := make(map[string]bool, len(bundle.methods))
	for _, method := range bundle.methods {
		available := true
		var err error
		switch bundle.kind {
		case functionBundleAgent:
			if method.Available != nil {
				available, err = callFunctionAvailability(method.Available)
			}
		case functionBundleJob:
			if callback, ok := bundle.job.Collector().(collectorapi.FunctionAvailability); ok && callback != nil {
				available, err = callFunctionAvailability(func() bool {
					return callback.FunctionAvailable(method.ID)
				})
			}
		default:
			err = errors.New("jobmgr Function bundle: invalid availability kind")
		}
		if err != nil {
			return nil, err
		}
		next[method.ID] = available
	}
	return next, nil
}

func (bundle *functionBundle) commitAvailability(next map[string]bool) bool {
	if bundle == nil {
		return false
	}
	bundle.mu.Lock()
	defer bundle.mu.Unlock()
	if bundle.retired {
		return false
	}
	changed := len(bundle.availability) != len(next)
	if !changed {
		for method, available := range next {
			if bundle.availability[method] != available {
				changed = true
				break
			}
		}
	}
	bundle.availability = next
	return changed
}

func (bundle *functionBundle) available(method string) bool {
	if bundle == nil {
		return false
	}
	bundle.mu.Lock()
	defer bundle.mu.Unlock()
	return !bundle.retired && !bundle.quarantined && bundle.availability[method]
}
