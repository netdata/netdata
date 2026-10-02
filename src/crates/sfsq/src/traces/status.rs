//! Query-level result status — the no-silent-degrade contract.
//!
//! Every traces operation — trace-by-id, search, overview, slowest,
//! attribute names/values — returns a [`QueryStatus`] beside its data:
//! [`Complete`](QueryStatus::Complete) means every relevant source was
//! successfully and fully examined; [`Partial`](QueryStatus::Partial)
//! carries the non-empty set of reasons why the result may be missing
//! something. Reasons coexist (a query can hit the span cap AND lose a
//! source), so the status holds a set, not a single variant — an
//! explicit precedence rule that discards reasons would hide
//! information.
//!
//! Consumers render it, never reinterpret it: the CLI prints the set,
//! and the ledger's `to_*_result` adapters map it 1:1 onto the wire
//! `StatusWire` (search pagination additionally requires
//! [`QueryStatus::is_complete`] before it emits a next cursor).

use std::collections::BTreeSet;

/// Why a result is partial. The derived `Ord` is declaration order, so
/// the set — and its wire rendering — is deterministic; add new
/// reasons at the end.
#[derive(Debug, Clone, Copy, PartialEq, Eq, PartialOrd, Ord)]
pub enum PartialReason {
    /// The span cap was reached: the result holds the globally earliest
    /// `cap` canonical spans (the combiner's total order); at least one
    /// more unique span existed (an exactly-cap trace is not truncated).
    SizeCap,
    /// A source failed (map/open, TIDX, column or EVNB/LNKB decode,
    /// tail scan): spans that may live there are absent from the result.
    SourceFailure,
    /// Search's work ceiling was hit before the candidate space was
    /// exhausted (the assembled-candidate ceiling, or the visited-rows
    /// ceiling that gates a grow-K refill). Search-only: trace-by-id
    /// never adds it — the span cap is that mode's runaway-work guard.
    WorkCeiling,
    /// The caller cancelled. Trace-by-id returns EMPTY before all
    /// source heads are resolved, and the deterministic merged prefix
    /// once the merge has begun (that mode's pinned exception); search,
    /// the attribute scans and the aggregate folds are ALL-OR-EMPTY — a
    /// mid-flight search prefix cannot be deterministic under canonical
    /// re-ranking and grow-K, and a mid-loop aggregate cancel discards
    /// the partial fold.
    Cancelled,
    /// The overview's visited-rows ceiling was hit before every
    /// in-window source was binned: the grid holds the deterministic
    /// prefix of sources ([`SourceId`](super::SourceId) order)
    /// processed so far. Distinct from
    /// [`WorkCeiling`](Self::WorkCeiling) — this budget counts rows
    /// visited per source (TRSU rows / tail spans), not search's
    /// candidate loop.
    OverviewCeiling,
    /// A sealed source has no trace rollup chunk (`TRSU`) and was
    /// EXCLUDED from trace-level aggregation: including it would
    /// silently undercount, and mixing span-level numbers into a
    /// trace-level result is forbidden (no mixed units). Two causes:
    /// the file predates the rollup (pre-rollup retention — the reason
    /// disappears when the file ages out), or the file stored no real
    /// traces: an all-UNSET trace-id file accumulates no rollup rows,
    /// writes no chunk by the seal's `is_meaningful` rule, and
    /// exclusion loses nothing.
    RollupAbsent,
    /// The slowest mode's visited-rows ceiling was hit before every
    /// in-window source was folded: the ranking covers the
    /// deterministic prefix of sources ([`SourceId`](super::SourceId)
    /// order) processed so far — the true slowest trace may live in an
    /// unvisited source.
    SlowestCeiling,
    /// A source's bytes could not be obtained from remote storage (the
    /// download failed, or the catalog listing the file could not be
    /// read): spans that may live there are absent. Distinct from
    /// [`SourceFailure`](Self::SourceFailure), which reports bytes that
    /// were available but could not be read. Declared last: wire order
    /// is declaration order.
    RemoteUnavailable,
}

/// The status of one query's result.
#[derive(Debug, Clone, PartialEq, Eq)]
pub enum QueryStatus {
    /// Every relevant source was successfully and fully examined.
    Complete,
    /// Something may be missing; the set is non-empty by construction
    /// ([`StatusBuilder::finish`]).
    Partial(BTreeSet<PartialReason>),
}

impl QueryStatus {
    /// True for [`QueryStatus::Complete`] — nothing is known to be
    /// missing.
    pub fn is_complete(&self) -> bool {
        matches!(self, QueryStatus::Complete)
    }

    /// Whether `reason` is among the partial reasons (`false` for
    /// `Complete`).
    pub fn has(&self, reason: PartialReason) -> bool {
        match self {
            QueryStatus::Complete => false,
            QueryStatus::Partial(reasons) => reasons.contains(&reason),
        }
    }
}

/// Accumulates partial reasons during a query; the empty accumulation IS
/// the `Complete` status, so the "non-empty reasons" invariant holds by
/// construction rather than by discipline at every return site.
#[derive(Debug, Default)]
pub struct StatusBuilder {
    reasons: BTreeSet<PartialReason>,
}

impl StatusBuilder {
    pub fn new() -> Self {
        Self::default()
    }

    /// Record a reason. Idempotent (a set); reasons coexist.
    pub fn add(&mut self, reason: PartialReason) {
        self.reasons.insert(reason);
    }

    /// `Complete` when nothing was recorded; otherwise `Partial` with
    /// every recorded reason.
    pub fn finish(self) -> QueryStatus {
        if self.reasons.is_empty() {
            QueryStatus::Complete
        } else {
            QueryStatus::Partial(self.reasons)
        }
    }
}

#[cfg(test)]
mod tests {
    use super::*;

    #[test]
    fn empty_builder_is_complete() {
        assert_eq!(StatusBuilder::new().finish(), QueryStatus::Complete);
        assert!(StatusBuilder::new().finish().is_complete());
    }

    #[test]
    fn reasons_coexist_and_render_deterministically() {
        // Insertion order must not matter (the contract: a set,
        // deterministic; e.g. SizeCap + SourceFailure both survive).
        let mut a = StatusBuilder::new();
        a.add(PartialReason::SourceFailure);
        a.add(PartialReason::SizeCap);
        a.add(PartialReason::SourceFailure); // idempotent

        let mut b = StatusBuilder::new();
        b.add(PartialReason::SizeCap);
        b.add(PartialReason::SourceFailure);

        let a = a.finish();
        assert_eq!(a, b.finish());
        assert!(!a.is_complete());
        assert!(a.has(PartialReason::SizeCap));
        assert!(a.has(PartialReason::SourceFailure));
        assert!(!a.has(PartialReason::Cancelled));
    }
}
