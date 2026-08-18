//! The optional per-file trace rollup (`TRSU` chunk): one row per
//! distinct set (non-UNSET) trace id in the file — the trace-level
//! aggregate the traces overview/slowest/facet queries fold WITHOUT
//! assembling traces.
//!
//! Semantics (pinned):
//!
//! - **Stored-row statistics**: counts count what is on disk. A
//!   resent span counts every time it is stored — the canonical
//!   `(span_id, kind)` dedup belongs to assembly (`trace_combine`),
//!   deliberately NOT replicated here. Consumers label the numbers so.
//! - **Root honest-or-absent**: root fields are set only from a
//!   span with a genuinely UNSET parent seen in THIS file (the earliest
//!   such span wins, mirroring `Trace::summary_root`'s convention);
//!   otherwise `root_is_true_root` is false and the root fields are
//!   sentinels. A consumer NEVER synthesizes a root from them. A full
//!   `(start_ns, span_id)` tie between candidates with DIFFERENT
//!   recorded facets ABSTAINS rather than guessing a pick the
//!   combiner's deeper tie-break keys could contradict — flagged
//!   [`ROOT_CLAIM_WITHHELD`], distinct from [`ROOT_CLAIM_NONE`]'s
//!   proof of local root absence (the flag a pruning consumer may
//!   act on under true-root filter semantics).
//! - **UNSET trace ids are excluded** (the TIDX rule): the all-zero id
//!   is the OTLP "unset" sentinel, not a trace.
//!
//! Accumulated inside the seal's existing single-pass span walk
//! ([`TraceRollupRows`], the `EventRows` pattern: holds interner
//! [`KvSlot`]s, the build translates them to file [`KvId`]s), emitted
//! sorted by trace id. Additive and TOC-indexed like `TIDX`/`EVNB` — no
//! format version bump; old readers skip it.

use std::collections::HashMap;

use serde::{Deserialize, Serialize};

use crate::kv_interner::KvSlot;
use crate::{KvId, SpanId, SpanIds, TraceId, TraceIds};

/// The sentinel `root_*_ref` value for "no true root / no such
/// attribute on the root": file KvIds are dense small integers, so
/// `u32::MAX` can never collide.
pub const ROLLUP_NO_REF: u32 = u32::MAX;

/// [`TraceRollup::root_is_true_root`] tri-state: NO unset-parent span
/// of this trace is stored in this file — under true-root filter
/// semantics this is PROOF the file contributes no root.
pub const ROOT_CLAIM_NONE: u8 = 0;
/// The root columns carry the file's true-root claim.
pub const ROOT_CLAIM_TRUE: u8 = 1;
/// Unset-parent spans exist in this file but the claim was WITHHELD
/// (an ambiguous full-key tie): consumers must treat the root as
/// unknown — neither absent nor any particular value.
pub const ROOT_CLAIM_WITHHELD: u8 = 2;

/// The sealed rollup: struct-of-arrays, index-parallel across every
/// field, sorted ascending by trace id. Root fields are meaningful only
/// where [`root_is_true_root`](Self::root_is_true_root) is `1`.
#[derive(Debug, Clone, Default, PartialEq, Serialize, Deserialize)]
pub struct TraceRollup {
    /// Distinct set trace ids, ascending (16-byte arena).
    pub trace_ids: TraceIds,
    /// The true root's span id per trace; UNSET when no true root.
    pub root_span_ids: SpanIds,
    /// Envelope start: min stored span start, ns.
    pub min_start_ns: Vec<i64>,
    /// Envelope end: max stored `start ⊕ duration` (saturating), ns.
    pub max_end_ns: Vec<i64>,
    /// Stored spans of this trace in THIS file (resends included).
    pub span_counts: Vec<u32>,
    /// Of those, spans with ERROR status.
    pub error_counts: Vec<u32>,
    /// The true root's raw OTLP span kind; 0 when no true root.
    pub root_kinds: Vec<i32>,
    /// Root-claim tri-state per row: [`ROOT_CLAIM_NONE`] (no
    /// unset-parent span stored here — proof of local root absence),
    /// [`ROOT_CLAIM_TRUE`] (the root columns carry the claim), or
    /// [`ROOT_CLAIM_WITHHELD`] (unset-parent spans exist, claim
    /// withheld on an ambiguous tie — root unknown).
    pub root_is_true_root: Vec<u8>,
    /// File-interner [`KvId`] of the root's resource `service.name`
    /// entry, or [`ROLLUP_NO_REF`].
    pub root_service_refs: Vec<u32>,
    /// File-interner [`KvId`] of the root's `name` entry, or
    /// [`ROLLUP_NO_REF`].
    pub root_name_refs: Vec<u32>,
}

