// SPDX-License-Identifier: GPL-3.0-or-later

package joboutput

import (
	"errors"
	"fmt"
	"iter"
	"maps"
	"reflect"
	"slices"
	"sync"

	"github.com/netdata/netdata/go/plugins/plugin/agent/jobmgr"
	"github.com/netdata/netdata/go/plugins/plugin/framework/confgroup"
	"github.com/netdata/netdata/go/plugins/plugin/framework/dyncfg"
)

// DiscoveredJobCandidate identifies one observed membership lifetime, not merely
// a configuration hash. Removing and re-adding an equal config changes Incarnation.
type DiscoveredJobCandidate struct {
	Config      confgroup.Config
	Source      string
	Incarnation uint64
}

// DiscoveredJobInventory is a complete immutable source observation for FullName.
// Sources preserves source-slot presence through rejected or malformed replacement.
// A source lifetime changes only after the slot disappears for this identity.
type DiscoveredJobInventory struct {
	FullName   string
	Version    uint64
	Candidates []DiscoveredJobCandidate
	Sources    map[string]uint64
	AutoEnable bool
}

type candidateKey struct {
	Source      string
	Incarnation uint64
	UID         string
	Pipeline    string
}

type candidateWorkToken struct {
	Run       uint64
	Intent    uint64
	Attempt   uint64
	Candidate candidateKey
}

type candidateWakeToken struct {
	Run      uint64
	Intent   uint64
	Schedule uint64
}

type candidateWork struct {
	Token       candidateWorkToken
	Config      confgroup.Config
	Tentative   bool
	Predecessor uint64
	// Replacement revokes the old source grant only after admission.
	Replacement *candidateKey
}

type candidateWake struct {
	Token candidateWakeToken
	Due   int64
}

type candidateBinding struct {
	Config confgroup.Config
	Key    candidateKey
	Health dyncfg.Status
	Listed bool
	Stamp  uint64
}

type candidateEnablement uint8

const (
	candidatePassive candidateEnablement = iota
	candidateEnabled
	candidateDisabled
)

type candidatePhase uint8

const (
	candidateDormant candidatePhase = iota
	candidatePreparing
	candidateWaitingPhase
	candidateStarting
	candidateRunning
	candidateRetryWait
	candidateFailedManual
	candidateBlocked
)

type candidateScope uint8

const (
	candidateSourceAlternatives candidateScope = iota
	candidateExactConfig
)

type candidateOutcomeKind uint8

const (
	candidateAdmitted candidateOutcomeKind = iota
	candidateRejected
	candidateFailed
	candidateWaiting
	candidateFrameworkBlocked
	candidateSourceRevoked
	candidateAdmittedInvalid
)

type candidateBlockPolicy uint8

const (
	candidateBlockQuarantined candidateBlockPolicy = iota
	candidateBlockAcceptedWait
	candidateBlockFailedRetry
)

type candidateManualReason uint8

const (
	candidateManualNone candidateManualReason = iota
	candidateManualSourceRevoked
	candidateManualPreparationInvalid
)

type candidateOutcome struct {
	Kind  candidateOutcomeKind
	Block candidateBlockPolicy
	// AdmitWaiting preserves the source-specific dependency/vnode acceptance boundary.
	AdmitWaiting bool
	// RetryAfter is supplied only for a positively classified automatic retry.
	RetryAfter int
	AfterReady bool
	// Listed supplies source-specific failed-stock removal policy.
	Listed bool
}

type candidateEntry struct {
	Candidate      DiscoveredJobCandidate
	Key            candidateKey
	SourceLifetime uint64
	Observed       bool
	SourceOwned    bool
	Admitted       bool
	GrantIntent    uint64
	Rejected       bool
	Initial        bool
	Due            int64
	HasDue         bool
}

// candidateRecord is the sole policy state for one public job identity. Its
// canonical lane serializes policy changes. Population preparation runs off-loop;
// live runtime settlement prepares only bounded entry/header changes on the loop.
type candidateRecord struct {
	Revision         uint64
	Inventory        DiscoveredJobInventory
	Cursor           *candidateKey
	CursorConfig     confgroup.Config
	CursorLifetime   uint64
	Binding          *candidateBinding
	Intent           uint64
	Enablement       candidateEnablement
	Scope            candidateScope
	Phase            candidatePhase
	SearchClosed     bool
	Entries          map[candidateKey]*candidateEntry
	Pass             []candidateKey
	PassCursor       int
	Active           *candidateWork
	Proposal         *candidateWork
	Wake             *candidateWake
	FrameworkBlocked bool
	Manual           candidateManualReason
	Block            candidateBlockPolicy
	// Only the owning lane reads the ledger while preparing. Entry values stay
	// immutable until Apply merges these private changes under the controller lock.
	entryChanges  map[candidateKey]*candidateEntry
	entriesOwned  bool
	owningTier    int
	tierDirty     bool
	grantCut      uint64
	grantSelected candidateKey
}

type candidateDecision struct {
	Ignored bool
	Binding *candidateBinding
	Remove  bool
	Work    *candidateWork
	Wake    *candidateWake
	Revoke  bool
}

type candidateController struct {
	mu       sync.Mutex
	run      uint64
	sequence uint64
	records  map[string]*candidateRecord
	closed   bool
}

type candidateDelta struct {
	owner    *candidateController
	name     string
	before   *candidateRecord
	after    *candidateRecord
	Decision candidateDecision
}

// Finalize the effect from the prepared state, so revocation cannot leave an old
// wake in the execution projection. This runs before callers prepare graph work.
func finalizeCandidateDelta(result **candidateDelta) {
	if delta := *result; delta != nil {
		delta.Decision.Wake = delta.after.Wake
	}
}

func newCandidateController(run uint64) *candidateController {
	return &candidateController{run: run, records: make(map[string]*candidateRecord)}
}

