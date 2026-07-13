//! The Tempo HTTP surface: an axum router over the `sfsq::traces`
//! engine, mounted by the ledger worker (SOW decision 1). Everything
//! Tempo-shaped — routes, params, headers, response bodies — lives
//! here; the host supplies data through [`SourceSupplier`] and owns the
//! listener lifecycle via [`serve`].
//!
//! Scaffold rules (SOW decisions 2-4 + recorded judgment calls):
//! - Partial search/tags results are served with an
//!   `X-Netdata-Partial: <reasons>` header + a log line; the body stays
//!   strict-jsonpb (Tempo's search/tags protos carry no status field).
//!   Trace-by-id v2 maps PARTIAL through tempopb's real status fields.
//! - Per-request cancellation is a fresh never-fired token (the HTTP
//!   client's disconnect is not propagated — acceptable for the
//!   scaffold; the engine's ceilings bound the work).
//! - `start`/`end` on trace-by-id are accepted and IGNORED: by-id
//!   pruning is the TBLM bloom's job across full retention (plan
//!   decision D1); honoring the window could silently drop spans of
//!   the requested trace.
//! - The `scope` param on tags and the `q` param on tag-values are
//!   accepted and ignored (phase-doc allowance); unknown query params
//!   are never errors (the datasource proxy forwards extras like
//!   `tag=`).
//! - Engine request errors map to 400 with the error's own text;
//!   source-set hygiene failures are OUR supplier's bug and map to 500.

use std::collections::HashMap;
use std::sync::Arc;
use std::sync::atomic::AtomicUsize;

use axum::Router;
use axum::extract::{Path, Query, State};
use axum::http::{StatusCode, header};
use axum::response::{IntoResponse, Response};
use axum::routing::get;
use prost::Message;
use sfsq::traces::{
    QueryStatus, SPSS_MAX, SearchQuery, SearchRequestError, SearchSources, TagKey, TagNamesQuery,
    TagRequestError, TagValuesData, TagValuesQuery, TagScope, TraceIntrinsic, TraceQuery,
    TraceRequestError, TraceSource, search, tag_names, tag_values, trace_by_id,
};
use tokio_util::sync::CancellationToken;

use crate::json::{TagValueStyle, search_response_json, tag_names_json, tag_values_json};
use crate::keywords::resolve_field;
use crate::pb;
use crate::reconstruct::reconstruct_trace;
use crate::request::{parse_trace_id_hex, window_from_unix_seconds};

/// The host's data feed: consistent snapshots of every queryable trace
/// source (sealed SFSTs + active-WAL chunks/tails).
#[async_trait::async_trait]
pub trait SourceSupplier: Send + Sync + 'static {
    /// One snapshot (by-id, tags). Errors are internal (HTTP 500).
    async fn snapshot(&self) -> Result<Vec<TraceSource>, String>;

    /// Two structurally identical snapshots built from ONE consistent
    /// view — search validates window ⊆ completion by source id, so
    /// both sets must come from the same captured state (glm F17).
    async fn snapshot_pair(&self) -> Result<(Vec<TraceSource>, Vec<TraceSource>), String>;
}

#[derive(Clone)]
struct ShimState {
    supplier: Arc<dyn SourceSupplier>,
}

/// The Tempo route table (plugin v13.1.5 surface; anything else is
/// axum's 404 — including the unimplemented `/api/metrics/*`).
pub fn router(supplier: Arc<dyn SourceSupplier>) -> Router {
    Router::new()
        .route("/api/echo", get(echo))
        .route("/api/traces/{id}", get(trace_v1))
        .route("/api/v2/traces/{id}", get(trace_v2))
        .route("/api/search", get(search_endpoint))
        .route("/api/v2/search/tags", get(tags_endpoint))
        .route("/api/v2/search/tag/{*rest}", get(tag_values_endpoint))
        .with_state(ShimState { supplier })
}

