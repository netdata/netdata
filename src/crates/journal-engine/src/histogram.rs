//! Facet-value histogram engine for journal log queries.
//!
//! [`HistogramEngine::compute_from_indexes`] aggregates pre-indexed journal
//! files into one [`BucketResponse`] per bucket of the query's time range:
//! entries per bucket and per indexed field=value pair, each split into an
//! (unfiltered, filtered) pair by the query filter, plus the fields the
//! files carry but do not index as facets. Responses are cached in the
//! engine, keyed by [`BucketRequest`].
//!
//! Not to be confused with journal-index's same-named `Histogram`
//! (`journal-index/src/histogram.rs`) - the per-file time-coverage
//! running-count structure these counts are read through; this module is
//! the query-level facet aggregation on top of it.
//!
//! Production flow: otel-legacy-logs keeps one shared engine
//! (`otel-legacy-logs/src/handler.rs` `LegacyLogsHandlerInner`) and calls
//! `compute_from_indexes` per query (`LegacyLogsHandler::on_call`); journal-function
//! turns the result into the viewer's facets, charts and column schema
//! (`journal-function/src/netdata/facets.rs` `facets`,
//! `journal-function/src/netdata/histogram.rs` `available_histograms` and
//! `journal-function/src/netdata/builder.rs` `build_ui_response`).

use crate::{cache::FileIndexKey, error::Result, facets::Facets};
use journal_core::collections::HashSet;
use journal_index::{Bitmap, FieldName, FieldValuePair, FileIndex, Filter, Seconds};
use lru::LruCache;
use parking_lot::RwLock;
use std::collections::HashMap;
use std::num::NonZeroUsize;
use std::time::Duration;

#[allow(unused_imports)]
use tracing::{debug, error};

/// Bucket width, in seconds, for a query time range: the largest of the
/// "nice" widths below that still divides the range into at least 50
/// buckets (integer division), or 1s when even 1s does not reach 50 (e.g.
/// a 1s range yields a single 1s bucket).
///
/// `QueryTimeRange::new` is the only internal caller
/// ([`crate::QueryTimeRange::new`]) and aligns the range onto this width, so query
/// buckets are always `[multiple of width, + width)`. otel-ledger's ledger
/// grid independently mirrors this width set
/// (`otel-ledger/src/ledger/rpc/grid.rs` `TARGET_BUCKETS`/`VALID_BUCKET_WIDTHS_S`).
pub fn calculate_bucket_duration(time_range_duration: u32) -> u32 {
    const MINUTE: Duration = Duration::from_secs(60);
    const HOUR: Duration = Duration::from_secs(60 * MINUTE.as_secs());
    const DAY: Duration = Duration::from_secs(24 * HOUR.as_secs());

    const VALID_DURATIONS: &[Duration] = &[
        // Seconds
        Duration::from_secs(1),
        Duration::from_secs(2),
        Duration::from_secs(5),
        Duration::from_secs(10),
        Duration::from_secs(15),
        Duration::from_secs(30),
        // Minutes
        MINUTE,
        Duration::from_secs(2 * MINUTE.as_secs()),
        Duration::from_secs(3 * MINUTE.as_secs()),
        Duration::from_secs(5 * MINUTE.as_secs()),
        Duration::from_secs(10 * MINUTE.as_secs()),
        Duration::from_secs(15 * MINUTE.as_secs()),
        Duration::from_secs(30 * MINUTE.as_secs()),
        // Hours
        HOUR,
        Duration::from_secs(2 * HOUR.as_secs()),
        Duration::from_secs(6 * HOUR.as_secs()),
        Duration::from_secs(8 * HOUR.as_secs()),
        Duration::from_secs(12 * HOUR.as_secs()),
        // Days
        DAY,
        Duration::from_secs(2 * DAY.as_secs()),
        Duration::from_secs(3 * DAY.as_secs()),
        Duration::from_secs(5 * DAY.as_secs()),
        Duration::from_secs(7 * DAY.as_secs()),
        Duration::from_secs(14 * DAY.as_secs()),
        Duration::from_secs(30 * DAY.as_secs()),
    ];

    VALID_DURATIONS
        .iter()
        .rev()
        .find(|&&bucket_width| time_range_duration as u64 / bucket_width.as_secs() >= 50)
        .map(|d| d.as_secs())
        .unwrap_or(1) as u32
}