func (c *candidateController) next() (uint64, error) {
	c.mu.Lock()
	defer c.mu.Unlock()
	if c.closed {
		return 0, jobmgr.ErrProcessAttemptRetired
	}
	if c.run == 0 {
		return 0, errors.New("job output: candidate controller retired")
	}
	if c.sequence == ^uint64(0) {
		return 0, errors.New("job output: candidate generation wrapped")
	}
	c.sequence++
	return c.sequence, nil
}

func (c *candidateController) prepare(name string) (*candidateDelta, error) {
	if c == nil || name == "" {
		return nil, errors.New("job output: invalid candidate identity")
	}
	c.mu.Lock()
	defer c.mu.Unlock()
	if c.closed {
		return nil, jobmgr.ErrProcessAttemptRetired
	}
	before := c.records[name]
	after := prepareCandidateRecord(before)
	return &candidateDelta{owner: c, name: name, before: before, after: after, Decision: candidateDecision{Wake: after.Wake}}, nil
}

// apply merges prepared entry changes and swaps the header after its predecessor check.
// Decision effects must be released
// after this and the corresponding graph update commit under the same claim.
func (d *candidateDelta) apply() bool {
	if d == nil || d.owner == nil {
		return false
	}
	c := d.owner
	c.mu.Lock()
	defer c.mu.Unlock()
	if c.closed || c.records[d.name] != d.before {
		return false
	}
	if d.Decision.Ignored {
		return true
	}
	if !d.after.entriesOwned {
		for key, entry := range d.after.entryChanges {
			if entry == nil {
				delete(d.after.Entries, key)
			} else {
				d.after.Entries[key] = entry
			}
		}
	}
	d.after.entryChanges = nil
	d.after.entriesOwned = false
	d.after.Revision++
	c.records[d.name] = d.after
	// Applying twice is an error even if the lane is reserved.
	d.before = nil
	return true
}

func (c *candidateController) snapshot(name string) *candidateRecord {
	c.mu.Lock()
	defer c.mu.Unlock()
	if r := c.records[name]; r != nil {
		return cloneCandidateRecord(r)
	}
	return nil
}

// candidateView is the immutable, constant-size projection used outside a job's
// lane. Config maps are shared immutable input; ledger maps are never exposed.
type candidateView struct {
	Binding          *candidateBinding
	Active           *candidateWork
	Proposal         *candidateWork
	Enablement       candidateEnablement
	Phase            candidatePhase
	Intent           uint64
	SearchClosed     bool
	FrameworkBlocked bool
	Manual           candidateManualReason
}

func (c *candidateController) view(name string) *candidateView {
	c.mu.Lock()
	defer c.mu.Unlock()
	r := c.records[name]
	if r == nil || c.closed {
		return nil
	}
	v := &candidateView{Enablement: r.Enablement, Phase: r.Phase, Intent: r.Intent, SearchClosed: r.SearchClosed, FrameworkBlocked: r.FrameworkBlocked, Manual: r.Manual}
	if r.Binding != nil {
		binding := *r.Binding
		v.Binding = &binding
	}
	v.Active = cloneCandidateWork(r.Active)
	v.Proposal = cloneCandidateWork(r.Proposal)
	return v
}

func (c *candidateController) valid(name string, token candidateWorkToken) bool {
	c.mu.Lock()
	defer c.mu.Unlock()
	r := c.records[name]
	return !c.closed && r != nil && c.validRecord(r, token)
}

func (c *candidateController) validRecord(r *candidateRecord, token candidateWorkToken) bool {
	if token.Run != c.run {
		return false
	}
	entry := r.entry(token.Candidate)
	if entry == nil || entry.Rejected {
		return false
	}
	if r.Proposal != nil && r.Proposal.Token == token {
		return r.Proposal.Predecessor == r.Intent && candidateAuthorized(r, token.Candidate)
	}
	return r.Enablement == candidateEnabled && r.Active != nil && r.Active.Token == token && token.Intent == r.Intent && candidateAuthorized(r, token.Candidate)
}

func (c *candidateController) retire() {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.closed = true
	c.records = make(map[string]*candidateRecord)
}

func (c *candidateController) prepareInventory(input DiscoveredJobInventory, now int64) (delta *candidateDelta, err error) {
	defer finalizeCandidateDelta(&delta)
	return c.prepareInventoryValidated(input, nil, now)
}

// prepareInventoryValidated quarantines metadata-invalid exact memberships before
// source arbitration, while retaining their source-slot presence.
func (c *candidateController) prepareInventoryValidated(input DiscoveredJobInventory, invalid map[candidateKey]bool, now int64) (delta *candidateDelta, err error) {
	defer finalizeCandidateDelta(&delta)
	inv, err := normalizeCandidateInventory(input)
	if err != nil {
		return nil, err
	}
	d, err := c.prepare(inv.FullName)
	if err != nil {
		return nil, err
	}
	r := d.after
	if inv.Version < r.Inventory.Version {
		d.Decision.Ignored = true
		return d, nil
	}
	if inv.Version == r.Inventory.Version {
		if !reflect.DeepEqual(inv, r.Inventory) {
			return nil, errors.New("job output: conflicting candidate inventory version")
		}
		d.Decision.Ignored = true
		return d, nil
	}
	// A producer should suppress identical snapshots. Keep that guarantee here too:
	// newer observation stamps alone cannot create an enabled lifetime.
	oldContent := r.Inventory
	oldContent.Version = inv.Version
	if reflect.DeepEqual(oldContent, inv) {
		r.Inventory = inv
		return d, nil
	}
	r.Inventory = inv
	r.ownEntries()
	for _, e := range r.entries() {
		if e.SourceOwned || e.Candidate.Config.SourceType() != confgroup.TypeDyncfg {
			e.Observed = false
		}
	}
	for _, observed := range inv.Candidates {
		key := candidateIdentity(observed)
		e := r.entry(key)
		if e == nil {
			e = &candidateEntry{Candidate: observed, Key: key, Initial: true}
			r.setEntry(key, e)
		}
		e.Observed = true
		e.SourceOwned = true
		e.SourceLifetime = inv.Sources[observed.Source]
		if invalid[key] {
			e.Rejected = true
		}
	}
	r.pruneRecoveryGrants()
	// Missing membership is not source removal: an admitted source grant survives
	// until a replacement is admitted, including while replacement validation runs.
	if r.Proposal != nil && (!candidateAuthorized(r, r.Proposal.Token.Candidate) || r.entry(r.Proposal.Token.Candidate).Rejected) {
		r.Proposal = nil
		d.Decision.Revoke = true
	}
	if r.Active != nil && (!candidateAuthorized(r, r.Active.Token.Candidate) || r.entry(r.Active.Token.Candidate).Rejected) {
		r.Active = nil
		r.Pass = nil
		r.PassCursor = 0
		r.Wake = nil
		d.Decision.Wake = nil
		if r.FrameworkBlocked && r.Block != candidateBlockQuarantined {
			r.FrameworkBlocked = false
			r.Phase = candidateDormant
		}
		d.Decision.Revoke = true
	}
	if r.Binding != nil && (r.entry(r.Binding.Key) == nil || r.entry(r.Binding.Key).SourceOwned) && !candidateAuthorized(r, r.Binding.Key) {
		r.Binding = nil
		d.Decision.Remove = true
		d.Decision.Revoke = true
	}
	return d, c.reconcileSource(d, now)
}

