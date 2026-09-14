//! L2 orchestration: service-specific client contexts and managed servers.
//!
//! Production-facing modules are service-kind specific. Internal helpers may
//! remain generic for tests and benchmarks, but every running endpoint still
//! serves exactly one request kind.

pub mod apps_lookup;
pub mod cgroups;
pub mod cgroups_lookup;
pub mod cgroups_snapshot;
#[doc(hidden)]
pub mod raw;
