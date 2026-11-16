//! `OtelTracesHandler` — the typed `FunctionHandler` for the
//! `otel-traces` Function.
//!
//! The full mode catalog lives here: the Functions view (the trace
//! list plus its trace-density grid), `info` (capability discovery),
//! `trace` (exact single-trace fetch), `suggest` (the search bar's
//! keys and values, and the span view's stack keys), and `explore`
//! (the span view). Mode selection and every request-SHAPE check happen at
//! deserialization (the wire's typed request — shape errors are
//! transport 400s); this handler owns only the semantic validation
//! (trace-id shape, zero limits, bounds).
//!
//! The wire contract lives in [`super::wire`], the engine mapping in
//! [`super::adapter`], source resolution in [`super::sources`], and
//! the shared time-grid derivation in [`super::super::grid`].
//!
//! Netdata-plugin glue only, like the logs handler: the engine
//! ([`sfsq::traces`]) stays wire-neutral; `otel-bridge`'s `HandlerAdapter`
//! owns the JSON round-trip, progress ticker, and cancellation.

use std::sync::Arc;

use async_trait::async_trait;
use file_registry::TenantId;
use netdata_plugin_protocol::FunctionDeclaration;
use netdata_plugin_types::HttpAccess;
use otel_bridge::function::{FunctionCallContext, FunctionHandler};
use tokio::sync::RwLock;

use file_lifecycle::chunk::ChunkCache;
use file_lifecycle::registry::TenantRegistries;

use sfsq::traces::{
    FieldKinds, OverviewQuery, OverviewRequestError, PartialReason, QueryStatus, SearchData,
    SearchQuery, SearchRequestError, SearchSources, SuggestQuery, SuggestRequestError, TimeWindow,
    TraceData, TraceQuery, TraceRequestError, overview, search, trace_by_id,
};

use super::adapter::{
    ShapedResources, WireShaper, build_tokens, check_request_text, completion_capture_range,
    explore_request, heatmap_tokens, parse_cursor, parse_explore_anchor, parse_trace_id,
    parse_values_key, resolve_window, to_explore_result, to_overview_section, to_search_result,
    to_suggest_result, to_trace_result, validate_trace_bounds,
};
use super::sources::{Capture, CaptureError, RemotePolicy, TracesSourceSupplier};
use super::wire::{
    CoverageWire, ExploreParams, FunctionsParams, FunctionsTracesResponse, InfoResponse,
    OtelTracesRequest, OtelTracesResponse, SUGGEST_ITEMS_MAX, SearchParams, SearchResult,
    SuggestParams, TraceParams, TracesMode, resolve_relative_window,
};
use crate::ledger::wal_prefix::WalPrefixes;
use file_lifecycle::remote_read::RemoteRead;

/// Shorthand for a failure on the agent's side (HTTP 500).
fn handler_err(message: String) -> netdata_plugin_error::NetdataPluginError {
    netdata_plugin_error::NetdataPluginError::FunctionHandler { message }
}

/// A request the handler rejects (HTTP 400): resending it unchanged fails
/// again.
fn client_err(e: impl std::fmt::Display) -> netdata_plugin_error::NetdataPluginError {
    netdata_plugin_error::NetdataPluginError::BadRequest {
        message: format!("invalid otel-traces request: {e}"),
    }
}

/// An engine request error after the handler's own checks passed: the
/// two disagree, so it is the agent's failure (HTTP 500), not the
/// caller's.
fn engine_rejected(e: impl std::fmt::Display) -> netdata_plugin_error::NetdataPluginError {
    handler_err(format!(
        "otel-traces internal error: the engine rejected a request the handler accepted: {e}"
    ))
}

