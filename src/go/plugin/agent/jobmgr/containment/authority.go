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
	"unicode/utf8"

	"github.com/netdata/netdata/go/plugins/plugin/agent/jobmgr"
)

const (
	DefaultFuse                        = 2 * time.Minute
	DefaultSupersessionGrace           = 2 * time.Second
	MaximumDiagnosticIdentitySample    = 8
	maximumIdentityKeyBytes            = 4 * 1024
	maximumDiagnosticResourceNameBytes = 256
)

func validIdentity(identity jobmgr.ProcessAttemptIdentity) bool {
	return identity.Namespace >= jobmgr.ProcessAttemptJob &&
		identity.Namespace <= jobmgr.ProcessAttemptServiceDiscovery &&
		identity.Key != "" &&
		len(identity.Key) <= maximumIdentityKeyBytes &&
		validDiagnosticResource(identity.Resource)
}

func validDiagnosticResource(resource string) bool {
	if resource == "" ||
		len(resource) > maximumDiagnosticResourceNameBytes ||
		!utf8.ValidString(resource) {
		return false
	}
	for _, char := range resource {
		if char < ' ' || char == 0x7f {
			return false
		}
	}
	return true
}

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

func (value policy) valid() bool {
	return value.fuse > 0 && value.supersessionGrace > 0
}

// Census is the exact process-owned attempt accounting.
type Census struct {
	Active    int
	Probing   int
	Admitted  int
	Contained int
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
	result   error
	fence    func()
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
		drained:     make(chan struct{}),
	}, nil
}

// Start reserves identity before starting exactly one worker.
func (authority *Authority) start(plan jobmgr.ProcessAttemptPlan) (*attempt, error) {
	if authority == nil || !validIdentity(plan.Identity) || plan.Work == nil {
		return nil, errors.New("jobmgr containment: invalid attempt plan")
	}
	authority.mu.Lock()
	if authority.stopping {
		authority.mu.Unlock()
		return nil, jobmgr.ErrProcessAttemptStopped
	}
	if plan.Target != 0 && plan.Target <= authority.retiredThrough {
		authority.mu.Unlock()
		return nil, jobmgr.ErrProcessAttemptRetired
	}
	key := mapKey(plan.Identity)
	if authority.attempts[key] != nil {
		authority.mu.Unlock()
		return nil, jobmgr.ErrProcessAttemptBusy
	}
	ctx, cancel := context.WithCancelCause(context.Background())
	attempt := &attempt{
		authority: authority,
		identity:  plan.Identity,
		target:    plan.Target,
		started:   time.Now(),
		ctx:       ctx,
		cancel:    cancel,
		state:     attemptStateProbing,
		fence:     plan.OnContainment,
		settled:   make(chan struct{}),
		released:  make(chan struct{}),
	}
	authority.attempts[key] = attempt
	authority.census.Active++
	authority.census.Probing++
	attempt.timer = time.AfterFunc(authority.policy.fuse, attempt.cutFuse)
	authority.mu.Unlock()

	go attempt.run(plan.Work)
	return attempt, nil
}

func (authority *Authority) StartProcessAttempt(
	plan jobmgr.ProcessAttemptPlan,
) (jobmgr.ProcessAttempt, error) {
	return authority.start(plan)
}

