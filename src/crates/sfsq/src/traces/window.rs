//! The shared query time window: one validated, half-open nanosecond
//! range serving every windowed trace operation — attribute
//! enumeration's file-granular pruning, search's candidate pruning and
//! span-start filtering, the aggregates' scan bounds and envelope-start
//! rank filter — so the half-open-nanosecond comparisons are written
//! once and can never fork between operations.
//!
//! Windows are built at the boundary — the CLI's paired
//! `--start-ns`/`--end-ns`, the ledger's canonicalized wire window (the
//! ledger owns that canonicalization; this is the engine's type) — and
//! inside the engine by the overview alone, from its grid geometry.
//! Every other windowed operation receives an already-validated
//! window; how an invalid one surfaces is the [`WindowError`] wrap
//! below.

/// A half-open `[start_ns, end_ns)` nanosecond window. Construction
/// validates `start_ns < end_ns` — zero-width and inverted windows are
/// refused, so the type never circulates a degenerate range and no
/// operation re-checks it.
#[derive(Debug, Clone, Copy, PartialEq, Eq)]
pub struct TimeWindow {
    start_ns: i64,
    end_ns: i64,
}

/// An invalid window is a request error on whichever operation received
/// it: the windowed operations' request-error enums (search, overview,
/// attributes) wrap this via `#[from]` — the per-op wrap pattern.
/// Search and attributes never construct a window (only the overview
/// does, from its grid), so their variant keeps the error surface
/// uniform; `slowest` takes an already-built window and has none.
#[derive(Debug, thiserror::Error)]
pub enum WindowError {
    #[error("invalid time window [{start_ns}, {end_ns}): start must be before end")]
    Invalid { start_ns: i64, end_ns: i64 },
}

impl TimeWindow {
    pub fn new(start_ns: i64, end_ns: i64) -> Result<Self, WindowError> {
        if start_ns >= end_ns {
            return Err(WindowError::Invalid { start_ns, end_ns });
        }
        Ok(Self { start_ns, end_ns })
    }

    /// Whether `t_ns` lies in the window — the span-start test (search
    /// decision 5: a span is in the window iff its START is). Consumed
    /// per-span by the predicate evaluator and once per merged trace by
    /// slowest's rank filter (on the envelope start).
    pub(crate) fn contains(&self, t_ns: i64) -> bool {
        self.start_ns <= t_ns && t_ns < self.end_ns
    }

    /// The window as the half-open range the position machinery
    /// consumes: the `timestamps.window` scan bounds in search's
    /// phase-1 discovery and the aggregates' shared fold, plus search's
    /// rank-bound clip.
    pub(crate) fn range_ns(&self) -> std::ops::Range<i64> {
        self.start_ns..self.end_ns
    }

    /// Whether a sealed file can hold data in this window, from its
    /// inclusive-seconds summary range `[min_s, max_s]` (decision 20A:
    /// time bounding is file-granular): the range expands to the
    /// half-open nanoseconds `[min_s·10⁹, (max_s+1)·10⁹)` (saturating)
    /// and half-open ranges intersect iff each starts before the other
    /// ends. THE file-pruning comparison — attribute enumeration and
    /// search call this one method, so their file granularity cannot
    /// drift. Tails are out of its reach (no summary, never pruned):
    /// attributes takes a tail wholesale; search filters its spans
    /// per-span.
    pub(crate) fn overlaps_summary(&self, min_s: u32, max_s: u32) -> bool {
        const NS: i64 = 1_000_000_000;
        let file_start = i64::from(min_s).saturating_mul(NS);
        let file_end = (i64::from(max_s) + 1).saturating_mul(NS);
        self.start_ns < file_end && file_start < self.end_ns
    }
}
