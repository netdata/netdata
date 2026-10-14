//! Mapping between the `otel-traces` wire types ([`super::wire`]) and
//! the wire-neutral `sfsq::traces` engine — the traces analogue of
//! the logs `adapter`: a pure translation layer, no I/O of its own.
//!
//! * wire → engine — request-side parsing: hex ids, the selection-key
//!   grammar, the pagination cursor, window canonicalization
//!   ([`resolve_window`]), the search predicate ([`build_predicate`]).
//! * engine → wire — the response shapers (`to_*_result`), ids rendered
//!   as W3C lowercase hex.
//!
//! The only consumer is the traces handler (`handler.rs`), which owns
//! mode dispatch and the sealed-SFST + live-WAL source capture
//! ([`super::sources`]) — the adapter never touches registry state.

use std::collections::{HashMap, HashSet};

use sfsq::traces::{
    AttributeKey, AttributeOwner, AttributeValuesData, BuiltinField, CompareOp, Condition,
    DURATION_BIN_LABELS, DurationPercentiles, FieldKinds, OverviewData, PartialReason, Predicate,
    PredicateTarget, PredicateValue, QueryStatus, SearchData, TraceData,
};

use super::wire::{
    AnchorWire, AttributeValueWire, AttributeValuesResult, CoverageWire, EventWire, FieldKindsWire,
    LinkWire, OverviewGridWire, OverviewPercentilesWire, OverviewSection, OverviewTotals,
    SearchItems, SearchResult, ServiceBreakdownWire, ServiceSpansWire, SpanWire, StatusWire,
    TraceItems, TraceResult, TraceSummaryWire,
};

/// Parse a W3C text-form trace id: exactly 32 hex chars (16 bytes),
/// case-insensitive. Shape only: the all-zero (unset) sentinel parses
/// here — rejecting it is the caller's semantic job (the handler
/// pre-captures the engine's `UnsetTraceId` rejection).
pub(crate) fn parse_trace_id(s: &str) -> Result<sfst::TraceId, String> {
    let s = s.trim();
    if s.len() != 32 || !s.bytes().all(|b| b.is_ascii_hexdigit()) {
        return Err(format!(
            "trace id must be 32 hex characters (16 bytes), got {s:?}"
        ));
    }
    let mut bytes = [0u8; 16];
    for (i, chunk) in s.as_bytes().chunks_exact(2).enumerate() {
        // Infallible: both bytes were checked hex above.
        bytes[i] = u8::from_str_radix(std::str::from_utf8(chunk).unwrap(), 16).unwrap();
    }
    Ok(sfst::TraceId::from(bytes))
}

/// Validate the `trace` sub-object's optional assembly bounds into a
/// capture range. Both-or-neither and `after < before` — violations
/// are client errors, like the window fields of every other mode. Any
/// width is accepted: the agent is relaxed on time ranges, and the
/// response's `coverage` declares the range used. `None` = full
/// retention (the handler passes `0..u32::MAX`).
pub(crate) fn validate_trace_bounds(
    after: Option<u32>,
    before: Option<u32>,
) -> Result<Option<std::ops::Range<u32>>, String> {
    let (after, before) = match (after, before) {
        (None, None) => return Ok(None),
        (Some(a), Some(b)) => (a, b),
        _ => {
            return Err(
                "trace bounds require both 'after' and 'before' (or neither for full retention)"
                    .to_string(),
            );
        }
    };
    if after >= before {
        return Err(format!(
            "invalid trace bounds: after {after} >= before {before}"
        ));
    }
    Ok(Some(after..before))
}

/// Search completion slack, per side, seconds: the completion capture
/// is the match window widened by `clamp(window_width, 1h, 24h)` per
/// side — saturating at the u32 second boundaries — so a hit whose
/// trace straddles the window edge still assembles its out-of-window
/// spans. The window width is the scale hint: a narrow view gets at
/// least an hour of slack, a wide one at most a day per side.
pub(crate) const SEARCH_COMPLETION_SLACK_MIN_S: u32 = 3_600;
pub(crate) const SEARCH_COMPLETION_SLACK_MAX_S: u32 = 86_400;

/// The completion capture range for a match window: window widened by
/// the clamped per-side slack. Deterministic in the window alone, so a
/// pagination cursor that freezes the ORIGINAL window re-derives the
/// same completion range on every page.
pub(crate) fn completion_capture_range(window: &std::ops::Range<u32>) -> std::ops::Range<u32> {
    let slack = (window.end - window.start)
        .clamp(SEARCH_COMPLETION_SLACK_MIN_S, SEARCH_COMPLETION_SLACK_MAX_S);
    window.start.saturating_sub(slack)..window.end.saturating_add(slack)
}

