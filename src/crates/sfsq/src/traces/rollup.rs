//! Trace-level aggregates WITHOUT assembly (traces-ui phase 2): the
//! neutral per-trace shape the cross-source folds (overview v2,
//! slowest, root facets — steps 2.3+) consume from BOTH source kinds:
//!
//! - **Sealed files / chunks** carry the `TRSU` rollup chunk
//!   ([`sfst::TraceRollup`]); [`sealed_trace_aggregates`] resolves its
//!   interner refs to strings through the file's own string table.
//! - **WAL tails** have no interner; [`tail_trace_aggregates`] folds the
//!   scan's decoded spans directly, with the SAME pinned semantics the
//!   seal accumulator applies (D8/D9):
//!   - stored-row counts (a resent span counts every time it is stored;
//!     canonical dedup belongs to assembly),
//!   - honest-or-absent roots (only a genuinely unset-parent span;
//!     earliest wins, equal starts tie-break by ascending span id),
//!   - the all-zero UNSET trace id excluded,
//!   - envelope end saturating.
//!
//! Parity contract (test-pinned): folding a tail equals reading the
//! `TRSU` of sealing the same data.

use std::collections::HashMap;

use super::vocab::{AttributeOwner, BuiltinField};
use super::wal_scan::TraceWalScan;

/// One trace's per-source aggregate, source-shape-neutral.
#[derive(Debug, Clone, PartialEq, Eq)]
pub struct TraceAggregate {
    pub trace_id: sfst::TraceId,
    /// Envelope start: min stored span start, ns.
    pub min_start_ns: i64,
    /// Envelope end: max stored `start ⊕ duration` (saturating), ns.
    pub max_end_ns: i64,
    /// Stored spans in THIS source (D9 — resends included).
    pub span_count: u64,
    /// Of those, spans with ERROR status.
    pub error_count: u64,
    /// The TRUE root's fields, when this source stored one (D8);
    /// `None` is honest absence — never synthesize a root from it.
    pub root: Option<TraceRootInfo>,
}

/// The true root's display fields.
#[derive(Debug, Clone, PartialEq, Eq)]
pub struct TraceRootInfo {
    pub span_id: sfst::SpanId,
    /// Raw OTLP span kind.
    pub kind: i32,
    /// The root's resource `service.name`, when it carries one.
    pub service: Option<String>,
    /// The root's span name, when it carries one.
    pub name: Option<String>,
}

/// Fold a WAL tail's decoded spans into per-trace aggregates, sorted by
/// trace id (the sealed rows' order).
pub fn tail_trace_aggregates(scan: &TraceWalScan) -> Vec<TraceAggregate> {
    // Storage names via the vocabulary — never hand-built (the same
    // spellings the seal-side capture and `summarize` use).
    let service_field = format!(
        "{}service.name",
        AttributeOwner::Resource
            .attribute_prefix()
            .expect("Resource is an attribute owner")
    );
    let name_field = BuiltinField::Name
        .dictionary_field()
        .expect("Name is dictionary-backed");
    let status_field = BuiltinField::Status
        .dictionary_field()
        .expect("Status is dictionary-backed");
    let field_of = |span: &sfst::TraceSpan, field: &str| -> Option<String> {
        span.fields
            .iter()
            .find(|(k, _)| k == field)
            .map(|(_, v)| v.clone())
    };

    struct Acc {
        min_start_ns: i64,
        max_end_ns: i64,
        span_count: u64,
        error_count: u64,
        /// (start_ns, span_id) of the current root pick — the tie-break key.
        root_key: Option<(i64, sfst::SpanId)>,
        root: Option<TraceRootInfo>,
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
            root_key: None,
            root: None,
        });
        acc.min_start_ns = acc.min_start_ns.min(span.start_ns);
        acc.max_end_ns = acc.max_end_ns.max(end_ns);
        acc.span_count += 1;
        if field_of(span, status_field).as_deref() == Some("ERROR") {
            acc.error_count += 1;
        }
        // D8 + the seal accumulator's exact rule: earliest unset-parent
        // span wins; equal starts tie-break by ascending span id.
        let key = (span.start_ns, span.span_id);
        if span.parent_span_id.is_unset() && acc.root_key.is_none_or(|k| key < k) {
            acc.root_key = Some(key);
            acc.root = Some(TraceRootInfo {
                span_id: span.span_id,
                kind: span.kind,
                service: field_of(span, &service_field),
                name: field_of(span, name_field),
            });
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
            root: acc.root,
        })
        .collect();
    out.sort_unstable_by_key(|a| a.trace_id);
    out
}

/// Resolve a sealed file's `TRSU` rows into the neutral shape. `strings`
/// is the file's `KvId → key=value` table
/// ([`sfst::IndexReader::build_string_table`]); refs resolve to the
/// VALUE half (the `kv_value` convention: split on the first `=`).
pub fn sealed_trace_aggregates(
    rollup: &sfst::TraceRollup,
    strings: &[String],
) -> Vec<TraceAggregate> {
    let value_of = |r: u32| -> Option<String> {
        if r == sfst::ROLLUP_NO_REF {
            return None;
        }
        strings
            .get(r as usize)
            .and_then(|s| s.split_once('=').map(|(_, v)| v.to_string()))
    };
    (0..rollup.len())
        .map(|i| TraceAggregate {
            trace_id: rollup.trace_ids.get(i),
            min_start_ns: rollup.min_start_ns[i],
            max_end_ns: rollup.max_end_ns[i],
            span_count: u64::from(rollup.span_counts[i]),
            error_count: u64::from(rollup.error_counts[i]),
            root: (rollup.root_is_true_root[i] == 1).then(|| TraceRootInfo {
                span_id: rollup.root_span_ids.get(i),
                kind: rollup.root_kinds[i],
                service: value_of(rollup.root_service_refs[i]),
                name: value_of(rollup.root_name_refs[i]),
            }),
        })
        .collect()
}