/// Run the router on `listener` until `cancel` fires (graceful drain).
pub async fn serve(
    listener: tokio::net::TcpListener,
    supplier: Arc<dyn SourceSupplier>,
    cancel: CancellationToken,
) -> std::io::Result<()> {
    axum::serve(listener, router(supplier))
        .with_graceful_shutdown(cancel.cancelled_owned())
        .await
}

// ── Error plumbing ──────────────────────────────────────────────────

enum ApiError {
    /// Request defect: 400 with the reason as a plain-text body. The
    /// plugin surfaces only the status line for search, so the body
    /// serves curl and logs.
    Bad(String),
    /// Our side failed: 500. Logged; the body stays generic.
    Internal(String),
}

impl IntoResponse for ApiError {
    fn into_response(self) -> Response {
        match self {
            ApiError::Bad(msg) => (StatusCode::BAD_REQUEST, msg).into_response(),
            ApiError::Internal(msg) => {
                tracing::warn!("tempo shim internal error: {msg}");
                (StatusCode::INTERNAL_SERVER_ERROR, "internal error".to_string()).into_response()
            }
        }
    }
}

impl From<SearchRequestError> for ApiError {
    fn from(e: SearchRequestError) -> Self {
        match e {
            // Source-set hygiene is the supplier's contract, not the
            // client's request.
            SearchRequestError::SourceSet(_) | SearchRequestError::WindowNotInCompletion(_) => {
                ApiError::Internal(e.to_string())
            }
            _ => ApiError::Bad(e.to_string()),
        }
    }
}

impl From<TraceRequestError> for ApiError {
    fn from(e: TraceRequestError) -> Self {
        match e {
            TraceRequestError::SourceSet(_) => ApiError::Internal(e.to_string()),
            _ => ApiError::Bad(e.to_string()),
        }
    }
}

impl From<TagRequestError> for ApiError {
    fn from(e: TagRequestError) -> Self {
        match e {
            TagRequestError::SourceSet(_) => ApiError::Internal(e.to_string()),
            _ => ApiError::Bad(e.to_string()),
        }
    }
}

/// Run a sync engine call off the async runtime (the engine functions
/// are pure sync by contract). A panicked/aborted task is a 500, never
/// a worker crash.
async fn run_engine<T: Send + 'static>(
    f: impl FnOnce() -> T + Send + 'static,
) -> Result<T, ApiError> {
    tokio::task::spawn_blocking(f)
        .await
        .map_err(|e| ApiError::Internal(format!("engine task failed: {e}")))
}

// ── Shared param helpers ────────────────────────────────────────────

type Params = HashMap<String, String>;

fn int_param(params: &Params, name: &str) -> Result<i64, ApiError> {
    match params.get(name) {
        None => Ok(0),
        Some(raw) => raw
            .parse()
            .map_err(|_| ApiError::Bad(format!("invalid {name}: {raw:?}"))),
    }
}

fn usize_param(params: &Params, name: &str) -> Result<Option<usize>, ApiError> {
    match params.get(name) {
        None => Ok(None),
        Some(raw) => raw
            .parse()
            .map(Some)
            .map_err(|_| ApiError::Bad(format!("invalid {name}: {raw:?}"))),
    }
}

fn window_param(params: &Params) -> Result<Option<sfsq::traces::TimeWindow>, ApiError> {
    let start = int_param(params, "start")?;
    let end = int_param(params, "end")?;
    window_from_unix_seconds(start, end).map_err(|e| ApiError::Bad(e.to_string()))
}

/// The partial-status header value: comma-joined reasons, `None` when
/// complete (no header). One log line accompanies every partial serve.
fn partial_header(what: &str, status: &QueryStatus) -> Option<String> {
    match status {
        QueryStatus::Complete => None,
        QueryStatus::Partial(reasons) => {
            let joined = reasons
                .iter()
                .map(|r| format!("{r:?}"))
                .collect::<Vec<_>>()
                .join(",");
            tracing::info!("tempo shim: partial {what} result ({joined})");
            Some(joined)
        }
    }
}

