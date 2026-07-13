//! The TraceQL name tables (decision 16A: the engine is wire-neutral;
//! every TraceQL string lives here).
//!
//! Sources, verified against grafana-tempo-datasource @ 95f2697:
//! - intrinsics incl. colon forms: `src/traceql/traceql.ts:34-62`;
//! - scope prefixes: `SearchTraceQLEditor/utils.ts:55-68` (lowercased);
//! - enum keyword sets: `traceql.ts:65` (`enumIntrinsics`), `:88`
//!   (`statusValues`); storage labels per `sfsq::traces::vocab`
//!   (`kind` ∈ INTERNAL/SERVER/CLIENT/PRODUCER/CONSUMER, `status` ∈
//!   OK/ERROR — uppercase).

use sfsq::traces::{PredicateTarget, TagScope, TraceIntrinsic};

/// TraceQL kind keyword (lowercase wire form) → storage label. The form
/// generates bare lowercase keywords for enum intrinsics; matching is
/// case-insensitive at the call site. `unspecified` maps to a label the
/// store never carries (OTLP kind 0), so it validly matches nothing.
pub(crate) fn kind_keyword_to_storage(kw: &str) -> Option<&'static str> {
    match kw {
        "internal" => Some("INTERNAL"),
        "server" => Some("SERVER"),
        "client" => Some("CLIENT"),
        "producer" => Some("PRODUCER"),
        "consumer" => Some("CONSUMER"),
        "unspecified" => Some("UNSPECIFIED"),
        _ => None,
    }
}

pub(crate) const KIND_ALLOWED: &str =
    "internal, server, client, producer, consumer, unspecified";

/// TraceQL status keyword → storage label. `unset` is accepted and maps
/// to a label the store never carries (status code 0 is not stored), so
/// it validly matches nothing — Tempo's own UNSET semantics. The
/// plugin's legacy `false`/`true` statusValues are NOT part of OTLP
/// status and are rejected.
pub(crate) fn status_keyword_to_storage(kw: &str) -> Option<&'static str> {
    match kw {
        "ok" => Some("OK"),
        "error" => Some("ERROR"),
        "unset" => Some("UNSET"),
        _ => None,
    }
}

pub(crate) const STATUS_ALLOWED: &str = "ok, error, unset";

/// Attribute scope prefixes the form generates (`utils.ts:55-68`).
const SCOPE_PREFIXES: [(&str, TagScope); 5] = [
    ("resource.", TagScope::Resource),
    ("span.", TagScope::Span),
    ("event.", TagScope::Event),
    ("link.", TagScope::Link),
    ("instrumentation.", TagScope::Instrumentation),
];

/// The full intrinsic name table: v1 intrinsics (no prefix) and every
/// colon form (`traceql.ts:34-62`). `span:duration` and `duration` are
/// the same engine intrinsic; likewise the `trace:` aliases of the
/// trace-level intrinsics.
fn intrinsic(name: &str) -> Option<TraceIntrinsic> {
    use TraceIntrinsic::*;
    Some(match name {
        "name" | "span:name" => Name,
        "kind" | "span:kind" => Kind,
        "status" | "span:status" => Status,
        "statusMessage" | "span:statusMessage" => StatusMessage,
        "duration" | "span:duration" => Duration,
        "rootName" | "trace:rootName" => RootName,
        "rootServiceName" | "trace:rootService" => RootServiceName,
        "traceDuration" | "trace:duration" => TraceDuration,
        "span:id" => SpanId,
        "span:parentID" => ParentSpanId,
        "trace:id" => TraceId,
        "event:name" => EventName,
        "event:timeSinceStart" => EventTimeSinceStart,
        "instrumentation:name" => InstrumentationName,
        "instrumentation:version" => InstrumentationVersion,
        "link:spanID" => LinkSpanId,
        "link:traceID" => LinkTraceId,
        _ => return None,
    })
}

/// Resolve a field token to its predicate target. Precedence: leading
/// dot = unscoped attribute; exact intrinsic name (colon forms
/// included); scope prefix; otherwise a bare ad-hoc field — the
/// dashboard ad-hoc filters generate those with no leading dot
/// (`language_provider.test.ts:267-281`) and they carry unscoped
/// semantics. Unknown colon forms and the `parent.` scope are rejected
/// rather than misread as ad-hoc keys.
pub(crate) fn resolve_field(field: &str) -> Result<PredicateTarget, String> {
    if let Some(key) = field.strip_prefix('.') {
        if key.is_empty() {
            return Err("empty attribute name after '.'".to_string());
        }
        return Ok(PredicateTarget::UnscopedAttribute(key.to_string()));
    }
    if let Some(i) = intrinsic(field) {
        return Ok(PredicateTarget::Intrinsic(i));
    }
    for (prefix, scope) in SCOPE_PREFIXES {
        if let Some(key) = field.strip_prefix(prefix) {
            if key.is_empty() {
                return Err(format!("empty attribute name under {prefix:?}"));
            }
            return Ok(PredicateTarget::Attribute(scope, key.to_string()));
        }
    }
    if field == "parent" || field.starts_with("parent.") {
        return Err(
            "the parent. scope is not supported (the search form never generates it)"
                .to_string(),
        );
    }
    if field.contains(':') {
        return Err(format!("unknown intrinsic {field:?}"));
    }
    Ok(PredicateTarget::UnscopedAttribute(field.to_string()))
}
