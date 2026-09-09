//! Deterministic synthetic OTLP **trace** generation — the traces analogue of
//! [`crate::synth`]. Pure (no RNG, no clock): span `i` is a deterministic
//! function of `i` + params, so the same params always produce the same corpus.
//!
//! Generates real trace *trees* (a root plus a binary-ish tree of children),
//! span events (exception-shaped and annotation-shaped), links across traces,
//! and — behind explicit knobs — the edge cases the read path must survive:
//! orphan spans (parent absent from the trace), extra roots, and resent
//! (duplicated) spans. `spans_per_trace = 1` degenerates to the original flat
//! corpus (per-span-unique names, no status/trace_state), keeping existing
//! callers' shape unchanged — with ONE deliberate deviation: trace/span ids now
//! ride a never-all-zero derivation (the old scheme emitted the UNSET all-zero
//! span id for index 0 with seed 0, which the reader skips as invalid).

use opentelemetry_proto::tonic::{
    collector::trace::v1::ExportTraceServiceRequest,
    common::v1::InstrumentationScope,
    resource::v1::Resource,
    trace::v1::{
        ResourceSpans, ScopeSpans, Span, Status,
        span::{Event, Link, SpanKind},
        status::StatusCode,
    },
};

use crate::otel::{kv, str_val};

#[derive(Debug, Clone)]
pub struct SynthTraceParams {
    /// Number of spans to generate (before `resend_every` duplication).
    pub count: usize,
    /// Start time of the first span (unix nanos); span `i` starts at
    /// `start_time_nanos + i * spacing_nanos`.
    pub start_time_nanos: u64,
    /// Nanoseconds between consecutive span start times.
    pub spacing_nanos: u64,
    /// Base span duration (a span's end = start + this scaled by its subtree).
    pub duration_nanos: u64,
    /// Offset added to trace/span indices before deriving ids/values.
    pub seed: u64,
    /// Spans per trace: `1` = flat (every span its own trace, the original
    /// shape); `> 1` = a root plus a binary-ish tree of children.
    pub spans_per_trace: usize,
    /// Events emitted on every span (`0` = none). The first event of a span is
    /// exception-shaped (`exception.type`/`.message`/`.stacktrace`), the rest
    /// are annotation-shaped.
    pub events_per_span: usize,
    /// Every Nth span (globally, `0` = never) links to the previous trace's
    /// root — the producer↔consumer correlation shape.
    pub links_every: usize,
    /// Every Nth trace's last span (`0` = never) gets a parent id that exists
    /// nowhere — an orphan the reader must promote to a root.
    pub orphan_every: usize,
    /// Every Nth trace's last span (`0` = never) gets an empty parent — a
    /// second root in the same trace. An orphan wins when both knobs select
    /// the same trace.
    pub extra_root_every: usize,
    /// Every Nth span (globally, `0` = never) is emitted twice — a resend the
    /// reader must collapse.
    pub resend_every: usize,
}

impl SynthTraceParams {
    /// The original flat corpus: one span per trace, no events/links/edge cases.
    pub fn flat(
        count: usize,
        start_time_nanos: u64,
        spacing_nanos: u64,
        duration_nanos: u64,
        seed: u64,
    ) -> Self {
        Self {
            count,
            start_time_nanos,
            spacing_nanos,
            duration_nanos,
            seed,
            spans_per_trace: 1,
            events_per_span: 0,
            links_every: 0,
            orphan_every: 0,
            extra_root_every: 0,
            resend_every: 0,
        }
    }
}

/// Deterministic 16-byte trace id for trace index `t` (never all-zero).
fn trace_id(t: u64) -> Vec<u8> {
    ((t as u128) | (1u128 << 120)).to_be_bytes().to_vec()
}

/// Deterministic 8-byte span id for (trace `t`, position `j`) (never all-zero).
fn span_id(t: u64, j: u64) -> Vec<u8> {
    ((t << 20) | (j + 1) | (1u64 << 63)).to_be_bytes().to_vec()
}

