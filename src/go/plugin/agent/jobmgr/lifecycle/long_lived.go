// SPDX-License-Identifier: GPL-3.0-or-later

package lifecycle

import (
	"errors"
	"math/bits"
	"slices"
	"strings"
	"sync"
)

var ErrLongLivedRecordCapacity = errors.New("jobmgr long-lived permit: capacity exhausted")

type LongLivedClass uint8

const (
	LongLivedPipeline LongLivedClass = iota + 1
	LongLivedJob
	LongLivedSecretStore
)

type LongLivedExternalFacet uint8

const (
	LongLivedEProvider LongLivedExternalFacet = 1 << iota
	LongLivedEJobResources
	LongLivedESecretStore
)

type LongLivedPlan struct {
	providerKeys []string               // secret-provider keys this plan pins (pipeline class)
	class        LongLivedClass         // permit class: pipeline, job, or secret store
	external     LongLivedExternalFacet // external ('E') facets the plan reserves
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
		external:     LongLivedEProvider,
	}
	return plan, plan.Validate()
}

func NewJobLongLivedPlan() LongLivedPlan {
	return LongLivedPlan{
		class: LongLivedJob, external: LongLivedEJobResources,
	}
}

func NewSecretStoreLongLivedPlan() LongLivedPlan {
	return LongLivedPlan{
		class: LongLivedSecretStore, external: LongLivedESecretStore,
	}
}

