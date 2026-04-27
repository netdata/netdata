// Package cgroups provides the public single-kind L2/L3 surface for the
// cgroups-snapshot service.
//
// Clients connect to a service kind, not to a plugin identity. One service
// endpoint serves one request kind only. The outer request code remains part
// of the envelope for validation, not public multi-method dispatch.
package cgroups

import (
	"github.com/netdata/netdata/go/plugins/pkg/netipc/protocol"
	raw "github.com/netdata/netdata/go/plugins/pkg/netipc/service/raw"
)

// ClientState represents the connection state machine.
type ClientState = raw.ClientState

const (
	StateDisconnected = raw.StateDisconnected
	StateConnecting   = raw.StateConnecting
	StateReady        = raw.StateReady
	StateNotFound     = raw.StateNotFound
	StateAuthFailed   = raw.StateAuthFailed
	StateIncompatible = raw.StateIncompatible
	StateBroken       = raw.StateBroken
)

// ClientStatus is a diagnostic counters snapshot.
type ClientStatus = raw.ClientStatus

// ClientConfig is the public L2/L3 client configuration for the
// cgroups-snapshot service.
//
// Transport-only tuning stays below the public typed API.
type ClientConfig struct {
	SupportedProfiles       uint32
	PreferredProfiles       uint32
	MaxRequestBatchItems    uint32
	MaxResponsePayloadBytes uint32
	AuthToken               uint64
}

// ServerConfig is the public typed-server configuration for the
// cgroups-snapshot service.
//
// Transport-only tuning stays below the public typed API.
type ServerConfig struct {
	SupportedProfiles       uint32
	PreferredProfiles       uint32
	MaxRequestBatchItems    uint32
	MaxResponsePayloadBytes uint32
	AuthToken               uint64
}

// SnapshotHandler is the typed callback used by the cgroups-snapshot service.
type SnapshotHandler = func(*protocol.CgroupsRequest, *protocol.CgroupsBuilder) bool

// Handler defines the public typed callback surface for the cgroups-snapshot
// service. A nil handler means the service is unavailable.
type Handler struct {
	Handle SnapshotHandler

	// SnapshotMaxItems optionally caps the number of snapshot items the
	// internal builder reserves directory space for. When zero, the library
	// derives a safe upper bound from the negotiated response buffer size.
	SnapshotMaxItems uint32
}

// CacheItem is an owned copy of a single cgroup item.
type CacheItem = raw.CacheItem

// CacheStatus is a diagnostic snapshot for the L3 cache.
type CacheStatus = raw.CacheStatus
