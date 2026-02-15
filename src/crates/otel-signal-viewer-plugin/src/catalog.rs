//! Journal catalog functionality with file monitoring and metadata tracking

use async_trait::async_trait;
use netdata_plugin_error::Result;
use netdata_plugin_protocol::FunctionDeclaration;
use netdata_plugin_schema::HttpAccess;
use parking_lot::RwLock;
use rt::{FunctionCallContext, FunctionHandler};
use std::sync::Arc;
use std::sync::atomic::AtomicUsize;
use tokio_util::sync::CancellationToken;
use tracing::{error, instrument, trace, warn};

// Import types from journal-function crate
use journal_function::{
    FileIndexCache, FileIndexCacheBuilder, FileIndexKey, HistogramEngine, IndexingLimits, Monitor,
    Registry, Result as CatalogResult, netdata,
};

/*
 * CatalogFunction
*/
use std::collections::HashMap;

/// Request parameters for the catalog function (uses journal request structure)
pub type CatalogRequest = netdata::JournalRequest;

/// Response from the catalog function (uses journal response structure)
pub type CatalogResponse = netdata::JournalResponse;

use journal_index::Filter;
use journal_index::{FieldName, FieldValuePair, Microseconds, Seconds};

/// Default facet fields when the request does not specify any.
const DEFAULT_FACETS: &[&str] = &[
    "_HOSTNAME",
    "PRIORITY",
    "SYSLOG_FACILITY",
    "ERRNO",
    "SYSLOG_IDENTIFIER",
    "USER_UNIT",
    "MESSAGE_ID",
    "_BOOT_ID",
    "_SYSTEMD_OWNER_UID",
    "_UID",
    "OBJECT_SYSTEMD_OWNER_UID",
    "OBJECT_UID",
    "_GID",
    "OBJECT_GID",
    "_CAP_EFFECTIVE",
    "_AUDIT_LOGINUID",
    "OBJECT_AUDIT_LOGINUID",
    "CODE_FUNC",
    "ND_LOG_SOURCE",
    "CODE_FILE",
    "ND_ALERT_NAME",
    "ND_ALERT_CLASS",
    "_SELINUX_CONTEXT",
    "_MACHINE_ID",
    "ND_ALERT_TYPE",
    "_SYSTEMD_SLICE",
    "_EXE",
    "_NAMESPACE",
    "_TRANSPORT",
    "_RUNTIME_SCOPE",
    "_STREAM_ID",
    "ND_NIDL_CONTEXT",
    "ND_ALERT_STATUS",
    "ND_NIDL_NODE",
    "ND_ALERT_COMPONENT",
    "_COMM",
    "_SYSTEMD_USER_UNIT",
    "_SYSTEMD_USER_SLICE",
    "__logs_sources",
    "log.severity_number",
];

/// Resolve facet field names from the request, falling back to defaults.
fn resolve_facets(request_facets: &[String]) -> Vec<FieldName> {
    if request_facets.is_empty() {
        DEFAULT_FACETS
            .iter()
            .filter_map(|s| FieldName::new(s))
            .collect()
    } else {
        request_facets
            .iter()
            .filter_map(|s| FieldName::new(s))
            .collect()
    }
}

/// Builds a Filter from the selections HashMap
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

        // Build OR filter for all values of this field
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

        let field_filter = Filter::or(value_filters);
        field_filters.push(field_filter);
    }

    if field_filters.is_empty() {
        Filter::none()
    } else {
        Filter::and(field_filters)
    }
}

fn accepted_params() -> Vec<netdata::RequestParam> {
    use netdata::RequestParam;

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
        RequestParam::IfModifiedSince,
        RequestParam::DataOnly,
        RequestParam::Delta,
        RequestParam::Tail,
        RequestParam::Sampling,
        RequestParam::Slice,
    ]
}

fn required_params() -> Vec<netdata::RequiredParam> {
    Vec::new()
}

#[derive(Debug)]
struct TransactionInner {
    id: String,
    start_time: tokio::time::Instant,
}

/// Represents a tracked transaction for a function call.
///
/// Transactions track the lifecycle of individual function calls.
#[derive(Debug, Clone)]
struct Transaction {
    inner: Arc<RwLock<TransactionInner>>,
}