// ── Response bounds ─────────────────────────────────────────────────

/// Per-value cap on the strings a trace's spans carry, in JSON bytes
/// (see [`truncate_value`]). The largest value seen in practice is a
/// ~6 KB exception stack trace, so real values stay whole; the cap only
/// stops a pathological value from dominating the response.
pub(crate) const TRACE_VALUE_MAX_BYTES: usize = 8 * 1024;

/// Budget for a trace's spans and their field kinds, in compact JSON
/// bytes. Past it the response keeps the earliest spans and reports
/// `byte_budget`.
pub(crate) const TRACE_SPANS_MAX_BYTES: usize = 2 * 1024 * 1024;

/// Per-value cap, in JSON bytes (see [`truncate_value`]), on the
/// free-length strings of list rows (root service and name, breakdown
/// service names) and of value lists. Both are for
/// display; the trace view carries values up to
/// [`TRACE_VALUE_MAX_BYTES`]. With at most
/// [`super::wire::SEARCH_LIMIT_MAX`] rows or
/// [`super::wire::MAX_VALUES_MAX`] values, a response stays well under
/// 1 MB by construction, so no byte budget has to cut items off a page.
pub(crate) const LIST_VALUE_MAX_BYTES: usize = 256;

/// Cut `s` at a character boundary so that it takes at most `max` bytes
/// as a JSON string body (escapes counted, quotes not); `true` when it
/// was cut. Counting the escaped form bounds what is sent: a control
/// character takes six bytes on the wire.
fn truncate_value(s: &mut String, max: usize) -> bool {
    let mut used = 0;
    for (i, c) in s.char_indices() {
        used += json_escaped_len(c);
        if used > max {
            s.truncate(i);
            return true;
        }
    }
    false
}

/// Bytes `c` takes inside a JSON string, as serde_json writes it.
fn json_escaped_len(c: char) -> usize {
    match c {
        '"' | '\\' | '\n' | '\r' | '\t' | '\u{08}' | '\u{0C}' => 2,
        c if u32::from(c) < 0x20 => 6,
        c => c.len_utf8(),
    }
}

/// Apply [`TRACE_VALUE_MAX_BYTES`] to every value one span carries;
/// `true` when any was cut.
fn cap_span_values(s: &mut sfst::TraceSpan) -> bool {
    let max = TRACE_VALUE_MAX_BYTES;
    let mut cut = false;
    for (_, v) in &mut s.fields {
        cut |= truncate_value(v, max);
    }
    for e in &mut s.events {
        cut |= truncate_value(&mut e.name, max);
        for (_, v) in &mut e.attributes {
            cut |= truncate_value(v, max);
        }
    }
    for l in &mut s.links {
        cut |= truncate_value(&mut l.trace_state, max);
        for (_, v) in &mut l.attributes {
            cut |= truncate_value(v, max);
        }
    }
    cut
}

/// Bytes `value` takes as compact JSON.
fn json_len(value: &impl serde::Serialize) -> usize {
    struct Count(usize);
    impl std::io::Write for Count {
        fn write(&mut self, buf: &[u8]) -> std::io::Result<usize> {
            self.0 += buf.len();
            Ok(buf.len())
        }
        fn flush(&mut self) -> std::io::Result<()> {
            Ok(())
        }
    }
    let mut count = Count(0);
    serde_json::to_writer(&mut count, value).expect("wire types serialize");
    count.0
}

/// `status` with `extra` added (a partial is a set, so a repeat is a no-op).
fn with_reason(status: QueryStatus, extra: PartialReason) -> QueryStatus {
    match status {
        QueryStatus::Complete => QueryStatus::Partial([extra].into()),
        QueryStatus::Partial(mut reasons) => {
            reasons.insert(extra);
            QueryStatus::Partial(reasons)
        }
    }
}