const PARTIAL_HEADER: &str = "x-netdata-partial";

fn json_response(body: String, partial: Option<String>) -> Response {
    let mut resp = ([(header::CONTENT_TYPE, "application/json")], body).into_response();
    if let Some(partial) = partial
        && let Ok(value) = partial.parse()
    {
        resp.headers_mut().insert(PARTIAL_HEADER, value);
    }
    resp
}

fn fresh_progress() -> Arc<AtomicUsize> {
    Arc::new(AtomicUsize::new(0))
}

// ── Handlers ────────────────────────────────────────────────────────

/// Save & test: any 200 passes.
async fn echo() -> &'static str {
    "echo"
}

async fn search_endpoint(
    State(st): State<ShimState>,
    Query(params): Query<Params>,
) -> Result<Response, ApiError> {
    // Absent q = empty form (the plugin omits it when the raw editor is
    // empty); the parser maps both to match-all.
    let q = params.get("q").map(String::as_str).unwrap_or("");
    let predicate = crate::parse::parse_query(q).map_err(|e| ApiError::Bad(e.to_string()))?;
    let window = window_param(&params)?;
    let limit = usize_param(&params, "limit")?;
    // Tempo treats spss=0 as "unlimited"; the engine treats 0 as "none
    // attached" — map to the engine maximum (glm F20).
    let spss = usize_param(&params, "spss")?.map(|s| if s == 0 { SPSS_MAX } else { s });

    let (window_set, completion) = st.supplier.snapshot_pair().await.map_err(ApiError::Internal)?;
    let mut query = SearchQuery::new(predicate);
    if let Some(w) = window {
        query = query.window(w);
    }
    if let Some(l) = limit {
        query = query.limit(l);
    }
    if let Some(s) = spss {
        query = query.spss(s);
    }
    let sources = SearchSources {
        window: window_set,
        completion,
    };
    let data = run_engine(move || {
        search(sources, query, CancellationToken::new(), fresh_progress())
    })
    .await??;
    let partial = partial_header("search", &data.status);
    Ok(json_response(search_response_json(&data), partial))
}

async fn trace_v2(
    State(st): State<ShimState>,
    Path(id): Path<String>,
    Query(_params): Query<Params>,
) -> Result<Response, ApiError> {
    let trace_id = parse_trace_id_hex(&id).map_err(|e| ApiError::Bad(e.to_string()))?;
    let sources = st.supplier.snapshot().await.map_err(ApiError::Internal)?;
    let data = run_engine(move || {
        trace_by_id(
            sources,
            TraceQuery::new(trace_id),
            CancellationToken::new(),
            fresh_progress(),
        )
    })
    .await??;
    // A missing trace is 200 with an EMPTY trace on v2 — the plugin
    // does its own not-found mapping (glm F26); 404 here would trigger
    // a pointless v1 fallback round-trip.
    let (status, message) = match &data.status {
        QueryStatus::Complete => (pb::PartialStatus::Complete, String::new()),
        QueryStatus::Partial(reasons) => {
            let joined = reasons
                .iter()
                .map(|r| format!("{r:?}"))
                .collect::<Vec<_>>()
                .join(",");
            tracing::info!("tempo shim: partial trace-by-id result ({joined})");
            (pb::PartialStatus::Partial, joined)
        }
    };
    let resp = pb::TraceByIdResponse {
        trace: Some(reconstruct_trace(trace_id, &data.trace, &data.field_kinds)),
        status: status as i32,
        message,
    };
    Ok((
        [(header::CONTENT_TYPE, "application/protobuf")],
        resp.encode_to_vec(),
    )
        .into_response())
}

