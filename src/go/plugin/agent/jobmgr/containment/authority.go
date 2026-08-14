// SPDX-License-Identifier: GPL-3.0-or-later

// Package containment owns process-lifetime work whose implementation may not
// cooperate with cancellation. It bounds logical waiting and same-identity
// concurrency; only process exit can reclaim a permanently blocked worker.
package containment

import (
	"context"
	"errors"
	"sort"
	"sync"
	"time"

	"github.com/netdata/netdata/go/plugins/plugin/agent/jobmgr"
	"github.com/netdata/netdata/go/plugins/plugin/agent/jobmgr/lifecycle"
)

const (
	DefaultFuse                     = 2 * time.Minute
	DefaultSupersessionGrace        = 2 * time.Second
	MaximumDiagnosticIdentitySample = 8
)

type identityKey struct {
	namespace jobmgr.ProcessAttemptNamespace
	key       string
}

func mapKey(identity jobmgr.ProcessAttemptIdentity) identityKey {
	return identityKey{
		namespace: identity.Namespace,
		key:       identity.Key,
	}
}

type policy struct {
	fuse              time.Duration
	supersessionGrace time.Duration
}

func (p policy) valid() bool {
	return p.fuse > 0 && p.supersessionGrace > 0
}

// Census is the exact process-owned attempt accounting.
type Census struct {
	Active      int
	Probing     int
	Admitted    int
	Contained   int
	Quarantined int
}

type attemptState uint8

const (
	attemptStateProbing attemptState = iota + 1
	attemptStateAdmitted
	attemptStateContained
	attemptStateReleased
)

// Authority owns the identity registry and the complete physical lifetime of
// every worker it starts.
type Authority struct {
	mu             sync.Mutex
	diagnostics    jobmgr.DiagnosticObserver
	policy         policy
	attempts       map[identityKey]*attempt
	quarantines    map[identityKey]struct{} // non-active identities unsafe to reuse before process exit
	census         Census
	retiredThrough uint64
	stopping       bool
	drained        chan struct{}
}

// attempt is a process-owned worker and its logical settlement.
type attempt struct {
	authority *Authority
	identity  jobmgr.ProcessAttemptIdentity
	target    uint64
	started   time.Time

	ctx      context.Context
	cancel   context.CancelCauseFunc
	timer    *time.Timer
	state    attemptState
	admitted bool
	result   error
	fence    func(error)
	fenceErr error
	settled  chan struct{}
	released chan struct{}
}

func NewAuthority(diagnostics jobmgr.DiagnosticObserver) (*Authority, error) {
	return newAuthority(diagnostics, policy{
		fuse:              DefaultFuse,
		supersessionGrace: DefaultSupersessionGrace,
	})
}

func newAuthority(diagnostics jobmgr.DiagnosticObserver, policy policy) (*Authority, error) {
	if !policy.valid() {
		return nil, errors.New("jobmgr containment: invalid authority policy")
	}
	return &Authority{
		diagnostics: diagnostics,
		policy:      policy,
		attempts:    make(map[identityKey]*attempt),
		quarantines: make(map[identityKey]struct{}),
		drained:     make(chan struct{}),
	}, nil
}