/// Shape one assembled trace into the wire result. `trace_id` is echoed
/// back in canonical (lowercase) hex regardless of the request's casing.
/// `coverage` is the capture range assembly actually used.
///
/// Bounded: values are capped ([`TRACE_VALUE_MAX_BYTES`], flagged per
/// span) and the spans stop at [`TRACE_SPANS_MAX_BYTES`]. A cut trace
/// keeps its earliest spans — the combiner's total order, as the
/// engine's own span cap does — with its graph rebuilt over them and
/// its field kinds narrowed to the names they carry, and reports
/// `byte_budget`.
pub(crate) fn to_trace_result(
    trace_id: &sfst::TraceId,
    data: TraceData,
    coverage: CoverageWire,
) -> TraceResult {
    let mut t = data.trace;
    let mut status = data.status;
    let mut field_kinds = data.field_kinds;
    let truncated: Vec<bool> = t.spans.iter_mut().map(cap_span_values).collect();
    let mut spans = Vec::with_capacity(t.spans.len());
    {
        // A name's kind entry is sent once: it counts with the first kept
        // span that carries the name.
        let mut kinds = KindCosts::new(&field_kinds);
        let mut bytes = 0usize;
        for (s, &cut) in t.spans.iter().zip(&truncated) {
            let wire = span_wire(s, cut);
            bytes += json_len(&wire) + kinds.first_seen_in(s);
            if bytes > TRACE_SPANS_MAX_BYTES {
                break;
            }
            spans.push(wire);
        }
    }
    if spans.len() < t.spans.len() {
        t.spans.truncate(spans.len());
        let (roots, children) = sfst::trace_combine::build_forest(&t.spans);
        t.roots = roots;
        t.children = children;
        retain_kinds_of(&mut field_kinds, &t.spans);
        status = with_reason(status, PartialReason::ByteBudget);
    }
    let summary_root = t.summary_root();
    TraceResult {
        mode: "trace",
        version: 1,
        trace_id: trace_id.to_string(),
        coverage,
        status: StatusWire::from(&status),
        items: TraceItems {
            returned: spans.len(),
        },
        summary_root,
        roots: t.roots,
        children: t.children,
        spans,
        field_kinds: field_kinds_wire(field_kinds),
    }
}

/// The wire cost of each name's field-kind entry, charged once, the
/// first time a span carries the name.
struct KindCosts<'a> {
    sections: [HashMap<&'a str, sfst::ValueKind>; 3],
}

impl<'a> KindCosts<'a> {
    fn new(kinds: &'a FieldKinds) -> Self {
        let index = |v: &'a [(String, sfst::ValueKind)]| -> HashMap<&'a str, sfst::ValueKind> {
            v.iter().map(|(n, k)| (n.as_str(), *k)).collect()
        };
        Self {
            sections: [
                index(&kinds.fields),
                index(&kinds.event_attributes),
                index(&kinds.link_attributes),
            ],
        }
    }

    /// Bytes of the kind entries `span` brings in first.
    fn first_seen_in(&mut self, span: &sfst::TraceSpan) -> usize {
        let [fields, events, links] = &mut self.sections;
        let mut cost = 0;
        let mut charge = |section: &mut HashMap<&'a str, sfst::ValueKind>, name: &str| {
            if let Some(kind) = section.remove(name) {
                cost += json_len(&(name, kind_word(kind))) + 1;
            }
        };
        for (name, _) in &span.fields {
            charge(fields, name);
        }
        for e in &span.events {
            for (name, _) in &e.attributes {
                charge(events, name);
            }
        }
        for l in &span.links {
            for (name, _) in &l.attributes {
                charge(links, name);
            }
        }
        cost
    }
}

/// Keep the kinds of the names `spans` carry: the engine projects them
/// onto every assembled span, and a cut trace sends only some of them.
fn retain_kinds_of(kinds: &mut FieldKinds, spans: &[sfst::TraceSpan]) {
    let mut fields: HashSet<&str> = HashSet::new();
    let mut event_attributes: HashSet<&str> = HashSet::new();
    let mut link_attributes: HashSet<&str> = HashSet::new();
    for s in spans {
        fields.extend(s.fields.iter().map(|(k, _)| k.as_str()));
        for e in &s.events {
            event_attributes.extend(e.attributes.iter().map(|(k, _)| k.as_str()));
        }
        for l in &s.links {
            link_attributes.extend(l.attributes.iter().map(|(k, _)| k.as_str()));
        }
    }
    kinds.fields.retain(|(n, _)| fields.contains(n.as_str()));
    kinds
        .event_attributes
        .retain(|(n, _)| event_attributes.contains(n.as_str()));
    kinds
        .link_attributes
        .retain(|(n, _)| link_attributes.contains(n.as_str()));
}

