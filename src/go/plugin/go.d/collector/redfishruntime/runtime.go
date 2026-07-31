// SPDX-License-Identifier: GPL-3.0-or-later

// Package redfishruntime contains the process-local composition state shared by
// the Redfish endpoint and log-backend collectors.
package redfishruntime

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"maps"
	"slices"
	"strings"
	"sync"
	"time"
)

var (
	ErrBackendUnavailable = errors.New("Redfish log backend is unavailable")
)

// JournalEntry is one structured Redfish LogEntry ready for durable storage.
type JournalEntry struct {
	RealtimeUsec       uint64
	SourceRealtimeUsec uint64
	Fields             map[string]string
}

// AppendResult describes one durably synchronized append batch.
type AppendResult struct {
	Committed           int
	DuplicateSuppressed int
}

// Backend is the narrow append contract exposed to endpoint collectors.
type Backend interface {
	Append(context.Context, []JournalEntry) (AppendResult, error)
	Contains(context.Context, []string) (map[string]bool, error)
}

// InventorySnapshot is an immutable current-observation snapshot for one job.
// Rows use the closed JSON-like value set accepted by PublishInventory.
type InventorySnapshot struct {
	Job         string
	ObservedAt  time.Time
	Complete    bool
	Rows        []map[string]any
	Diagnostics []string
}

// InventoryHost is one host identity exposed by a job snapshot.
type InventoryHost struct {
	URI  string
	Name string
}

// InventoryCatalog is the bounded selector metadata for one job snapshot.
type InventoryCatalog struct {
	Job           string
	Hosts         []InventoryHost
	ResourceKinds []string
}

// Runtime owns only feature-local routing and Function snapshot state.
type Runtime struct {
	mu          sync.RWMutex
	backends    map[string]*backendEntry
	producers   map[string]map[string]struct{}
	inventories map[string]*inventoryEntry
	logRoots    map[string]string
	endpoints   map[string]*endpointEntry
}

type endpointEntry struct {
	digest string
	refs   int
}

type inventoryEntry struct {
	snapshot InventorySnapshot
	catalog  InventoryCatalog
	slices   map[inventorySliceKey][]int
}

type inventorySliceKey struct {
	host string
	kind string
}

type backendEntry struct {
	name    string
	key     string
	backend Backend

	mu      sync.Mutex
	closing bool
	refs    int
	drained chan struct{}
}

// New returns an empty feature runtime.
func New() *Runtime {
	return &Runtime{
		backends:    make(map[string]*backendEntry),
		producers:   make(map[string]map[string]struct{}),
		inventories: make(map[string]*inventoryEntry),
		logRoots:    make(map[string]string),
		endpoints:   make(map[string]*endpointEntry),
	}
}

// RegisterEndpoint protects process-local endpoint-key use against a truncated
// digest collision. Equal canonical origins may have several explicit jobs.
func (r *Runtime) RegisterEndpoint(key, fullDigest string) (func(), error) {
	if r == nil {
		return nil, errors.New("nil Redfish runtime")
	}
	if key == "" || fullDigest == "" {
		return nil, errors.New("empty Redfish endpoint identity")
	}
	r.mu.Lock()
	entry := r.endpoints[key]
	if entry != nil && entry.digest != fullDigest {
		r.mu.Unlock()
		return nil, fmt.Errorf("Redfish endpoint key %q collides with a different canonical origin", key)
	}
	if entry == nil {
		entry = &endpointEntry{digest: fullDigest}
		r.endpoints[key] = entry
	}
	entry.refs++
	r.mu.Unlock()

	var once sync.Once
	return func() {
		once.Do(func() {
			r.mu.Lock()
			if current := r.endpoints[key]; current == entry {
				current.refs--
				if current.refs == 0 {
					delete(r.endpoints, key)
				}
			}
			r.mu.Unlock()
		})
	}, nil
}

// BackendRegistration owns one registered backend route.
type BackendRegistration struct {
	runtime *Runtime
	entry   *backendEntry
	once    sync.Once
}

