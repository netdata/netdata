// SPDX-License-Identifier: GPL-3.0-or-later

package statsd

import (
	"slices"
	"strings"
	"sync"
	"time"

	"github.com/netdata/netdata/go/plugins/pkg/metrix"
)

type identity struct{ name, labels string }
type declarationKey struct {
	name string
	kind wireType
}
type declaration struct {
	key declarationKey
	presentation
	encodedName       string
	refs              int
	committed, staged bool
	lastWrite         uint64
}
type typeBinding struct {
	kind wireType
	refs int
}
type series struct {
	id            identity
	meta          *declaration
	labels        []metrix.Label
	lastInput     time.Time
	pending       bool
	value         float64
	window, spare *interval
}
type measurement struct {
	owner  *series
	value  float64
	window *interval
}

// Receiver ownership is limited to maxSeries entries and one Collect-owned
// batch of at most maxSeries entries. The framework serializes Collect calls.
type receiver struct {
	mu                       sync.Mutex
	active                   bool
	capacity                 int
	idle                     time.Duration
	entries                  map[identity]*series
	bindings                 map[string]typeBinding
	metadata                 map[declarationKey]*declaration
	retention                metrix.DescriptorRetention
	horizon, priorCutSuccess uint64

	// Configured profiles, in precedence order. membership records native
	// profiles activated by admitted input; it only grows until restart.
	profiles    []*profile
	membership  []bool
	activated   int
	activatable int

	accepted uint64
	rejects  [len(rejectReasons)]uint64
}

// cutResult is one coherent handoff: the batch, the desired profile membership
// captured with the input that activated it, and receiver diagnostics.
type cutResult struct {
	batch      []measurement
	membership []bool // nil when unchanged since the caller's last build
	activated  int
	accepted   uint64
	rejects    [len(rejectReasons)]uint64
	series     int
}

// lifetime is the longest prepared chart or dimension expiry in successful cycles.
func newReceiver(capacity int, idle time.Duration, retention metrix.DescriptorRetention, lifetime uint64, profiles []*profile) *receiver {
	r := &receiver{
		capacity:   capacity,
		idle:       idle,
		entries:    make(map[identity]*series),
		bindings:   make(map[string]typeBinding),
		metadata:   make(map[declarationKey]*declaration),
		retention:  retention,
		horizon:    max(retention.DescriptorRetentionWindow(), lifetime+1),
		profiles:   profiles,
		membership: make([]bool, len(profiles)),
	}
	for _, p := range profiles {
		if p.groups != nil {
			r.activatable++
		}
	}
	return r
}

// start is called only after the required receiver resources are acquired.
func (r *receiver) start() { r.mu.Lock(); defer r.mu.Unlock(); r.active = true }
func (r *receiver) stop()  { r.mu.Lock(); defer r.mu.Unlock(); r.active = false }

func (r *receiver) expired(e *series, now time.Time) bool {
	return r.idle > 0 && now.Sub(e.lastInput) >= r.idle
}

func (r *receiver) remove(e *series) {
	delete(r.entries, e.id)
	e.meta.refs--
	b := r.bindings[e.id.name]
	b.refs--
	if b.refs == 0 {
		delete(r.bindings, e.id.name)
	} else {
		r.bindings[e.id.name] = b
	}
}

// ingest owns preparation and mutation under one lock, including the selected
// profile's replace pipeline, whose compiled processors own mutable scratch.
func (r *receiver) ingest(line string, now time.Time) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	if !r.active {
		return rejectUnavailable
	}
	err := r.ingestLocked(line, now)
	if err == nil {
		r.accepted++
	} else if rejected, ok := err.(rejection); ok {
		r.countLocked(rejected)
	}
	return err
}

func (r *receiver) ingestLocked(line string, now time.Time) error {
	input, err := parseRecord(line)
	if err != nil {
		return err
	}
	original := input.name
	// Original names choose the single preprocessing owner; pipelines never chain.
	for _, p := range r.profiles {
		if p.replaces(original) {
			if input, err = p.replace(input); err != nil {
				return err
			}
			break
		}
	}
	p, err := prepareRecord(input)
	if err != nil {
		return err
	}
	if err := r.admit(p, now); err != nil {
		return err
	}
	// Only fully admitted input activates native profiles.
	if r.activated < r.activatable {
		for i, p := range r.profiles {
			if p.groups != nil && !r.membership[i] && p.root.MatchString(original) {
				r.membership[i] = true
				r.activated++
			}
		}
	}
	return nil
}