impl TraceRollup {
    /// Validate the decoded chunk's structural invariants (the reader
    /// calls this; unit-testable without a file, the `LinkIndex`
    /// precedent): index-parallel arrays, in-range root refs (`kv_total`
    /// exclusive, [`ROLLUP_NO_REF`] allowed), tri-state claim flags, and strictly
    /// increasing trace ids (the seal sorts; a crafted duplicate would
    /// silently double-count in every cross-source merge).
    pub(crate) fn validate(&self, kv_total: u32) -> Result<(), crate::Error> {
        // `len()` floors (`bytes / WIDTH`), so a trailing-bytes arena would
        // pass the parallelism check alone — reject non-whole arenas
        // explicitly, like `LinkIndex::validate` and the per-row readers do.
        if !self.trace_ids.well_formed() || !self.root_span_ids.well_formed() {
            return Err(crate::Error::CorruptIndex(
                "trace rollup id arena is not a whole number of ids".into(),
            ));
        }
        let n = self.min_start_ns.len();
        let parallel = self.trace_ids.len() == n
            && self.root_span_ids.len() == n
            && self.max_end_ns.len() == n
            && self.span_counts.len() == n
            && self.error_counts.len() == n
            && self.root_kinds.len() == n
            && self.root_is_true_root.len() == n
            && self.root_service_refs.len() == n
            && self.root_name_refs.len() == n;
        if !parallel {
            return Err(crate::Error::CorruptIndex(
                "trace rollup fields are not index-parallel".into(),
            ));
        }
        let ref_ok = |r: u32| r == ROLLUP_NO_REF || r < kv_total;
        if !self.root_service_refs.iter().all(|&r| ref_ok(r))
            || !self.root_name_refs.iter().all(|&r| ref_ok(r))
            || !self.root_is_true_root.iter().all(|&f| f <= ROOT_CLAIM_WITHHELD)
        {
            return Err(crate::Error::CorruptIndex(
                "trace rollup carries out-of-range root refs or flags".into(),
            ));
        }
        for i in 1..n {
            if self.trace_ids.get(i - 1) >= self.trace_ids.get(i) {
                return Err(crate::Error::CorruptIndex(
                    "trace rollup ids are not strictly increasing".into(),
                ));
            }
        }
        // Internal consistency — the invariants pruning consumers treat
        // as proof, enforced as far as a self-contained check can: a
        // non-claiming row must carry only sentinels (a flipped flag
        // beside real root fields is a detectable contradiction), and
        // the envelope must be well-formed (end = start ⊕ duration ≥
        // start for every honest producer). A CONSISTENT lie — flag and
        // fields forged together — remains outside the trust model, the
        // same boundary as row-completeness (rows cannot be proven
        // complete without re-reading TRCE, which would defeat the
        // rollup's purpose).
        for i in 0..n {
            if self.root_is_true_root[i] != ROOT_CLAIM_TRUE
                && (!self.root_span_ids.get(i).is_unset()
                    || self.root_kinds[i] != 0
                    || self.root_service_refs[i] != ROLLUP_NO_REF
                    || self.root_name_refs[i] != ROLLUP_NO_REF)
            {
                return Err(crate::Error::CorruptIndex(
                    "non-claiming trace rollup row carries root fields".into(),
                ));
            }
            if self.min_start_ns[i] > self.max_end_ns[i] {
                return Err(crate::Error::CorruptIndex(
                    "trace rollup envelope is inverted".into(),
                ));
            }
            // Counter contradictions the writer cannot emit: every row
            // exists because record_span ran at least once, and the
            // error counter increments alongside the span counter.
            if self.span_counts[i] == 0 {
                return Err(crate::Error::CorruptIndex(
                    "trace rollup row carries zero spans".into(),
                ));
            }
            if self.error_counts[i] > self.span_counts[i] {
                return Err(crate::Error::CorruptIndex(
                    "trace rollup error count exceeds span count".into(),
                ));
            }
        }
        Ok(())
    }

