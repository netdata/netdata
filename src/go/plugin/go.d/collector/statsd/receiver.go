// SPDX-License-Identifier: GPL-3.0-or-later

package statsd

import (
	"strings"
	"sync"
	"time"

	"github.com/netdata/netdata/go/plugins/pkg/metrix"
)

// identity is a final name plus its canonical application labels.
type identity struct{ name, labels string }

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

// series is one admitted identity. value holds counter and gauge state; ms, h
// and s observations accumulate in window while spare awaits reuse after the
// detached batch is released.
type series struct {
	id            identity
	meta          *declaration
	labels        []metrix.Label
	lastInput     time.Time
	pending       bool
	value         float64
	window, spare *interval
}

// Receiver ownership is limited to maxSeries entries and one Collect-owned
// batch of at most maxSeries entries. The framework serializes Collect calls.
type receiver struct {
	mu                       sync.Mutex
	running                  bool
	capacity                 int
	idle                     time.Duration
	entries                  map[identity]*series
	bindings                 map[string]typeBinding
	metadata                 map[declarationKey]*declaration
	retention                metrix.DescriptorRetention
	horizon, priorCutSuccess uint64

	// profiles are in precedence order. membership records the native profiles
	// activated by admitted input; it only grows until restart.
	profiles   []*profile
	membership []bool
	activated  int // set entries in membership
	templated  int // profiles with templates; activation stops scanning once all are active

	accepted uint64
	rejects  [len(rejectReasons)]uint64
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

func (r *receiver) ingestLocked(line string, now time.Time) error {
	input, err := parseRecord(line)
	if err != nil {
		return err
	}
	original := input.name
	if input, err = r.preprocess(input); err != nil {
		return err
	}
	p, err := prepareRecord(input)
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
// pipelines never chain.
func (r *receiver) preprocess(input record) (record, error) {
	for _, p := range r.profiles {
		if p.owns(input.name) {
			return p.replace(input)
		}
	}
	return input, nil
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
	id := identity{p.name, p.labelKey}
	e := r.entries[id]
	if r.expiryMayMatter(e, p.name, p.kind, now) {
		r.retireIdle(now)
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
		e = r.addSeries(id, meta, p)
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

// retireIdle removes expired entries without input awaiting handoff.
func (r *receiver) retireIdle(now time.Time) {
	for _, e := range r.entries {
		if !e.pending && r.expired(e, now) {
			r.remove(e)
		}
	}
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
func (r *receiver) addSeries(id identity, meta *declaration, p preparedRecord) *series {
	id.name = meta.key.name
	e := &series{
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
	return e
}
