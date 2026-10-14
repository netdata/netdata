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
//! folding a tail equals reading the `TRSU` of sealing the same data,
//! and a tail's [`TracePart`]s equal the `TRSU` + `TRSS` rows of
//! sealing it ([`sealed_trace_part`]), root claims and per-service
//! counts included.

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

/// One source's root claim for a trace, under the seal's rollup rules
/// (`sfst::TraceRollupRows::record_span`): the earliest unset-parent
/// span by `(start, span id)`; a full tie whose spans differ in kind,
/// service or name withholds the claim.
#[derive(Debug, Clone, PartialEq, Eq)]
pub enum RootClaim {
    /// No unset-parent span of the trace in this source.
    None,
    /// Unset-parent spans exist, but the pick is undecidable.
    Withheld { start_ns: i64 },
    /// The source's root candidate.
    True {
        start_ns: i64,
        span_id: sfst::SpanId,
        service: Option<String>,
        name: Option<String>,
    },
}

/// One source's part of a trace, as a list row reads it: the
/// aggregate, the latest stored start, the root claim and the stored
/// spans and ERROR spans per resource `service.name` (`None` = spans
/// without one), sorted by name with `None` last.
#[derive(Debug, Clone, PartialEq, Eq)]
pub struct TracePart {
    pub aggregate: TraceAggregate,
    pub max_start_ns: i64,
    pub root: RootClaim,
    pub services: Vec<(Option<String>, u64, u64)>,
}

/// The running root pick of one trace in one source (the seal's rule).
pub(crate) struct RootPick<'s> {
    start_ns: i64,
    span_id: sfst::SpanId,
    kind: i32,
    service: Option<&'s str>,
    name: Option<&'s str>,
    ambiguous: bool,
}

impl<'s> RootPick<'s> {
    /// Offer an unset-parent span.
    pub(crate) fn offer(pick: &mut Option<Self>, span: &'s sfst::TraceSpan) {
        let (service, name) = root_facets(span);
        let key = (span.start_ns, span.span_id);
        match pick {
            Some(p) if key == (p.start_ns, p.span_id) => {
                if (span.kind, service, name) != (p.kind, p.service, p.name) {
                    p.ambiguous = true;
                }
            }
            Some(p) if key > (p.start_ns, p.span_id) => {}
            _ => {
                *pick = Some(Self {
                    start_ns: span.start_ns,
                    span_id: span.span_id,
                    kind: span.kind,
                    service,
                    name,
                    ambiguous: false,
                })
            }
        }
    }

    /// The pick's claim; `true_root` is `Some((service, name))` only for
    /// an unambiguous pick.
    pub(crate) fn true_root(&self) -> Option<(Option<&'s str>, Option<&'s str>)> {
        (!self.ambiguous).then_some((self.service, self.name))
    }

    fn claim(&self) -> RootClaim {
        if self.ambiguous {
            return RootClaim::Withheld {
                start_ns: self.start_ns,
            };
        }
        RootClaim::True {
            start_ns: self.start_ns,
            span_id: self.span_id,
            service: self.service.map(str::to_owned),
            name: self.name.map(str::to_owned),
        }
    }
}

/// A span's root facets: its resource `service.name` and its name.
fn root_facets(span: &sfst::TraceSpan) -> (Option<&str>, Option<&str>) {
    let name_field = BuiltinField::Name
        .dictionary_field()
        .expect("Name is dictionary-backed");
    (
        span_field(span, &resource_service_field()),
        span_field(span, name_field),
    )
}

/// Sort per-service counts by name, `None` last.
fn sort_services(services: &mut [(Option<String>, u64, u64)]) {
    services.sort_by(|a, b| (a.0.is_none(), &a.0).cmp(&(b.0.is_none(), &b.0)));
}