    /// Number of distinct set trace ids (rows).
    pub fn len(&self) -> usize {
        self.min_start_ns.len()
    }

    pub fn is_empty(&self) -> bool {
        self.min_start_ns.is_empty()
    }

    /// Row index of `trace_id`, or `None` when this file holds no spans
    /// of that trace. Binary search over the id column — validation
    /// guarantees strictly increasing ids, so `partition_point` is exact.
    pub fn find(&self, trace_id: TraceId) -> Option<usize> {
        let n = self.trace_ids.len();
        let mut lo = 0usize;
        let mut hi = n;
        while lo < hi {
            let mid = lo + (hi - lo) / 2;
            if self.trace_ids.get(mid) < trace_id {
                lo = mid + 1;
            } else {
                hi = mid;
            }
        }
        (lo < n && self.trace_ids.get(lo) == trace_id).then_some(lo)
    }
}

/// One trace's in-progress aggregate (insertion phase).
#[derive(Debug, Clone)]
struct Acc {
    min_start_ns: i64,
    max_end_ns: i64,
    span_count: u32,
    error_count: u32,
    /// The earliest unset-parent span seen so far, when any.
    root: Option<Root>,
    /// The incumbent tied another root candidate with different
    /// recorded facets: the seal abstains from claiming a root.
    root_ambiguous: bool,
}

#[derive(Debug, Clone)]
struct Root {
    start_ns: i64,
    span_id: SpanId,
    kind: i32,
    service: Option<KvSlot>,
    name: Option<KvSlot>,
}

/// Insertion-order accumulator for [`TraceRollup`]: the traces seal
/// calls [`record_span`](Self::record_span) once per stored span inside
/// its existing walk. Holds interner [`KvSlot`]s; the build translates
/// them to file [`KvId`]s and emits rows sorted by trace id.
#[derive(Debug, Clone, Default)]
pub struct TraceRollupRows {
    map: HashMap<TraceId, Acc>,
}

impl TraceRollupRows {
    pub fn new() -> Self {
        Self::default()
    }

    /// Fold one stored span. `duration_ns` is the stored (clamped ≥ 0)
    /// span duration; `service`/`name` are the span's captured interner
    /// slots (resource `service.name`, span `name`) — consulted only
    /// when this span becomes the trace's root candidate.
    ///
    /// `parent_unset` is the typed all-zero check on the STORED parent
    /// id — ingest normalizes an empty/malformed parent to UNSET, so the
    /// check is exactly the OTLP root convention.
    #[allow(clippy::too_many_arguments)]
    pub fn record_span(
        &mut self,
        trace_id: TraceId,
        span_id: SpanId,
        parent_unset: bool,
        start_ns: i64,
        duration_ns: i64,
        kind: i32,
        is_error: bool,
        service: Option<KvSlot>,
        name: Option<KvSlot>,
    ) {
        if trace_id.is_unset() {
            return; // the TIDX rule: the unset sentinel is not a trace
        }
        let end_ns = start_ns.saturating_add(duration_ns.max(0));
        let acc = self.map.entry(trace_id).or_insert(Acc {
            min_start_ns: i64::MAX,
            max_end_ns: i64::MIN,
            span_count: 0,
            error_count: 0,
            root: None,
            root_ambiguous: false,
        });
        acc.min_start_ns = acc.min_start_ns.min(start_ns);
        acc.max_end_ns = acc.max_end_ns.max(end_ns);
        acc.span_count = acc.span_count.saturating_add(1);
        if is_error {
            acc.error_count = acc.error_count.saturating_add(1);
        }
        // The earliest genuinely-unset-parent span wins the root
        // (the summary_root convention). Equal starts tie-break by
        // ascending span id — the combiner total order's next key. On a
        // FULL (start_ns, span_id) tie the canonical pick continues
        // through keys (kind, content) the recorder does not model, so
        // the recorder ABSTAINS instead of guessing: a tie challenger
        // whose recorded facets (kind, service, name) differ from the
        // incumbent's marks the claim AMBIGUOUS, and the seal emits no
        // root for the trace — honest-or-absent, storage-order
        // independent, and never a wrong claim a pruning consumer could
        // act on. Identical-facet ties (plain resends) keep the claim; a
        // strictly earlier span installs a fresh unambiguous incumbent.
        if parent_unset {
            match &acc.root {
                Some(r) if (start_ns, span_id) == (r.start_ns, r.span_id) => {
                    if kind != r.kind || service != r.service || name != r.name {
                        acc.root_ambiguous = true;
                    }
                }
                Some(r) if (start_ns, span_id) > (r.start_ns, r.span_id) => {}
                _ => {
                    acc.root = Some(Root {
                        start_ns,
                        span_id,
                        kind,
                        service,
                        name,
                    });
                    acc.root_ambiguous = false;
                }
            }
        }
    }

