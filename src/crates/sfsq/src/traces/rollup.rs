//! Trace-level aggregates WITHOUT assembly: the neutral per-trace
//! shape the cross-source fold (overview) consumes from BOTH
//! source kinds:
//!
//! - **Sealed files / chunks** carry the `TRSU` rollup chunk
//!   (`sfst::TraceRollup`, folded per file by the indexer's span
//!   walk — `src/crates/ng-index/src/sfst_build.rs`), read through
//!   [`sealed_trace_envelopes`].
//! - **WAL tails** have no rollup; [`tail_trace_aggregates`] folds the
//!   scan's decoded spans directly, with the SAME pinned semantics the
//!   seal accumulator applies:
//!   - stored-row counts (a resent span counts every time it is stored;
//!     canonical dedup belongs to assembly),
//!   - the all-zero UNSET trace id excluded,
//!   - envelope end saturating.
//!
//! Parity contract (test-pinned, `tests/traces_rollup_tail.rs`):
//! folding a tail equals reading the `TRSU` of sealing the same data.
//! The rollup's root columns serve the search gate
//! (`sfst::RollupRootResolver`), not these aggregates.

use std::collections::HashMap;

use super::vocab::{BuiltinField, span_field};
use super::wal_scan::TraceWalScan;

/// One trace's per-source aggregate, source-shape-neutral.
#[derive(Debug, Clone, PartialEq, Eq)]
pub struct TraceAggregate {
    pub trace_id: sfst::TraceId,
    /// Envelope start: min stored span start, ns.
    pub min_start_ns: i64,
    /// Envelope end: max stored `start ⊕ duration` (saturating), ns.
    pub max_end_ns: i64,
    /// Stored spans in THIS source (resends included).
    pub span_count: u64,
    /// Of those, spans with ERROR status.
    pub error_count: u64,
}

/// Fold a WAL tail's decoded spans into per-trace aggregates, sorted by
/// trace id (the sealed rows' order).
pub fn tail_trace_aggregates(scan: &TraceWalScan) -> Vec<TraceAggregate> {
    // Storage name via the vocabulary — never hand-built (the same
    // spelling the seal-side capture and `summarize` use).
    let status_field = BuiltinField::Status
        .dictionary_field()
        .expect("Status is dictionary-backed");

    struct Acc {
        min_start_ns: i64,
        max_end_ns: i64,
        span_count: u64,
        error_count: u64,
    }

    let mut map: HashMap<sfst::TraceId, Acc> = HashMap::new();
    for (trace_id, span) in scan.spans_with_ids() {
        if trace_id.is_unset() {
            continue; // the TIDX/TRSU rule: the unset sentinel is not a trace
        }
        let end_ns = span.start_ns.saturating_add(span.duration_ns.max(0));
        let acc = map.entry(trace_id).or_insert(Acc {
            min_start_ns: i64::MAX,
            max_end_ns: i64::MIN,
            span_count: 0,
            error_count: 0,
        });
        acc.min_start_ns = acc.min_start_ns.min(span.start_ns);
        acc.max_end_ns = acc.max_end_ns.max(end_ns);
        // Counts clamp at u32::MAX — the TRSU count columns' width,
        // where the seal saturates too — so the parity contract holds
        // even at the (astronomical) saturation point.
        acc.span_count = (acc.span_count + 1).min(u64::from(u32::MAX));
        if span_field(span, status_field) == Some("ERROR") {
            acc.error_count = (acc.error_count + 1).min(u64::from(u32::MAX));
        }
    }

    let mut out: Vec<TraceAggregate> = map
        .into_iter()
        .map(|(trace_id, acc)| TraceAggregate {
            trace_id,
            min_start_ns: acc.min_start_ns,
            max_end_ns: acc.max_end_ns,
            span_count: acc.span_count,
            error_count: acc.error_count,
        })
        .collect();
    out.sort_unstable_by_key(|a| a.trace_id);
    out
}

/// The envelope-and-counts view of a sealed file's `TRSU` rows.
///
/// Precondition: `rollup` comes from `IndexReader::trace_rollup()` (the
/// validating accessor) — the struct-of-arrays fields are indexed in
/// parallel here on that guarantee.
pub fn sealed_trace_envelopes(rollup: &sfst::TraceRollup) -> Vec<TraceAggregate> {
    (0..rollup.len())
        .map(|i| TraceAggregate {
            trace_id: rollup.trace_ids.get(i),
            min_start_ns: rollup.min_start_ns[i],
            max_end_ns: rollup.max_end_ns[i],
            span_count: u64::from(rollup.span_counts[i]),
            error_count: u64::from(rollup.error_counts[i]),
        })
        .collect()
}
