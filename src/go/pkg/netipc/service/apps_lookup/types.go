// Package apps_lookup provides the public single-kind L2 surface for the
// apps-lookup service.
package apps_lookup

import (
	"github.com/netdata/netdata/go/plugins/pkg/netipc/protocol"
	raw "github.com/netdata/netdata/go/plugins/pkg/netipc/service/raw"
)

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

type ClientStatus = raw.ClientStatus

type ClientConfig struct {
	SupportedProfiles       uint32
	PreferredProfiles       uint32
	MaxRequestBatchItems    uint32
	MaxResponsePayloadBytes uint32
	AuthToken               uint64
}

type ServerConfig struct {
	SupportedProfiles       uint32
	PreferredProfiles       uint32
	MaxRequestBatchItems    uint32
	MaxResponsePayloadBytes uint32
	AuthToken               uint64
}

type HandlerFunc = func(*protocol.AppsLookupRequestView, *protocol.AppsLookupBuilder) bool

type Handler struct {
	Handle HandlerFunc
}