/// One query bucket: the engine's unit of computation, caching and result.
///
/// `[start, end)` is one window of the query grid
/// ([`crate::QueryTimeRange::buckets`]): `start`
/// inclusive, `end` exclusive, `end - start` the query's bucket width. The
/// derived `Eq`/`Hash` make a whole request the identity of a cached
/// response, so the same window, facet set and filter reuse one entry:
/// `facets` hashes its canonical, order-insensitive field set
/// ([`Facets`]) and `filter_expr` hashes its expression tree
/// (`journal-index/src/filter.rs` `Hash for FilterExpr`).
#[derive(Debug, Clone, Eq, PartialEq, Hash)]
pub struct BucketRequest {
    /// Bucket start: inclusive, a multiple of the query's bucket width
    pub start: Seconds,
    /// Bucket end: exclusive; `end - start` is the query's bucket width
    pub end: Seconds,
    /// Facet set the counts are computed over; also part of the cache identity
    pub facets: Facets,
    /// Query filter applied to every count; `Filter::none()` means unfiltered
    pub filter_expr: Filter,
}

impl BucketRequest {
    /// Bucket width in seconds (`end - start`)
    pub fn duration(&self) -> Seconds {
        self.end - self.start
    }
}

/// Aggregated counts for one bucket, summed across all overlapping files.
///
/// Every count is an (unfiltered, filtered) pair: entries in the bucket,
/// and entries that additionally match the request's `filter_expr`. The
/// two are equal when no filter applies. Consumers read the filtered
/// member: journal-function's facet options
/// (`journal-function/src/netdata/facets.rs` `facets`) and chart dimensions
/// (`journal-function/src/netdata/histogram.rs` `histogram`).
#[derive(Debug, Clone)]
pub struct BucketResponse {
    /// Counts per indexed field=value pair: (unfiltered, filtered)
    pub fv_counts: HashMap<FieldValuePair, (usize, usize)>,
    /// Fields present in the files but outside the facet set (no bitmaps)
    pub unindexed_fields: HashSet<FieldName>,
    /// Entries in the bucket across all overlapping files: (unfiltered, filtered)
    pub total_entries: (usize, usize),
}

impl BucketResponse {
    /// Creates a new empty bucket response.
    pub(crate) fn new() -> Self {
        Self {
            fv_counts: HashMap::default(),
            unindexed_fields: HashSet::default(),
            total_entries: (0, 0),
        }
    }

    /// Fields of the `fv_counts` keys: every field=value pair an
    /// overlapping file indexes is counted here, even when this bucket
    /// holds zero of its entries.
    pub fn indexed_fields(&self) -> HashSet<FieldName> {
        self.fv_counts
            .keys()
            .map(|pair| pair.extract_field())
            .collect()
    }
}

/// A complete query histogram: one (request, response) pair per bucket,
/// in time order.
///
/// For engine output `buckets` is never empty - `QueryTimeRange` yields at
/// least one bucket ([`crate::QueryTimeRange::buckets`]) and every request
/// resolves to a response (see `compute_from_indexes`) - so the accessors'
/// `expect`s below cannot fire on it. journal-function walks the pairs
/// directly for the viewer's facets, charts and columns
/// (`journal-function/src/netdata/facets.rs` `facets`,
/// `journal-function/src/netdata/histogram.rs` `histogram` and
/// `journal-function/src/netdata/builder.rs` `build_ui_response`).
#[derive(Debug, Clone)]
pub struct Histogram {
    pub buckets: Vec<(BucketRequest, BucketResponse)>,
}

impl Histogram {
    /// Returns the start time of the histogram (first bucket's start time).
    pub fn start_time(&self) -> Seconds {
        let bucket_request = &self
            .buckets
            .first()
            .expect("histogram with at least one bucket")
            .0;
        bucket_request.start
    }

