// Package raw provides internal L2/L3 helpers used by the protocol, fixtures,
// and benchmarks.
//
// Pure composition of L1 transport + Codec. No direct socket/pipe calls.
// Client manages connection lifecycle with at-least-once retry.
// Server handles accept, read, dispatch, respond.
//
// Pure Go — no cgo. Works with CGO_ENABLED=0.
package raw

// Poll/receive timeout for server loops (ms). Controls shutdown detection latency.
const serverPollTimeoutMs = 100

// ---------------------------------------------------------------------------
//  Client state (shared across platforms)
// ---------------------------------------------------------------------------

// ClientState represents the connection state machine.
type ClientState int

const (
	StateDisconnected ClientState = iota
	StateConnecting
	StateReady
	StateNotFound
	StateAuthFailed
	StateIncompatible
	StateBroken
)

// ClientStatus is a diagnostic counters snapshot.
type ClientStatus struct {
	State          ClientState
	ConnectCount   uint32
	ReconnectCount uint32
	CallCount      uint32
	ErrorCount     uint32
}