    /// Distinct set trace ids accumulated so far.
    pub fn num_traces(&self) -> usize {
        self.map.len()
    }

    /// Whether the chunk would carry any rows (the producer skips the
    /// chunk otherwise — an empty rollup is never written).
    pub fn is_meaningful(&self) -> bool {
        !self.map.is_empty()
    }

    /// Slot→file-id translation + trace-id sort (build phase 2 — the
    /// same `kv_to_file` table every stream batch and structure rides).
    pub(crate) fn sealed(&self, kv_to_file: &[KvId]) -> TraceRollup {
        let translate = |slot: &Option<KvSlot>| -> u32 {
            slot.as_ref()
                .map(|s| kv_to_file[s.idx()].0)
                .unwrap_or(ROLLUP_NO_REF)
        };
        let mut ids: Vec<&TraceId> = self.map.keys().collect();
        ids.sort_unstable();

        let mut out = TraceRollup::default();
        for id in ids {
            let acc = &self.map[id];
            out.trace_ids.push(*id);
            out.min_start_ns.push(acc.min_start_ns);
            out.max_end_ns.push(acc.max_end_ns);
            out.span_counts.push(acc.span_count);
            out.error_counts.push(acc.error_count);
            match &acc.root {
                // An ambiguous tie abstains — sentinel fields, but the
                // WITHHELD flag keeps it distinct from proven absence
                // (a pruning consumer may act on NONE, never on this).
                Some(_) if acc.root_ambiguous => {
                    out.root_span_ids.push(SpanId::UNSET);
                    out.root_kinds.push(0);
                    out.root_is_true_root.push(ROOT_CLAIM_WITHHELD);
                    out.root_service_refs.push(ROLLUP_NO_REF);
                    out.root_name_refs.push(ROLLUP_NO_REF);
                }
                Some(r) => {
                    out.root_span_ids.push(r.span_id);
                    out.root_kinds.push(r.kind);
                    out.root_is_true_root.push(ROOT_CLAIM_TRUE);
                    out.root_service_refs.push(translate(&r.service));
                    out.root_name_refs.push(translate(&r.name));
                }
                None => {
                    out.root_span_ids.push(SpanId::UNSET);
                    out.root_kinds.push(0);
                    out.root_is_true_root.push(ROOT_CLAIM_NONE);
                    out.root_service_refs.push(ROLLUP_NO_REF);
                    out.root_name_refs.push(ROLLUP_NO_REF);
                }
            }
        }
        out
    }
}

#[cfg(test)]
mod tests {
    use super::*;

    fn tid(b: u8) -> TraceId {
        TraceId::from([b; 16])
    }
    fn sid(b: u8) -> SpanId {
        SpanId::from([b; 8])
    }

    #[test]
    fn unset_trace_ids_are_excluded_and_counts_are_stored_rows() {
        let mut rows = TraceRollupRows::new();
        rows.record_span(TraceId::UNSET, sid(1), true, 10, 5, 1, false, None, None);
        // The same span stored twice (a resend) counts twice.
        rows.record_span(tid(1), sid(1), true, 10, 5, 1, true, None, None);
        rows.record_span(tid(1), sid(1), true, 10, 5, 1, true, None, None);
        assert_eq!(rows.num_traces(), 1);
        let sealed = rows.sealed(&[]);
        assert_eq!(sealed.span_counts, vec![2]);
        assert_eq!(sealed.error_counts, vec![2]);
    }

