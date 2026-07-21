// SPDX-License-Identifier: GPL-3.0-or-later

package lifecycle

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"sync"
)

type InheritedTaskRole uint8

const (
	InheritedPipelineSupervisor InheritedTaskRole = iota + 1
	InheritedPipelineProvider
	InheritedV1Runtime
	InheritedV2Runner
)

func (itr InheritedTaskRole) valid() bool {
	return itr >= InheritedPipelineSupervisor && itr <= InheritedV2Runner
}

type InheritedTaskRef struct {
	Slot       uint32
	Generation uint32
}

func (ref InheritedTaskRef) valid() bool {
	return ref.Slot != 0 && ref.Generation != 0
}

type InheritedTaskWork func(context.Context) error

type inheritedTaskRegistry struct {
	mu             sync.Mutex
	slots          map[uint32]*inheritedTaskSlot
	owners         map[inheritedOwnerRole]InheritedTaskRef
	nextSlot       uint32
	census         InheritedTaskCensus
	sealed         bool
	activeHead     uint32
	activeTail     uint32
	shutdownCursor uint32
}

type inheritedTaskSlot struct {
	owner          ResourceIdentity        // owning resource identity
	role           InheritedTaskRole       // inherited task role (V1 runtime / V2 runner / pipeline)
	key            string                  // identity tuple key into the owners map
	cancel         context.CancelCauseFunc // child context canceller
	done           chan struct{}           // closed when the child goroutine exits
	cancelled      bool                    // cancellation requested
	finished       bool                    // child completed
	joined         bool                    // child goroutine joined
	releasing      bool                    // release is in flight
	permit         LongLivedPermitRef      // long-lived permit backing this task, if any
	err            error                   // captured child error
	activePrevious uint32                  // previous slot in the active list (shutdown cursor)
	activeNext     uint32                  // next slot in the active list (shutdown cursor)
}

type inheritedOwnerRole struct {
	owner ResourceIdentity
	role  InheritedTaskRole
	key   string
}

type InheritedTaskCensus struct {
	Active    int
	Cancelled int
	Finished  int
	Joined    int
}

type ShutdownCancellationCensus struct {
	Visited          int
	Signalled        int
	AlreadyCancelled int
}

func (ts *TaskSupervisor) StartInherited(parent context.Context, owner ResourceIdentity, role InheritedTaskRole, work InheritedTaskWork) (InheritedTaskRef, error) {
	if role == InheritedPipelineSupervisor ||
		role == InheritedPipelineProvider {
		return InheritedTaskRef{},
			errors.New("jobmgr task supervisor: pipeline task requires a permit")
	}
	return ts.startInherited(
		parent, owner, role, "", work, LongLivedPermitRef{},
	)
}

func (ts *TaskSupervisor) StartInheritedWithPermit(parent context.Context, owner ResourceIdentity, role InheritedTaskRole, permit LongLivedPermit, work InheritedTaskWork) (InheritedTaskRef, error) {
	return ts.startInheritedWithPermit(
		parent, owner, role, "", permit, work,
	)
}

// StartInheritedWithPermitKey starts one independently owned child for a
// stable key within a shared owner and role.
func (ts *TaskSupervisor) StartInheritedWithPermitKey(
	parent context.Context,
	owner ResourceIdentity,
	role InheritedTaskRole,
	key string,
	permit LongLivedPermit,
	work InheritedTaskWork,
) (InheritedTaskRef, error) {
	if key == "" || key != strings.TrimSpace(key) {
		return InheritedTaskRef{},
			errors.New("jobmgr task supervisor: invalid inherited task key")
	}
	return ts.startInheritedWithPermit(
		parent, owner, role, key, permit, work,
	)
}

func (ts *TaskSupervisor) startInheritedWithPermit(
	parent context.Context,
	owner ResourceIdentity,
	role InheritedTaskRole,
	key string,
	permit LongLivedPermit,
	work InheritedTaskWork,
) (InheritedTaskRef, error) {
	if ts == nil || permit.supervisor != ts || permit.owner != owner {
		return InheritedTaskRef{}, errors.New("jobmgr task supervisor: inherited permit differs")
	}
	if _, ok := longLivedGKeyForInherited(role, key); !ok {
		return InheritedTaskRef{}, errors.New("jobmgr task supervisor: inherited role has no long-lived facet")
	}
	if err := ts.activateLongLivedG(
		permit.ref,
		owner,
		role,
		key,
	); err != nil {
		return InheritedTaskRef{}, err
	}
	ref, err := ts.startInherited(
		parent, owner, role, key, work, permit.ref,
	)
	if err != nil {
		return InheritedTaskRef{}, errors.Join(
			err,
			ts.restoreLongLivedG(
				permit.ref,
				owner,
				role,
				key,
			),
		)
	}
	return ref, nil
}