    /// Returns the end time of the histogram (last bucket's end time).
    pub fn end_time(&self) -> Seconds {
        let bucket_request = &self
            .buckets
            .last()
            .expect("histogram with at least one bucket")
            .0;
        bucket_request.end
    }

    /// Returns the duration of each bucket in seconds.
    pub fn bucket_duration(&self) -> Seconds {
        self.buckets
            .first()
            .expect("histogram with at least one bucket")
            .0
            .duration()
    }

    /// Every field seen in any bucket - indexed (from `fv_counts` keys) or
    /// unindexed - deduplicated and sorted. journal-function feeds this to the
    /// viewer's column schema (`journal-function/src/netdata/builder.rs` `build_ui_response`).
    pub fn discovered_fields(&self) -> Vec<FieldName> {
        let mut fields = HashSet::default();
        for (_, bucket_response) in &self.buckets {
            fields.extend(bucket_response.indexed_fields());
            fields.extend(bucket_response.unindexed_fields.iter().cloned());
        }

        let mut v: Vec<FieldName> = fields.into_iter().collect();
        v.sort();
        v
    }
}

/// Reusable response cache for bucket computations.
///
/// The engine holds an LRU of [`BucketResponse`]s keyed by
/// [`BucketRequest`] behind a `parking_lot` lock, making it `Send + Sync`
/// and shareable: otel-legacy-logs keeps a single engine in `Arc`-shared
/// handler state, so all concurrent queries hit the same cache
/// (`otel-legacy-logs/src/handler.rs` `LegacyLogsHandlerInner`). Counting runs outside the
/// lock; the lock guards only membership checks, inserts and the
/// LRU-updating reads that assemble the result.
/// `compute_from_indexes` is synchronous and runs on the caller's thread -
/// in the handler, on a runtime worker between the `spawn_blocking`
/// indexing and log-query steps
/// (`otel-legacy-logs/src/handler.rs` `LegacyLogsHandler::on_call`).
pub struct HistogramEngine {
    responses: RwLock<LruCache<BucketRequest, BucketResponse>>,
}

impl HistogramEngine {
    /// Creates a new HistogramEngine with a default capacity of 1000 bucket responses.
    pub fn new() -> Self {
        Self::with_capacity(1000)
    }

    /// Cache capacity in responses; the oldest entries are evicted LRU-first
    /// once full. Panics on zero (`expect`). No in-tree caller passes anything
    /// but the `new()` default of 1000.
    pub fn with_capacity(capacity: usize) -> Self {
        Self {
            responses: RwLock::new(LruCache::new(
                NonZeroUsize::new(capacity).expect("capacity must be non-zero"),
            )),
        }
    }