func normalizeCandidateInventory(input DiscoveredJobInventory) (DiscoveredJobInventory, error) {
	if input.FullName == "" || input.Version == 0 {
		return DiscoveredJobInventory{}, errors.New("job output: invalid candidate inventory")
	}
	out := input
	out.Sources = maps.Clone(input.Sources)
	if out.Sources == nil {
		out.Sources = make(map[string]uint64)
	}
	out.Candidates = make([]DiscoveredJobCandidate, 0, len(input.Candidates))
	seen := make(map[candidateKey]struct{}, len(input.Candidates))
	for _, candidate := range input.Candidates {
		if candidate.Config == nil || candidate.Config.FullName() != input.FullName || candidate.Source == "" || candidate.Incarnation == 0 {
			return DiscoveredJobInventory{}, errors.New("job output: invalid candidate membership")
		}
		config, err := candidate.Config.Clone()
		if err != nil {
			return DiscoveredJobInventory{}, fmt.Errorf("job output: clone candidate: %w", err)
		}
		candidate.Config = config
		key := candidateIdentity(candidate)
		if _, ok := seen[key]; ok {
			return DiscoveredJobInventory{}, errors.New("job output: duplicate candidate membership")
		}
		seen[key] = struct{}{}
		out.Candidates = append(out.Candidates, candidate)
		if _, ok := out.Sources[candidate.Source]; !ok {
			out.Sources[candidate.Source] = 1
		}
	}
	slices.SortFunc(out.Candidates, func(a, b DiscoveredJobCandidate) int {
		return compareCandidateKeys(candidateIdentity(a), candidateIdentity(b))
	})
	return out, nil
}

func candidateIdentity(candidate DiscoveredJobCandidate) candidateKey {
	return candidateKey{Source: candidate.Source, Incarnation: candidate.Incarnation, UID: candidate.Config.UID(), Pipeline: candidate.Config.DiscoveryPipelineID()}
}

func compareCandidateKeys(a, b candidateKey) int {
	if a.UID < b.UID {
		return -1
	}
	if a.UID > b.UID {
		return 1
	}
	if a.Source < b.Source {
		return -1
	}
	if a.Source > b.Source {
		return 1
	}
	if a.Incarnation < b.Incarnation {
		return -1
	}
	if a.Incarnation > b.Incarnation {
		return 1
	}
	if a.Pipeline < b.Pipeline {
		return -1
	}
	if a.Pipeline > b.Pipeline {
		return 1
	}
	return 0
}

func candidateAuthorized(r *candidateRecord, key candidateKey) bool {
	e := r.entry(key)
	if e == nil {
		return false
	}
	if e.Observed {
		return true
	}
	lifetime, present := r.Inventory.Sources[key.Source]
	return candidateRetainedGrant(r, key, e) && present && lifetime == e.SourceLifetime
}

// Readiness revokes old sibling recovery in O(1). Membership is independent of
// this retained-admission grant; a genuine pending successor keeps its exact grant.
func candidateRetainedGrant(r *candidateRecord, key candidateKey, e *candidateEntry) bool {
	if !e.Admitted {
		return false
	}
	if r.grantCut == 0 || e.GrantIntent > r.grantCut || key == r.grantSelected {
		return true
	}
	return r.Proposal != nil && r.Proposal.Predecessor == r.Intent && r.Proposal.Token.Candidate == key
}

// Called only in population preparation, never from runtime settlement Apply.
func (r *candidateRecord) pruneRecoveryGrants() {
	r.ownEntries()
	for key, entry := range r.entries() {
		if entry.Admitted && !candidateRetainedGrant(r, key, entry) {
			entry.Admitted = false
			entry.GrantIntent = 0
		}
		if !candidateAuthorized(r, key) {
			r.removeEntry(key)
		}
	}
}

func candidateSourceBlocked(r *candidateRecord, key candidateKey) bool {
	found := false
	for _, e := range r.entries() {
		if !e.Observed || e.Key.Source != key.Source {
			continue
		}
		found = true
		if !e.Rejected {
			return false
		}
	}
	return found
}

