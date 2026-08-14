//! `OtelTracesHandler` — typed `FunctionHandler` implementation for the
//! `otel-traces` Function.
//!
//! The full mode catalog is implemented: `info` (capability
//! discovery), `trace` (exact single-trace fetch), `search` (bounded
//! most-recent-first trace search), the enumeration pair `attributes`
//! / `attribute_values` (the facet rail's vocabulary), `overview` (the
//! trace-density grid — the UI's default paint), and `slowest` (the
//! window's duration-ranked top-K traces). Mode selection and every
//! request-SHAPE validation happen during deserialization (the wire's
//! typed request — shape errors are transport 400s); this handler owns
//! only the semantic validation (trace-id shape, zero limit, bounds).
//! The wire contract lives in [`super::wire`], the engine mapping in
//! [`super::adapter`], and source resolution in [`super::sources`].
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
    AttributeNamesQuery, AttributeRequestError, AttributeValuesQuery, DEFAULT_SLOWEST_LIMIT,
    OverviewQuery, OverviewRequestError, SLOWEST_LIMIT_MAX, SPANS_PER_TRACE_MAX, SearchQuery,
    SearchRequestError, SearchSources, SlowestQuery, SlowestRequestError, TimeWindow, TraceQuery,
    TraceRequestError, attribute_names, attribute_values, overview, search, slowest, trace_by_id,
};

