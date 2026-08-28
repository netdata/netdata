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

// JobConfigLifecycle lets a collector project a small diagnostic lifecycle
// snapshot at the authoritative Job Manager configuration-commit boundary.
// Implementations must be fail-open and must not retain configuration values.
type JobConfigLifecycle interface {
	Bind(JobConfigIdentity, RuntimeJob)
	Capture(JobConfigIdentity, RuntimeJob) JobConfigLifecycleSnapshot
	Remove(JobConfigIdentity)
}

// JobConfigLifecycleSnapshot is detached from candidate cleanup. Commit is
// called only after the matching configuration graph transition commits.
type JobConfigLifecycleSnapshot interface {
	Identity() JobConfigIdentity
	Commit(previous JobConfigIdentity)
}