    #[test]
    fn envelope_folds_min_start_and_saturating_max_end() {
        let mut rows = TraceRollupRows::new();
        rows.record_span(tid(1), sid(1), false, 100, 50, 0, false, None, None);
        rows.record_span(tid(1), sid(2), false, 20, 10, 0, false, None, None);
        rows.record_span(tid(1), sid(3), false, i64::MAX - 1, 10, 0, false, None, None);
        let sealed = rows.sealed(&[]);
        assert_eq!(sealed.min_start_ns, vec![20]);
        assert_eq!(sealed.max_end_ns, vec![i64::MAX], "saturating end");
    }

    #[test]
    fn earliest_unset_parent_span_wins_the_root_and_absence_is_honest() {
        let mut rows = TraceRollupRows::new();
        // No unset-parent span → no true root, sentinels everywhere.
        rows.record_span(tid(1), sid(1), false, 10, 5, 2, false, None, None);
        // A true root, then an EARLIER true root displaces it (the
        // summary_root convention); a later one does not.
        rows.record_span(tid(2), sid(5), true, 50, 5, 2, false, None, None);
        rows.record_span(tid(2), sid(3), true, 30, 5, 3, false, None, None);
        rows.record_span(tid(2), sid(9), true, 90, 5, 4, false, None, None);

        let sealed = rows.sealed(&[]);
        // Rows sort by trace id: tid(1) first.
        assert_eq!(sealed.root_is_true_root, vec![0, 1]);
        assert!(sealed.root_span_ids.get(0).is_unset());
        assert_eq!(sealed.root_kinds[0], 0);
        assert_eq!(sealed.root_service_refs[0], ROLLUP_NO_REF);
        assert_eq!(sealed.root_span_ids.get(1), sid(3));
        assert_eq!(sealed.root_kinds[1], 3);
    }

    #[test]
    fn equal_start_root_ties_break_by_ascending_span_id() {
        // Deterministic regardless of storage order — the combiner total
        // order's next key after start.
        let mut a = TraceRollupRows::new();
        a.record_span(tid(1), sid(7), true, 100, 1, 0, false, None, None);
        a.record_span(tid(1), sid(3), true, 100, 1, 0, false, None, None);
        let mut b = TraceRollupRows::new();
        b.record_span(tid(1), sid(3), true, 100, 1, 0, false, None, None);
        b.record_span(tid(1), sid(7), true, 100, 1, 0, false, None, None);
        assert_eq!(a.sealed(&[]).root_span_ids.get(0), sid(3));
        assert_eq!(b.sealed(&[]).root_span_ids.get(0), sid(3));
    }

    #[test]
    fn sealed_translates_root_slots_through_the_kv_table() {
        use crate::kv_interner::KvSlot;
        let mut rows = TraceRollupRows::new();
        rows.record_span(
            tid(1),
            sid(1),
            true,
            10,
            5,
            2,
            false,
            Some(KvSlot(0)),
            Some(KvSlot(2)),
        );
        // Slot 0 → file id 7; slot 2 → file id 9.
        let table = [KvId(7), KvId(8), KvId(9)];
        let sealed = rows.sealed(&table);
        assert_eq!(sealed.root_service_refs, vec![7]);
        assert_eq!(sealed.root_name_refs, vec![9]);
    }

    #[test]
    fn rows_emit_sorted_by_trace_id() {
        let mut rows = TraceRollupRows::new();
        rows.record_span(tid(9), sid(1), false, 1, 1, 0, false, None, None);
        rows.record_span(tid(2), sid(1), false, 1, 1, 0, false, None, None);
        rows.record_span(tid(5), sid(1), false, 1, 1, 0, false, None, None);
        let sealed = rows.sealed(&[]);
        let ids: Vec<TraceId> = (0..sealed.len()).map(|i| sealed.trace_ids.get(i)).collect();
        assert_eq!(ids, vec![tid(2), tid(5), tid(9)]);
        assert_eq!(sealed.len(), 3);
        assert!(!sealed.is_empty());
    }