func (ts *TaskSupervisor) startInherited(parent context.Context, owner ResourceIdentity, role InheritedTaskRole, key string, work InheritedTaskWork, permit LongLivedPermitRef) (InheritedTaskRef, error) {
	if ts == nil || parent == nil || !owner.Valid() || !role.valid() || work == nil {
		return InheritedTaskRef{}, errors.New("jobmgr task supervisor: invalid inherited task")
	}
	registry := &ts.inherited
	registry.mu.Lock()
	if registry.sealed {
		registry.mu.Unlock()
		if ts.run != nil && ts.run.IsStopping() {
			return InheritedTaskRef{},
				ts.run.StoppingCause()
		}
		return InheritedTaskRef{}, errors.New("jobmgr task supervisor: inherited activation is sealed")
	}
	ownerKey := inheritedOwnerRole{owner: owner, role: role, key: key}
	if _, ok := registry.owners[ownerKey]; ok {
		registry.mu.Unlock()
		return InheritedTaskRef{}, errors.New("jobmgr task supervisor: duplicate inherited owner role")
	}
	slotID, slot, err := registry.allocateSlot()
	if err != nil {
		registry.mu.Unlock()
		return InheritedTaskRef{}, err
	}
	ctx, cancel := context.WithCancelCause(parent)
	done := make(chan struct{})
	started := make(chan struct{})
	*slot = inheritedTaskSlot{
		owner: owner, role: role,
		key:    key,
		cancel: cancel, done: done, permit: permit,
	}
	registry.census.Active++
	ref := InheritedTaskRef{Slot: slotID, Generation: 1}
	registry.owners[ownerKey] = ref
	slot.activePrevious = registry.activeTail
	if registry.activeTail != 0 {
		registry.slots[registry.activeTail].activeNext = slotID
	} else {
		registry.activeHead = slotID
	}
	registry.activeTail = slotID
	registry.mu.Unlock()

	go func() {
		close(started)
		err := runInheritedTask(ctx, work)
		ts.observeTaskPanic(err)
		ts.completeInherited(ref, err)
	}()
	<-started
	return ref, nil
}

func (registry *inheritedTaskRegistry) allocateSlot() (
	uint32,
	*inheritedTaskSlot,
	error,
) {
	next := registry.nextSlot + 1
	if next == 0 {
		return 0, nil,
			errors.New("jobmgr task supervisor: inherited reference space exhausted")
	}
	registry.nextSlot = next
	slot := &inheritedTaskSlot{}
	registry.slots[next] = slot
	return next, slot, nil
}

func (ts *TaskSupervisor) CancelInherited(ref InheritedTaskRef, owner ResourceIdentity) error {
	if ts == nil {
		return errors.New("jobmgr task supervisor: nil inherited registry")
	}
	registry := &ts.inherited
	registry.mu.Lock()
	slot, err := registry.slot(ref, owner)
	if err != nil {
		registry.mu.Unlock()
		return err
	}
	if slot.joined {
		registry.mu.Unlock()
		return errors.New("jobmgr task supervisor: inherited task already canceled or joined")
	}
	if slot.cancelled {
		registry.mu.Unlock()
		return nil
	}
	slot.cancelled = true
	registry.census.Cancelled++
	cancel := slot.cancel
	registry.mu.Unlock()
	cancel(context.Canceled)
	return nil
}

