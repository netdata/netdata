// SPDX-License-Identifier: GPL-3.0-or-later

package lifecycle

import (
	"errors"
	"slices"
	"strings"
	"sync"
)

type LongLivedClass uint8

const (
	LongLivedPipeline LongLivedClass = iota + 1
	LongLivedJob
	LongLivedSecretStore
)

type LongLivedPlan struct {
	providerKeys []string       // secret-provider keys this plan pins (pipeline class)
	class        LongLivedClass // permit class: pipeline, job, or secret store
}

func NewPipelineLongLivedPlan(providerKeys []string) (LongLivedPlan, error) {
	if len(providerKeys) == 0 {
		return LongLivedPlan{},
			errors.New("jobmgr long-lived permit: invalid pipeline provider keys")
	}
	keys := slices.Clone(providerKeys)
	slices.Sort(keys)
	for index, key := range keys {
		if key == "" || key != strings.TrimSpace(key) ||
			index > 0 && key == keys[index-1] {
			return LongLivedPlan{},
				errors.New("jobmgr long-lived permit: invalid pipeline provider key")
		}
	}
	plan := LongLivedPlan{
		class:        LongLivedPipeline,
		providerKeys: keys,
	}
	return plan, plan.Validate()
}

func NewJobLongLivedPlan() LongLivedPlan {
	return LongLivedPlan{class: LongLivedJob}
}

func NewSecretStoreLongLivedPlan() LongLivedPlan {
	return LongLivedPlan{class: LongLivedSecretStore}
}

func (llp LongLivedPlan) Validate() error {
	switch llp.class {
	case LongLivedPipeline:
		if !validPipelineProviderKeys(llp.providerKeys) {
			return errors.New("jobmgr long-lived permit: invalid pipeline facets")
		}
	case LongLivedJob:
		if len(llp.providerKeys) != 0 {
			return errors.New("jobmgr long-lived permit: invalid job facets")
		}
	case LongLivedSecretStore:
		if len(llp.providerKeys) != 0 {
			return errors.New("jobmgr long-lived permit: invalid SecretStore facets")
		}
	default:
		return errors.New("jobmgr long-lived permit: invalid class")
	}
	return nil
}

func validPipelineProviderKeys(keys []string) bool {
	if len(keys) == 0 {
		return false
	}
	for index, key := range keys {
		if key == "" || key != strings.TrimSpace(key) ||
			index > 0 && key <= keys[index-1] {
			return false
		}
	}
	return true
}

func (llp LongLivedPlan) validateReplacementClass() error {
	switch llp.class {
	case LongLivedJob, LongLivedSecretStore:
		return nil
	default:
		return errors.New("jobmgr long-lived permit: class cannot replace a resource")
	}
}

func (llp LongLivedPlan) Class() LongLivedClass { return llp.class }

type LongLivedPermitRef uint32

func (ref LongLivedPermitRef) valid() bool {
	return ref != 0
}

type LongLivedPermit struct {
	supervisor *TaskSupervisor    // issuing task supervisor
	ref        LongLivedPermitRef // slot reference in the long-lived registry
	owner      ResourceIdentity   // resource identity that owns the permit
	class      LongLivedClass     // permit class (job / secret-store / pipeline)
}

func (llp LongLivedPermit) Valid() bool {
	if llp.supervisor == nil || !llp.ref.valid() || !llp.owner.Valid() {
		return false
	}
	return llp.class >= LongLivedPipeline && llp.class <= LongLivedSecretStore
}

func (llp LongLivedPermit) Owner() ResourceIdentity { return llp.owner }
func (llp LongLivedPermit) Class() LongLivedClass   { return llp.class }

// ValidateLive verifies cold-path ownership against the permit registry.
func (llp LongLivedPermit) ValidateLive() error {
	if !llp.Valid() {
		return errors.New("jobmgr long-lived permit: invalid live validation")
	}
	registry := &llp.supervisor.longLived
	registry.mu.Lock()
	defer registry.mu.Unlock()
	_, err := registry.slot(llp.ref, llp.owner)
	return err
}

