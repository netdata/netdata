//! `OtelTracesHandler` — typed `FunctionHandler` implementation for the
//! `otel-traces` Function.
//!
//! The full phase-1 mode catalog is implemented: `info` (capability
//! discovery), `trace` (exact single-trace fetch), `search` (bounded
//! most-recent-first trace search, the default mode), the enumeration
//! pair `attributes` / `attribute_values` (the facet rail's
//! vocabulary), and `overview` (the trace-density grid — the UI's
//! default paint). The wire contract lives in [`super::wire`], the
//! engine mapping in [`super::adapter`], and source resolution in
//! [`super::sources`].
//!
//! Netdata-plugin glue only, like the logs handler: the engine
//! ([`sfsq::traces`]) stays wire-neutral; the bridge's `HandlerAdapter`
//! owns the JSON round-trip, progress ticker, and cancellation.

use std::sync::Arc;

use async_trait::async_trait;
use bridge::function::{FunctionCallContext, FunctionHandler};
use file_registry::TenantId;
use netdata_plugin_protocol::FunctionDeclaration;
use netdata_plugin_types::HttpAccess;
use tokio::sync::RwLock;

use file_lifecycle::chunk::ChunkCache;
use file_lifecycle::registry::TenantRegistries;

use sfsq::traces::{
    AttributeNamesQuery, AttributeRequestError, AttributeValuesQuery, OverviewQuery,
    OverviewRequestError, SearchQuery, SearchRequestError, SearchSources, TimeWindow, TraceQuery,
    TraceRequestError, attribute_names, attribute_values, overview, search, trace_by_id,
};

use super::adapter::{
    ResolvedWindow, build_predicate, parse_cursor, parse_enumeration_key, parse_owner_word,
    parse_trace_id, resolve_window, to_attribute_values_result, to_attributes_result,
    to_overview_result, to_search_result, to_trace_result,
};
use super::sources::TracesSourceSupplier;
use super::wire::{InfoResponse, OtelTracesRequest, OtelTracesResponse, RequestMode, SearchResult};

/// Shorthand for the handler-level error every failure path maps to.
fn handler_err(message: String) -> netdata_plugin_error::NetdataPluginError {
    netdata_plugin_error::NetdataPluginError::FunctionHandler { message }
}

pub(crate) struct OtelTracesHandler {
    /// Live source resolution (registries snapshot + WAL chunk builds).
    supplier: TracesSourceSupplier,
}

impl OtelTracesHandler {
    pub(crate) fn new(
        registries: Arc<RwLock<TenantRegistries>>,
        chunk_cache: Arc<ChunkCache>,
        min_entries: u64,
    ) -> Self {
        Self {
            supplier: TracesSourceSupplier::new(registries, chunk_cache, min_entries),
        }
    }

    /// The `trace` mode: exact single-trace fetch via the engine's
    /// cross-source `trace_by_id`.
    ///
    /// Deliberately IGNORES the request window: a trace is an exact
    /// object whose spans straddle files (WAL rotation is
    /// content-agnostic), so by-id captures the FULL range —
    /// window-pruning would silently drop spans, exactly the degradation
    /// the traces engine forbids. An absent id is a Complete empty
    /// trace, not an error.
    async fn trace(
        &self,
        ctx: &FunctionCallContext,
        req: &OtelTracesRequest,
    ) -> netdata_plugin_error::Result<OtelTracesResponse> {
        let params = req
            .trace_params()
            .map_err(|e| handler_err(format!("invalid otel-traces request: {e}")))?;
        let trace_id = parse_trace_id(&params.id)
            .map_err(|e| handler_err(format!("invalid otel-traces request: {e}")))?;
        let mut query = TraceQuery::new(trace_id);
        if let Some(cap) = params.span_cap {
            query = query.span_cap(cap);
        }

        let tenant = TenantId::resolve_query(req.tenant.as_deref());
        // A cancelled capture returns NO copies; the empty default flows
        // into the engine, which polls the same token up front and
        // reports the Cancelled partial — one consistent cancel path.
        let sources = self
            .supplier
            .capture(&tenant, 0..u32::MAX, 1, &ctx.cancellation)
            .await
            .pop()
            .unwrap_or_default();

        // The engine ticks its progress counter once per source; the
        // bridge's ticker renders it. Set before handing the counter off.
        ctx.progress.set_total(sources.len());
        let done = ctx.progress.done_counter();
        let cancel = ctx.cancellation.clone();

        // Sync engine (maps + decompresses files) — off the runtime
        // thread. Engine request errors (unset id, zero cap) are clean
        // client errors; a rejected SOURCE SET is the supplier's
        // inconsistency (duplicate ids / overlapping WAL coverage), not
        // the client's — framed as internal so a debugger looks at the
        // right side. A panicked task is a handler failure.
        let data = match tokio::task::spawn_blocking(move || {
            trace_by_id(sources, query, cancel, done)
        })
        .await
        {
            Ok(Ok(data)) => data,
            Ok(Err(TraceRequestError::SourceSet(e))) => {
                return Err(handler_err(format!(
                    "otel-traces internal error: captured source set is inconsistent: {e}"
                )));
            }
            Ok(Err(e)) => {
                return Err(handler_err(format!("invalid otel-traces request: {e}")));
            }
            Err(e) => {
                return Err(handler_err(format!("otel-traces trace task failed: {e}")));
            }
        };

        Ok(OtelTracesResponse::Trace(Box::new(to_trace_result(
            &trace_id, data,
        ))))
    }