func (llp LongLivedPlan) Validate() error {
	switch llp.class {
	case LongLivedPipeline:
		if !validPipelineProviderKeys(llp.providerKeys) ||
			llp.external != LongLivedEProvider {
			return errors.New("jobmgr long-lived permit: invalid pipeline facets")
		}
	case LongLivedJob:
		if len(llp.providerKeys) != 0 ||
			llp.external != LongLivedEJobResources {
			return errors.New("jobmgr long-lived permit: invalid job facets")
		}
	case LongLivedSecretStore:
		if len(llp.providerKeys) != 0 ||
			llp.external != LongLivedESecretStore {
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

type LongLivedPermitRef struct {
	Slot       uint32
	Generation uint32
}

func (ref LongLivedPermitRef) valid() bool {
	return ref.Slot != 0 && ref.Generation != 0
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

func (llp LongLivedPermit) ActivateExternal(facet LongLivedExternalFacet) error {
	if !llp.Valid() {
		return errors.New("jobmgr long-lived permit: invalid external activation")
	}
	return llp.supervisor.activateLongLivedExternal(llp.ref, llp.owner, facet)
}

func (llp LongLivedPermit) ReleaseExternal(facet LongLivedExternalFacet) error {
	if !llp.Valid() {
		return errors.New("jobmgr long-lived permit: invalid external release")
	}
	return llp.supervisor.releaseLongLivedExternal(llp.ref, llp.owner, facet)
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
	slots    map[uint32]*longLivedSlot               // active permits by monotonic slot
	owners   map[ResourceIdentity]LongLivedPermitRef // active permit ref by owning resource identity
	classes  [LongLivedSecretStore + 1]int           // per-class active permit counts
	nextSlot uint32                                  // next monotonic permit slot
	census   LongLivedCensus                         // cached census projection
	sealed   bool                                    // no further permits may be issued (shutdown)
}

type longLivedSlot struct {
	owner     ResourceIdentity                  // owning resource identity
	class     LongLivedClass                    // permit class
	gClaims   map[longLivedGKey]longLivedGState // per-inherited-task facet states (one 'G' facet per inherited task)
	eReserved LongLivedExternalFacet            // reserved external ('E') facets
	eActive   LongLivedExternalFacet            // activated external ('E') facets
}

type longLivedGKey struct {
	role InheritedTaskRole
	key  string
}

type longLivedGState uint8

const (
	longLivedGReserved longLivedGState = iota + 1
	longLivedGActive
	longLivedGReleased
)

type LongLivedCensus struct {
	Active           int // total active long-lived permits
	Pipelines        int // active pipeline-class permits
	Jobs             int // active job-class permits
	SecretStores     int // active secret-store-class permits
	GReserved        int // reserved inherited-task ('G') facets across all permits
	GActive          int // activated inherited-task ('G') facets
	ExternalReserved int // reserved external ('E') facets
	ExternalActive   int // activated external ('E') facets
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
	for _, state := range claims {
		if state != longLivedGReleased {
			return true
		}
	}
	return false
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
	llr.slots = make(map[uint32]*longLivedSlot)
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
	if longLivedClassFull(registry.classes[plan.class], plan) {
		registry.mu.Unlock()
		return LongLivedPermit{}, ErrLongLivedRecordCapacity
	}
	slotID, slot, allocationErr := registry.allocateSlot()
	if allocationErr != nil {
		registry.mu.Unlock()
		return LongLivedPermit{}, allocationErr
	}
	*slot = longLivedSlot{
		owner: owner, class: plan.class,
		gClaims: gClaims, eReserved: plan.external,
	}
	ref := LongLivedPermitRef{Slot: slotID, Generation: 1}
	registry.owners[owner] = ref
	registry.classes[plan.class]++
	registry.census.Active++
	registry.incrementClassCensus(plan.class)
	registry.census.GReserved += len(gClaims)
	registry.census.ExternalReserved += bits.OnesCount8(uint8(plan.external))
	registry.mu.Unlock()
	return LongLivedPermit{supervisor: ts, ref: ref, owner: owner, class: plan.class}, nil
}

func (registry *longLivedRegistry) allocateSlot() (
	uint32,
	*longLivedSlot,
	error,
) {
	next := registry.nextSlot + 1
	if next == 0 {
		return 0, nil,
			errors.New("jobmgr long-lived permit: reference space exhausted")
	}
	registry.nextSlot = next
	slot := &longLivedSlot{}
	registry.slots[next] = slot
	return next, slot, nil
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

func (ts *TaskSupervisor) activateLongLivedExternal(ref LongLivedPermitRef, owner ResourceIdentity, facet LongLivedExternalFacet) error {
	if !singleExternalFacet(facet) {
		return errors.New("jobmgr long-lived permit: invalid external facet")
	}
	registry := &ts.longLived
	registry.mu.Lock()
	defer registry.mu.Unlock()
	slot, err := registry.slot(ref, owner)
	if err != nil {
		return err
	}
	if slot.eReserved&facet == 0 || slot.eActive&facet != 0 {
		return errors.New("jobmgr long-lived permit: external facet is not reserved")
	}
	slot.eReserved &^= facet
	slot.eActive |= facet
	registry.census.ExternalReserved--
	registry.census.ExternalActive++
	return nil
}

func (ts *TaskSupervisor) releaseLongLivedExternal(ref LongLivedPermitRef, owner ResourceIdentity, facet LongLivedExternalFacet) error {
	if !singleExternalFacet(facet) {
		return errors.New("jobmgr long-lived permit: invalid external facet")
	}
	registry := &ts.longLived
	registry.mu.Lock()
	defer registry.mu.Unlock()
	slot, err := registry.slot(ref, owner)
	if err != nil {
		return err
	}
	if slot.eActive&facet != 0 {
		slot.eActive &^= facet
		registry.census.ExternalActive--
		return nil
	}
	if slot.eReserved&facet != 0 {
		slot.eReserved &^= facet
		registry.census.ExternalReserved--
		return nil
	}
	return errors.New("jobmgr long-lived permit: external facet already released or absent")
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
		slot.eReserved != 0 || slot.eActive != 0 {
		return errors.New("jobmgr long-lived permit: return with retained inherited-task/external facets")
	}
	delete(registry.owners, slot.owner)
	class := slot.class
	delete(registry.slots, ref.Slot)
	registry.classes[class]--
	registry.census.Active--
	registry.decrementClassCensus(class)
	return nil
}

func longLivedClassFull(count int, plan LongLivedPlan) bool {
	switch plan.class {
	case LongLivedPipeline:
		return count >= 1
	case LongLivedJob:
		return false
	case LongLivedSecretStore:
		return false
	default:
		return true
	}
}

func (llr *longLivedRegistry) incrementClassCensus(class LongLivedClass) {
	switch class {
	case LongLivedPipeline:
		llr.census.Pipelines++
	case LongLivedJob:
		llr.census.Jobs++
	case LongLivedSecretStore:
		llr.census.SecretStores++
	}
}

func (llr *longLivedRegistry) decrementClassCensus(class LongLivedClass) {
	switch class {
	case LongLivedPipeline:
		llr.census.Pipelines--
	case LongLivedJob:
		llr.census.Jobs--
	case LongLivedSecretStore:
		llr.census.SecretStores--
	}
}

func (ts *TaskSupervisor) releaseReservedLongLivedFacets(ref LongLivedPermitRef, owner ResourceIdentity) error {
	registry := &ts.longLived
	registry.mu.Lock()
	defer registry.mu.Unlock()
	slot, err := registry.slot(ref, owner)
	if err != nil {
		return err
	}
	if longLivedGClaimsActive(slot.gClaims) || slot.eActive != 0 {
		return errors.New("jobmgr long-lived permit: unused abort with active facets")
	}
	released := 0
	for key, state := range slot.gClaims {
		if state == longLivedGReserved {
			slot.gClaims[key] = longLivedGReleased
			released++
		}
	}
	registry.census.GReserved -= released
	registry.census.ExternalReserved -= bits.OnesCount8(uint8(slot.eReserved))
	slot.eReserved = 0
	return nil
}

func (llr *longLivedRegistry) slot(ref LongLivedPermitRef, owner ResourceIdentity) (*longLivedSlot, error) {
	if !ref.valid() || !owner.Valid() {
		return nil, errors.New("jobmgr long-lived permit: invalid reference")
	}
	slot := llr.slots[ref.Slot]
	if slot == nil {
		return nil, errors.New("jobmgr long-lived permit: invalid reference")
	}
	if ref.Generation != 1 || slot.owner != owner {
		return nil, errors.New("jobmgr long-lived permit: stale or cross-owner reference")
	}
	return slot, nil
}

func singleExternalFacet(facet LongLivedExternalFacet) bool {
	return facet != 0 && facet&(facet-1) == 0
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
	registry.census.GReserved--
	registry.census.GActive++
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
	slot.gClaims[claim] = longLivedGReleased
	registry.census.GReserved--
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
	registry.census.GActive--
	registry.census.GReserved++
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
	slot.gClaims[claim] = longLivedGReleased
	registry.census.GActive--
	return nil
}