/// One assembled span → the wire shape (events and links ride along).
/// `truncated` says whether [`cap_span_values`] cut any of its values.
fn span_wire(s: &sfst::TraceSpan, truncated: bool) -> SpanWire {
    SpanWire {
        span_id: s.span_id.to_string(),
        // OTel semantics: an unset parent means "root", rendered as an
        // absent field rather than the all-zero sentinel string.
        parent_span_id: (!s.parent_span_id.is_unset()).then(|| s.parent_span_id.to_string()),
        start_ns: s.start_ns,
        duration_ns: s.duration_ns,
        kind: s.kind,
        flags: s.flags,
        dropped_attributes_count: s.dropped_attributes_count,
        dropped_events_count: s.dropped_events_count,
        dropped_links_count: s.dropped_links_count,
        fields: s.fields.clone(),
        events: s.events.iter().map(event_wire).collect(),
        links: s.links.iter().map(link_wire).collect(),
        truncated,
    }
}

/// One span event → the wire shape.
fn event_wire(e: &sfst::TraceEvent) -> EventWire {
    EventWire {
        time_unix_nano: e.time_unix_nano,
        name: e.name.clone(),
        dropped_attributes_count: e.dropped_attributes_count,
        attributes: e.attributes.clone(),
    }
}

/// One span link → the wire shape (the linked trace id, W3C hex).
fn link_wire(l: &sfst::TraceLink) -> LinkWire {
    LinkWire {
        trace_id: l.trace_id.to_string(),
        span_id: l.span_id.to_string(),
        trace_state: l.trace_state.clone(),
        flags: l.flags,
        dropped_attributes_count: l.dropped_attributes_count,
        attributes: l.attributes.clone(),
    }
}

/// Field-kind tables → the wire shape, each kind spelled as its wire
/// word ([`kind_word`]).
fn field_kinds_wire(k: FieldKinds) -> FieldKindsWire {
    let section = |v: Vec<(String, sfst::ValueKind)>| -> Vec<(String, &'static str)> {
        v.into_iter().map(|(n, k)| (n, kind_word(k))).collect()
    };
    FieldKindsWire {
        fields: section(k.fields),
        event_attributes: section(k.event_attributes),
        link_attributes: section(k.link_attributes),
    }
}

// ── Search: selection-key grammar ───────────────────────────────────

/// The wire word for each builtin field (snake_case). Also the words
/// key enumeration emits ([`render_attribute_key`]), so filters and
/// facet keys stay one vocabulary. The lockstep test walks
/// `BuiltinField::ALL`, so a new engine builtin breaks the suite until
/// it gets a wire word.
const BUILTIN_WORDS: [(&str, BuiltinField); 17] = [
    ("name", BuiltinField::Name),
    ("kind", BuiltinField::Kind),
    ("status", BuiltinField::Status),
    ("status_message", BuiltinField::StatusMessage),
    ("instrumentation_name", BuiltinField::InstrumentationName),
    (
        "instrumentation_version",
        BuiltinField::InstrumentationVersion,
    ),
    ("event_name", BuiltinField::EventName),
    ("duration", BuiltinField::Duration),
    ("span_id", BuiltinField::SpanId),
    ("parent_span_id", BuiltinField::ParentSpanId),
    ("trace_id", BuiltinField::TraceId),
    ("link_span_id", BuiltinField::LinkSpanId),
    ("link_trace_id", BuiltinField::LinkTraceId),
    ("event_time_since_start", BuiltinField::EventTimeSinceStart),
    ("root_name", BuiltinField::RootName),
    ("root_service_name", BuiltinField::RootServiceName),
    ("trace_duration", BuiltinField::TraceDuration),
];

/// The attribute owners a selection key may name, as `<owner>.<key>`.
/// The engine enum has two further owners that stay un-named here:
/// `Builtin` is spelled as the bare builtin words above, and the
/// predicates-only `Any` is not wire-nameable at all. Enumeration emits
/// attribute keys owner-qualified ([`render_attribute_key`]), so
/// attribute selections are always qualified; its `.expect` keeps this
/// table in lockstep with the engine enum.
const OWNER_WORDS: [(&str, AttributeOwner); 5] = [
    ("resource", AttributeOwner::Resource),
    ("span", AttributeOwner::Span),
    ("instrumentation", AttributeOwner::Instrumentation),
    ("event", AttributeOwner::Event),
    ("link", AttributeOwner::Link),
];