func (llp LongLivedPermit) ActivateExternal() error {
	if !llp.Valid() {
		return errors.New("jobmgr long-lived permit: invalid external activation")
	}
	return llp.supervisor.activateLongLivedExternal(llp.ref, llp.owner)
}

func (llp LongLivedPermit) ReleaseExternal() error {
	if !llp.Valid() {
		return errors.New("jobmgr long-lived permit: invalid external release")
	}
	return llp.supervisor.releaseLongLivedExternal(llp.ref, llp.owner)
}

func (llp LongLivedPermit) ReleaseUnusedInherited(
	role InheritedTaskRole,
	key string,
) error {
	if !llp.Valid() {
		return errors.New("jobmgr long-lived permit: invalid unused G release")
	}
	return llp.supervisor.releaseUnusedLongLivedG(
		llp.ref,
		llp.owner,
		role,
		key,
	)
}

func (llp LongLivedPermit) Return() error {
	if !llp.Valid() {
		return errors.New("jobmgr long-lived permit: invalid return")
	}
	return llp.supervisor.returnLongLivedPermit(llp.ref, llp.owner)
}

func (llp LongLivedPermit) AbortUnused() error {
	if !llp.Valid() {
		return errors.New("jobmgr long-lived permit: invalid unused abort")
	}
	if err := llp.supervisor.releaseReservedLongLivedFacets(llp.ref, llp.owner); err != nil {
		return err
	}
	return llp.Return()
}

type longLivedRegistry struct {
	mu       sync.Mutex                              // guards all fields
	slots    map[LongLivedPermitRef]*longLivedSlot   // active permits by monotonic ID
	owners   map[ResourceIdentity]LongLivedPermitRef // active permit ref by owning resource identity
	nextSlot uint32                                  // next monotonic permit slot
	census   LongLivedCensus                         // cached census projection
	sealed   bool                                    // no further permits may be issued (shutdown)
}

type longLivedSlot struct {
	owner    ResourceIdentity                  // owning resource identity
	class    LongLivedClass                    // permit class
	gClaims  map[longLivedGKey]longLivedGState // per-inherited-task facet states (one 'G' facet per inherited task)
	external longLivedExternalState            // class-derived external resource state
}

type longLivedGKey struct {
	role InheritedTaskRole
	key  string
}

type longLivedGState uint8

const (
	longLivedGReserved longLivedGState = iota + 1
	longLivedGActive
)

type longLivedExternalState uint8

const (
	longLivedExternalReserved longLivedExternalState = iota + 1
	longLivedExternalActive
	longLivedExternalReleased
)

type LongLivedCensus struct {
	Active       int // total active long-lived permits
	SecretStores int // active secret-store-class permits
}

func longLivedGClaims(plan LongLivedPlan) map[longLivedGKey]longLivedGState {
	if plan.class != LongLivedPipeline {
		return nil
	}
	claims := make(
		map[longLivedGKey]longLivedGState,
		len(plan.providerKeys)+1,
	)
	claims[longLivedGKey{
		role: InheritedPipelineSupervisor,
	}] = longLivedGReserved
	for _, key := range plan.providerKeys {
		claims[longLivedGKey{
			role: InheritedPipelineProvider,
			key:  key,
		}] = longLivedGReserved
	}
	return claims
}

func longLivedGClaimsRetained(
	claims map[longLivedGKey]longLivedGState,
) bool {
	return len(claims) != 0
}

func longLivedGClaimsActive(
	claims map[longLivedGKey]longLivedGState,
) bool {
	for _, state := range claims {
		if state == longLivedGActive {
			return true
		}
	}
	return false
}

func (llr *longLivedRegistry) initialize() {
	llr.slots = make(map[LongLivedPermitRef]*longLivedSlot)
	llr.owners = make(map[ResourceIdentity]LongLivedPermitRef)
}

