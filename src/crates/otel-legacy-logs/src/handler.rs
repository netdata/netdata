//! Read-only `legacy-otel-logs` function handler.
//!
//! Ported from the former `otel-signal-viewer-plugin` catalog handler. It
//! serves the systemd-journal facets protocol over the journal files written
//! by the former otel plugin, driving the restored `journal-function` query
//! stack. It never writes to the journal directory.

use std::collections::HashMap;
use std::sync::Arc;
use std::sync::atomic::AtomicUsize;

use async_trait::async_trait;
use bridge::config::LegacyLogsConfig;
use bridge::function::{FunctionCallContext, FunctionHandler};
use netdata_plugin_error::Result;
use netdata_plugin_protocol::FunctionDeclaration;
use netdata_plugin_schema::HttpAccess;
use tokio_util::sync::CancellationToken;
use tracing::{error, instrument, trace, warn};

use journal_function::{
    Facets, FileIndexCache, FileIndexCacheBuilder, FileIndexKey, HistogramEngine, IndexingLimits,
    Monitor, Registry, netdata,
};
use journal_index::Filter;
use journal_index::{FieldName, FieldValuePair, Microseconds, Seconds};

/// Request parameters for the function (systemd-journal request structure).
type LegacyLogsRequestParams = netdata::JournalRequest;

/// Response from the function (systemd-journal response structure).
type LegacyLogsResponseBody = netdata::JournalResponse;

/// Translate the legacy GET-style args (`info`, `after:N`, `before:M`) into a
/// JSON request payload, matching the new ledger's shim. The Netdata logs UI
/// issues these functions as GET with space-separated args; the bridge handler
/// engine deserializes the request from the payload only.
pub(crate) fn patch_args_into_payload(args: &[String], payload: Option<&[u8]>) -> Option<Vec<u8>> {
    if args.is_empty() || payload.is_some() {
        return None;
    }

    let info = args.iter().any(|a| a == "info");
    let mut map = serde_json::Map::new();
    map.insert("info".into(), serde_json::json!(info));

    for arg in args {
        if let Some(rest) = arg.strip_prefix("after:") {
            if let Ok(v) = rest.parse::<u64>() {
                map.insert("after".into(), serde_json::json!(v));
            }
        } else if let Some(rest) = arg.strip_prefix("before:") {
            if let Ok(v) = rest.parse::<u64>() {
                map.insert("before".into(), serde_json::json!(v));
            }
        }
    }

    serde_json::to_vec(&serde_json::Value::Object(map)).ok()
}

/// Builds a Filter from the selections HashMap.
#[instrument(skip(selections))]
fn build_filter_from_selections(selections: &HashMap<String, Vec<String>>) -> Filter {
    if selections.is_empty() {
        return Filter::none();
    }

    let mut field_filters = Vec::new();

    for (field, values) in selections {
        if values.is_empty() {
            continue;
        }

        let value_filters: Vec<_> = values
            .iter()
            .filter_map(|value| {
                let pair_str = format!("{}={}", field, value);
                FieldValuePair::parse(&pair_str).map(Filter::match_field_value_pair)
            })
            .collect();

        if value_filters.is_empty() {
            warn!("All values failed to parse for field '{}'", field);
            continue;
        }

        field_filters.push(Filter::or(value_filters));
    }

    if field_filters.is_empty() {
        Filter::none()
    } else {
        Filter::and(field_filters)
    }
}

fn accepted_params() -> Vec<netdata::RequestParam> {
    use netdata::RequestParam;

    // Advertise only what this viewer deserializes and honors (mirrors
    // otel-ledger's ACCEPTED_PARAMS). `DataOnly` is deliberately omitted: the
    // cloud UI computes `dataOnly = data_only && accepted_params.includes("data_only")`,
    // so advertising it would make the UI preserve stale columns/facets/pagination
    // instead of refreshing from each full response — and this viewer recomputes
    // everything per call. `IfModifiedSince`/`Delta`/`Tail`/`Sampling` drive
    // incremental/live-tail/sampling modes that are not implemented here.
    vec![
        RequestParam::Info,
        RequestParam::After,
        RequestParam::Before,
        RequestParam::Anchor,
        RequestParam::Direction,
        RequestParam::Last,
        RequestParam::Query,
        RequestParam::Facets,
        RequestParam::Histogram,
        RequestParam::Slice,
    ]
}

