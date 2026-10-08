//! Shared foundation for the journal stack: time units, collection aliases,
//! host-identity loaders and the `compat` shim.
//!
//! The lowest crate of the stack — its only dependencies are generic
//! infrastructure (`nix`, `uuid`, `serde`, `rustc-hash`, optional
//! `allocative`), so the stack's shared types live here rather than in any
//! higher crate, letting the journal crates share them without depending on
//! one another. Consumers (Cargo.toml): journal-core, journal-index,
//! journal-registry, journal-engine and journal-log-writer, plus the
//! ng-ingest OTLP ingest binaries; journal-function consumes only the
//! collection aliases, transitively through journal-core's re-export shim.
//!
//! Modules:
//! - `time` - type-safe `Seconds`/`Microseconds` units, the strictly
//!   monotonic `RealtimeClock`, and `monotonic_now` (CLOCK_MONOTONIC µs
//!   since boot, matching systemd's journal monotonic-timestamp semantics).
//! - `system` - loaders for the host machine ID, boot ID and hostname
//!   (per-platform implementations for Linux and macOS).
//! - `collections` - `rustc_hash::FxHashMap`/`FxHashSet` aliases plus std's
//!   `VecDeque`; the single file that decides the stack's hasher.
//! - `compat` - the legacy `is_multiple_of` shim; the workspace MSRV (1.91)
//!   is past the 1.87 std stabilization, so this stays only because its two
//!   callers (journal-core, journal-index) still use it. Reached via the
//!   module path, not re-exported here.
//!
//! Feature `allocative` gates an `allocative::Allocative` derive on the
//! [`time`] units [`Seconds`] and [`Microseconds`] (the `cfg_attr` derives on
//! their definitions in `src/time.rs`); journal-index and journal-registry
//! forward their own `allocative` features to it.

pub mod collections;
pub mod compat;
pub mod system;
pub mod time;

// The import surface the stack uses for the time types: no consumer
// references the `journal_common::time` module path.
// - `Seconds`: journal-registry (time ranges, repositories), journal-engine
//   (tests, examples) and journal-index's integration tests; re-exported
//   onward by `journal-index/src/lib.rs`
//   (`pub use journal_common::{Microseconds, Seconds}`).
// - `Microseconds`: journal-log-writer (entry timestamps); re-exported
//   onward by journal-index.
// - `RealtimeClock`, `monotonic_now`: journal-log-writer only — the writer's
//   realtime clock, resumed from the persisted chain tail, and the per-entry
//   monotonic timestamp.
pub use time::{Microseconds, RealtimeClock, Seconds, monotonic_now};

// Flat re-export of the collection aliases; unused on this path. The stack
// reaches them via the `collections` module instead: journal-registry
// imports it directly, journal-core wraps it in a re-export shim
// (`journal-core/src/collections.rs`) that the rest of the stack consumes.
pub use collections::{HashMap, HashSet, VecDeque};

// Host-identity loaders; like `time`, reached only through this re-export
// (no `journal_common::system` module-path importer). journal-log-writer
// names its per-host journal directory with `load_machine_id` and stamps
// entries' `_BOOT_ID` field with `load_boot_id`
// (journal-log-writer/src/log/mod.rs); the ng-ingest OTLP ingest binaries
// build their WAL file identity from both (ng-ingest/src/main.rs,
// ng-ingest/src/bin/traces.rs). `load_hostname` has no external importer.
pub use system::{load_boot_id, load_hostname, load_machine_id};