// start reserves identity before starting exactly one worker.
func (a *Authority) start(ctx context.Context, plan jobmgr.ProcessAttemptPlan) (*attempt, error) {
	if a == nil || ctx == nil || !plan.Identity.Valid() || plan.Work == nil {
		return nil, errors.New("jobmgr containment: invalid attempt plan")
	}
	a.mu.Lock()
	if cause := context.Cause(ctx); cause != nil {
		a.mu.Unlock()
		return nil, cause
	}
	if a.stopping {
		a.mu.Unlock()
		return nil, jobmgr.ErrProcessAttemptStopped
	}
	if plan.Target != 0 && plan.Target <= a.retiredThrough {
		a.mu.Unlock()
		return nil, jobmgr.ErrProcessAttemptRetired
	}
	key := mapKey(plan.Identity)
	if _, quarantined := a.quarantines[key]; quarantined {
		a.mu.Unlock()
		return nil, jobmgr.ErrProcessAttemptQuarantined
	}
	if a.attempts[key] != nil {
		a.mu.Unlock()
		return nil, jobmgr.ErrProcessAttemptBusy
	}
	workerCtx, cancel := context.WithCancelCause(context.Background())
	attempt := &attempt{
		authority: a,
		identity:  plan.Identity,
		target:    plan.Target,
		started:   time.Now(),
		ctx:       workerCtx,
		cancel:    cancel,
		state:     attemptStateProbing,
		fence:     plan.OnContainment,
		settled:   make(chan struct{}),
		released:  make(chan struct{}),
	}
	a.attempts[key] = attempt
	a.census.Active++
	a.census.Probing++
	attempt.timer = time.AfterFunc(a.policy.fuse, attempt.cutFuse)
	a.mu.Unlock()

	go attempt.run(plan.Work)
	return attempt, nil
}

// StartProcessAttempt checks caller cancellation while holding the identity
// registry lock. A successful reservation linearizes at that snapshot; the
// caller context gates admission only and the worker remains process-owned.
func (a *Authority) StartProcessAttempt(
	ctx context.Context,
	plan jobmgr.ProcessAttemptPlan,
) (jobmgr.ProcessAttempt, error) {
	return a.start(ctx, plan)
}

func (at *attempt) run(work func(context.Context, jobmgr.ProcessAttemptAdmission) error) {
	err := callWork(at.ctx, at, work)
	at.authority.finish(at, err)
}

func callWork(
	ctx context.Context,
	admission jobmgr.ProcessAttemptAdmission,
	work func(context.Context, jobmgr.ProcessAttemptAdmission) error,
) (err error) {
	defer func() {
		if recover() != nil {
			err = jobmgr.ErrProcessAttemptWorkerPanic
		}
	}()
	return work(ctx, admission)
}

func callContainmentFence(fence func(error), cause error) (err error) {
	if fence == nil {
		return nil
	}
	defer func() {
		if recover() != nil {
			err = jobmgr.ErrProcessAttemptFencePanic
		}
	}()
	fence(cause)
	return nil
}

// Admit atomically stops the preparation fuse. The identity remains occupied
// until the complete physical worker and cleanup lifetime returns.
func (at *attempt) Admit() error {
	if at == nil || at.authority == nil {
		return errors.New("jobmgr containment: invalid attempt admission")
	}
	authority := at.authority
	authority.mu.Lock()
	if at.state != attemptStateProbing {
		result := error(jobmgr.ErrProcessAttemptSettled)
		if at.state == attemptStateContained && at.result != nil {
			// Containment owns the terminal disposition; do not erase it when
			// the cut wins the registration-to-admission race.
			result = at.result
		}
		authority.mu.Unlock()
		return result
	}
	at.state = attemptStateAdmitted
	at.admitted = true
	authority.census.Probing--
	authority.census.Admitted++
	at.timer.Stop()
	authority.mu.Unlock()
	return nil
}

// Cut settles logical waiting immediately and cancels the worker. The identity
// stays occupied until Work returns.
func (at *attempt) Cut(cause error) bool {
	if at == nil || at.authority == nil {
		return false
	}
	if cause == nil {
		cause = context.Canceled
	}
	return at.authority.cut(at, cause, false)
}

func (at *attempt) cutFuse() {
	at.authority.cut(at, jobmgr.ErrProcessAttemptDeadline, true)
}

