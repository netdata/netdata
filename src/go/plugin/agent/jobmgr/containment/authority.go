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

var (
	ErrIdentityBusy        = errors.New("jobmgr containment: identity busy")
	ErrContainmentDeadline = errors.New("jobmgr containment: attempt crossed the containment fuse")
	ErrSuperseded          = errors.New("jobmgr containment: attempt superseded")
	ErrTargetRetired       = errors.New("jobmgr containment: target generation retired")
	ErrAuthorityStopped    = errors.New("jobmgr containment: process authority stopped")
	ErrAttemptSettled      = errors.New("jobmgr containment: attempt already settled")
	ErrWorkerPanic         = errors.New("jobmgr containment: worker panic")
)

// Namespace keeps operational work and same-payload tests in independent
// identity domains.
type Namespace uint8

const (
	NamespaceJob Namespace = iota + 1
	NamespaceJobTest
	NamespaceStore
	NamespaceStoreTest
	NamespaceFunctionPoll
	NamespaceFunctionInvocation
	NamespaceServiceDiscovery
)

func (namespace Namespace) valid() bool {
	return namespace >= NamespaceJob && namespace <= NamespaceServiceDiscovery
}

func (namespace Namespace) String() string {
	switch namespace {
	case NamespaceJob:
		return "job"
	case NamespaceJobTest:
		return "job-test"
	case NamespaceStore:
		return "store"
	case NamespaceStoreTest:
		return "store-test"
	case NamespaceFunctionPoll:
		return "function-poll"
	case NamespaceFunctionInvocation:
		return "function-invocation"
	case NamespaceServiceDiscovery:
		return "service-discovery"
	default:
		return "invalid"
	}
}

// Identity contains an opaque equality key and a separately supplied,
// diagnostic-safe resource name. Key is memory-only and is never observed.
type Identity struct {
	Namespace Namespace
	Key       string
	Resource  string
}

func (identity Identity) valid() bool {
	return identity.Namespace.valid() &&
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
	namespace Namespace
	key       string
}

func (identity Identity) mapKey() identityKey {
	return identityKey{
		namespace: identity.Namespace,
		key:       identity.Key,
	}
}

// Plan describes one complete physical lifetime. Work must include any
// required cleanup before returning.
type Plan struct {
	Identity Identity
	Target   uint64
	Work     func(context.Context) error
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

// AttemptState is the current physical ownership state.
type AttemptState uint8

const (
	AttemptStateProbing AttemptState = iota + 1
	AttemptStateAdmitted
	AttemptStateContained
	AttemptStateReleased
)

// Authority owns the identity registry and the complete physical lifetime of
// every worker it starts.
type Authority struct {
	mu          sync.Mutex
	diagnostics jobmgr.DiagnosticObserver
	policy      policy
	attempts    map[identityKey]*Attempt
	census      Census
	stopping    bool
	drained     chan struct{}
}

// Attempt is a process-owned worker and its logical settlement.
type Attempt struct {
	authority *Authority
	identity  Identity
	target    uint64
	started   time.Time

	ctx      context.Context
	cancel   context.CancelCauseFunc
	timer    *time.Timer
	state    AttemptState
	result   error
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
		attempts:    make(map[identityKey]*Attempt),
		drained:     make(chan struct{}),
	}, nil
}

