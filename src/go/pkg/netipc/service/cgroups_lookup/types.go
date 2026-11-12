// Package cgroups_lookup provides the public single-kind L2 surface for the
// cgroups-lookup service.
package cgroups_lookup

import (
	"github.com/netdata/netdata/go/plugins/pkg/netipc/protocol"
	"github.com/netdata/netdata/go/plugins/pkg/netipc/service/internal/transportconfig"
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

type ClientConfig transportconfig.TypedConfig

type ServerConfig transportconfig.TypedConfig

// HandlerFunc is the typed callback used by the cgroups-lookup service.
// The request borrows the receive buffer and the builder writes into the
// response buffer; both are valid only during the call, so copy what you
// keep.
type HandlerFunc = func(*protocol.CgroupsLookupRequestView, *protocol.CgroupsLookupBuilder) bool

type Handler struct {
	Handle HandlerFunc
}
