// SPDX-License-Identifier: GPL-3.0-or-later

package diagnostic

import (
	"encoding/json"
	"errors"
	"fmt"
	"slices"
	"sync"
	"sync/atomic"
)

var (
	ErrRecorderClosed     = errors.New("diagnostic recorder is closed")
	ErrRecorderSaturated  = errors.New("diagnostic recorder is saturated")
	ErrTransactionClosed  = errors.New("diagnostic transaction is already closed")
	ErrCaptureAborted     = errors.New("diagnostic capture aborted")
	errCaptureMemberLimit = errors.New("capture member limit exceeded")
)

const (
	CapabilityCaptureAccounting = "capture_accounting"
	KindCaptureGap              = "capture_gap"
)

type HandleState string

const (
	HandlePending HandleState = "pending"
	HandleSealed  HandleState = "sealed"
	HandleFailed  HandleState = "failed"
)

type memberFuture struct {
	mu        sync.RWMutex
	state     HandleState
	ref       ContentRef
	inventory []ContentRef
	err       error
}

// MemberHandle is a process-local reference to asynchronously sealed content.
// It deliberately cannot cross the JSON wire boundary.
type MemberHandle struct {
	id     uint64
	future *memberFuture
}

func (h MemberHandle) ID() uint64 { return h.id }

func (h MemberHandle) Resolve() (ContentRef, error, HandleState) {
	if h.id == 0 || h.future == nil {
		return ContentRef{}, errors.New("invalid member handle"), HandleFailed
	}
	h.future.mu.RLock()
	defer h.future.mu.RUnlock()
	return h.future.ref, h.future.err, h.future.state
}

func (h MemberHandle) MarshalJSON() ([]byte, error) {
	return nil, errors.New("process-local diagnostic member handles are not serializable")
}

func (h MemberHandle) String() string { return fmt.Sprintf("member-handle:%d", h.id) }

func (f *memberFuture) resolve(ref ContentRef, inventory []ContentRef, err error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	if f.state != HandlePending {
		return
	}
	if err != nil {
		f.state = HandleFailed
		f.err = err
		return
	}
	f.state = HandleSealed
	f.ref = ref
	f.inventory = slices.Clone(inventory)
}

func (h MemberHandle) resolveInventory() (ContentRef, []ContentRef, error, HandleState) {
	if h.id == 0 || h.future == nil {
		return ContentRef{}, nil, errors.New("invalid member handle"), HandleFailed
	}
	h.future.mu.RLock()
	defer h.future.mu.RUnlock()
	return h.future.ref, slices.Clone(h.future.inventory), h.future.err, h.future.state
}

type RecorderConfig struct {
	QueueCapacity    uint64
	MaxMembers       uint64
	MaxRetainedBytes uint64
	Sink             CaptureSink
}

func (c RecorderConfig) Validate() error {
	if c.QueueCapacity == 0 {
		return errors.New("queue capacity must be nonzero")
	}
	if c.MaxMembers < 2 {
		return errors.New("max members must allow a capability root and one payload member")
	}
	if c.MaxRetainedBytes == 0 {
		return errors.New("max retained bytes must be nonzero")
	}
	if c.Sink == nil {
		return errors.New("capture sink is required")
	}
	if c.QueueCapacity > uint64(^uint(0)>>1) {
		return errors.New("queue capacity exceeds platform size")
	}
	return nil
}

type CaptureSink interface {
	Store(CaptureResult)
}

type CaptureResult struct {
	CaptureID     uint64
	Registration  uint64
	Manifest      ManifestV1
	Members       MemorySource
	RetainedBytes uint64
	Err           error
}

type CaptureGapV1 struct {
	FirstAttempt uint64 `json:"first_attempt"`
	LastAttempt  uint64 `json:"last_attempt"`
	Count        uint64 `json:"count"`
	Reason       string `json:"reason"`
}

func (g CaptureGapV1) Validate() error {
	if g.FirstAttempt == 0 || g.LastAttempt < g.FirstAttempt {
		return errors.New("invalid capture gap attempt range")
	}
	if g.Count == 0 {
		return errors.New("capture gap count must be nonzero")
	}
	if err := validateID("capture gap reason", g.Reason); err != nil {
		return err
	}
	return nil
}

type Recorder struct {
	config RecorderConfig

	mu        sync.Mutex
	closed    bool
	active    sync.WaitGroup
	admission chan struct{}
	jobs      chan captureJob
	worker    sync.WaitGroup

	attemptSequence atomic.Uint64
	handleSequence  atomic.Uint64
	gapMu           sync.Mutex
	rejectedCount   uint64
	rejectedFirst   uint64
	rejectedLast    uint64
}

