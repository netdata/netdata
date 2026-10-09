//! The substrate's query filter: [`Query`], the time-range + partition
//! request parameter handed to every candidate-selection function, and
//! [`range_overlaps`], the single overlap predicate they all share.
//!
//! A `Query` is answerable from cheap inline summaries alone — a file's
//! `[min, max]` timestamps and its partition key — without opening the
//! file; within-file predicate pushdown is the readers' concern, and
//! selected files travel as [`SelectedFile`](crate::SelectedFile), not
//! through this type.
//!
//! Contracts owned here:
//!
//! - Time units and bounds: `time_range` is a half-open `[start, end)`
//!   window in seconds since the Unix epoch — the
//!   [`FileSummary`](crate::FileSummary) unit (`types.rs` owns the summary
//!   fields and unit conventions). Data ranges are inclusive `[min, max]`, so overlap is
//!   `max >= start && min < end`, and a degenerate window
//!   (`start >= end`) matches nothing.
//! - Partition filter: an empty `partition_keys` set matches every
//!   partition; a non-empty set is membership-only over the opaque
//!   [`FileId::part_key`](crate::FileId#structfield.part_key). For OTel
//!   logs the keys are the content plane's service-stream hashes, driven
//!   by the `__streams` selector — one stream identity per key within a
//!   tenant via the ingestor's collision table.
//! - Plain in-process value: `Clone`, no serde; the predicates here are
//!   pure and infallible — no I/O. Every caller constructs the query at
//!   its own boundary (RPC handler, CLI discovery, tests).
//!
//! Consumers (grep-verified): `wal::registry` (scales `time_range` to ns
//! and re-applies [`range_overlaps`] over `u64` bounds), `sfst::registry`,
//! `otel-catalog` (`Catalog::find` and the registry-side file filter), and
//! `file-lifecycle` (the candidate pass-throughs in `registry.rs`;
//! `query.rs::select_remote_only` is the only `matches_partition` caller).
//! `otel-ledger`'s logs/traces RPC handlers build the queries; `sfsq-cli`
//! discovers offline SFST candidates with one.
use std::ops::Range;

/// A time-range + optional partition filter identifying which files can
/// serve a read.
///
/// Intentionally minimal: it carries only what a registry or catalog can
/// answer from its cheap inline summaries (per-file `(min, max)`
/// timestamps and the opaque partition key) without opening any file.
/// Predicate pushdown for within-file selection is a separate concern,
/// handled by the readers. Taken by the per-source candidate functions
/// (`wal::Registry::candidates`, `sfst::Registry::candidates`,
/// `otel_catalog::Catalog::find`) and composed over them by
/// `file-lifecycle`.
#[derive(Debug, Clone)]
pub struct Query {
    /// Half-open `[start, end)` window in seconds since the Unix epoch
    /// (the [`FileSummary`](crate::FileSummary) unit). A file is a
    /// candidate when its inclusive `[min, max]` range overlaps the
    /// window — [`range_overlaps`] is the rule; `start >= end` matches
    /// nothing.
    pub time_range: Range<u32>,
    /// Partition filter over the opaque
    /// [`FileId::part_key`](crate::FileId#structfield.part_key) values.
    /// **Empty matches every partition**; a non-empty set keeps only files
    /// whose `part_key` is one of these values
    /// ([`matches_partition`](Query::matches_partition)). The substrate
    /// compares the key as an opaque `u64` and ascribes it no meaning; the
    /// content plane supplies the set (for OTel logs, the service-stream
    /// hashes driven by the `__streams` selector — one stream per key
    /// within a tenant via the ingestor's collision table).
    pub partition_keys: Vec<u64>,
}

impl Query {
    /// Whether a file whose data spans the inclusive `[min_s, max_s]`
    /// seconds overlaps this query's window — see [`range_overlaps`] for
    /// the rule.
    pub fn overlaps(&self, min_s: u32, max_s: u32) -> bool {
        range_overlaps(&self.time_range, min_s, max_s)
    }

    /// Whether a file with partition key `part_key` passes the partition
    /// filter: an empty [`Query::partition_keys`] matches every partition,
    /// otherwise the key must be in the set.
    ///
    /// The per-source candidate filters (`wal`, `sfst`, `otel-catalog`)
    /// inline the same empty-or-contains test over their entries instead of
    /// calling this; the method is the rule's canonical statement and the
    /// form used where filtering runs after a registry scan
    /// (file-lifecycle's remote-only catalog selection).
    pub fn matches_partition(&self, part_key: u64) -> bool {
        self.partition_keys.is_empty() || self.partition_keys.contains(&part_key)
    }
}

/// The one time-overlap rule every registry and catalog uses: a data
/// range `[min, max]` (inclusive on both ends) overlaps a query window
/// `[start, end)` (half-open) iff `max >= start && min < end`; a window
/// with `start >= end` matches nothing — the early return guards this,
/// since the comparison alone would still match a containing range.
///
/// Generic over the unit so second-based (`u32`) and nanosecond-based
/// (`u64`) candidates share one predicate: `sfst` and the catalog compare
/// summary seconds directly, the WAL registry scales
/// [`Query::time_range`] to ns and calls this with `u64` bounds.
/// Centralized because a drift between copies of this rule means silent
/// query gaps — one source skipping files another would serve.
pub fn range_overlaps<T: Ord + Copy>(window: &Range<T>, min: T, max: T) -> bool {
    if window.start >= window.end {
        return false;
    }
    max >= window.start && min < window.end
}

#[cfg(test)]
mod tests {
    use super::{Query, range_overlaps};

    #[test]
    fn matches_partition_empty_is_all_nonempty_is_membership() {
        let all = Query {
            time_range: 0..1,
            partition_keys: Vec::new(),
        };
        // Empty set matches every partition, including the `0` key.
        assert!(all.matches_partition(0));
        assert!(all.matches_partition(42));

        let filtered = Query {
            time_range: 0..1,
            partition_keys: vec![7, 9],
        };
        assert!(filtered.matches_partition(7));
        assert!(filtered.matches_partition(9));
        assert!(!filtered.matches_partition(8));
    }

    #[test]
    fn overlap_rule_contract() {
        // Inclusive data range vs half-open window.
        assert!(range_overlaps(&(10u32..20), 5, 10)); // max == start: in
        assert!(range_overlaps(&(10u32..20), 19, 25)); // min == end-1: in
        assert!(!range_overlaps(&(10u32..20), 20, 25)); // min == end: out
        assert!(!range_overlaps(&(10u32..20), 0, 9)); // max < start: out
        assert!(range_overlaps(&(10u32..11), 10, 10)); // single-point window
        // Empty window matches nothing — even a containing range. The
        // inverted range is deliberate: it pins that `start > end` is
        // treated as empty, not as a panic or a wraparound match.
        assert!(!range_overlaps(&(10u32..10), 0, 100));
        #[allow(clippy::reversed_empty_ranges)]
        let inverted = 20u32..10;
        assert!(!range_overlaps(&inverted, 0, 100));
        // Generic over the unit (the wal registry's u64 nanoseconds).
        assert!(range_overlaps(&(1_000u64..2_000), 1_999, 5_000));
    }
}
