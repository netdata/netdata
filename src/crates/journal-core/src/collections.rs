//! Re-exports the shared collection type aliases (`HashMap`, `HashSet`,
//! `VecDeque`) from `journal-common`, so crates that depend on `journal-core`
//! can import them from here. The types and the rationale behind them
//! (FxHasher-backed map and set on a single import path) are documented where
//! they are defined, in `src/crates/journal-common/src/collections.rs`.
//!
//! The hard need is `journal-function`: its Cargo.toml has no
//! `journal-common` dependency, so this module is its only route to the
//! aliases. The other `journal-core` dependents (journal-index,
//! journal-engine, journal-log-writer) and journal-core's own modules import
//! through here as well, keeping one import path across the stack;
//! `journal-registry`, which does not depend on `journal-core`, imports
//! `journal_common::collections` directly.
//!
//! Consumers (grep-verified): journal-core `field_map.rs`, `file/file.rs`
//! (via `crate::collections`); journal-index `file_index.rs`,
//! `file_indexer.rs`; journal-function `netdata/facets.rs`,
//! `netdata/histogram.rs`; journal-engine `histogram.rs`; journal-log-writer
//! `log/chain.rs`. Without this module, journal-function would not compile,
//! and the rest would need direct `journal_common::collections` imports.

pub use journal_common::collections::{HashMap, HashSet, VecDeque};
