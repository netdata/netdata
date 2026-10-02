//! The logs engine's view of the shared source plumbing
//! ([`crate::source`]): the same `Mapped`/`release_cold_region`
//! re-exported unchanged, plus the logs-historical **log-and-degrade**
//! wrapper over [`crate::source::map_source`] — a source that fails to
//! map is logged and contributes nothing, so one bad source never sinks
//! a query. (The traces engine deliberately does NOT share this shape:
//! it consumes the structured error and reports the failure as an
//! explicit partial-result reason.)
//!
//! "Mapping" is an mmap only for file-backed sources — a sealed SFST,
//! mapped read-only. An in-memory candidate (an SFST built from a chunk
//! of an active WAL) shares its bytes by refcount and cannot fail, and
//! WAL tails are never mapped at all — engine.rs row-scans them.
//! Callers: `engine::run`, which maps every SFST source once per query
//! and shares each mapping with the stats and page passes, and
//! `LogsShard::evaluate` (aggregate.rs), the standalone path.

// The shared plumbing, re-exported so the logs modules import these two
// via `super::mmap` rather than `crate::source` directly.
pub(super) use crate::source::{Mapped, release_cold_region};

use crate::source::Source;

/// Obtain a candidate's bytes from its [`Source`], logging and
/// returning `None` on failure.
///
/// The warning embeds the error's `Display` — the failed operation
/// (`open` or `mmap`), the path, and the OS error — so the log names
/// the skipped source. `None` is the degrade value: callers swap in an
/// empty shard or skip the source; a mapping failure never errors.
pub(super) fn map_source(source: &Source) -> Option<Mapped> {
    match crate::source::map_source(source) {
        Ok(mapped) => Some(mapped),
        Err(e) => {
            tracing::warn!("sfsq: failed to {e}");
            None
        }
    }
}

#[cfg(test)]
mod tests {
    use super::*;

    /// The logs contract this wrapper preserves: a mapping failure
    /// degrades to `None` (logged), it does not error.
    #[test]
    fn failure_degrades_to_none() {
        let missing = Source::File(std::path::PathBuf::from("/nonexistent/sfsq-logs-mmap-test"));
        assert!(map_source(&missing).is_none());
    }
}
