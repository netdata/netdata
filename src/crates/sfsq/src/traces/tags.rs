//! Tag / tag-value enumeration — the phase-4b operations.
//!
//! [`tag_names`] and [`tag_values`] answer "what can be filtered on, and
//! with which values" straight off the dictionaries: a sealed or
//! in-memory SFST contributes its field table and per-field dictionary
//! chunks ([`sfst::IndexReader::field_values`] — never stream batches or
//! row columns), a WAL tail contributes its frame-decoded pair table.
//! Keys and values come back as the TYPED, wire-neutral vocabulary of
//! [`super::vocab`]; the Intrinsic scope is the full static
//! [`TraceIntrinsic`] set regardless of data (intrinsics are engine
//! capabilities, not data properties).
//!
//! Determinism (design-record decision 13): enumerate ALL candidate
//! sources, merge into ordered sets, then limit — `truncated` is exact
//! for keys and values, and source order never changes a result.
//! Consequently cancellation is ALL-OR-EMPTY (phase-4b pin C2): a merge
//! prefix over "the sources processed so far" would depend on caller
//! order, so cancellation observed before every source's contribution is
//! collected returns an EMPTY result with
//! [`Cancelled`](PartialReason::Cancelled); once everything is collected
//! there is nothing left to cancel.
//!
//! Time bounding is FILE-GRANULAR (decision 20A): an optional window
//! prunes SFST candidates whose summary range does not overlap it, but
//! an overlapping file contributes its whole dictionaries — values that
//! occur only outside the window included. A tail always contributes
//! when offered (no time metadata; it is the newest data). The window is
//! half-open nanoseconds; summaries are inclusive seconds, expanded to
//! `[min_s·10⁹, (max_s+1)·10⁹)` (saturating) for the overlap test — a
//! sub-second window prunes conservatively (pin C3).
//!
//! Failure honesty matches [`trace_by_id`](super::trace_by_id): a source
//! that fails to map, open, or read a dictionary is a
//! [`SourceFailure`](PartialReason::SourceFailure) on the query status,
//! never a silent skip. Under a `Partial` status the exact-`truncated`
//! guarantee is relative to the successfully observed sources.

use std::collections::BTreeSet;
use std::sync::Arc;
use std::sync::atomic::{AtomicUsize, Ordering};

use tokio_util::sync::CancellationToken;

use super::sources::{SourceSetError, TraceSource, validate_sources};
use super::status::{PartialReason, QueryStatus, StatusBuilder};
use super::vocab::{TagKey, TagScope, TraceIntrinsic, storage_to_tag};
use super::wal_scan::TraceWalScan;
use crate::source::map_source;

/// A half-open `[start_ns, end_ns)` nanosecond window (pin C3).
/// Construction validates `start_ns < end_ns`.
#[derive(Debug, Clone, Copy, PartialEq, Eq)]
pub struct TimeWindow {
    start_ns: i64,
    end_ns: i64,
}

impl TimeWindow {
    pub fn new(start_ns: i64, end_ns: i64) -> Result<Self, TagRequestError> {
        if start_ns >= end_ns {
            return Err(TagRequestError::InvalidWindow { start_ns, end_ns });
        }
        Ok(Self { start_ns, end_ns })
    }

    /// Whether a summary's inclusive-seconds `[min_s, max_s]` range
    /// overlaps this window: the file range expands to nanoseconds as
    /// `[min_s·10⁹, (max_s+1)·10⁹)` (saturating) and the two half-open
    /// ranges intersect iff each starts before the other ends.
    fn overlaps_summary(&self, min_s: u32, max_s: u32) -> bool {
        const NS: i64 = 1_000_000_000;
        let file_start = i64::from(min_s).saturating_mul(NS);
        let file_end = (i64::from(max_s) + 1).saturating_mul(NS);
        self.start_ns < file_end && file_start < self.end_ns
    }
}

/// A tag-names request. [`new`](Self::new) enumerates every scope,
/// unlimited; narrow with the builder methods.
#[derive(Debug, Clone, Default)]
pub struct TagNamesQuery {
    window: Option<TimeWindow>,
    scope: Option<TagScope>,
    max_keys: Option<usize>,
}

impl TagNamesQuery {
    pub fn new() -> Self {
        Self::default()
    }

    /// Bound the enumeration to files overlapping `window`
    /// (file-granular — see the module docs).
    pub fn window(mut self, window: TimeWindow) -> Self {
        self.window = Some(window);
        self
    }