/// Fold a WAL tail's decoded spans into per-trace parts, sorted by trace
/// id (the sealed rows' order), under the seal's rules.
pub fn tail_trace_parts(scan: &TraceWalScan) -> Vec<TracePart> {
    let status_field = BuiltinField::Status
        .dictionary_field()
        .expect("Status is dictionary-backed");
    struct Acc<'s> {
        max_start_ns: i64,
        root: Option<RootPick<'s>>,
        services: Vec<(Option<&'s str>, u64, u64)>,
    }
    let mut map: HashMap<sfst::TraceId, Acc<'_>> = HashMap::new();
    for (trace_id, span) in scan.spans_with_ids() {
        if trace_id.is_unset() {
            continue;
        }
        let acc = map.entry(trace_id).or_insert(Acc {
            max_start_ns: i64::MIN,
            root: None,
            services: Vec::new(),
        });
        acc.max_start_ns = acc.max_start_ns.max(span.start_ns);
        if span.parent_span_id.is_unset() {
            RootPick::offer(&mut acc.root, span);
        }
        let service = root_facets(span).0;
        let error = u64::from(span_field(span, status_field) == Some("ERROR"));
        match acc.services.iter_mut().find(|(s, ..)| *s == service) {
            Some((_, spans, errors)) => {
                *spans += 1;
                *errors += error;
            }
            None => acc.services.push((service, 1, error)),
        }
    }
    let mut parts: Vec<TracePart> = tail_trace_aggregates(scan)
        .into_iter()
        .map(|aggregate| {
            let acc = &map[&aggregate.trace_id];
            let mut services: Vec<(Option<String>, u64, u64)> = acc
                .services
                .iter()
                .map(|(s, spans, errors)| (s.map(str::to_owned), *spans, *errors))
                .collect();
            sort_services(&mut services);
            TracePart {
                max_start_ns: acc.max_start_ns,
                root: acc.root.as_ref().map_or(RootClaim::None, RootPick::claim),
                services,
                aggregate,
            }
        })
        .collect();
    parts.sort_unstable_by_key(|p| p.aggregate.trace_id);
    parts
}

/// Row `index` of a sealed file's `TRSU` + `TRSS` as a [`TracePart`],
/// resolving the root and service refs through `resolve(field, ref)`
/// (`None` for the no-ref sentinel; an error for a corrupt ref).
///
/// Precondition: both come from the validating accessors
/// (`trace_rollup`, `trace_rollup_stats`).
pub fn sealed_trace_part(
    rollup: &sfst::TraceRollup,
    stats: &sfst::TraceRollupStats,
    index: usize,
    resolve: &mut dyn FnMut(&str, u32) -> Result<Option<String>, sfst::Error>,
) -> Result<TracePart, sfst::Error> {
    let name_field = BuiltinField::Name
        .dictionary_field()
        .expect("Name is dictionary-backed");
    let service_field = resource_service_field();
    let min_start = rollup.min_start_ns[index];
    let at = |offset: u64| min_start.saturating_add_unsigned(offset);
    let root_start = at(stats.root_start_offsets_ns[index]);
    let root = match rollup.root_is_true_root[index] {
        sfst::ROOT_CLAIM_TRUE => RootClaim::True {
            start_ns: root_start,
            span_id: rollup.root_span_ids.get(index),
            service: resolve(&service_field, rollup.root_service_refs[index])?,
            name: resolve(name_field, rollup.root_name_refs[index])?,
        },
        sfst::ROOT_CLAIM_WITHHELD => RootClaim::Withheld {
            start_ns: root_start,
        },
        _ => RootClaim::None,
    };
    let (from, to) = (
        stats.service_offsets[index] as usize,
        stats.service_offsets[index + 1] as usize,
    );
    let mut services = Vec::with_capacity(to - from);
    for e in from..to {
        services.push((
            resolve(&service_field, stats.service_refs[e])?,
            u64::from(stats.service_spans[e]),
            u64::from(stats.service_errors[e]),
        ));
    }
    sort_services(&mut services);
    Ok(TracePart {
        aggregate: TraceAggregate {
            trace_id: rollup.trace_ids.get(index),
            min_start_ns: min_start,
            max_end_ns: rollup.max_end_ns[index],
            span_count: u64::from(rollup.span_counts[index]),
            error_count: u64::from(rollup.error_counts[index]),
        },
        max_start_ns: at(stats.max_start_offsets_ns[index]),
        root,
        services,
    })
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
