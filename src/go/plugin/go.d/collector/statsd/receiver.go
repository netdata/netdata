// SPDX-License-Identifier: GPL-3.0-or-later

package statsd

import (
	"strings"
	"sync"
	"time"

	"github.com/netdata/netdata/go/plugins/pkg/metrix"
)

type declarationKey struct {
	name string
	kind wireType
}

// declaration is the name-wide presentation fixed by the first admitted record.
// refs counts receiver and detached-batch references; committed, staged and
// lastWrite track whether its last metric write was committed.
type declaration struct {
	key declarationKey
	presentation
	encodedName       string
	refs              int
	committed, staged bool
	lastWrite         uint64
}

// typeBinding fixes one wire type per final name while any series uses it.
type typeBinding struct {
	kind wireType
	refs int
}

// series is one admitted identity: a final name plus its canonical application
// labels, keyed by id (see prepareRecord). value holds counter and gauge state;
// ms, h and s observations accumulate in window while spare awaits reuse after
// the detached batch is released.
type series struct {
	id            string
	meta          *declaration
	labels        []metrix.Label
	lastInput     time.Time
	pending       bool
	value         float64
	window, spare *interval
	out           *instruments // Collect-owned
}

// Receiver ownership is limited to maxSeries entries and one Collect-owned
// batch of at most maxSeries entries. The framework serializes Collect calls.
type receiver struct {
	mu                       sync.Mutex
	running                  bool
	capacity                 int
	idle                     time.Duration
	entries                  map[string]*series
	bindings                 map[string]typeBinding
	metadata                 map[declarationKey]*declaration
	retention                metrix.DescriptorRetention
	horizon, priorCutSuccess uint64

	// retireAt is the earliest idle expiry of an entry without pending input, zero
	// when there is none. Only a cut clears pending input, and it recomputes the
	// bound; between cuts entries only gain input, so admission skips its idle scan
	// until retireAt.
	retireAt time.Time

	// profiles are in precedence order. membership records the native profiles
	// activated by admitted input; it only grows until restart.
	profiles   []*profile
	membership []bool
	activated  int // set entries in membership
	templated  int // profiles with templates; activation stops scanning once all are active

	accepted uint64
	rejects  [len(rejectReasons)]uint64

	// Record storage reused under the lock; records needing more grow on the heap.
	tagBuf, replaceBuf, labelBuf [16]metrix.Label
	idBuf                        [512]byte
}

// lifetime is the longest prepared chart or dimension expiry in successful cycles.
func newReceiver(
	capacity int,
	idle time.Duration,
	retention metrix.DescriptorRetention,
	lifetime uint64,
	profiles []*profile,
) *receiver {
	r := &receiver{
		capacity:   capacity,
		idle:       idle,
		entries:    make(map[string]*series),
		bindings:   make(map[string]typeBinding),
		metadata:   make(map[declarationKey]*declaration),
		retention:  retention,
		horizon:    max(retention.DescriptorRetentionWindow(), lifetime+1),
		profiles:   profiles,
		membership: make([]bool, len(profiles)),
	}
	for _, p := range profiles {
		if p.groups != nil {
			r.templated++
		}
	}
	return r
}

// start is called only after the required receiver resources are acquired.
func (r *receiver) start() { r.mu.Lock(); defer r.mu.Unlock(); r.running = true }

func (r *receiver) stop() { r.mu.Lock(); defer r.mu.Unlock(); r.running = false }

// ingest owns preparation and mutation under one lock, including the selected
// profile's replace pipeline, whose compiled processors own mutable scratch.
func (r *receiver) ingest(line string, now time.Time) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	if !r.running {
		return rejectUnavailable
	}
	err := r.ingestLocked(line, now)
	if err == nil {
		r.accepted++
	} else if rejected, ok := err.(rejection); ok {
		r.countRejection(rejected)
	}
	return err
}

// ingestLocked parses and prepares into receiver-owned storage and clears the
// labels it wrote before returning, so the storage never retains a receive
// record. Admission clones everything it keeps.
func (r *receiver) ingestLocked(line string, now time.Time) error {
	input, err := parseRecord(line, r.tagBuf[:0])
	defer clear(r.tagBuf[:min(len(input.labels), len(r.tagBuf))])
	if err != nil {
		return err
	}
	original := input.name
	input, replaced, err := r.preprocess(input)
	if replaced {
		defer clear(r.replaceBuf[:min(len(input.labels), len(r.replaceBuf))])
	}
	if err != nil {
		return err
	}
	p, err := prepareRecord(input, r.labelBuf[:0], r.idBuf[:0])
	defer clear(r.labelBuf[:min(len(p.labels), len(r.labelBuf))])
	if err != nil {
		return err
	}
	if err := r.admit(p, now); err != nil {
		return err
	}
	r.activate(original)
	return nil
}

