// SPDX-License-Identifier: GPL-3.0-or-later

// Package registry coordinates admitted RUM site generations and receiver availability. Desired
// configuration, startup, retries and job status belong to the Agent framework.
package registry

import (
	"context"
	"errors"
	"sync"

	redact "github.com/netdata/netdata/go/plugins/plugin/dem/internal/redact"
	"github.com/netdata/netdata/go/plugins/plugin/dem/rum/aggregate"
	"github.com/netdata/netdata/go/plugins/plugin/dem/rum/diagnostics"
	"github.com/netdata/netdata/go/plugins/plugin/dem/rum/geoip"
	"github.com/netdata/netdata/go/plugins/plugin/dem/rum/httpapi"
)

type Site struct {
	Route       *httpapi.Route
	Diagnostics *diagnostics.State
	Aggregator  *aggregate.Aggregator
	Generation  string
	Redactor    *redact.Redactor
}
type Availability struct {
	// Listen is the actual bound socket, never an advertised browser address.
	Listen    string
	Serving   bool
	PublicURL string
	GeoIP     geoip.Status
}
type registration struct {
	data   *Site
	ctx    context.Context
	cancel context.CancelFunc
	users  sync.WaitGroup
}
type receiver struct{ availability Availability }

type Registry struct {
	mu           sync.Mutex
	sites        map[string]*registration
	receiver     *receiver
	lastReceiver Availability
}

func New() *Registry {
	return &Registry{
		sites: map[string]*registration{},
		lastReceiver: Availability{
			GeoIP: geoip.Status{
				State:  "unavailable",
				Reason: "receiver_stopped",
			},
		},
	}
}

// Register begins admission for one exact runtime. Retire revokes admission,
// interrupts its HTTP producers and waits for every admitted reader to release.
// Waiting never holds the registry mutex or blocks admission to other sites.
func (h *Registry) Register(key string, site *Site) (func(), error) {
	h.mu.Lock()
	if _, exists := h.sites[key]; exists {
		h.mu.Unlock()
		return nil, errors.New("site runtime already registered")
	}
	ctx, cancel := context.WithCancel(context.Background())
	entry := &registration{
		data:   site,
		ctx:    ctx,
		cancel: cancel,
	}
	h.sites[key] = entry
	h.mu.Unlock()
	return func() {
		h.mu.Lock()
		if h.sites[key] == entry {
			delete(h.sites, key)
		}
		h.mu.Unlock()
		entry.cancel()
		entry.users.Wait()
	}, nil
}
func (h *Registry) AcquireSite(key string) (*Site, context.Context, func(), bool) {
	h.mu.Lock()
	defer h.mu.Unlock()
	entry, ok := h.sites[key]
	if !ok {
		return nil, nil, nil, false
	}
	entry.users.Add(1)
	return entry.data, entry.ctx, entry.users.Done, true
}
func (h *Registry) Acquire(key string) (*httpapi.Route, context.Context, func(), bool) {
	data, ctx, release, ok := h.AcquireSite(key)
	if !ok {
		return nil, nil, nil, false
	}
	return data.Route, ctx, release, true
}
func (h *Registry) Keys() []string {
	h.mu.Lock()
	defer h.mu.Unlock()
	keys := make([]string, 0, len(h.sites))
	for key := range h.sites {
		keys = append(keys, key)
	}
	return keys
}

// ReceiverRegistration fences diagnostic updates and retirement to one receiver.
type ReceiverRegistration struct {
	registry *Registry
	entry    *receiver
}

func (h *Registry) PublishReceiver(state Availability) *ReceiverRegistration {
	entry := &receiver{
		availability: state,
	}
	h.mu.Lock()
	h.receiver = entry
	h.lastReceiver = state
	h.mu.Unlock()
	return &ReceiverRegistration{
		registry: h,
		entry:    entry,
	}
}
func (r *ReceiverRegistration) SetGeoIP(status geoip.Status) {
	h := r.registry
	h.mu.Lock()
	defer h.mu.Unlock()
	if h.receiver == r.entry {
		r.entry.availability.GeoIP = status
		h.lastReceiver = r.entry.availability
	}
}
func (r *ReceiverRegistration) Close() {
	h := r.registry
	h.mu.Lock()
	defer h.mu.Unlock()
	if h.receiver == r.entry {
		h.receiver = nil
		h.lastReceiver.Serving = false
		h.lastReceiver.GeoIP = geoip.Status{
			Selection: h.lastReceiver.GeoIP.Selection,
			State:     "unavailable",
			Reason:    "receiver_stopped",
		}
	}
}
func (h *Registry) Availability() Availability {
	h.mu.Lock()
	defer h.mu.Unlock()
	if h.receiver != nil {
		return h.receiver.availability
	}
	return h.lastReceiver
}