func NewRecorder(config RecorderConfig) (*Recorder, error) {
	if err := config.Validate(); err != nil {
		return nil, err
	}
	capacity := int(config.QueueCapacity)
	r := &Recorder{
		config:    config,
		admission: make(chan struct{}, capacity),
		jobs:      make(chan captureJob, capacity),
	}
	r.worker.Add(1)
	go r.run()
	return r, nil
}

// Begin is non-blocking. Successful admission reserves one terminal queue
// position, so Commit and Abort never wait for worker progress.
func (r *Recorder) Begin(registration uint64) (*CaptureTransaction, error) {
	if r == nil {
		return nil, ErrRecorderClosed
	}
	attempt := r.attemptSequence.Add(1)
	if registration == 0 {
		return nil, errors.New("registration must be nonzero")
	}

	r.mu.Lock()
	defer r.mu.Unlock()
	if r.closed {
		return nil, ErrRecorderClosed
	}
	select {
	case r.admission <- struct{}{}:
		r.active.Add(1)
		return &CaptureTransaction{
			recorder:     r,
			captureID:    attempt,
			registration: registration,
			sections:     make(map[string]*captureSection),
		}, nil
	default:
		r.recordRejected(attempt)
		return nil, ErrRecorderSaturated
	}
}

func (r *Recorder) recordRejected(attempt uint64) {
	r.gapMu.Lock()
	defer r.gapMu.Unlock()
	r.rejectedCount++
	if r.rejectedCount == 1 {
		r.rejectedFirst = attempt
	}
	r.rejectedLast = attempt
}

func (r *Recorder) Close() {
	if r == nil {
		return
	}
	r.mu.Lock()
	if r.closed {
		r.mu.Unlock()
		return
	}
	r.closed = true
	r.mu.Unlock()

	r.active.Wait()
	close(r.jobs)
	r.worker.Wait()
}

type CaptureTransaction struct {
	recorder     *Recorder
	captureID    uint64
	registration uint64

	mu            sync.Mutex
	closed        bool
	members       []pendingMember
	sections      map[string]*captureSection
	retainedBytes uint64
	terminalErr   error
}

type captureSection struct {
	name            string
	state           TerminalState
	expectedRecords uint64
	members         []int
}

type pendingMember struct {
	memberType    MemberType
	value         any
	dependencies  []MemberHandle
	build         DerivedMemberBuilder
	reference     MemberHandle
	handle        MemberHandle
	retainedBytes uint64
}

type DerivedMemberBuilder func([]ContentRef) (any, error)

type captureJob struct {
	captureID     uint64
	registration  uint64
	capability    CapabilityKey
	state         TerminalState
	members       []pendingMember
	sections      []captureSection
	retainedBytes uint64
	terminalErr   error
	abortErr      error
}

type boundedManifestInventory struct {
	limit uint64
	refs  []ContentRef
	seen  map[string]struct{}
}

func newBoundedManifestInventory(limit uint64) *boundedManifestInventory {
	return &boundedManifestInventory{limit: limit, seen: make(map[string]struct{})}
}

func (i *boundedManifestInventory) addAll(refs []ContentRef) error {
	start := len(i.refs)
	for _, ref := range refs {
		if ref == (ContentRef{}) {
			continue
		}
		key := ref.Key()
		if _, exists := i.seen[key]; exists {
			continue
		}
		if uint64(len(i.refs)) >= i.limit {
			for _, added := range i.refs[start:] {
				delete(i.seen, added.Key())
			}
			i.refs = i.refs[:start]
			return errCaptureMemberLimit
		}
		i.seen[key] = struct{}{}
		i.refs = append(i.refs, ref)
	}
	return nil
}

func (t *CaptureTransaction) CaptureID() uint64 {
	if t == nil {
		return 0
	}
	return t.captureID
}

func (t *CaptureTransaction) DefineSection(name string, state TerminalState, expectedRecords uint64) error {
	if t == nil || t.recorder == nil {
		return ErrTransactionClosed
	}
	if err := validateID("section", name); err != nil {
		return err
	}
	if !state.valid() {
		return fmt.Errorf("invalid section state %q", state)
	}
	if state == StateSuccess && expectedRecords == 0 {
		return errors.New("successful section must expect records")
	}
	if state == StateEmpty && expectedRecords != 0 {
		return errors.New("empty section must expect zero records")
	}

	t.mu.Lock()
	defer t.mu.Unlock()
	if t.closed {
		return ErrTransactionClosed
	}
	if _, exists := t.sections[name]; exists {
		return fmt.Errorf("section %q is already defined", name)
	}
	t.sections[name] = &captureSection{name: name, state: state, expectedRecords: expectedRecords}
	return nil
}

