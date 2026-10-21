//! The span explorer: one request answers the span view's histogram
//! (spans per time bucket stacked by a key, the error count, the window's
//! duration percentiles and a duration heatmap) and its rows (the newest
//! matching spans, paged) over the stored spans themselves — one row per
//! stored span, no trace assembly. Consumer: the `explore` mode of the
//! otel-traces Function, shaped by the ledger's adapter.
//!
//! Pinned semantics:
//!
//! - **Spans, not traces**: every stored span that starts inside the
//!   grid's range counts once; a resent span counts each time it is
//!   stored, as the overview's span totals do.
//! - **Tokens per span**: a positive token holds for a span that
//!   satisfies its group, a negated token for a span that does not (the
//!   span reading of the search's NOT EXISTS), and tokens AND on the same
//!   span. A `trace_id` token selects the spans of the given traces (`!=`:
//!   the spans of other traces). Trace-level keys have no span meaning
//!   and are refused ([`ExploreRequestError::TraceLevelToken`]).
//! - **One range**: every section counts and lists over the grid's range,
//!   so the rows' matched count equals the histogram's total.
//! - **Every source one way**: sealed files and chunk images through the
//!   index reader, a WAL tail built into an image with the seal's own
//!   builder, so tokens evaluate identically everywhere.
//! - **Stack**: the [`STACK_DIMENSIONS_MAX`] values with the most spans,
//!   the rest summed into `other`, spans without the key into `unset`. A
//!   span carrying several values of the key counts under each. A file
//!   where the key is high-cardinality cannot be stacked: its spans count
//!   in `other` and the answer is partial
//!   [`StackFieldHighCard`](PartialReason::StackFieldHighCard).
//! - **Rows**: newest first by [`RowKey`] (start, trace id, span id), a
//!   strict total order, so pages never split or repeat a span; a walk
//!   continues below the last key it was given.
//! - **One budget**: sources are visited newest first; plan compiles,
//!   duration reads and row candidates count into one rows budget, and
//!   once it runs out the older sources are left out
//!   ([`WorkCeiling`](PartialReason::WorkCeiling)).
//!
//! Engine contracts as in the sibling operations: a source that fails is
//! a [`SourceFailure`](PartialReason::SourceFailure), an unavailable one
//! a [`RemoteUnavailable`](PartialReason::RemoteUnavailable) (the rest
//! still count), and cancellation is all-or-empty.

use std::sync::Arc;
use std::sync::atomic::{AtomicUsize, Ordering};

use tokio_util::sync::CancellationToken;

use super::duration_hist::{DurationHistogram, DurationPercentiles, HEATMAP_ROWS};
use super::order::newest_first;
use super::overview::top_k;
use super::predicate::{PredicateError, TokenKind, TokenQuery};
use super::sources::{SourceSetError, TraceSource, validate_sources};
use super::status::{PartialReason, QueryStatus, StatusBuilder};
use super::vocab::{AttributeKey, KeyError, parse_key};
use super::window::{TimeWindow, WindowError};
use crate::logs::merge::merge_timelines;
use crate::source::{Mapped, map_source};

/// The most rows one page lists.
pub const ROWS_LIMIT_MAX: usize = 200;
/// The rows a page lists when the request does not say.
pub const DEFAULT_ROWS_LIMIT: usize = 100;
/// The most columns a row carries.
pub const ROW_COLUMNS_MAX: usize = 32;
/// The most stack values the histogram names; the rest are `other`.
pub const STACK_DIMENSIONS_MAX: usize = 20;
/// The rows budget of one request (stream-batch rows scanned, positions
/// emitted, row candidates): the overview's fold budget, since the
/// explorer is polled like the list.
const ROWS_CEILING: u64 = 4_000_000;
/// The dictionary-bytes budget (the search's value and reasoning: a
/// free-text term over a day of files).
const DICT_BYTES_CEILING: u64 = 512 << 20;

/// A span's place in the newest-first row order: start, then trace id,
/// then span id. Unique per stored span except for exact resends.
#[derive(Debug, Clone, Copy, PartialEq, Eq, PartialOrd, Ord, Hash)]
pub struct RowKey {
    pub start_ns: i64,
    pub trace_id: sfst::TraceId,
    pub span_id: sfst::SpanId,
}

