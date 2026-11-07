// SPDX-License-Identifier: GPL-3.0-or-later

package joboutput

import (
	"context"
	"errors"
	"fmt"
	"reflect"
	"slices"
	"sync"
	"sync/atomic"

	"github.com/netdata/netdata/go/plugins/plugin/agent/jobmgr"
	"github.com/netdata/netdata/go/plugins/plugin/agent/jobmgr/lifecycle"
	"github.com/netdata/netdata/go/plugins/plugin/framework/confgroup"
	"github.com/netdata/netdata/go/plugins/plugin/framework/dyncfg"
	"github.com/netdata/netdata/go/plugins/plugin/framework/ownedjobs"
)

var ErrOwnedJobStale = ownedjobs.ErrStale

// ownedChildIntent is an admission capability, independent of graph visibility.
// Its revoke never waits for a collector callback, wire publication or cleanup.
type ownedChildIntent struct {
	owner        *nativeJobOwner
	key          string
	id           string
	config       confgroup.Config
	enabled      bool
	token        uint64
	parentSource string
	removed      bool
	revoked      atomic.Bool
	mu           sync.Mutex
	generations  map[*JobGeneration]struct{}
	applied      atomic.Uint64
}

func (i *ownedChildIntent) revoke() {
	i.revoked.Store(true)
	i.mu.Lock()
	for generation := range i.generations {
		generation.revokeOwnedAdmissions()
	}
	i.mu.Unlock()
}

func (i *ownedChildIntent) attach(generation *JobGeneration) {
	i.mu.Lock()
	defer i.mu.Unlock()
	if i.generations == nil {
		i.generations = make(map[*JobGeneration]struct{})
	}
	i.generations[generation] = struct{}{}
	if i.revoked.Load() {
		generation.revokeOwnedAdmissions()
	}
}

func (i *ownedChildIntent) detach(generation *JobGeneration) {
	i.mu.Lock()
	delete(i.generations, generation)
	i.mu.Unlock()
}

type ownedJobRegistry struct {
	mu           sync.Mutex
	controller   *DynCfgJobController
	ctx          context.Context
	commands     jobmgr.PreparedCommandPort
	owners       map[string]*nativeJobOwner
	reservations map[string]*ownedChildIntent
	sequence     uint64
	closed       bool
	enabled      atomic.Bool
	dispatch     sync.WaitGroup
	dispatchDone chan struct{}
}

type nativeJobOwner struct {
	registry *ownedJobRegistry
	id       string
	lifetime uint64
	revision uint64
	retired  bool
	children map[string]*ownedChildIntent
}

type ownedJobSnapshot struct {
	mu          sync.Mutex
	consumed    bool
	applyErr    error
	owner       *nativeJobOwner
	predecessor uint64
	revision    uint64
	retire      bool
	children    map[string]*ownedChildIntent
}

// BindJobOwners supplies the run's composition-only capability. It is bound
// after the native command kernel and workers, before parent services start.
func (dcjc *DynCfgJobController) BindJobOwners(
	ctx context.Context,
	commands jobmgr.PreparedCommandPort,
) (ownedjobs.Registry, error) {
	if ctx == nil || commands == nil || dcjc.owned == nil {
		return nil, errors.New("job output: invalid owner binding")
	}
	r := dcjc.owned
	r.mu.Lock()
	defer r.mu.Unlock()
	if r.commands != nil || r.closed {
		return nil, ErrOwnedJobStale
	}
	r.ctx, r.commands = ctx, commands
	r.enabled.Store(true)
	return r, nil
}

func (r *ownedJobRegistry) nextLocked() uint64 { r.sequence++; return r.sequence }