func (attempt *attempt) run(work func(context.Context, jobmgr.ProcessAttemptAdmission) error) {
	err := callWork(attempt.ctx, attempt, work)
	attempt.authority.finish(attempt, err)
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

func callContainmentFence(fence func()) (err error) {
	if fence == nil {
		return nil
	}
	defer func() {
		if recover() != nil {
			err = jobmgr.ErrProcessAttemptFencePanic
		}
	}()
	fence()
	return nil
}

// Admit atomically stops the preparation fuse. The identity remains occupied
// until the complete physical worker and cleanup lifetime returns.
func (attempt *attempt) Admit() error {
	if attempt == nil || attempt.authority == nil {
		return errors.New("jobmgr containment: invalid attempt admission")
	}
	authority := attempt.authority
	authority.mu.Lock()
	if attempt.state != attemptStateProbing {
		authority.mu.Unlock()
		return jobmgr.ErrProcessAttemptSettled
	}
	attempt.state = attemptStateAdmitted
	authority.census.Probing--
	authority.census.Admitted++
	attempt.timer.Stop()
	authority.mu.Unlock()
	return nil
}

// Cut settles logical waiting immediately and cancels the worker. The identity
// stays occupied until Work returns.
func (attempt *attempt) Cut(cause error) bool {
	if attempt == nil || attempt.authority == nil {
		return false
	}
	if cause == nil {
		cause = context.Canceled
	}
	return attempt.authority.cut(attempt, cause, false)
}

func (attempt *attempt) cutFuse() {
	attempt.authority.cut(attempt, jobmgr.ErrProcessAttemptDeadline, true)
}

func (authority *Authority) cut(attempt *attempt, cause error, probingOnly bool) bool {
	authority.mu.Lock()
	if attempt.authority != authority ||
		(probingOnly && attempt.state != attemptStateProbing) {
		authority.mu.Unlock()
		return false
	}
	switch attempt.state {
	case attemptStateProbing:
		authority.census.Probing--
	case attemptStateAdmitted:
		authority.census.Admitted--
	default:
		authority.mu.Unlock()
		return false
	}
	attempt.state = attemptStateContained
	attempt.result = cause
	authority.census.Contained++
	attempt.timer.Stop()
	attempt.cancel(cause)
	fenceErr := callContainmentFence(attempt.fence)
	if fenceErr != nil {
		attempt.result = errors.Join(cause, fenceErr)
	}
	close(attempt.settled)
	census := authority.census
	age := time.Since(attempt.started)
	identity := attempt.identity
	target := attempt.target
	authority.mu.Unlock()

	diagnosticErr := safeCutError(cause)
	if fenceErr != nil {
		diagnosticErr = errors.Join(diagnosticErr, fenceErr)
	}
	jobmgr.ObserveDiagnostic(authority.diagnostics, jobmgr.DiagnosticEvent{
		Level:      jobmgr.DiagnosticError,
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

func (authority *Authority) finish(attempt *attempt, result error) {
	authority.mu.Lock()
	if attempt.timer != nil {
		attempt.timer.Stop()
	}
	attempt.cancel(nil)
	previous := attempt.state
	switch previous {
	case attemptStateProbing:
		authority.census.Probing--
		attempt.result = result
		close(attempt.settled)
	case attemptStateAdmitted:
		authority.census.Admitted--
		attempt.result = result
		close(attempt.settled)
	case attemptStateContained:
		authority.census.Contained--
	case attemptStateReleased:
		authority.mu.Unlock()
		return
	}
	attempt.state = attemptStateReleased
	key := mapKey(attempt.identity)
	if authority.attempts[key] == attempt {
		delete(authority.attempts, key)
	}
	authority.census.Active--
	close(attempt.released)
	if authority.stopping && authority.census.Active == 0 {
		close(authority.drained)
	}
	identity := attempt.identity
	target := attempt.target
	age := time.Since(attempt.started)
	authority.mu.Unlock()

	if errors.Is(result, jobmgr.ErrProcessAttemptWorkerPanic) {
		jobmgr.ObserveDiagnostic(authority.diagnostics, jobmgr.DiagnosticEvent{
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
		jobmgr.ObserveDiagnostic(authority.diagnostics, jobmgr.DiagnosticEvent{
			Level:      jobmgr.DiagnosticInfo,
			Name:       "job manager contained attempt released",
			Resource:   identity.Resource,
			State:      identity.Namespace.String(),
			Generation: target,
			Age:        age,
		})
	}
}

// Await returns the first logical disposition. Caller cancellation cuts the
// attempt but never waits for physical worker return.
func (attempt *attempt) Await(ctx context.Context) error {
	if attempt == nil || attempt.authority == nil || ctx == nil {
		return errors.New("jobmgr containment: invalid attempt wait")
	}
	if err := ctx.Err(); err != nil {
		attempt.Cut(err)
		return err
	}
	select {
	case <-attempt.settled:
	case <-ctx.Done():
		err := ctx.Err()
		attempt.Cut(err)
		return err
	}
	if err := ctx.Err(); err != nil {
		attempt.Cut(err)
		return err
	}
	attempt.authority.mu.Lock()
	defer attempt.authority.mu.Unlock()
	return attempt.result
}

// Released closes only after Work, including required cleanup, has returned.
func (attempt *attempt) Released() <-chan struct{} {
	if attempt == nil {
		return nil
	}
	return attempt.released
}

func (attempt *attempt) stateSnapshot() attemptState {
	if attempt == nil || attempt.authority == nil {
		return 0
	}
	attempt.authority.mu.Lock()
	defer attempt.authority.mu.Unlock()
	return attempt.state
}

func (authority *Authority) Census() Census {
	if authority == nil {
		return Census{}
	}
	authority.mu.Lock()
	defer authority.mu.Unlock()
	return authority.census
}

// CutProcessAttempt logically settles one identity without waiting for its
// physical worker to return.
func (authority *Authority) CutProcessAttempt(
	identity jobmgr.ProcessAttemptIdentity,
	cause error,
) bool {
	if authority == nil || !validIdentity(identity) {
		return false
	}
	authority.mu.Lock()
	attempt := authority.attempts[mapKey(identity)]
	authority.mu.Unlock()
	return attempt != nil && attempt.Cut(cause)
}

// ProcessAttemptReleased returns the current physical owner's release signal.
func (authority *Authority) ProcessAttemptReleased(
	identity jobmgr.ProcessAttemptIdentity,
) (<-chan struct{}, bool) {
	if authority == nil || !validIdentity(identity) {
		return nil, false
	}
	authority.mu.Lock()
	attempt := authority.attempts[mapKey(identity)]
	authority.mu.Unlock()
	if attempt == nil {
		return nil, false
	}
	return attempt.Released(), true
}

// Supersede cancels one identity and waits only the fixed grace for physical
// release. A still-live worker remains the exclusive owner.
func (authority *Authority) SupersedeProcessAttempt(
	ctx context.Context,
	identity jobmgr.ProcessAttemptIdentity,
) error {
	if authority == nil || ctx == nil || !validIdentity(identity) {
		return errors.New("jobmgr containment: invalid supersession")
	}
	authority.mu.Lock()
	attempt := authority.attempts[mapKey(identity)]
	authority.mu.Unlock()
	if attempt == nil {
		return nil
	}
	attempt.Cut(jobmgr.ErrProcessAttemptSuperseded)
	timer := time.NewTimer(authority.policy.supersessionGrace)
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
func (authority *Authority) CutTarget(target uint64) int {
	if authority == nil || target == 0 {
		return 0
	}
	authority.mu.Lock()
	authority.retiredThrough = max(authority.retiredThrough, target)
	attempts := make([]*attempt, 0)
	for _, attempt := range authority.attempts {
		if attempt.target != 0 && attempt.target <= authority.retiredThrough {
			attempts = append(attempts, attempt)
		}
	}
	authority.mu.Unlock()
	cut := 0
	for _, attempt := range attempts {
		if attempt.Cut(jobmgr.ErrProcessAttemptRetired) {
			cut++
		}
	}
	return cut
}

// Shutdown rejects new work, cancels every live identity, emits a bounded
// retained sample, and waits only the caller's existing shutdown budget.
func (authority *Authority) BeginShutdown() {
	if authority == nil {
		return
	}
	authority.mu.Lock()
	first := !authority.stopping
	if first {
		authority.stopping = true
		if authority.census.Active == 0 {
			close(authority.drained)
		}
	}
	attempts := make([]*attempt, 0, len(authority.attempts))
	for _, attempt := range authority.attempts {
		attempts = append(attempts, attempt)
	}
	total := authority.census.Active
	authority.mu.Unlock()

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
		jobmgr.ObserveDiagnostic(authority.diagnostics, jobmgr.DiagnosticEvent{
			Level: jobmgr.DiagnosticError,
			Name:  "job manager process retained attempts",
			State: "stopping",
			Count: total,
		})
		limit := min(len(attempts), MaximumDiagnosticIdentitySample)
		for _, attempt := range attempts[:limit] {
			jobmgr.ObserveDiagnostic(authority.diagnostics, jobmgr.DiagnosticEvent{
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
func (authority *Authority) Shutdown(ctx context.Context) error {
	if authority == nil || ctx == nil {
		return errors.New("jobmgr containment: invalid authority shutdown")
	}
	authority.BeginShutdown()
	authority.mu.Lock()
	drained := authority.drained
	authority.mu.Unlock()
	select {
	case <-drained:
		return nil
	case <-ctx.Done():
		return ctx.Err()
	}
}