// AddOwned transfers value ownership on success. The caller MUST pass a fully
// detached immutable DTO and MUST NOT mutate it after this call.
func (t *CaptureTransaction) AddOwned(section string, memberType MemberType, value any, retainedBytes uint64) (MemberHandle, error) {
	return t.addOwned(section, memberType, value, nil, nil, retainedBytes)
}

// AddDerivedOwned transfers an immutable builder plus its process-local
// dependencies. The builder runs on the recorder worker after earlier handles
// have sealed and receives only portable ContentRef values.
func (t *CaptureTransaction) AddDerivedOwned(
	section string,
	memberType MemberType,
	dependencies []MemberHandle,
	build DerivedMemberBuilder,
	retainedBytes uint64,
) (MemberHandle, error) {
	if t == nil || t.recorder == nil {
		return MemberHandle{}, ErrTransactionClosed
	}
	if len(dependencies) == 0 {
		return MemberHandle{}, errors.New("derived member requires dependencies")
	}
	if build == nil {
		return MemberHandle{}, errors.New("derived member builder is nil")
	}
	if uint64(len(dependencies)) > t.recorder.config.MaxMembers-1 {
		return t.addOwned(section, memberType, nil, dependencies, build, retainedBytes)
	}
	for i, dependency := range dependencies {
		if dependency.id == 0 || dependency.future == nil {
			return MemberHandle{}, fmt.Errorf("dependency %d is invalid", i)
		}
	}
	return t.addOwned(section, memberType, nil, dependencies, build, retainedBytes)
}

// AddReference attaches an earlier process-local handle to this capability
// root without copying or resealing its content. Its complete transitive
// inventory is carried into the new manifest.
func (t *CaptureTransaction) AddReference(section string, handle MemberHandle) error {
	if t == nil || t.recorder == nil {
		return ErrTransactionClosed
	}
	if handle.id == 0 || handle.future == nil {
		return errors.New("referenced member handle is invalid")
	}

	t.mu.Lock()
	defer t.mu.Unlock()
	if t.closed {
		return ErrTransactionClosed
	}
	defined, ok := t.sections[section]
	if !ok {
		return fmt.Errorf("section %q is not defined", section)
	}
	if uint64(len(t.members)) >= t.recorder.config.MaxMembers-1 {
		t.terminalErr = errors.Join(t.terminalErr, errCaptureMemberLimit)
		defined.state = StateIncomplete
		return errCaptureMemberLimit
	}

	defined.members = append(defined.members, len(t.members))
	t.members = append(t.members, pendingMember{reference: handle})
	return nil
}

func (t *CaptureTransaction) addOwned(
	section string,
	memberType MemberType,
	value any,
	dependencies []MemberHandle,
	build DerivedMemberBuilder,
	retainedBytes uint64,
) (MemberHandle, error) {
	if t == nil || t.recorder == nil {
		return MemberHandle{}, ErrTransactionClosed
	}
	if err := memberType.Validate(); err != nil {
		return MemberHandle{}, err
	}
	if value == nil && build == nil {
		return MemberHandle{}, errors.New("owned member value is nil")
	}
	if retainedBytes == 0 {
		return MemberHandle{}, errors.New("owned member retained bytes must be nonzero")
	}

	t.mu.Lock()
	defer t.mu.Unlock()
	if t.closed {
		return MemberHandle{}, ErrTransactionClosed
	}
	defined, ok := t.sections[section]
	if !ok {
		return MemberHandle{}, fmt.Errorf("section %q is not defined", section)
	}
	if uint64(len(t.members)) >= t.recorder.config.MaxMembers-1 || uint64(len(dependencies)) > t.recorder.config.MaxMembers-1 {
		t.terminalErr = errors.Join(t.terminalErr, errCaptureMemberLimit)
		defined.state = StateIncomplete
		return MemberHandle{}, errCaptureMemberLimit
	}
	nextBytes, err := checkedAdd(t.retainedBytes, retainedBytes)
	if err != nil || nextBytes > t.recorder.config.MaxRetainedBytes {
		t.terminalErr = errors.Join(t.terminalErr, errors.New("capture retained-byte limit exceeded"))
		defined.state = StateIncomplete
		return MemberHandle{}, errors.New("capture retained-byte limit exceeded")
	}

	handle := MemberHandle{
		id:     t.recorder.handleSequence.Add(1),
		future: &memberFuture{state: HandlePending},
	}
	defined.members = append(defined.members, len(t.members))
	t.members = append(t.members, pendingMember{
		memberType:    memberType,
		value:         value,
		dependencies:  slices.Clone(dependencies),
		build:         build,
		handle:        handle,
		retainedBytes: retainedBytes,
	})
	t.retainedBytes = nextBytes
	return handle, nil
}