/// Build the deterministic span batch. Spans are grouped into traces of
/// `spans_per_trace`: position 0 is the root (`SERVER`), position `j > 0`
/// parents on position `(j - 1) / 2` (`CLIENT`/`INTERNAL` alternating) — a
/// binary-ish tree with depth ~log2 and fanout 2. Start times are globally
/// monotonic; a span's duration scales with its distance from the trace's
/// tail so parents envelope their children. Roots of every 7th trace carry an
/// `ERROR` status + message; roots of every 5th trace carry a `trace_state` —
/// so the deferred scalar fields flow through any corpus with `count ≥ ~10`.
pub fn generate(p: &SynthTraceParams) -> Vec<Span> {
    let m = p.spans_per_trace.max(1) as u64;
    let mut out: Vec<Span> = Vec::with_capacity(p.count);
    for i in 0..p.count as u64 {
        let t = i / m + p.seed;
        let j = i % m;
        let start = p
            .start_time_nanos
            .saturating_add(i.saturating_mul(p.spacing_nanos));
        let is_root = j == 0;

        let parent_span_id = if is_root {
            Vec::new()
        } else {
            span_id(t, (j - 1) / 2)
        };
        let kind = if is_root {
            SpanKind::Server
        } else if j % 2 == 1 {
            SpanKind::Client
        } else {
            SpanKind::Internal
        };

        // The span's full extent (see end_time_unix_nano below): events
        // clamp to it so they stay span-contained at any events_per_span
        // (unclamped, an event landed past a leaf span's end whenever
        // (k+1)*(duration/4+1) > extent — at the default duration that
        // is k >= 3; at small or zero durations, earlier).
        let extent = p.duration_nanos.saturating_add(
            (m - 1 - j).saturating_mul(p.spacing_nanos.saturating_add(p.duration_nanos)),
        );
        let events = (0..p.events_per_span as u64)
            .map(|k| {
                let time = start.saturating_add(
                    (k + 1).saturating_mul(p.duration_nanos / 4 + 1).min(extent),
                );
                if k == 0 {
                    Event {
                        time_unix_nano: time,
                        name: "exception".into(),
                        attributes: vec![
                            kv("exception.type", str_val("SyntheticError")),
                            kv("exception.message", str_val(&format!("failure {t}/{j}"))),
                            kv(
                                "exception.stacktrace",
                                str_val("at synth()\n  at generate()\n  at main()"),
                            ),
                        ],
                        dropped_attributes_count: (j % 5 == 4) as u32,
                    }
                } else {
                    Event {
                        time_unix_nano: time,
                        name: format!("annotation-{k}"),
                        attributes: vec![kv("event.index", str_val(&k.to_string()))],
                        dropped_attributes_count: 0,
                    }
                }
            })
            .collect();

        // Every Nth span links to the PREVIOUS trace's root (skipping the very
        // first trace of this run, which has no predecessor).
        let links = if p.links_every > 0 && i % p.links_every as u64 == 0 && t > p.seed {
            vec![Link {
                trace_id: trace_id(t - 1),
                span_id: span_id(t - 1, 0),
                trace_state: "synth=1".into(),
                attributes: vec![kv("messaging.operation", str_val("publish"))],
                dropped_attributes_count: 0,
                flags: 0x100,
            }]
        } else {
            Vec::new()
        };

        // Tree mode only: flat mode (m == 1) preserves the original corpus shape
        // (no status, no trace_state, per-span-unique names below).
        let status = if is_root && m > 1 && t % 7 == 3 {
            Some(Status {
                code: StatusCode::Error as i32,
                message: format!("synthetic failure in trace {t}"),
            })
        } else {
            None
        };
        let trace_state = if is_root && m > 1 && t % 5 == 2 {
            "ot=th:8;synth=1".to_string()
        } else {
            String::new()
        };

        // Flat mode names by GLOBAL index (the original per-span-unique shape);
        // tree mode names by in-trace position so siblings across traces align.
        let name = if m == 1 {
            format!("synthetic-span-{i}")
        } else {
            format!("synthetic-span-{j}")
        };
        out.push(Span {
            trace_id: trace_id(t),
            span_id: span_id(t, j),
            parent_span_id,
            name,
            kind: kind as i32,
            start_time_unix_nano: start,
            // `extent = duration + (m-1-j)·(spacing+duration)`: parents always
            // envelope their children (each level up gains one child's
            // start-gap plus duration), and `m == 1` degenerates to exactly
            // `duration_nanos`.
            end_time_unix_nano: start.saturating_add(extent),
            attributes: vec![kv("span.index", str_val(&i.to_string()))],
            events,
            links,
            status,
            trace_state,
            ..Default::default()
        });
    }

    // Edge cases, applied deterministically over the generated corpus.
    let m_us = m as usize;
    for (idx, span) in out.iter_mut().enumerate() {
        let t = idx / m_us;
        let is_last_of_trace = idx % m_us == m_us - 1;
        if !is_last_of_trace || m_us < 2 {
            continue;
        }
        if p.orphan_every > 0 && (t + 1) % p.orphan_every == 0 {
            // A parent id that exists nowhere in the corpus.
            span.parent_span_id = u64::MAX.to_be_bytes().to_vec();
        } else if p.extra_root_every > 0 && (t + 1) % p.extra_root_every == 0 {
            span.parent_span_id = Vec::new(); // second root
        }
    }
    if p.resend_every > 0 {
        let dups = out.len().checked_div(p.resend_every).unwrap_or(0);
        let mut resent = Vec::with_capacity(out.len() + dups + 1);
        for (idx, span) in out.into_iter().enumerate() {
            let dup = idx % p.resend_every == p.resend_every - 1;
            if dup {
                resent.push(span.clone());
            }
            resent.push(span);
        }
        out = resent;
    }
    out
}

