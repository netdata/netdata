// Package raw provides internal L2/L3 helpers used by the protocol, fixtures,
// and benchmarks.
//
// Pure composition of L1 transport + Codec. No direct socket/pipe calls.
// Client manages connection lifecycle with at-least-once retry.
// Server handles accept, read, dispatch, respond.
//
// Pure Go — no cgo. Works with CGO_ENABLED=0.
package raw

import "time"

// Poll/receive timeout for server loops (ms). Controls shutdown detection latency.
const serverPollTimeoutMs = 100

// ClientCallTimeoutDefaultMs is the default synchronous client call timeout.
const ClientCallTimeoutDefaultMs uint32 = 30000

const clientAbortPollMs uint32 = 100

func boundedClientWaitMs(remaining time.Duration, pollCapMs uint32) uint32 {
	if remaining <= 0 {
		return 0
	}
	if remaining >= time.Duration(pollCapMs)*time.Millisecond {
		return pollCapMs
	}
	waitMs := uint32((remaining + time.Millisecond - 1) / time.Millisecond) // #nosec G115 -- remaining is positive and below pollCapMs here.
	if waitMs == 0 {
		return 1
	}
	return waitMs
}

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