func (ts *TaskSupervisor) IssueLongLivedPermit(owner ResourceIdentity, plan LongLivedPlan) (LongLivedPermit, error) {
	if ts == nil || !owner.Valid() {
		return LongLivedPermit{}, errors.New("jobmgr long-lived permit: invalid issue")
	}
	if err := plan.Validate(); err != nil {
		return LongLivedPermit{}, err
	}
	gClaims := longLivedGClaims(plan)

	registry := &ts.longLived
	registry.mu.Lock()
	if _, exists := registry.owners[owner]; registry.sealed || exists {
		registry.mu.Unlock()
		return LongLivedPermit{}, errors.New(
			"jobmgr long-lived permit: activation sealed or duplicate owner",
		)
	}
	ref, slot, allocationErr := registry.allocateSlot()
	if allocationErr != nil {
		registry.mu.Unlock()
		return LongLivedPermit{}, allocationErr
	}
	*slot = longLivedSlot{
		owner: owner, class: plan.class,
		gClaims: gClaims, external: longLivedExternalReserved,
	}
	registry.owners[owner] = ref
	registry.census.Active++
	if plan.class == LongLivedSecretStore {
		registry.census.SecretStores++
	}
	registry.mu.Unlock()
	return LongLivedPermit{supervisor: ts, ref: ref, owner: owner, class: plan.class}, nil
}

func (registry *longLivedRegistry) allocateSlot() (
	LongLivedPermitRef,
	*longLivedSlot,
	error,
) {
	next := registry.nextSlot + 1
	if next == 0 {
		return 0, nil,
			errors.New("jobmgr long-lived permit: reference space exhausted")
	}
	registry.nextSlot = next
	ref := LongLivedPermitRef(next)
	slot := &longLivedSlot{}
	registry.slots[ref] = slot
	return ref, slot, nil
}

func (ts *TaskSupervisor) LongLivedCensus() LongLivedCensus {
	if ts == nil {
		return LongLivedCensus{}
	}
	registry := &ts.longLived
	registry.mu.Lock()
	defer registry.mu.Unlock()
	return registry.census
}

func (ts *TaskSupervisor) activateLongLivedExternal(ref LongLivedPermitRef, owner ResourceIdentity) error {
	registry := &ts.longLived
	registry.mu.Lock()
	defer registry.mu.Unlock()
	slot, err := registry.slot(ref, owner)
	if err != nil {
		return err
	}
	if slot.external != longLivedExternalReserved {
		return errors.New("jobmgr long-lived permit: external resource is not reserved")
	}
	slot.external = longLivedExternalActive
	return nil
}

func (ts *TaskSupervisor) releaseLongLivedExternal(ref LongLivedPermitRef, owner ResourceIdentity) error {
	registry := &ts.longLived
	registry.mu.Lock()
	defer registry.mu.Unlock()
	slot, err := registry.slot(ref, owner)
	if err != nil {
		return err
	}
	if slot.external == longLivedExternalReserved ||
		slot.external == longLivedExternalActive {
		slot.external = longLivedExternalReleased
		return nil
	}
	return errors.New("jobmgr long-lived permit: external resource already released")
}

func (ts *TaskSupervisor) returnLongLivedPermit(ref LongLivedPermitRef, owner ResourceIdentity) error {
	registry := &ts.longLived
	registry.mu.Lock()
	defer registry.mu.Unlock()
	slot, err := registry.slot(ref, owner)
	if err != nil {
		return err
	}
	if longLivedGClaimsRetained(slot.gClaims) ||
		slot.external != longLivedExternalReleased {
		return errors.New("jobmgr long-lived permit: return with retained inherited-task/external facets")
	}
	delete(registry.owners, slot.owner)
	delete(registry.slots, ref)
	registry.census.Active--
	if slot.class == LongLivedSecretStore {
		registry.census.SecretStores--
	}
	return nil
}

func (ts *TaskSupervisor) releaseReservedLongLivedFacets(ref LongLivedPermitRef, owner ResourceIdentity) error {
	registry := &ts.longLived
	registry.mu.Lock()
	defer registry.mu.Unlock()
	slot, err := registry.slot(ref, owner)
	if err != nil {
		return err
	}
	if longLivedGClaimsActive(slot.gClaims) || slot.external == longLivedExternalActive {
		return errors.New("jobmgr long-lived permit: unused abort with active facets")
	}
	clear(slot.gClaims)
	slot.external = longLivedExternalReleased
	return nil
}

