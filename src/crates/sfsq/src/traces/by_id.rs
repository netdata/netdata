//! Cross-source trace-by-id: assemble ONE trace from many sources.
//!
//! [`trace_by_id`] walks a validated set of [`TraceSource`]s (sealed
//! SFSTs, in-memory chunk SFSTs, WAL tails) through the shared combiner
//! (`sfst::trace_combine`): per-file sessions decode the TBLM bloom once
//! and test the id against it, surviving sources feed lightweight span
//! refs into the comparator-ordered k-way merge, and payloads
//! materialize only as the merge accepts them.
//!
//! Status honesty (the design-record contract): a source that fails to
//! map, open, or decode surfaces as a
//! [`SourceFailure`](PartialReason::SourceFailure) reason on the
//! query-level [`QueryStatus`], and an unavailable source as
//! [`RemoteUnavailable`](PartialReason::RemoteUnavailable) — never a
//! silent skip. Cancellation before all source heads are resolved
//! returns an EMPTY result with
//! [`Cancelled`](PartialReason::Cancelled); cancellation during the
//! merge returns the deterministic merged prefix. The span cap yields
//! [`SizeCap`](PartialReason::SizeCap) keeping the globally earliest
//! spans (cap+1 detection — an exactly-cap trace is `Complete`).
//!
//! The result also carries a field→schema-kind map, so consumers type
//! values from the declared schema kinds instead of inferring them from
//! rendered strings. Its sectioning and the cross-source coalescing
//! rule (the [`sfst::join_value_kinds`] lattice over exactly the
//! sources that contributed retained spans) live on [`FieldKinds`].

use std::collections::BTreeMap;
use std::sync::Arc;
use std::sync::atomic::{AtomicUsize, Ordering};

use tokio_util::sync::CancellationToken;

use sfst::trace_combine::{SpanSource, combine};

use super::sources::{SourceSetError, TraceSource, validate_sources};
use super::status::{PartialReason, QueryStatus, StatusBuilder};
use super::wal_scan::TraceWalScan;
use crate::source::map_source;

/// The library default span cap ([`TraceQuery::new`] applies it): far
/// above any honest trace, low enough that a runaway merge stays bounded.
pub const DEFAULT_SPAN_CAP: usize = 65_536;

/// A trace-by-id request in engine terms. [`new`](Self::new) applies the
/// default span cap; [`span_cap`](Self::span_cap) overrides it and
/// [`unbounded`](Self::unbounded) removes it.
#[derive(Debug, Clone)]
pub struct TraceQuery {
    trace_id: sfst::TraceId,
    span_cap: Option<usize>,
}

impl TraceQuery {
    pub fn new(trace_id: sfst::TraceId) -> Self {
        Self {
            trace_id,
            span_cap: Some(DEFAULT_SPAN_CAP),
        }
    }

    /// Override the span cap. Zero is rejected at [`trace_by_id`]'s
    /// request validation.
    pub fn span_cap(mut self, cap: usize) -> Self {
        self.span_cap = Some(cap);
        self
    }

    /// Remove the span cap entirely.
    pub fn unbounded(mut self) -> Self {
        self.span_cap = None;
        self
    }
}

