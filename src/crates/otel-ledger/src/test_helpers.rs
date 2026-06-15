//! Shared test fixtures.
//!
//! Only compiled under `#[cfg(test)]`; use across the crate's test modules
//! to avoid duplicating common construction patterns.

/// A `Summary` populated with zero-valued fields and an empty stream
/// identity. Used by registry/recovery tests that need to call
/// `Registry::track` without caring about the summary's contents.
pub(crate) fn empty_summary() -> sfst::Summary {
    sfst::Summary {
        min_timestamp_s: 0,
        max_timestamp_s: 0,
        total_logs: 0,
        stream: sfst::ServiceStream::new("", ""),
    }
}