func (ts *TaskSupervisor) JoinInherited(ctx context.Context, ref InheritedTaskRef, owner ResourceIdentity) (bool, error) {
	if ts == nil || ctx == nil {
		return false, errors.New("jobmgr task supervisor: invalid inherited join")
	}
	registry := &ts.inherited
	registry.mu.Lock()
	slot, err := registry.slot(ref, owner)
	if err != nil {
		registry.mu.Unlock()
		return false, err
	}
	if !slot.cancelled || slot.joined {
		registry.mu.Unlock()
		return false, errors.New("jobmgr task supervisor: inherited join before cancel or after join")
	}
	done := slot.done
	registry.mu.Unlock()

	select {
	case <-done:
	case <-ctx.Done():
		return false, ctx.Err()
	}

	registry.mu.Lock()
	slot, err = registry.slot(ref, owner)
	if err != nil {
		registry.mu.Unlock()
		return false, err
	}
	if !slot.finished || slot.joined {
		registry.mu.Unlock()
		return false, errors.New("jobmgr task supervisor: inherited completion differs")
	}
	slot.joined = true
	registry.census.Joined++
	childErr := slot.err
	registry.mu.Unlock()
	return true, childErr
}

func (ts *TaskSupervisor) ReleaseInherited(ref InheritedTaskRef, owner ResourceIdentity) error {
	if ts == nil {
		return errors.New("jobmgr task supervisor: nil inherited registry")
	}
	registry := &ts.inherited
	registry.mu.Lock()
	slot, err := registry.slot(ref, owner)
	if err != nil {
		registry.mu.Unlock()
		return err
	}
	if !slot.cancelled || !slot.finished || !slot.joined || slot.releasing {
		registry.mu.Unlock()
		return errors.New("jobmgr task supervisor: inherited release before cancel and join")
	}
	permit := slot.permit
	if permit.valid() {
		role, taskKey := slot.role, slot.key
		slot.releasing = true
		registry.mu.Unlock()
		if err := ts.releaseLongLivedG(
			permit,
			owner,
			role,
			taskKey,
		); err != nil {
			registry.mu.Lock()
			if current, lookupErr := registry.slot(ref, owner); lookupErr == nil {
				current.releasing = false
			}
			registry.mu.Unlock()
			return err
		}
		registry.mu.Lock()
		slot, err = registry.slot(ref, owner)
		if err != nil || !slot.releasing {
			registry.mu.Unlock()
			return errors.Join(err, errors.New("jobmgr task supervisor: inherited release changed"))
		}
	}
	key := inheritedOwnerRole{
		owner: slot.owner,
		role:  slot.role,
		key:   slot.key,
	}
	previous, next := slot.activePrevious, slot.activeNext
	if registry.shutdownCursor == ref.Slot {
		registry.shutdownCursor = next
	}
	if previous != 0 {
		registry.slots[previous].activeNext = next
	} else {
		registry.activeHead = next
	}
	if next != 0 {
		registry.slots[next].activePrevious = previous
	} else {
		registry.activeTail = previous
	}
	delete(registry.slots, ref.Slot)
	delete(registry.owners, key)
	registry.census.Active--
	registry.census.Cancelled--
	registry.census.Finished--
	registry.census.Joined--
	registry.mu.Unlock()
	return nil
}

func (ts *TaskSupervisor) SealInherited() error {
	if ts == nil {
		return errors.New("jobmgr task supervisor: nil shutdown seal")
	}
	longLived := &ts.longLived
	registry := &ts.inherited
	longLived.mu.Lock()
	registry.mu.Lock()
	if longLived.sealed || registry.sealed {
		registry.mu.Unlock()
		longLived.mu.Unlock()
		return errors.New("jobmgr task supervisor: shutdown sealed twice")
	}
	longLived.sealed = true
	registry.sealed = true
	registry.shutdownCursor = registry.activeHead
	registry.mu.Unlock()
	longLived.mu.Unlock()
	return nil
}

func (ts *TaskSupervisor) CancelInheritedBatch(
	quantum int,
) (ShutdownCancellationCensus, bool, error) {
	if ts == nil || quantum <= 0 || quantum > InheritedCancellationServiceQuantum {
		return ShutdownCancellationCensus{}, false,
			errors.New("jobmgr task supervisor: invalid inherited cancellation batch")
	}
	registry := &ts.inherited
	var cancels [InheritedCancellationServiceQuantum]context.CancelCauseFunc
	cancelCount := 0
	var census ShutdownCancellationCensus
	registry.mu.Lock()
	if !registry.sealed {
		registry.mu.Unlock()
		return ShutdownCancellationCensus{}, false,
			errors.New("jobmgr task supervisor: inherited cancellation before seal")
	}
	for census.Visited < quantum && registry.shutdownCursor != 0 {
		handle := registry.shutdownCursor
		slot := registry.slots[handle]
		registry.shutdownCursor = slot.activeNext
		census.Visited++
		if slot.cancelled {
			census.AlreadyCancelled++
			continue
		}
		slot.cancelled = true
		registry.census.Cancelled++
		cancels[cancelCount] = slot.cancel
		cancelCount++
		census.Signalled++
	}
	more := registry.shutdownCursor != 0
	registry.mu.Unlock()
	cause := error(context.Canceled)
	if ts.run != nil && ts.run.IsStopping() {
		cause = ts.run.StoppingCause()
	}
	for index := 0; index < cancelCount; index++ {
		cancels[index](cause)
	}
	return census, more, nil
}