fn required_params() -> Vec<netdata::RequiredParam> {
    Vec::new()
}

struct LegacyLogsHandlerInner {
    registry: Registry,
    cache: FileIndexCache,
    histogram_engine: Arc<HistogramEngine>,
    indexing_limits: IndexingLimits,
}

/// Read-only handler that serves the former otel plugin's journal logs.
#[derive(Clone)]
pub struct LegacyLogsHandler {
    inner: Arc<LegacyLogsHandlerInner>,
}

impl LegacyLogsHandler {
    /// Create the handler, discover existing journal files in `journal_dir`,
    /// and watch the directory for changes. The viewer is read-only.
    pub async fn new(config: &LegacyLogsConfig) -> anyhow::Result<Self> {
        let (monitor, mut notify_rx) = Monitor::new()?;
        let registry = Registry::new(monitor);

        let cache = FileIndexCacheBuilder::new()
            .with_cache_path(&config.cache_dir)
            .with_memory_capacity(config.memory_capacity)
            .with_disk_capacity(config.disk_capacity.as_u64() as usize)
            .with_block_size(4 * 1024 * 1024)
            .build()
            .await?;

        let indexing_limits = IndexingLimits {
            max_unique_values_per_field: config.max_unique_values_per_field,
            max_field_payload_size: config.max_field_payload_size,
        };

        let inner = Arc::new(LegacyLogsHandlerInner {
            registry,
            cache,
            histogram_engine: Arc::new(HistogramEngine::new()),
            indexing_limits,
        });

        let handler = Self { inner };

        // Initial scan discovers existing files (recursively, including the
        // per-machine-id subdirectory); also watches for later changes. A watch
        // failure (e.g. permissions) is non-fatal: log and continue serving
        // whatever was discovered rather than taking down the otel-plugin.
        let journal_dir = config.journal_dir.to_string_lossy().into_owned();
        if let Err(e) = handler.inner.registry.watch_directory(&journal_dir) {
            tracing::warn!("failed to watch journal directory {journal_dir}: {e:#}");
        }

        // Forward filesystem events so a still-live directory stays current.
        // For a stopped former agent the directory is static and this idles.
        // Hold a Weak ref: the watcher's event sender lives inside `inner`, so a
        // strong clone here would keep it alive, the channel would never close,
        // and this task would never exit (a self-sustaining cycle). With Weak,
        // once the handler is dropped the sender closes and `recv` returns None.
        let forwarder = Arc::downgrade(&handler.inner);
        tokio::spawn(async move {
            while let Some(event) = notify_rx.recv().await {
                let Some(inner) = forwarder.upgrade() else {
                    break;
                };
                if let Err(e) = inner.registry.process_event(event) {
                    error!("failed to process notify event: {}", e);
                }
            }
        });

        Ok(handler)
    }

