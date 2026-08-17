//! Trace-level aggregates WITHOUT assembly: the neutral per-trace
//! shape the cross-source folds (overview, slowest, root facets)
//! consume from BOTH source kinds:
//!
//! - **Sealed files / chunks** carry the `TRSU` rollup chunk
//!   ([`sfst::TraceRollup`]); [`sealed_trace_aggregates`] resolves its
//!   interner refs through the validating [`sfst::RollupRootResolver`]
//!   (a corrupt ref fails the source, never renders a wrong root).
//! - **WAL tails** have no interner; [`tail_trace_aggregates`] folds the
//!   scan's decoded spans directly, with the SAME pinned semantics the
//!   seal accumulator applies:
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

use super::vocab::{BuiltinField, resource_service_field, span_field};
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
    /// The TRUE root's fields, when this source stored one;
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
    let service_field = resource_service_field();
    let name_field = BuiltinField::Name
        .dictionary_field()
        .expect("Name is dictionary-backed");
    let status_field = BuiltinField::Status
        .dictionary_field()
        .expect("Status is dictionary-backed");

    struct Acc {
        min_start_ns: i64,
        max_end_ns: i64,
        span_count: u64,
        error_count: u64,
        /// (start_ns, span_id) of the current root pick — the tie-break key.
        root_key: Option<(i64, sfst::SpanId)>,
        root: Option<TraceRootInfo>,
        /// The incumbent tied a candidate with different facets — the
        /// seal accumulator's abstention, mirrored for parity.
        root_ambiguous: bool,
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
            root_ambiguous: false,
        });
        acc.min_start_ns = acc.min_start_ns.min(span.start_ns);
        acc.max_end_ns = acc.max_end_ns.max(end_ns);
        // Counts clamp at u32::MAX — the sealed rows' width — so the
        // parity contract holds even at the (astronomical) wrap point.
        acc.span_count = (acc.span_count + 1).min(u64::from(u32::MAX));
        if span_field(span, status_field) == Some("ERROR") {
            acc.error_count = (acc.error_count + 1).min(u64::from(u32::MAX));
        }
        // The seal accumulator's exact rule: earliest unset-parent
        // span wins; equal starts tie-break by ascending span id; a
        // FULL-key tie with differing facets ABSTAINS (mirrors the
        // seal's ambiguity state machine — the tail/seal parity
        // contract covers the abstention too).
        if span.parent_span_id.is_unset() {
            let key = (span.start_ns, span.span_id);
            let candidate = || TraceRootInfo {
                span_id: span.span_id,
                kind: span.kind,
                service: span_field(span, &service_field).map(str::to_string),
                name: span_field(span, name_field).map(str::to_string),
            };
            match acc.root_key {
                Some(k) if key == k => {
                    if acc.root.as_ref() != Some(&candidate()) {
                        acc.root_ambiguous = true;
                    }
                }
                Some(k) if key > k => {}
                _ => {
                    acc.root_key = Some(key);
                    acc.root = Some(candidate());
                    acc.root_ambiguous = false;
                }
            }
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
            root: if acc.root_ambiguous { None } else { acc.root },
        })
        .collect();
    out.sort_unstable_by_key(|a| a.trace_id);
    out
}

/// The envelope-and-counts view of a sealed file's `TRSU` rows — the
/// grid path. `root` is always `None` here (UNRESOLVED, not honest-absent):
/// resolving roots needs the root-field dictionaries decoded, and the
/// overview grid discards roots anyway. Root-consuming callers (slowest,
/// facets) use [`sealed_trace_aggregates`].
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
            root: None,
        })
        .collect()
}

/// Resolve a sealed file's `TRSU` rows into the neutral shape. Root refs
/// resolve through [`sfst::RollupRootResolver`] — the same validating,
/// closed-partition seam the trace-level gate uses: a ref is either the
/// target field's proven value or corruption evidence for the whole
/// file. A `Corrupt` outcome escalates as [`sfst::Error::CorruptIndex`]
/// so the caller marks the source failed (never a silently wrong or
/// absent root — a bare string-table lookup here would render another
/// field's value as the root on a corrupted in-range ref).
///
/// Precondition: `rollup` comes from `IndexReader::trace_rollup()` (the
/// validating accessor) — the struct-of-arrays fields are indexed in
/// parallel here on that guarantee.
pub fn sealed_trace_aggregates(
    rollup: &sfst::TraceRollup,
    reader: &sfst::IndexReader<'_>,
) -> Result<Vec<TraceAggregate>, sfst::Error> {
    let service_field = resource_service_field();
    let name_field = BuiltinField::Name
        .dictionary_field()
        .expect("Name is dictionary-backed");
    let mut resolver = sfst::RollupRootResolver::new(reader);
    let mut value_of = |field_name: &str, r: u32| -> Result<Option<String>, sfst::Error> {
        if r == sfst::ROLLUP_NO_REF {
            return Ok(None);
        }
        match resolver.resolve(field_name, r) {
            sfst::RollupRefOutcome::Value(v) => Ok(Some(v.to_string())),
            sfst::RollupRefOutcome::Corrupt => Err(sfst::Error::CorruptIndex(format!(
                "trace rollup root ref {r} does not resolve in `{field_name}`"
            ))),
        }
    };
    let mut out = Vec::with_capacity(rollup.len());
    for i in 0..rollup.len() {
        let root = if rollup.root_is_true_root[i] == sfst::ROOT_CLAIM_TRUE {
            Some(TraceRootInfo {
                span_id: rollup.root_span_ids.get(i),
                kind: rollup.root_kinds[i],
                service: value_of(&service_field, rollup.root_service_refs[i])?,
                name: value_of(name_field, rollup.root_name_refs[i])?,
            })
        } else {
            None
        };
        out.push(TraceAggregate {
            trace_id: rollup.trace_ids.get(i),
            min_start_ns: rollup.min_start_ns[i],
            max_end_ns: rollup.max_end_ns[i],
            span_count: u64::from(rollup.span_counts[i]),
            error_count: u64::from(rollup.error_counts[i]),
            root,
        });
    }
    Ok(out)
}
