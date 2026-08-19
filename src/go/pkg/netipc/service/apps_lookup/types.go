// Package apps_lookup provides the public single-kind L2 surface for the
// apps-lookup service.
package apps_lookup

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

type HandlerFunc = func(*protocol.AppsLookupRequestView, *protocol.AppsLookupBuilder) bool

type Handler struct {
	Handle HandlerFunc
}
