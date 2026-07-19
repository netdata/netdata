// SPDX-License-Identifier: GPL-3.0-or-later

package lifecycle

import (
	"errors"
	"math/bits"
	"sort"
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
	class              LongLivedClass
	bytes              int64
	replacementOverlap bool
	providerKeys       []string
	e                  LongLivedExternalFacet
}

func NewPipelineLongLivedPlan(providerKeys []string) (LongLivedPlan, error) {
	if len(providerKeys) == 0 {
		return LongLivedPlan{},
			errors.New("jobmgr long-lived permit: invalid pipeline provider keys")
	}
	keys := append([]string(nil), providerKeys...)
	sort.Strings(keys)
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
		e:            LongLivedEProvider,
	}
	return plan, plan.Validate()
}

func NewJobLongLivedPlan(bytes int64) (LongLivedPlan, error) {
	plan := LongLivedPlan{class: LongLivedJob, e: LongLivedEJobResources}
	if err := plan.setResourceBytes(bytes); err != nil {
		return LongLivedPlan{}, err
	}
	return plan, plan.Validate()
}

func NewSecretStoreLongLivedPlan(bytes int64) (LongLivedPlan, error) {
	plan := LongLivedPlan{class: LongLivedSecretStore, e: LongLivedESecretStore}
	if err := plan.setResourceBytes(bytes); err != nil {
		return LongLivedPlan{}, err
	}
	return plan, plan.Validate()
}

func NewSecretStoreReplacementLongLivedPlan(bytes int64) (LongLivedPlan, error) {
	plan := LongLivedPlan{
		class: LongLivedSecretStore, replacementOverlap: true,
		e: LongLivedESecretStore,
	}
	if err := plan.setResourceBytes(bytes); err != nil {
		return LongLivedPlan{}, err
	}
	return plan, plan.Validate()
}

func (llp *LongLivedPlan) setResourceBytes(bytes int64) error {
	if llp == nil || bytes <= 0 || bytes >= OrdinaryBudgetBytes {
		return errors.New("jobmgr long-lived permit: invalid retained byte charge")
	}
	llp.bytes = bytes
	return nil
}