// preprocess runs the one replace pipeline chosen by the original name;
// pipelines never chain. replaced reports whether a pipeline ran.
func (r *receiver) preprocess(input record) (_ record, replaced bool, _ error) {
	for _, p := range r.profiles {
		if p.owns(input.name) {
			input, err := p.replace(input, r.replaceBuf[:0])
			return input, true, err
		}
	}
	return input, false, nil
}

// activate adds every native profile whose root matches the original name of a
// fully admitted record.
func (r *receiver) activate(original string) {
	if r.activated == r.templated {
		return
	}
	for i, p := range r.profiles {
		if p.groups != nil && !r.membership[i] && p.root.MatchString(original) {
			r.membership[i] = true
			r.activated++
		}
	}
}

// reject counts input refused before record ingestion: framing and connection limits.
func (r *receiver) reject(reason rejection) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.countRejection(reason)
}

func (r *receiver) countRejection(reason rejection) {
	for i, known := range rejectReasons {
		if known == reason {
			r.rejects[i]++
			return
		}
	}
}

// admit requires the receiver lock and a fully prepared record. Every check
// runs before any mutation, so a rejected record changes nothing.
func (r *receiver) admit(p preparedRecord, now time.Time) error {
	e := r.entries[string(p.id)]
	if r.expiryMayMatter(e, p.name, p.kind, now) {
		r.retireIdle(now)
		e = r.entries[string(p.id)]
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
	meta := r.metadata[declarationKey{p.name, p.kind}]
	if meta != nil && meta.presentation.conflicts(p.metadata) {
		return rejectMetadata
	}
	value, count, sum, err := prospectiveUpdate(p.record, e)
	if err != nil {
		return err
	}
	if meta == nil {
		meta = r.declare(p)
	}
	if e == nil {
		e = r.addSeries(meta, p)
	}
	e.update(p.record, value, count, sum)
	e.lastInput, e.pending = now, true
	return nil
}

// expiryMayMatter reports whether retiring idle entries could change this
// admission: the entry itself expired, or a new identity needs a free slot or
// the release of a binding to another wire type. Collect normally retires
// entries; with idle expiry disabled nothing can expire.
func (r *receiver) expiryMayMatter(e *series, name string, kind wireType, now time.Time) bool {
	if r.idle <= 0 {
		return false
	}
	if e != nil {
		return r.expired(e, now)
	}
	b, bound := r.bindings[name]
	return len(r.entries) >= r.capacity || bound && b.kind != kind
}

func (r *receiver) expired(e *series, now time.Time) bool {
	return r.idle > 0 && now.Sub(e.lastInput) >= r.idle
}

// retireIdle removes expired entries without input awaiting handoff. It scans
// only once the earliest such entry can have expired.
func (r *receiver) retireIdle(now time.Time) {
	if r.retireAt.IsZero() || now.Before(r.retireAt) {
		return
	}
	r.retireAt = time.Time{}
	for _, e := range r.entries {
		switch {
		case e.pending:
		case r.expired(e, now):
			r.remove(e)
		default:
			r.boundRetirement(e)
		}
	}
}

// boundRetirement lowers retireAt to the idle expiry of an entry without pending input.
func (r *receiver) boundRetirement(e *series) {
	if at := e.lastInput.Add(r.idle); r.retireAt.IsZero() || at.Before(r.retireAt) {
		r.retireAt = at
	}
}

func (r *receiver) remove(e *series) {
	delete(r.entries, e.id)
	e.meta.refs--
	name := e.meta.key.name
	b := r.bindings[name]
	b.refs--
	if b.refs == 0 {
		delete(r.bindings, name)
	} else {
		r.bindings[name] = b
	}
}

// declare fixes the name-wide presentation, defaults included, from the first
// admitted record. Retained strings are cloned so they cannot pin the input.
func (r *receiver) declare(p preparedRecord) *declaration {
	m := defaultPresentation(p)
	meta := &declaration{
		key:          declarationKey{strings.Clone(p.name), p.kind},
		presentation: presentation{strings.Clone(m.unit), strings.Clone(m.title), strings.Clone(m.family)},
		encodedName:  encodeName(p.name),
	}
	r.metadata[meta.key] = meta
	return meta
}

// addSeries admits a new identity and binds its name to the wire type.
func (r *receiver) addSeries(meta *declaration, p preparedRecord) *series {
	e := &series{
		id:     string(p.id),
		meta:   meta,
		labels: make([]metrix.Label, len(p.labels)),
	}
	for i, l := range p.labels {
		e.labels[i] = metrix.Label{
			Key:   strings.Clone(l.Key),
			Value: strings.Clone(l.Value),
		}
	}
	r.entries[e.id] = e
	meta.refs++
	b := r.bindings[meta.key.name]
	b.kind = p.kind
	b.refs++
	r.bindings[meta.key.name] = b
	return e
}
