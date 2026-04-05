//! L2 orchestration: service-specific client contexts and managed servers.
//!
//! Production-facing modules are service-kind specific. Internal helpers may
//! remain generic for tests and benchmarks, but every running endpoint still
//! serves exactly one request kind.

pub mod cgroups;
#[doc(hidden)]
pub mod raw;
