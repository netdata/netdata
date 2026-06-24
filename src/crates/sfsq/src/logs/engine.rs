//! Composition: the all-in-one local query.
//!
//! [`run`] ties the two steps together for the local case: it evaluates
//! every candidate into a [`LogsShard`](super::LogsShard) (step 1, see
//! [`aggregate`](super::aggregate)), merges the shards, then paginates and
//! materializes a page (step 2, see [`page`](super::page)), and assembles
//! a single [`LogsData`].
//!
//! It opens each file once for step 1 and again for step 2; the re-open is
//! deliberate — step 1's shards are fully owned and drop their readers, and
//! the heavy work is the bounded page materialization, not the re-read.
//!
//! The caller supplies a fully-specified [`LogsQuery`] — including the
//! histogram [`grid`](LogsQuery::grid), whose span is the query window —
//! and selects the candidates whose range overlaps that window. The work
//! is pure and synchronous — no I/O scheduling, no locks, no
//! window/geometry policy — but since it reads and decompresses files the
//! caller is expected to invoke it off any async runtime thread.

use std::path::PathBuf;
use std::sync::Arc;
use std::sync::atomic::{AtomicUsize, Ordering};

use tokio_util::sync::CancellationToken;

use super::aggregate::LogsShard;
use super::cursor::Part;
use super::mmap::{self, Mapped};
use super::page::paginate;
use super::query::LogsQuery;
use super::result::LogsData;
use super::wal_scan::WalScan;

/// Where an SFST candidate's bytes come from.
///
/// `File` is the steady-state case — a sealed index on disk, memory-
/// mapped lazily. `Memory` is an in-memory SFST built from a chunk of an
/// active WAL (`sfst_indexer::index_range`); the bytes are shared (`Arc`) so a
/// query holds them alive even if the producing cache evicts the entry
/// mid-query.
#[derive(Clone)]
pub enum Source {
    File(PathBuf),
    Memory(Arc<Vec<u8>>),
}

impl Source {
    /// A short label for log/error context.
    pub(super) fn describe(&self) -> std::borrow::Cow<'_, str> {
        match self {
            Source::File(p) => p.display().to_string().into(),
            Source::Memory(_) => "<in-memory chunk>".into(),
        }
    }
}

/// A query candidate: an SFST whose time range overlaps the request
/// window — either a sealed on-disk file or an in-memory chunk built
/// from an active WAL's durable prefix. Owned, so the caller can release
/// any lock on the file source before the query does I/O.
///
/// `file_seq` and `part` together place this candidate's rows in the
/// pagination cursor's total order (see [`Cursor`](super::cursor::Cursor)).
pub struct SfstCandidate {
    /// Cheap time/stream/size facts ([`sfst::Summary`]); its `[min, max]`
    /// second-range is what overlapped the request window to make this a
    /// candidate.
    pub summary: sfst::Summary,
    /// Globally-unique sequence of the underlying file — the sealed
    /// SFST's own seq, or the active WAL's seq for an in-memory chunk
    /// (so all chunks of one WAL share it). The cursor's second key
    /// ([`Cursor::file_seq`](super::cursor::Cursor::file_seq)).
    pub file_seq: u64,
    /// Which indexed sub-source of `file_seq` this is — always a
    /// [`Part::Indexed`]: `Indexed(0)` for a sealed SFST, or
    /// `Indexed(chunk index)` for an in-memory chunk. The cursor's third
    /// key, breaking ties at equal `(timestamp, seq)`. (An SFST candidate
    /// is never [`Part::Tail`]; that is the row-scanned [`WalTail`].)
    pub part: Part,
    /// Where the candidate's bytes come from — [`Source::File`] for a
    /// sealed index, [`Source::Memory`] for an in-memory WAL chunk.
    pub source: Source,
}

/// A byte range of an active WAL whose log records have not been indexed
/// into an SFST — the sub-chunk *tail*. Evaluated by a row scan
/// ([`WalScan`]) rather than the SFST engine. Bounded (< one chunk) by
/// construction, so re-scanning it per query is affordable.
pub struct WalTail {
    /// The active WAL's sequence — the same globally-unique id the
    /// pagination cursor orders by as `file_seq`. The tail's rows sort
    /// under it with [`Part::Tail`](super::cursor::Part::Tail), after
    /// every chunk of the same `file_seq`.
    pub file_seq: u64,
    /// Path to the active WAL file to scan.
    pub path: PathBuf,
    /// The un-indexed byte range to scan — from the end of the last
    /// indexed chunk (or `HEADER_SIZE` if none) to the WAL's durable bound
    /// (`valid_up_to`). Both ends are frame boundaries; the half-open
    /// `[start, end)` rule means a torn trailing frame past the bound is
    /// never read. See [`wal::FrameRange`].
    pub range: wal::FrameRange,
}