/// Parse one selection key: `<owner>.<key>` (the key part verbatim, dots
/// included — `resource.service.name` → the `service.name` resource
/// attribute) or a bare builtin word.
pub(crate) fn parse_selection_key(key: &str) -> Result<PredicateTarget, String> {
    for (word, owner) in OWNER_WORDS {
        if let Some(rest) = key.strip_prefix(word).and_then(|r| r.strip_prefix('.')) {
            if rest.is_empty() {
                return Err(format!("selection key {key:?} names no attribute"));
            }
            return Ok(PredicateTarget::Attribute(owner, rest.to_string()));
        }
    }
    BUILTIN_WORDS
        .iter()
        .find(|(w, _)| *w == key)
        .map(|(_, f)| PredicateTarget::Builtin(*f))
        .ok_or_else(|| {
            format!(
                "unknown selection key {key:?}: use resource.<key> / span.<key> / \
                 instrumentation.<key> / event.<key> / link.<key>, or a builtin \
                 field word"
            )
        })
}

/// Build the engine predicate from the wire's filters: each selection
/// key is one Eq condition (values OR within the key, keys AND — the
/// engine's multi-value semantics); the trace-duration bounds become
/// builtin TraceDuration comparisons. Keys iterate sorted so requests are
/// deterministic. An empty value list is no constraint.
pub(crate) fn build_predicate(
    selections: &HashMap<String, Vec<String>>,
    min_trace_duration_ns: Option<i64>,
    max_trace_duration_ns: Option<i64>,
) -> Result<Predicate, String> {
    if let (Some(min), Some(max)) = (min_trace_duration_ns, max_trace_duration_ns)
        && min > max
    {
        return Err(format!(
            "min_trace_duration_ns {min} exceeds max_trace_duration_ns {max}: nothing can match"
        ));
    }
    let mut conditions = Vec::new();
    let mut keys: Vec<&String> = selections.keys().collect();
    keys.sort();
    for key in keys {
        // Validate the key even when its value list is empty — a typo'd
        // key must not hide behind an empty selection.
        let target = parse_selection_key(key)?;
        let values = &selections[key];
        if values.is_empty() {
            continue;
        }
        conditions.push(Condition {
            target,
            op: CompareOp::Eq,
            values: values
                .iter()
                .map(|v| PredicateValue::Text(v.clone()))
                .collect(),
        });
    }
    if let Some(min) = min_trace_duration_ns {
        conditions.push(Condition {
            target: PredicateTarget::Builtin(BuiltinField::TraceDuration),
            op: CompareOp::Gte,
            values: vec![PredicateValue::Integer(min)],
        });
    }
    if let Some(max) = max_trace_duration_ns {
        conditions.push(Condition {
            target: PredicateTarget::Builtin(BuiltinField::TraceDuration),
            op: CompareOp::Lte,
            values: vec![PredicateValue::Integer(max)],
        });
    }
    Ok(Predicate { conditions })
}

// ── Enumeration: key parsing ────────────────────────────────────────

/// A selection-grammar key as the enumeration API's (owner, key) pair.
pub(crate) fn parse_enumeration_key(key: &str) -> Result<(AttributeOwner, AttributeKey), String> {
    Ok(match parse_selection_key(key)? {
        PredicateTarget::Attribute(owner, a) => (owner, AttributeKey::Attribute(a)),
        PredicateTarget::Builtin(f) => (AttributeOwner::Builtin, AttributeKey::Builtin(f)),
    })
}

/// Shape one key's value enumeration into the wire result. `key` is the
/// request's key, echoed in the selection grammar. Values are capped at
/// [`LIST_VALUE_MAX_BYTES`] and flagged when cut.
pub(crate) fn to_attribute_values_result(
    data: AttributeValuesData,
    key: String,
) -> AttributeValuesResult {
    let mut values: Vec<AttributeValueWire> = Vec::with_capacity(data.values.len());
    let mut listed: HashMap<String, usize> = HashMap::new();
    for v in data.values {
        let mut value = v.value;
        let truncated = truncate_value(&mut value, LIST_VALUE_MAX_BYTES);
        // Values sharing a cut prefix list it once, flagged if any was cut.
        if let Some(&i) = listed.get(&value) {
            values[i].truncated |= truncated;
            continue;
        }
        listed.insert(value.clone(), values.len());
        values.push(AttributeValueWire {
            value,
            kind: v.kind.map(kind_word),
            truncated,
        });
    }
    AttributeValuesResult {
        mode: "attribute_values",
        version: 1,
        status: StatusWire::from(&data.status),
        truncated: data.truncated,
        key,
        values,
    }
}

// ── Overview: engine data → wire ────────────────────────────────────

