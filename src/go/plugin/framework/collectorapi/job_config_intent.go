// SPDX-License-Identifier: GPL-3.0-or-later

package collectorapi

// JobConfigIntentOwner owns resources needed by committed enabled configurations.
// Unlike JobConfigLifecycle, this capability is authoritative and not fail-open.
// Running, Failed and retry transitions retain intent until disable, removal,
// replacement or final joined run retirement.
//
// Before Prepare receives a changed nonzero next identity, the framework validates
// raw structure and literal field types without resolving references or checking
// dependency readiness. Operational Init/Check failure does not revoke that proof.
//
// Prepare reserves protection without starting work, downloading dependencies or
// changing idle timestamps. Zero previous/next identities mean no enabled intent.
// Config is the borrowed raw next configuration (nil when next is zero); it must
// not be retained. An error leaves the previous intent intact and returns no
// reservation. Calls for different jobs may overlap; implementations must be safe
// for concurrent use. Preparation must be bounded and must not wait for downloads.
type JobConfigIntentOwner interface {
	Prepare(previous, next JobConfigIdentity, config map[string]any) (PreparedJobConfigIntent, error)
}

// PreparedJobConfigIntent is consumed exactly once by Commit or Abort.
// Commit runs synchronously after graph adoption and before activation or reply.
// It must transfer authoritative intent without operational failure. Subsequent
// setup/storage failures must retain reserved protection and remain visible as
// unavailable dependency state; they must not silently discard enabled demand.
// Abort releases only the uncommitted reservation and preserves incumbent intent.
// If Abort fails, the owner remains responsible for unreleased resources and
// recovery; the framework consumes the reservation and will not call Abort again.
// Both operations must be bounded; Commit may enqueue asynchronous preparation.
type PreparedJobConfigIntent interface {
	Commit()
	Abort() error
}
