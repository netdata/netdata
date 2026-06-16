use std::ops::Range;

use crate::ServiceStream;

/// A time-range + optional stream filter, used by `Registry::candidates`
/// implementations across the file-registry-backed sources (`sfst`,
/// `wal`, …) to identify which files satisfy a read.
///
/// The query is intentionally minimal: it carries only what the
/// registries can answer from their cheap inline summaries (per-file
/// `(min, max)` timestamps and stream identity), without opening any
/// file. Predicate pushdown for within-file selection is a separate
/// concern handled by the readers.
#[derive(Debug, Clone)]
pub struct Query {
    /// Time window of interest, in seconds since the Unix epoch.
    /// Inclusive lower bound, exclusive upper bound. A registry treats a
    /// file as a candidate if its `[min_timestamp, max_timestamp]` range
    /// overlaps `[start, end)`.
    pub time_range: Range<u32>,
    /// Stream filter. `None` matches every stream; `Some(s)` requires
    /// exact equality with the file's stream identity (or, for sources
    /// that only carry `ns_hash`, equality with [`ServiceStream::ns_hash`]).
    /// An empty field equals an absent one, so a query for an absent
    /// `service.namespace` matches files written with an empty one and
    /// vice versa.
    pub stream: Option<ServiceStream>,
}

impl Query {
    /// Whether a file whose data spans `[min_s, max_s]` overlaps this
    /// query's window — see [`range_overlaps`] for the rule.
    pub fn overlaps(&self, min_s: u32, max_s: u32) -> bool {
        range_overlaps(&self.time_range, min_s, max_s)
    }
}

/// The one time-overlap rule every registry and catalog uses: a data
/// range `[min, max]` (inclusive on both ends) overlaps a query window
/// `[start, end)` (half-open) iff `max >= start && min < end`; an empty
/// window (`start >= end`) matches nothing.
///
/// Centralized because a drift between copies of this predicate means
/// silent query gaps — one source skipping files another would serve.
/// Generic over the unit so second-based (`u32`) and nanosecond-based
/// (`u64`) candidates share it.
pub fn range_overlaps<T: Ord + Copy>(window: &Range<T>, min: T, max: T) -> bool {
    if window.start >= window.end {
        return false;
    }
    max >= window.start && min < window.end
}

#[cfg(test)]
mod tests {
    use super::range_overlaps;

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