    /// Builds a [`Histogram`] over `time_range` from already-indexed files.
    ///
    /// Produces one [`BucketResponse`] per bucket of `time_range` (aligned
    /// windows from [`crate::QueryTimeRange::buckets`]) by counting, in every file that
    /// overlaps a bucket: total entries and per field=value pair entries,
    /// unfiltered and filtered; plus the file's non-facet fields. Callers
    /// normally pass the output of `batch_compute_file_indexes` built with
    /// this same `time_range`, whose cache gate keeps every index's histogram
    /// width a divisor of the query's (the gate in `batch_compute_file_indexes`) - so no index
    /// bucket straddles a query-bucket edge and the whole-bucket counting
    /// below equals counting by entry timestamp.
    ///
    /// The `Result` never carries an `Err`: nothing here is fallible. The
    /// `None` cases of `count_entries_in_time_range` (inverted or misaligned
    /// bounds, `journal-index/src/histogram.rs` `Histogram::count_entries_in_time_range`) are unreachable given
    /// the gate above and would coalesce to 0 via `unwrap_or` regardless; the
    /// type exists for symmetry with the crate's other fallible calls, and the
    /// caller's `map_err` (`otel-legacy-logs/src/handler.rs` `LegacyLogsHandler::on_call`) is dead in
    /// practice.
    ///
    /// Caching: a bucket touched by an online file is recomputed on every call
    /// (online indexes go stale within 1s,
    /// `journal-index/src/file_index.rs` `FileIndex::is_fresh`); a bucket whose overlapping
    /// files were all archived is cached and frozen until LRU eviction.
    /// Concurrent identical computations are not deduplicated - both run,
    /// results are equal.
    pub fn compute_from_indexes(
        &self,
        indexed_files: &[(FileIndexKey, FileIndex)],
        time_range: &crate::QueryTimeRange,
        facets: &[String],
        filter_expr: &Filter,
    ) -> Result<Histogram> {
        // One bucket request per grid window; each clones the shared facets and filter.
        let facets = Facets::new(facets);
        let bucket_requests: Vec<BucketRequest> = time_range
            .buckets()
            .map(|(start, end)| BucketRequest {
                start: Seconds(start),
                end: Seconds(end),
                facets: facets.clone(),
                filter_expr: filter_expr.clone(),
            })
            .collect();

        // Membership check under the read lock; counting below runs outside
        // any lock. Not deduplicated: two concurrent queries can both see the
        // same bucket missing and compute it - the results are equal and the
        // last insert wins.
        let buckets_to_compute: Vec<BucketRequest> = {
            let responses = self.responses.read();

            bucket_requests
                .iter()
                .filter(|br| !responses.contains(br))
                .cloned()
                .collect()
        };

        if !buckets_to_compute.is_empty() {
            let mut new_responses: HashMap<BucketRequest, BucketResponse> = buckets_to_compute
                .iter()
                .map(|br| (br.clone(), BucketResponse::new()))
                .collect();

            // Track which buckets can be cached (no online file contributions)
            let mut bucket_cacheable: HashMap<BucketRequest, bool> = buckets_to_compute
                .iter()
                .map(|br| (br.clone(), true))
                .collect();

            for (_, file_index) in indexed_files {
                let is_online = file_index.online();
                let file_start = file_index.start_time();
                let file_end = file_index.end_time();

                for bucket_request in &buckets_to_compute {
                    // Always present: both maps above come from buckets_to_compute
                    let response = match new_responses.get_mut(bucket_request) {
                        Some(r) => r,
                        None => continue,
                    };

                    // Files whose covered range [start_time, end_time) does not overlap the
                    // bucket window contribute nothing and are skipped. Besides saving work,
                    // the overlap is what keeps the counting below well-defined:
                    // count_entries_in_time_range selects whole index buckets by start time
                    // and its end-bucket search treats the first bucket as the end bucket
                    // when the query ends at or before it
                    // (`journal-index/src/histogram.rs` `Histogram::count_entries_in_time_range`) - unreachable here, since a
                    // file reaching this point starts before bucket.end.
                    if file_start >= bucket_request.end || file_end <= bucket_request.start {
                        continue;
                    }

                    // An online file overlapping the bucket makes it uncacheable: online
                    // indexes go stale within 1s
                    // (`journal-index/src/file_index.rs` `FileIndex::is_fresh`)
                    // and are re-indexed per query, so their contribution must be
                    // recomputed too. The overlap skip above runs first, so a
                    // non-overlapping online file leaves the bucket cacheable.
                    if is_online {
                        bucket_cacheable.insert(bucket_request.clone(), false);
                    }

                    // `Filter::none()` is the "no filter" sentinel, not "match nothing":
                    // evaluating it would return an empty bitmap
                    // (`journal-index/src/filter.rs` `Filter::evaluate`) and zero every filtered count.
                    // Skipping it leaves `filter_bitmap` None and every filtered count
                    // equal to its unfiltered count.
                    let filter_bitmap = if !bucket_request.filter_expr.is_none() {
                        Some(bucket_request.filter_expr.evaluate(file_index))
                    } else {
                        None
                    };

                    // Entries in the bucket window, unfiltered and filtered. The
                    // full-coverage bitmap below makes the unfiltered count purely the
                    // file histogram's own bucket-window total; with no filter the
                    // filtered total is the same number.
                    let all_entries = Bitmap::insert_range(0..file_index.total_entries() as u32);
                    let unfiltered_total = file_index
                        .count_entries_in_time_range(
                            &all_entries,
                            bucket_request.start,
                            bucket_request.end,
                        )
                        .unwrap_or(0);

                    let filtered_total = if let Some(ref filter_bitmap) = filter_bitmap {
                        file_index
                            .count_entries_in_time_range(
                                filter_bitmap,
                                bucket_request.start,
                                bucket_request.end,
                            )
                            .unwrap_or(0)
                    } else {
                        unfiltered_total
                    };

                    response.total_entries.0 += unfiltered_total;
                    response.total_entries.1 += filtered_total;

                    // Fields the file carries but that are outside the facet set: no
                    // bitmaps, so they surface only as names for the viewer's column
                    // schema. Journal field names are recorded unvalidated
                    // (`journal-index/src/file_indexer.rs` `FileIndexer::index`), so the `FieldName::new`
                    // guard below drops any that cannot round-trip (empty, or containing
                    // '=').
                    for field in file_index.fields() {
                        if !file_index.is_indexed(field) {
                            if let Some(field_name) = FieldName::new(field) {
                                response.unindexed_fields.insert(field_name);
                            }
                        }
                    }

                    for (indexed_field, field_bitmap) in file_index.bitmaps() {
                        let unfiltered_count = file_index
                            .count_entries_in_time_range(
                                field_bitmap,
                                bucket_request.start,
                                bucket_request.end,
                            )
                            .unwrap_or(0);

                        let filtered_count = if let Some(ref filter_bitmap) = filter_bitmap {
                            let filtered_bitmap = field_bitmap & filter_bitmap;
                            file_index
                                .count_entries_in_time_range(
                                    &filtered_bitmap,
                                    bucket_request.start,
                                    bucket_request.end,
                                )
                                .unwrap_or(0)
                        } else {
                            unfiltered_count
                        };

                        // Accumulate into the response. Each bitmap key is re-parsed as
                        // "field=value"; a key that cannot parse (empty field name)
                        // contributes nothing.
                        if let Some(pair) = FieldValuePair::parse(indexed_field) {
                            let counts = response.fv_counts.entry(pair).or_insert((0, 0));
                            counts.0 += unfiltered_count;
                            counts.1 += filtered_count;
                        }
                    }
                }
            }

            // Only cacheable buckets enter the LRU. A cached response is never
            // refreshed - there is no invalidation short of LRU eviction - so it
            // must hold archived-file counts only: archived files never change
            // (`journal-index/src/file_index.rs` `FileIndex::is_fresh`). A bucket no file overlapped
            // is cached empty; entries a later-growing online file brings into it
            // stay hidden until that response is evicted.
            let mut responses_guard = self.responses.write();
            for (bucket_request, response) in &new_responses {
                if bucket_cacheable
                    .get(bucket_request)
                    .copied()
                    .unwrap_or(false)
                {
                    responses_guard.put(bucket_request.clone(), response.clone());
                }
            }
            drop(responses_guard);

            // Build histogram from all responses (cached + newly computed non-cacheable)
            let mut responses_guard = self.responses.write();
            let buckets = bucket_requests
                .into_iter()
                .filter_map(|bucket_request| {
                    // Try to get from cache first (updates LRU), then from newly computed responses
                    let response = responses_guard
                        .get(&bucket_request)
                        .cloned()
                        .or_else(|| new_responses.get(&bucket_request).cloned());

                    response.map(|r| (bucket_request, r))
                })
                .collect();

            Ok(Histogram { buckets })
        } else {
            // All buckets were cached, just build histogram from cache
            let mut responses = self.responses.write();
            let buckets = bucket_requests
                .into_iter()
                .filter_map(|bucket_request| {
                    // Use get() to update LRU order for accessed entries
                    responses
                        .get(&bucket_request)
                        .map(|response| (bucket_request, response.clone()))
                })
                .collect();

            Ok(Histogram { buckets })
        }
    }
}