// RegisterBackend makes a ready backend available to endpoint jobs.
func (r *Runtime) RegisterBackend(name, key, root string, backend Backend) (*BackendRegistration, error) {
	if r == nil {
		return nil, errors.New("nil Redfish runtime")
	}
	if name == "" {
		return nil, errors.New("empty Redfish log backend name")
	}
	if key == "" {
		return nil, errors.New("empty Redfish log backend key")
	}
	if backend == nil {
		return nil, errors.New("nil Redfish log backend")
	}

	entry := &backendEntry{
		name:    name,
		key:     key,
		backend: backend,
		drained: make(chan struct{}),
	}

	r.mu.Lock()
	defer r.mu.Unlock()
	if _, ok := r.backends[name]; ok {
		return nil, fmt.Errorf("Redfish log backend %q is already registered", name)
	}
	r.backends[name] = entry
	if root != "" {
		r.logRoots[name] = root
	}
	return &BackendRegistration{runtime: r, entry: entry}, nil
}

// Close removes the route before waiting for in-flight leases to drain.
func (reg *BackendRegistration) Close(ctx context.Context) error {
	if reg == nil || reg.runtime == nil || reg.entry == nil {
		return nil
	}

	reg.once.Do(func() {
		reg.runtime.mu.Lock()
		if reg.runtime.backends[reg.entry.name] == reg.entry {
			delete(reg.runtime.backends, reg.entry.name)
			delete(reg.runtime.logRoots, reg.entry.name)
		}
		reg.runtime.mu.Unlock()

		reg.entry.mu.Lock()
		reg.entry.closing = true
		if reg.entry.refs == 0 {
			close(reg.entry.drained)
		}
		reg.entry.mu.Unlock()
	})
	select {
	case <-reg.entry.drained:
		return nil
	case <-ctx.Done():
		return ctx.Err()
	}
}

// BackendLease pins a backend while one producer append is in flight.
type BackendLease struct {
	entry *backendEntry
	once  sync.Once
}

// AcquireBackend returns a lease only for a currently ready route.
func (r *Runtime) AcquireBackend(name string) (*BackendLease, bool) {
	if r == nil {
		return nil, false
	}
	r.mu.RLock()
	entry := r.backends[name]
	if entry == nil {
		r.mu.RUnlock()
		return nil, false
	}
	entry.mu.Lock()
	if entry.closing {
		entry.mu.Unlock()
		r.mu.RUnlock()
		return nil, false
	}
	entry.refs++
	entry.mu.Unlock()
	r.mu.RUnlock()
	return &BackendLease{entry: entry}, true
}

// Append forwards one batch through the pinned route.
func (l *BackendLease) Append(ctx context.Context, entries []JournalEntry) (AppendResult, error) {
	if l == nil || l.entry == nil {
		return AppendResult{}, ErrBackendUnavailable
	}
	l.entry.mu.Lock()
	backend := l.entry.backend
	l.entry.mu.Unlock()
	return backend.Append(ctx, entries)
}

// Contains classifies record keys only after the backend has synchronized its
// current writer, so a positive result is durable duplicate evidence.
func (l *BackendLease) Contains(ctx context.Context, keys []string) (map[string]bool, error) {
	if l == nil || l.entry == nil {
		return nil, ErrBackendUnavailable
	}
	l.entry.mu.Lock()
	backend := l.entry.backend
	l.entry.mu.Unlock()
	return backend.Contains(ctx, keys)
}

// Release relinquishes the lease.
func (l *BackendLease) Release() {
	if l == nil || l.entry == nil {
		return
	}
	l.once.Do(func() {
		l.entry.mu.Lock()
		l.entry.refs--
		if l.entry.closing && l.entry.refs == 0 {
			close(l.entry.drained)
		}
		l.entry.mu.Unlock()
	})
}

// BackendAvailable reports whether the named route is currently ready.
func (r *Runtime) BackendAvailable(name string) bool {
	lease, ok := r.AcquireBackend(name)
	if ok {
		lease.Release()
	}
	return ok
}

// BackendLeaseCount returns the number of in-flight producer leases.
func (r *Runtime) BackendLeaseCount(name string) int {
	if r == nil {
		return 0
	}
	r.mu.RLock()
	entry := r.backends[name]
	if entry == nil {
		r.mu.RUnlock()
		return 0
	}
	entry.mu.Lock()
	refs := entry.refs
	entry.mu.Unlock()
	r.mu.RUnlock()
	return refs
}

