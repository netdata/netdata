// SPDX-License-Identifier: GPL-3.0-or-later

package functions

import (
	"context"
	"errors"
	"fmt"
	"reflect"
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

	kind            functionBundleKind
	module          string
	job             collectorapi.RuntimeJob
	handler         funcapi.MethodHandler
	methods         []funcapi.FunctionConfig
	availability    map[string]bool
	pollable        bool
	references      int
	retired         bool
	cleanupStart    bool
	cleanupErr      error
	cleanupDone     chan struct{}
	attempts        jobmgr.ProcessAttemptAuthority
	identityKey     string
	resource        string
	target          uint64
	invocationID    uint64
	activeCallbacks int
	quarantined     bool
	quarantineFixed bool
}

type functionAvailabilityPoll struct {
	attempt      jobmgr.ProcessAttempt
	workerResult <-chan functionAvailabilityResult
	bundle       *functionBundle
}

type functionAvailabilityResult struct {
	availability map[string]bool
	err          error
}

type functionCallback struct {
	bundle    *functionBundle
	completed bool
}

type functionInvocationResult struct {
	result lifecycle.SealedResult
	err    error
}

func (fb *functionBundle) bindContainment(
	attempts jobmgr.ProcessAttemptAuthority,
	target uint64,
	key string,
	resource string,
) error {
	if fb == nil ||
		attempts == nil ||
		target == 0 ||
		key == "" ||
		resource == "" {
		return errors.New("jobmgr Function bundle: invalid containment binding")
	}
	fb.mu.Lock()
	defer fb.mu.Unlock()
	if fb.attempts != nil || fb.identityKey != "" || fb.target != 0 {
		return errors.New("jobmgr Function bundle: duplicate containment binding")
	}
	fb.attempts = attempts
	fb.identityKey = key
	fb.resource = resource
	fb.target = target
	return nil
}