func candidateSourceChoice(r *candidateRecord) *candidateKey {
	var selected *candidateEntry
	for _, e := range r.entries() {
		if !e.Observed || !e.SourceOwned || e.Rejected {
			continue
		}
		if selected == nil || e.Candidate.Config.SourceTypePriority() > selected.Candidate.Config.SourceTypePriority() || e.Candidate.Config.SourceTypePriority() == selected.Candidate.Config.SourceTypePriority() && compareCandidateKeys(e.Key, selected.Key) < 0 {
			selected = e
		}
	}
	if r.Cursor != nil && r.CursorConfig != nil {
		previous := r.entry(*r.Cursor)
		lifetime, present := r.Inventory.Sources[r.Cursor.Source]
		observed := previous != nil && previous.Observed && !previous.Rejected
		protected := present && lifetime == r.CursorLifetime && (previous == nil || !previous.Rejected) && candidateSourceBlocked(r, *r.Cursor)
		if (observed || protected) && (selected == nil || selected.Candidate.Config.SourceTypePriority() <= r.CursorConfig.SourceTypePriority()) {
			key := *r.Cursor
			return &key
		}
	}
	if selected == nil {
		return nil
	}
	key := selected.Key
	return &key
}

func (c *candidateController) reconcileSource(d *candidateDelta, now int64) error {
	r := d.after
	choice := candidateSourceChoice(r)
	changed := !equalCandidateKey(r.Cursor, choice)
	r.Cursor = choice
	if choice != nil {
		if entry := r.entry(*choice); entry != nil {
			r.CursorConfig = entry.Candidate.Config
			r.CursorLifetime = entry.SourceLifetime
		}
	} else {
		r.CursorConfig = nil
		r.CursorLifetime = 0
	}
	if r.Phase == candidateBlocked {
		d.Decision.Wake = r.Wake
		return nil
	}
	if r.Binding != nil && r.Binding.Config.SourceType() == confgroup.TypeDyncfg && !r.entry(r.Binding.Key).SourceOwned {
		return nil
	}
	// A selected admitted source remains authoritative against rejected revisions.
	if r.Binding != nil && candidateSourceBlocked(r, r.Binding.Key) {
		current := r.Binding.Config.SourceTypePriority()
		if choice == nil || r.entry(*choice).Candidate.Config.SourceTypePriority() <= current {
			return c.progress(d, now)
		}
	}
	if choice == nil {
		if r.Binding != nil {
			r.Binding = nil
			d.Decision.Remove = true
			c.revoke(r)
			d.Decision.Revoke = true
		}
		return c.progress(d, now)
	}
	e := r.entry(*choice)
	if e == nil {
		return nil
	}
	if r.Binding != nil && r.Binding.Key == *choice {
		return c.progress(d, now)
	}
	if !changed && r.Binding == nil {
		return c.progress(d, now)
	}
	if r.Proposal != nil && r.Proposal.Token.Candidate == *choice {
		return nil
	}
	// Same-tier additions belong to an existing open search. A still-observed
	// admitted incumbent is sticky even if an alternative's UID sorts first.
	if r.Binding != nil && candidateAuthorized(r, r.Binding.Key) && r.entry(r.Binding.Key).Observed && r.Binding.Config.SourceTypePriority() >= e.Candidate.Config.SourceTypePriority() {
		return c.progress(d, now)
	}
	if !r.Inventory.AutoEnable {
		c.revoke(r)
		intent, err := c.next()
		if err != nil {
			return err
		}
		r.Intent = intent
		r.Enablement = candidatePassive
		r.Scope = candidateSourceAlternatives
		if e.Candidate.Config.SourceType() == confgroup.TypeDyncfg {
			r.Scope = candidateExactConfig
		}
		r.SearchClosed = false
		r.Phase = candidateDormant
		return c.admit(d, e.Key, dyncfg.StatusAccepted, true)
	}
	if r.Intent == 0 {
		intent, err := c.next()
		if err != nil {
			return err
		}
		r.Intent = intent
	}
	generation, err := c.next()
	if err != nil {
		return err
	}
	attempt, err := c.next()
	if err != nil {
		return err
	}
	work := &candidateWork{Token: candidateWorkToken{Run: c.run, Intent: generation, Attempt: attempt, Candidate: e.Key}, Config: e.Candidate.Config, Tentative: true, Predecessor: r.Intent}
	if r.Binding != nil && r.Binding.Key.Source == e.Key.Source && !r.entry(r.Binding.Key).Observed {
		key := r.Binding.Key
		work.Replacement = &key
	}
	r.Proposal = work
	// Initial auto-source work is enabled; existing disabled intent stays disabled
	// until this meaningful replacement is admitted.
	if r.Binding == nil {
		r.Enablement = candidateEnabled
	}
	d.Decision.Work = work
	return nil
}

func equalCandidateKey(a, b *candidateKey) bool {
	return a == nil && b == nil || a != nil && b != nil && *a == *b
}

func (c *candidateController) revoke(r *candidateRecord) {
	r.Active = nil
	r.Proposal = nil
	r.Wake = nil
	r.Pass = nil
	r.PassCursor = 0
	r.FrameworkBlocked = false
	r.Manual = candidateManualNone
	r.Block = candidateBlockQuarantined
}

func (c *candidateController) admit(d *candidateDelta, key candidateKey, health dyncfg.Status, listed bool) error {
	r := d.after
	e := r.editEntry(key)
	e.Admitted = true
	e.GrantIntent = r.Intent
	stamp, err := c.next()
	if err != nil {
		return err
	}
	r.Binding = &candidateBinding{Config: e.Candidate.Config, Key: key, Health: health, Listed: listed, Stamp: stamp}
	if r.Scope == candidateSourceAlternatives && e.Candidate.Config.SourceType() != confgroup.TypeDyncfg {
		selected := key
		r.Cursor = &selected
		r.CursorConfig = e.Candidate.Config
		r.CursorLifetime = e.SourceLifetime
	}
	d.Decision.Binding = r.Binding
	d.Decision.Remove = !listed
	return nil
}