use super::adapter::{
    ResolvedWindow, build_predicate, parse_cursor, parse_enumeration_key, parse_owner_word,
    completion_capture_range, parse_trace_id, resolve_window, to_attribute_values_result,
    to_attributes_result, to_overview_result, to_search_result, to_slowest_result,
    to_trace_result, validate_trace_bounds,
};
use super::sources::TracesSourceSupplier;
use super::wire::{
    AttributeValuesParams, AttributesParams, CoverageWire, InfoResponse, OtelTracesRequest,
    OtelTracesResponse, OverviewParams, SearchParams, SearchResult, SlowestParams, TraceParams,
    TracesMode,
};

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
    /// Ignores the ENVELOPE window; assembly bounds live in the `trace`
    /// sub-object. Absent bounds capture the FULL range — a trace is an
    /// exact object whose spans straddle files (WAL rotation is
    /// content-agnostic), and only the caller knows how much slack its
    /// anchor deserves. Present bounds prune the capture file-granularly
    /// (a file overlapping the bounds is probed whole). Either way the
    /// response DECLARES the range used (`coverage`) — spans beyond it
    /// are unknown, never silently dropped: the declaration is the
    /// honesty. An absent id is a Complete empty trace, not an error.
    async fn trace(
        &self,
        ctx: &FunctionCallContext,
        params: &TraceParams,
        tenant: Option<&str>,
    ) -> netdata_plugin_error::Result<OtelTracesResponse> {
        let trace_id = parse_trace_id(&params.id)
            .map_err(|e| handler_err(format!("invalid otel-traces request: {e}")))?;
        // Pre-capture twin of the engine's UnsetTraceId check (the
        // parser deliberately lets the sentinel through).
        if trace_id.is_unset() {
            return Err(handler_err(
                "invalid otel-traces request: the all-zero (unset) trace id cannot be looked up"
                    .to_string(),
            ));
        }
        let mut query = TraceQuery::new(trace_id);
        if let Some(cap) = params.span_cap {
            // Pre-capture twin of the engine's ZeroSpanCap check.
            if cap == 0 {
                return Err(handler_err(
                    "invalid otel-traces request: a zero span cap would return nothing; \
                     omit the cap or raise it"
                        .to_string(),
                ));
            }
            // The wire may TIGHTEN the engine's runaway-merge bound,
            // never loosen it — an oversized cap would defeat the
            // default's documented purpose (see DEFAULT_SPAN_CAP).
            if cap > sfsq::traces::DEFAULT_SPAN_CAP {
                return Err(handler_err(format!(
                    "invalid otel-traces request: 'span_cap' {cap} exceeds the maximum {}",
                    sfsq::traces::DEFAULT_SPAN_CAP
                )));
            }
            query = query.span_cap(cap);
        }

        let bounds = validate_trace_bounds(params.after, params.before)
            .map_err(|e| handler_err(format!("invalid otel-traces request: {e}")))?;
        let capture_range = bounds.unwrap_or(0..u32::MAX);
        let coverage = CoverageWire {
            after: capture_range.start,
            before: capture_range.end,
        };

        let tenant = TenantId::resolve_query(tenant);
        // A cancelled capture returns NO copies; the empty default flows
        // into the engine, which polls the same token up front and
        // reports the Cancelled partial — one consistent cancel path.
        let sources = self
            .supplier
            .capture(&tenant, capture_range, 1, &ctx.cancellation)
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
            &trace_id, data, coverage,
        ))))
    }

    /// The `search` mode: the engine's bounded most-recent-first trace
    /// search over the request's (canonicalized) window, with wire-level
    /// tie-safe pagination — see the adapter's cursor docs.
    async fn search(
        &self,
        ctx: &FunctionCallContext,
        params: &SearchParams,
        tenant: Option<&str>,
    ) -> netdata_plugin_error::Result<OtelTracesResponse> {
        let client_err =
            |e: String| handler_err(format!("invalid otel-traces request: {e}"));

        // Zero must be rejected BEFORE the anchor allowance is added —
        // `limit=0` with a cursor would otherwise sneak a positive engine
        // limit past the engine's own ZeroLimit check.
        if params.limit == 0 {
            return Err(client_err(
                "a zero 'limit' would return nothing; search has no unbounded option".into(),
            ));
        }
        // The wire cap on the CLIENT half of the limit; the engine's
        // effective limit (served + limit on cursor walks) is bounded by
        // construction once this side is capped (see SEARCH_LIMIT_MAX).
        if params.limit > super::wire::SEARCH_LIMIT_MAX {
            return Err(client_err(format!(
                "'limit' {} exceeds the maximum {}",
                params.limit,
                super::wire::SEARCH_LIMIT_MAX
            )));
        }
        // Pre-capture twin of the engine's own check: reject before the
        // registry read and chunk builds capture() performs (the engine
        // re-checks as defense in depth).
        if let Some(spt) = params.spans_per_trace {
            if spt > SPANS_PER_TRACE_MAX {
                return Err(client_err(format!(
                    "spans_per_trace {spt} exceeds the library maximum {SPANS_PER_TRACE_MAX}"
                )));
            }
        }
        let cursor = params
            .anchor
            .as_deref()
            .map(parse_cursor)
            .transpose()
            .map_err(client_err)?;
        let predicate = build_predicate(
            &params.selections,
            params.min_duration_ns,
            params.max_duration_ns,
            params.min_trace_duration_ns,
            params.max_trace_duration_ns,
        )
        .map_err(client_err)?;

        let now_s = unix_now_s();
        // An anchor page reruns the SAME query over the cursor's frozen
        // window (never narrowed — the rank is window-dependent) with an
        // over-fetch covering the served prefix; the adapter drops it.
        let window = resolve_window(params.after, params.before, now_s, cursor.as_ref())
            .map_err(client_err)?;

        let mut query = SearchQuery::new(predicate)
            .window(
                TimeWindow::new(window.start_ns, window.end_ns)
                    .map_err(|e| client_err(e.to_string()))?,
            )
            .limit(params.limit.saturating_add(cursor.map_or(0, |c| c.served)));
        if let Some(spt) = params.spans_per_trace {
            query = query.spans_per_trace(spt);
        }

        // ONE capture over the COMPLETION range (the match window
        // widened by the clamped slack), two roles: the engine narrows
        // the window role internally (SFSTs by summary overlap, tail
        // spans per-span), so identical copies keep window ⊆ completion
        // by construction while slack-only files still complete
        // straddling hits. The cursor keeps freezing the ORIGINAL
        // window (window.capture) — the completion range re-derives
        // from it deterministically on every page.
        let completion_range = completion_capture_range(&window.capture);
        let completion_coverage = CoverageWire {
            after: completion_range.start,
            before: completion_range.end,
        };
        let tenant = TenantId::resolve_query(tenant);
        let mut sets = self
            .supplier
            .capture(&tenant, completion_range, 2, &ctx.cancellation)
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
            params.limit,
            cursor.as_ref(),
            (window.capture.start, window.capture.end),
            completion_coverage,
        );
        Ok(OtelTracesResponse::Search(Box::new(result)))
    }

    /// Common setup for the windowed fold modes (enumeration, slowest):
    /// canonicalized window + one captured source set + the engine
    /// window/progress plumbing. Callers pass their own params' window
    /// fields — every mode's window is self-contained on the wire.
    async fn enumeration_setup(
        &self,
        ctx: &FunctionCallContext,
        after: u32,
        before: u32,
        tenant: Option<&str>,
    ) -> netdata_plugin_error::Result<(Vec<sfsq::traces::TraceSource>, TimeWindow)> {
        let now_s = unix_now_s();
        let window: ResolvedWindow = resolve_window(after, before, now_s, None)
            .map_err(|e| handler_err(format!("invalid otel-traces request: {e}")))?;
        let engine_window = TimeWindow::new(window.start_ns, window.end_ns)
            .map_err(|e| handler_err(format!("invalid otel-traces request: {e}")))?;

        let tenant = TenantId::resolve_query(tenant);
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
        params: &AttributesParams,
        tenant: Option<&str>,
    ) -> netdata_plugin_error::Result<OtelTracesResponse> {
        let client_err = |e: String| handler_err(format!("invalid otel-traces request: {e}"));
        let mut query = AttributeNamesQuery::new();
        if let Some(word) = params.owner.as_deref() {
            query = query.owner(parse_owner_word(word).map_err(client_err)?);
        }
        if let Some(max) = params.max_keys {
            // Pre-capture twin of the engine's own check.
            if max == 0 {
                return Err(client_err(
                    "a zero key/value limit would return nothing; omit the limit or raise it"
                        .into(),
                ));
            }
            query = query.max_keys(max);
        }
        let (sources, window) = self
            .enumeration_setup(ctx, params.after, params.before, tenant)
            .await?;
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
        params: &AttributeValuesParams,
        tenant: Option<&str>,
    ) -> netdata_plugin_error::Result<OtelTracesResponse> {
        let client_err = |e: String| handler_err(format!("invalid otel-traces request: {e}"));
        let (owner, key) = parse_enumeration_key(&params.key).map_err(client_err)?;
        let mut query = AttributeValuesQuery::new(owner, key);
        if let Some(max) = params.max_values {
            // Pre-capture twin of the engine's own check.
            if max == 0 {
                return Err(client_err(
                    "a zero key/value limit would return nothing; omit the limit or raise it"
                        .into(),
                ));
            }
            query = query.max_values(max);
        }
        let (sources, window) = self
            .enumeration_setup(ctx, params.after, params.before, tenant)
            .await?;
        query = query.window(window);

        let done = ctx.progress.done_counter();
        let cancel = ctx.cancellation.clone();
        let wire_key = params.key.clone();
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
    /// totals are their stored-row sums (labeled on the wire).
    async fn overview(
        &self,
        ctx: &FunctionCallContext,
        params: &OverviewParams,
        tenant: Option<&str>,
    ) -> netdata_plugin_error::Result<OtelTracesResponse> {
        let client_err = |e: String| handler_err(format!("invalid otel-traces request: {e}"));

        let now_s = unix_now_s();
        let window: ResolvedWindow =
            resolve_window(params.after, params.before, now_s, None).map_err(client_err)?;
        let (grid, aligned_after, aligned_before) =
            super::super::grid::grid_for_window_s(window.capture.start, window.capture.end);
        let query = OverviewQuery::new(grid).root_facets(params.facets.unwrap_or(false));

        // Alignment can widen the window; prune files by the widened one.
        let tenant = TenantId::resolve_query(tenant);
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

    /// The `slowest` mode: the window's duration-ranked top-K traces —
    /// the UI's explicit "Slowest" sort. Row numbers are stored-row
    /// sums; pre-rollup files are excluded under `rollup_absent`;
    /// no pagination by design.
    async fn slowest(
        &self,
        ctx: &FunctionCallContext,
        params: &SlowestParams,
        tenant: Option<&str>,
    ) -> netdata_plugin_error::Result<OtelTracesResponse> {
        let client_err = |e: String| handler_err(format!("invalid otel-traces request: {e}"));
        let limit = params.limit.unwrap_or(DEFAULT_SLOWEST_LIMIT);
        // Pre-capture twins of the engine's own checks: reject before
        // the registry read and chunk builds capture() performs (the
        // engine re-checks as defense in depth).
        if limit == 0 {
            return Err(client_err(
                "a zero limit would return nothing; slowest has no unbounded option".into(),
            ));
        }
        if limit > SLOWEST_LIMIT_MAX {
            return Err(client_err(format!(
                "limit {limit} exceeds the library maximum {SLOWEST_LIMIT_MAX}"
            )));
        }

        let (sources, window) = self
            .enumeration_setup(ctx, params.after, params.before, tenant)
            .await?;
        let query = SlowestQuery::new(window).limit(limit);

        let done = ctx.progress.done_counter();
        let cancel = ctx.cancellation.clone();
        match tokio::task::spawn_blocking(move || slowest(sources, query, cancel, done)).await {
            Ok(Ok(data)) => Ok(OtelTracesResponse::Slowest(Box::new(to_slowest_result(
                data, limit,
            )))),
            Ok(Err(SlowestRequestError::SourceSet(e))) => Err(handler_err(format!(
                "otel-traces internal error: captured source set is inconsistent: {e}"
            ))),
            Ok(Err(e)) => Err(client_err(e.to_string())),
            Err(e) => Err(handler_err(format!("otel-traces slowest task failed: {e}"))),
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
        let tenant = req.tenant.as_deref();
        match &req.mode {
            TracesMode::Info => Ok(OtelTracesResponse::Info(InfoResponse::default())),
            TracesMode::Trace(params) => self.trace(&ctx, params, tenant).await,
            TracesMode::Search(params) => self.search(&ctx, params, tenant).await,
            TracesMode::Attributes(params) => self.attributes(&ctx, params, tenant).await,
            TracesMode::AttributeValues(params) => {
                self.attribute_values(&ctx, params, tenant).await
            }
            TracesMode::Overview(params) => self.overview(&ctx, params, tenant).await,
            TracesMode::Slowest(params) => self.slowest(&ctx, params, tenant).await,
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