func (t *CaptureTransaction) Commit(capability CapabilityKey, state TerminalState) error {
	if err := capability.Validate(); err != nil {
		return err
	}
	if !state.valid() {
		return fmt.Errorf("invalid capability state %q", state)
	}
	return t.finish(capability, state, nil)
}

// MarkIncomplete records a non-fatal capture defect. Admitted members still
// seal, but the capability cannot claim completeness.
func (t *CaptureTransaction) MarkIncomplete(cause error) error {
	if t == nil || t.recorder == nil {
		return ErrTransactionClosed
	}
	if cause == nil {
		return errors.New("incomplete cause is required")
	}
	t.mu.Lock()
	defer t.mu.Unlock()
	if t.closed {
		return ErrTransactionClosed
	}
	t.terminalErr = errors.Join(t.terminalErr, cause)
	return nil
}

func (t *CaptureTransaction) Abort(capability CapabilityKey, cause error) error {
	if err := capability.Validate(); err != nil {
		return err
	}
	if cause == nil {
		cause = ErrCaptureAborted
	}
	return t.finish(capability, StateIncomplete, errors.Join(ErrCaptureAborted, cause))
}

func (t *CaptureTransaction) finish(capability CapabilityKey, state TerminalState, abortErr error) error {
	if t == nil || t.recorder == nil {
		return ErrTransactionClosed
	}

	t.mu.Lock()
	if t.closed {
		t.mu.Unlock()
		return ErrTransactionClosed
	}
	t.closed = true
	if t.terminalErr != nil {
		state = StateIncomplete
	}
	sections := make([]captureSection, 0, len(t.sections))
	for _, section := range t.sections {
		copySection := *section
		copySection.members = slices.Clone(section.members)
		if abortErr != nil && copySection.state != StateNotApplicable {
			copySection.state = StateIncomplete
		}
		sections = append(sections, copySection)
	}
	slices.SortFunc(sections, func(a, b captureSection) int {
		if a.name < b.name {
			return -1
		}
		if a.name > b.name {
			return 1
		}
		return 0
	})
	job := captureJob{
		captureID:     t.captureID,
		registration:  t.registration,
		capability:    capability,
		state:         state,
		members:       t.members,
		sections:      sections,
		retainedBytes: t.retainedBytes,
		terminalErr:   t.terminalErr,
		abortErr:      abortErr,
	}
	t.members = nil
	t.sections = nil
	t.mu.Unlock()

	// Begin reserved this position. The default branch protects the invariant
	// from future implementation changes without ever blocking this call.
	select {
	case t.recorder.jobs <- job:
		t.recorder.active.Done()
		return nil
	default:
		err := errors.New("reserved diagnostic terminal queue position was unavailable")
		for _, member := range job.members {
			if member.handle.future != nil {
				member.handle.future.resolve(ContentRef{}, nil, err)
			}
		}
		<-t.recorder.admission
		t.recorder.active.Done()
		return err
	}
}

func (r *Recorder) run() {
	defer r.worker.Done()
	for job := range r.jobs {
		r.emitGap()
		r.process(job)
		<-r.admission
	}
	r.emitGap()
}

