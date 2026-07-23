//! The optional per-file trace rollup (`TRSU` chunk): one row per
//! distinct set (non-UNSET) trace id in the file — the trace-level
//! aggregate the traces overview/slowest/facet queries fold WITHOUT
//! assembling traces (traces-ui design §5 phase 2, decisions D6–D10).
//!
//! Semantics (pinned):
//!
//! - **Stored-row statistics (D9)**: counts count what is on disk. A
//!   resent span counts every time it is stored — the canonical
//!   `(span_id, kind)` dedup belongs to assembly (`trace_combine`),
//!   deliberately NOT replicated here. Consumers label the numbers so.
//! - **Root honest-or-absent (D8)**: root fields are set only from a
//!   span with a genuinely UNSET parent seen in THIS file (the earliest
//!   such span wins, mirroring `Trace::summary_root`'s convention);
//!   otherwise `root_is_true_root` is false and the root fields are
//!   sentinels. A consumer NEVER synthesizes a root from them.
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
    /// Stored spans of this trace in THIS file (D9 — resends included).
    pub span_counts: Vec<u32>,
    /// Of those, spans with ERROR status.
    pub error_counts: Vec<u32>,
    /// The true root's raw OTLP span kind; 0 when no true root.
    pub root_kinds: Vec<i32>,
    /// 1 when a genuinely unset-parent span of this trace exists in this
    /// file (its fields populate the root columns); else 0.
    pub root_is_true_root: Vec<u8>,
    /// File-interner [`KvId`] of the root's resource `service.name`
    /// entry, or [`ROLLUP_NO_REF`].
    pub root_service_refs: Vec<u32>,
    /// File-interner [`KvId`] of the root's `name` entry, or
    /// [`ROLLUP_NO_REF`].
    pub root_name_refs: Vec<u32>,
}

impl TraceRollup {
    /// Number of distinct set trace ids (rows).
    pub fn len(&self) -> usize {
        self.min_start_ns.len()
    }

    pub fn is_empty(&self) -> bool {
        self.min_start_ns.is_empty()
    }
}

/// One trace's in-progress aggregate (insertion phase).
#[derive(Debug, Clone)]
struct Acc {
    min_start_ns: i64,
    max_end_ns: i64,
    span_count: u32,
    error_count: u32,
    /// The earliest unset-parent span seen so far, when any (D8).
    root: Option<Root>,
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
        });
        acc.min_start_ns = acc.min_start_ns.min(start_ns);
        acc.max_end_ns = acc.max_end_ns.max(end_ns);
        acc.span_count = acc.span_count.saturating_add(1);
        if is_error {
            acc.error_count = acc.error_count.saturating_add(1);
        }
        // D8: the earliest genuinely-unset-parent span wins the root
        // (the summary_root convention). Equal starts tie-break by
        // ascending span id — the combiner total order's next key — so
        // the pick is deterministic regardless of storage order.
        if parent_unset
            && acc
                .root
                .as_ref()
                .is_none_or(|r| (start_ns, span_id) < (r.start_ns, r.span_id))
        {
            acc.root = Some(Root {
                start_ns,
                span_id,
                kind,
                service,
                name,
            });
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
                Some(r) => {
                    out.root_span_ids.push(r.span_id);
                    out.root_kinds.push(r.kind);
                    out.root_is_true_root.push(1);
                    out.root_service_refs.push(translate(&r.service));
                    out.root_name_refs.push(translate(&r.name));
                }
                None => {
                    out.root_span_ids.push(SpanId::UNSET);
                    out.root_kinds.push(0);
                    out.root_is_true_root.push(0);
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
        // The same span stored twice (a resend) counts twice — D9.
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
        // A true root, then an EARLIER true root displaces it (D8 /
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
}