func (a *Authority) cut(attempt *attempt, cause error, probingOnly bool) bool {
	a.mu.Lock()
	if attempt.authority != a ||
		(probingOnly && attempt.state != attemptStateProbing) {
		a.mu.Unlock()
		return false
	}
	switch attempt.state {
	case attemptStateProbing:
		a.census.Probing--
	case attemptStateAdmitted:
		a.census.Admitted--
	default:
		a.mu.Unlock()
		return false
	}
	attempt.state = attemptStateContained
	attempt.result = cause
	a.census.Contained++
	attempt.timer.Stop()
	attempt.cancel(cause)
	fenceErr := callContainmentFence(attempt.fence, cause)
	if fenceErr != nil {
		attempt.fenceErr = fenceErr
		attempt.result = errors.Join(cause, fenceErr)
	}
	close(attempt.settled)
	census := a.census
	age := time.Since(attempt.started)
	identity := attempt.identity
	target := attempt.target
	a.mu.Unlock()

	diagnosticErr := safeCutError(cause)
	if fenceErr != nil {
		diagnosticErr = errors.Join(diagnosticErr, fenceErr)
	}
	jobmgr.ObserveDiagnostic(a.diagnostics, jobmgr.DiagnosticEvent{
		Level:      cutDiagnosticLevel(cause, fenceErr),
		Name:       "job manager attempt contained",
		Resource:   identity.Resource,
		State:      identity.Namespace.String(),
		Generation: target,
		Count:      census.Active,
		Age:        age,
		Err:        diagnosticErr,
	})
	return true
}

func cutDiagnosticLevel(cause, fenceErr error) jobmgr.DiagnosticLevel {
	if fenceErr == nil && errors.Is(cause, jobmgr.ErrProcessAttemptRetired) {
		return jobmgr.DiagnosticInfo
	}
	return jobmgr.DiagnosticError
}

func safeCutError(cause error) error {
	switch {
	case errors.Is(cause, jobmgr.ErrProcessAttemptDeadline):
		return jobmgr.ErrProcessAttemptDeadline
	case errors.Is(cause, jobmgr.ErrProcessAttemptSuperseded):
		return jobmgr.ErrProcessAttemptSuperseded
	case errors.Is(cause, jobmgr.ErrProcessAttemptRetired):
		return jobmgr.ErrProcessAttemptRetired
	case errors.Is(cause, jobmgr.ErrProcessAttemptStopped):
		return jobmgr.ErrProcessAttemptStopped
	case errors.Is(cause, context.DeadlineExceeded):
		return context.DeadlineExceeded
	default:
		return context.Canceled
	}
}

func (a *Authority) finish(attempt *attempt, result error) {
	a.mu.Lock()
	if attempt.timer != nil {
		attempt.timer.Stop()
	}
	attempt.cancel(nil)
	previous := attempt.state
	quarantine := (attempt.admitted && result != nil) ||
		lifecycle.OwnershipRetained(result) ||
		errors.Is(result, jobmgr.ErrProcessAttemptWorkerPanic) ||
		attempt.fenceErr != nil
	settledResult := result
	if quarantine {
		settledResult = quarantineAttemptResult(result, attempt.fenceErr)
	}
	switch previous {
	case attemptStateProbing:
		a.census.Probing--
		attempt.result = settledResult
		close(attempt.settled)
	case attemptStateAdmitted:
		a.census.Admitted--
		attempt.result = settledResult
		close(attempt.settled)
	case attemptStateContained:
		a.census.Contained--
	case attemptStateReleased:
		a.mu.Unlock()
		return
	}
	attempt.state = attemptStateReleased
	key := mapKey(attempt.identity)
	if quarantine {
		if _, exists := a.quarantines[key]; !exists {
			a.quarantines[key] = struct{}{}
			a.census.Quarantined++
		}
	}
	if a.attempts[key] == attempt {
		delete(a.attempts, key)
	}
	a.census.Active--
	close(attempt.released)
	if a.stopping && a.census.Active == 0 {
		close(a.drained)
	}
	identity := attempt.identity
	target := attempt.target
	age := time.Since(attempt.started)
	census := a.census
	a.mu.Unlock()

	if errors.Is(result, jobmgr.ErrProcessAttemptWorkerPanic) {
		jobmgr.ObserveDiagnostic(a.diagnostics, jobmgr.DiagnosticEvent{
			Level:      jobmgr.DiagnosticError,
			Name:       "job manager attempt worker panicked",
			Resource:   identity.Resource,
			State:      identity.Namespace.String(),
			Generation: target,
			Age:        age,
			Err:        jobmgr.ErrProcessAttemptWorkerPanic,
		})
	}
	if previous == attemptStateContained {
		jobmgr.ObserveDiagnostic(a.diagnostics, jobmgr.DiagnosticEvent{
			Level:      jobmgr.DiagnosticInfo,
			Name:       "job manager contained attempt released",
			Resource:   identity.Resource,
			State:      identity.Namespace.String(),
			Generation: target,
			Age:        age,
		})
	}
	if quarantine {
		jobmgr.ObserveDiagnostic(a.diagnostics, jobmgr.DiagnosticEvent{
			Level:      jobmgr.DiagnosticError,
			Name:       "job manager attempt identity quarantined",
			Resource:   identity.Resource,
			State:      identity.Namespace.String(),
			Generation: target,
			Count:      census.Quarantined,
			Age:        age,
			Err:        jobmgr.ErrProcessAttemptQuarantined,
		})
	}
}