func (c *candidateController) prepareEnable(name string, now int64) (delta *candidateDelta, err error) {
	defer finalizeCandidateDelta(&delta)
	d, err := c.prepare(name)
	if err != nil {
		return nil, err
	}
	r := d.after
	if r.Binding == nil {
		return nil, errors.New("job output: candidate enable has no admitted binding")
	}
	if r.Enablement == candidateEnabled && (r.Phase == candidateRunning || r.Active != nil || r.Proposal != nil || r.Phase == candidateBlocked) {
		d.Decision.Ignored = true
		return d, nil
	}
	intent, err := c.next()
	if err != nil {
		return nil, err
	}
	c.revoke(r)
	d.Decision.Revoke = true
	r.Intent = intent
	r.Enablement = candidateEnabled
	r.SearchClosed = false
	r.Phase = candidateDormant
	r.Scope = candidateSourceAlternatives
	r.pruneRecoveryGrants()
	if r.Binding.Config.SourceType() == confgroup.TypeDyncfg {
		r.Scope = candidateExactConfig
	}
	for _, e := range r.entries() {
		e.Initial = true
		e.HasDue = false
	}
	// Re-resolve an unaccepted replacement interrupted by DISABLE.
	choice := candidateSourceChoice(r)
	if choice != nil && *choice != r.Binding.Key && !r.entry(r.Binding.Key).Observed && !candidateSourceBlocked(r, r.Binding.Key) {
		r.Cursor = nil
		return d, c.reconcileSource(d, now)
	}
	if err := c.admit(d, r.Binding.Key, dyncfg.StatusAccepted, r.Binding.Listed); err != nil {
		return nil, err
	}
	return d, c.progress(d, now)
}

func (c *candidateController) prepareDisable(name string) (delta *candidateDelta, err error) {
	defer finalizeCandidateDelta(&delta)
	d, err := c.prepare(name)
	if err != nil {
		return nil, err
	}
	intent, err := c.next()
	if err != nil {
		return nil, err
	}
	c.revoke(d.after)
	d.after.Intent = intent
	d.after.Enablement = candidateDisabled
	d.after.Phase = candidateDormant
	d.Decision.Revoke = true
	if d.after.Binding != nil {
		binding := *d.after.Binding
		binding.Health = dyncfg.StatusDisabled
		d.after.Binding = &binding
		d.Decision.Binding = &binding
	}
	return d, nil
}

func (c *candidateController) prepareRemove(name string) (delta *candidateDelta, err error) {
	defer finalizeCandidateDelta(&delta)
	d, err := c.prepareDisable(name)
	if err != nil {
		return nil, err
	}
	if d.after.Binding != nil && d.after.Binding.Config.SourceType() == confgroup.TypeDyncfg {
		d.after.removeEntry(d.after.Binding.Key)
	}
	d.after.Binding = nil
	d.Decision.Binding = nil
	d.Decision.Remove = true
	// Cursor deliberately survives removal of a DynCfg override (#23302).
	return d, nil
}

// prepareExact is used at an interactive acceptance boundary, after preflight.
// A failed/rejected preflight must not call it.
func (c *candidateController) prepareExact(config confgroup.Config, enabled bool, now int64) (delta *candidateDelta, err error) {
	defer finalizeCandidateDelta(&delta)
	if config == nil {
		return nil, errors.New("job output: nil exact candidate")
	}
	cloned, err := config.Clone()
	if err != nil {
		return nil, err
	}
	d, err := c.prepare(config.FullName())
	if err != nil {
		return nil, err
	}
	r := d.after
	r.ownEntries()
	key := candidateKey{Source: cloned.Source(), UID: cloned.UID(), Pipeline: cloned.DiscoveryPipelineID()}
	found := false
	if r.Binding != nil && r.Binding.Config.UID() == cloned.UID() && r.Binding.Config.DiscoveryPipelineID() == cloned.DiscoveryPipelineID() {
		key = r.Binding.Key
		found = true
	} else {
		for existing := range r.entries() {
			if existing.UID == cloned.UID() && existing.Pipeline == cloned.DiscoveryPipelineID() {
				key = existing
				found = true
				break
			}
		}
	}
	if !found {
		incarnation, err := c.next()
		if err != nil {
			return nil, err
		}
		key.Incarnation = incarnation
	}
	intent, err := c.next()
	if err != nil {
		return nil, err
	}
	c.revoke(r)
	d.Decision.Revoke = true
	r.Intent = intent
	r.Scope = candidateExactConfig
	r.SearchClosed = false
	r.Phase = candidateDormant
	r.Enablement = candidatePassive
	// Exact operations pin execution without deleting the observed inventory or
	// other source-admission grants. A later explicit ENABLE reopens that scope.
	for oldKey, old := range r.entries() {
		if oldKey != key && !old.SourceOwned && old.Candidate.Config.SourceType() == confgroup.TypeDyncfg {
			r.removeEntry(oldKey)
		}
	}
	entry := r.entry(key)
	if entry == nil {
		entry = &candidateEntry{Candidate: DiscoveredJobCandidate{Config: cloned, Source: key.Source, Incarnation: key.Incarnation}, Key: key, SourceLifetime: r.Inventory.Sources[key.Source]}
		entry.Observed = cloned.SourceType() == confgroup.TypeDyncfg || r.Inventory.Version == 0
		r.setEntry(key, entry)
	}
	entry.Admitted = true
	entry.Initial = true
	entry.HasDue = false

	if err := c.admit(d, key, dyncfg.StatusAccepted, true); err != nil {
		return nil, err
	}
	if enabled {
		r.Enablement = candidateEnabled
		err = c.progress(d, now)
	}
	return d, err
}