// RegisterProducer records one log-enabled endpoint route independently of
// backend readiness. The returned function removes only this registration.
func (r *Runtime) RegisterProducer(backend, producer string) (func(), error) {
	if r == nil {
		return nil, errors.New("nil Redfish runtime")
	}
	if backend == "" || producer == "" {
		return nil, errors.New("empty Redfish log producer route")
	}
	r.mu.Lock()
	set := r.producers[backend]
	if set == nil {
		set = make(map[string]struct{})
		r.producers[backend] = set
	}
	if _, ok := set[producer]; ok {
		r.mu.Unlock()
		return nil, fmt.Errorf("Redfish log producer %q is already registered", producer)
	}
	set[producer] = struct{}{}
	r.mu.Unlock()

	var once sync.Once
	return func() {
		once.Do(func() {
			r.mu.Lock()
			if set := r.producers[backend]; set != nil {
				delete(set, producer)
				if len(set) == 0 {
					delete(r.producers, backend)
				}
			}
			r.mu.Unlock()
		})
	}, nil
}

// BackendProducerCount returns the number of endpoint jobs routed to a backend.
func (r *Runtime) BackendProducerCount(name string) int {
	if r == nil {
		return 0
	}
	r.mu.RLock()
	defer r.mu.RUnlock()
	return len(r.producers[name])
}

// AnyBackendAvailable reports whether at least one ready backend exists.
func (r *Runtime) AnyBackendAvailable() bool {
	if r == nil {
		return false
	}
	r.mu.RLock()
	defer r.mu.RUnlock()
	return len(r.backends) > 0
}

// LogRoots returns a stable copy of recognized active backend roots.
func (r *Runtime) LogRoots() map[string]string {
	if r == nil {
		return nil
	}
	r.mu.RLock()
	defer r.mu.RUnlock()
	return maps.Clone(r.logRoots)
}

// PublishInventory atomically replaces one job snapshot. Unsupported mutable
// row values reject the whole replacement, preserving the prior snapshot.
func (r *Runtime) PublishInventory(snapshot InventorySnapshot) error {
	if r == nil || snapshot.Job == "" {
		return nil
	}
	stored, err := cloneInventorySnapshot(snapshot)
	if err != nil {
		return err
	}
	r.mu.Lock()
	r.inventories[snapshot.Job] = buildInventoryEntry(stored)
	r.mu.Unlock()
	return nil
}

// RemoveInventory removes one stopped job from the Function catalog.
func (r *Runtime) RemoveInventory(job string) {
	if r == nil || job == "" {
		return
	}
	r.mu.Lock()
	delete(r.inventories, job)
	r.mu.Unlock()
}

// VisitInventoryCatalog visits immutable selector metadata without cloning row bodies.
func (r *Runtime) VisitInventoryCatalog(
	ctx context.Context,
	maxJobs int,
	visitJob func(string) bool,
	visitHost func(uri, name string) bool,
	visitKind func(string) bool,
) bool {
	if r == nil || maxJobs < 0 {
		return true
	}
	r.mu.RLock()
	if len(r.inventories) > maxJobs {
		r.mu.RUnlock()
		return false
	}
	entries := make([]*inventoryEntry, 0, min(len(r.inventories), 1_024))
	for _, entry := range r.inventories {
		if ctx.Err() != nil {
			r.mu.RUnlock()
			return false
		}
		entries = append(entries, entry)
	}
	r.mu.RUnlock()
	for _, entry := range entries {
		if ctx.Err() != nil {
			return false
		}
		if !visitJob(entry.catalog.Job) {
			return false
		}
		for _, host := range entry.catalog.Hosts {
			if !visitHost(host.URI, host.Name) {
				return false
			}
		}
		for _, kind := range entry.catalog.ResourceKinds {
			if !visitKind(kind) {
				return false
			}
		}
	}
	return true
}

// VisitInventorySlice visits cloned rows from one exact sorted source slice.
func (r *Runtime) VisitInventorySlice(
	ctx context.Context,
	job, host, resourceKind string,
	visit func(map[string]any) bool,
) (int, bool) {
	if r == nil || job == "" || host == "" || resourceKind == "" {
		return 0, false
	}
	r.mu.RLock()
	entry := r.inventories[job]
	r.mu.RUnlock()
	if entry == nil {
		return 0, false
	}
	indices := entry.slices[inventorySliceKey{host: host, kind: resourceKind}]
	if len(indices) == 0 {
		return 0, false
	}
	for _, index := range indices {
		if ctx.Err() != nil {
			break
		}
		row, err := cloneInventoryRow(entry.snapshot.Rows[index])
		if err != nil {
			// PublishInventory admits only values this clone supports.
			return len(indices), false
		}
		if !visit(row) {
			break
		}
	}
	return len(indices), true
}