impl Transaction {
    /// Create a new transaction with the given ID.
    fn new(id: String) -> Self {
        Self {
            inner: Arc::new(RwLock::new(TransactionInner {
                id,
                start_time: tokio::time::Instant::now(),
            })),
        }
    }

    /// Get the transaction ID.
    fn id(&self) -> String {
        self.inner.read().id.clone()
    }

    /// Get the elapsed time since the transaction started.
    fn elapsed(&self) -> std::time::Duration {
        self.inner.read().start_time.elapsed()
    }
}

/// Registry for managing active transactions.
///
/// Provides thread-safe storage and lookup for transactions, allowing
/// multiple parts of the application to track and manage ongoing operations.
#[derive(Debug, Clone)]
struct TransactionRegistry {
    transactions: Arc<RwLock<HashMap<String, Transaction>>>,
}

impl TransactionRegistry {
    /// Create a new transaction registry.
    fn new() -> Self {
        Self {
            transactions: Arc::new(RwLock::new(HashMap::new())),
        }
    }

    /// Create and register a new transaction with the given ID.
    ///
    /// Returns None if a transaction with this ID already exists.
    fn create(&self, id: String) -> Option<Transaction> {
        let mut transactions = self.transactions.write();

        if transactions.contains_key(&id) {
            warn!("Transaction {} already exists in registry", id);
            return None;
        }

        let transaction = Transaction::new(id.clone());
        transactions.insert(id, transaction.clone());

        Some(transaction)
    }

    /// Remove a transaction from the registry.
    ///
    /// Returns the removed transaction if it existed.
    fn remove(&self, id: &str) -> Option<Transaction> {
        self.transactions.write().remove(id)
    }
}

/// Inner state for CatalogFunction
struct CatalogFunctionInner {
    registry: Registry,
    cache: FileIndexCache,
    histogram_engine: Arc<HistogramEngine>,
    transaction_registry: TransactionRegistry,
    indexing_limits: IndexingLimits,
}

/// Function handler that provides catalog information about journal files
#[derive(Clone)]
pub struct CatalogFunction {
    inner: Arc<CatalogFunctionInner>,
}

impl CatalogFunction {
    /// Query log entries from pre-indexed files.
    ///
    /// This method:
    /// 1. Queries log entries using LogQuery from pre-indexed files
    /// 2. Returns raw log entry data and pagination flags
    ///
    /// Returns: (entries, has_before, has_after)
    /// - entries: The log entries matching the query
    /// - has_before: true if there are more entries before the returned window
    /// - has_after: true if there are more entries after the returned window
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

        // Convert time range boundaries to microseconds
        let after_usec = time_range.aligned_start() as u64 * 1_000_000;
        let before_usec = time_range.aligned_end() as u64 * 1_000_000;

        // Query log entries
        // Determine anchor point: use explicit anchor if provided, otherwise use time range boundary
        let query_anchor = if let Some(anchor_usec) = anchor {
            match direction {
                journal_index::Direction::Forward => {
                    // Forward: start from lower boundary
                    journal_index::Anchor::Timestamp(Microseconds(anchor_usec + 1))
                }
                journal_index::Direction::Backward => {
                    // Backward: start from upper boundary
                    journal_index::Anchor::Timestamp(Microseconds(anchor_usec - 1))
                }
            }
        } else {
            // No explicit anchor: start from time range boundary based on direction
            match direction {
                journal_index::Direction::Forward => {
                    // Forward: start from lower boundary
                    journal_index::Anchor::Timestamp(Microseconds(after_usec))
                }
                journal_index::Direction::Backward => {
                    // Backward: start from upper boundary
                    journal_index::Anchor::Timestamp(Microseconds(before_usec))
                }
            }
        };

        // Query with limit + 1 to detect if there are more entries in this direction
        let mut query = LogQuery::new(&indexed_files, query_anchor, direction)
            .with_limit(limit + 1)
            .with_after_usec(after_usec)
            .with_before_usec(before_usec);

        if let Some(ref t) = cancellation {
            query = query.with_cancellation(t.clone());
        }

        if let Some(counter) = progress {
            query = query.with_progress(counter);
        }

        // Only apply filter if it's not Filter::none() (which matches nothing)
        if !filter.is_none() {
            query = query.with_filter(filter.clone());
        }