/// Wrap spans in an [`ExportTraceServiceRequest`] under one resource/scope.
pub fn build_request(
    spans: Vec<Span>,
    service_name: &str,
    service_namespace: Option<&str>,
    scope_name: &str,
    scope_version: &str,
) -> ExportTraceServiceRequest {
    let mut attributes = vec![kv("service.name", str_val(service_name))];
    if let Some(namespace) = service_namespace {
        attributes.push(kv("service.namespace", str_val(namespace)));
    }
    ExportTraceServiceRequest {
        resource_spans: vec![ResourceSpans {
            resource: Some(Resource {
                attributes,
                dropped_attributes_count: 0,
                entity_refs: vec![],
            }),
            scope_spans: vec![ScopeSpans {
                scope: Some(InstrumentationScope {
                    name: scope_name.to_string(),
                    version: scope_version.to_string(),
                    attributes: vec![],
                    dropped_attributes_count: 0,
                }),
                spans,
                schema_url: String::new(),
            }],
            schema_url: String::new(),
        }],
    }
}

#[cfg(test)]
mod tests {
    use super::*;
    use std::collections::{BTreeMap, BTreeSet};

    fn params(count: usize) -> SynthTraceParams {
        SynthTraceParams::flat(count, 1_000_000_000_000, 1_000_000_000, 5_000_000, 0)
    }

    fn tree_params(count: usize, per_trace: usize) -> SynthTraceParams {
        SynthTraceParams {
            spans_per_trace: per_trace,
            events_per_span: 2,
            links_every: 5,
            ..params(count)
        }
    }

    /// The clamp's invariant: every event stays within its span's
    /// [start, end] at an events_per_span past the clamp threshold
    /// (offsets exceed the extent on leaf spans from k >= 3 at the
    /// default duration) — including the flat shape, where every span
    /// is a leaf.
    #[test]
    fn events_stay_span_contained_past_the_clamp_threshold() {
        for per_trace in [1, 8] {
            let spans = generate(&SynthTraceParams {
                spans_per_trace: per_trace,
                events_per_span: 5,
                ..params(64)
            });
            for s in &spans {
                for e in &s.events {
                    assert!(
                        e.time_unix_nano >= s.start_time_unix_nano
                            && e.time_unix_nano <= s.end_time_unix_nano,
                        "event at {} outside span [{}, {}] (per_trace={per_trace})",
                        e.time_unix_nano,
                        s.start_time_unix_nano,
                        s.end_time_unix_nano
                    );
                }
            }
        }
    }

    #[test]
    fn count_ids_and_monotonic_times() {
        let spans = generate(&params(5));
        assert_eq!(spans.len(), 5);
        for w in spans.windows(2) {
            assert!(w[1].start_time_unix_nano > w[0].start_time_unix_nano);
        }
        // Ids are the right widths and unique per span.
        assert!(
            spans
                .iter()
                .all(|s| s.trace_id.len() == 16 && s.span_id.len() == 8)
        );
        let ids: BTreeSet<_> = spans.iter().map(|s| s.span_id.clone()).collect();
        assert_eq!(ids.len(), 5);
        // Flat corpus preserves the original shape: every span its own root
        // with a per-span-unique name; no events/links/status/trace_state.
        assert!(spans.iter().all(|s| s.parent_span_id.is_empty()));
        let names: BTreeSet<_> = spans.iter().map(|s| s.name.clone()).collect();
        assert_eq!(names.len(), 5, "flat names are per-span unique");
        assert_eq!(spans[3].name, "synthetic-span-3");
        assert!(
            spans
                .iter()
                .all(|s| s.events.is_empty() && s.links.is_empty())
        );
        assert!(
            spans
                .iter()
                .all(|s| s.status.is_none() && s.trace_state.is_empty()),
            "flat mode emits no status/trace_state regardless of trace index"
        );
        // Ids never collide with the UNSET sentinel, even at index 0 / seed 0.
        assert!(spans.iter().all(|s| s.span_id != vec![0u8; 8]));
    }

    #[test]
    fn deterministic_for_same_params() {
        assert_eq!(generate(&params(8)), generate(&params(8)));
        assert_eq!(generate(&tree_params(40, 8)), generate(&tree_params(40, 8)));
    }

