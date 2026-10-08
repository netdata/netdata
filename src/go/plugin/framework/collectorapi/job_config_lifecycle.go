// SPDX-License-Identifier: GPL-3.0-or-later

package collectorapi

import "encoding/hex"

// JobConfigIdentity is an opaque identity for one exact job configuration.
// Job Manager derives it; collectors must not derive or decode it.
type JobConfigIdentity [32]byte

func (id JobConfigIdentity) Valid() bool {
	return id != JobConfigIdentity{}
}

func (id JobConfigIdentity) String() string {
	if !id.Valid() {
		return ""
	}
	return hex.EncodeToString(id[:])
}

// JobConfigLifecycle projects a small diagnostic lifecycle snapshot at the
// authoritative Job Manager configuration-commit boundary. Implementations
// must be fail-open. Project must not retain config, and snapshots must contain
// no configuration or runtime objects. Reconcile receives the prior graph
// incarnation plus a runtime only for a successfully accepted job; failed or
// pre-construction states receive nil.
type JobConfigLifecycle interface {
	Project(JobConfigIdentity, map[string]any) JobConfigLifecycleSnapshot
	Bind(JobConfigIdentity, RuntimeJob)
	Capture(JobConfigIdentity, RuntimeJob) JobConfigLifecycleSnapshot
	Reconcile(JobConfigIdentity, JobConfigLifecycleSnapshot, RuntimeJob)
	Remove(JobConfigIdentity)
}

// JobConfigLifecycleSnapshot is a credential-free value detached from runtime
// and candidate cleanup.
type JobConfigLifecycleSnapshot interface {
	Identity() JobConfigIdentity
}