async fn trace_v1(
    State(st): State<ShimState>,
    Path(id): Path<String>,
    Query(_params): Query<Params>,
) -> Result<Response, ApiError> {
    let trace_id = parse_trace_id_hex(&id).map_err(|e| ApiError::Bad(e.to_string()))?;
    let sources = st.supplier.snapshot().await.map_err(ApiError::Internal)?;
    let data = run_engine(move || {
        trace_by_id(
            sources,
            TraceQuery::new(trace_id),
            CancellationToken::new(),
            fresh_progress(),
        )
    })
    .await??;
    // v1 semantics: 404 for a missing trace, RAW `tempopb.Trace` bytes
    // (no envelope — the plugin proto.Unmarshals straight into Trace)
    // for a found one. The v1 wire has no status field; partiality
    // still gets the diagnostic header (the plugin ignores it) — on the
    // 404 too, where Partial means "not found among the READABLE
    // sources" rather than a proven absence.
    let partial = partial_header("trace-by-id (v1)", &data.status);
    if data.trace.spans.is_empty() {
        let mut resp = StatusCode::NOT_FOUND.into_response();
        if let Some(partial) = partial
            && let Ok(value) = partial.parse()
        {
            resp.headers_mut().insert(PARTIAL_HEADER, value);
        }
        return Ok(resp);
    }
    let trace = reconstruct_trace(trace_id, &data.trace, &data.field_kinds);
    let mut resp = (
        [(header::CONTENT_TYPE, "application/protobuf")],
        trace.encode_to_vec(),
    )
        .into_response();
    if let Some(partial) = partial
        && let Ok(value) = partial.parse()
    {
        resp.headers_mut().insert(PARTIAL_HEADER, value);
    }
    Ok(resp)
}

async fn tags_endpoint(
    State(st): State<ShimState>,
    Query(params): Query<Params>,
) -> Result<Response, ApiError> {
    // `scope` is accepted and ignored: the response is grouped per
    // scope anyway and the plugin consumes the full grouping.
    let window = window_param(&params)?;
    let limit = usize_param(&params, "limit")?;
    let sources = st.supplier.snapshot().await.map_err(ApiError::Internal)?;
    let mut query = TagNamesQuery::new();
    if let Some(w) = window {
        query = query.window(w);
    }
    if let Some(l) = limit {
        query = query.max_keys(l);
    }
    let data = run_engine(move || {
        tag_names(sources, query, CancellationToken::new(), fresh_progress())
    })
    .await??;
    let partial = partial_header("tags", &data.status);
    Ok(json_response(tag_names_json(&data), partial))
}