/// A request error — the caller built an invalid request or source set.
/// Distinct from a [`Partial`](QueryStatus::Partial) result: nothing was
/// queried.
#[derive(Debug, thiserror::Error)]
pub enum TraceRequestError {
    /// The all-zero trace id is the OTLP "unset/invalid" sentinel: TIDX
    /// never indexes it (`sfst/src/trace_index.rs` `TraceIdIndex::build`
    /// excludes unset ids) while a tail scan would still serve its
    /// spans — the answer would depend on data layout.
    #[error("the all-zero (unset) trace id cannot be looked up")]
    UnsetTraceId,
    /// A zero cap accepts nothing and truncates on the first group —
    /// the answer would be a guaranteed-empty `SizeCap` partial.
    #[error("a zero span cap would return nothing; omit the cap or raise it")]
    ZeroSpanCap,
    #[error(transparent)]
    SourceSet(#[from] SourceSetError),
}

/// The coalesced field→schema-kind maps for typed reconstruction,
/// SECTIONED to mirror how the trace exposes names: span-level `fields`
/// keep their storage names; event/link attribute keys are the
/// prefix-stripped names [`sfst::TraceEvent`]/[`sfst::TraceLink`] expose
/// (a flat result map would collide an event attr `foo` with a link
/// attr `foo` whose kinds differ). Every section is FILTERED to exactly
/// the names the returned trace exposes; the KINDS coalesce — via the
/// [`sfst::join_value_kinds`] lattice — from exactly the sources whose
/// spans the result retained. Sorted by name.
#[derive(Debug, Default, PartialEq, Eq)]
pub struct FieldKinds {
    pub fields: Vec<(String, sfst::ValueKind)>,
    pub event_attributes: Vec<(String, sfst::ValueKind)>,
    pub link_attributes: Vec<(String, sfst::ValueKind)>,
}

/// One assembled trace plus everything the consumer needs to interpret
/// it honestly.
#[derive(Debug)]
pub struct TraceData {
    /// The merged trace (combiner total order, node-index graph). Empty
    /// when the id is absent everywhere — that is a `Complete` empty,
    /// not an error.
    pub trace: sfst::Trace,
    /// Query-level completeness.
    pub status: QueryStatus,
    /// Field → coalesced scalar kind for the returned trace; see
    /// [`FieldKinds`] for the sectioning and coalescing contract.
    pub field_kinds: FieldKinds,
}

/// Run a cross-source trace-by-id. See the module docs for the status
/// and cancellation contract.
///
/// `progress` ticks once per source during setup, success or failure
/// alike; the caller advertises the `sources.len()` total out of band
/// (the otel-ledger handler sets it on the capture and runs this
/// through `spawn_blocking`). Callers that don't report pass a fresh
/// counter; callers that don't cancel pass `CancellationToken::new()`
/// (as `sfsq-cli` does).
///
/// Pure sync — reads and decompresses files; invoke off any async
/// runtime thread (the logs-engine contract).
pub fn trace_by_id(
    sources: Vec<TraceSource>,
    query: TraceQuery,
    cancel: CancellationToken,
    progress: Arc<AtomicUsize>,
) -> Result<TraceData, TraceRequestError> {
    if query.trace_id.is_unset() {
        return Err(TraceRequestError::UnsetTraceId);
    }
    if query.span_cap == Some(0) {
        return Err(TraceRequestError::ZeroSpanCap);
    }
    validate_sources(&sources)?;

    let mut status = StatusBuilder::new();
    let cancelled_empty = |mut status: StatusBuilder| {
        status.add(PartialReason::Cancelled);
        TraceData {
            trace: sfst::Trace {
                spans: Vec::new(),
                roots: Vec::new(),
                children: Vec::new(),
            },
            status: status.finish(),
            field_kinds: FieldKinds::default(),
        }
    };
    // Pre-heads cancellation returns EMPTY + Cancelled — polled up front
    // so an already-cancelled call (a zero-source one included) can
    // never report Complete. The combiner re-polls identically.
    if cancel.is_cancelled() {
        return Ok(cancelled_empty(status));
    }

    // ── Setup: obtain every source's bytes ────────────────────────────
    // Map SFST bytes and decode tail frames. A failure is reported
    // (SourceFailure) and the source dropped; cancellation HERE returns
    // the empty result — no deterministic prefix exists before every
    // source is ready.
    let mut mapped_sfsts: Vec<(crate::source::Mapped, &TraceSource)> = Vec::new();
    let mut tails: Vec<(super::sources::SourceId, TraceWalScan)> = Vec::new();
    for source in &sources {
        if cancel.is_cancelled() {
            return Ok(cancelled_empty(status));
        }
        match source {
            TraceSource::Sfst(c) => match map_source(&c.source) {
                Ok(mapped) => mapped_sfsts.push((mapped, source)),
                Err(e) => {
                    tracing::warn!("sfsq traces: source {} failed to map: {e}", c.source_id);
                    status.add(PartialReason::SourceFailure);
                }
            },
            // The scan reads the VALIDATED coverage range — the one field
            // the overlap check saw (no second range to diverge).
            TraceSource::Tail(t) => match TraceWalScan::scan_range(&t.path, t.coverage.range) {
                Ok(scan) => tails.push((t.source_id.clone(), scan)),
                Err(e) => {
                    tracing::warn!("sfsq traces: tail {} failed to scan: {e}", t.source_id);
                    status.add(PartialReason::SourceFailure);
                }
            },
            // No summary pruning here (the bloom prunes a by-id probe),
            // so any unavailable source may hold the trace's spans.
            TraceSource::Unavailable(_) => status.add(PartialReason::RemoteUnavailable),
        }
        progress.fetch_add(1, Ordering::Relaxed);
    }

    // Readers borrow the mappings (which stay put from here on); an
    // unparseable SFST is a SourceFailure. Sessions then borrow the
    // readers; the merge borrows the sessions — strictly sequential
    // borrows, no self-reference.
    let mut readers: Vec<(sfst::IndexReader<'_>, &TraceSource)> = Vec::new();
    for (mapped, source) in &mapped_sfsts {
        if cancel.is_cancelled() {
            return Ok(cancelled_empty(status));
        }
        match sfst::IndexReader::open(mapped.bytes()) {
            Ok(reader) => readers.push((reader, source)),
            Err(e) => {
                tracing::warn!(
                    "sfsq traces: source {} failed to parse: {e}",
                    source.source_id()
                );
                status.add(PartialReason::SourceFailure);
            }
        }
    }
    let mut sessions: Vec<sfst::TraceFileSession<'_, '_>> = readers
        .iter()
        .map(|(reader, _)| sfst::TraceFileSession::open(reader))
        .collect();

    // The combiner's input slice: sessions first, tails after — the
    // index split the field-kind pass below relies on. `origin` maps a
    // merge index back to a diagnostic label.
    let session_count = sessions.len();
    let mut origin: Vec<String> = readers
        .iter()
        .map(|(_, source)| source.source_id().to_string())
        .collect();
    let mut merged: Vec<&mut dyn SpanSource> = Vec::new();
    for session in sessions.iter_mut() {
        merged.push(session);
    }
    for (source_id, tail) in tails.iter_mut() {
        merged.push(tail);
        origin.push(source_id.to_string());
    }

    let outcome = combine(&mut merged, query.trace_id, query.span_cap, &|| {
        cancel.is_cancelled()
    });
    // Release the merge's &mut borrows (each session, each tail) so the
    // field-kind pass below can read tails by index again.
    drop(merged);

    if outcome.truncated {
        status.add(PartialReason::SizeCap);
    }
    if outcome.cancelled {
        status.add(PartialReason::Cancelled);
    }
    for (idx, e) in &outcome.failures {
        let who = origin.get(*idx).map(String::as_str).unwrap_or("?");
        tracing::warn!("sfsq traces: source {who} failed during merge: {e}");
        status.add(PartialReason::SourceFailure);
    }

    // ── Field kinds: from exactly the contributing sources, projected
    // onto exactly the names the result exposes ───────────────────────
    let mut kinds: BTreeMap<String, sfst::ValueKind> = BTreeMap::new();
    let mut fold = |pairs: Vec<(String, sfst::ValueKind)>| {
        for (field, kind) in pairs {
            kinds
                .entry(field)
                .and_modify(|k| *k = sfst::join_value_kinds(*k, kind))
                .or_insert(kind);
        }
    };
    for &idx in &outcome.contributing_sources {
        if idx < session_count {
            fold(readers[idx].0.metadata().tree.derive_scalar_kinds());
        } else {
            fold(tails[idx - session_count].1.field_kinds().to_vec());
        }
    }
    // Exposed names per section. Span fields keep storage names; event/
    // link attribute names are prefix-stripped in the result, so their
    // kinds resolve through the storage prefix.
    use std::collections::BTreeSet;
    let mut field_names: BTreeSet<&str> = BTreeSet::new();
    let mut event_attr_names: BTreeSet<&str> = BTreeSet::new();
    let mut link_attr_names: BTreeSet<&str> = BTreeSet::new();
    for span in &outcome.trace.spans {
        field_names.extend(span.fields.iter().map(|(k, _)| k.as_str()));
        for ev in &span.events {
            event_attr_names.extend(ev.attributes.iter().map(|(k, _)| k.as_str()));
        }
        for lk in &span.links {
            link_attr_names.extend(lk.attributes.iter().map(|(k, _)| k.as_str()));
        }
    }
    let project = |names: &BTreeSet<&str>, prefix: &str| -> Vec<(String, sfst::ValueKind)> {
        names
            .iter()
            .filter_map(|name| {
                kinds
                    .get(&format!("{prefix}{name}"))
                    .map(|k| (name.to_string(), *k))
            })
            .collect()
    };
    let field_kinds = FieldKinds {
        fields: project(&field_names, ""),
        event_attributes: project(&event_attr_names, "events.attributes."),
        link_attributes: project(&link_attr_names, "links.attributes."),
    };

    Ok(TraceData {
        trace: outcome.trace,
        status: status.finish(),
        field_kinds,
    })
}
