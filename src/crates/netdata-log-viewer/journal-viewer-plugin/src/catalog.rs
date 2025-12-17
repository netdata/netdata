//! Journal catalog functionality with file monitoring and metadata tracking

use async_trait::async_trait;
use netdata_plugin_error::Result;
use netdata_plugin_protocol::FunctionDeclaration;
use netdata_plugin_schema::HttpAccess;
use parking_lot::RwLock;
use rt::FunctionHandler;
use std::sync::Arc;
use std::sync::atomic::{AtomicU64, Ordering};
use tracing::{debug, error, info, instrument, warn};

// Import types from journal-function crate
use journal_function::{
    Facets, FileIndexKey, Histogram, HistogramEngine, IndexingEngine, IndexingEngineBuilder,
    Monitor, Registry, Result as CatalogResult, netdata,
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

/// Builds a Filter from the selections HashMap
#[instrument(skip(selections))]
fn build_filter_from_selections(selections: &HashMap<String, Vec<String>>) -> Filter {
    if selections.is_empty() {
        info!("no selections provided, using empty filter");
        return Filter::none();
    }

    let mut field_filters = Vec::new();

    for (field, values) in selections {
        // Ignore log sources. We've not implemented this functionality.
        if field == "__logs_sources" {
            info!("ignoring __logs_sources field");
            continue;
        }

        if values.is_empty() {
            continue;
        }

        info!(
            "building filter for field '{}' with {} values",
            field,
            values.len()
        );

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
        info!("no valid field filters, using empty filter");
        Filter::none()
    } else {
        info!("created filter with {} field filters", field_filters.len());
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
    report_progress: bool,
    cancel_call: bool,
}

/// Represents a tracked transaction for a function call.
///
/// Transactions track the lifecycle and state of individual function calls,
/// allowing for cancellation checks, progress reporting, and timeout detection.
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
                report_progress: false,
                cancel_call: false,
            })),
        }
    }

    /// Get the transaction ID.
    fn id(&self) -> String {
        self.inner.read().id.clone()
    }

    /// Check if the transaction has been marked for cancellation.
    #[allow(dead_code)]
    fn is_cancelled(&self) -> bool {
        self.inner.read().cancel_call
    }

    /// Mark the transaction for cancellation.
    fn cancel(&self) {
        self.inner.write().cancel_call = true;
    }

    /// Check if progress reporting is requested for this transaction.
    #[allow(dead_code)]
    fn should_report_progress(&self) -> bool {
        self.inner.read().report_progress
    }

    /// Set the progress reporting flag.
    fn set_report_progress(&self, report: bool) {
        self.inner.write().report_progress = report;
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
        debug!("Created transaction {} in registry", transaction.id());

        Some(transaction)
    }

    /// Get an existing transaction by ID.
    fn get(&self, id: &str) -> Option<Transaction> {
        self.transactions.read().get(id).cloned()
    }

    /// Remove a transaction from the registry.
    ///
    /// Returns the removed transaction if it existed.
    fn remove(&self, id: &str) -> Option<Transaction> {
        let transaction = self.transactions.write().remove(id);
        if transaction.is_some() {
            debug!("Removed transaction {} from registry", id);
        }
        transaction
    }

    /// Cancel a transaction by ID.
    ///
    /// Returns true if the transaction was found and cancelled.
    fn cancel(&self, id: &str) -> bool {
        if let Some(transaction) = self.get(id) {
            transaction.cancel();
            info!("Cancelled transaction {}", id);
            true
        } else {
            warn!("Cannot cancel non-existent transaction {}", id);
            false
        }
    }

    /// Get the number of active transactions.
    #[allow(dead_code)]
    fn len(&self) -> usize {
        self.transactions.read().len()
    }

    /// Check if the registry is empty.
    #[allow(dead_code)]
    fn is_empty(&self) -> bool {
        self.transactions.read().is_empty()
    }
}

/// Inner state for CatalogFunction (enables cloning)
struct CatalogFunctionInner {
    registry: Registry,
    indexing_engine: IndexingEngine,
    histogram_engine: Arc<HistogramEngine>,
    request_counter: AtomicU64,
    transaction_registry: TransactionRegistry,
}