func (llr *longLivedRegistry) slot(ref LongLivedPermitRef, owner ResourceIdentity) (*longLivedSlot, error) {
	if !ref.valid() || !owner.Valid() {
		return nil, errors.New("jobmgr long-lived permit: invalid reference")
	}
	slot := llr.slots[ref]
	if slot == nil {
		return nil, errors.New("jobmgr long-lived permit: invalid reference")
	}
	if slot.owner != owner {
		return nil, errors.New("jobmgr long-lived permit: stale or cross-owner reference")
	}
	return slot, nil
}

func longLivedGKeyForInherited(
	role InheritedTaskRole,
	key string,
) (longLivedGKey, bool) {
	switch role {
	case InheritedPipelineSupervisor:
		if key != "" {
			return longLivedGKey{}, false
		}
	case InheritedPipelineProvider:
		if key == "" || key != strings.TrimSpace(key) {
			return longLivedGKey{}, false
		}
	default:
		return longLivedGKey{}, false
	}
	return longLivedGKey{role: role, key: key}, true
}

func (ts *TaskSupervisor) activateLongLivedG(
	ref LongLivedPermitRef,
	owner ResourceIdentity,
	role InheritedTaskRole,
	key string,
) error {
	claim, ok := longLivedGKeyForInherited(role, key)
	if !ok {
		return errors.New("jobmgr long-lived permit: invalid G facet")
	}
	registry := &ts.longLived
	registry.mu.Lock()
	defer registry.mu.Unlock()
	slot, err := registry.slot(ref, owner)
	if err != nil {
		return err
	}
	if registry.sealed || slot.gClaims[claim] != longLivedGReserved {
		return errors.New("jobmgr long-lived permit: G facet is not reserved")
	}
	slot.gClaims[claim] = longLivedGActive
	return nil
}

func (ts *TaskSupervisor) releaseUnusedLongLivedG(
	ref LongLivedPermitRef,
	owner ResourceIdentity,
	role InheritedTaskRole,
	key string,
) error {
	claim, ok := longLivedGKeyForInherited(role, key)
	if !ok {
		return errors.New("jobmgr long-lived permit: invalid G facet")
	}
	registry := &ts.longLived
	registry.mu.Lock()
	defer registry.mu.Unlock()
	slot, err := registry.slot(ref, owner)
	if err != nil {
		return err
	}
	if slot.gClaims[claim] != longLivedGReserved {
		return errors.New("jobmgr long-lived permit: G facet is not unused")
	}
	delete(slot.gClaims, claim)
	return nil
}

func (ts *TaskSupervisor) restoreLongLivedG(
	ref LongLivedPermitRef,
	owner ResourceIdentity,
	role InheritedTaskRole,
	key string,
) error {
	claim, ok := longLivedGKeyForInherited(role, key)
	if !ok {
		return errors.New("jobmgr long-lived permit: invalid G facet")
	}
	registry := &ts.longLived
	registry.mu.Lock()
	defer registry.mu.Unlock()
	slot, err := registry.slot(ref, owner)
	if err != nil {
		return err
	}
	if slot.gClaims[claim] != longLivedGActive {
		return errors.New("jobmgr long-lived permit: G facet cannot be restored")
	}
	slot.gClaims[claim] = longLivedGReserved
	return nil
}

func (ts *TaskSupervisor) releaseLongLivedG(
	ref LongLivedPermitRef,
	owner ResourceIdentity,
	role InheritedTaskRole,
	key string,
) error {
	claim, ok := longLivedGKeyForInherited(role, key)
	if !ok {
		return errors.New("jobmgr long-lived permit: invalid G facet")
	}
	registry := &ts.longLived
	registry.mu.Lock()
	defer registry.mu.Unlock()
	slot, err := registry.slot(ref, owner)
	if err != nil {
		return err
	}
	if slot.gClaims[claim] != longLivedGActive {
		return errors.New("jobmgr long-lived permit: G facet is not active")
	}
	delete(slot.gClaims, claim)
	return nil
}