/// A source of log rows for a query: an SFST — a sealed on-disk file or
/// an in-memory chunk of an active WAL, evaluated through the indexed
/// engine — or an active WAL's row-scanned [`tail`](WalTail). [`run`]
/// folds every source into the same merged result, so the caller passes
/// one mixed list rather than pre-bucketing by kind.
pub enum LogSource {
    /// An indexed source evaluated through the SFST engine. The name
    /// tracks the inner [`SfstCandidate`] type, not on-disk storage: the
    /// bytes are a sealed on-disk file *or* an in-memory chunk of an active
    /// WAL ([`Source`]) — both are queried as SFSTs.
    Sfst(SfstCandidate),
    /// An active WAL's un-indexed tail, evaluated by a row scan.
    Tail(WalTail),
}

impl LogSource {
    /// Evaluate this source into a statistics shard (step 1): the indexed
    /// engine for an SFST (sealed file or in-memory chunk), a row scan for
    /// a WAL tail. An SFST reads the bytes `run` mapped once up front
    /// (`mapped`; `None` means the map failed — already logged — and
    /// degrades to empty). A per-source failure is logged and degrades to
    /// an empty shard — the monoid identity under [`LogsShard::merge`] —
    /// so one bad source never sinks the merged result.
    pub(super) fn to_shard(&self, query: &LogsQuery, mapped: Option<&Mapped>) -> LogsShard {
        match self {
            LogSource::Sfst(c) => match mapped {
                Some(m) => LogsShard::evaluate_mapped(c, m, query),
                None => LogsShard::default(),
            },
            LogSource::Tail(tail) => match WalScan::scan_range(&tail.path, tail.range) {
                Ok(scan) => scan.evaluate(query),
                Err(e) => {
                    tracing::warn!(
                        "sfsq: WAL tail scan failed for {} [{}..{}]: {e}",
                        tail.path.display(),
                        tail.range.start(),
                        tail.range.end()
                    );
                    LogsShard::default()
                }
            },
        }
    }
}

/// Run the merged query over the query's log sources.
///
/// Evaluates every source into a [`LogsShard`] (step 1), merges them,
/// then paginates and materializes a page (step 2), and assembles the
/// [`LogsData`]. The grid's span is the window every count and the
/// materialized page clip to.
///
/// Per-source errors (corrupt file, missing field, unreadable WAL tail,
/// etc.) are logged and that source is skipped — others still
/// contribute. An empty source set (or one where everything fails)
/// yields an empty `LogsData` aligned to the grid (the monoid identity).
///
/// Statistics (matched, facets, histogram, field table) and the row
/// table both reflect **every** source — sealed SFSTs, in-memory chunks
/// of active WALs, and the WAL tails — interleaved under the unified
/// cursor order `(timestamp_ns, file_seq, part, position)`, where
/// `part` distinguishes the sub-sources of one active WAL that share a `seq`.
///
/// Pure sync — no I/O scheduling, no locks, no geometry policy — but
/// since it reads and decompresses files the caller is expected to invoke
/// it off any async runtime thread.
///
/// **Cancellation** is cooperative, polled once per source (a single
/// in-flight source still runs to completion): once `cancel` fires, the
/// loops stop opening further sources and `run` returns whatever was
/// assembled so far. The caller racing the call against the token (the
/// bridge's cancel `select!`) discards that partial result — its only
/// purpose is to stop burning CPU/IO promptly. Callers that don't
/// cancel pass `CancellationToken::new()`.
///
/// **Progress**: `progress` is incremented by one as each source's
/// step-1 shard completes; the caller advertises the total
/// (`sources.len()`) out of band. Callers that don't report pass a
/// fresh `Arc::new(AtomicUsize::new(0))`.
pub fn run(
    sources: Vec<LogSource>,
    query: LogsQuery,
    cancel: CancellationToken,
    progress: Arc<AtomicUsize>,
) -> LogsData {
    let grid = query.grid;

    // Map every SFST source once, up front, and share each mapping with
    // both passes. An open mapping pins the file's inode, so an SFST
    // unlinked by retention mid-query stays readable and the stats and
    // page passes always see the same source set — `matched` can no
    // longer count rows the page pass fails to re-open. A source that
    // fails to map (logged in `map_source`) is `None` and contributes
    // nothing to either pass. Tails are row-scanned, not mapped.
    let mapped: Vec<Option<Mapped>> = sources
        .iter()
        .map(|source| match source {
            LogSource::Sfst(c) => mmap::map_source(&c.source),
            LogSource::Tail(_) => None,
        })
        .collect();

    // Step 1: evaluate every source into a shard (see `LogSource::to_shard`)
    // and merge them. The merge is a monoid, so source order is irrelevant
    // and a failed source's empty shard is its identity — which also makes
    // a cancelled partial merge well-formed.
    let mut shards = Vec::with_capacity(sources.len());
    for (source, mapping) in sources.iter().zip(&mapped) {
        if cancel.is_cancelled() {
            break;
        }
        shards.push(source.to_shard(&query, mapping.as_ref()));
        progress.fetch_add(1, Ordering::Relaxed);
    }
    let stats = LogsShard::merge(shards);

    // `available_fields` is the merged table with high-card fields
    // dropped — the offerable facet / histogram set — while `columns`
    // keeps the full name set, all tiers. The high-card drop happens
    // here, once, at the root.
    let available_fields: sfst::FieldTable = stats
        .fields
        .iter()
        .filter(|field| !field.is_high_card())
        .cloned()
        .collect();
    let columns: Vec<String> = stats.fields.names().map(str::to_owned).collect();
    let histogram = stats.timeline.unwrap_or_else(|| empty_timeline(grid));

    // Step 2: paginate across every source under the unified cursor
    // order — on-disk SFSTs and in-memory chunks (`Part::Indexed`), and
    // the WAL tails (`Part::Tail`) — reading the same mappings as step 1.
    let page = paginate(&sources, &mapped, &query, &cancel);

    LogsData {
        matched: stats.matched,
        facets: stats.facets,
        histogram_field: query.histogram_field,
        histogram,
        available_fields,
        columns,
        rows: page.rows,
        has_newer: page.has_newer,
        has_older: page.has_older,
    }
}