    #[test]
    fn trees_have_one_root_and_valid_parents() {
        let spans = generate(&tree_params(24, 8));
        let mut by_trace: BTreeMap<Vec<u8>, Vec<&Span>> = BTreeMap::new();
        for s in &spans {
            by_trace.entry(s.trace_id.clone()).or_default().push(s);
        }
        assert_eq!(by_trace.len(), 3);
        for (_, spans) in by_trace {
            let ids: BTreeSet<_> = spans.iter().map(|s| s.span_id.clone()).collect();
            let roots: Vec<_> = spans
                .iter()
                .filter(|s| s.parent_span_id.is_empty())
                .collect();
            assert_eq!(roots.len(), 1, "exactly one root per trace");
            assert_eq!(roots[0].kind, SpanKind::Server as i32);
            for s in &spans {
                if !s.parent_span_id.is_empty() {
                    assert!(ids.contains(&s.parent_span_id), "parent exists in-trace");
                    // Parents envelope children.
                    let parent = spans
                        .iter()
                        .find(|p| p.span_id == s.parent_span_id)
                        .unwrap();
                    assert!(parent.start_time_unix_nano <= s.start_time_unix_nano);
                    assert!(parent.end_time_unix_nano >= s.end_time_unix_nano);
                }
            }
        }
    }

    #[test]
    fn events_and_links_are_emitted() {
        // 8 traces so the every-7th/every-5th scalar knobs both fire.
        let spans = generate(&tree_params(64, 8));
        assert!(spans.iter().all(|s| s.events.len() == 2));
        let first = &spans[0].events[0];
        assert_eq!(first.name, "exception");
        assert!(
            first
                .attributes
                .iter()
                .any(|a| a.key == "exception.stacktrace")
        );
        assert!(first.time_unix_nano > spans[0].start_time_unix_nano);
        // links_every=5 across 3 traces; the first trace has no predecessor.
        let linked: Vec<_> = spans.iter().filter(|s| !s.links.is_empty()).collect();
        assert!(!linked.is_empty());
        for s in &linked {
            let link = &s.links[0];
            assert_eq!(link.trace_id.len(), 16);
            assert_eq!(link.span_id.len(), 8);
            assert_eq!(link.trace_state, "synth=1");
        }
        // Deferred scalars flow: some root carries status/message, some trace_state.
        assert!(spans.iter().any(|s| s.status.is_some()));
        assert!(spans.iter().any(|s| !s.trace_state.is_empty()));
    }

    #[test]
    fn orphans_extra_roots_and_resends() {
        let p = SynthTraceParams {
            spans_per_trace: 4,
            orphan_every: 2,     // traces 2, 4, 6 (1-based) get an orphan
            extra_root_every: 3, // traces 3 (1-based) get a second root (orphan wins ties)
            resend_every: 7,
            ..params(24)
        };
        let spans = generate(&p);
        // Resends: 24 spans + one dup per 7 → 3 dups.
        assert_eq!(spans.len(), 27);
        let mut counts: BTreeMap<Vec<u8>, usize> = BTreeMap::new();
        for s in &spans {
            *counts.entry(s.span_id.clone()).or_default() += 1;
        }
        assert_eq!(counts.values().filter(|&&c| c == 2).count(), 3);

        // Orphans: the last span of qualifying traces parents a nonexistent id.
        let orphan_parent = u64::MAX.to_be_bytes().to_vec();
        let all_ids: BTreeSet<_> = spans.iter().map(|s| s.span_id.clone()).collect();
        let orphans: Vec<_> = spans
            .iter()
            .filter(|s| s.parent_span_id == orphan_parent)
            .collect();
        assert_eq!(orphans.len(), 3);
        assert!(!all_ids.contains(&orphan_parent));

        // Extra roots: 1-based trace 3 qualifies for extra_root and not orphan
        // → two parentless spans in that trace (0-based trace index 2).
        let tid = trace_id(2);
        let roots = spans
            .iter()
            .filter(|s| s.trace_id == tid && s.parent_span_id.is_empty())
            .count();
        assert_eq!(roots, 2);
    }

    #[test]
    fn request_carries_service_identity_and_spans() {
        let req = build_request(generate(&params(3)), "svc", Some("ns"), "scope", "1.0");
        let rs = &req.resource_spans[0];
        let attrs: Vec<_> = rs
            .resource
            .as_ref()
            .unwrap()
            .attributes
            .iter()
            .map(|a| a.key.clone())
            .collect();
        assert_eq!(attrs, vec!["service.name", "service.namespace"]);
        assert_eq!(rs.scope_spans[0].spans.len(), 3);
    }
}