    /// Query log entries from pre-indexed files.
    ///
    /// Returns: (entries, has_before, has_after).
    #[allow(clippy::too_many_arguments)]
    fn query_logs_from_indexes(
        indexed_files: &[journal_index::FileIndex],
        time_range: &journal_function::QueryTimeRange,
        anchor: Option<u64>,
        filter: &Filter,
        search_query: &str,
        limit: usize,
        direction: journal_index::Direction,
        cancellation: Option<CancellationToken>,
        progress: Option<Arc<AtomicUsize>>,
    ) -> (Vec<journal_function::LogEntryData>, bool, bool) {
        use journal_function::LogQuery;

        if indexed_files.is_empty() {
            return (Vec::new(), false, false);
        }

        let after_usec = time_range.aligned_start() as u64 * 1_000_000;
        let before_usec = time_range.aligned_end() as u64 * 1_000_000;

        let query_anchor = if let Some(anchor_usec) = anchor {
            match direction {
                journal_index::Direction::Forward => {
                    journal_index::Anchor::Timestamp(Microseconds(anchor_usec.saturating_add(1)))
                }
                journal_index::Direction::Backward => {
                    journal_index::Anchor::Timestamp(Microseconds(anchor_usec.saturating_sub(1)))
                }
            }
        } else {
            match direction {
                journal_index::Direction::Forward => {
                    journal_index::Anchor::Timestamp(Microseconds(after_usec))
                }
                journal_index::Direction::Backward => {
                    journal_index::Anchor::Timestamp(Microseconds(before_usec))
                }
            }
        };

        let mut query = LogQuery::new(indexed_files, query_anchor, direction)
            .with_limit(limit + 1)
            .with_after_usec(after_usec)
            .with_before_usec(before_usec);

        if let Some(ref t) = cancellation {
            query = query.with_cancellation(t.clone());
        }

        if let Some(counter) = progress {
            query = query.with_progress(counter);
        }

        if !filter.is_none() {
            query = query.with_filter(filter.clone());
        }

        if !search_query.is_empty() {
            query = query.with_regex(search_query);
        }

        let mut log_entries = match query.execute() {
            Ok(entries) => entries,
            Err(e) => {
                error!("log query execution failed: {}", e);
                if !search_query.is_empty() {
                    error!(
                        "query may have failed due to invalid regex pattern: {:?}",
                        search_query
                    );
                }
                return (Vec::new(), false, false);
            }
        };

        let has_more_in_query_direction = log_entries.len() > limit;
        if has_more_in_query_direction {
            log_entries.truncate(limit);
        }

        let has_more_in_opposite_direction = if let Some(anchor_usec) = anchor {
            let opposite_direction = match direction {
                journal_index::Direction::Forward => journal_index::Direction::Backward,
                journal_index::Direction::Backward => journal_index::Direction::Forward,
            };

            let opposite_anchor = match opposite_direction {
                journal_index::Direction::Forward => {
                    journal_index::Anchor::Timestamp(Microseconds(anchor_usec.saturating_add(1)))
                }
                journal_index::Direction::Backward => {
                    journal_index::Anchor::Timestamp(Microseconds(anchor_usec.saturating_sub(1)))
                }
            };

            let mut opposite_query =
                LogQuery::new(indexed_files, opposite_anchor, opposite_direction)
                    .with_limit(1)
                    .with_after_usec(after_usec)
                    .with_before_usec(before_usec);

            if let Some(ref t) = cancellation {
                opposite_query = opposite_query.with_cancellation(t.clone());
            }

            if !filter.is_none() {
                opposite_query = opposite_query.with_filter(filter.clone());
            }

            if !search_query.is_empty() {
                trace!("applying regex filter to opposite direction query");
                opposite_query = opposite_query.with_regex(search_query);
            }

            match opposite_query.execute() {
                Ok(entries) => !entries.is_empty(),
                Err(e) => {
                    warn!("opposite direction query error: {}", e);
                    false
                }
            }
        } else {
            false
        };

        let (has_before, has_after) = match direction {
            journal_index::Direction::Forward => {
                (has_more_in_opposite_direction, has_more_in_query_direction)
            }
            journal_index::Direction::Backward => {
                (has_more_in_query_direction, has_more_in_opposite_direction)
            }
        };

        // UI always expects logs sorted descending by timestamp (newest first).
        log_entries.sort_by_key(|e| std::cmp::Reverse(e.timestamp));

        (log_entries, has_before, has_after)
    }
}

#[async_trait]
impl FunctionHandler for LegacyLogsHandler {
    type Request = LegacyLogsRequestParams;
    type Response = LegacyLogsResponseBody;

