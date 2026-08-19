//! Attribute / attribute-value enumeration — the phase-4b operations.
//!
//! [`attribute_names`] and [`attribute_values`] answer "what can be filtered on, and
//! with which values" straight off the dictionaries: a sealed or
//! in-memory SFST contributes its field table and per-field dictionary
//! chunks ([`sfst::IndexReader::field_values`] — never stream batches or
//! row columns), a WAL tail contributes its frame-decoded pair table.
//! Keys and values come back as the TYPED, wire-neutral vocabulary of
//! [`super::vocab`]; the Builtin owner is the full static
//! [`BuiltinField`] set regardless of data (builtin fields are engine
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
use super::vocab::{AttributeKey, AttributeOwner, BuiltinField, storage_to_attribute};
use super::wal_scan::TraceWalScan;
use super::window::{TimeWindow, WindowError};
use crate::source::map_source;

/// A key-names request. [`new`](Self::new) enumerates every owner,
/// unlimited; narrow with the builder methods.
#[derive(Debug, Clone, Default)]
pub struct AttributeNamesQuery {
    window: Option<TimeWindow>,
    owner: Option<AttributeOwner>,
    max_keys: Option<usize>,
}

impl AttributeNamesQuery {
    pub fn new() -> Self {
        Self::default()
    }

    /// Bound the enumeration to files overlapping `window`
    /// (file-granular — see the module docs).
    pub fn window(mut self, window: TimeWindow) -> Self {
        self.window = Some(window);
        self
    }

    /// Enumerate only one owner.
    pub fn owner(mut self, owner: AttributeOwner) -> Self {
        self.owner = Some(owner);
        self
    }

    /// Cap the key list (one global cap; `truncated` is exact). Zero is
    /// rejected at request validation.
    pub fn max_keys(mut self, max: usize) -> Self {
        self.max_keys = Some(max);
        self
    }
}

/// A values request for one `(owner, key)` pair.
#[derive(Debug, Clone)]
pub struct AttributeValuesQuery {
    owner: AttributeOwner,
    key: AttributeKey,
    window: Option<TimeWindow>,
    max_values: Option<usize>,
}