func quarantineAttemptResult(result, fenceErr error) error {
	err := jobmgr.ErrProcessAttemptQuarantined
	if errors.Is(result, jobmgr.ErrProcessAttemptWorkerPanic) {
		err = errors.Join(err, jobmgr.ErrProcessAttemptWorkerPanic)
	}
	if fenceErr != nil {
		err = errors.Join(err, jobmgr.ErrProcessAttemptFencePanic)
	}
	return err
}

// Await returns the first logical disposition handed to the caller. Cancellation
// observed before that handoff wins even when settlement is already selectable;
// it cuts the attempt but never waits for physical worker return.
func (at *attempt) Await(ctx context.Context) error {
	if at == nil || at.authority == nil || ctx == nil {
		return errors.New("jobmgr containment: invalid attempt wait")
	}
	if err := ctx.Err(); err != nil {
		at.Cut(err)
		return err
	}
	select {
	case <-at.settled:
	case <-ctx.Done():
		err := ctx.Err()
		at.Cut(err)
		return err
	}
	if err := ctx.Err(); err != nil {
		at.Cut(err)
		return err
	}
	at.authority.mu.Lock()
	defer at.authority.mu.Unlock()
	return at.result
}

// Released closes only after Work, including required cleanup, has returned.
func (at *attempt) Released() <-chan struct{} {
	if at == nil {
		return nil
	}
	return at.released
}

func (a *Authority) Census() Census {
	if a == nil {
		return Census{}
	}
	a.mu.Lock()
	defer a.mu.Unlock()
	return a.census
}

// CutProcessAttempt logically settles one identity without waiting for its
// physical worker to return.
func (a *Authority) CutProcessAttempt(
	identity jobmgr.ProcessAttemptIdentity,
	cause error,
) bool {
	if a == nil || !identity.Valid() {
		return false
	}
	a.mu.Lock()
	attempt := a.attempts[mapKey(identity)]
	a.mu.Unlock()
	return attempt != nil && attempt.Cut(cause)
}

// ProcessAttemptReleased returns the current physical owner's release signal.
func (a *Authority) ProcessAttemptReleased(
	identity jobmgr.ProcessAttemptIdentity,
) (<-chan struct{}, bool) {
	if a == nil || !identity.Valid() {
		return nil, false
	}
	a.mu.Lock()
	attempt := a.attempts[mapKey(identity)]
	a.mu.Unlock()
	if attempt == nil {
		return nil, false
	}
	return attempt.Released(), true
}