    /// The `search` mode: the engine's bounded most-recent-first trace
    /// search over the request's (canonicalized) window, with wire-level
    /// tie-safe pagination — see the adapter's cursor docs.
    async fn search(
        &self,
        ctx: &FunctionCallContext,
        req: &OtelTracesRequest,
    ) -> netdata_plugin_error::Result<OtelTracesResponse> {
        let client_err =
            |e: String| handler_err(format!("invalid otel-traces request: {e}"));

        // Zero must be rejected BEFORE the anchor allowance is added —
        // `last=0` with a cursor would otherwise sneak a positive engine
        // limit past the engine's own ZeroLimit check.
        if req.last == 0 {
            return Err(client_err(
                "a zero 'last' would return nothing; search has no unbounded option".into(),
            ));
        }
        let cursor = req
            .anchor
            .as_deref()
            .map(parse_cursor)
            .transpose()
            .map_err(client_err)?;
        let predicate = build_predicate(&req.selections, req.min_duration_ns, req.max_duration_ns)
            .map_err(client_err)?;

        let now_s = unix_now_s();
        // An anchor page reruns the SAME query over the cursor's frozen
        // window (never narrowed — the rank is window-dependent) with an
        // over-fetch covering the served prefix; the adapter drops it.
        let window = resolve_window(req.after, req.before, now_s, cursor.as_ref())
            .map_err(client_err)?;

        let mut query = SearchQuery::new(predicate)
            .window(
                TimeWindow::new(window.start_ns, window.end_ns)
                    .map_err(|e| client_err(e.to_string()))?,
            )
            .limit(req.last.saturating_add(cursor.map_or(0, |c| c.served)));
        if let Some(spt) = req.spans_per_trace {
            query = query.spans_per_trace(spt);
        }

        // ONE capture, two roles: search validates window ⊆ completion
        // by source id, so both vectors must be copies of the same
        // captured state.
        let tenant = TenantId::resolve_query(req.tenant.as_deref());
        let mut sets = self
            .supplier
            .capture(&tenant, window.capture.clone(), 2, &ctx.cancellation)
            .await;
        let completion = sets.pop().unwrap_or_default();
        let window_sources = sets.pop().unwrap_or_default();

        ctx.progress.set_total(completion.len());
        let done = ctx.progress.done_counter();
        let cancel = ctx.cancellation.clone();

        let data = match tokio::task::spawn_blocking(move || {
            search(
                SearchSources {
                    window: window_sources,
                    completion,
                },
                query,
                cancel,
                done,
            )
        })
        .await
        {
            Ok(Ok(data)) => data,
            // These two are structurally impossible from one capture —
            // an occurrence means the supplier broke its contract.
            Ok(Err(e @ SearchRequestError::SourceSet(_)))
            | Ok(Err(e @ SearchRequestError::WindowNotInCompletion(_))) => {
                return Err(handler_err(format!(
                    "otel-traces internal error: captured source set is inconsistent: {e}"
                )));
            }
            Ok(Err(e)) => return Err(client_err(e.to_string())),
            Err(e) => {
                return Err(handler_err(format!("otel-traces search task failed: {e}")));
            }
        };

        let result: SearchResult = to_search_result(
            data,
            req.last,
            cursor.as_ref(),
            (window.capture.start, window.capture.end),
        );
        Ok(OtelTracesResponse::Search(Box::new(result)))
    }