func (llp LongLivedPlan) Validate() error {
	if llp.bytes < 0 || llp.bytes >= OrdinaryBudgetBytes {
		return errors.New("jobmgr long-lived permit: invalid retained byte charge")
	}
	switch llp.class {
	case LongLivedPipeline:
		if llp.replacementOverlap ||
			!validPipelineProviderKeys(llp.providerKeys) ||
			llp.e != LongLivedEProvider ||
			llp.bytes != 0 {
			return errors.New("jobmgr long-lived permit: invalid pipeline facets")
		}
	case LongLivedJob:
		if llp.bytes <= 0 ||
			len(llp.providerKeys) != 0 ||
			llp.e != LongLivedEJobResources {
			return errors.New("jobmgr long-lived permit: invalid job facets")
		}
	case LongLivedSecretStore:
		if llp.bytes <= 0 ||
			len(llp.providerKeys) != 0 ||
			llp.e != LongLivedESecretStore {
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

func (llp LongLivedPlan) forReplacement() (LongLivedPlan, error) {
	switch llp.class {
	case LongLivedJob, LongLivedSecretStore:
		llp.replacementOverlap = true
	default:
		return LongLivedPlan{},
			errors.New("jobmgr long-lived permit: class has no replacement overlap")
	}
	return llp, llp.Validate()
}

func (llp LongLivedPlan) Class() LongLivedClass { return llp.class }
func (llp LongLivedPlan) Bytes() int64          { return llp.bytes }

type LongLivedCarrier interface {
	Valid() bool
	Owner() ResourceIdentity
	Class() LongLivedClass
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

func (llp LongLivedPermit) Valid() bool {
	if llp.supervisor == nil || !llp.ref.valid() || !llp.owner.Valid() {
		return false
	}
	switch llp.class {
	case LongLivedPipeline:
		return llp.bytes == 0
	case LongLivedJob, LongLivedSecretStore:
		return llp.bytes > 0
	default:
		return false
	}
}

func (llp LongLivedPermit) Owner() ResourceIdentity { return llp.owner }
func (llp LongLivedPermit) Class() LongLivedClass   { return llp.class }

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

func (llp LongLivedPermit) ReleaseBytes() error {
	if !llp.Valid() {
		return errors.New("jobmgr long-lived permit: invalid byte release")
	}
	return llp.supervisor.releaseLongLivedBytes(llp.ref, llp.owner)
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
	if err := llp.ReleaseBytes(); err != nil {
		return err
	}
	return llp.Return()
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
	gClaims        map[longLivedGKey]longLivedGState
	eReserved      LongLivedExternalFacet
	eActive        LongLivedExternalFacet
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
	llr.owners = make(map[ResourceIdentity]LongLivedPermitRef)
}

func (ts *TaskSupervisor) IssueLongLivedPermit(admission *AdmissionLedger, admissionRef AdmissionRef, owner ResourceIdentity, plan LongLivedPlan) (LongLivedPermit, error) {
	if ts == nil || admission == nil || !admissionRef.Valid() || !owner.Valid() {
		return LongLivedPermit{}, errors.New("jobmgr long-lived permit: invalid issue")
	}
	if err := plan.Validate(); err != nil {
		return LongLivedPermit{}, err
	}
	gClaims := longLivedGClaims(plan)
	if err := reserveLongLivedAdmission(
		admission,
		admissionRef,
		plan,
	); err != nil {
		return LongLivedPermit{}, err
	}

	registry := &ts.longLived
	registry.mu.Lock()
	if _, exists := registry.owners[owner]; registry.sealed || exists {
		registry.mu.Unlock()
		releaseErr := ts.releaseLongLivedAdmission(admission, admissionRef, plan.bytes)
		return LongLivedPermit{}, errors.Join(
			errors.New(
				"jobmgr long-lived permit: activation sealed or duplicate owner",
			),
			releaseErr,
		)
	}
	if longLivedClassFull(registry.classes[plan.class], plan) {
		registry.mu.Unlock()
		releaseErr := ts.releaseLongLivedAdmission(
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
			releaseErr := ts.releaseLongLivedAdmission(
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
			releaseErr := ts.releaseLongLivedAdmission(
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
		releaseErr := ts.releaseLongLivedAdmission(admission, admissionRef, plan.bytes)
		return LongLivedPermit{}, errors.Join(errors.New("jobmgr long-lived permit: generation wrapped"), releaseErr)
	}
	*slot = longLivedSlot{
		generation: generation, active: true, owner: owner, class: plan.class,
		bytes:   plan.bytes,
		gClaims: gClaims, eReserved: plan.e,
	}
	if plan.bytes > 0 {
		slot.admission = admission
		slot.admissionRef = admissionRef
	}
	ref := LongLivedPermitRef{Slot: uint32(index), Generation: generation}
	registry.owners[owner] = ref
	registry.classes[plan.class]++
	registry.census.Active++
	registry.incrementClassCensus(plan.class)
	registry.census.Bytes += plan.bytes
	registry.census.GReserved += len(gClaims)
	registry.census.ExternalReserved += bits.OnesCount8(uint8(plan.e))
	if plan.class == LongLivedSecretStore {
		registry.census.FinalizerOwnedActive++
		registry.census.FinalizerOwnedRecords++
		registry.census.FinalizerOwnedBytes += plan.bytes
	}
	registry.mu.Unlock()
	return LongLivedPermit{supervisor: ts, ref: ref, owner: owner, class: plan.class, bytes: plan.bytes}, nil
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
	if registry.sealed || slot.eReserved&facet == 0 || slot.eActive&facet != 0 {
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

func (ts *TaskSupervisor) releaseLongLivedBytes(ref LongLivedPermitRef, owner ResourceIdentity) error {
	registry := &ts.longLived
	registry.mu.Lock()
	slot, err := registry.slot(ref, owner)
	if err != nil {
		registry.mu.Unlock()
		return err
	}
	if slot.bytesReleasing {
		registry.mu.Unlock()
		return errors.New("jobmgr long-lived permit: bytes already released")
	}
	if longLivedGClaimsRetained(slot.gClaims) ||
		slot.eReserved != 0 || slot.eActive != 0 {
		registry.mu.Unlock()
		return errors.New("jobmgr long-lived permit: byte release before G/E facets")
	}
	if slot.bytes == 0 {
		if slot.class != LongLivedPipeline ||
			slot.admission != nil ||
			slot.admissionRef.Valid() {
			registry.mu.Unlock()
			return errors.New(
				"jobmgr long-lived permit: invalid charge-free ownership",
			)
		}
		registry.mu.Unlock()
		return nil
	}
	if slot.bytes < 0 ||
		slot.admission == nil ||
		!slot.admissionRef.Valid() {
		registry.mu.Unlock()
		return errors.New("jobmgr long-lived permit: invalid byte ownership")
	}
	slot.bytesReleasing = true
	admission, admissionRef, byteCount := slot.admission, slot.admissionRef, slot.bytes
	registry.mu.Unlock()

	releaseErr := ts.releaseLongLivedAdmission(admission, admissionRef, byteCount)

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

func (ts *TaskSupervisor) releaseLongLivedAdmission(admission *AdmissionLedger, ref AdmissionRef, bytes int64) error {
	if bytes == 0 {
		return nil
	}
	wake, err := admission.releaseLongLived(ref, bytes)
	if wake {
		ts.notifyAdmissionReady()
	}
	return err
}

func reserveLongLivedAdmission(
	admission *AdmissionLedger,
	ref AdmissionRef,
	plan LongLivedPlan,
) error {
	if plan.bytes == 0 {
		if plan.class != LongLivedPipeline {
			return errors.New(
				"jobmgr long-lived permit: invalid charge-free class",
			)
		}
		return admission.validateChargeFreeLongLived(ref)
	}
	return admission.transferLongLived(ref, plan.bytes)
}

func (ts *TaskSupervisor) returnLongLivedPermit(ref LongLivedPermitRef, owner ResourceIdentity) error {
	registry := &ts.longLived
	registry.mu.Lock()
	defer registry.mu.Unlock()
	slot, err := registry.slot(ref, owner)
	if err != nil {
		return err
	}
	if slot.bytes != 0 || slot.bytesReleasing ||
		longLivedGClaimsRetained(slot.gClaims) ||
		slot.eReserved != 0 || slot.eActive != 0 {
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
	if uint64(ref.Slot) >= uint64(len(llr.slots)) {
		return nil, errors.New("jobmgr long-lived permit: invalid reference")
	}
	slot := llr.slots[ref.Slot]
	if slot == nil {
		return nil, errors.New("jobmgr long-lived permit: invalid reference")
	}
	if !slot.active || slot.generation != ref.Generation || slot.owner != owner {
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