// Start reserves identity before starting exactly one worker.
func (authority *Authority) Start(plan Plan) (*Attempt, error) {
	if authority == nil || !plan.Identity.valid() || plan.Work == nil {
		return nil, errors.New("jobmgr containment: invalid attempt plan")
	}
	authority.mu.Lock()
	if authority.stopping {
		authority.mu.Unlock()
		return nil, ErrAuthorityStopped
	}
	key := plan.Identity.mapKey()
	if authority.attempts[key] != nil {
		authority.mu.Unlock()
		return nil, ErrIdentityBusy
	}
	ctx, cancel := context.WithCancelCause(context.Background())
	attempt := &Attempt{
		authority: authority,
		identity:  plan.Identity,
		target:    plan.Target,
		started:   time.Now(),
		ctx:       ctx,
		cancel:    cancel,
		state:     AttemptStateProbing,
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

func (attempt *Attempt) run(work func(context.Context) error) {
	err := callWork(attempt.ctx, work)
	attempt.authority.finish(attempt, err)
}

func callWork(ctx context.Context, work func(context.Context) error) (err error) {
	defer func() {
		if recover() != nil {
			err = ErrWorkerPanic
		}
	}()
	return work(ctx)
}

// Admit atomically stops the preparation fuse. The identity remains occupied
// until the complete physical worker and cleanup lifetime returns.
func (attempt *Attempt) Admit() error {
	if attempt == nil || attempt.authority == nil {
		return errors.New("jobmgr containment: invalid attempt admission")
	}
	authority := attempt.authority
	authority.mu.Lock()
	if attempt.state != AttemptStateProbing {
		authority.mu.Unlock()
		return ErrAttemptSettled
	}
	attempt.state = AttemptStateAdmitted
	authority.census.Probing--
	authority.census.Admitted++
	attempt.timer.Stop()
	authority.mu.Unlock()
	return nil
}

// Cut settles logical waiting immediately and cancels the worker. The identity
// stays occupied until Work returns.
func (attempt *Attempt) Cut(cause error) bool {
	if attempt == nil || attempt.authority == nil {
		return false
	}
	if cause == nil {
		cause = context.Canceled
	}
	return attempt.authority.cut(attempt, cause, false)
}

func (attempt *Attempt) cutFuse() {
	attempt.authority.cut(attempt, ErrContainmentDeadline, true)
}

func (authority *Authority) cut(attempt *Attempt, cause error, probingOnly bool) bool {
	authority.mu.Lock()
	if attempt.authority != authority ||
		(probingOnly && attempt.state != AttemptStateProbing) {
		authority.mu.Unlock()
		return false
	}
	switch attempt.state {
	case AttemptStateProbing:
		authority.census.Probing--
	case AttemptStateAdmitted:
		authority.census.Admitted--
	default:
		authority.mu.Unlock()
		return false
	}
	attempt.state = AttemptStateContained
	attempt.result = cause
	authority.census.Contained++
	attempt.timer.Stop()
	attempt.cancel(cause)
	close(attempt.settled)
	census := authority.census
	age := time.Since(attempt.started)
	identity := attempt.identity
	target := attempt.target
	authority.mu.Unlock()

	jobmgr.ObserveDiagnostic(authority.diagnostics, jobmgr.DiagnosticEvent{
		Level:      jobmgr.DiagnosticError,
		Name:       "job manager attempt contained",
		Resource:   identity.Resource,
		State:      identity.Namespace.String(),
		Generation: target,
		Count:      census.Active,
		Age:        age,
		Err:        safeCutError(cause),
	})
	return true
}

func safeCutError(cause error) error {
	switch {
	case errors.Is(cause, ErrContainmentDeadline):
		return ErrContainmentDeadline
	case errors.Is(cause, ErrSuperseded):
		return ErrSuperseded
	case errors.Is(cause, ErrTargetRetired):
		return ErrTargetRetired
	case errors.Is(cause, ErrAuthorityStopped):
		return ErrAuthorityStopped
	case errors.Is(cause, context.DeadlineExceeded):
		return context.DeadlineExceeded
	default:
		return context.Canceled
	}
}

func (authority *Authority) finish(attempt *Attempt, result error) {
	authority.mu.Lock()
	if attempt.timer != nil {
		attempt.timer.Stop()
	}
	attempt.cancel(nil)
	previous := attempt.state
	switch previous {
	case AttemptStateProbing:
		authority.census.Probing--
		attempt.result = result
		close(attempt.settled)
	case AttemptStateAdmitted:
		authority.census.Admitted--
		attempt.result = result
		close(attempt.settled)
	case AttemptStateContained:
		authority.census.Contained--
	case AttemptStateReleased:
		authority.mu.Unlock()
		return
	}
	attempt.state = AttemptStateReleased
	key := attempt.identity.mapKey()
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

	if errors.Is(result, ErrWorkerPanic) {
		jobmgr.ObserveDiagnostic(authority.diagnostics, jobmgr.DiagnosticEvent{
			Level:      jobmgr.DiagnosticError,
			Name:       "job manager attempt worker panicked",
			Resource:   identity.Resource,
			State:      identity.Namespace.String(),
			Generation: target,
			Age:        age,
			Err:        ErrWorkerPanic,
		})
	}
	if previous == AttemptStateContained {
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
func (attempt *Attempt) Await(ctx context.Context) error {
	if attempt == nil || attempt.authority == nil || ctx == nil {
		return errors.New("jobmgr containment: invalid attempt wait")
	}
	select {
	case <-attempt.settled:
	case <-ctx.Done():
		attempt.Cut(ctx.Err())
		<-attempt.settled
	}
	attempt.authority.mu.Lock()
	defer attempt.authority.mu.Unlock()
	return attempt.result
}

// Released closes only after Work, including required cleanup, has returned.
func (attempt *Attempt) Released() <-chan struct{} {
	if attempt == nil {
		return nil
	}
	return attempt.released
}

func (attempt *Attempt) State() AttemptState {
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

// Supersede cancels one identity and waits only the fixed grace for physical
// release. A still-live worker remains the exclusive owner.
func (authority *Authority) Supersede(ctx context.Context, identity Identity) error {
	if authority == nil || ctx == nil || !identity.valid() {
		return errors.New("jobmgr containment: invalid supersession")
	}
	authority.mu.Lock()
	attempt := authority.attempts[identity.mapKey()]
	authority.mu.Unlock()
	if attempt == nil {
		return nil
	}
	attempt.Cut(ErrSuperseded)
	timer := time.NewTimer(authority.policy.supersessionGrace)
	defer timer.Stop()
	select {
	case <-attempt.released:
		return nil
	case <-ctx.Done():
		return ctx.Err()
	case <-timer.C:
		return ErrIdentityBusy
	}
}

// CutTarget permanently fences every live attempt associated with target.
func (authority *Authority) CutTarget(target uint64) int {
	if authority == nil || target == 0 {
		return 0
	}
	authority.mu.Lock()
	attempts := make([]*Attempt, 0)
	for _, attempt := range authority.attempts {
		if attempt.target == target {
			attempts = append(attempts, attempt)
		}
	}
	authority.mu.Unlock()
	cut := 0
	for _, attempt := range attempts {
		if attempt.Cut(ErrTargetRetired) {
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
	attempts := make([]*Attempt, 0, len(authority.attempts))
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
		attempt.Cut(ErrAuthorityStopped)
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