func (r *Recorder) process(job captureJob) {
	members := make(MemorySource, len(job.members)+1)
	refs := make([]ContentRef, len(job.members))
	inventory := newBoundedManifestInventory(r.config.MaxMembers - 1)
	captureErr := job.terminalErr
	memberLimitExceeded := false

	for i, member := range job.members {
		if job.abortErr != nil {
			if member.handle.future != nil {
				member.handle.future.resolve(ContentRef{}, nil, job.abortErr)
			}
			continue
		}
		if memberLimitExceeded {
			if member.handle.future != nil {
				member.handle.future.resolve(ContentRef{}, nil, errCaptureMemberLimit)
			}
			continue
		}
		if member.reference.future != nil {
			ref, referencedInventory, err, state := member.reference.resolveInventory()
			if state != HandleSealed || err != nil {
				if err == nil {
					err = fmt.Errorf("referenced %s is %s", member.reference, state)
				}
				captureErr = errors.Join(captureErr, err)
				continue
			}
			if err := inventory.addAll(referencedInventory); err != nil {
				captureErr = errors.Join(captureErr, err)
				memberLimitExceeded = true
				continue
			}
			refs[i] = ref
			continue
		}
		value := member.value
		var memberInventory *boundedManifestInventory
		if member.build != nil {
			memberInventory = newBoundedManifestInventory(r.config.MaxMembers - 1)
			resolved := make([]ContentRef, 0, len(member.dependencies))
			var dependencyErr error
			for _, dependency := range member.dependencies {
				ref, dependencyInventory, err, state := dependency.resolveInventory()
				if state != HandleSealed || err != nil {
					if err == nil {
						err = fmt.Errorf("dependency %s is %s", dependency, state)
					}
					captureErr = errors.Join(captureErr, err)
					dependencyErr = err
					resolved = nil
					break
				}
				resolved = append(resolved, ref)
				if err := memberInventory.addAll(dependencyInventory); err != nil {
					captureErr = errors.Join(captureErr, err)
					dependencyErr = err
					memberLimitExceeded = true
					resolved = nil
					break
				}
			}
			if resolved == nil {
				member.handle.future.resolve(ContentRef{}, nil, errors.Join(errors.New("derived member dependency failed"), dependencyErr))
				continue
			}
			var err error
			value, err = member.build(resolved)
			if err != nil {
				captureErr = errors.Join(captureErr, err)
				member.handle.future.resolve(ContentRef{}, nil, err)
				continue
			}
		}
		ref, data, err := Seal(member.memberType, value)
		if err != nil {
			captureErr = errors.Join(captureErr, fmt.Errorf("seal %s@%s: %w", member.memberType.Kind, member.memberType.Schema, err))
			member.handle.future.resolve(ContentRef{}, nil, err)
			continue
		}
		memberInventoryRefs := []ContentRef{ref}
		if memberInventory != nil {
			if err := memberInventory.addAll(memberInventoryRefs); err != nil {
				captureErr = errors.Join(captureErr, err)
				memberLimitExceeded = true
				member.handle.future.resolve(ContentRef{}, nil, err)
				continue
			}
			SortContentRefs(memberInventory.refs)
			memberInventoryRefs = memberInventory.refs
		}
		if err := inventory.addAll(memberInventoryRefs); err != nil {
			captureErr = errors.Join(captureErr, err)
			memberLimitExceeded = true
			member.handle.future.resolve(ContentRef{}, nil, err)
			continue
		}
		refs[i] = ref
		members[ref.Key()] = data
		member.handle.future.resolve(ref, memberInventoryRefs, nil)
	}

	state := job.state
	if job.abortErr != nil || captureErr != nil {
		state = StateIncomplete
	}
	root := CapabilityRootV1{Capability: job.capability, State: state}
	for _, section := range job.sections {
		inventory := SectionInventoryV1{
			Name:            section.name,
			State:           section.state,
			ExpectedRecords: section.expectedRecords,
		}
		for _, index := range section.members {
			if index >= 0 && index < len(refs) && refs[index] != (ContentRef{}) {
				inventory.Members = append(inventory.Members, refs[index])
			}
		}
		if len(inventory.Members) != len(section.members) && inventory.State != StateNotApplicable {
			inventory.State = StateIncomplete
		}
		root.Sections = append(root.Sections, inventory)
	}
	if len(root.Sections) == 0 {
		root.Sections = []SectionInventoryV1{{Name: "capture", State: StateIncomplete, ExpectedRecords: 0}}
		root.State = StateIncomplete
	}
	if err := root.Validate(); err != nil {
		captureErr = errors.Join(captureErr, fmt.Errorf("validate capability root: %w", err))
		r.config.Sink.Store(CaptureResult{
			CaptureID: job.captureID, Registration: job.registration, RetainedBytes: job.retainedBytes,
			Err: errors.Join(job.abortErr, captureErr),
		})
		return
	}

	rootRef, rootData, err := Seal(MemberType{Kind: KindCapabilityRoot, Schema: SchemaV1}, root)
	if err != nil {
		captureErr = errors.Join(captureErr, fmt.Errorf("seal capability root: %w", err))
		r.config.Sink.Store(CaptureResult{
			CaptureID: job.captureID, Registration: job.registration, RetainedBytes: job.retainedBytes,
			Err: errors.Join(job.abortErr, captureErr),
		})
		return
	}
	members[rootRef.Key()] = rootData
	manifestRefs := make([]ContentRef, 0, len(inventory.refs)+1)
	manifestRefs = append(manifestRefs, inventory.refs...)
	if _, exists := inventory.seen[rootRef.Key()]; !exists {
		manifestRefs = append(manifestRefs, rootRef)
	}
	SortContentRefs(manifestRefs)
	manifest := ManifestV1{
		Format:           FormatV1,
		Canonicalization: CanonicalJSONV1,
		Sensitivity:      ExactRestrictedSensitivity(),
		Authenticity:     AuthenticityV1{State: TrustNotProvided},
		Roots: []CapabilityRefV1{{
			CapabilityKey: job.capability,
			State:         root.State,
			Root:          rootRef,
		}},
		Members: manifestRefs,
	}
	r.config.Sink.Store(CaptureResult{
		CaptureID:     job.captureID,
		Registration:  job.registration,
		Manifest:      manifest,
		Members:       members,
		RetainedBytes: job.retainedBytes,
		Err:           errors.Join(job.abortErr, captureErr),
	})
}