func (ts *TaskSupervisor) InheritedCancellationPending() bool {
	if ts == nil {
		return false
	}
	registry := &ts.inherited
	registry.mu.Lock()
	defer registry.mu.Unlock()
	return registry.shutdownCursor != 0
}

func (itr *inheritedTaskRegistry) initialize() {
	itr.slots = make(map[uint32]*inheritedTaskSlot)
	itr.owners = make(map[inheritedOwnerRole]InheritedTaskRef)
}

func (ts *TaskSupervisor) InheritedActive() int {
	return ts.InheritedCensus().Active
}

func (ts *TaskSupervisor) InheritedCensus() InheritedTaskCensus {
	if ts == nil {
		return InheritedTaskCensus{}
	}
	registry := &ts.inherited
	registry.mu.Lock()
	defer registry.mu.Unlock()
	return registry.census
}

func (itr *inheritedTaskRegistry) slot(ref InheritedTaskRef, owner ResourceIdentity) (*inheritedTaskSlot, error) {
	if !ref.valid() || !owner.Valid() {
		return nil, errors.New("jobmgr task supervisor: invalid inherited reference")
	}
	slot := itr.slots[ref.Slot]
	if slot == nil {
		return nil, errors.New("jobmgr task supervisor: stale or cross-owner inherited reference")
	}
	if ref.Generation != 1 || slot.owner != owner {
		return nil, errors.New("jobmgr task supervisor: stale or cross-owner inherited reference")
	}
	return slot, nil
}

func (ts *TaskSupervisor) completeInherited(
	ref InheritedTaskRef,
	taskErr error,
) {
	if ts == nil {
		return
	}
	cleanStop := false
	normalized := taskErr
	if ts.run != nil &&
		ts.run.IsStopping() &&
		onlyCurrentStoppingRejections(
			taskErr,
			ts.run.Generation(),
		) {
		cleanStop = true
		normalized = nil
	}
	spontaneousFailure, normalized :=
		ts.inherited.complete(ref, normalized, cleanStop)
	if !spontaneousFailure || cleanStop {
		return
	}
	if ts.run != nil {
		ts.run.Dirty(normalized)
	}
	if ts.wake != nil {
		ts.wake()
	}
}

func (itr *inheritedTaskRegistry) complete(
	ref InheritedTaskRef,
	taskErr error,
	cleanStop bool,
) (bool, error) {
	itr.mu.Lock()
	defer itr.mu.Unlock()
	if !ref.valid() {
		return false, taskErr
	}
	slot := itr.slots[ref.Slot]
	if slot == nil || ref.Generation != 1 {
		return false, taskErr
	}
	if slot.finished {
		return false, taskErr
	}
	spontaneousFailure := !slot.cancelled &&
		(taskErr != nil ||
			slot.role != InheritedPipelineProvider)
	if spontaneousFailure && taskErr == nil && !cleanStop {
		taskErr = errors.New(
			"jobmgr task supervisor: inherited task exited before cancellation",
		)
	}
	slot.err = taskErr
	slot.finished = true
	itr.census.Finished++
	close(slot.done)
	return spontaneousFailure, taskErr
}

func onlyCurrentStoppingRejections(
	err error,
	generation uint64,
) bool {
	if generation == 0 {
		return false
	}
	return allErrorLeavesMatch(err, func(leaf error) bool {
		stopping, ok := leaf.(*StoppingRejection)
		return ok && stopping.Generation == generation
	})
}

func runInheritedTask(ctx context.Context, work InheritedTaskWork) (err error) {
	defer func() {
		if recovered := recover(); recovered != nil {
			err = fmt.Errorf("%w in inherited task: %v", ErrTaskPanic, recovered)
		}
	}()
	return work(ctx)
}