// SupersedeProcessAttempt cancels the owner observed during its locked registry
// snapshot and waits only the fixed grace for physical release. Success is not
// an identity reservation; callers must perform an authoritative start.
func (a *Authority) SupersedeProcessAttempt(
	ctx context.Context,
	identity jobmgr.ProcessAttemptIdentity,
) error {
	if a == nil || ctx == nil || !identity.Valid() {
		return errors.New("jobmgr containment: invalid supersession")
	}
	a.mu.Lock()
	key := mapKey(identity)
	attempt := a.attempts[key]
	_, quarantined := a.quarantines[key]
	a.mu.Unlock()
	if quarantined {
		return jobmgr.ErrProcessAttemptQuarantined
	}
	if attempt == nil {
		return nil
	}
	attempt.Cut(jobmgr.ErrProcessAttemptSuperseded)
	timer := time.NewTimer(a.policy.supersessionGrace)
	defer timer.Stop()
	select {
	case <-attempt.released:
		return nil
	case <-ctx.Done():
		return ctx.Err()
	case <-timer.C:
		return jobmgr.ErrProcessAttemptBusy
	}
}

// CutTarget permanently fences every live attempt associated with target.
func (a *Authority) CutTarget(target uint64) int {
	if a == nil || target == 0 {
		return 0
	}
	a.mu.Lock()
	a.retiredThrough = max(a.retiredThrough, target)
	attempts := make([]*attempt, 0)
	for _, attempt := range a.attempts {
		if attempt.target != 0 && attempt.target <= a.retiredThrough {
			attempts = append(attempts, attempt)
		}
	}
	a.mu.Unlock()
	cut := 0
	for _, attempt := range attempts {
		if attempt.Cut(jobmgr.ErrProcessAttemptRetired) {
			cut++
		}
	}
	return cut
}

// BeginShutdown rejects new work, cancels every live identity, and emits a
// bounded retained sample. Shutdown performs the bounded wait.
func (a *Authority) BeginShutdown() {
	if a == nil {
		return
	}
	a.mu.Lock()
	first := !a.stopping
	if first {
		a.stopping = true
		if a.census.Active == 0 {
			close(a.drained)
		}
	}
	attempts := make([]*attempt, 0, len(a.attempts))
	for _, attempt := range a.attempts {
		attempts = append(attempts, attempt)
	}
	total := a.census.Active
	a.mu.Unlock()

	if !first {
		return
	}
	sort.Slice(attempts, func(left, right int) bool {
		lhs, rhs := attempts[left].identity, attempts[right].identity
		if lhs.Namespace != rhs.Namespace {
			return lhs.Namespace < rhs.Namespace
		}
		if lhs.Resource != rhs.Resource {
			return lhs.Resource < rhs.Resource
		}
		return lhs.Key < rhs.Key
	})
	for _, attempt := range attempts {
		attempt.Cut(jobmgr.ErrProcessAttemptStopped)
	}
	if total > 0 {
		jobmgr.ObserveDiagnostic(a.diagnostics, jobmgr.DiagnosticEvent{
			Level: jobmgr.DiagnosticError,
			Name:  "job manager process retained attempts",
			State: "stopping",
			Count: total,
		})
		limit := min(len(attempts), MaximumDiagnosticIdentitySample)
		for _, attempt := range attempts[:limit] {
			jobmgr.ObserveDiagnostic(a.diagnostics, jobmgr.DiagnosticEvent{
				Level:      jobmgr.DiagnosticWarning,
				Name:       "job manager process retained attempt",
				Resource:   attempt.identity.Resource,
				State:      attempt.identity.Namespace.String(),
				Generation: attempt.target,
				Age:        time.Since(attempt.started),
			})
		}
	}
}

// Shutdown rejects new work, cancels every live identity, emits a bounded
// retained sample, and waits only the caller's existing shutdown budget.
func (a *Authority) Shutdown(ctx context.Context) error {
	if a == nil || ctx == nil {
		return errors.New("jobmgr containment: invalid authority shutdown")
	}
	a.BeginShutdown()
	a.mu.Lock()
	drained := a.drained
	a.mu.Unlock()
	select {
	case <-drained:
		return nil
	case <-ctx.Done():
		return ctx.Err()
	}
}