async fn tag_values_endpoint(
    State(st): State<ShimState>,
    Path(rest): Path<String>,
    Query(params): Query<Params>,
) -> Result<Response, ApiError> {
    // The wildcard captures `{scoped.tag}/values` (tag names can carry
    // dots, slashes, `[]` — a plain segment param would truncate them).
    let Some(tag) = rest.strip_suffix("/values") else {
        return Ok(StatusCode::NOT_FOUND.into_response());
    };
    // `q` (autocomplete narrowing) is accepted and ignored — phase-doc
    // allowance; values come from the whole window universe.
    let window = window_param(&params)?;
    let limit = usize_param(&params, "limit")?;
    let target =
        resolve_field(tag).map_err(|why| ApiError::Bad(format!("invalid tag {tag:?}: {why}")))?;

    use sfsq::traces::PredicateTarget;
    let (queries, style): (Vec<(TagScope, TagKey)>, TagValueStyle) = match target {
        PredicateTarget::Intrinsic(TraceIntrinsic::Kind) => (
            vec![(TagScope::Intrinsic, TagKey::Intrinsic(TraceIntrinsic::Kind))],
            TagValueStyle::KindKeywords,
        ),
        PredicateTarget::Intrinsic(TraceIntrinsic::Status) => (
            vec![(TagScope::Intrinsic, TagKey::Intrinsic(TraceIntrinsic::Status))],
            TagValueStyle::StatusKeywords,
        ),
        PredicateTarget::Intrinsic(i) => (
            vec![(TagScope::Intrinsic, TagKey::Intrinsic(i))],
            TagValueStyle::Typed,
        ),
        PredicateTarget::Attribute(scope, key) => {
            (vec![(scope, TagKey::Attribute(key))], TagValueStyle::Typed)
        }
        // An unscoped tag is the resource ∪ span union, mirroring its
        // search semantics — which is Tempo's OWN rule: it routes
        // AttributeScopeNone conditions to span+resource only
        // (tempo vparquet4/block_traceql.go:1665-1670); event/link/
        // instrumentation attributes require their explicit scope
        // there too. Grafana's unscoped picker flattens every
        // non-intrinsic scope into the list, so it can offer tags this
        // lookup returns no values for — the identical dead-end exists
        // against real Tempo (a UI quirk, not a shim divergence).
        PredicateTarget::UnscopedAttribute(key) => (
            vec![
                (TagScope::Resource, TagKey::Attribute(key.clone())),
                (TagScope::Span, TagKey::Attribute(key)),
            ],
            TagValueStyle::Typed,
        ),
    };

    // Both scope queries of an unscoped union come from ONE consistent
    // snapshot (two copies with equal source ids), and each carries the
    // caller's limit: a scope's returned values are its sorted prefix,
    // so any value in the true union's top-`limit` is inside one of the
    // prefixes — the sorted merge below plus a global cap is exact.
    let mut source_sets: Vec<Vec<TraceSource>> = if queries.len() == 2 {
        let (a, b) = st.supplier.snapshot_pair().await.map_err(ApiError::Internal)?;
        vec![a, b]
    } else {
        vec![st.supplier.snapshot().await.map_err(ApiError::Internal)?]
    };
    let mut merged: Option<TagValuesData> = None;
    for (scope, key) in queries {
        let mut query = TagValuesQuery::new(scope, key);
        if let Some(w) = window {
            query = query.window(w);
        }
        if let Some(l) = limit {
            query = query.max_values(l);
        }
        let sources = source_sets.pop().expect("one source set per query");
        let data = run_engine(move || {
            tag_values(sources, query, CancellationToken::new(), fresh_progress())
        })
        .await??;
        merged = Some(match merged {
            None => data,
            Some(prev) => merge_tag_values(prev, data, limit),
        });
    }
    let data = merged.expect("queries is never empty");
    let partial = partial_header("tag-values", &data.status);
    Ok(json_response(tag_values_json(&data, style), partial))
}

/// Fold two per-scope value sets into one (the unscoped union): a
/// sorted merge deduped by value bytes (first kind wins — engine values
/// arrive sorted, and the merge re-sorts defensively), then ONE global
/// cap with an exact `truncated` (either scope truncated, or the union
/// itself overflowed the cap); partial reasons union.
fn merge_tag_values(mut a: TagValuesData, b: TagValuesData, limit: Option<usize>) -> TagValuesData {
    let known: std::collections::HashSet<String> =
        a.values.iter().map(|v| v.value.clone()).collect();
    for v in b.values {
        if !known.contains(&v.value) {
            a.values.push(v);
        }
    }
    a.values.sort_by(|x, y| x.value.cmp(&y.value));
    a.truncated |= b.truncated;
    if let Some(limit) = limit
        && a.values.len() > limit
    {
        a.values.truncate(limit);
        a.truncated = true;
    }
    a.status = match (a.status, b.status) {
        (QueryStatus::Complete, s) | (s, QueryStatus::Complete) => s,
        (QueryStatus::Partial(mut x), QueryStatus::Partial(y)) => {
            x.extend(y);
            QueryStatus::Partial(x)
        }
    };
    a
}

#[cfg(test)]
#[path = "server_tests.rs"]
mod tests;