func (c *candidateController) prepareOutcome(name string, token candidateWorkToken, outcome candidateOutcome, now int64) (delta *candidateDelta, err error) {
	defer finalizeCandidateDelta(&delta)
	d, err := c.prepare(name)
	if err != nil {
		return nil, err
	}
	r := d.after
	if !c.validRecord(r, token) {
		d.Decision.Ignored = true
		return d, nil
	}
	tentative := r.Proposal != nil && r.Proposal.Token == token
	previousBinding := r.Binding
	work := r.Active
	if tentative {
		work = r.Proposal
	}
	if r.FrameworkBlocked && !tentative {
		d.Decision.Ignored = true
		return d, nil
	}
	if outcome.Kind == candidateRejected {
		r.editEntry(token.Candidate).Rejected = true
		r.tierDirty = true
		if tentative {
			r.Proposal = nil
			r.Cursor = nil
			r.CursorConfig = nil
			r.CursorLifetime = 0
			if r.Binding != nil {
				key := r.Binding.Key
				r.Cursor = &key
				r.CursorConfig = r.Binding.Config
				if old := r.entry(key); old != nil {
					r.CursorLifetime = old.SourceLifetime
				}
			}
		} else {
			r.Active = nil
		}
		// Invalid competitors do not reset the predecessor's pass or deadlines.
		return d, c.reconcileSource(d, now)
	}
	if outcome.Kind == candidateWaiting && !outcome.AdmitWaiting {
		if !tentative {
			r.Phase = candidateWaitingPhase
		}
		return d, nil
	}
	if tentative {
		// Readiness closed only the predecessor's alternative search. A genuine
		// source replacement opens a fresh initial pass when it is admitted.
		if r.SearchClosed {
			r.pruneRecoveryGrants()
			for _, entry := range r.entries() {
				entry.Initial = true
				entry.HasDue = false
			}
		}
		r.Proposal = nil
		r.Active = work
		r.Intent = token.Intent
		r.Enablement = candidateEnabled
		r.Scope = candidateSourceAlternatives
		if work.Config.SourceType() == confgroup.TypeDyncfg {
			r.Scope = candidateExactConfig
		}
		r.SearchClosed = false
		r.FrameworkBlocked = false
		r.Manual = candidateManualNone
		r.Block = candidateBlockQuarantined
		r.Wake = nil
		d.Decision.Wake = nil
		work.Tentative = false
		work.Predecessor = 0
		if work.Replacement != nil {
			r.removeEntry(*work.Replacement)
		}
		r.Pass = nil
		r.PassCursor = 0
		r.Cursor = &work.Token.Candidate
		r.CursorConfig = work.Config
		r.CursorLifetime = r.entry(work.Token.Candidate).SourceLifetime
	}
	e := r.editEntry(token.Candidate)
	e.Initial = false
	e.HasDue = false
	switch outcome.Kind {
	case candidateWaiting:
		if err := c.admit(d, e.Key, dyncfg.StatusAccepted, true); err != nil {
			return nil, err
		}
		r.Phase = candidateWaitingPhase
		return d, nil
	case candidateSourceRevoked, candidateAdmittedInvalid:
		reason, listed := candidateManualSourceRevoked, outcome.Listed
		if outcome.Kind == candidateAdmittedInvalid {
			if previousBinding == nil || previousBinding.Key != e.Key {
				return nil, errors.New("job output: invalid admitted candidate does not own binding")
			}
			reason = candidateManualPreparationInvalid
			listed = previousBinding.Listed
		} else {
			if previousBinding != nil && previousBinding.Key != e.Key {
				r.removeEntry(previousBinding.Key)
			}
			r.Proposal = nil
			r.SearchClosed = false
		}
		if err := c.admit(d, e.Key, dyncfg.StatusFailed, listed); err != nil {
			return nil, err
		}
		r.Active = nil
		r.Pass = nil
		r.PassCursor = 0
		r.Wake = nil
		r.FrameworkBlocked = false
		r.Manual = reason
		r.Phase = candidateFailedManual
		d.Decision.Work = nil
		d.Decision.Wake = nil
		// An existing source successor owns a distinct proposed intent; failing
		// preparation of its predecessor does not cancel that successor's worker.
		d.Decision.Revoke = r.Proposal == nil
		return d, nil
	case candidateFrameworkBlocked:
		health, listed := dyncfg.StatusFailed, outcome.Listed
		if outcome.Block == candidateBlockAcceptedWait && !outcome.AfterReady && !r.SearchClosed {
			health = dyncfg.StatusAccepted
			listed = true
		}
		if err := c.admit(d, e.Key, health, listed); err != nil {
			return nil, err
		}
		r.Wake = nil
		d.Decision.Wake = nil
		if outcome.AfterReady || r.SearchClosed {
			r.Active = nil
			c.closeSearch(r, e.Key)
			r.Phase = candidateFailedManual
			return d, nil
		}
		r.FrameworkBlocked = true
		r.Block = outcome.Block
		r.Phase = candidateBlocked
		if outcome.Block == candidateBlockAcceptedWait {
			r.Phase = candidateWaitingPhase
		}
		if outcome.Block == candidateBlockFailedRetry && outcome.RetryAfter > 0 {
			e.Due = now + int64(outcome.RetryAfter)
			e.HasDue = true
			schedule, err := c.next()
			if err != nil {
				return nil, err
			}
			r.Wake = &candidateWake{Token: candidateWakeToken{Run: c.run, Intent: r.Intent, Schedule: schedule}, Due: e.Due}
			d.Decision.Wake = r.Wake
		}
	case candidateAdmitted:
		if err := c.admit(d, e.Key, dyncfg.StatusAccepted, true); err != nil {
			return nil, err
		}
		r.Phase = candidateStarting
	case candidateFailed:
		if err := c.admit(d, e.Key, dyncfg.StatusFailed, outcome.Listed); err != nil {
			return nil, err
		}
		r.Active = nil
		if outcome.AfterReady || r.SearchClosed {
			c.closeSearch(r, e.Key)
			r.Phase = candidateFailedManual
			return d, nil
		}
		if outcome.RetryAfter > 0 {
			e.Due = now + int64(outcome.RetryAfter)
			e.HasDue = true
		}
		r.Phase = candidateDormant
		return d, c.progress(d, now)
	default:
		return nil, errors.New("job output: invalid candidate outcome")
	}
	return d, nil
}