    /// Common setup for the enumeration modes: canonicalized window +
    /// one captured source set + the engine window/progress plumbing.
    async fn enumeration_setup(
        &self,
        ctx: &FunctionCallContext,
        req: &OtelTracesRequest,
    ) -> netdata_plugin_error::Result<(Vec<sfsq::traces::TraceSource>, TimeWindow)> {
        let now_s = unix_now_s();
        let window: ResolvedWindow = resolve_window(req.after, req.before, now_s, None)
            .map_err(|e| handler_err(format!("invalid otel-traces request: {e}")))?;
        let engine_window = TimeWindow::new(window.start_ns, window.end_ns)
            .map_err(|e| handler_err(format!("invalid otel-traces request: {e}")))?;

        let tenant = TenantId::resolve_query(req.tenant.as_deref());
        let sources = self
            .supplier
            .capture(&tenant, window.capture, 1, &ctx.cancellation)
            .await
            .pop()
            .unwrap_or_default();
        ctx.progress.set_total(sources.len());
        Ok((sources, engine_window))
    }

    /// The `attributes` mode: exact dictionary-backed key enumeration —
    /// the facet rail's vocabulary, in the selection grammar.
    async fn attributes(
        &self,
        ctx: &FunctionCallContext,
        req: &OtelTracesRequest,
    ) -> netdata_plugin_error::Result<OtelTracesResponse> {
        let client_err = |e: String| handler_err(format!("invalid otel-traces request: {e}"));
        let params = req.attributes_params().map_err(client_err)?;
        let mut query = AttributeNamesQuery::new();
        if let Some(word) = params.owner.as_deref() {
            query = query.owner(parse_owner_word(word).map_err(client_err)?);
        }
        if let Some(max) = params.max_keys {
            query = query.max_keys(max);
        }
        let (sources, window) = self.enumeration_setup(ctx, req).await?;
        query = query.window(window);

        let done = ctx.progress.done_counter();
        let cancel = ctx.cancellation.clone();
        match tokio::task::spawn_blocking(move || attribute_names(sources, query, cancel, done))
            .await
        {
            Ok(Ok(data)) => Ok(OtelTracesResponse::Attributes(to_attributes_result(data))),
            Ok(Err(e)) => Err(map_attribute_error(e)),
            Err(e) => Err(handler_err(format!(
                "otel-traces attributes task failed: {e}"
            ))),
        }
    }

    /// The `attribute_values` mode: one key's exact value vocabulary
    /// (storage labels — what search selections match on).
    async fn attribute_values(
        &self,
        ctx: &FunctionCallContext,
        req: &OtelTracesRequest,
    ) -> netdata_plugin_error::Result<OtelTracesResponse> {
        let client_err = |e: String| handler_err(format!("invalid otel-traces request: {e}"));
        let params = req.attribute_values_params().map_err(client_err)?;
        let (owner, key) = parse_enumeration_key(&params.key).map_err(client_err)?;
        let mut query = AttributeValuesQuery::new(owner, key);
        if let Some(max) = params.max_values {
            query = query.max_values(max);
        }
        let (sources, window) = self.enumeration_setup(ctx, req).await?;
        query = query.window(window);

        let done = ctx.progress.done_counter();
        let cancel = ctx.cancellation.clone();
        let wire_key = params.key;
        match tokio::task::spawn_blocking(move || attribute_values(sources, query, cancel, done))
            .await
        {
            Ok(Ok(data)) => Ok(OtelTracesResponse::AttributeValues(
                to_attribute_values_result(data, wire_key),
            )),
            Ok(Err(e)) => Err(map_attribute_error(e)),
            Err(e) => Err(handler_err(format!(
                "otel-traces attribute_values task failed: {e}"
            ))),
        }
    }
}

