// SPDX-License-Identifier: GPL-3.0-or-later

package lifecycle

import (
	"errors"
	"math/bits"
	"sync"
)

var ErrLongLivedRecordCapacity = errors.New("jobmgr long-lived permit: capacity exhausted")

type LongLivedClass uint8

const (
	LongLivedPipeline LongLivedClass = iota + 1
	LongLivedJob
	LongLivedSecretStore
)

type LongLivedGFacet uint8

const (
	LongLivedGPipelineSupervisor LongLivedGFacet = 1 << iota
	LongLivedGPipelineProvider
)

type LongLivedExternalFacet uint8

const (
	LongLivedEProvider LongLivedExternalFacet = 1 << iota
	LongLivedEJobResources
	LongLivedESecretStore
)

type LongLivedPlan struct {
	class              LongLivedClass
	bytes              int64
	replacementOverlap bool
	g                  LongLivedGFacet
	e                  LongLivedExternalFacet
}

func NewPipelineLongLivedPlan(bytes int64) (LongLivedPlan, error) {
	plan := LongLivedPlan{
		class: LongLivedPipeline,
		bytes: bytes,
		g:     LongLivedGPipelineSupervisor | LongLivedGPipelineProvider,
		e:     LongLivedEProvider,
	}
	return plan, plan.Validate()
}

func NewJobLongLivedPlan(bytes int64) (LongLivedPlan, error) {
	plan := LongLivedPlan{class: LongLivedJob, bytes: bytes, e: LongLivedEJobResources}
	return plan, plan.Validate()
}

func NewSecretStoreLongLivedPlan(bytes int64) (LongLivedPlan, error) {
	plan := LongLivedPlan{class: LongLivedSecretStore, bytes: bytes, e: LongLivedESecretStore}
	return plan, plan.Validate()
}

func NewSecretStoreReplacementLongLivedPlan(bytes int64) (LongLivedPlan, error) {
	plan := LongLivedPlan{class: LongLivedSecretStore, bytes: bytes, replacementOverlap: true, e: LongLivedESecretStore}
	return plan, plan.Validate()
}

func (plan LongLivedPlan) Validate() error {
	if plan.bytes <= 0 || plan.bytes >= OrdinaryBudgetBytes {
		return errors.New("jobmgr long-lived permit: invalid byte entitlement")
	}
	switch plan.class {
	case LongLivedPipeline:
		if plan.replacementOverlap || plan.g != LongLivedGPipelineSupervisor|LongLivedGPipelineProvider || plan.e != LongLivedEProvider {
			return errors.New("jobmgr long-lived permit: invalid pipeline facets")
		}
	case LongLivedJob:
		if plan.g != 0 || plan.e != LongLivedEJobResources {
			return errors.New("jobmgr long-lived permit: invalid job facets")
		}
	case LongLivedSecretStore:
		if plan.g != 0 || plan.e != LongLivedESecretStore {
			return errors.New("jobmgr long-lived permit: invalid SecretStore facets")
		}
	default:
		return errors.New("jobmgr long-lived permit: invalid class")
	}
	return nil
}

func (plan LongLivedPlan) forReplacement() (LongLivedPlan, error) {
	switch plan.class {
	case LongLivedJob, LongLivedSecretStore:
		plan.replacementOverlap = true
	default:
		return LongLivedPlan{},
			errors.New("jobmgr long-lived permit: class has no replacement overlap")
	}
	return plan, plan.Validate()
}

func (plan LongLivedPlan) Class() LongLivedClass { return plan.class }
func (plan LongLivedPlan) Bytes() int64          { return plan.bytes }

type LongLivedCarrier interface {
	Valid() bool
	Owner() ResourceIdentity
	Class() LongLivedClass
	CapacityBytes() int64
	ActivateExternal(LongLivedExternalFacet) error
	ReleaseExternal(LongLivedExternalFacet) error
	ReleaseBytes() error
	Return() error
}

type LongLivedPermitRef struct {
	Slot       uint32
	Generation uint32
}

func (ref LongLivedPermitRef) valid() bool {
	return ref.Generation != 0
}

type LongLivedPermit struct {
	supervisor *TaskSupervisor
	ref        LongLivedPermitRef
	owner      ResourceIdentity
	class      LongLivedClass
	bytes      int64
}