func (c *candidateController) prepareReady(name string, token candidateWorkToken) (delta *candidateDelta, err error) {
	defer finalizeCandidateDelta(&delta)
	d, err := c.prepare(name)
	if err != nil {
		return nil, err
	}
	r := d.after
	if !c.validRecord(r, token) || r.FrameworkBlocked || r.Active == nil || r.Active.Token != token {
		d.Decision.Ignored = true
		return d, nil
	}
	c.closeSearch(r, token.Candidate)
	r.Phase = candidateRunning
	if err := c.admit(d, token.Candidate, dyncfg.StatusRunning, true); err != nil {
		return nil, err
	}
	return d, nil
}

func (c *candidateController) closeSearch(r *candidateRecord, selected candidateKey) {
	if r.SearchClosed {
		return
	}
	r.SearchClosed = true
	r.Pass = nil
	r.PassCursor = 0
	r.Wake = nil
	// Runtime settlement may call this on the kernel loop. The intent cut
	// revokes historical sibling recovery without cloning or scanning entries.
	r.grantCut = r.Intent
	r.grantSelected = selected
	r.tierDirty = true
}

func (c *candidateController) prepareWake(name string, token candidateWakeToken, now int64) (delta *candidateDelta, err error) {
	defer finalizeCandidateDelta(&delta)
	d, err := c.prepare(name)
	if err != nil {
		return nil, err
	}
	r := d.after
	if r.Wake == nil || r.Wake.Token != token || token.Run != c.run || token.Intent != r.Intent || r.Wake.Due > now {
		d.Decision.Ignored = true
		return d, nil
	}
	r.Wake = nil
	d.Decision.Wake = nil
	if r.FrameworkBlocked {
		if r.Block != candidateBlockFailedRetry || r.Active == nil {
			d.Decision.Ignored = true
			return d, nil
		}
		return d, c.resumeCandidate(d, r.Active, now)
	}
	return d, c.progress(d, now)
}

// prepareResume is an acknowledged recovery after execution was physically cut.
// The old capability must still own this exact slot; duplicate resumes are stale.
func (c *candidateController) prepareResume(name string, token candidateWorkToken, now int64) (delta *candidateDelta, err error) {
	defer finalizeCandidateDelta(&delta)
	d, err := c.prepare(name)
	if err != nil {
		return nil, err
	}
	r := d.after
	if !c.validRecord(r, token) {
		d.Decision.Ignored = true
		return d, nil
	}
	work := r.Active
	if r.Proposal != nil && r.Proposal.Token == token {
		work = r.Proposal
	}
	if r.FrameworkBlocked {
		entry := r.entry(token.Candidate)
		if r.Block == candidateBlockQuarantined || r.Block == candidateBlockFailedRetry && (!entry.HasDue || entry.Due > now) {
			d.Decision.Ignored = true
			return d, nil
		}
	}
	return d, c.resumeCandidate(d, work, now)
}

func (c *candidateController) resumeCandidate(d *candidateDelta, work *candidateWork, now int64) error {
	r := d.after
	attempt, err := c.next()
	if err != nil {
		return err
	}
	work.Token.Attempt = attempt
	if r.FrameworkBlocked {
		r.FrameworkBlocked = false
		r.Block = candidateBlockQuarantined
		r.Wake = nil
		r.Phase = candidatePreparing
		d.Decision.Wake = nil
		e := r.editEntry(work.Token.Candidate)
		e.HasDue = false
		if err := c.admit(d, e.Key, dyncfg.StatusAccepted, true); err != nil {
			return err
		}
	}
	d.Decision.Work = work
	return nil
}

func (c *candidateController) progress(d *candidateDelta, now int64) error {
	r := d.after
	if r.Enablement != candidateEnabled || r.SearchClosed || r.Manual != candidateManualNone || r.Active != nil || r.Proposal != nil || r.Phase == candidateBlocked {
		d.Decision.Wake = r.Wake
		return nil
	}
	if r.Scope == candidateSourceAlternatives && r.tierDirty {
		r.owningTier = -1
		for _, entry := range r.entries() {
			if entry.SourceOwned && candidateAuthorized(r, entry.Key) && !entry.Rejected && entry.Candidate.Config.SourceTypePriority() > r.owningTier {
				r.owningTier = entry.Candidate.Config.SourceTypePriority()
			}
		}
		r.tierDirty = false
	}
	tier := r.owningTier
	eligible := func(key candidateKey) bool {
		e := r.entry(key)
		if e == nil || e.Rejected || !candidateAuthorized(r, key) {
			return false
		}
		if r.Scope == candidateExactConfig {
			return r.Binding != nil && key == r.Binding.Key
		}
		return e.SourceOwned && e.Candidate.Config.SourceTypePriority() == tier
	}
	for {
		for r.PassCursor < len(r.Pass) {
			key := r.Pass[r.PassCursor]
			r.PassCursor++
			if !eligible(key) {
				continue
			}
			e := r.editEntry(key)
			retrying := !e.Initial && e.HasDue
			attempt, err := c.next()
			if err != nil {
				return err
			}
			work := &candidateWork{Token: candidateWorkToken{Run: c.run, Intent: r.Intent, Attempt: attempt, Candidate: key}, Config: e.Candidate.Config}
			r.Active = work
			r.Wake = nil
			r.Phase = candidatePreparing
			e.Initial = false
			e.HasDue = false
			d.Decision.Work = work
			d.Decision.Wake = nil
			// A retry of the admitted binding re-enters Accepted before slow
			// preparation. A different sibling acquires exposure only on admission.
			if retrying && r.Binding != nil && r.Binding.Key == key {
				if err := c.admit(d, key, dyncfg.StatusAccepted, r.Binding.Listed); err != nil {
					return err
				}
			}
			return nil
		}
		var initial, due []candidateKey
		var earliest int64
		hasFuture := false
		for key, e := range r.entries() {
			if !eligible(key) {
				continue
			}
			if e.Initial {
				initial = append(initial, key)
			} else if e.HasDue && e.Due <= now {
				due = append(due, key)
			} else if e.HasDue && (!hasFuture || e.Due < earliest) {
				earliest = e.Due
				hasFuture = true
			}
		}
		order := func(a, b candidateKey) int {
			if r.Binding != nil {
				if a == r.Binding.Key {
					return -1
				}
				if b == r.Binding.Key {
					return 1
				}
			}
			return compareCandidateKeys(a, b)
		}
		slices.SortFunc(initial, order)
		slices.SortFunc(due, order)
		r.Pass = append(initial, due...)
		r.PassCursor = 0
		if len(r.Pass) > 0 {
			continue
		}
		if hasFuture {
			if r.Wake == nil || r.Wake.Due != earliest {
				generation, err := c.next()
				if err != nil {
					return err
				}
				r.Wake = &candidateWake{Token: candidateWakeToken{Run: c.run, Intent: r.Intent, Schedule: generation}, Due: earliest}
			}
			r.Phase = candidateRetryWait
			d.Decision.Wake = r.Wake
		} else {
			r.Wake = nil
			r.Phase = candidateFailedManual
			d.Decision.Wake = nil
		}
		return nil
	}
}