/// Shape the list response's embedded window aggregate. The wire's
/// second-granular geometry derives from the SAME `grid` the engine
/// binned on — one source of truth; the shared derivation guarantees
/// whole seconds (debug-asserted). `coverage` is the grid's aligned
/// window (derived together with `grid`) and `scope` is what the
/// engine pass actually carried, which only the caller knows. The wire
/// contract is `OverviewSection`.
pub(crate) fn to_overview_section(
    data: OverviewData,
    grid: sfst::Grid,
    coverage: CoverageWire,
    scope: &'static str,
) -> OverviewSection {
    const NS_PER_S: i64 = 1_000_000_000;
    debug_assert_eq!(grid.bucket_start_ns % NS_PER_S, 0);
    debug_assert_eq!(grid.bucket_width_ns % NS_PER_S, 0);
    OverviewSection {
        version: 1,
        unit: "traces",
        status: StatusWire::from(&data.status),
        coverage,
        scope,
        grid: OverviewGridWire {
            bucket_start_s: (grid.bucket_start_ns / NS_PER_S) as u32,
            bucket_width_s: (grid.bucket_width_ns / NS_PER_S) as u32,
            duration_bins: DURATION_BIN_LABELS.to_vec(),
            cells: data.cells.iter().map(|row| row.to_vec()).collect(),
            error_cells: data.error_cells.iter().map(|row| row.to_vec()).collect(),
            duration_percentiles_ns: to_percentiles(&data.bucket_percentiles),
        },
        totals: OverviewTotals {
            traces: data.total_traces,
            spans: data.total_spans,
            errors: data.total_errors,
        },
    }
}

/// Transpose the engine's per-bucket percentile triples into one array
/// per rank — the shape a per-rank series is drawn from.
fn to_percentiles(buckets: &[Option<DurationPercentiles>]) -> OverviewPercentilesWire {
    OverviewPercentilesWire {
        p50: buckets.iter().map(|b| b.map(|p| p.p50)).collect(),
        p95: buckets.iter().map(|b| b.map(|p| p.p95)).collect(),
        p99: buckets.iter().map(|b| b.map(|p| p.p99)).collect(),
    }
}

/// The heatmap's predicate from a request's `selections` alone — the
/// page's grammar and builder, minus every duration bound (the grid
/// never applies those; see the wire's `scope` doc). `None` when the
/// selections constrain nothing, so the caller runs the plain grid.
pub(crate) fn heatmap_predicate(
    selections: &HashMap<String, Vec<String>>,
) -> Result<Option<Predicate>, String> {
    let predicate = build_predicate(selections, None, None)?;
    Ok((!predicate.is_all()).then_some(predicate))
}

// ── Search: pagination cursor ───────────────────────────────────────

/// The wire pagination state. The engine has no anchor concept, and its
/// rank — (newest matched-span start DESC, trace_id ASC) — is
/// WINDOW-DEPENDENT: narrowing the window re-ranks any trace whose
/// matched spans straddle the boundary through an older span,
/// duplicating it on a later page. So the cursor NEVER narrows the
/// window. Instead it:
///
/// - FREEZES the canonicalized window of page 1, so every page ranks
///   the same corpus the same way (and a default `now`-derived window
///   can't drift between pages);
/// - carries the AFTER-KEY — the last served trace's (rank, trace_id) —
///   and the served count. The next page re-runs the SAME query with
///   `limit = served + limit` and drops everything ranked at-or-above
///   the key: at a fixed corpus the engine's deterministic total order
///   makes the pages an exact partition, tie runs and straddling
///   traces included.
///
/// Live-data caveats (inherent to most-recent-first pagination; walk a
/// window that ends in the past to avoid them):
///
/// - Late arrivals ranking ABOVE the key are never shown by this walk
///   AND consume the over-fetch allowance — a burst larger than `limit`
///   can shorten a page below `limit` and end the walk early (silently:
///   indistinguishable from exhaustion). Late arrivals ranking BELOW
///   the key appear on later pages normally.
/// - The cursor freezes the window but NOT the filters: echoing it with
///   different selections re-partitions a different result set. The
///   client must keep the filters fixed for the walk's lifetime.
///
/// Cost note: each page reruns the query with `limit = served + limit`,
/// so a full walk of N traces does O(N²/limit) engine ranking work —
/// acceptable at human paging depths and hard-capped by
/// [`CURSOR_SERVED_CAP`]; the engine's own ceilings bound any single
/// request.
#[derive(Debug, Clone, Copy, PartialEq, Eq)]
pub(crate) struct SearchCursor {
    /// The frozen page-1 window, unix seconds.
    pub after_s: u32,
    pub before_s: u32,
    /// The after-key: the last served trace's rank (newest matched-span
    /// start) and id. A trace is already served iff it ranks at-or-above
    /// this key.
    pub rank_ns: i64,
    pub trace_id: sfst::TraceId,
    /// Total traces served so far — the engine over-fetch allowance.
    pub served: usize,
}