        // Apply regex search if search_query is not empty
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

        // Check if we got more than limit entries (meaning there are more in this direction)
        let has_more_in_query_direction = log_entries.len() > limit;
        if has_more_in_query_direction {
            log_entries.truncate(limit);
        }

        // Query 1 entry in the opposite direction to check if there are entries there
        // However, if there's no anchor (initial query), we're starting from the time range
        // boundary, so there are no entries in the opposite direction by definition.
        let has_more_in_opposite_direction = if anchor.is_none() {
            // No anchor: we're at the time range boundary, no entries in opposite direction
            false
        } else {
            let opposite_direction = match direction {
                journal_index::Direction::Forward => journal_index::Direction::Backward,
                journal_index::Direction::Backward => journal_index::Direction::Forward,
            };

            let opposite_anchor = match opposite_direction {
                journal_index::Direction::Forward => {
                    journal_index::Anchor::Timestamp(Microseconds(anchor.unwrap() + 1))
                }
                journal_index::Direction::Backward => {
                    journal_index::Anchor::Timestamp(Microseconds(anchor.unwrap() - 1))
                }
            };

            let mut opposite_query =
                LogQuery::new(&indexed_files, opposite_anchor, opposite_direction)
                    .with_limit(1)
                    .with_after_usec(after_usec)
                    .with_before_usec(before_usec);

            if let Some(ref t) = cancellation {
                opposite_query = opposite_query.with_cancellation(t.clone());
            }

            // Only apply filter if it's not Filter::none() (which matches nothing)
            if !filter.is_none() {
                opposite_query = opposite_query.with_filter(filter.clone());
            }

            // Apply regex search if search_query is not empty
            if !search_query.is_empty() {
                trace!("applying regex filter to opposite direction query");
                opposite_query = opposite_query.with_regex(search_query);
            }

            match opposite_query.execute() {
                Ok(entries) => !entries.is_empty(),
                Err(e) => {
                    warn!("opposite direction query error: {}", e);
                    if !search_query.is_empty() {
                        warn!(
                            "opposite direction query may have failed due to invalid regex pattern"
                        );
                    }
                    false
                }
            }
        };

        // Calculate has_before and has_after based on the query direction
        let (has_before, has_after) = match direction {
            journal_index::Direction::Forward => {
                // Forward query: has_more means has_after, opposite check gives has_before
                (has_more_in_opposite_direction, has_more_in_query_direction)
            }
            journal_index::Direction::Backward => {
                // Backward query: has_more means has_before, opposite check gives has_after
                (has_more_in_query_direction, has_more_in_opposite_direction)
            }
        };

        // UI always expects logs sorted descending by timestamp (newest first)
        // regardless of query direction
        log_entries.sort_by(|a, b| b.timestamp.cmp(&a.timestamp));

        (log_entries, has_before, has_after)
    }

    /// Create a new catalog function with the given monitor and cache configuration.
    ///
    /// # Arguments
    /// * `monitor` - File system monitor for watching journal directories
    /// * `memory_capacity` - Number of file indexes to keep in memory
    /// * `indexing_limits` - Configuration limits for indexing (cardinality, payload size)
    pub async fn new(
        monitor: Monitor,
        memory_capacity: usize,
        indexing_limits: IndexingLimits,
    ) -> CatalogResult<Self> {
        let registry = Registry::new(monitor);

        // Create file index cache (in-memory LRU)
        let cache = FileIndexCacheBuilder::new()
            .with_memory_capacity(memory_capacity)
            .build();

        // Create histogram engine
        let histogram_engine = HistogramEngine::new();

        let inner = CatalogFunctionInner {
            registry,
            cache,
            histogram_engine: Arc::new(histogram_engine),
            transaction_registry: TransactionRegistry::new(),
            indexing_limits,
        };

        Ok(Self {
            inner: Arc::new(inner),
        })
    }

    /// Watch a directory for journal files
    pub fn watch_directory(&self, path: &str) -> Result<()> {
        self.inner.registry.watch_directory(path).map_err(|e| {
            netdata_plugin_error::NetdataPluginError::Other {
                message: format!("failed to watch directory: {}", e),
            }
        })
    }

    /// Process a notify event
    pub fn process_notify_event(&self, event: notify::Event) {
        if let Err(e) = self.inner.registry.process_event(event) {
            error!("failed to process notify event: {}", e);
        }
    }
}