/// A capture that failed as a whole: a hard error the caller can act on.
/// Too much remote data is the request's (a narrower window fits).
fn capture_error(e: CaptureError) -> netdata_plugin_error::NetdataPluginError {
    let size = |bytes: u64| bytesize::ByteSize::b(bytes).display().si().to_string();
    match e {
        CaptureError::TooLarge { at_least, capacity } => {
            // A 400 is logged at `info` by the bridge; the cache size is
            // the operator's to raise, so say it where they look.
            tracing::warn!(
                "otel-traces: a query needs at least {} of remote trace data, more than the {} \
                 download cache (`remote_storage.read_cache_max_size`)",
                size(at_least),
                size(capacity)
            );
            netdata_plugin_error::NetdataPluginError::BadRequest {
                message: format!(
                    "this query needs more than {} of remote trace data (at least {}), more \
                     than the download cache holds; narrow the time range or raise \
                     `remote_storage.read_cache_max_size`",
                    size(capacity),
                    size(at_least)
                ),
            }
        }
        CaptureError::EvictionFailed => handler_err(
            "the remote-read download cache directory is unwritable (eviction failed); check \
             its permissions and free space"
                .to_string(),
        ),
        CaptureError::Planning(e) => {
            handler_err(format!("otel-traces remote planning task failed: {e}"))
        }
    }
}

/// A WAL the capture refused ([`Capture::refused`]) leaves its spans out
/// of the answer: a failed source, never a silent gap.
fn reported(refused: bool, status: QueryStatus) -> QueryStatus {
    if refused {
        status.with(PartialReason::SourceFailure)
    } else {
        status
    }
}

/// How many suggestion requests may run at once (the keystroke class).
/// They arrive per keystroke (the span view's tier list once per
/// window); beyond this a request answers `busy` at once instead of
/// queueing CPU and memory behind the others.
const SUGGEST_CONCURRENCY: usize = 2;

/// How many list requests (the Functions view: search page plus its
/// heatmap, under one permit) may run at once. A list over a long window
/// is the costliest request; beyond this one answers `busy` at once, so a
/// burst of them cannot stack CPU and memory. A pool of its own, so a
/// slow list never makes a keystroke or a trace click `busy`.
const LIST_CONCURRENCY: usize = 2;

/// How many trace lookups may run at once (a pool of its own, as for
/// lists).
const TRACE_CONCURRENCY: usize = 2;
/// How many span-view requests may run at once (a pool of its own, as
/// for lists): the span view polls, so a slow window answers `busy`
/// instead of stacking up behind itself.
const EXPLORE_CONCURRENCY: usize = 2;

/// How long a suggestion request may walk sources, from its arrival:
/// suggestions answer per keystroke, so a partial answer now beats a
/// complete one later. A tier list shares it: its keys come from file
/// metadata, which a walk reads fast. The deadline is checked between
/// sources (a key walk reads each visited source whole), and the newest
/// source is always visited.
const SUGGEST_DEADLINE: std::time::Duration = std::time::Duration::from_millis(250);

/// The engine's span cap for one trace lookup: the item bound beside the
/// byte budget the wire shaper enforces (`TRACE_SPANS_MAX_BYTES`, about
/// two thousand spans of a typical multi-service trace, whose resources
/// go out once each), for traces of tiny spans. The engine's own
/// default (65,536) is a runaway-merge guard, not a response size.
const TRACE_SPAN_CAP: usize = 10_000;

pub(crate) struct OtelTracesHandler {
    /// Live source resolution (registries snapshot + WAL chunk indexing).
    supplier: TracesSourceSupplier,
    /// Admission for suggestions (see [`SUGGEST_CONCURRENCY`]).
    suggestions: Arc<tokio::sync::Semaphore>,
    /// Admission for the Functions view (see [`LIST_CONCURRENCY`]).
    lists: Arc<tokio::sync::Semaphore>,
    /// Admission for trace lookups (see [`TRACE_CONCURRENCY`]).
    traces: Arc<tokio::sync::Semaphore>,
    /// Admission for the span view (see [`EXPLORE_CONCURRENCY`]).
    explores: Arc<tokio::sync::Semaphore>,
}