/// Walks deeper than this stop paging (the engine limit grows with the
/// served count); the cursor is rejected with advice to narrow.
const CURSOR_SERVED_CAP: usize = 10_000;

/// The cursor's wire form: the `t2:` version prefix plus the five
/// frozen fields, colon-joined. The version prefix (with the exact
/// field count [`parse_cursor`] demands) turns an older format into a
/// clean "malformed anchor" instead of a misread.
pub(crate) fn encode_cursor(c: &SearchCursor) -> String {
    format!(
        "t2:{}:{}:{}:{}:{}",
        c.after_s, c.before_s, c.rank_ns, c.trace_id, c.served
    )
}

/// Parse a cursor produced by [`encode_cursor`]: exact shape, all
/// fields numeric, and the structural checks re-applied — `served` at
/// least 1 (an anchor only exists after a served page) and the frozen
/// window ordered. Anything off is one "malformed anchor" error (the
/// wire calls the cursor an `anchor`); only an over-cap `served` gets
/// its own message.
pub(crate) fn parse_cursor(s: &str) -> Result<SearchCursor, String> {
    let malformed = || format!("malformed anchor {s:?}");
    let mut parts = s.split(':');
    let (Some("t2"), Some(after), Some(before), Some(rank), Some(id), Some(served), None) = (
        parts.next(),
        parts.next(),
        parts.next(),
        parts.next(),
        parts.next(),
        parts.next(),
        parts.next(),
    ) else {
        return Err(malformed());
    };
    let cursor = SearchCursor {
        after_s: after.parse().map_err(|_| malformed())?,
        before_s: before.parse().map_err(|_| malformed())?,
        rank_ns: rank.parse().map_err(|_| malformed())?,
        trace_id: parse_trace_id(id).map_err(|_| malformed())?,
        served: served.parse().map_err(|_| malformed())?,
    };
    if cursor.served == 0 || cursor.after_s >= cursor.before_s {
        return Err(malformed());
    }
    if cursor.served > CURSOR_SERVED_CAP {
        return Err(format!(
            "anchor walk exceeds {CURSOR_SERVED_CAP} traces; narrow the filters or window"
        ));
    }
    Ok(cursor)
}

/// Whether `t` was already served by the cursor's walk: it ranks
/// at-or-above the after-key in the engine's total order — a higher
/// rank, or the same rank with a trace id at-or-before the key's
/// (equal ranks order by ascending id).
fn is_served(c: &SearchCursor, t: &sfsq::traces::TraceSummary) -> bool {
    t.newest_matched_start_ns > c.rank_ns
        || (t.newest_matched_start_ns == c.rank_ns && t.trace_id <= c.trace_id)
}

// ── Search: window canonicalization ─────────────────────────────────

/// The canonicalized search window: the engine bounds in nanoseconds
/// plus the same range as whole seconds — the capture range the
/// supplier prunes candidate files with (file summaries are
/// second-granular; the ns bounds do the exact filtering inside the
/// engine).
#[derive(Debug, Clone, PartialEq, Eq)]
pub(crate) struct ResolvedWindow {
    pub start_ns: i64,
    pub end_ns: i64,
    pub capture: std::ops::Range<u32>,
}

/// Canonicalize the wire window (unix seconds; the logs precedent for
/// unspecified bounds: `before` → now, `after` → before − 900 s). An
/// anchor page uses the cursor's FROZEN window verbatim — the request's
/// own window fields (and the drifting `now`) are ignored so every page
/// of one walk ranks the same corpus.
pub(crate) fn resolve_window(
    after_s: u32,
    before_s: u32,
    now_s: u32,
    anchor: Option<&SearchCursor>,
) -> Result<ResolvedWindow, String> {
    let (after, before) = match anchor {
        Some(c) => (c.after_s, c.before_s), // parse_cursor validated after < before
        None => {
            let before = if before_s == 0 { now_s } else { before_s };
            let after = if after_s == 0 {
                before.saturating_sub(900)
            } else {
                after_s
            };
            if after >= before {
                return Err(format!("invalid window: after {after} >= before {before}"));
            }
            (after, before)
        }
    };
    Ok(ResolvedWindow {
        start_ns: i64::from(after) * 1_000_000_000,
        end_ns: i64::from(before) * 1_000_000_000,
        // Second-granular equivalent for registry file pruning; the ns
        // bounds do the exact filtering inside the engine.
        capture: after..before,
    })
}