func (r *ownedJobRegistry) Bind(ctx context.Context, id string) (ownedjobs.Owner, error) {
	if ctx == nil || ctx.Err() != nil || id == "" || dyncfg.JobNameRuleStrict(id) != nil {
		return nil, errors.New("job output: invalid owner identity")
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	if r.closed || r.commands == nil {
		return nil, ErrOwnedJobStale
	}
	if incumbent := r.owners[id]; incumbent != nil && !incumbent.retired {
		return nil, errors.New("job output: owner already bound")
	}
	owner := &nativeJobOwner{
		registry: r,
		id:       id,
		lifetime: r.nextLocked(),
		children: make(map[string]*ownedChildIntent),
	}
	r.owners[id] = owner
	return owner, nil
}

func (o *nativeJobOwner) currentLocked() bool {
	r := o.registry
	return !r.closed && r.ctx != nil && r.ctx.Err() == nil && !o.retired && r.owners[o.id] == o
}

func (o *nativeJobOwner) Prepare(
	ctx context.Context,
	revision uint64,
	children []ownedjobs.Child,
) (ownedjobs.Snapshot, error) {
	if ctx == nil || ctx.Err() != nil {
		return nil, errors.New("job output: invalid owner preparation context")
	}
	r := o.registry
	r.mu.Lock()
	if !o.currentLocked() || revision <= o.revision {
		r.mu.Unlock()
		return nil, ErrOwnedJobStale
	}
	predecessor := o.revision
	r.mu.Unlock()
	snapshot := &ownedJobSnapshot{
		owner:       o,
		predecessor: predecessor,
		revision:    revision,
		children:    make(map[string]*ownedChildIntent, len(children)),
	}
	identities := make(map[string]bool, len(children))
	for _, child := range children {
		if ctx.Err() != nil {
			return nil, ctx.Err()
		}
		if dyncfg.JobNameRuleStrict(child.Key) != nil || child.Key == "" {
			return nil, errors.New("job output: invalid child key")
		}
		if snapshot.children[child.Key] != nil {
			return nil, fmt.Errorf("job output: duplicate child key %q", child.Key)
		}
		config, err := child.Config.Clone()
		if err != nil {
			return nil, err
		}
		// Keep trusted native provenance. Ownership is a separate capability.
		switch config.SourceType() {
		case confgroup.TypeUser, confgroup.TypeStock, confgroup.TypeDyncfg:
		default:
			return nil, errors.New("job output: owned child requires trusted parent provenance")
		}
		if !r.controller.ownerOnly[config.Module()] {
			return nil, errors.New("job output: module does not declare parent-owned jobs")
		}
		parentSource := config.SourceType()
		config.SetSourceType(confgroup.TypeUser).SetProvider("owned").SetSource("owner=" + o.id)
		if err := r.controller.validateCandidateMetadata(config); err != nil {
			return nil, err
		}
		if identities[config.FullName()] {
			return nil, fmt.Errorf("job output: duplicate child identity %q", config.FullName())
		}
		identities[config.FullName()] = true
		if defaults, ok := r.controller.defaults.Lookup(config.Module()); ok {
			config.ApplyDefaults(defaults)
		}
		intent := &ownedChildIntent{
			owner:   o,
			key:     child.Key,
			id:      config.FullName(),
			config:  config,
			enabled: child.Enabled,
		}
		intent.parentSource = parentSource
		snapshot.children[child.Key] = intent
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	if !o.currentLocked() || o.revision != predecessor {
		return nil, ErrOwnedJobStale
	}
	if err := snapshot.validateCollisionsLocked(); err != nil {
		return nil, err
	}
	return snapshot, nil
}

func (s *ownedJobSnapshot) validateCollisionsLocked() error {
	r := s.owner.registry
	for _, child := range s.children {
		incumbent := r.reservations[child.id]
		if incumbent != nil {
			if incumbent.owner == s.owner || incumbent.owner.id == s.owner.id && incumbent.owner.retired {
				continue
			}
			return fmt.Errorf("job output: child identity %q belongs to another owner", child.id)
		}
		if _, exists := r.controller.graph.Lookup(child.id); exists {
			return fmt.Errorf("job output: child identity %q already exists", child.id)
		}
		if state := r.controller.candidates.view(child.id); state != nil {
			return fmt.Errorf("job output: child identity %q has native source intent", child.id)
		}
	}
	return nil
}

func (s *ownedJobSnapshot) Apply(ctx context.Context) error {
	if ctx == nil {
		return errors.New("job output: invalid owner apply context")
	}
	s.mu.Lock()
	if s.consumed {
		s.mu.Unlock()
		return ErrOwnedJobStale
	}
	s.consumed = true
	s.mu.Unlock()
	r := s.owner.registry
	r.mu.Lock()
	if !s.owner.currentLocked() || s.owner.revision != s.predecessor {
		r.mu.Unlock()
		return ErrOwnedJobStale
	}
	commands := r.commands
	uid := r.nextLocked()
	r.mu.Unlock()
	id := fmt.Sprintf("\x00owned/%d", s.owner.lifetime)
	plan := jobmgr.WorkPlan{
		Claims:     []string{DynCfgJobGraphClaim},
		NoResponse: true,
		Transaction: &jobmgr.ResourceTransactionPlan{
			ID: id,
			Prepare: func(_ context.Context, current lifecycle.ReadyResource, scope lifecycle.ResourceTransactionScope, permit lifecycle.LongLivedPermit) (lifecycle.PreparedResourceTransaction, error) {
				return r.controller.noopWithAfterApply(scope, current, permit, noResponseResult(), s.adopt)
			},
		},
	}
	err := commands.SubmitPreparedAndWait(ctx, jobmgr.Request{
		UID:     fmt.Sprintf("owned-adopt-%d-%d", r.controller.generation, uid),
		LaneKey: id,
		Source:  lifecycle.SourceJobManager,
		Route:   "internal/jobs/owner",
	}, plan)
	if err != nil {
		return err
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.applyErr
}

// adopt runs under the graph claim. All changed admission is revoked before any
// native child transaction can begin slow detachment or predecessor release.
func (s *ownedJobSnapshot) adopt() {
	r := s.owner.registry
	r.mu.Lock()
	if !s.owner.currentLocked() || s.owner.revision != s.predecessor {
		s.mu.Lock()
		s.applyErr = ErrOwnedJobStale
		s.mu.Unlock()
		r.mu.Unlock()
		return
	}
	if err := s.validateCollisionsLocked(); err != nil {
		s.mu.Lock()
		s.applyErr = err
		s.mu.Unlock()
		r.mu.Unlock()
		return
	}
	var changed []*ownedChildIntent
	for key, old := range s.owner.children {
		next := s.children[key]
		if next != nil && old.id == next.id && old.enabled == next.enabled &&
			reflect.DeepEqual(old.config, next.config) {
			old.parentSource = next.parentSource
			s.children[key] = old
			continue
		}
		old.revoke()
		// Hold the reservation until native removal has detached the predecessor.
		// Owner-only module admission rejects delayed daemon/source replay even
		// after the reservation and native candidate record are reclaimed.
		tombstone := &ownedChildIntent{
			owner:   s.owner,
			key:     key,
			id:      old.id,
			token:   r.nextLocked(),
			removed: true,
			config:  old.config,
		}
		r.reservations[old.id] = tombstone
		changed = append(changed, tombstone)
	}
	for key, child := range s.children {
		if s.owner.children[key] == child {
			continue
		}
		if previous := r.reservations[child.id]; previous != nil {
			previous.revoke()
		}
		child.token = r.nextLocked()
		r.reservations[child.id] = child
		changed = append(changed, child)
	}
	s.owner.children = s.children
	s.owner.revision = s.revision
	if s.retire {
		s.owner.retired = true
		delete(r.owners, s.owner.id)
	}
	// Cancel accepted preparation and retry wakes at the same authority cut,
	// even when a changed child's resource lane is still occupied.
	for _, child := range changed {
		r.controller.scheduler.accepted.cancelUnless(child.id, "")
		r.controller.scheduler.retries.cancel(child.id)
	}
	r.dispatch.Add(1)
	r.mu.Unlock()
	// This only submits ordinary per-job transactions. Native workers retain
	// construction, retries, predecessor release and physical cleanup ownership.
	go func() {
		defer r.dispatch.Done()
		for _, child := range changed {
			if err := r.submitChild(child); err != nil && r.ctx.Err() == nil && !r.isClosed() {
				r.controller.candidateFailure(err)
				return
			}
		}
	}()
}

func (r *ownedJobRegistry) submitChild(child *ownedChildIntent) error {
	r.mu.Lock()
	if r.closed {
		r.mu.Unlock()
		return nil
	}
	uid := r.nextLocked()
	r.mu.Unlock()
	plan := jobmgr.WorkPlan{
		Claims:     []string{DynCfgJobGraphClaim},
		NoResponse: true,
		Transaction: &jobmgr.ResourceTransactionPlan{
			ID: child.id,
			Prepare: func(ctx context.Context, current lifecycle.ReadyResource, scope lifecycle.ResourceTransactionScope, permit lifecycle.LongLivedPermit) (lifecycle.PreparedResourceTransaction, error) {
				if r.current(child.id) != child {
					return r.controller.noop(scope, current, permit, noResponseResult())
				}
				var delta *candidateDelta
				var err error
				if child.removed {
					delta, err = r.controller.candidates.prepareRemove(child.id)
				} else {
					status := dyncfg.StatusAccepted
					if !child.enabled {
						status = dyncfg.StatusDisabled
					}
					delta, err = r.controller.candidates.prepareAdoptExact(child.config, child.enabled, status, true, candidateRetryPolicy{}, r.controller.scheduler.retries.now())
				}
				if err != nil {
					return nil, err
				}
				// Owner-only jobs have one exact configuration and no source alternatives.
				// Keep no historical generated payloads across repeated parent edits.
				if !child.removed && delta.after.Binding != nil {
					for key := range delta.after.Entries {
						if key != delta.after.Binding.Key {
							delete(delta.after.Entries, key)
						}
					}
				}
				delta.after.Owner = child
				if delta.after.Active != nil {
					delta.after.Active.Token.Owner = child
					delta.Decision.Work = delta.after.Active
				}
				prepared, err := r.controller.prepareCandidateDecision(ctx, scope, current, permit, delta)
				if err != nil {
					return nil, err
				}
				return &ownedReconciliation{
					PreparedResourceTransaction: prepared,
					child:                       child,
				}, nil
			},
		},
	}
	return r.commands.SubmitPrepared(r.ctx, jobmgr.Request{
		UID:     fmt.Sprintf("owned-child-%d-%d", r.controller.generation, uid),
		LaneKey: child.id,
		Source:  lifecycle.SourceJobManager,
		Route:   "internal/jobs/owned-child",
	}, plan)
}

type ownedReconciliation struct {
	lifecycle.PreparedResourceTransaction
	child *ownedChildIntent
}

func (t *ownedReconciliation) Apply(ctx context.Context) (lifecycle.AppliedResourceTransaction, error) {
	applied, err := t.PreparedResourceTransaction.Apply(ctx)
	if err == nil && t.child.owner.registry.current(t.child.id) == t.child {
		t.child.applied.Store(t.child.token)
		if t.child.removed && t.child.owner.registry.controller.ownerOnly[t.child.config.Module()] {
			r := t.child.owner.registry
			r.mu.Lock()
			if r.reservations[t.child.id] == t.child {
				delete(r.reservations, t.child.id)
			}
			r.mu.Unlock()
			r.controller.candidates.mu.Lock()
			if record := r.controller.candidates.records[t.child.id]; record != nil && record.Owner == t.child {
				delete(r.controller.candidates.records, t.child.id)
			}
			r.controller.candidates.mu.Unlock()
		}
	}
	return applied, err
}

func (r *ownedJobRegistry) isClosed() bool { r.mu.Lock(); defer r.mu.Unlock(); return r.closed }

func (r *ownedJobRegistry) current(id string) *ownedChildIntent {
	r.mu.Lock()
	defer r.mu.Unlock()
	return r.reservations[id]
}
func (r *ownedJobRegistry) protected(id, module string) bool {
	r.mu.Lock()
	defer r.mu.Unlock()
	return r.reservations[id] != nil || r.controller.ownerOnly[module]
}

func (o *nativeJobOwner) Retire(ctx context.Context) error {
	if ctx == nil || ctx.Err() != nil {
		return errors.New("job output: invalid owner retirement context")
	}
	r := o.registry
	r.mu.Lock()
	if !o.currentLocked() {
		r.mu.Unlock()
		return ErrOwnedJobStale
	}
	snapshot := &ownedJobSnapshot{
		owner:       o,
		predecessor: o.revision,
		revision:    o.revision,
		retire:      true,
		children:    make(map[string]*ownedChildIntent),
	}
	r.mu.Unlock()
	return snapshot.Apply(ctx)
}

func (o *nativeJobOwner) Status() []ownedjobs.State {
	r := o.registry
	r.mu.Lock()
	result := make([]ownedjobs.State, 0, len(o.children))
	for _, child := range o.children {
		state := ownedjobs.State{
			Key:           child.key,
			JobID:         child.id,
			Revision:      o.revision,
			Desired:       child.token,
			Applied:       child.applied.Load(),
			Enabled:       child.enabled,
			RuntimeStatus: "pending",
			SourceType:    child.parentSource,
		}
		if record, ok := r.controller.graph.Lookup(child.id); ok && state.Applied == state.Desired {
			state.ConfigStatus = record.Status
			if record.Status == dyncfg.StatusDisabled.String() || record.Status == dyncfg.StatusFailed.String() {
				state.RuntimeStatus = "absent"
			}
		}
		child.mu.Lock()
		for generation := range child.generations {
			if generation.Generation > state.Generation {
				state.Generation = generation.Generation
				state.RuntimeStatus = generation.State().String()
			}
		}
		child.mu.Unlock()
		result = append(result, state)
	}
	r.mu.Unlock()
	slices.SortFunc(result, func(a, b ownedjobs.State) int {
		if a.Key < b.Key {
			return -1
		}
		if a.Key > b.Key {
			return 1
		}
		return 0
	})
	return result
}

func (r *ownedJobRegistry) retire() {
	r.mu.Lock()
	if !r.closed {
		r.closed = true
		for _, child := range r.reservations {
			child.revoke()
		}
		// No adoption can Add after closed is installed under this lock.
		go func() { r.dispatch.Wait(); close(r.dispatchDone) }()
	}
	r.mu.Unlock()
}

// ownedTransactionGuard rechecks authority at Apply, including transactions
// whose metadata/dependency preparation temporarily yielded the graph claim.
type ownedTransactionGuard struct {
	lifecycle.PreparedResourceTransaction
	registry *ownedJobRegistry
	expected *ownedChildIntent
	scope    lifecycle.ResourceTransactionScope
	result   lifecycle.SealedResult
}

func (t *ownedTransactionGuard) Apply(ctx context.Context) (lifecycle.AppliedResourceTransaction, error) {
	if t.registry.current(t.scope.ID) == t.expected {
		return t.PreparedResourceTransaction.Apply(ctx)
	}
	current, err := t.PreparedResourceTransaction.Dispose(ctx)
	if err != nil {
		return lifecycle.AppliedResourceTransaction{}, err
	}
	return lifecycle.NewAppliedResourceTransaction(
		t.scope,
		lifecycle.ResourceTransactionUnchanged,
		current,
		t.result,
		func() error { return nil },
	)
}

func (dcjc *DynCfgJobController) guardOwned(
	transaction lifecycle.PreparedResourceTransaction,
	expected *ownedChildIntent,
	result lifecycle.SealedResult,
) lifecycle.PreparedResourceTransaction {
	if transaction == nil {
		return nil
	}
	if !dcjc.owned.enabled.Load() || expected == nil && dcjc.owned.current(transaction.Scope().ID) == nil {
		return transaction
	}
	return &ownedTransactionGuard{
		PreparedResourceTransaction: transaction,
		registry:                    dcjc.owned,
		expected:                    expected,
		scope:                       transaction.Scope(),
		result:                      result,
	}
}

// WaitJobOwners joins only native transaction submission, never collector work.
// Physical work remains accounted for by the existing native lifecycle owners.
func (dcjc *DynCfgJobController) WaitJobOwners(ctx context.Context) error {
	if !dcjc.owned.enabled.Load() {
		return nil
	}
	select {
	case <-dcjc.owned.dispatchDone:
		return nil
	case <-ctx.Done():
		return ctx.Err()
	}
}