func buildInventoryEntry(snapshot InventorySnapshot) *inventoryEntry {
	entry := &inventoryEntry{
		snapshot: snapshot,
		catalog:  buildInventoryCatalog(snapshot),
		slices:   make(map[inventorySliceKey][]int),
	}
	for index, row := range snapshot.Rows {
		key := inventorySliceKey{
			host: inventoryString(row, "host_uri"),
			kind: inventoryString(row, "resource_kind"),
		}
		if key.host != "" && key.kind != "" {
			entry.slices[key] = append(entry.slices[key], index)
		}
	}
	for key, indices := range entry.slices {
		slices.SortStableFunc(indices, func(a, b int) int {
			return strings.Compare(
				inventoryString(snapshot.Rows[a], "sort_key"),
				inventoryString(snapshot.Rows[b], "sort_key"),
			)
		})
		entry.slices[key] = indices
	}
	return entry
}

func buildInventoryCatalog(snapshot InventorySnapshot) InventoryCatalog {
	type hostKey struct {
		uri  string
		name string
	}
	hosts := make(map[hostKey]struct{})
	kinds := make(map[string]struct{})
	for _, row := range snapshot.Rows {
		if uri := inventoryString(row, "host_uri"); uri != "" {
			hosts[hostKey{uri: uri, name: inventoryString(row, "host_name")}] = struct{}{}
		}
		if kind := inventoryString(row, "resource_kind"); kind != "" {
			kinds[kind] = struct{}{}
		}
	}
	catalog := InventoryCatalog{Job: snapshot.Job}
	for host := range hosts {
		catalog.Hosts = append(catalog.Hosts, InventoryHost{URI: host.uri, Name: host.name})
	}
	slices.SortFunc(catalog.Hosts, func(a, b InventoryHost) int {
		if compared := strings.Compare(a.URI, b.URI); compared != 0 {
			return compared
		}
		return strings.Compare(a.Name, b.Name)
	})
	for kind := range kinds {
		catalog.ResourceKinds = append(catalog.ResourceKinds, kind)
	}
	slices.Sort(catalog.ResourceKinds)
	return catalog
}

func inventoryString(row map[string]any, key string) string {
	value, _ := row[key].(string)
	return value
}

func cloneInventorySnapshot(src InventorySnapshot) (InventorySnapshot, error) {
	dst := src
	dst.Diagnostics = slices.Clone(src.Diagnostics)
	dst.Rows = make([]map[string]any, len(src.Rows))
	for i, row := range src.Rows {
		cloned, err := cloneInventoryRow(row)
		if err != nil {
			return InventorySnapshot{}, fmt.Errorf("inventory row %d: %w", i, err)
		}
		dst.Rows[i] = cloned
	}
	return dst, nil
}

func cloneInventoryRow(src map[string]any) (map[string]any, error) {
	dst := make(map[string]any, len(src))
	for key, value := range src {
		cloned, err := cloneInventoryValue(value)
		if err != nil {
			return nil, fmt.Errorf("field %q: %w", key, err)
		}
		dst[key] = cloned
	}
	return dst, nil
}

func cloneInventoryValue(value any) (any, error) {
	switch value := value.(type) {
	case nil, bool, string,
		int, int8, int16, int32, int64,
		uint, uint8, uint16, uint32, uint64,
		float32, float64, json.Number:
		return value, nil
	case map[string]any:
		return cloneInventoryRow(value)
	case []any:
		result := make([]any, len(value))
		for i := range value {
			cloned, err := cloneInventoryValue(value[i])
			if err != nil {
				return nil, fmt.Errorf("array item %d: %w", i, err)
			}
			result[i] = cloned
		}
		return result, nil
	case []string:
		return slices.Clone(value), nil
	case *bool:
		if value == nil {
			return (*bool)(nil), nil
		}
		copy := *value
		return &copy, nil
	case *int:
		if value == nil {
			return (*int)(nil), nil
		}
		copy := *value
		return &copy, nil
	case *int64:
		if value == nil {
			return (*int64)(nil), nil
		}
		copy := *value
		return &copy, nil
	case *float64:
		if value == nil {
			return (*float64)(nil), nil
		}
		copy := *value
		return &copy, nil
	case *string:
		if value == nil {
			return (*string)(nil), nil
		}
		copy := *value
		return &copy, nil
	default:
		return nil, fmt.Errorf("unsupported mutable inventory value type %T", value)
	}
}