/// The wall clock as unix seconds, saturating at the u32 horizon (the
/// registry's second-granular time type).
fn unix_now_s() -> u32 {
    std::time::SystemTime::now()
        .duration_since(std::time::UNIX_EPOCH)
        .unwrap_or_default()
        .as_secs()
        .min(u64::from(u32::MAX)) as u32
}

impl OtelTracesHandler {
    /// The `overview` mode: the trace-density grid (time bucket ×
    /// log-scale duration bin) — the UI's default paint. Geometry
    /// derives from the canonicalized window via the shared nice-width
    /// grid; cells count TRACES by merged envelope, and the span/error
    /// totals are their stored-row sums (labeled on the wire — D9/D10).
    async fn overview(
        &self,
        ctx: &FunctionCallContext,
        req: &OtelTracesRequest,
    ) -> netdata_plugin_error::Result<OtelTracesResponse> {
        let client_err = |e: String| handler_err(format!("invalid otel-traces request: {e}"));
        // v1 carries no knobs, but the parse still rejects null/junk.
        let _params = req.overview_params().map_err(client_err)?;

        let now_s = unix_now_s();
        let window: ResolvedWindow =
            resolve_window(req.after, req.before, now_s, None).map_err(client_err)?;
        let (grid, aligned_after, aligned_before) =
            super::super::grid::grid_for_window_s(window.capture.start, window.capture.end);
        let query = OverviewQuery::new(grid);

        // Alignment can widen the window; prune files by the widened one.
        let tenant = TenantId::resolve_query(req.tenant.as_deref());
        let sources = self
            .supplier
            .capture(&tenant, aligned_after..aligned_before, 1, &ctx.cancellation)
            .await
            .pop()
            .unwrap_or_default();
        ctx.progress.set_total(sources.len());
        let done = ctx.progress.done_counter();
        let cancel = ctx.cancellation.clone();

        match tokio::task::spawn_blocking(move || overview(sources, query, cancel, done)).await {
            Ok(Ok(data)) => Ok(OtelTracesResponse::Overview(Box::new(to_overview_result(
                data, grid,
            )))),
            Ok(Err(OverviewRequestError::SourceSet(e))) => Err(handler_err(format!(
                "otel-traces internal error: captured source set is inconsistent: {e}"
            ))),
            Ok(Err(e)) => Err(client_err(e.to_string())),
            Err(e) => Err(handler_err(format!("otel-traces overview task failed: {e}"))),
        }
    }
}

/// Enumeration request errors: a rejected source set is the supplier's
/// inconsistency (internal); everything else is the client's request.
fn map_attribute_error(e: AttributeRequestError) -> netdata_plugin_error::NetdataPluginError {
    match e {
        AttributeRequestError::SourceSet(e) => handler_err(format!(
            "otel-traces internal error: captured source set is inconsistent: {e}"
        )),
        other => handler_err(format!("invalid otel-traces request: {other}")),
    }
}

#[async_trait]
impl FunctionHandler for OtelTracesHandler {
    type Request = OtelTracesRequest;
    type Response = OtelTracesResponse;

    async fn on_call(
        &self,
        ctx: FunctionCallContext,
        req: Self::Request,
    ) -> netdata_plugin_error::Result<Self::Response> {
        let mode = req
            .mode()
            .map_err(|conflict| handler_err(format!("invalid otel-traces request: {conflict}")))?;
        match mode {
            RequestMode::Info => Ok(OtelTracesResponse::Info(InfoResponse::default())),
            RequestMode::Trace => self.trace(&ctx, &req).await,
            RequestMode::Search => self.search(&ctx, &req).await,
            RequestMode::Attributes => self.attributes(&ctx, &req).await,
            RequestMode::AttributeValues => self.attribute_values(&ctx, &req).await,
            RequestMode::Overview => self.overview(&ctx, &req).await,
        }
    }

    fn declaration(&self) -> FunctionDeclaration {
        let mut d = FunctionDeclaration::new("otel-traces", "Query OpenTelemetry traces");
        d.global = true;
        d.tags = Some("traces".to_string());
        d.access =
            Some(HttpAccess::SIGNED_ID | HttpAccess::SAME_SPACE | HttpAccess::SENSITIVE_DATA);
        d
    }
}

#[cfg(test)]
mod tests;