/// The rows section of a request.
#[derive(Debug, Clone, PartialEq, Eq)]
pub struct RowsSpec {
    /// At most [`ROWS_LIMIT_MAX`].
    pub limit: usize,
    /// Continue a walk: only rows ordered below this key.
    pub before: Option<RowKey>,
    /// Keys whose stored values each row carries, in order; at most
    /// [`ROW_COLUMNS_MAX`], each a key with stored values.
    pub columns: Vec<String>,
}

/// An explorer request over the exact time grid (the consumer owns bucket
/// geometry, as for the overview).
#[derive(Debug, Clone)]
pub struct ExploreQuery {
    grid: sfst::Grid,
    tokens: TokenQuery,
    stack: Option<String>,
    rows: Option<RowsSpec>,
    rows_ceiling: u64,
}

impl ExploreQuery {
    /// No sections: add [`histogram`](Self::histogram) and/or
    /// [`rows`](Self::rows).
    pub fn new(grid: sfst::Grid) -> Self {
        Self {
            grid,
            tokens: TokenQuery::default(),
            stack: None,
            rows: None,
            rows_ceiling: ROWS_CEILING,
        }
    }

    /// Count only the spans the tokens select (module docs).
    pub fn tokens(mut self, tokens: impl Into<TokenQuery>) -> Self {
        self.tokens = tokens.into();
        self
    }

    /// Answer the histogram, stacked by `stack`, a key with stored values.
    pub fn histogram(mut self, stack: impl Into<String>) -> Self {
        self.stack = Some(stack.into());
        self
    }

    /// Answer a rows page.
    pub fn rows(mut self, spec: RowsSpec) -> Self {
        self.rows = Some(spec);
        self
    }

    /// Test-only budget override — proves ceiling termination without a
    /// four-million-row corpus. NOT a tuning surface.
    #[doc(hidden)]
    pub fn rows_ceiling_for_tests(mut self, ceiling: u64) -> Self {
        self.rows_ceiling = ceiling;
        self
    }
}