// ── Search: engine data → wire ──────────────────────────────────────

/// Shape one search result page. Drops everything the cursor's walk
/// already served (the engine re-emits the served prefix first — same
/// query, same deterministic total order), trims to `limit`, and emits
/// the next cursor only for a FULL page with a COMPLETE status: a short
/// page means the walk is exhausted, and a PARTIAL page's continuation
/// would not be a stable prefix (the engine's work ceilings scale with
/// the limit), so the walk ends there with the status saying why.
/// `frozen` is the window this page actually ran (carried into the next
/// cursor).
pub(crate) fn to_search_result(
    data: SearchData,
    limit: usize,
    incoming: Option<&SearchCursor>,
    frozen: (u32, u32),
) -> SearchResult {
    let mut traces = data.traces;
    if let Some(c) = incoming {
        traces.retain(|t| !is_served(c, t));
    }
    traces.truncate(limit);

    let complete = data.status.is_complete();
    let served_before = incoming.map_or(0, |c| c.served);
    let anchor = (traces.len() == limit && limit > 0 && complete)
        .then(|| {
            let tail = traces.last().expect("page is full, limit > 0");
            let served = served_before + traces.len();
            // A walk past the cap can't over-fetch any further — end it
            // here (the client sees a full page with no continuation).
            (served <= CURSOR_SERVED_CAP).then(|| AnchorWire {
                next: encode_cursor(&SearchCursor {
                    after_s: frozen.0,
                    before_s: frozen.1,
                    rank_ns: tail.newest_matched_start_ns,
                    trace_id: tail.trace_id,
                    served,
                }),
            })
        })
        .flatten();

    SearchResult {
        mode: "search",
        version: 1,
        status: StatusWire::from(&data.status),
        items: SearchItems {
            returned: traces.len(),
            max_to_return: limit,
        },
        traces: traces.into_iter().map(summary_wire).collect(),
        anchor,
        // Composed by the caller when it ran the aggregate pass: only
        // the Functions view does, and only it holds the second engine
        // call's result.
        overview: None,
    }
}

/// One search-page summary → the wire shape, its free-length strings
/// capped at [`LIST_VALUE_MAX_BYTES`].
fn summary_wire(mut t: sfsq::traces::TraceSummary) -> TraceSummaryWire {
    let max = LIST_VALUE_MAX_BYTES;
    let mut truncated = false;
    for s in [&mut t.root_service, &mut t.root_name]
        .into_iter()
        .flatten()
    {
        truncated |= truncate_value(s, max);
    }
    for (name, _) in &mut t.service_breakdown.top {
        truncated |= truncate_value(name, max);
    }
    TraceSummaryWire {
        trace_id: t.trace_id.to_string(),
        root_service: t.root_service,
        root_name: t.root_name,
        start_ns: t.start_ns,
        newest_matched_start_ns: t.newest_matched_start_ns,
        duration_ns: t.duration_ns,
        span_count: t.span_count,
        error_count: t.error_count,
        service_breakdown: service_breakdown_wire(t.service_breakdown),
        exact: t.exact,
        truncated,
    }
}

/// The per-service span breakdown → the wire shape.
fn service_breakdown_wire(b: sfsq::traces::ServiceBreakdown) -> ServiceBreakdownWire {
    ServiceBreakdownWire {
        top: b
            .top
            .into_iter()
            .map(|(value, spans)| ServiceSpansWire { value, spans })
            .collect(),
        other: b.other,
        unattributed: b.unattributed,
    }
}

/// The wire word for each schema value kind. Exhaustive on purpose: a
/// new engine kind fails compilation here until its wire name is decided.
fn kind_word(k: sfst::ValueKind) -> &'static str {
    match k {
        sfst::ValueKind::Null => "null",
        sfst::ValueKind::Bool => "bool",
        sfst::ValueKind::Int => "int",
        sfst::ValueKind::Double => "double",
        sfst::ValueKind::Str => "str",
        sfst::ValueKind::Bytes => "bytes",
        sfst::ValueKind::EmptyKvlist => "empty_kvlist",
        sfst::ValueKind::EmptyArray => "empty_array",
        sfst::ValueKind::Kvlist => "kvlist",
        sfst::ValueKind::Array => "array",
    }
}

#[cfg(test)]
mod tests;