impl AttributeValuesQuery {
    pub fn new(owner: AttributeOwner, key: AttributeKey) -> Self {
        Self {
            owner,
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
/// nothing was queried. Data conditions are NOT errors: a key absent
/// from every source yields an empty `Complete` result.
#[derive(Debug, thiserror::Error)]
pub enum AttributeRequestError {
    #[error("a zero key/value limit would return nothing; omit the limit or raise it")]
    ZeroLimit,
    #[error(transparent)]
    Window(#[from] WindowError),
    /// The static virtual/dictionary split is known at the boundary
    /// (decision 18B), so asking for a virtual builtin's values is a
    /// caller bug, not a data condition.
    #[error("builtin field {0:?} is virtual (no value dictionary); its values cannot be enumerated")]
    NotEnumerable(BuiltinField),
    #[error("a builtin key requires the Builtin owner, got {0:?} (pin C4)")]
    BuiltinKeyOutsideBuiltinOwner(AttributeOwner),
    #[error("the Builtin owner holds no attribute keys, got attribute {0:?} (pin C4)")]
    AttributeKeyUnderBuiltinOwner(String),
    /// `Any` exists for predicates (the owner-agnostic filter);
    /// enumeration is per concrete owner — omit the filter to list all.
    #[error("the Any owner is a predicate construct; enumeration takes a concrete owner")]
    AnyOwnerNotEnumerable,
    #[error(transparent)]
    SourceSet(#[from] SourceSetError),
}

/// The enumerated keys: sorted `(owner, key)`, one exact global
/// `truncated` flag.
#[derive(Debug)]
pub struct AttributeNamesData {
    pub keys: Vec<(AttributeOwner, AttributeKey)>,
    pub truncated: bool,
    pub status: QueryStatus,
}

/// One enumerated value plus the FIELD-coalesced schema kind: the same
/// kind for every value of the field by design (per-occurrence kinds are
/// not distinguishable — phase-1 decision F), `None` when no
/// contributing source has a scalar occurrence of the field (a null- or
/// empty-container-only attribute — pin C1).
#[derive(Debug, Clone, PartialEq, Eq)]
pub struct AttributeValue {
    pub value: String,
    pub kind: Option<sfst::ValueKind>,
}

/// The enumerated values of one key: sorted by value bytes, one exact
/// `truncated` flag.
#[derive(Debug)]
pub struct AttributeValuesData {
    pub values: Vec<AttributeValue>,
    pub truncated: bool,
    pub status: QueryStatus,
}

/// Resolve a values-request `(owner, key)` pair to the storage
/// dictionary field it reads, enforcing pin C4 and decision 18B.
fn storage_field_of(owner: AttributeOwner, key: &AttributeKey) -> Result<String, AttributeRequestError> {
    match (owner, key) {
        (AttributeOwner::Any, _) => Err(AttributeRequestError::AnyOwnerNotEnumerable),
        (AttributeOwner::Builtin, AttributeKey::Builtin(i)) => i
            .dictionary_field()
            .map(str::to_string)
            .ok_or(AttributeRequestError::NotEnumerable(*i)),
        (AttributeOwner::Builtin, AttributeKey::Attribute(a)) => {
            Err(AttributeRequestError::AttributeKeyUnderBuiltinOwner(a.clone()))
        }
        (owner, AttributeKey::Builtin(_)) => {
            Err(AttributeRequestError::BuiltinKeyOutsideBuiltinOwner(owner))
        }
        (owner, AttributeKey::Attribute(bare)) => {
            let prefix = owner.attribute_prefix().expect("validated: a concrete attribute owner");
            Ok(format!("{prefix}{bare}"))
        }
    }
}

/// Whether a window (if any) prunes an SFST candidate. A pruned file is
/// SKIPPED, not failed — out-of-window is a data condition.
fn pruned(window: Option<TimeWindow>, summary: &sfst::Summary) -> bool {
    window.is_some_and(|w| !w.overlaps_summary(summary.min_timestamp_s, summary.max_timestamp_s))
}

/// Enumerate attribute and builtin-field keys across `sources`. See the module docs for the
/// determinism, cancellation, window, and failure contracts. `progress`
/// ticks once per source, success or failure, exactly as
/// [`trace_by_id`](super::trace_by_id)'s does.
///
/// Pure sync — reads and decompresses files; invoke off any async
/// runtime thread (the logs-engine contract).
pub fn attribute_names(
    sources: Vec<TraceSource>,
    query: AttributeNamesQuery,
    cancel: CancellationToken,
    progress: Arc<AtomicUsize>,
) -> Result<AttributeNamesData, AttributeRequestError> {
    if query.max_keys == Some(0) {
        return Err(AttributeRequestError::ZeroLimit);
    }
    if query.owner == Some(AttributeOwner::Any) {
        return Err(AttributeRequestError::AnyOwnerNotEnumerable);
    }
    validate_sources(&sources)?;

    let mut status = StatusBuilder::new();
    let cancelled_empty = |mut status: StatusBuilder| {
        status.add(PartialReason::Cancelled);
        AttributeNamesData {
            keys: Vec::new(),
            truncated: false,
            status: status.finish(),
        }
    };
    if cancel.is_cancelled() {
        return Ok(cancelled_empty(status));
    }

    let in_owner = |owner: AttributeOwner| query.owner.is_none_or(|s| s == owner);

    // The static builtin-field vocabulary (18B) — present regardless of what
    // the sources hold, deterministic by construction.
    let mut keys: BTreeSet<(AttributeOwner, AttributeKey)> = BTreeSet::new();
    if in_owner(AttributeOwner::Builtin) {
        keys.extend(
            BuiltinField::ALL
                .into_iter()
                .map(|i| (AttributeOwner::Builtin, AttributeKey::Builtin(i))),
        );
    }

    for source in &sources {
        // All-or-empty cancellation (pin C2).
        if cancel.is_cancelled() {
            return Ok(cancelled_empty(status));
        }
        let mut add_storage_names = |names: &mut dyn Iterator<Item = &str>| {
            for name in names {
                if let Some((owner, key)) = storage_to_attribute(name)
                    && in_owner(owner)
                {
                    keys.insert((owner, key));
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
    // (owner, key) order; the cap truncates the tail with an exact flag.
    let mut keys: Vec<(AttributeOwner, AttributeKey)> = keys.into_iter().collect();
    let truncated = query.max_keys.is_some_and(|max| keys.len() > max);
    if let Some(max) = query.max_keys {
        keys.truncate(max);
    }
    Ok(AttributeNamesData {
        keys,
        truncated,
        status: status.finish(),
    })
}

/// Enumerate one key's values across `sources`. See the module docs for
/// the determinism, cancellation, window, and failure contracts; the
/// kind on each value follows pin C1. `progress` ticks once per source.
///
/// Pure sync — reads and decompresses files; invoke off any async
/// runtime thread (the logs-engine contract).
pub fn attribute_values(
    sources: Vec<TraceSource>,
    query: AttributeValuesQuery,
    cancel: CancellationToken,
    progress: Arc<AtomicUsize>,
) -> Result<AttributeValuesData, AttributeRequestError> {
    if query.max_values == Some(0) {
        return Err(AttributeRequestError::ZeroLimit);
    }
    let storage = storage_field_of(query.owner, &query.key)?;
    validate_sources(&sources)?;

    let mut status = StatusBuilder::new();
    let cancelled_empty = |mut status: StatusBuilder| {
        status.add(PartialReason::Cancelled);
        AttributeValuesData {
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
    let mut out: Vec<AttributeValue> = values
        .into_iter()
        .map(|value| AttributeValue { value, kind })
        .collect();
    let truncated = query.max_values.is_some_and(|max| out.len() > max);
    if let Some(max) = query.max_values {
        out.truncate(max);
    }
    Ok(AttributeValuesData {
        values: out,
        truncated,
        status: status.finish(),
    })
}