/// Function handler that provides catalog information about journal files
#[derive(Clone)]
pub struct CatalogFunction {
    inner: Arc<CatalogFunctionInner>,
}

impl CatalogFunction {
    /// Query log entries from the indexed files (generic).
    ///
    /// This method:
    /// 1. Finds journal files in the time range
    /// 2. Retrieves indexed files from cache
    /// 3. Queries log entries using LogQuery
    /// 4. Returns raw log entry data and pagination flags
    ///
    /// Returns: (entries, has_before, has_after)
    /// - entries: The log entries matching the query
    /// - has_before: true if there are more entries before the returned window
    /// - has_after: true if there are more entries after the returned window
    async fn query_logs(
        &self,
        after: u32,
        before: u32,
        anchor: Option<u64>,
        facets: &[String],
        filter: &Filter,
        search_query: &str,
        limit: usize,
        direction: journal_index::Direction,
    ) -> (Vec<journal_function::LogEntryData>, bool, bool) {
        use journal_function::LogQuery;

        info!("querying logs for time range [{}, {})", after, before);

        // Find files in the time range
        let file_infos = match self
            .inner
            .registry
            .find_files_in_range(Seconds(after), Seconds(before))
        {
            Ok(files) => files,
            Err(e) => {
                warn!("Failed to find files in range: {}", e);
                return (Vec::new(), false, false);
            }
        };

        info!("found {} files in range", file_infos.len());

        // Collect indexed files from cache
        let mut indexed_files = Vec::new();
        let facets_obj = Facets::new(facets);

        for file_info in file_infos.iter() {
            let key = FileIndexKey::new(&file_info.file, &facets_obj);
            match self.inner.indexing_engine.get(&key).await {
                Ok(Some(index)) => indexed_files.push(index),
                Ok(None) => {
                    error!(
                        "file index is not ready for logs querying: {}",
                        file_info.file.path()
                    );
                    panic!("Adios");
                    continue;
                }
                Err(e) => {
                    error!("Failed to get index from cache: {}", e);
                    continue;
                }
            }
        }

        info!("found {} indexed files in cache", indexed_files.len());

        if indexed_files.is_empty() {
            info!("no indexed files available for log query");
            return (Vec::new(), false, false);
        }

        // Convert time range boundaries to microseconds
        let after_usec = after as u64 * 1_000_000;
        let before_usec = before as u64 * 1_000_000;

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

        // Only apply filter if it's not Filter::none() (which matches nothing)
        if !filter.is_none() {
            query = query.with_filter(filter.clone());
        }

        // Apply regex search if search_query is not empty
        if !search_query.is_empty() {
            debug!("applying regex filter to log query: {:?}", search_query);
            query = query.with_regex(search_query);
        }

        let mut log_entries = match query.execute() {
            Ok(entries) => {
                info!("retrieved {} log entries (limit + 1 query)", entries.len());
                entries
            }
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
            info!(
                "truncated to {} entries, more available in query direction",
                limit
            );
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

            // Only apply filter if it's not Filter::none() (which matches nothing)
            if !filter.is_none() {
                opposite_query = opposite_query.with_filter(filter.clone());
            }

            // Apply regex search if search_query is not empty
            if !search_query.is_empty() {
                debug!("applying regex filter to opposite direction query");
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

        info!(
            "pagination flags: has_before={}, has_after={}",
            has_before, has_after
        );

        // UI always expects logs sorted descending by timestamp (newest first)
        // regardless of query direction
        log_entries.sort_by(|a, b| b.timestamp.cmp(&a.timestamp));

        (log_entries, has_before, has_after)
    }

    /// Create a new catalog function with the given monitor and cache configuration.
    ///
    /// # Arguments
    /// * `monitor` - File system monitor for watching journal directories
    /// * `cache_dir` - Directory path for disk cache storage
    /// * `memory_capacity` - Number of file indexes to keep in memory
    /// * `disk_capacity` - Disk cache size in bytes
    /// * `file_indexing_metrics` - Metrics chart for file indexing operations
    /// * `bucket_cache_metrics` - Metrics chart for bucket cache operations
    /// * `bucket_operations_metrics` - Metrics chart for bucket operations
    pub async fn new(
        monitor: Monitor,
        cache_dir: impl Into<std::path::PathBuf>,
        memory_capacity: usize,
        disk_capacity: usize,
    ) -> CatalogResult<Self> {
        let registry = Registry::new(monitor);

        // Create indexing engine with disk-backed cache
        let indexing_engine = IndexingEngineBuilder::new()
            .with_cache_path(cache_dir)
            .with_memory_capacity(memory_capacity)
            .with_disk_capacity(disk_capacity)
            .with_block_size(4 * 1024 * 1024)
            .build()
            .await?;

        // Create histogram engine
        let histogram_engine = HistogramEngine::new(registry.clone(), indexing_engine.clone());

        // Initialize response logging directory at info level
        if tracing::enabled!(tracing::Level::INFO) {
            let response_dir = std::path::Path::new("/tmp/responses");
            if response_dir.exists() {
                if let Err(e) = std::fs::remove_dir_all(response_dir) {
                    warn!("Failed to remove existing /tmp/responses directory: {}", e);
                }
            }
            if let Err(e) = std::fs::create_dir_all(response_dir) {
                warn!("Failed to create /tmp/responses directory: {}", e);
            } else {
                info!("created /tmp/responses directory for response logging");
            }
        }

        let inner = CatalogFunctionInner {
            registry,
            indexing_engine,
            histogram_engine: Arc::new(histogram_engine),
            request_counter: AtomicU64::new(0),
            transaction_registry: TransactionRegistry::new(),
        };

        Ok(Self {
            inner: Arc::new(inner),
        })
    }

    /// Get a histogram for the given parameters
    pub async fn get_histogram(
        &self,
        after: u32,
        before: u32,
        facets: &[String],
        filter: &Filter,
    ) -> CatalogResult<Histogram> {
        self.inner
            .histogram_engine
            .query()
            .with_time_range(after, before)
            .with_facets(facets)
            .with_filter(filter)
            .execute()
            .await
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

    /// Logs the response as pretty-printed JSON to /tmp/responses/<request-number>-<timestamp>.json
    fn log_response(&self, response: &CatalogResponse) {
        // Early return if INFO level is not enabled
        if !tracing::enabled!(tracing::Level::INFO) {
            return;
        }

        const MAX_RESPONSES: usize = 60;
        const RESPONSE_DIR: &str = "/tmp/responses";

        // Get request number and increment counter
        let request_num = self.inner.request_counter.fetch_add(1, Ordering::SeqCst);

        // Get current timestamp in microseconds
        let timestamp = std::time::SystemTime::now()
            .duration_since(std::time::UNIX_EPOCH)
            .unwrap_or_default()
            .as_micros();

        // Build filename
        let filename = format!("{}/{:06}-{}.json", RESPONSE_DIR, request_num, timestamp);

        // Serialize response as pretty-printed JSON
        match serde_json::to_string_pretty(response) {
            Ok(json) => {
                if let Err(e) = std::fs::write(&filename, json) {
                    warn!("failed to write response to {}: {}", filename, e);
                    return;
                }
                info!("logged response to {}", filename);
            }
            Err(e) => {
                warn!("failed to serialize response to JSON: {}", e);
                return;
            }
        }

        // Clean up old files, keeping only the latest MAX_RESPONSES
        if let Ok(entries) = std::fs::read_dir(RESPONSE_DIR) {
            let mut files: Vec<_> = entries
                .filter_map(|entry| {
                    let entry = entry.ok()?;
                    let path = entry.path();

                    // Only consider .json files
                    if !path.extension().map_or(false, |ext| ext == "json") {
                        return None;
                    }

                    // Extract request number from filename (format: NNNNNN-timestamp.json)
                    let filename = path.file_name()?.to_str()?;
                    let request_num_str = filename.split('-').next()?;
                    let request_num: u64 = request_num_str.parse().ok()?;

                    Some((path, request_num))
                })
                .collect();

            // Sort by request number (newest/highest first)
            files.sort_by(|a, b| b.1.cmp(&a.1));

            // Remove files beyond MAX_RESPONSES
            for (path, _) in files.iter().skip(MAX_RESPONSES) {
                if let Err(e) = std::fs::remove_file(path) {
                    warn!("failed to remove old response file {:?}: {}", path, e);
                } else {
                    info!("removed old response file {:?}", path);
                }
            }
        }
    }
}

#[async_trait]
impl FunctionHandler for CatalogFunction {
    type Request = CatalogRequest;
    type Response = CatalogResponse;

    #[instrument(name = "catalog_function_call", skip_all, fields(
        after = request.after,
        before = request.before,
        num_selections = request.selections.len()
    ))]
    async fn on_call(&self, transaction: String, request: Self::Request) -> Result<Self::Response> {
        // Register the transaction
        let txn = self.inner.transaction_registry.create(transaction.clone());
        if txn.is_none() {
            warn!(
                "Transaction {} already exists, continuing anyway",
                transaction
            );
        }

        info!("Got function call for transaction {}", transaction);

        // Log if regex search is being used
        if !request.query.is_empty() {
            info!("regex search query provided: {:?}", request.query);
        } else {
            debug!("no regex search query provided");
        }

        let filter_expr = build_filter_from_selections(&request.selections);

        if request.after >= request.before {
            error!(
                "invalid time range: after={} >= before={}",
                request.after, request.before
            );
            return Err(netdata_plugin_error::NetdataPluginError::Other {
                message: "invalid time range: after must be less than before".to_string(),
            });
        }

        info!(
            "Creating histogram request: after={}, before={}",
            request.after, request.before
        );

        // Get facets from request or use empty list
        // FIXME: Need to review this
        let facets: Vec<String> = request
            .facets
            .iter()
            .filter(|f| *f != "__logs_sources") // Ignore special fields
            .cloned()
            .collect();

        info!("getting histogram from catalog");
        let histogram = self
            .get_histogram(request.after, request.before, &facets, &filter_expr)
            .await
            .map_err(|e| netdata_plugin_error::NetdataPluginError::Other {
                message: format!("failed to get histogram: {}", e),
            })?;
        info!("histogram computation complete");

        let limit = request.last.unwrap_or(200);
        let (log_entries, has_before, has_after) = self
            .query_logs(
                request.after,
                request.before,
                request.anchor,
                &facets,
                &filter_expr,
                &request.query,
                limit,
                request.direction,
            )
            .await;

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

        info!(
            "successfully created response with {} facets",
            response.facets.len()
        );

        // Log response to file
        self.log_response(&response);

        // Remove the transaction from the registry
        self.inner.transaction_registry.remove(&transaction);
        info!("Transaction {} completed successfully", transaction);

        Ok(response)
    }