// reject counts input refused before record ingestion: framing and connection limits.
func (r *receiver) reject(reason rejection) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.countLocked(reason)
}

func (r *receiver) countLocked(reason rejection) {
	for i, known := range rejectReasons {
		if known == reason {
			r.rejects[i]++
			return
		}
	}
}

// admit requires the receiver lock and a fully prepared record.
func (r *receiver) admit(p preparedRecord, now time.Time) error {
	id := identity{p.name, p.labelKey}
	e := r.entries[id]
	// Collect normally retires entries. On receive, scan only when expiry could
	// release a needed slot/type or invalidate the requested gauge baseline.
	if e != nil && r.expired(e, now) || e == nil && (len(r.entries) >= r.capacity || r.bindings[p.name].refs > 0) {
		for _, old := range r.entries {
			if !old.pending && r.expired(old, now) {
				r.remove(old)
			}
		}
		e = r.entries[id]
	}
	if b, ok := r.bindings[p.name]; ok && b.kind != p.kind {
		return rejectType
	}
	if p.kind == gauge && p.delta && (e == nil || r.expired(e, now)) {
		return rejectBaseline
	}
	if e == nil && len(r.entries) >= r.capacity {
		return rejectCapacity
	}
	key := declarationKey{p.name, p.kind}
	meta := r.metadata[key]
	if meta != nil && meta.presentation.conflicts(p.metadata) {
		return rejectMetadata
	}
	value, count, sum, err := prospectiveUpdate(p.record, e)
	if err != nil {
		return err
	}
	if meta == nil {
		key.name = strings.Clone(p.name)
		m := defaultPresentation(p)
		meta = &declaration{
			key:          key,
			presentation: presentation{strings.Clone(m.unit), strings.Clone(m.title), strings.Clone(m.family)},
			encodedName:  encodeName(p.name),
		}
		r.metadata[key] = meta
	}
	if e == nil {
		id.name = meta.key.name
		e = &series{
			id:     id,
			meta:   meta,
			labels: make([]metrix.Label, len(p.labels)),
		}
		for i, l := range p.labels {
			e.labels[i] = metrix.Label{
				Key:   strings.Clone(l.Key),
				Value: strings.Clone(l.Value),
			}
		}
		r.entries[id] = e
		meta.refs++
		b := r.bindings[id.name]
		b.kind = p.kind
		b.refs++
		r.bindings[id.name] = b
	}
	e.update(p.record, value, count, sum)
	e.lastInput, e.pending = now, true
	return nil
}

func (r *receiver) reconcileMetadata() {
	success := r.retention.SuccessfulCommits()
	for key, meta := range r.metadata {
		if meta.staged {
			meta.staged = false
			if success > r.priorCutSuccess {
				meta.committed = true
				meta.lastWrite = success
			}
		}
		if meta.refs == 0 && (!meta.committed || success-meta.lastWrite >= r.horizon) {
			delete(r.metadata, key)
		}
	}
	r.priorCutSuccess = success
}

// cut detaches the batch. built is the membership count the caller last
// published; a changed count returns a membership copy with this batch.
func (r *receiver) cut(now time.Time, built int) (cutResult, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	if !r.active {
		return cutResult{}, rejectUnavailable
	}
	// Only a later Collect knows whether the previous staged cycle committed.
	// Incoming traffic must retain its metadata authority until this point.
	r.reconcileMetadata()
	batch := make([]measurement, 0, len(r.entries))
	for _, e := range r.entries {
		expired := r.expired(e, now)
		if !expired || e.pending {
			e.meta.refs++
			batch = append(batch, measurement{e, e.value, e.window})
			e.window, e.spare = e.spare, nil
			e.pending = false
		}
		if expired {
			r.remove(e)
		}
	}
	result := cutResult{
		batch:     batch,
		activated: r.activated,
		accepted:  r.accepted,
		rejects:   r.rejects,
		series:    len(r.entries),
	}
	if r.activated != built {
		result.membership = slices.Clone(r.membership)
	}
	return result, nil
}

func (r *receiver) release(batch []measurement) {
	// Clear detached data before returning it to the receiver. New input can
	// continue in its other window while Collect queries/writes this batch.
	for _, m := range batch {
		if m.window != nil {
			m.window.reset()
		}
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	for _, m := range batch {
		e := m.owner
		e.meta.staged = true
		e.meta.refs--
		if r.entries[e.id] == e {
			e.spare = m.window
		}
	}
}