/// An explorer request error — nothing was queried.
#[derive(Debug, thiserror::Error)]
pub enum ExploreRequestError {
    #[error("the grid is empty (zero buckets or non-positive width)")]
    EmptyGrid,
    #[error("the grid's end overflows i64 (width x buckets past the epoch range)")]
    GridOverflow,
    #[error(transparent)]
    SourceSet(#[from] SourceSetError),
    #[error(transparent)]
    Predicate(#[from] PredicateError),
    #[error(transparent)]
    Window(#[from] WindowError),
    /// A trace-level key: it describes a whole trace, so it cannot select
    /// spans.
    #[error("{target} describes a whole trace and cannot select spans")]
    TraceLevelToken { target: String },
    /// A stack or column key that is not a key at all.
    #[error("{key:?} is not a key")]
    UnknownKey { key: String },
    /// A stack or column key with no stored values (a virtual builtin
    /// such as `duration` or `trace_id`).
    #[error("{key} has no stored values to stack or list")]
    NoStoredValues { key: String },
    #[error("rows limit {got} is outside 1..={max}", max = ROWS_LIMIT_MAX)]
    RowsLimit { got: usize },
    #[error("{got} row columns exceed the limit of {max}", max = ROW_COLUMNS_MAX)]
    TooManyColumns { got: usize },
}

/// One explorer answer: the sections the request asked for.
#[derive(Debug, Clone, PartialEq, Eq)]
pub struct ExploreData {
    pub status: QueryStatus,
    pub histogram: Option<HistogramData>,
    pub rows: Option<RowsData>,
}

/// The spans per grid bucket.
#[derive(Debug, Clone, PartialEq, Eq)]
pub struct HistogramData {
    /// The stack's named values, most spans first.
    pub dimensions: Vec<String>,
    /// One per grid bucket, in time order.
    pub buckets: Vec<HistogramBucket>,
    /// Every counted span (the sum over buckets of the spans, each counted
    /// once).
    pub total: u64,
    /// Of those, the `ERROR`-status ones.
    pub errors: u64,
    /// The window's duration percentiles; `None` when no span counted.
    pub percentiles: Option<DurationPercentiles>,
}

/// One grid bucket of the histogram.
#[derive(Debug, Clone, PartialEq, Eq)]
pub struct HistogramBucket {
    /// Spans per named dimension, parallel to
    /// [`HistogramData::dimensions`].
    pub counts: Vec<u64>,
    /// Spans with an unnamed value, or from a file where the key is
    /// high-cardinality.
    pub other: u64,
    /// Spans without the key.
    pub unset: u64,
    /// Spans per duration heatmap row (`heatmap_row_bounds`).
    pub durations: [u64; HEATMAP_ROWS],
}

/// One rows page.
#[derive(Debug, Clone, PartialEq, Eq)]
pub struct RowsData {
    /// Every span in scope, whatever the page (equals the histogram's
    /// total).
    pub matched: u64,
    /// Newest first.
    pub items: Vec<SpanRow>,
    /// Whether rows below the page's last key exist.
    pub more: bool,
}

/// One listed span.
#[derive(Debug, Clone, PartialEq, Eq)]
pub struct SpanRow {
    pub key: RowKey,
    pub duration_ns: i64,
    /// The stored values of each requested column, parallel to
    /// [`RowsSpec::columns`]: empty when the span lacks the key.
    pub columns: Vec<Vec<String>>,
}

/// Run an explorer request. See the module docs for the pinned semantics.
/// `progress` ticks once per source; callers that don't report pass a
/// fresh counter. Pure sync — invoke off any async runtime thread.
pub fn explore(
    mut sources: Vec<TraceSource>,
    query: ExploreQuery,
    cancel: CancellationToken,
    progress: Arc<AtomicUsize>,
) -> Result<ExploreData, ExploreRequestError> {
    let grid = query.grid;
    if grid.num_buckets == 0 || grid.bucket_width_ns <= 0 {
        return Err(ExploreRequestError::EmptyGrid);
    }
    let Some(grid_end) = grid
        .bucket_width_ns
        .checked_mul(grid.num_buckets as i64)
        .and_then(|span| grid.bucket_start_ns.checked_add(span))
    else {
        return Err(ExploreRequestError::GridOverflow);
    };
    let window = TimeWindow::new(grid.bucket_start_ns, grid_end)?;
    validate_sources(&sources)?;
    let scope = Scope::new(&query.tokens)?;
    let stack = query.stack.as_deref().map(stored_key).transpose()?;
    if let Some(rows) = &query.rows {
        if rows.limit == 0 || rows.limit > ROWS_LIMIT_MAX {
            return Err(ExploreRequestError::RowsLimit { got: rows.limit });
        }
        if rows.columns.len() > ROW_COLUMNS_MAX {
            return Err(ExploreRequestError::TooManyColumns {
                got: rows.columns.len(),
            });
        }
        for column in &rows.columns {
            stored_key(column)?;
        }
    }

    newest_first(&mut sources);
    let request = Request {
        scope: &scope,
        range: grid.bucket_start_ns..grid_end,
        grid,
        stack: stack.as_deref(),
        rows: query.rows.as_ref(),
        limits: sfst::ScanLimits {
            rows: query.rows_ceiling,
            dict_bytes: DICT_BYTES_CEILING,
        },
    };
    let mut work = sfst::ScanWork::default();
    let mut status = StatusBuilder::new();
    let mut fold = Fold::new(grid);
    // The sources that list row candidates, kept mapped for the page's
    // column read; a candidate names its source by index here.
    let mut row_sources: Vec<Mapped> = Vec::new();
    for source in &sources {
        if cancel.is_cancelled() {
            return Ok(ExploreData::cancelled());
        }
        let mapped = match prepare(source, &window) {
            Prepared::Outside => None,
            Prepared::Unavailable => {
                status.add(PartialReason::RemoteUnavailable);
                None
            }
            Prepared::Failed(error) => {
                tracing::warn!(
                    "sfsq explore: source {} failed: {error}",
                    source.source_id()
                );
                status.add(PartialReason::SourceFailure);
                None
            }
            Prepared::Open(mapped) => Some(mapped),
        };
        if let Some(mapped) = mapped {
            match evaluate(mapped.bytes(), &request, row_sources.len(), &mut work) {
                Ok(Some(shard)) => {
                    if shard.rows.as_ref().is_some_and(|r| !r.rows.is_empty()) {
                        row_sources.push(mapped);
                    }
                    fold.add(shard);
                }
                Ok(None) => {
                    status.add(PartialReason::WorkCeiling);
                    break;
                }
                Err(error) => {
                    tracing::warn!(
                        "sfsq explore: source {} failed: {error}",
                        source.source_id()
                    );
                    status.add(PartialReason::SourceFailure);
                }
            }
        }
        progress.fetch_add(1, Ordering::Relaxed);
    }
    if fold.stack_high {
        status.add(PartialReason::StackFieldHighCard);
    }

    let histogram = stack.as_ref().map(|_| fold.histogram());
    let rows = match &query.rows {
        Some(spec) => match fold.page(spec, &row_sources) {
            Ok(rows) => Some(rows),
            Err(error) => {
                tracing::warn!("sfsq explore: reading the page's columns failed: {error}");
                status.add(PartialReason::SourceFailure);
                Some(RowsData {
                    matched: fold.matched,
                    items: Vec::new(),
                    more: fold.below > 0,
                })
            }
        },
        None => None,
    };
    Ok(ExploreData {
        status: status.finish(),
        histogram,
        rows,
    })
}

impl ExploreData {
    fn cancelled() -> Self {
        let mut status = StatusBuilder::new();
        status.add(PartialReason::Cancelled);
        Self {
            status: status.finish(),
            histogram: None,
            rows: None,
        }
    }
}

/// The storage field of a stack or column key: the key itself, for an
/// attribute or a builtin with stored values.
fn stored_key(key: &str) -> Result<String, ExploreRequestError> {
    match parse_key(key) {
        Ok((_, AttributeKey::Attribute(_))) => Ok(key.to_string()),
        Ok((_, AttributeKey::Builtin(b))) => {
            b.dictionary_field().map(str::to_string).ok_or_else(|| {
                ExploreRequestError::NoStoredValues {
                    key: key.to_string(),
                }
            })
        }
        Err(KeyError::EmptyAttribute | KeyError::Unknown) => Err(ExploreRequestError::UnknownKey {
            key: key.to_string(),
        }),
    }
}

/// The request's tokens lowered once, compiled per file.
struct Scope {
    /// Compiled together per file: every row (the empty plan), then each
    /// positive token's group, then each negated token's group, then the
    /// `ERROR` status.
    plans: Vec<sfst::TracePlan>,
    positive: usize,
    negated: usize,
    /// `trace_id` tokens: the ids, and whether a span must carry one of
    /// them (`true`) or none (`false`).
    trace_ids: Vec<(Vec<sfst::TraceId>, bool)>,
}

impl Scope {
    fn new(tokens: &TokenQuery) -> Result<Self, ExploreRequestError> {
        tokens.validate()?;
        let mut positive = Vec::new();
        let mut negated = Vec::new();
        let mut trace_ids = Vec::new();
        for token in &tokens.tokens {
            match token.kind() {
                TokenKind::SpanLocal => {
                    let plan = token.group.to_trace_plan();
                    if token.negated {
                        negated.push(plan);
                    } else {
                        positive.push(plan);
                    }
                }
                TokenKind::TraceId => {
                    let (_, constraints) = token.group.split_trace_id_conditions();
                    let (ids, carries) = match constraints.pins {
                        Some(pins) => (pins, true),
                        None => (constraints.excluded, false),
                    };
                    trace_ids.push((ids.into_iter().collect(), carries != token.negated));
                }
                TokenKind::TraceLevel => {
                    return Err(ExploreRequestError::TraceLevelToken {
                        target: token.group.conditions[0].target.to_string(),
                    });
                }
            }
        }
        let errors = sfst::TracePlan {
            terms: vec![sfst::PlanTerm::tokens(
                super::vocab::BuiltinField::Status.key(),
                vec!["ERROR".to_string()],
                Vec::new(),
            )],
        };
        let (positive_n, negated_n) = (positive.len(), negated.len());
        let mut plans = Vec::with_capacity(2 + positive_n + negated_n);
        plans.push(sfst::TracePlan::default());
        plans.extend(positive);
        plans.extend(negated);
        plans.push(errors);
        Ok(Self {
            plans,
            positive: positive_n,
            negated: negated_n,
            trace_ids,
        })
    }
}

/// What one source contributes to a request.
enum Prepared {
    Open(Mapped),
    /// Holds data for the window but cannot be read.
    Failed(String),
    /// A remote file whose bytes could not be obtained.
    Unavailable,
    /// Holds no data for the window.
    Outside,
}

/// Map a source, or build a WAL tail into an image with the seal's own
/// builder, so it evaluates exactly like a sealed file.
fn prepare(source: &TraceSource, window: &TimeWindow) -> Prepared {
    let overlaps = |summary: &sfst::Summary| {
        window.overlaps_summary(summary.min_timestamp_s, summary.max_timestamp_s)
    };
    match source {
        TraceSource::Sfst(candidate) => {
            if !overlaps(&candidate.summary) {
                return Prepared::Outside;
            }
            match map_source(&candidate.source) {
                Ok(mapped) => Prepared::Open(mapped),
                Err(error) => Prepared::Failed(error.to_string()),
            }
        }
        TraceSource::Tail(tail) => {
            match ng_index::build_sfst_traces_range(&tail.path, tail.coverage.range) {
                Ok((summary, _)) if !overlaps(&summary) => Prepared::Outside,
                Ok((_, bytes)) => Prepared::Open(Mapped::Memory(Arc::new(bytes))),
                Err(error) => Prepared::Failed(format!("building the tail image: {error}")),
            }
        }
        TraceSource::Unavailable(unavailable) => {
            if overlaps(&unavailable.summary) {
                Prepared::Unavailable
            } else {
                Prepared::Outside
            }
        }
    }
}

/// One source's numbers for the request.
struct Shard {
    matched: u64,
    errors: u64,
    /// Per bucket, stacked; `None` when the key is high-cardinality here
    /// (see `high`) or no histogram was asked for.
    timeline: Option<sfst::Timeline>,
    /// Per bucket, the spans of a source that cannot stack.
    high: Option<Vec<u64>>,
    /// Per bucket, the span durations.
    durations: Vec<DurationHistogram>,
    rows: Option<SourceRows>,
}

/// A source's row candidates: its newest rows below the walk's key (at
/// most the page's limit), and how many rows below that key it holds.
struct SourceRows {
    below: u64,
    rows: Vec<Candidate>,
}

struct Candidate {
    key: RowKey,
    duration_ns: i64,
    /// Which source (index into the kept mappings) and where in it.
    source: usize,
    position: u32,
}

/// What every source of a request is evaluated against.
struct Request<'a> {
    scope: &'a Scope,
    /// The grid's range, `[start, end)` nanoseconds.
    range: std::ops::Range<i64>,
    grid: sfst::Grid,
    /// The stack's storage field, when the histogram is asked for.
    stack: Option<&'a str>,
    rows: Option<&'a RowsSpec>,
    limits: sfst::ScanLimits,
}

/// Evaluate one source; `source` is the index its row candidates name.
/// Any error drops the whole source; the caller reports it. `Ok(None)`:
/// the request's budget ran out.
fn evaluate(
    bytes: &[u8],
    request: &Request<'_>,
    source: usize,
    work: &mut sfst::ScanWork,
) -> Result<Option<Shard>, sfst::Error> {
    let (scope, grid, limits) = (request.scope, request.grid, request.limits);
    let reader = sfst::IndexReader::open(bytes)?;
    let timestamps = reader.load_timestamps()?;
    let (lo, hi) = timestamps.window(request.range.clone());
    let mut shard = Shard {
        matched: 0,
        errors: 0,
        timeline: None,
        high: None,
        durations: Vec::new(),
        rows: None,
    };
    if lo >= hi {
        return Ok(Some(shard));
    }
    let Some(compiled) = reader.compile_trace_plans(&scope.plans, (lo, hi), limits, work)? else {
        return Ok(None);
    };
    let mut compiled = compiled.into_iter();
    let mut set = compiled.next().expect("the every-row plan");
    for _ in 0..scope.positive {
        set.and_assign(&compiled.next().expect("a positive token's plan"));
    }
    for _ in 0..scope.negated {
        set.and_not_assign(&compiled.next().expect("a negated token's plan"));
    }
    for (ids, carries) in &scope.trace_ids {
        let traces = reader.compile_trace_ids(ids, (lo, hi))?;
        if *carries {
            set.and_assign(&traces);
        } else {
            set.and_not_assign(&traces);
        }
    }
    let mut errors = compiled.next().expect("the errors plan");
    errors.and_assign(&set);
    shard.matched = set.count_in_range(lo, hi);
    shard.errors = errors.count_in_range(lo, hi);

    if let Some(field) = request.stack {
        let filter = set.to_filter();
        match reader.timeline(field, &filter, grid) {
            Ok(timeline) => shard.timeline = Some(timeline),
            Err(sfst::Error::HighCardFacet(_)) => {
                shard.high = Some(reader.timeline_totals(&filter, grid)?);
            }
            Err(error) => return Err(error),
        }
        if work.rows_visited.saturating_add(shard.matched) > limits.rows {
            return Ok(None);
        }
        let positions = set.matched_in_range(lo, hi, work);
        let durations = reader.durations()?;
        shard.durations = (0..grid.num_buckets)
            .map(|_| DurationHistogram::new())
            .collect();
        let mut next = 0;
        for (bucket, (bucket_lo, bucket_hi)) in
            timestamps.bucket_ranges(grid).into_iter().enumerate()
        {
            while next < positions.len() && positions[next] < bucket_lo {
                next += 1;
            }
            while next < positions.len() && positions[next] < bucket_hi {
                shard.durations[bucket].record(durations.0[positions[next] as usize]);
                next += 1;
            }
        }
    }

    if let Some(spec) = request.rows {
        let rows = source_rows(&reader, &timestamps, &set, (lo, hi), spec, source, work)?;
        if work.rows_visited > limits.rows {
            return Ok(None);
        }
        shard.rows = Some(rows);
    }
    Ok(Some(shard))
}

/// A source's row candidates (see [`SourceRows`]). Positions are in time
/// order, but rows sharing a start order by trace and span id, so every
/// row at the anchor's start and at the oldest candidate's start is a
/// candidate too: the key, not the position, decides the cut.
fn source_rows(
    reader: &sfst::IndexReader<'_>,
    timestamps: &sfst::Timestamps,
    set: &sfst::CompiledTracePlan,
    (lo, hi): (u32, u32),
    spec: &RowsSpec,
    source: usize,
    work: &mut sfst::ScanWork,
) -> Result<SourceRows, sfst::Error> {
    let starts = timestamps.as_slice();
    let trace_ids = reader.trace_ids()?;
    let span_ids = reader.span_ids()?;
    let durations = reader.durations()?;
    let key_at = |position: u32| RowKey {
        start_ns: starts[position as usize],
        trace_id: trace_ids.get(position as usize),
        span_id: span_ids.get(position as usize),
    };
    let at_start = |start_ns: i64| -> (u32, u32) {
        let (a, z) = timestamps.window(start_ns..start_ns.saturating_add(1));
        (a.clamp(lo, hi), z.clamp(lo, hi))
    };

    let mut positions = Vec::new();
    let mut below = 0;
    let older_end = match spec.before {
        None => hi,
        Some(before) => {
            let (tie_lo, tie_hi) = at_start(before.start_ns);
            for position in set.matched_in_range(tie_lo, tie_hi, work) {
                if key_at(position) < before {
                    positions.push(position);
                    below += 1;
                }
            }
            tie_lo
        }
    };
    below += set.count_in_range(lo, older_end);
    let newest = set.newest_in_range(lo, older_end, spec.limit, work);
    if let Some(&oldest) = newest.first() {
        let (tie_lo, _) = at_start(starts[oldest as usize]);
        positions.extend(set.matched_in_range(tie_lo, oldest, work));
    }
    positions.extend(newest);
    let mut rows: Vec<Candidate> = positions
        .into_iter()
        .map(|position| Candidate {
            key: key_at(position),
            duration_ns: durations.0[position as usize],
            source,
            position,
        })
        .collect();
    rows.sort_unstable_by_key(|row| std::cmp::Reverse(row.key));
    rows.truncate(spec.limit);
    Ok(SourceRows { below, rows })
}

/// The answer folded over the evaluated sources.
struct Fold {
    grid: sfst::Grid,
    matched: u64,
    errors: u64,
    timelines: Vec<sfst::Timeline>,
    /// Per bucket, the spans of the sources that cannot stack.
    high: Vec<u64>,
    stack_high: bool,
    durations: Vec<DurationHistogram>,
    below: u64,
    rows: Vec<Candidate>,
}

impl Fold {
    fn new(grid: sfst::Grid) -> Self {
        Self {
            grid,
            matched: 0,
            errors: 0,
            timelines: Vec::new(),
            high: vec![0; grid.num_buckets],
            stack_high: false,
            durations: (0..grid.num_buckets)
                .map(|_| DurationHistogram::new())
                .collect(),
            below: 0,
            rows: Vec::new(),
        }
    }

    fn add(&mut self, shard: Shard) {
        self.matched += shard.matched;
        self.errors += shard.errors;
        if let Some(timeline) = shard.timeline {
            self.timelines.push(timeline);
        }
        if let Some(high) = shard.high {
            self.stack_high = true;
            for (sum, count) in self.high.iter_mut().zip(high) {
                *sum += count;
            }
        }
        for (sum, histogram) in self.durations.iter_mut().zip(&shard.durations) {
            sum.merge(histogram);
        }
        if let Some(rows) = shard.rows {
            self.below += rows.below;
            self.rows.extend(rows.rows);
        }
    }

    fn histogram(&mut self) -> HistogramData {
        let merged = merge_timelines(std::mem::take(&mut self.timelines));
        let num_buckets = self.grid.num_buckets;
        let (dimensions, stacked): (Vec<String>, Vec<sfst::Bucket>) = match merged {
            Some(timeline) => (timeline.dimensions, timeline.buckets),
            None => (Vec::new(), Vec::new()),
        };
        // Name the values with the most spans; the rest are `other`. A
        // file's timeline lists every value it holds, matched or not, so
        // values without spans are dropped first.
        let totals: Vec<(String, u64)> = dimensions
            .iter()
            .enumerate()
            .map(|(d, value)| (value.clone(), stacked.iter().map(|b| b.counts[d]).sum()))
            .filter(|&(_, total)| total > 0)
            .collect();
        let (named, _) = top_k(totals, STACK_DIMENSIONS_MAX);
        let index: std::collections::HashMap<&str, usize> = dimensions
            .iter()
            .enumerate()
            .map(|(i, d)| (d.as_str(), i))
            .collect();
        let named_index: Vec<usize> = named.iter().map(|(d, _)| index[d.as_str()]).collect();
        let mut window = DurationHistogram::new();
        let buckets = (0..num_buckets)
            .map(|b| {
                window.merge(&self.durations[b]);
                let (counts, other, unset) = match stacked.get(b) {
                    Some(bucket) => {
                        let counts: Vec<u64> =
                            named_index.iter().map(|&d| bucket.counts[d]).collect();
                        let other = bucket.counts.iter().sum::<u64>() - counts.iter().sum::<u64>();
                        (counts, other, bucket.unset)
                    }
                    None => (vec![0; named.len()], 0, 0),
                };
                HistogramBucket {
                    counts,
                    other: other + self.high[b],
                    unset,
                    durations: self.durations[b].heatmap_rows(),
                }
            })
            .collect();
        HistogramData {
            dimensions: named.into_iter().map(|(d, _)| d).collect(),
            buckets,
            total: self.matched,
            errors: self.errors,
            percentiles: window.percentiles(),
        }
    }

    /// The page: the newest candidates over every source, with their
    /// columns read from the sources that list them.
    fn page(&mut self, spec: &RowsSpec, sources: &[Mapped]) -> Result<RowsData, sfst::Error> {
        let mut rows = std::mem::take(&mut self.rows);
        rows.sort_unstable_by_key(|row| std::cmp::Reverse(row.key));
        rows.truncate(spec.limit);
        let more = self.below > rows.len() as u64;
        let fields: Vec<&str> = spec.columns.iter().map(String::as_str).collect();
        let mut columns: Vec<Vec<Vec<String>>> = vec![Vec::new(); rows.len()];
        let mut by_source: Vec<usize> = rows.iter().map(|r| r.source).collect();
        by_source.sort_unstable();
        by_source.dedup();
        for source in by_source {
            let reader = sfst::IndexReader::open(sources[source].bytes())?;
            let slots: Vec<usize> = (0..rows.len())
                .filter(|&i| rows[i].source == source)
                .collect();
            let positions: Vec<u32> = slots.iter().map(|&i| rows[i].position).collect();
            let values = reader.materialize_fields(&fields, &positions)?;
            for (at, &slot) in slots.iter().enumerate() {
                columns[slot] = values.iter().map(|column| column[at].clone()).collect();
            }
        }
        let items = rows
            .into_iter()
            .zip(columns)
            .map(|(row, columns)| SpanRow {
                key: row.key,
                duration_ns: row.duration_ns,
                columns,
            })
            .collect();
        Ok(RowsData {
            matched: self.matched,
            items,
            more,
        })
    }
}