    async fn on_call(
        &self,
        ctx: FunctionCallContext,
        request: Self::Request,
    ) -> Result<Self::Response> {
        let transaction = ctx.transaction().to_owned();
        let started = tokio::time::Instant::now();
        trace!("[{}] started transaction", transaction);

        // Capability/`info` probes may arrive without a usable time range
        // (e.g. tooling that discovers parameters before querying). The query
        // engine requires after < before, so substitute a minimal recent
        // window in that case; real queries always send a valid range.
        let (after, before) = if request.after < request.before {
            (request.after, request.before)
        } else {
            let now = std::time::SystemTime::now()
                .duration_since(std::time::UNIX_EPOCH)
                .map(|d| d.as_secs() as u32)
                .unwrap_or(0);
            (now.saturating_sub(1), now)
        };

        let time_range = journal_function::QueryTimeRange::new(after, before).map_err(|e| {
            netdata_plugin_error::NetdataPluginError::Other {
                message: format!("[{}] {}", transaction, e),
            }
        })?;

        let files = self
            .inner
            .registry
            .find_files_in_range(Seconds(after), Seconds(before))
            .map_err(|e| netdata_plugin_error::NetdataPluginError::Other {
                message: format!("[{}] failed to find files in range: {}", transaction, e),
            })?;
        trace!("[{}] found {} files in time range", transaction, files.len());

        let filter_expr = build_filter_from_selections(&request.selections);
        let facets = Facets::new(&request.facets);

        let source_timestamp_field = FieldName::new_unchecked("_SOURCE_REALTIME_TIMESTAMP");
        let keys: Vec<FileIndexKey> = files
            .iter()
            .map(|f| FileIndexKey::new(&f.file, &facets, Some(source_timestamp_field.clone())))
            .collect();

        // Progress: indexing phase first, then query phase.
        let num_files = keys.len();
        ctx.progress.set_total(num_files);

        let indexed_files = journal_function::batch_compute_file_indexes(
            &self.inner.cache,
            &self.inner.registry,
            keys,
            &time_range,
            ctx.cancellation.clone(),
            self.inner.indexing_limits,
            Some(ctx.progress.done_counter()),
        )
        .await
        .map_err(|e| netdata_plugin_error::NetdataPluginError::Other {
            message: format!("[{}] failed to index files: {}", transaction, e),
        })?;

        ctx.progress.set_total(2 * num_files);

        let histogram = self
            .inner
            .histogram_engine
            .compute_from_indexes(&indexed_files, &time_range, &request.facets, &filter_expr)
            .map_err(|e| netdata_plugin_error::NetdataPluginError::Other {
                message: format!("[{}] failed to compute histogram: {}", transaction, e),
            })?;

        let limit = request.last.unwrap_or(200);
        let file_indexes: Vec<_> = indexed_files.iter().map(|(_, idx)| idx.clone()).collect();
        let query_progress = ctx.progress.done_counter();

        let query_filter = filter_expr.clone();
        let query_search = request.query.clone();
        let query_direction = request.direction;
        let query_anchor = request.anchor;
        let query_time_range = time_range;
        let query_cancellation = ctx.cancellation.clone();

        let query_task = tokio::task::spawn_blocking(move || {
            LegacyLogsHandler::query_logs_from_indexes(
                &file_indexes,
                &query_time_range,
                query_anchor,
                &query_filter,
                &query_search,
                limit,
                query_direction,
                Some(query_cancellation),
                Some(query_progress),
            )
        });

        let (log_entries, has_before, has_after) = tokio::select! {
            result = query_task => {
                match result {
                    Ok(result) => result,
                    Err(e) => {
                        error!("[{}] log query task panicked: {}", transaction, e);
                        (Vec::new(), false, false)
                    }
                }
            }
            _ = ctx.cancellation.cancelled() => {
                warn!("[{}] log query cancelled", transaction);
                (Vec::new(), false, false)
            }
        };

        let (columns, data) = netdata::build_ui_response(&histogram, &log_entries);
        let transformations = netdata::systemd_transformations();

        let histogram_field_name = if request.histogram.is_empty() {
            "PRIORITY"
        } else {
            &request.histogram
        };
        let histogram_field = FieldName::new_unchecked(histogram_field_name);
        let ui_histogram = netdata::histogram(&histogram, &histogram_field, &transformations);

        let items = netdata::Items {
            // `u32::MAX` is the logs UI's "not computed" sentinel: this read-only
            // viewer does not track sampling statistics, so evaluated/unsampled/
            // estimated are reported as unknown rather than a misleading count.
            evaluated: u32::MAX as usize,
            unsampled: u32::MAX as usize,
            estimated: u32::MAX as usize,
            matched: ui_histogram.count(),
            // The logs UI treats `before`/`after` as booleans (0 = none, >0 = more
            // exist) in *display* order, which is inverted from the query's
            // chronological `has_before`/`has_after`. The crossover is intentional
            // and matches the former otel-signal-viewer catalog handler + the ledger.
            before: if has_after { 1 } else { 0 },
            after: if has_before { 1 } else { 0 },
            returned: log_entries.len(),
            max_to_return: limit,
        };

        let response = LegacyLogsResponseBody {
            progress: 100,
            version: netdata::Version::default(),
            accepted_params: accepted_params(),
            required_params: required_params(),
            facets: netdata::facets(&histogram, &transformations),
            histogram: ui_histogram,
            available_histograms: netdata::available_histograms(&histogram),
            columns,
            data,
            default_charts: Vec::new(),
            items,
            show_ids: false,
            has_history: true,
            status: 200,
            response_type: String::from("table"),
            help: String::from(
                "View, search and analyze OpenTelemetry logs stored by the former otel plugin.",
            ),
            pagination: netdata::Pagination::default(),
        };

        trace!(
            "[{}] completed transaction (total: {:?})",
            transaction,
            started.elapsed()
        );

        Ok(response)
    }

