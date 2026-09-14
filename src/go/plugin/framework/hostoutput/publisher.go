// SPDX-License-Identifier: GPL-3.0-or-later

// Package hostoutput owns host definitions for one plugin output stream.
package hostoutput

import (
	"bytes"
	"errors"
	"maps"
	"sync"

	"github.com/google/uuid"
	"github.com/netdata/netdata/go/plugins/pkg/netdataapi"
	"github.com/netdata/netdata/go/plugins/plugin/framework/chartemit"
)

// Definition is immutable, validated metadata prepared outside output admission.
type Definition struct {
	info netdataapi.HostInfo
	wire []byte
}

func NewDefinition(info netdataapi.HostInfo) (*Definition, error) {
	info.GUID = GUIDKey(info.GUID)
	prepared, err := chartemit.PrepareHostInfo(info)
	if err != nil {
		return nil, err
	}
	var buf bytes.Buffer
	netdataapi.New(&buf).HOSTINFO(prepared)
	return &Definition{
		info: prepared,
		wire: buf.Bytes(),
	}, nil
}

// GUIDKey compares UUID aliases without changing unrelated generated identifiers.
func GUIDKey(guid string) string {
	if len(guid) == 32 || len(guid) == 36 {
		if parsed, err := uuid.Parse(guid); err == nil {
			return parsed.String()
		}
	}
	return guid
}

func (d *Definition) Info() netdataapi.HostInfo {
	if d == nil {
		return netdataapi.HostInfo{}
	}
	info := d.info
	info.Labels = maps.Clone(info.Labels)
	return info
}

func (d *Definition) Equal(other *Definition) bool {
	return d == other || d != nil && other != nil && bytes.Equal(d.wire, other.wire)
}

func (d *Definition) suppressCleanup() bool {
	return d != nil && d.info.Labels["_node_stale_after_seconds"] != "" &&
		d.info.Labels["_node_stale_after_seconds"] != "0"
}

// Transaction settles chart state together with its output.
type Transaction interface {
	Commit() error
	Abort() error
}

type Lookup func(string) *Definition

type Publisher struct {
	mu     sync.Mutex
	lookup Lookup
	hosts  map[string]*host
}

type host struct {
	owners      map[*Owner]struct{}
	configured  *Definition // last successfully published configured definition
	reported    map[string]struct{}
	reportOrder []string
}

// Owner identifies a runtime incarnation, not a reusable job or scope name.
type Owner struct {
	publisher *Publisher
	guid      string
	closed    bool
	observed  *Definition
	// Preparation is serialized by the owning runtime, outside frame admission.
	raw      netdataapi.HostInfo
	prepared *Definition
}

func New() *Publisher {
	return &Publisher{
		hosts: make(map[string]*host),
	}
}

// Bind installs an activated run's configuration without discarding publication history.
func (p *Publisher) Bind(lookup Lookup) { p.mu.Lock(); p.lookup = lookup; p.mu.Unlock() }

func (p *Publisher) NewOwner(guid string) *Owner {
	return &Owner{
		publisher: p,
		guid:      GUIDKey(guid),
	}
}

func (o *Owner) Prepare(info netdataapi.HostInfo) (*Definition, error) {
	if o.prepared != nil && info.GUID == o.raw.GUID && info.Hostname == o.raw.Hostname &&
		maps.Equal(info.Labels, o.raw.Labels) {
		return o.prepared, nil
	}
	d, err := NewDefinition(info)
	if err != nil {
		return nil, err
	}
	if d.info.GUID != o.guid {
		return nil, errors.New("hostoutput: owner GUID differs from definition")
	}
	o.raw = info
	o.raw.Labels = maps.Clone(info.Labels)
	o.prepared = d
	return d, nil
}

func (o *Owner) Release() {
	if o == nil {
		return
	}
	p := o.publisher
	p.mu.Lock()
	defer p.mu.Unlock()
	if o.closed {
		return
	}
	o.closed = true
	h := p.hosts[o.guid]
	if h == nil {
		return
	}
	delete(h.owners, o)
	if len(h.owners) == 0 {
		delete(p.hosts, o.guid)
	}
}

// Len reports retained host lifetimes, including quiet contributors.
func (p *Publisher) Len() int { p.mu.Lock(); defer p.mu.Unlock(); return len(p.hosts) }

// OwnerCount reports handles for a GUID registered by publication admission.
func (p *Publisher) OwnerCount(guid string) int {
	p.mu.Lock()
	defer p.mu.Unlock()
	if h := p.hosts[GUIDKey(guid)]; h != nil {
		return len(h.owners)
	}
	return 0
}