    #[test]
    fn full_tie_with_differing_facets_abstains_in_both_orders() {
        // Same (start, span_id), different kind: the recorder cannot
        // know the combiner's deeper tie-break — no claim, either way.
        for flip in [false, true] {
            let mut rows = TraceRollupRows::new();
            let (a, b) = ((2, false), (3, true));
            let (first, second) = if flip { (b, a) } else { (a, b) };
            rows.record_span(tid(1), sid(7), true, 100, 5, first.0, first.1, None, None);
            rows.record_span(tid(1), sid(7), true, 100, 5, second.0, second.1, None, None);
            let sealed = rows.sealed(&[]);
            assert_eq!(sealed.root_is_true_root, vec![ROOT_CLAIM_WITHHELD], "flip={flip}");
            assert!(sealed.root_span_ids.get(0).is_unset());
            assert_eq!(sealed.root_service_refs[0], ROLLUP_NO_REF);
            // The abstention drops only the CLAIM, not the row stats.
            assert_eq!(sealed.span_counts, vec![2]);
        }
    }

    #[test]
    fn full_tie_with_identical_facets_keeps_the_claim() {
        // A plain resend (same kind/service/name) is not ambiguous.
        let mut rows = TraceRollupRows::new();
        rows.record_span(tid(1), sid(7), true, 100, 5, 2, false, None, None);
        rows.record_span(tid(1), sid(7), true, 100, 5, 2, false, None, None);
        let sealed = rows.sealed(&[]);
        assert_eq!(sealed.root_is_true_root, vec![1]);
        assert_eq!(sealed.root_span_ids.get(0), sid(7));
    }

    #[test]
    fn strictly_earlier_span_revives_an_ambiguous_claim() {
        let mut rows = TraceRollupRows::new();
        rows.record_span(tid(1), sid(7), true, 100, 5, 2, false, None, None);
        rows.record_span(tid(1), sid(7), true, 100, 5, 3, false, None, None);
        // A strictly earlier root candidate is unambiguous again.
        rows.record_span(tid(1), sid(9), true, 50, 5, 4, false, None, None);
        let sealed = rows.sealed(&[]);
        assert_eq!(sealed.root_is_true_root, vec![1]);
        assert_eq!(sealed.root_span_ids.get(0), sid(9));
        assert_eq!(sealed.root_kinds[0], 4);
    }

    #[test]
    fn find_hits_every_row_and_misses_between_and_beyond() {
        let mut rows = TraceRollupRows::new();
        rows.record_span(tid(9), sid(1), false, 1, 1, 0, false, None, None);
        rows.record_span(tid(2), sid(1), false, 1, 1, 0, false, None, None);
        rows.record_span(tid(5), sid(1), false, 1, 1, 0, false, None, None);
        let sealed = rows.sealed(&[]);
        assert_eq!(sealed.find(tid(2)), Some(0));
        assert_eq!(sealed.find(tid(5)), Some(1));
        assert_eq!(sealed.find(tid(9)), Some(2));
        assert_eq!(sealed.find(tid(1)), None, "below the first row");
        assert_eq!(sealed.find(tid(3)), None, "between rows");
        assert_eq!(sealed.find(tid(200)), None, "past the last row");
        assert_eq!(TraceRollup::default().find(tid(2)), None, "empty rollup");
    }