func (permit LongLivedPermit) Valid() bool {
	return permit.supervisor != nil && permit.ref.valid() && permit.owner.Valid() && permit.class != 0 && permit.bytes > 0
}

func (permit LongLivedPermit) Owner() ResourceIdentity { return permit.owner }
func (permit LongLivedPermit) Class() LongLivedClass   { return permit.class }
func (permit LongLivedPermit) CapacityBytes() int64    { return permit.bytes }

func (permit LongLivedPermit) ActivateExternal(facet LongLivedExternalFacet) error {
	if !permit.Valid() {
		return errors.New("jobmgr long-lived permit: invalid external activation")
	}
	return permit.supervisor.activateLongLivedExternal(permit.ref, permit.owner, facet)
}

func (permit LongLivedPermit) ReleaseExternal(facet LongLivedExternalFacet) error {
	if !permit.Valid() {
		return errors.New("jobmgr long-lived permit: invalid external release")
	}
	return permit.supervisor.releaseLongLivedExternal(permit.ref, permit.owner, facet)
}

func (permit LongLivedPermit) ReleaseBytes() error {
	if !permit.Valid() {
		return errors.New("jobmgr long-lived permit: invalid byte release")
	}
	return permit.supervisor.releaseLongLivedBytes(permit.ref, permit.owner)
}

func (permit LongLivedPermit) Return() error {
	if !permit.Valid() {
		return errors.New("jobmgr long-lived permit: invalid return")
	}
	return permit.supervisor.returnLongLivedPermit(permit.ref, permit.owner)
}

func (permit LongLivedPermit) AbortUnused() error {
	if !permit.Valid() {
		return errors.New("jobmgr long-lived permit: invalid unused abort")
	}
	if err := permit.supervisor.releaseReservedLongLivedFacets(permit.ref, permit.owner); err != nil {
		return err
	}
	if err := permit.ReleaseBytes(); err != nil {
		return err
	}
	return permit.Return()
}

type longLivedRegistry struct {
	mu       sync.Mutex
	slots    []*longLivedSlot
	owners   map[ResourceIdentity]LongLivedPermitRef
	classes  [LongLivedSecretStore + 1]int
	freeHead uint32
	census   LongLivedCensus
	sealed   bool
}

type longLivedSlot struct {
	generation     uint32
	freeNext       uint32
	active         bool
	owner          ResourceIdentity
	class          LongLivedClass
	admission      *AdmissionLedger
	admissionRef   AdmissionRef
	bytes          int64
	bytesReleasing bool
	gReserved      LongLivedGFacet
	gActive        LongLivedGFacet
	eReserved      LongLivedExternalFacet
	eActive        LongLivedExternalFacet
}

type LongLivedCensus struct {
	Active                int
	Pipelines             int
	Jobs                  int
	SecretStores          int
	Bytes                 int64
	GReserved             int
	GActive               int
	ExternalReserved      int
	ExternalActive        int
	FinalizerOwnedActive  int
	FinalizerOwnedRecords int
	FinalizerOwnedBytes   int64
}

func (registry *longLivedRegistry) initialize() {
	registry.owners = make(map[ResourceIdentity]LongLivedPermitRef)
}