    /// Enumerate only one scope.
    pub fn scope(mut self, scope: TagScope) -> Self {
        self.scope = Some(scope);
        self
    }

    /// Cap the key list (one global cap; `truncated` is exact). Zero is
    /// rejected at request validation.
    pub fn max_keys(mut self, max: usize) -> Self {
        self.max_keys = Some(max);
        self
    }
}

/// A tag-values request for one `(scope, key)` tag.
#[derive(Debug, Clone)]
pub struct TagValuesQuery {
    scope: TagScope,
    key: TagKey,
    window: Option<TimeWindow>,
    max_values: Option<usize>,
}

impl TagValuesQuery {
    pub fn new(scope: TagScope, key: TagKey) -> Self {
        Self {
            scope,
            key,
            window: None,
            max_values: None,
        }
    }

    /// Bound the enumeration to files overlapping `window`
    /// (file-granular — see the module docs).
    pub fn window(mut self, window: TimeWindow) -> Self {
        self.window = Some(window);
        self
    }

    /// Cap the value list (`truncated` is exact). Zero is rejected at
    /// request validation.
    pub fn max_values(mut self, max: usize) -> Self {
        self.max_values = Some(max);
        self
    }
}

/// A request error — the caller built an invalid request or source set;
/// nothing was queried. Data conditions are NOT errors: a tag absent
/// from every source yields an empty `Complete` result.
#[derive(Debug, thiserror::Error)]
pub enum TagRequestError {
    #[error("a zero key/value limit would return nothing; omit the limit or raise it")]
    ZeroLimit,
    #[error("invalid time window [{start_ns}, {end_ns}): start must be before end")]
    InvalidWindow { start_ns: i64, end_ns: i64 },
    /// The static virtual/dictionary split is known at the boundary
    /// (decision 18B), so asking for a virtual intrinsic's values is a
    /// caller bug, not a data condition.
    #[error("intrinsic {0:?} is virtual (no value dictionary); its values cannot be enumerated")]
    NotEnumerable(TraceIntrinsic),
    #[error("an intrinsic key requires the Intrinsic scope, got {0:?} (pin C4)")]
    IntrinsicKeyOutsideIntrinsicScope(TagScope),
    #[error("the Intrinsic scope holds no attribute keys, got attribute {0:?} (pin C4)")]
    AttributeKeyInIntrinsicScope(String),
    #[error(transparent)]
    SourceSet(#[from] SourceSetError),
}

/// The enumerated keys: sorted `(scope, key)`, one exact global
/// `truncated` flag.
#[derive(Debug)]
pub struct TagNamesData {
    pub keys: Vec<(TagScope, TagKey)>,
    pub truncated: bool,
    pub status: QueryStatus,
}

/// One enumerated value plus the FIELD-coalesced schema kind: the same
/// kind for every value of the field by design (per-occurrence kinds are
/// not distinguishable — phase-1 decision F), `None` when no
/// contributing source has a scalar occurrence of the field (a null- or
/// empty-container-only attribute — pin C1).
#[derive(Debug, Clone, PartialEq, Eq)]
pub struct TagValue {
    pub value: String,
    pub kind: Option<sfst::ValueKind>,
}

/// The enumerated values of one tag: sorted by value bytes, one exact
/// `truncated` flag.
#[derive(Debug)]
pub struct TagValuesData {
    pub values: Vec<TagValue>,
    pub truncated: bool,
    pub status: QueryStatus,
}

/// Resolve a values-request `(scope, key)` pair to the storage
/// dictionary field it reads, enforcing pin C4 and decision 18B.
fn storage_field_of(scope: TagScope, key: &TagKey) -> Result<String, TagRequestError> {
    match (scope, key) {
        (TagScope::Intrinsic, TagKey::Intrinsic(i)) => i
            .dictionary_field()
            .map(str::to_string)
            .ok_or(TagRequestError::NotEnumerable(*i)),
        (TagScope::Intrinsic, TagKey::Attribute(a)) => {
            Err(TagRequestError::AttributeKeyInIntrinsicScope(a.clone()))
        }
        (scope, TagKey::Intrinsic(_)) => {
            Err(TagRequestError::IntrinsicKeyOutsideIntrinsicScope(scope))
        }
        (scope, TagKey::Attribute(bare)) => {
            let prefix = scope.attribute_prefix().expect("non-Intrinsic scope");
            Ok(format!("{prefix}{bare}"))
        }
    }
}

/// Whether a window (if any) prunes an SFST candidate. A pruned file is
/// SKIPPED, not failed — out-of-window is a data condition.
fn pruned(window: Option<TimeWindow>, summary: &sfst::Summary) -> bool {
    window.is_some_and(|w| !w.overlaps_summary(summary.min_timestamp_s, summary.max_timestamp_s))
}

/// Enumerate tag keys across `sources`. See the module docs for the
/// determinism, cancellation, window, and failure contracts. `progress`
/// ticks once per source, success or failure, exactly as
/// [`trace_by_id`](super::trace_by_id)'s does.
///
/// Pure sync — reads and decompresses files; invoke off any async
/// runtime thread (the logs-engine contract).
pub fn tag_names(
    sources: Vec<TraceSource>,
    query: TagNamesQuery,
    cancel: CancellationToken,
    progress: Arc<AtomicUsize>,
) -> Result<TagNamesData, TagRequestError> {
    if query.max_keys == Some(0) {
        return Err(TagRequestError::ZeroLimit);
    }
    validate_sources(&sources)?;

    let mut status = StatusBuilder::new();
    let cancelled_empty = |mut status: StatusBuilder| {
        status.add(PartialReason::Cancelled);
        TagNamesData {
            keys: Vec::new(),
            truncated: false,
            status: status.finish(),
        }
    };
    if cancel.is_cancelled() {
        return Ok(cancelled_empty(status));
    }

    let in_scope = |scope: TagScope| query.scope.is_none_or(|s| s == scope);

    // The static intrinsic vocabulary (18B) — present regardless of what
    // the sources hold, deterministic by construction.
    let mut keys: BTreeSet<(TagScope, TagKey)> = BTreeSet::new();
    if in_scope(TagScope::Intrinsic) {
        keys.extend(
            TraceIntrinsic::ALL
                .into_iter()
                .map(|i| (TagScope::Intrinsic, TagKey::Intrinsic(i))),
        );
    }

    for source in &sources {
        // All-or-empty cancellation (pin C2).
        if cancel.is_cancelled() {
            return Ok(cancelled_empty(status));
        }
        let mut add_storage_names = |names: &mut dyn Iterator<Item = &str>| {
            for name in names {
                if let Some((scope, key)) = storage_to_tag(name)
                    && in_scope(scope)
                {
                    keys.insert((scope, key));
                }
            }
        };
        match source {
            TraceSource::Sfst(c) => {
                if !pruned(query.window, &c.summary) {
                    match map_source(&c.source) {
                        Ok(mapped) => match sfst::IndexReader::open(mapped.bytes()) {
                            Ok(reader) => {
                                add_storage_names(&mut reader.field_table().names());
                            }
                            Err(e) => {
                                tracing::warn!(
                                    "sfsq traces: source {} failed to parse: {e}",
                                    c.source_id
                                );
                                status.add(PartialReason::SourceFailure);
                            }
                        },
                        Err(e) => {
                            tracing::warn!("sfsq traces: source {} failed to map: {e}", c.source_id);
                            status.add(PartialReason::SourceFailure);
                        }
                    }
                }
            }
            TraceSource::Tail(t) => match TraceWalScan::scan_range(&t.path, t.coverage.range) {
                Ok(scan) => {
                    add_storage_names(&mut scan.pair_table().keys().map(String::as_str));
                }
                Err(e) => {
                    tracing::warn!("sfsq traces: tail {} failed to scan: {e}", t.source_id);
                    status.add(PartialReason::SourceFailure);
                }
            },
        }
        progress.fetch_add(1, Ordering::Relaxed);
    }

    // Merge-then-limit (decision 13): the BTreeSet IS the deterministic
    // (scope, key) order; the cap truncates the tail with an exact flag.
    let mut keys: Vec<(TagScope, TagKey)> = keys.into_iter().collect();
    let truncated = query.max_keys.is_some_and(|max| keys.len() > max);
    if let Some(max) = query.max_keys {
        keys.truncate(max);
    }
    Ok(TagNamesData {
        keys,
        truncated,
        status: status.finish(),
    })
}

/// Enumerate one tag's values across `sources`. See the module docs for
/// the determinism, cancellation, window, and failure contracts; the
/// kind on each value follows pin C1. `progress` ticks once per source.
///
/// Pure sync — reads and decompresses files; invoke off any async
/// runtime thread (the logs-engine contract).
pub fn tag_values(
    sources: Vec<TraceSource>,
    query: TagValuesQuery,
    cancel: CancellationToken,
    progress: Arc<AtomicUsize>,
) -> Result<TagValuesData, TagRequestError> {
    if query.max_values == Some(0) {
        return Err(TagRequestError::ZeroLimit);
    }
    let storage = storage_field_of(query.scope, &query.key)?;
    validate_sources(&sources)?;

    let mut status = StatusBuilder::new();
    let cancelled_empty = |mut status: StatusBuilder| {
        status.add(PartialReason::Cancelled);
        TagValuesData {
            values: Vec::new(),
            truncated: false,
            status: status.finish(),
        }
    };
    if cancel.is_cancelled() {
        return Ok(cancelled_empty(status));
    }

    let mut values: BTreeSet<String> = BTreeSet::new();
    // Field-coalesced kind over exactly the sources whose dictionary
    // contributed (the design-record coalescing domain), via the shared
    // lattice; stays `None` when no contributor has a scalar occurrence
    // (pin C1).
    let mut kind: Option<sfst::ValueKind> = None;
    let fold_kind = |kind: &mut Option<sfst::ValueKind>, found: Option<sfst::ValueKind>| {
        if let Some(k) = found {
            *kind = Some(match *kind {
                Some(prev) => sfst::join_value_kinds(prev, k),
                None => k,
            });
        }
    };
    let kind_of = |pairs: &[(String, sfst::ValueKind)]| -> Option<sfst::ValueKind> {
        pairs
            .iter()
            .find(|(name, _)| *name == storage)
            .map(|(_, k)| *k)
    };

    for source in &sources {
        // All-or-empty cancellation (pin C2).
        if cancel.is_cancelled() {
            return Ok(cancelled_empty(status));
        }
        match source {
            TraceSource::Sfst(c) => {
                if !pruned(query.window, &c.summary) {
                    match map_source(&c.source) {
                        Ok(mapped) => match sfst::IndexReader::open(mapped.bytes()) {
                            // A file without the field contributes
                            // nothing — absent-matches-nothing, a data
                            // condition (21A).
                            Ok(reader) if reader.field_table().contains(&storage) => {
                                match reader.field_values(&storage) {
                                    Ok(vs) => {
                                        values.extend(vs);
                                        let kinds = reader.metadata().tree.derive_scalar_kinds();
                                        fold_kind(&mut kind, kind_of(&kinds));
                                    }
                                    Err(e) => {
                                        tracing::warn!(
                                            "sfsq traces: source {} failed to enumerate {storage}: {e}",
                                            c.source_id
                                        );
                                        status.add(PartialReason::SourceFailure);
                                    }
                                }
                            }
                            Ok(_) => {}
                            Err(e) => {
                                tracing::warn!(
                                    "sfsq traces: source {} failed to parse: {e}",
                                    c.source_id
                                );
                                status.add(PartialReason::SourceFailure);
                            }
                        },
                        Err(e) => {
                            tracing::warn!("sfsq traces: source {} failed to map: {e}", c.source_id);
                            status.add(PartialReason::SourceFailure);
                        }
                    }
                }
            }
            TraceSource::Tail(t) => match TraceWalScan::scan_range(&t.path, t.coverage.range) {
                Ok(scan) => {
                    if let Some(vs) = scan.pair_table().get(&storage) {
                        values.extend(vs.iter().cloned());
                        fold_kind(&mut kind, kind_of(scan.field_kinds()));
                    }
                }
                Err(e) => {
                    tracing::warn!("sfsq traces: tail {} failed to scan: {e}", t.source_id);
                    status.add(PartialReason::SourceFailure);
                }
            },
        }
        progress.fetch_add(1, Ordering::Relaxed);
    }

    // Merge-then-limit (decision 13): sorted by value bytes; the cap
    // truncates with an exact flag.
    let mut out: Vec<TagValue> = values
        .into_iter()
        .map(|value| TagValue { value, kind })
        .collect();
    let truncated = query.max_values.is_some_and(|max| out.len() > max);
    if let Some(max) = query.max_values {
        out.truncate(max);
    }
    Ok(TagValuesData {
        values: out,
        truncated,
        status: status.finish(),
    })
}