#[async_trait]
impl FunctionHandler for CatalogFunction {
    type Request = CatalogRequest;
    type Response = CatalogResponse;

    async fn on_call(
        &self,
        ctx: FunctionCallContext,
        request: Self::Request,
    ) -> Result<Self::Response> {
        let transaction = ctx.transaction();

        // Register the transaction
        let Some(txn) = self
            .inner
            .transaction_registry
            .create(transaction.to_owned())
        else {
            return Err(netdata_plugin_error::NetdataPluginError::Other {
                message: format!("[{}] transaction already exists", transaction),
            });
        };
        trace!("[{}] started transaction", txn.id());

        // Create query time range with automatic alignment
        let time_range = journal_function::QueryTimeRange::new(request.after, request.before)
            .map_err(|e| {
                let msg = format!("[{}] {}", txn.id(), e);
                netdata_plugin_error::NetdataPluginError::Other { message: msg }
            })?;

        trace!(
            "[{}] time range: [{}, {}), aligned: [{}, {}), bucket duration: {} seconds",
            txn.id(),
            time_range.requested_start(),
            time_range.requested_end(),
            time_range.aligned_start(),
            time_range.aligned_end(),
            time_range.bucket_duration()
        );

        // Find files in the time range
        let op_start = std::time::Instant::now();
        let files = self
            .inner
            .registry
            .find_files_in_range(Seconds(request.after), Seconds(request.before))
            .map_err(|e| {
                let msg = format!("[{}] failed to find files in range: {}", txn.id(), e);
                netdata_plugin_error::NetdataPluginError::Other { message: msg }
            })?;
        let find_files_duration = op_start.elapsed();
        trace!("[{}] found {} files in time range", txn.id(), files.len(),);
        if tracing::enabled!(tracing::Level::TRACE) {
            for (idx, file_info) in files.iter().enumerate() {
                tracing::trace!(
                    "[{}] file[{}/{}]: {}",
                    txn.id(),
                    idx + 1,
                    files.len(),
                    file_info.file.path(),
                );
            }
        }

        // Build filter expression
        let filter_expr = build_filter_from_selections(&request.selections);
        trace!("[{}] filter expression: {}", txn.id(), filter_expr);

        // Build file index keys
        let source_timestamp_field = FieldName::new_unchecked("_SOURCE_REALTIME_TIMESTAMP");
        let keys: Vec<FileIndexKey> = files
            .iter()
            .map(|f| FileIndexKey::new(&f.file, Some(source_timestamp_field.clone())))
            .collect();

        // Progress is reported in two phases: indexing and querying.
        // Start with total = number of files for the indexing phase. After
        // indexing completes we extend the total so the query phase gets its
        // own progress range. This avoids over-estimating total work when the
        // second phase is fast (which is the common case).
        let num_files = keys.len();
        ctx.progress.set_total(num_files);

        let op_start = std::time::Instant::now();
        let indexed_files = journal_function::batch_compute_file_indexes(
            &self.inner.cache,
            &self.inner.registry,
            keys,
            ctx.cancellation.clone(),
            self.inner.indexing_limits,
            Some(ctx.progress.done_counter()),
        )
        .await
        .map_err(|e| {
            let msg = format!("[{}] failed to index files: {}", txn.id(), e);
            netdata_plugin_error::NetdataPluginError::Other { message: msg }
        })?;
        let indexing_duration = op_start.elapsed();

        // Extend progress to cover the query phase. The done counter is
        // already at ~keys.len(), so the UI will show ~50% until querying
        // catches up.
        ctx.progress.set_total(2 * num_files);

        trace!(
            "[{}] retrieved {}/{} file indexes for histogram buckets and log entries",
            txn.id(),
            indexed_files.len(),
            files.len(),
        );
        if tracing::enabled!(tracing::Level::TRACE) {
            for (idx, (key, file_index)) in indexed_files.iter().enumerate() {
                tracing::trace!(
                    "[{}] file index[{}/{}]: {}, indexed at: {}, online: {}, bucket duration: {}",
                    txn.id(),
                    idx + 1,
                    files.len(),
                    key.file.path(),
                    file_index.indexed_at().0,
                    file_index.online(),
                    file_index.bucket_duration().0
                );
            }
        }

        // Resolve which facet fields to include in the histogram response
        let facets = resolve_facets(&request.facets);

        // Compute histogram from pre-indexed files
        let op_start = std::time::Instant::now();
        let histogram = self
            .inner
            .histogram_engine
            .compute_from_indexes(&indexed_files, &time_range, &facets, &filter_expr)
            .map_err(|e| {
                let msg = format!("[{}] failed to compute histogram: {}", txn.id(), e);
                netdata_plugin_error::NetdataPluginError::Other { message: msg }
            })?;
        let histogram_duration = op_start.elapsed();

        // Query logs from pre-indexed files, wrapped in spawn_blocking so the
        // async runtime can handle cancellation while this runs.
        let op_start = std::time::Instant::now();
        let limit = request.last.unwrap_or(200);
        let file_indexes: Vec<_> = indexed_files.iter().map(|(_, idx)| idx.clone()).collect();
        let query_progress = ctx.progress.done_counter();

        let query_filter = filter_expr.clone();
        let query_search = request.query.clone();
        let query_direction = request.direction;
        let query_anchor = request.anchor;
        let query_time_range = time_range.clone();
        let query_cancellation = ctx.cancellation.clone();

        let query_task = tokio::task::spawn_blocking(move || {
            CatalogFunction::query_logs_from_indexes(
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
                        error!("[{}] log query task panicked: {}", txn.id(), e);
                        (Vec::new(), false, false)
                    }
                }
            }
            _ = ctx.cancellation.cancelled() => {
                warn!("[{}] log query cancelled", txn.id());
                (Vec::new(), false, false)
            }
        };
        let query_logs_duration = op_start.elapsed();
        trace!(
            "[{}] retrieved {} log entries (has before: {}, has after: {})",
            txn.id(),
            log_entries.len(),
            has_before,
            has_after
        );