func (fb *functionBundle) startAvailabilityPoll() (functionAvailabilityPoll, error) {
	if fb == nil {
		return functionAvailabilityPoll{}, errors.New("jobmgr Function bundle: nil availability poll")
	}
	fb.mu.Lock()
	if fb.retired || !fb.pollable {
		fb.mu.Unlock()
		return functionAvailabilityPoll{}, nil
	}
	if fb.quarantined {
		fb.mu.Unlock()
		return functionAvailabilityPoll{}, jobmgr.ErrProcessAttemptBusy
	}
	callback := &functionCallback{bundle: fb}
	fb.activeCallbacks++
	fb.references++
	attempts := fb.attempts
	key := fb.identityKey
	resource := fb.resource
	target := fb.target
	fb.mu.Unlock()
	if attempts == nil {
		callback.complete()
		return functionAvailabilityPoll{}, errors.New("jobmgr Function bundle: availability containment is not bound")
	}
	workerResult := make(chan functionAvailabilityResult, 1)
	attempt, err := attempts.StartProcessAttempt(context.Background(), jobmgr.ProcessAttemptPlan{
		Identity: jobmgr.ProcessAttemptIdentity{
			Namespace: jobmgr.ProcessAttemptFunctionPoll,
			Key:       key,
			Resource:  resource,
		},
		Target:        target,
		OnContainment: callback.quarantineIfActive,
		Work: func(ctx context.Context, _ jobmgr.ProcessAttemptAdmission) error {
			defer callback.complete()
			availability, pollErr := fb.evaluateAvailability()
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
		callback.complete()
		return functionAvailabilityPoll{}, err
	}
	return functionAvailabilityPoll{
		attempt:      attempt,
		workerResult: workerResult,
		bundle:       fb,
	}, nil
}

func (fb *functionBundle) invoke(
	ctx context.Context,
	call func(context.Context) (lifecycle.SealedResult, error),
) (lifecycle.SealedResult, error) {
	if fb == nil || ctx == nil || call == nil {
		return functionErrorResult(503, "Function handler is unavailable")
	}
	if context.Cause(ctx) != nil {
		return functionErrorResult(503, "Function handler did not complete before its deadline")
	}
	fb.mu.Lock()
	if fb.retired || fb.quarantined {
		fb.mu.Unlock()
		return functionErrorResult(503, "Function handler is unavailable")
	}
	fb.invocationID++
	if fb.invocationID == 0 {
		fb.quarantined = true
		fb.quarantineFixed = true
		fb.mu.Unlock()
		return functionErrorResult(503, "Function handler is unavailable")
	}
	invocationID := fb.invocationID
	fb.activeCallbacks++
	fb.references++
	attempts := fb.attempts
	key := fb.identityKey
	resource := fb.resource
	target := fb.target
	callback := &functionCallback{bundle: fb}
	fb.mu.Unlock()

	result := make(chan functionInvocationResult, 1)
	attempt, err := attempts.StartProcessAttempt(ctx, jobmgr.ProcessAttemptPlan{
		Identity: jobmgr.ProcessAttemptIdentity{
			Namespace: jobmgr.ProcessAttemptFunctionInvocation,
			Key:       fmt.Sprintf("%s/invocation/%d", key, invocationID),
			Resource:  resource,
		},
		Target:        target,
		OnContainment: callback.quarantineIfActive,
		Work: func(attemptCtx context.Context, _ jobmgr.ProcessAttemptAdmission) error {
			defer callback.complete()
			callCtx, cancel := context.WithCancelCause(attemptCtx)
			stop := context.AfterFunc(ctx, func() {
				cancel(context.Cause(ctx))
			})
			sealed, callErr, panicked := callFunctionInvocation(callCtx, call)
			if panicked {
				callback.quarantinePermanently()
				sealed, callErr = functionErrorResult(503, "Function handler is unavailable")
			}
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
		callback.complete()
		return functionErrorResult(503, "Function handler is unavailable")
	}
	if err := attempt.Await(ctx); err != nil {
		if ctx.Err() != nil || errors.Is(err, jobmgr.ErrProcessAttemptDeadline) {
			return functionErrorResult(503, "Function handler did not complete before its deadline")
		}
		return functionErrorResult(503, "Function handler is unavailable")
	}
	response := <-result
	return response.result, response.err
}

func (fc *functionCallback) quarantineIfActive(error) {
	if fc == nil || fc.bundle == nil {
		return
	}
	// Containment invokes this fence under the attempt-authority lock.
	// Bundle admissions must therefore release bundle.mu before entering it.
	bundle := fc.bundle
	bundle.mu.Lock()
	if !fc.completed {
		bundle.quarantined = true
	}
	bundle.mu.Unlock()
}

func (fc *functionCallback) quarantinePermanently() {
	if fc == nil || fc.bundle == nil {
		return
	}
	bundle := fc.bundle
	bundle.mu.Lock()
	if !fc.completed {
		bundle.quarantined = true
		// A panicked handler may have corrupted its own state. Do not re-enter
		// it merely because the callback unwound and released physical ownership.
		bundle.quarantineFixed = true
	}
	bundle.mu.Unlock()
}

func (fc *functionCallback) complete() {
	if fc == nil || fc.bundle == nil {
		return
	}
	bundle := fc.bundle
	bundle.mu.Lock()
	if fc.completed {
		bundle.mu.Unlock()
		return
	}
	fc.completed = true
	if bundle.activeCallbacks <= 0 {
		bundle.mu.Unlock()
		panic("jobmgr Function bundle: active callback underflow")
	}
	bundle.activeCallbacks--
	// A sibling can cross its logical deadline before its caller records
	// retained ownership. Once established, quarantine therefore stays closed
	// until every callback admitted before it has physically returned.
	if bundle.quarantined &&
		bundle.activeCallbacks == 0 &&
		!bundle.retired &&
		!bundle.quarantineFixed {
		bundle.quarantined = false
	}
	if bundle.references <= 0 {
		bundle.mu.Unlock()
		panic("jobmgr Function bundle: callback reference underflow")
	}
	bundle.references--
	start := bundle.startCleanupLocked()
	bundle.mu.Unlock()
	if start {
		go bundle.cleanup()
	}
}

func callFunctionInvocation(
	ctx context.Context,
	call func(context.Context) (lifecycle.SealedResult, error),
) (result lifecycle.SealedResult, err error, panicked bool) {
	defer func() {
		if recover() != nil {
			result = lifecycle.SealedResult{}
			err = nil
			panicked = true
		}
	}()
	result, err = call(ctx)
	return result, err, false
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
) (*functionBundle, error) {
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
		if nilMethodHandler(handler) {
			return nil, errors.New("jobmgr Function bundle: collector returned a nil method handler")
		}
	}
	bundle := &functionBundle{
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
	if _, err := bundle.refreshAvailability(); err != nil {
		bundle.retire()
		return nil, joinRetainedBundleCleanup(
			err,
			bundle.wait(context.Background()),
		)
	}
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

func nilMethodHandler(handler funcapi.MethodHandler) bool {
	if handler == nil {
		return true
	}
	reflected := reflect.ValueOf(handler)
	switch reflected.Kind() {
	case reflect.Chan, reflect.Func, reflect.Map, reflect.Pointer, reflect.Slice:
		return reflected.IsNil()
	default:
		return false
	}
}

func joinRetainedBundleCleanup(err, cleanupErr error) error {
	err = errors.Join(err, cleanupErr)
	if cleanupErr != nil {
		err = lifecycle.RetainOwnership(err)
	}
	return err
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

func (fb *functionBundle) acquire() error {
	if fb == nil {
		return errors.New("jobmgr Function bundle: nil acquire")
	}
	fb.mu.Lock()
	defer fb.mu.Unlock()
	if fb.retired || fb.references <= 0 {
		return errors.New("jobmgr Function bundle: acquire after retirement")
	}
	fb.references++
	return nil
}

func (fb *functionBundle) release() {
	if fb == nil {
		return
	}
	fb.mu.Lock()
	if fb.references <= 0 {
		fb.mu.Unlock()
		panic("jobmgr Function bundle: reference underflow")
	}
	fb.references--
	start := fb.startCleanupLocked()
	fb.mu.Unlock()
	if start {
		go fb.cleanup()
	}
}

func (fb *functionBundle) retire() {
	if fb == nil {
		return
	}
	fb.mu.Lock()
	if fb.retired {
		fb.mu.Unlock()
		return
	}
	fb.retired = true
	fb.references--
	start := fb.startCleanupLocked()
	fb.mu.Unlock()
	if start {
		go fb.cleanup()
	}
}

func (fb *functionBundle) startCleanupLocked() bool {
	if !fb.retired || fb.references != 0 || fb.cleanupStart {
		return false
	}
	fb.cleanupStart = true
	return true
}

func (fb *functionBundle) cleanup() {
	if fb.handler != nil {
		// Caller waits stay cancelable; the enclosing module/job attempt owns
		// physical cleanup until this callback returns or the process exits.
		fb.cleanupErr = callMethodCleanup(context.Background(), fb.handler)
	}
	close(fb.cleanupDone)
}

func (fb *functionBundle) wait(ctx context.Context) error {
	if fb == nil {
		return nil
	}
	select {
	case <-fb.cleanupDone:
		fb.mu.Lock()
		defer fb.mu.Unlock()
		return fb.cleanupErr
	case <-ctx.Done():
		return ctx.Err()
	}
}

func (fb *functionBundle) refreshAvailability() (bool, error) {
	next, err := fb.evaluateAvailability()
	if err != nil {
		return false, err
	}
	return fb.commitAvailability(next), nil
}

func (fb *functionBundle) evaluateAvailability() (map[string]bool, error) {
	if fb == nil {
		return nil, errors.New("jobmgr Function bundle: nil availability refresh")
	}
	next := make(map[string]bool, len(fb.methods))
	for _, method := range fb.methods {
		available := true
		var err error
		switch fb.kind {
		case functionBundleAgent:
			if method.Available != nil {
				available, err = callFunctionAvailability(method.Available)
			}
		case functionBundleJob:
			if callback, ok := fb.job.Collector().(collectorapi.FunctionAvailability); ok && callback != nil {
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

func (fb *functionBundle) commitAvailability(next map[string]bool) bool {
	if fb == nil {
		return false
	}
	fb.mu.Lock()
	defer fb.mu.Unlock()
	if fb.retired {
		return false
	}
	changed := len(fb.availability) != len(next)
	if !changed {
		for method, available := range next {
			if fb.availability[method] != available {
				changed = true
				break
			}
		}
	}
	fb.availability = next
	return changed
}

func (fb *functionBundle) available(method string) bool {
	if fb == nil {
		return false
	}
	fb.mu.Lock()
	defer fb.mu.Unlock()
	return !fb.retired && !fb.quarantined && fb.availability[method]
}