func prepareCandidateRecord(source *candidateRecord) *candidateRecord {
	if source == nil {
		return &candidateRecord{Entries: make(map[candidateKey]*candidateEntry), tierDirty: true}
	}
	out := *source
	out.entryChanges = nil
	out.entriesOwned = false
	if source.Cursor != nil {
		key := *source.Cursor
		out.Cursor = &key
	}
	if source.Binding != nil {
		binding := *source.Binding
		out.Binding = &binding
	}
	out.Active = cloneCandidateWork(source.Active)
	out.Proposal = cloneCandidateWork(source.Proposal)
	if source.Wake != nil {
		wake := *source.Wake
		out.Wake = &wake
	}
	return &out
}

func cloneCandidateWork(work *candidateWork) *candidateWork {
	if work == nil {
		return nil
	}
	copy := *work
	if work.Replacement != nil {
		key := *work.Replacement
		copy.Replacement = &key
	}
	return &copy
}

func cloneCandidateRecord(source *candidateRecord) *candidateRecord {
	out := prepareCandidateRecord(source)
	if source == nil {
		return out
	}
	out.Entries = make(map[candidateKey]*candidateEntry, len(source.Entries))
	for key, entry := range source.entries() {
		copy := *entry
		out.Entries[key] = &copy
	}
	out.Pass = slices.Clone(source.Pass)
	return out
}

func (r *candidateRecord) entry(key candidateKey) *candidateEntry {
	if entry, changed := r.entryChanges[key]; changed {
		return entry
	}
	return r.Entries[key]
}

func (r *candidateRecord) editEntry(key candidateKey) *candidateEntry {
	entry := r.entry(key)
	if entry == nil || r.entriesOwned {
		return entry
	}
	if changed, ok := r.entryChanges[key]; ok {
		return changed
	}
	copy := *entry
	r.setEntry(key, &copy)
	return &copy
}

func (r *candidateRecord) setEntry(key candidateKey, entry *candidateEntry) {
	if r.entriesOwned {
		if entry == nil {
			delete(r.Entries, key)
		} else {
			r.Entries[key] = entry
		}
		return
	}
	if r.entryChanges == nil {
		r.entryChanges = make(map[candidateKey]*candidateEntry)
	}
	r.entryChanges[key] = entry
}

func (r *candidateRecord) removeEntry(key candidateKey) {
	r.setEntry(key, nil)
	r.tierDirty = true
}

func (r *candidateRecord) entries() iter.Seq2[candidateKey, *candidateEntry] {
	return func(yield func(candidateKey, *candidateEntry) bool) {
		for key, entry := range r.Entries {
			if changed, ok := r.entryChanges[key]; ok {
				entry = changed
			}
			if entry != nil && !yield(key, entry) {
				return
			}
		}
		for key, entry := range r.entryChanges {
			if _, exists := r.Entries[key]; !exists && entry != nil && !yield(key, entry) {
				return
			}
		}
	}
}

// Population changes are prepared off-loop and installed by one map swap.
func (r *candidateRecord) ownEntries() {
	if r.entriesOwned {
		return
	}
	owned := make(map[candidateKey]*candidateEntry, len(r.Entries)+len(r.entryChanges))
	for key, entry := range r.entries() {
		copy := *entry
		owned[key] = &copy
	}
	r.Entries = owned
	r.entryChanges = nil
	r.entriesOwned = true
	r.tierDirty = true
}

// prepareAdoptExact combines interactive acceptance and its health projection.
// The caller supplies a retained checked stage for Accepted work, so execution
// can continue under Work.Token without probing the submitted config twice.
func (c *candidateController) prepareAdoptExact(config confgroup.Config, enabled bool, health dyncfg.Status, listed bool, retryAfter int, now int64) (delta *candidateDelta, err error) {
	defer finalizeCandidateDelta(&delta)
	d, err := c.prepareExact(config, false, now)
	if err != nil {
		return nil, err
	}
	r := d.after
	r.Binding.Health = health
	r.Binding.Listed = listed
	d.Decision.Binding = r.Binding
	d.Decision.Remove = !listed
	if health == dyncfg.StatusDisabled {
		r.Enablement = candidateDisabled
		return d, nil
	}
	if !enabled {
		return d, nil
	}
	r.Enablement = candidateEnabled
	e := r.entry(r.Binding.Key)
	switch health {
	case dyncfg.StatusRunning:
		c.closeSearch(r, e.Key)
		r.Phase = candidateRunning
	case dyncfg.StatusFailed:
		e.Initial = false
		if retryAfter > 0 {
			e.HasDue = true
			e.Due = now + int64(retryAfter)
		}
		if err = c.progress(d, now); err != nil {
			return nil, err
		}
	case dyncfg.StatusAccepted:
		if err = c.progress(d, now); err != nil {
			return nil, err
		}
		r.Phase = candidateStarting
	default:
		return nil, errors.New("job output: invalid exact candidate health")
	}
	return d, nil
}