impl OtelTracesHandler {
    /// `remote` reads files that local retention evicted back from remote
    /// storage; `None` when remote storage is disabled.
    pub(crate) fn new(
        registries: Arc<RwLock<TenantRegistries>>,
        chunk_cache: Arc<ChunkCache>,
        prefixes: Arc<WalPrefixes>,
        remote: Option<RemoteRead>,
    ) -> Self {
        Self {
            supplier: TracesSourceSupplier::new(registries, chunk_cache, prefixes, remote),
            suggestions: Arc::new(tokio::sync::Semaphore::new(SUGGEST_CONCURRENCY)),
            lists: Arc::new(tokio::sync::Semaphore::new(LIST_CONCURRENCY)),
            traces: Arc::new(tokio::sync::Semaphore::new(TRACE_CONCURRENCY)),
            explores: Arc::new(tokio::sync::Semaphore::new(EXPLORE_CONCURRENCY)),
        }
    }

    /// Test-only admission override: `0` makes every suggestion request
    /// answer `busy`.
    #[cfg(test)]
    pub(crate) fn with_suggest_concurrency(mut self, permits: usize) -> Self {
        self.suggestions = Arc::new(tokio::sync::Semaphore::new(permits));
        self
    }

    /// Test-only admission override for the span view.
    #[cfg(test)]
    pub(crate) fn with_explore_concurrency(mut self, permits: usize) -> Self {
        self.explores = Arc::new(tokio::sync::Semaphore::new(permits));
        self
    }

    /// Test-only admission override for lists and trace lookups.
    #[cfg(test)]
    pub(crate) fn with_list_and_trace_concurrency(mut self, lists: usize, traces: usize) -> Self {
        self.lists = Arc::new(tokio::sync::Semaphore::new(lists));
        self.traces = Arc::new(tokio::sync::Semaphore::new(traces));
        self
    }