func (supervisor *TaskSupervisor) IssueLongLivedPermit(admission *AdmissionLedger, admissionRef AdmissionRef, owner ResourceIdentity, plan LongLivedPlan) (LongLivedPermit, error) {
	if supervisor == nil || admission == nil || !admissionRef.Valid() || !owner.Valid() {
		return LongLivedPermit{}, errors.New("jobmgr long-lived permit: invalid issue")
	}
	if err := plan.Validate(); err != nil {
		return LongLivedPermit{}, err
	}
	if err := admission.transferLongLived(admissionRef, plan.bytes); err != nil {
		return LongLivedPermit{}, err
	}

	registry := &supervisor.longLived
	registry.mu.Lock()
	if _, exists := registry.owners[owner]; registry.sealed || exists {
		registry.mu.Unlock()
		releaseErr := supervisor.releaseLongLivedAdmission(admission, admissionRef, plan.bytes)
		return LongLivedPermit{}, errors.Join(
			errors.New(
				"jobmgr long-lived permit: activation sealed or duplicate owner",
			),
			releaseErr,
		)
	}
	if longLivedClassFull(registry.classes[plan.class], plan) {
		registry.mu.Unlock()
		releaseErr := supervisor.releaseLongLivedAdmission(
			admission,
			admissionRef,
			plan.bytes,
		)
		return LongLivedPermit{}, errors.Join(
			ErrLongLivedRecordCapacity,
			releaseErr,
		)
	}
	index := 0
	var slot *longLivedSlot
	if registry.freeHead == 0 {
		if uint64(len(registry.slots)) > uint64(^uint32(0)) {
			registry.mu.Unlock()
			releaseErr := supervisor.releaseLongLivedAdmission(
				admission,
				admissionRef,
				plan.bytes,
			)
			return LongLivedPermit{}, errors.Join(
				errors.New("jobmgr long-lived permit: reference space exhausted"),
				releaseErr,
			)
		}
		index = len(registry.slots)
		slot = &longLivedSlot{}
		registry.slots = append(registry.slots, slot)
	} else {
		index = int(registry.freeHead - 1)
		slot = registry.slots[index]
		if slot == nil {
			registry.mu.Unlock()
			releaseErr := supervisor.releaseLongLivedAdmission(
				admission,
				admissionRef,
				plan.bytes,
			)
			return LongLivedPermit{}, errors.Join(
				errors.New("jobmgr long-lived permit: invalid free slot"),
				releaseErr,
			)
		}
		registry.freeHead = slot.freeNext
	}
	generation := slot.generation + 1
	if generation == 0 {
		*slot = longLivedSlot{
			generation: slot.generation,
			freeNext:   registry.freeHead,
		}
		registry.freeHead = uint32(index + 1)
		registry.mu.Unlock()
		releaseErr := supervisor.releaseLongLivedAdmission(admission, admissionRef, plan.bytes)
		return LongLivedPermit{}, errors.Join(errors.New("jobmgr long-lived permit: generation wrapped"), releaseErr)
	}
	*slot = longLivedSlot{
		generation: generation, active: true, owner: owner, class: plan.class,
		admission: admission, admissionRef: admissionRef, bytes: plan.bytes,
		gReserved: plan.g, eReserved: plan.e,
	}
	ref := LongLivedPermitRef{Slot: uint32(index), Generation: generation}
	registry.owners[owner] = ref
	registry.classes[plan.class]++
	registry.census.Active++
	registry.incrementClassCensus(plan.class)
	registry.census.Bytes += plan.bytes
	registry.census.GReserved += bits.OnesCount8(uint8(plan.g))
	registry.census.ExternalReserved += bits.OnesCount8(uint8(plan.e))
	if plan.class == LongLivedSecretStore {
		registry.census.FinalizerOwnedActive++
		registry.census.FinalizerOwnedRecords++
		registry.census.FinalizerOwnedBytes += plan.bytes
	}
	registry.mu.Unlock()
	return LongLivedPermit{supervisor: supervisor, ref: ref, owner: owner, class: plan.class, bytes: plan.bytes}, nil
}

func (supervisor *TaskSupervisor) LongLivedCensus() LongLivedCensus {
	if supervisor == nil {
		return LongLivedCensus{}
	}
	registry := &supervisor.longLived
	registry.mu.Lock()
	defer registry.mu.Unlock()
	return registry.census
}

func (supervisor *TaskSupervisor) activateLongLivedExternal(ref LongLivedPermitRef, owner ResourceIdentity, facet LongLivedExternalFacet) error {
	if !singleExternalFacet(facet) {
		return errors.New("jobmgr long-lived permit: invalid external facet")
	}
	registry := &supervisor.longLived
	registry.mu.Lock()
	defer registry.mu.Unlock()
	slot, err := registry.slot(ref, owner)
	if err != nil {
		return err
	}
	if registry.sealed || slot.eReserved&facet == 0 || slot.eActive&facet != 0 {
		return errors.New("jobmgr long-lived permit: external facet is not reserved")
	}
	slot.eReserved &^= facet
	slot.eActive |= facet
	registry.census.ExternalReserved--
	registry.census.ExternalActive++
	return nil
}