    #[test]
    fn validate_rejects_each_corruption_class() {
        let mut rows = TraceRollupRows::default();
        rows.record_span(
            TraceId::from([1u8; 16]),
            SpanId::from([1u8; 8]),
            true,
            1,
            1,
            0,
            false,
            None,
            None,
        );
        rows.record_span(
            TraceId::from([2u8; 16]),
            SpanId::from([2u8; 8]),
            true,
            2,
            1,
            0,
            false,
            None,
            None,
        );
        let good = rows.sealed(&[]);
        assert!(good.validate(0).is_ok(), "the sealed shape passes");

        let mut broken = good.clone();
        broken.span_counts.pop();
        assert!(broken.validate(0).is_err(), "non-parallel arrays");

        let mut broken = good.clone();
        broken.root_service_refs[0] = 5; // kv_total is 0 → out of range
        assert!(broken.validate(0).is_err(), "out-of-range ref");

        let mut broken = good.clone();
        broken.root_is_true_root[0] = 3;
        assert!(broken.validate(0).is_err(), "flag beyond the tri-state");

        let mut broken = good.clone();
        let dup = broken.trace_ids.get(0);
        broken.trace_ids = TraceIds::default();
        broken.trace_ids.push(dup);
        broken.trace_ids.push(dup);
        assert!(broken.validate(0).is_err(), "duplicate ids");

        // Internal contradictions a pruning consumer would act on: a
        // flipped claim flag beside real root fields, and an inverted
        // envelope. (A CONSISTENT lie stays outside the trust model.)
        let mut broken = good.clone();
        broken.root_is_true_root[0] = ROOT_CLAIM_NONE; // fields still set
        assert!(
            broken.validate(0).is_err(),
            "non-claiming row with root fields"
        );

        let mut broken = good.clone();
        broken.min_start_ns[0] = broken.max_end_ns[0] + 1;
        assert!(broken.validate(0).is_err(), "inverted envelope");

        let mut broken = good.clone();
        broken.span_counts[0] = 0;
        broken.error_counts[0] = 0;
        assert!(broken.validate(0).is_err(), "zero-span row");

        let mut broken = good.clone();
        broken.error_counts[0] = broken.span_counts[0] + 1;
        assert!(broken.validate(0).is_err(), "error count exceeds spans");

        // An id arena with trailing bytes: len() floors to the right
        // count, so only the well_formed() check catches it — the ids
        // stay distinct and increasing so every OTHER check passes.
        let mut broken = good.clone();
        let mut arena = vec![1u8; 16]; // id 1
        arena.extend_from_slice(&[2u8; 16]); // id 2 (strictly greater)
        arena.push(0xAA); // the trailing byte
        let raw = bincode::serde::encode_to_vec(
            serde_bytes::ByteBuf::from(arena),
            bincode::config::standard(),
        )
        .unwrap();
        broken.trace_ids = bincode::serde::decode_from_slice(&raw, bincode::config::standard())
            .unwrap()
            .0;
        assert!(broken.validate(0).is_err(), "trailing arena bytes");
    }

    /// The dependency guard: a TRSU chunk in a file whose manifest lacks
    /// the TRCE column — the reader must reject the rollup
    /// (require_column), not let a missing row prove "trace absent".
    /// Same shape as `reader_rejects_bloom_without_trace_column`.
    #[test]
    fn reader_rejects_rollup_without_trace_column() {
        use crate::{ColumnsTable, Histogram, IdRanges, Metadata, SchemaTree, Summary};

        let lvl = crate::ZSTD_LEVEL_DEFAULT;
        let summary = Summary {
            min_timestamp_s: 1,
            max_timestamp_s: 1,
            record_count: 1,
            content_meta: Vec::new(),
        };
        let metadata = Metadata {
            histogram: Histogram {
                timestamps: vec![1],
                counts: vec![1],
            },
            id_ranges: IdRanges {
                low_end: crate::KvId(0),
                mid_end: crate::KvId(0),
                high_end: crate::KvId(0),
            },
            tree: SchemaTree::flat(&Vec::new().into()),
            columns: ColumnsTable::default(), // no TRCE in the manifest
        };
        let prim = crate::PrefixMap::<crate::BitmapValue>::build(
            Vec::<(&str, crate::BitmapValue)>::new(),
        )
        .unwrap();

        let mut w = chunk_file::container::StreamingWriter::new(
            std::io::Cursor::new(Vec::new()),
            *crate::MAGIC,
            crate::VERSION,
            6,
        )
        .unwrap();
        w.write_chunk(crate::CHUNK_SUMMARY, &crate::writer::pack(&summary, lvl).unwrap())
            .unwrap();
        w.write_chunk(crate::CHUNK_META, &crate::writer::pack(&metadata, lvl).unwrap())
            .unwrap();
        w.write_chunk(crate::CHUNK_TIMS, &crate::writer::pack(&[1i64][..], lvl).unwrap())
            .unwrap();
        w.write_chunk(crate::CHUNK_PRIMARY, &crate::writer::pack(&prim, lvl).unwrap())
            .unwrap();
        w.write_chunk(
            crate::CHUNK_TRACE_ROLLUP,
            &crate::writer::pack(&TraceRollup::default(), lvl).unwrap(),
        )
        .unwrap();
        w.write_chunk(
            crate::stream_batch_id(0),
            &crate::writer::pack(&crate::StreamBatch::for_write(&[]), lvl).unwrap(),
        )
        .unwrap();
        let buf = w.finish().unwrap().into_inner();

        let reader = crate::reader::ChunkReader::open(&buf).unwrap();
        assert!(reader.has_trace_rollup());
        assert!(
            matches!(reader.trace_rollup(), Err(crate::Error::ColumnMismatch(_))),
            "missing TRCE column rejected"
        );
    }
}