// Request contains a rendered host batch and an optional global-host tail.
// Cleanup may suppress the host batch, but never the global tail.
type Request struct {
	Owner      *Owner
	Definition *Definition
	Payload    []byte
	Tail       []byte
	Cleanup    bool
}

type Publication struct {
	request    Request
	state      Transaction
	selected   *Definition
	configured bool
	built      bool
	Conflict   *Definition
}

func Prepare(request Request, state Transaction) *Publication {
	return &Publication{
		request: request,
		state:   state,
	}
}

// Build must run inside the writer's existing frame admission.
func (t *Publication) Build() ([]byte, error) {
	r := t.request
	if r.Owner == nil {
		t.built = true
		return join(nil, r.Payload, r.Tail), nil
	}
	p := r.Owner.publisher
	p.mu.Lock()
	defer p.mu.Unlock()
	if r.Owner.closed {
		return nil, errors.New("hostoutput: retired output owner")
	}
	selected := r.Definition
	if p.lookup != nil {
		if configured := p.lookup(r.Owner.guid); configured != nil {
			selected = configured
			t.configured = true
		}
	}
	t.selected = selected
	t.built = true
	if r.Cleanup {
		if selected.suppressCleanup() {
			return r.Tail, nil
		}
		return join(nil, r.Payload, r.Tail), nil
	}
	if len(r.Payload) == 0 {
		return r.Tail, nil
	}
	if selected == nil {
		return nil, errors.New("hostoutput: missing host definition")
	}
	h := p.hosts[r.Owner.guid]
	if h == nil {
		h = &host{
			owners: make(map[*Owner]struct{}),
		}
		p.hosts[r.Owner.guid] = h
	}
	h.owners[r.Owner] = struct{}{}
	define := !r.Owner.observed.Equal(r.Definition)
	if t.configured {
		// VNodeConfiguration.Definition returns cached immutable definitions and reuses
		// equal pointers, keeping this change check constant-time under frame admission.
		define = h.configured != selected
	} else if h.configured != nil {
		define = true
	}
	if define {
		return join(selected.wire, r.Payload, r.Tail), nil
	}
	return join(nil, r.Payload, r.Tail), nil
}

func (t *Publication) Commit() error {
	if !t.built {
		return errors.New("hostoutput: publication was not built")
	}
	if t.state != nil {
		if err := t.state.Commit(); err != nil {
			return err
		}
	}
	r := t.request
	if r.Owner == nil || r.Cleanup || len(r.Payload) == 0 {
		return nil
	}
	p := r.Owner.publisher
	p.mu.Lock()
	defer p.mu.Unlock()
	if r.Owner.closed {
		return nil
	}
	h := p.hosts[r.Owner.guid]
	changed := !r.Owner.observed.Equal(r.Definition)
	r.Owner.observed = r.Definition
	if t.configured {
		h.configured = t.selected
		return nil
	}
	h.configured = nil
	if changed {
		for other := range h.owners {
			if other != r.Owner && other.observed != nil && !other.observed.Equal(r.Definition) {
				if h.markReported(string(r.Definition.wire)) {
					t.Conflict = other.observed
				}
				break
			}
		}
	}
	return nil
}

func (t *Publication) Abort() error {
	var err error
	if t.state != nil {
		err = t.state.Abort()
	}
	if o := t.request.Owner; o != nil {
		p := o.publisher
		p.mu.Lock()
		if o.observed == nil {
			if h := p.hosts[o.guid]; h != nil {
				delete(h.owners, o)
				if len(h.owners) == 0 {
					delete(p.hosts, o.guid)
				}
			}
		}
		p.mu.Unlock()
	}
	return err
}

// Preserve the diagnostic registry's bounded suppression of repeated metadata conflicts.
func (h *host) markReported(key string) bool {
	if h.reported == nil {
		h.reported = make(map[string]struct{})
	}
	if _, ok := h.reported[key]; ok {
		return false
	}
	const maxReportedStates = 64
	if len(h.reportOrder) == maxReportedStates {
		delete(h.reported, h.reportOrder[0])
		copy(h.reportOrder, h.reportOrder[1:])
		h.reportOrder = h.reportOrder[:len(h.reportOrder)-1]
	}
	h.reported[key] = struct{}{}
	h.reportOrder = append(h.reportOrder, key)
	return true
}

func join(prefix, body, tail []byte) []byte {
	if len(prefix) == 0 && len(tail) == 0 {
		return body
	}
	if len(prefix) == 0 && len(body) == 0 {
		return tail
	}
	payload := make([]byte, 0, len(prefix)+len(body)+len(tail))
	payload = append(payload, prefix...)
	payload = append(payload, body...)
	return append(payload, tail...)
}
