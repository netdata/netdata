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

// InheritedTaskRef is a monotonic task identity. IDs are never reused during a
// supervisor lifetime.
type InheritedTaskRef uint32

func (ref InheritedTaskRef) valid() bool {
	return ref != 0
}

type InheritedTaskWork func(context.Context) error

type inheritedTaskRegistry struct {
	mu             sync.Mutex                              // guards all fields
	slots          map[InheritedTaskRef]*inheritedTaskSlot // active inherited tasks by monotonic ID
	owners         map[inheritedOwnerRole]InheritedTaskRef // active inherited-task ref by owner role
	nextSlot       uint32                                  // next monotonic inherited-task slot
	active         int                                     // count of active inherited tasks
	sealed         bool                                    // no further inherited tasks may start (shutdown)
	activeHead     InheritedTaskRef                        // oldest active task; shutdown drains from here
	activeTail     InheritedTaskRef                        // newest active task; append point for the active list
	shutdownCursor InheritedTaskRef                        // next active task the shutdown sweep will cancel
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
	activePrevious InheritedTaskRef        // previous task in the active list (shutdown cursor)
	activeNext     InheritedTaskRef        // next task in the active list (shutdown cursor)
}

type inheritedOwnerRole struct {
	owner ResourceIdentity
	role  InheritedTaskRole
	key   string
}

func (ts *TaskSupervisor) StartInherited(parent context.Context, owner ResourceIdentity, role InheritedTaskRole, work InheritedTaskWork) (InheritedTaskRef, error) {
	if role == InheritedPipelineSupervisor ||
		role == InheritedPipelineProvider {
		return 0,
			errors.New("jobmgr task supervisor: pipeline task requires a permit")
	}
	return ts.startInherited(
		parent, owner, role, "", work, 0,
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
		return 0,
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
		return 0, errors.New("jobmgr task supervisor: inherited permit differs")
	}
	if _, ok := longLivedGKeyForInherited(role, key); !ok {
		return 0, errors.New("jobmgr task supervisor: inherited role has no long-lived facet")
	}
	if err := ts.activateLongLivedG(
		permit.ref,
		owner,
		role,
		key,
	); err != nil {
		return 0, err
	}
	ref, err := ts.startInherited(
		parent, owner, role, key, work, permit.ref,
	)
	if err != nil {
		return 0, errors.Join(
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
		return 0, errors.New("jobmgr task supervisor: invalid inherited task")
	}
	registry := &ts.inherited
	registry.mu.Lock()
	if registry.sealed {
		registry.mu.Unlock()
		if ts.run != nil && ts.run.IsStopping() {
			return 0,
				ts.run.StoppingCause()
		}
		return 0, errors.New("jobmgr task supervisor: inherited activation is sealed")
	}
	ownerKey := inheritedOwnerRole{owner: owner, role: role, key: key}
	if _, ok := registry.owners[ownerKey]; ok {
		registry.mu.Unlock()
		return 0, errors.New("jobmgr task supervisor: duplicate inherited owner role")
	}
	ref, slot, err := registry.allocateSlot()
	if err != nil {
		registry.mu.Unlock()
		return 0, err
	}
	ctx, cancel := context.WithCancelCause(parent)
	done := make(chan struct{})
	*slot = inheritedTaskSlot{
		owner: owner, role: role,
		key:    key,
		cancel: cancel, done: done, permit: permit,
	}
	registry.active++
	registry.owners[ownerKey] = ref
	slot.activePrevious = registry.activeTail
	if registry.activeTail != 0 {
		registry.slots[registry.activeTail].activeNext = ref
	} else {
		registry.activeHead = ref
	}
	registry.activeTail = ref
	registry.mu.Unlock()

	go func() {
		err := runInheritedTask(ctx, work)
		ts.observeTaskPanic(err)
		ts.completeInherited(ref, err)
	}()
	return ref, nil
}

func (registry *inheritedTaskRegistry) allocateSlot() (
	InheritedTaskRef,
	*inheritedTaskSlot,
	error,
) {
	next := registry.nextSlot + 1
	if next == 0 {
		return 0, nil,
			errors.New("jobmgr task supervisor: inherited reference space exhausted")
	}
	registry.nextSlot = next
	ref := InheritedTaskRef(next)
	slot := &inheritedTaskSlot{}
	registry.slots[ref] = slot
	return ref, slot, nil
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
	if registry.shutdownCursor == ref {
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
	delete(registry.slots, ref)
	delete(registry.owners, key)
	registry.active--
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
) (bool, error) {
	if ts == nil || quantum <= 0 || quantum > InheritedCancellationServiceQuantum {
		return false,
			errors.New("jobmgr task supervisor: invalid inherited cancellation batch")
	}
	registry := &ts.inherited
	var cancels [InheritedCancellationServiceQuantum]context.CancelCauseFunc
	cancelCount := 0
	registry.mu.Lock()
	if !registry.sealed {
		registry.mu.Unlock()
		return false,
			errors.New("jobmgr task supervisor: inherited cancellation before seal")
	}
	for visited := 0; visited < quantum && registry.shutdownCursor != 0; visited++ {
		handle := registry.shutdownCursor
		slot := registry.slots[handle]
		registry.shutdownCursor = slot.activeNext
		if slot.cancelled {
			continue
		}
		slot.cancelled = true
		cancels[cancelCount] = slot.cancel
		cancelCount++
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
	return more, nil
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
	itr.slots = make(map[InheritedTaskRef]*inheritedTaskSlot)
	itr.owners = make(map[inheritedOwnerRole]InheritedTaskRef)
}

func (ts *TaskSupervisor) InheritedActive() int {
	if ts == nil {
		return 0
	}
	registry := &ts.inherited
	registry.mu.Lock()
	defer registry.mu.Unlock()
	return registry.active
}

func (itr *inheritedTaskRegistry) slot(ref InheritedTaskRef, owner ResourceIdentity) (*inheritedTaskSlot, error) {
	if !ref.valid() || !owner.Valid() {
		return nil, errors.New("jobmgr task supervisor: invalid inherited reference")
	}
	slot := itr.slots[ref]
	if slot == nil {
		return nil, errors.New("jobmgr task supervisor: stale or cross-owner inherited reference")
	}
	if slot.owner != owner {
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
	slot := itr.slots[ref]
	if slot == nil {
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