func (r *Recorder) emitGap() {
	r.gapMu.Lock()
	count := r.rejectedCount
	if count == 0 {
		r.gapMu.Unlock()
		return
	}
	gap := CaptureGapV1{
		FirstAttempt: r.rejectedFirst,
		LastAttempt:  r.rejectedLast,
		Count:        count,
		Reason:       "admission_saturated",
	}
	r.rejectedCount = 0
	r.rejectedFirst = 0
	r.rejectedLast = 0
	r.gapMu.Unlock()
	gapRef, gapData, err := Seal(MemberType{Kind: KindCaptureGap, Schema: SchemaV1}, gap)
	if err != nil {
		r.config.Sink.Store(CaptureResult{Err: err})
		return
	}
	capability := CapabilityKey{Name: CapabilityCaptureAccounting, Revision: 1}
	root := CapabilityRootV1{
		Capability: capability,
		State:      StateSuccess,
		Sections: []SectionInventoryV1{{
			Name:            "gaps",
			State:           StateSuccess,
			ExpectedRecords: gap.Count,
			Members:         []ContentRef{gapRef},
		}},
	}
	rootRef, rootData, err := Seal(MemberType{Kind: KindCapabilityRoot, Schema: SchemaV1}, root)
	if err != nil {
		r.config.Sink.Store(CaptureResult{Err: err})
		return
	}
	refs := []ContentRef{gapRef, rootRef}
	SortContentRefs(refs)
	r.config.Sink.Store(CaptureResult{
		CaptureID: gap.LastAttempt,
		Manifest: ManifestV1{
			Format:           FormatV1,
			Canonicalization: CanonicalJSONV1,
			Sensitivity:      ExactRestrictedSensitivity(),
			Authenticity:     AuthenticityV1{State: TrustNotProvided},
			Roots:            []CapabilityRefV1{{CapabilityKey: capability, State: StateSuccess, Root: rootRef}},
			Members:          refs,
		},
		Members: MemorySource{gapRef.Key(): gapData, rootRef.Key(): rootData},
	})
}

// MemorySink is an in-memory test sink. Production archive ownership and
// retention bounds belong to the later storage stage.
type MemorySink struct {
	mu      sync.Mutex
	results []CaptureResult
	members MemorySource
}

func (s *MemorySink) Store(result CaptureResult) {
	if s == nil {
		return
	}
	s.mu.Lock()
	if s.members == nil {
		s.members = make(MemorySource)
	}
	for key, data := range result.Members {
		if _, exists := s.members[key]; !exists {
			s.members[key] = slices.Clone(data)
		}
	}
	s.results = append(s.results, result)
	s.mu.Unlock()
}

func (s *MemorySink) Source() MemorySource {
	if s == nil {
		return nil
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	result := make(MemorySource, len(s.members))
	for key, data := range s.members {
		result[key] = slices.Clone(data)
	}
	return result
}

func (s *MemorySink) Results() []CaptureResult {
	if s == nil {
		return nil
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	return slices.Clone(s.results)
}

var _ json.Marshaler = MemberHandle{}