    fn declaration(&self) -> FunctionDeclaration {
        let mut func_decl = FunctionDeclaration::new(
            "legacy-otel-logs",
            "Query logs stored by the former OpenTelemetry plugin (read-only)",
        );
        func_decl.global = true;
        func_decl.tags = Some(String::from("logs"));
        func_decl.access =
            Some(HttpAccess::SIGNED_ID | HttpAccess::SAME_SPACE | HttpAccess::SENSITIVE_DATA);
        func_decl
    }
}

#[cfg(test)]
mod tests {
    use super::patch_args_into_payload;

    fn parse(out: Option<Vec<u8>>) -> serde_json::Value {
        serde_json::from_slice(&out.expect("expected a synthesized payload")).unwrap()
    }

    #[test]
    fn no_args_yields_no_payload() {
        assert!(patch_args_into_payload(&[], None).is_none());
    }

    #[test]
    fn existing_payload_is_not_overwritten() {
        // When the call already carries a POST payload, the GET-args shim is a no-op.
        let args = vec!["info".to_string()];
        assert!(patch_args_into_payload(&args, Some(b"{}")).is_none());
    }

    #[test]
    fn info_arg_sets_info_true() {
        let v = parse(patch_args_into_payload(&["info".to_string()], None));
        assert_eq!(v["info"], serde_json::json!(true));
    }

    #[test]
    fn without_info_arg_info_is_false() {
        let v = parse(patch_args_into_payload(&["after:10".to_string()], None));
        assert_eq!(v["info"], serde_json::json!(false));
    }

    #[test]
    fn after_and_before_are_parsed_as_numbers() {
        let args = vec!["after:100".to_string(), "before:200".to_string()];
        let v = parse(patch_args_into_payload(&args, None));
        assert_eq!(v["after"], serde_json::json!(100));
        assert_eq!(v["before"], serde_json::json!(200));
    }

    #[test]
    fn non_numeric_after_is_ignored() {
        // A malformed range token is dropped rather than poisoning the payload.
        let v = parse(patch_args_into_payload(&["after:soon".to_string()], None));
        assert!(v.get("after").is_none());
        assert_eq!(v["info"], serde_json::json!(false));
    }
}