    async fn on_cancellation(&self, transaction: String) -> Result<Self::Response> {
        warn!("catalog function call {} cancelled by Netdata", transaction);

        // Mark the transaction as cancelled
        self.inner.transaction_registry.cancel(&transaction);

        // Remove the transaction from the registry
        self.inner.transaction_registry.remove(&transaction);

        Err(netdata_plugin_error::NetdataPluginError::Other {
            message: "catalog function cancelled by user".to_string(),
        })
    }

    async fn on_progress(&self, transaction: String) {
        info!(
            "progress report requested for catalog function call {}",
            transaction
        );

        // Mark the transaction for progress reporting
        if let Some(txn) = self.inner.transaction_registry.get(&transaction) {
            txn.set_report_progress(true);
            debug!(
                "Transaction {} marked for progress reporting (elapsed: {:?})",
                transaction,
                txn.elapsed()
            );
        } else {
            warn!(
                "Progress requested for non-existent transaction {}",
                transaction
            );
        }
    }

    fn declaration(&self) -> FunctionDeclaration {
        info!("generating function declaration for systemd-journal");
        let mut func_decl = FunctionDeclaration::new(
            "systemd-journal",
            "Query and visualize systemd journal entries with histograms and facets",
        );
        func_decl.global = true;
        func_decl.tags = Some(String::from("logs"));
        func_decl.access =
            Some(HttpAccess::SIGNED_ID | HttpAccess::SAME_SPACE | HttpAccess::SENSITIVE_DATA);
        func_decl
    }
}