/// An empty timeline aligned to `grid`: no dimensions, all-zero buckets.
fn empty_timeline(grid: sfst::Grid) -> sfst::Timeline {
    sfst::Timeline {
        grid,
        dimensions: Vec::new(),
        buckets: vec![
            sfst::Bucket {
                counts: Vec::new(),
                unset: 0,
            };
            grid.num_buckets
        ],
    }
}

#[cfg(test)]
mod tests {
    use super::*;
    use crate::logs::LogsQueryBuilder;

    /// Sources whose bytes are garbage: each one degrades to an empty
    /// shard, but `run` still walks it — which is exactly what the
    /// progress/cancel contract is about (one tick per source visited).
    fn garbage_sources(n: usize) -> Vec<LogSource> {
        (0..n)
            .map(|i| {
                LogSource::Sfst(SfstCandidate {
                    summary: sfst::Summary {
                        min_timestamp_s: 0,
                        max_timestamp_s: 10,
                        record_count: 0,
                        content_meta: Vec::new(),
                    },
                    file_seq: i as u64 + 1,
                    part: Part::Indexed(0),
                    source: Source::Memory(Arc::new(vec![0u8; 4])),
                })
            })
            .collect()
    }

    fn query() -> LogsQuery {
        LogsQueryBuilder::new(sfst::Grid::new(0, 10_000_000_000, 1)).build()
    }

    #[test]
    fn progress_reaches_total_on_normal_run() {
        let progress = Arc::new(AtomicUsize::new(0));
        let data = run(
            garbage_sources(5),
            query(),
            CancellationToken::new(),
            Arc::clone(&progress),
        );
        assert_eq!(progress.load(Ordering::Relaxed), 5);
        assert_eq!(data.matched, 0);
    }

    #[test]
    fn pre_cancelled_token_evaluates_no_source_and_still_returns() {
        let progress = Arc::new(AtomicUsize::new(0));
        let cancel = CancellationToken::new();
        cancel.cancel();
        // Returns a well-formed (empty, grid-aligned) LogsData without
        // touching any source — the caller discards it.
        let data = run(garbage_sources(5), query(), cancel, Arc::clone(&progress));
        assert_eq!(progress.load(Ordering::Relaxed), 0);
        assert_eq!(data.matched, 0);
        assert_eq!(data.rows.len(), 0);
        assert_eq!(data.histogram.buckets.len(), 1);
    }
}