    /// The `trace` mode: exact single-trace fetch via the engine's
    /// cross-source `trace_by_id`.
    ///
    /// Ignores the envelope window; assembly bounds live in the `trace`
    /// sub-object. Absent bounds capture the FULL local range; offloaded
    /// files are skipped, never downloaded (each reports
    /// `remote_unavailable`), so a lookup without bounds stays cheap
    /// however much retention is offloaded. Present bounds also download
    /// the offloaded files they select (too-large history fails via the
    /// download cache rather than truncate) — a trace is one object whose
    /// spans straddle files. Present bounds prune file-granularly (a file
    /// overlapping the bounds is probed whole). Either way the response DECLARES
    /// the range used (`coverage`) — spans beyond it are unknown, never
    /// silently dropped. An id absent from the sources is a Complete
    /// empty trace, not an error.
    async fn trace(
        &self,
        ctx: &FunctionCallContext,
        params: &TraceParams,
        tenant: Option<&str>,
    ) -> netdata_plugin_error::Result<OtelTracesResponse> {
        let trace_id = parse_trace_id(&params.id).map_err(client_err)?;
        // Pre-capture twin of the engine's UnsetTraceId check (the
        // parser deliberately lets the sentinel through).
        if trace_id.is_unset() {
            return Err(client_err(
                "the all-zero (unset) trace id cannot be looked up",
            ));
        }
        // The list's chips mark the matching spans; validated before
        // admission like the list's.
        let highlight = build_tokens(&params.conditions, &params.text).map_err(client_err)?;
        let shaper = WireShaper::default();
        let resources = shaper.resources();
        let query = TraceQuery::new(trace_id)
            .span_cap(TRACE_SPAN_CAP)
            .shaper(Box::new(shaper))
            .highlight(highlight);

        let bounds = validate_trace_bounds(params.after, params.before).map_err(client_err)?;
        let remote_policy = if bounds.is_some() {
            RemotePolicy::Download
        } else {
            RemotePolicy::Skip
        };
        let capture_range = bounds.unwrap_or(0..u32::MAX);
        let coverage = CoverageWire {
            after: capture_range.start,
            before: capture_range.end,
        };
        // Admission before capture (which may download); the permit
        // moves into the blocking task, held until the engine stops.
        let Ok(permit) = Arc::clone(&self.traces).try_acquire_owned() else {
            let busy = TraceData {
                trace: sfst::Trace {
                    spans: Vec::new(),
                    roots: Vec::new(),
                    children: Vec::new(),
                },
                status: QueryStatus::Partial([PartialReason::Busy].into()),
                field_kinds: FieldKinds::default(),
                span_cut: Vec::new(),
                matched: Vec::new(),
            };
            return Ok(OtelTracesResponse::Trace(Box::new(to_trace_result(
                &trace_id,
                busy,
                ShapedResources::default(),
                coverage,
            ))));
        };

        let tenant = TenantId::resolve_query(tenant);
        // A cancelled capture returns NO copies; the empty default flows
        // into the engine, which polls the same token up front and
        // reports the Cancelled partial — one consistent cancel path.
        // The capture sets the progress total; the engine ticks the done
        // counter once per source and the bridge's ticker renders it.
        let Capture {
            mut sets,
            pins,
            refused,
        } = self
            .supplier
            .capture(
                &tenant,
                capture_range,
                1,
                remote_policy,
                &ctx.cancellation,
                &ctx.progress,
            )
            .await
            .map_err(capture_error)?;
        let sources = sets.pop().unwrap_or_default();
        let refused = refused.first().copied().unwrap_or(false);
        let done = ctx.progress.done_counter();
        let cancel = ctx.cancellation.clone();

        // Sync engine (maps + decompresses files) — off the runtime
        // thread. Engine request errors are internal: the handler checked
        // each one first ([`engine_rejected`]). A rejected SOURCE SET is
        // the supplier's inconsistency (duplicate ids / overlapping WAL
        // coverage), framed as internal so a debugger looks at the right
        // side. A panicked task is a handler failure.
        let mut data = match tokio::task::spawn_blocking(move || {
            let _permit = permit;
            let _pins = pins;
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
            Ok(Err(
                e @ (TraceRequestError::UnsetTraceId
                | TraceRequestError::ZeroSpanCap
                | TraceRequestError::Highlight(_)),
            )) => return Err(engine_rejected(e)),
            Err(e) => {
                return Err(handler_err(format!("otel-traces trace task failed: {e}")));
            }
        };

        data.status = reported(refused, data.status);
        Ok(OtelTracesResponse::Trace(Box::new(to_trace_result(
            &trace_id,
            data,
            resources.take(),
            coverage,
        ))))
    }

    /// The engine's bounded most-recent-first search over the request's
    /// (canonicalized) window, with wire-level tie-safe pagination —
    /// see the adapter's cursor docs. The Functions view's page core
    /// (it layers the aggregate on through `aggregate`).
    ///
    /// `aggregate` adds the Functions view's full-window section: a
    /// SECOND engine pass in this same call, off the same capture, over
    /// the page's tokens minus the trace-duration and trace-start ones
    /// (the grid's own axes; `heatmap_tokens`). It cannot ride along with the
    /// first — search is top-K with early termination and hard-caps its
    /// assembly far below the window's population, so an accumulator on
    /// its scan loop would describe the few hundred traces the ranker
    /// happened to touch, not the window. The Functions view passes
    /// `false` on an ANCHOR page (the anchor gate in `functions`).
    async fn search_result(
        &self,
        ctx: &FunctionCallContext,
        params: &SearchParams,
        tenant: Option<&str>,
        aggregate: bool,
    ) -> netdata_plugin_error::Result<SearchResult> {
        // Zero must be rejected BEFORE the anchor allowance is added —
        // `limit=0` with a cursor would otherwise sneak a positive engine
        // limit past the engine's own ZeroLimit check.
        if params.limit == 0 {
            return Err(client_err(
                "a zero 'last' would return nothing; the list has no unbounded option",
            ));
        }
        // The wire cap on the CLIENT half of the limit; the engine's
        // effective limit (served + limit on cursor walks) is bounded by
        // construction once this side is capped (see SEARCH_LIMIT_MAX).
        if params.limit > super::wire::SEARCH_LIMIT_MAX {
            return Err(client_err(format!(
                "'last' {} exceeds the maximum {}",
                params.limit,
                super::wire::SEARCH_LIMIT_MAX
            )));
        }
        let cursor = params
            .anchor
            .as_deref()
            .map(parse_cursor)
            .transpose()
            .map_err(client_err)?;
        let tokens = build_tokens(&params.conditions, &params.text).map_err(client_err)?;
        let aggregate_tokens = aggregate.then(|| heatmap_tokens(&tokens));

        let now_s = unix_now_s();
        // An anchor page reruns the SAME query over the cursor's frozen
        // window (never narrowed — the rank is window-dependent) with an
        // over-fetch covering the served prefix; the adapter drops it.
        let window = resolve_window(params.after, params.before, now_s, cursor.as_ref())
            .map_err(client_err)?;

        // The list attaches no matched spans to its rows.
        let query = SearchQuery::new(tokens)
            .window(TimeWindow::new(window.start_ns, window.end_ns).map_err(client_err)?)
            .limit(params.limit.saturating_add(cursor.map_or(0, |c| c.served)));

        // ONE capture over the COMPLETION range (the match window
        // widened by the clamped slack), feeding two roles: the engine
        // narrows the window role internally, so identical copies keep
        // window ⊆ completion by construction while slack-only files
        // still complete straddling hits. The cursor keeps freezing the
        // ORIGINAL window (window.capture) — the completion range
        // re-derives from it deterministically on every page.
        let completion_range = completion_capture_range(&window.capture);
        // The aggregate's geometry and window: the grid's, snapped
        // outward to wall-clock bucket multiples — NOT the page's
        // slack-widened range. Its sources are captured by that window
        // alone.
        let aggregate_grid = aggregate.then(|| {
            super::super::grid::grid_for_window_s(window.capture.start, window.capture.end)
        });

        // Admission before capture (which may download); one permit for
        // the page and its heatmap, moved into their blocking task.
        let Ok(permit) = Arc::clone(&self.lists).try_acquire_owned() else {
            let busy = SearchData {
                traces: Vec::new(),
                status: QueryStatus::Partial([PartialReason::Busy].into()),
            };
            return Ok(to_search_result(
                busy,
                params.limit,
                cursor.as_ref(),
                (window.capture.start, window.capture.end),
            ));
        };

        let tenant = TenantId::resolve_query(tenant);
        // ONE capture for every pass: the two search roles over the
        // completion range, the aggregate over the grid's. One snapshot
        // means one `valid_up_to`, so the page and the section can never
        // describe two different corpora.
        let mut ranges = vec![completion_range.clone(), completion_range];
        if let Some((_, aligned_after, aligned_before)) = aggregate_grid {
            ranges.push(aligned_after..aligned_before);
        }
        // The capture sets the progress total: the completion range's
        // sources plus the aggregate's (the window role shares the
        // completion range and does not tick).
        let Capture {
            mut sets,
            pins,
            refused,
        } = self
            .supplier
            .capture_ranges(
                &tenant,
                &ranges,
                RemotePolicy::Download,
                &ctx.cancellation,
                &ctx.progress,
            )
            .await
            .map_err(capture_error)?;
        // The pops unwind `ranges` (aggregate was pushed last).
        let aggregate_sources = match aggregate_grid {
            Some(_) => sets.pop().unwrap_or_default(),
            None => Vec::new(),
        };
        let completion = sets.pop().unwrap_or_default();
        let window_sources = sets.pop().unwrap_or_default();
        let done = ctx.progress.done_counter();
        let cancel = ctx.cancellation.clone();
        let aggregate_query = aggregate_grid
            .zip(aggregate_tokens)
            .map(|((grid, ..), tokens)| OverviewQuery::new(grid).tokens(tokens));

        // Both engine calls are pure-sync and expect to run off the
        // runtime thread, so one blocking task covers the pair — and a
        // cancellation between them cannot pair mismatched snapshots.
        let joined = tokio::task::spawn_blocking(move || {
            let _permit = permit;
            let _pins = pins;
            let page = search(
                SearchSources {
                    window: window_sources,
                    completion,
                },
                query,
                cancel.clone(),
                Arc::clone(&done),
            );
            let section = aggregate_query.map(|q| overview(aggregate_sources, q, cancel, done));
            (page, section)
        })
        .await;
        let (page, section) = match joined {
            Ok(both) => both,
            Err(e) => {
                return Err(handler_err(format!("otel-traces search task failed: {e}")));
            }
        };

        let mut data = match page {
            Ok(data) => data,
            // These two are structurally impossible from one capture —
            // an occurrence means the supplier broke its contract.
            Err(e @ SearchRequestError::SourceSet(_))
            | Err(e @ SearchRequestError::WindowNotInCompletion(_)) => {
                return Err(handler_err(format!(
                    "otel-traces internal error: captured source set is inconsistent: {e}"
                )));
            }
            Err(
                e @ (SearchRequestError::ZeroLimit
                | SearchRequestError::Predicate(_)
                | SearchRequestError::Window(_)),
            ) => return Err(engine_rejected(e)),
        };

        // Set 0 is the window role, set 2 the aggregate's (`ranges`).
        data.status = reported(refused.first().copied().unwrap_or(false), data.status);
        let mut result: SearchResult = to_search_result(
            data,
            params.limit,
            cursor.as_ref(),
            (window.capture.start, window.capture.end),
        );
        if let (Some(section), Some((grid, aligned_after, aligned_before))) =
            (section, aggregate_grid)
        {
            let mut section = match section {
                Ok(section) => section,
                Err(OverviewRequestError::SourceSet(e)) => {
                    return Err(handler_err(format!(
                        "otel-traces internal error: captured source set is inconsistent: {e}"
                    )));
                }
                // The handler builds the grid and drops the axis chips,
                // and the list pass already accepted the same chips.
                Err(e) => {
                    return Err(handler_err(format!(
                        "otel-traces internal error: the window aggregate request is invalid: {e}"
                    )));
                }
            };
            section.status = reported(refused.get(2).copied().unwrap_or(false), section.status);
            result.overview = Some(to_overview_section(
                section,
                grid,
                CoverageWire {
                    after: aligned_after,
                    before: aligned_before,
                },
            ));
        }
        Ok(result)
    }

    /// The Functions view: the search page plus the full-window
    /// aggregate, in one request. Nothing else composes both — and only
    /// a FIRST page does, since an anchor page's aggregate would repeat
    /// the first page's (the anchor gate below).
    async fn functions(
        &self,
        ctx: &FunctionCallContext,
        params: &FunctionsParams,
        tenant: Option<&str>,
    ) -> netdata_plugin_error::Result<OtelTracesResponse> {
        let search_params = params.search_params(unix_now_s()).map_err(client_err)?;
        // The anchor gate. An anchor page reruns the same query over the
        // cursor's FROZEN window, so the section it would compose is the
        // first page's, byte for byte — only the rows advance. It is
        // skipped and the section simply ABSENT (never a `null` on the
        // wire); consumers re-read it on the next anchor-less request,
        // the only kind that can move the window. Details on the wire
        // (`SearchResult::overview`). A caller that keeps drawing an
        // earlier section (a heatmap cell's list) turns it off.
        let aggregate = !params.skip_overview && params.anchor.is_none();
        let data = self
            .search_result(ctx, &search_params, tenant, aggregate)
            .await?;
        Ok(OtelTracesResponse::Functions(Box::new(
            FunctionsTracesResponse::new(data),
        )))
    }

    /// The `suggest` mode: keys, or one key's values, matching a typed
    /// term (see `sfsq::traces::suggest`), bounded in sources and time,
    /// and in items unless it is a tier list.
    async fn suggest(
        &self,
        ctx: &FunctionCallContext,
        params: &SuggestParams,
        tenant: Option<&str>,
    ) -> netdata_plugin_error::Result<OtelTracesResponse> {
        let deadline = std::time::Instant::now() + SUGGEST_DEADLINE;
        let max = match (params.tier, params.max_items) {
            (Some(_), Some(_)) => {
                return Err(client_err("a 'tier' list is never cut; omit 'max_items'"));
            }
            (Some(_), None) => None,
            (None, max) => Some(max.unwrap_or(sfsq::traces::DEFAULT_SUGGEST_ITEMS)),
        };
        if let Some(max) = max {
            if max == 0 {
                return Err(client_err(
                    "a zero 'max_items' would return nothing; omit it or raise it",
                ));
            }
            if max > SUGGEST_ITEMS_MAX {
                return Err(client_err(format!(
                    "'max_items' {max} exceeds the maximum {SUGGEST_ITEMS_MAX}"
                )));
            }
        }
        check_request_text("term", &params.term).map_err(client_err)?;
        let query = match (&params.key, params.tier) {
            (Some(_), Some(_)) => {
                return Err(client_err("'tier' lists keys; omit 'key' or 'tier'"));
            }
            (Some(key), None) => {
                let (owner, key) = parse_values_key(key).map_err(client_err)?;
                SuggestQuery::values(owner, key)
            }
            (None, Some(tier)) => SuggestQuery::keys_of_tier(tier.into()),
            (None, None) => SuggestQuery::keys(),
        };
        // Validated before admission: a bad request is a client error,
        // not `busy`.
        let window =
            resolve_window(params.after, params.before, unix_now_s(), None).map_err(&client_err)?;
        let engine_window = TimeWindow::new(window.start_ns, window.end_ns).map_err(client_err)?;
        let Ok(permit) = Arc::clone(&self.suggestions).try_acquire_owned() else {
            return Ok(OtelTracesResponse::Suggest(to_suggest_result(
                sfsq::traces::SuggestData {
                    items: Vec::new(),
                    truncated: false,
                    status: QueryStatus::Partial([PartialReason::Busy].into()),
                    complete_after: None,
                },
                params.key.clone(),
            )));
        };
        let mut query = query
            .term(params.term.clone())
            .window(engine_window)
            .deadline(deadline);
        if let Some(max) = max {
            query = query.max_items(max);
        }
        let Capture {
            mut sets,
            pins,
            refused,
        } = self
            .supplier
            .capture(
                &TenantId::resolve_query(tenant),
                window.capture.clone(),
                1,
                // Suggestions answer per keystroke: offloaded files are
                // reported, never downloaded.
                RemotePolicy::Skip,
                &ctx.cancellation,
                &ctx.progress,
            )
            .await
            .map_err(capture_error)?;
        let sources = sets.pop().unwrap_or_default();

        let done = ctx.progress.done_counter();
        let cancel = ctx.cancellation.clone();
        let key = params.key.clone();
        match tokio::task::spawn_blocking(move || {
            let _permit = permit;
            let _pins = pins;
            sfsq::traces::suggest(sources, query, cancel, done)
        })
        .await
        {
            Ok(Ok(mut data)) => {
                data.status = reported(refused.first().copied().unwrap_or(false), data.status);
                Ok(OtelTracesResponse::Suggest(to_suggest_result(data, key)))
            }
            Ok(Err(e)) => Err(map_suggest_error(e)),
            Err(e) => Err(handler_err(format!("otel-traces suggest task failed: {e}"))),
        }
    }

    /// The `explore` mode: the span view's histogram and rows over the
    /// window aligned to its grid (see `sfsq::traces::explore`). An
    /// anchored page answers over the anchor's window. Offloaded files
    /// download as for the list, so the same window shows the same data
    /// in both views.
    async fn explore(
        &self,
        ctx: &FunctionCallContext,
        params: &ExploreParams,
        tenant: Option<&str>,
    ) -> netdata_plugin_error::Result<OtelTracesResponse> {
        let anchor = params
            .anchor
            .as_deref()
            .map(parse_explore_anchor)
            .transpose()
            .map_err(client_err)?;
        // The window as asked, before alignment: an anchored page re-derives
        // its grid from the same pair, so every page of a walk answers over
        // the first page's window and grid.
        let asked = match &anchor {
            Some(a) => (a.after_s, a.before_s),
            None => {
                let now_s = unix_now_s();
                let (after, before) = resolve_relative_window(params.after, params.before, now_s)
                    .map_err(client_err)?;
                let window = resolve_window(after, before, now_s, None).map_err(client_err)?;
                (window.capture.start, window.capture.end)
            }
        };
        let (grid, after, before) = super::super::grid::grid_for_window_s(asked.0, asked.1);
        // Validated before admission: a bad request is a client error,
        // not `busy`.
        let request = explore_request(params, grid, anchor.map(|a| a.key)).map_err(client_err)?;
        let Ok(permit) = Arc::clone(&self.explores).try_acquire_owned() else {
            let busy = sfsq::traces::ExploreData {
                status: QueryStatus::Partial([PartialReason::Busy].into()),
                histogram: None,
                rows: None,
            };
            return Ok(OtelTracesResponse::Explore(Box::new(to_explore_result(
                busy,
                &request,
                grid,
                (after, before),
                asked,
            ))));
        };
        let Capture {
            mut sets,
            pins,
            refused,
        } = self
            .supplier
            .capture(
                &TenantId::resolve_query(tenant),
                after..before,
                1,
                RemotePolicy::Download,
                &ctx.cancellation,
                &ctx.progress,
            )
            .await
            .map_err(capture_error)?;
        let sources = sets.pop().unwrap_or_default();
        let done = ctx.progress.done_counter();
        let cancel = ctx.cancellation.clone();
        let query = request.query.clone();
        let mut data = match tokio::task::spawn_blocking(move || {
            let _permit = permit;
            let _pins = pins;
            sfsq::traces::explore(sources, query, cancel, done)
        })
        .await
        {
            Ok(Ok(data)) => data,
            Ok(Err(sfsq::traces::ExploreRequestError::SourceSet(e))) => {
                return Err(handler_err(format!(
                    "otel-traces internal error: captured source set is inconsistent: {e}"
                )));
            }
            Ok(Err(e)) => return Err(engine_rejected(e)),
            Err(e) => {
                return Err(handler_err(format!("otel-traces explore task failed: {e}")));
            }
        };
        data.status = reported(refused.first().copied().unwrap_or(false), data.status);
        Ok(OtelTracesResponse::Explore(Box::new(to_explore_result(
            data,
            &request,
            grid,
            (after, before),
            asked,
        ))))
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

/// Suggestion request errors are all the agent's: a rejected source set
/// is the supplier's inconsistency, and the rest are checked by the
/// handler first (`parse_values_key`, the limit and window checks).
fn map_suggest_error(e: SuggestRequestError) -> netdata_plugin_error::NetdataPluginError {
    match e {
        SuggestRequestError::SourceSet(e) => handler_err(format!(
            "otel-traces internal error: captured source set is inconsistent: {e}"
        )),
        e @ (SuggestRequestError::ZeroLimit
        | SuggestRequestError::Window(_)
        | SuggestRequestError::NotEnumerable(_)
        | SuggestRequestError::BuiltinKeyOutsideBuiltinOwner(_)
        | SuggestRequestError::AttributeKeyUnderBuiltinOwner(_)) => engine_rejected(e),
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
            TracesMode::Functions(params) => self.functions(&ctx, params, tenant).await,
            TracesMode::Info => Ok(OtelTracesResponse::Info(InfoResponse::default())),
            TracesMode::Trace(params) => self.trace(&ctx, params, tenant).await,
            TracesMode::Suggest(params) => self.suggest(&ctx, params, tenant).await,
            TracesMode::Explore(params) => self.explore(&ctx, params, tenant).await,
        }
    }

    /// Advertise the function to the agent/Cloud: `global` (not scoped
    /// to one node), tagged `traces`, accessible to signed-in users of
    /// the same space on a commercial plan (traces are a paid feature),
    /// with the SENSITIVE_DATA flag since trace payloads may carry
    /// sensitive content.
    fn declaration(&self) -> FunctionDeclaration {
        let mut d = FunctionDeclaration::new("otel-traces", "Query OpenTelemetry traces");
        d.global = true;
        d.tags = Some("traces".to_string());
        d.access = Some(
            HttpAccess::SIGNED_ID
                | HttpAccess::SAME_SPACE
                | HttpAccess::COMMERCIAL_SPACE
                | HttpAccess::SENSITIVE_DATA,
        );
        d
    }
}

#[cfg(test)]
mod tests;

#[cfg(test)]
mod remote_tests;
