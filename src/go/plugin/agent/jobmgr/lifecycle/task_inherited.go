// SPDX-License-Identifier: GPL-3.0-or-later

package lifecycle

import (
	"context"
	"errors"
	"fmt"
	"sync"
)

type InheritedTaskRole uint8

const (
	InheritedPipelineSupervisor InheritedTaskRole = iota + 1
	InheritedPipelineProvider
	InheritedV1Runtime
	InheritedV2Runner
)

func (role InheritedTaskRole) valid() bool {
	return role >= InheritedPipelineSupervisor && role <= InheritedV2Runner
}

type InheritedTaskRef struct {
	Slot       uint32
	Generation uint32
}

func (ref InheritedTaskRef) valid() bool {
	return ref.Generation != 0
}

type InheritedTaskWork func(context.Context) error

type inheritedTaskRegistry struct {
	mu             sync.Mutex
	slots          []*inheritedTaskSlot
	owners         map[inheritedOwnerRole]InheritedTaskRef
	freeHead       uint32
	census         InheritedTaskCensus
	sealed         bool
	activeHead     uint32
	activeTail     uint32
	shutdownCursor uint32
}

type inheritedTaskSlot struct {
	generation     uint32
	freeNext       uint32
	active         bool
	owner          ResourceIdentity
	role           InheritedTaskRole
	cancel         context.CancelFunc
	done           chan struct{}
	cancelled      bool
	finished       bool
	joined         bool
	releasing      bool
	permit         LongLivedPermitRef
	permitG        LongLivedGFacet
	err            error
	activePrevious uint32
	activeNext     uint32
}