func (supervisor *TaskSupervisor) releaseLongLivedExternal(ref LongLivedPermitRef, owner ResourceIdentity, facet LongLivedExternalFacet) error {
	if !singleExternalFacet(facet) {
		return errors.New("jobmgr long-lived permit: invalid external facet")
	}
	registry := &supervisor.longLived
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

func (supervisor *TaskSupervisor) releaseLongLivedBytes(ref LongLivedPermitRef, owner ResourceIdentity) error {
	registry := &supervisor.longLived
	registry.mu.Lock()
	slot, err := registry.slot(ref, owner)
	if err != nil {
		registry.mu.Unlock()
		return err
	}
	if slot.bytes <= 0 || slot.bytesReleasing {
		registry.mu.Unlock()
		return errors.New("jobmgr long-lived permit: bytes already released")
	}
	if slot.gReserved != 0 || slot.gActive != 0 || slot.eReserved != 0 || slot.eActive != 0 {
		registry.mu.Unlock()
		return errors.New("jobmgr long-lived permit: byte release before G/E facets")
	}
	slot.bytesReleasing = true
	admission, admissionRef, byteCount := slot.admission, slot.admissionRef, slot.bytes
	registry.mu.Unlock()

	releaseErr := supervisor.releaseLongLivedAdmission(admission, admissionRef, byteCount)

	registry.mu.Lock()
	slot, lookupErr := registry.slot(ref, owner)
	if lookupErr != nil {
		registry.mu.Unlock()
		return errors.Join(releaseErr, lookupErr)
	}
	if releaseErr != nil {
		slot.bytesReleasing = false
		registry.mu.Unlock()
		return releaseErr
	}
	slot.bytes = 0
	slot.bytesReleasing = false
	registry.census.Bytes -= byteCount
	if slot.class == LongLivedSecretStore {
		registry.census.FinalizerOwnedRecords--
		registry.census.FinalizerOwnedBytes -= byteCount
	}
	registry.mu.Unlock()
	return nil
}

func (supervisor *TaskSupervisor) releaseLongLivedAdmission(admission *AdmissionLedger, ref AdmissionRef, bytes int64) error {
	wake, err := admission.releaseLongLived(ref, bytes)
	if wake {
		supervisor.notifyAdmissionReady()
	}
	return err
}

func (supervisor *TaskSupervisor) returnLongLivedPermit(ref LongLivedPermitRef, owner ResourceIdentity) error {
	registry := &supervisor.longLived
	registry.mu.Lock()
	defer registry.mu.Unlock()
	slot, err := registry.slot(ref, owner)
	if err != nil {
		return err
	}
	if slot.bytes != 0 || slot.bytesReleasing || slot.gReserved != 0 || slot.gActive != 0 || slot.eReserved != 0 || slot.eActive != 0 {
		return errors.New("jobmgr long-lived permit: return with retained B/G/E facets")
	}
	delete(registry.owners, slot.owner)
	generation := slot.generation
	class := slot.class
	*slot = longLivedSlot{generation: generation, freeNext: registry.freeHead}
	registry.freeHead = ref.Slot + 1
	registry.classes[class]--
	registry.census.Active--
	registry.decrementClassCensus(class)
	if class == LongLivedSecretStore {
		registry.census.FinalizerOwnedActive--
	}
	return nil
}

func longLivedClassFull(count int, plan LongLivedPlan) bool {
	if plan.replacementOverlap {
		return false
	}
	switch plan.class {
	case LongLivedPipeline:
		return count >= 1
	case LongLivedJob:
		return false
	case LongLivedSecretStore:
		return count >= 1
	default:
		return true
	}
}

func (registry *longLivedRegistry) incrementClassCensus(class LongLivedClass) {
	switch class {
	case LongLivedPipeline:
		registry.census.Pipelines++
	case LongLivedJob:
		registry.census.Jobs++
	case LongLivedSecretStore:
		registry.census.SecretStores++
	}
}

func (registry *longLivedRegistry) decrementClassCensus(class LongLivedClass) {
	switch class {
	case LongLivedPipeline:
		registry.census.Pipelines--
	case LongLivedJob:
		registry.census.Jobs--
	case LongLivedSecretStore:
		registry.census.SecretStores--
	}
}

func (supervisor *TaskSupervisor) releaseReservedLongLivedFacets(ref LongLivedPermitRef, owner ResourceIdentity) error {
	registry := &supervisor.longLived
	registry.mu.Lock()
	defer registry.mu.Unlock()
	slot, err := registry.slot(ref, owner)
	if err != nil {
		return err
	}
	if slot.gActive != 0 || slot.eActive != 0 {
		return errors.New("jobmgr long-lived permit: unused abort with active facets")
	}
	registry.census.GReserved -= bits.OnesCount8(uint8(slot.gReserved))
	registry.census.ExternalReserved -= bits.OnesCount8(uint8(slot.eReserved))
	slot.gReserved = 0
	slot.eReserved = 0
	return nil
}

func (registry *longLivedRegistry) slot(ref LongLivedPermitRef, owner ResourceIdentity) (*longLivedSlot, error) {
	if !ref.valid() || !owner.Valid() {
		return nil, errors.New("jobmgr long-lived permit: invalid reference")
	}
	if uint64(ref.Slot) >= uint64(len(registry.slots)) {
		return nil, errors.New("jobmgr long-lived permit: invalid reference")
	}
	slot := registry.slots[ref.Slot]
	if slot == nil {
		return nil, errors.New("jobmgr long-lived permit: invalid reference")
	}
	if !slot.active || slot.generation != ref.Generation || slot.owner != owner {
		return nil, errors.New("jobmgr long-lived permit: stale or cross-owner reference")
	}
	return slot, nil
}

func singleGFacet(facet LongLivedGFacet) bool {
	return facet != 0 && facet&(facet-1) == 0
}

func singleExternalFacet(facet LongLivedExternalFacet) bool {
	return facet != 0 && facet&(facet-1) == 0
}

func gFacetForInheritedRole(role InheritedTaskRole) (LongLivedGFacet, bool) {
	switch role {
	case InheritedPipelineSupervisor:
		return LongLivedGPipelineSupervisor, true
	case InheritedPipelineProvider:
		return LongLivedGPipelineProvider, true
	default:
		return 0, false
	}
}

func (supervisor *TaskSupervisor) activateLongLivedG(ref LongLivedPermitRef, owner ResourceIdentity, facet LongLivedGFacet) error {
	if !singleGFacet(facet) {
		return errors.New("jobmgr long-lived permit: invalid G facet")
	}
	registry := &supervisor.longLived
	registry.mu.Lock()
	defer registry.mu.Unlock()
	slot, err := registry.slot(ref, owner)
	if err != nil {
		return err
	}
	if registry.sealed || slot.gReserved&facet == 0 || slot.gActive&facet != 0 {
		return errors.New("jobmgr long-lived permit: G facet is not reserved")
	}
	slot.gReserved &^= facet
	slot.gActive |= facet
	registry.census.GReserved--
	registry.census.GActive++
	return nil
}

func (supervisor *TaskSupervisor) restoreLongLivedG(ref LongLivedPermitRef, owner ResourceIdentity, facet LongLivedGFacet) error {
	registry := &supervisor.longLived
	registry.mu.Lock()
	defer registry.mu.Unlock()
	slot, err := registry.slot(ref, owner)
	if err != nil {
		return err
	}
	if slot.gActive&facet == 0 || slot.gReserved&facet != 0 {
		return errors.New("jobmgr long-lived permit: G facet cannot be restored")
	}
	slot.gActive &^= facet
	slot.gReserved |= facet
	registry.census.GActive--
	registry.census.GReserved++
	return nil
}

func (supervisor *TaskSupervisor) releaseLongLivedG(ref LongLivedPermitRef, owner ResourceIdentity, facet LongLivedGFacet) error {
	registry := &supervisor.longLived
	registry.mu.Lock()
	defer registry.mu.Unlock()
	slot, err := registry.slot(ref, owner)
	if err != nil {
		return err
	}
	if slot.gActive&facet == 0 {
		return errors.New("jobmgr long-lived permit: G facet is not active")
	}
	slot.gActive &^= facet
	registry.census.GActive--
	return nil
}