        // Build Netdata UI response (columns + data)
        let (columns, data) = netdata::build_ui_response(&histogram, &log_entries);

        // Get transformations for histogram chart labels
        let transformations = netdata::systemd_transformations();

        // Determine which field to use for the histogram (default to PRIORITY if not specified)
        let histogram_field_name = if request.histogram.is_empty() {
            "PRIORITY"
        } else {
            &request.histogram
        };
        let histogram_field = FieldName::new_unchecked(histogram_field_name);

        let ui_histogram = netdata::histogram(&histogram, &histogram_field, &transformations);

        let items = netdata::Items {
            evaluated: u32::MAX as usize,
            unsampled: u32::MAX as usize,
            estimated: u32::MAX as usize,
            matched: ui_histogram.count(),
            // UI treats these as booleans: 0 = false, >0 = true
            before: if has_after { 1 } else { 0 },
            after: if has_before { 1 } else { 0 },
            returned: log_entries.len(),
            max_to_return: limit,
        };

        let response = CatalogResponse {
            progress: 100, // All responses are now complete
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
            help: String::from("View, search and analyze systemd journal entries."),
            pagination: netdata::Pagination::default(),
        };

        let Some(txn) = self.inner.transaction_registry.remove(&transaction) else {
            return Err(netdata_plugin_error::NetdataPluginError::Other {
                message: format!("[{}] transaction does not exist", transaction),
            });
        };
        trace!(
            "[{}] completed transaction (find_files: {:?}, indexing: {:?}, histogram: {:?}, query_logs: {:?}, total: {:?})",
            txn.id(),
            find_files_duration,
            indexing_duration,
            histogram_duration,
            query_logs_duration,
            txn.elapsed()
        );

        Ok(response)
    }

    fn declaration(&self) -> FunctionDeclaration {
        // NOTE: `rt` special cases this function call to handle GET/POST
        // calls in a consistent way. If you rename this function, you should
        // update the `rt` crate as well.

        let mut func_decl =
            FunctionDeclaration::new("otel-logs", "Query and visualize OpenTelemetry logs");
        func_decl.global = true;
        func_decl.tags = Some(String::from("logs"));
        func_decl.access =
            Some(HttpAccess::SIGNED_ID | HttpAccess::SAME_SPACE | HttpAccess::SENSITIVE_DATA);
        func_decl
    }
}