type inheritedOwnerRole struct {
	owner ResourceIdentity
	role  InheritedTaskRole
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

func (supervisor *TaskSupervisor) StartInherited(parent context.Context, owner ResourceIdentity, role InheritedTaskRole, work InheritedTaskWork) (InheritedTaskRef, error) {
	return supervisor.startInherited(parent, owner, role, work, LongLivedPermitRef{}, 0)
}

func (supervisor *TaskSupervisor) StartInheritedWithPermit(parent context.Context, owner ResourceIdentity, role InheritedTaskRole, permit LongLivedPermit, work InheritedTaskWork) (InheritedTaskRef, error) {
	if supervisor == nil || permit.supervisor != supervisor || permit.owner != owner {
		return InheritedTaskRef{}, errors.New("jobmgr task supervisor: inherited permit differs")
	}
	facet, ok := gFacetForInheritedRole(role)
	if !ok {
		return InheritedTaskRef{}, errors.New("jobmgr task supervisor: inherited role has no long-lived facet")
	}
	if err := supervisor.activateLongLivedG(permit.ref, owner, facet); err != nil {
		return InheritedTaskRef{}, err
	}
	ref, err := supervisor.startInherited(parent, owner, role, work, permit.ref, facet)
	if err != nil {
		return InheritedTaskRef{}, errors.Join(err, supervisor.restoreLongLivedG(permit.ref, owner, facet))
	}
	return ref, nil
}

func (supervisor *TaskSupervisor) startInherited(parent context.Context, owner ResourceIdentity, role InheritedTaskRole, work InheritedTaskWork, permit LongLivedPermitRef, permitG LongLivedGFacet) (InheritedTaskRef, error) {
	if supervisor == nil || parent == nil || !owner.Valid() || !role.valid() || work == nil {
		return InheritedTaskRef{}, errors.New("jobmgr task supervisor: invalid inherited task")
	}
	registry := &supervisor.inherited
	registry.mu.Lock()
	if registry.sealed {
		registry.mu.Unlock()
		return InheritedTaskRef{}, errors.New("jobmgr task supervisor: inherited activation is sealed")
	}
	key := inheritedOwnerRole{owner: owner, role: role}
	if _, ok := registry.owners[key]; ok {
		registry.mu.Unlock()
		return InheritedTaskRef{}, errors.New("jobmgr task supervisor: duplicate inherited owner role")
	}
	var index uint32
	var slot *inheritedTaskSlot
	if registry.freeHead != 0 {
		handle := registry.freeHead
		index = handle - 1
		slot = registry.slots[index]
		registry.freeHead = slot.freeNext
	} else {
		if uint64(len(registry.slots)) > uint64(^uint32(0)) {
			registry.mu.Unlock()
			return InheritedTaskRef{}, errors.New("jobmgr task supervisor: inherited reference space exhausted")
		}
		index = uint32(len(registry.slots))
		slot = &inheritedTaskSlot{}
		registry.slots = append(registry.slots, slot)
	}
	generation := slot.generation + 1
	if generation == 0 {
		*slot = inheritedTaskSlot{generation: slot.generation, freeNext: registry.freeHead}
		registry.freeHead = index + 1
		registry.mu.Unlock()
		return InheritedTaskRef{}, errors.New("jobmgr task supervisor: inherited task generation wrapped")
	}
	ctx, cancel := context.WithCancel(parent)
	done := make(chan struct{})
	started := make(chan struct{})
	*slot = inheritedTaskSlot{
		generation: generation, active: true, owner: owner, role: role,
		cancel: cancel, done: done, permit: permit, permitG: permitG,
	}
	registry.census.Active++
	ref := InheritedTaskRef{Slot: index, Generation: generation}
	registry.owners[key] = ref
	handle := index + 1
	slot.activePrevious = registry.activeTail
	if registry.activeTail != 0 {
		registry.slots[registry.activeTail-1].activeNext = handle
	} else {
		registry.activeHead = handle
	}
	registry.activeTail = handle
	registry.mu.Unlock()

	go func() {
		close(started)
		err := runInheritedTask(ctx, work)
		registry.complete(ref, err)
	}()
	<-started
	return ref, nil
}

func (supervisor *TaskSupervisor) CancelInherited(ref InheritedTaskRef, owner ResourceIdentity) error {
	if supervisor == nil {
		return errors.New("jobmgr task supervisor: nil inherited registry")
	}
	registry := &supervisor.inherited
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
	cancel()
	return nil
}

func (supervisor *TaskSupervisor) JoinInherited(ctx context.Context, ref InheritedTaskRef, owner ResourceIdentity) (bool, error) {
	if supervisor == nil || ctx == nil {
		return false, errors.New("jobmgr task supervisor: invalid inherited join")
	}
	registry := &supervisor.inherited
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

func (supervisor *TaskSupervisor) ReleaseInherited(ref InheritedTaskRef, owner ResourceIdentity) error {
	if supervisor == nil {
		return errors.New("jobmgr task supervisor: nil inherited registry")
	}
	registry := &supervisor.inherited
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
	permit, permitG := slot.permit, slot.permitG
	if permit.valid() {
		slot.releasing = true
		registry.mu.Unlock()
		if err := supervisor.releaseLongLivedG(permit, owner, permitG); err != nil {
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
	key := inheritedOwnerRole{owner: slot.owner, role: slot.role}
	previous, next := slot.activePrevious, slot.activeNext
	handle := ref.Slot + 1
	if registry.shutdownCursor == handle {
		registry.shutdownCursor = next
	}
	if previous != 0 {
		registry.slots[previous-1].activeNext = next
	} else {
		registry.activeHead = next
	}
	if next != 0 {
		registry.slots[next-1].activePrevious = previous
	} else {
		registry.activeTail = previous
	}
	generation := slot.generation
	*slot = inheritedTaskSlot{generation: generation, freeNext: registry.freeHead}
	registry.freeHead = ref.Slot + 1
	delete(registry.owners, key)
	registry.census.Active--
	registry.census.Cancelled--
	registry.census.Finished--
	registry.census.Joined--
	registry.mu.Unlock()
	return nil
}

func (supervisor *TaskSupervisor) SealInherited() error {
	if supervisor == nil {
		return errors.New("jobmgr task supervisor: nil shutdown seal")
	}
	longLived := &supervisor.longLived
	registry := &supervisor.inherited
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

func (supervisor *TaskSupervisor) CancelInheritedBatch(
	quantum int,
) (ShutdownCancellationCensus, bool, error) {
	if supervisor == nil || quantum <= 0 || quantum > InheritedCancellationServiceQuantum {
		return ShutdownCancellationCensus{}, false,
			errors.New("jobmgr task supervisor: invalid inherited cancellation batch")
	}
	registry := &supervisor.inherited
	var cancels [InheritedCancellationServiceQuantum]context.CancelFunc
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
		slot := registry.slots[handle-1]
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
	for index := 0; index < cancelCount; index++ {
		cancels[index]()
	}
	return census, more, nil
}

func (supervisor *TaskSupervisor) InheritedCancellationPending() bool {
	if supervisor == nil {
		return false
	}
	registry := &supervisor.inherited
	registry.mu.Lock()
	defer registry.mu.Unlock()
	return registry.shutdownCursor != 0
}

func (registry *inheritedTaskRegistry) initialize() {
	registry.owners = make(map[inheritedOwnerRole]InheritedTaskRef)
}

func (supervisor *TaskSupervisor) InheritedActive() int {
	return supervisor.InheritedCensus().Active
}

func (supervisor *TaskSupervisor) InheritedCensus() InheritedTaskCensus {
	if supervisor == nil {
		return InheritedTaskCensus{}
	}
	registry := &supervisor.inherited
	registry.mu.Lock()
	defer registry.mu.Unlock()
	return registry.census
}

func (registry *inheritedTaskRegistry) slot(ref InheritedTaskRef, owner ResourceIdentity) (*inheritedTaskSlot, error) {
	if !ref.valid() || !owner.Valid() {
		return nil, errors.New("jobmgr task supervisor: invalid inherited reference")
	}
	if uint64(ref.Slot) >= uint64(len(registry.slots)) ||
		registry.slots[ref.Slot] == nil {
		return nil, errors.New("jobmgr task supervisor: stale or cross-owner inherited reference")
	}
	slot := registry.slots[ref.Slot]
	if !slot.active || slot.generation != ref.Generation || slot.owner != owner {
		return nil, errors.New("jobmgr task supervisor: stale or cross-owner inherited reference")
	}
	return slot, nil
}

func (registry *inheritedTaskRegistry) complete(ref InheritedTaskRef, taskErr error) {
	registry.mu.Lock()
	defer registry.mu.Unlock()
	if !ref.valid() {
		return
	}
	if uint64(ref.Slot) >= uint64(len(registry.slots)) ||
		registry.slots[ref.Slot] == nil {
		return
	}
	slot := registry.slots[ref.Slot]
	if !slot.active || slot.generation != ref.Generation || slot.finished {
		return
	}
	slot.err = taskErr
	slot.finished = true
	registry.census.Finished++
	close(slot.done)
}

func runInheritedTask(ctx context.Context, work InheritedTaskWork) (err error) {
	defer func() {
		if recovered := recover(); recovered != nil {
			err = fmt.Errorf("%w in inherited task: %v", ErrTaskPanic, recovered)
		}
	}()
	return work(ctx)
}
