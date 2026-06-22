//! Step 2: row materialization (select-then-fetch).
//!
//! Returning a page of rows needs a global order over cursors
//! `(timestamp_ns, file_seq, part, position)` across files, so — unlike step 1 —
//! it isn't a plain fold. It decomposes into seams a cross-node fan-out
//! reuses:
//!
//! - [`PageShard::evaluate`] (map: one file -> its page candidates),
//! - [`PageShard::merge`] (reduce: combine candidate sets — associative),
//! - [`finalize_page`] (root: pick the page + the has-more flags),
//! - [`materialize`] (fetch: chosen cursors -> row bodies).
//!
//! [`paginate`] is the local orchestration of all four; a distributed
//! parent would run `merge`/`finalize_page` on candidate sets received
//! from children and route `materialize` back to the file's owning node by
//! `file_seq`.

use std::collections::HashMap;

use super::mmap::Mapped;

use super::cursor::{Cursor, NS_PER_S, Part};
use super::engine::{LogSource, SfstCandidate, WalTail};
use super::mmap;
use super::query::{Anchor, Direction, LogsQuery};
use super::wal_scan::WalScan;

/// One file's (or one node's) page candidates: the window-matching
/// cursors on the requested side of the anchor, ordered closest-to-anchor
/// first, plus whether the file has any match on the *opposite* side
/// (which becomes the opposite direction's has-more flag).
///
/// [`PageShard::evaluate`] produces one per file; [`PageShard::merge`] folds
/// them. The candidate list may be bounded to the page size (a later
/// step) — all a fan-out needs to ship — or unbounded.
#[derive(Debug, Default)]
pub struct PageShard {
    /// Candidate cursors, ordered closest-to-anchor first — the order
    /// [`merge`](PageShard::merge) and `finalize_page` take a prefix of.
    pub cursors: Vec<Cursor>,
    /// Whether this shard has any match on the side of the anchor *away*
    /// from the page direction (the source of the opposite has-more flag).
    pub has_opposite: bool,
}

impl PageShard {
    /// Reduce: combine page-candidate shards into one.
    ///
    /// Pools the candidates, re-orders them closest-to-anchor first for
    /// `direction`, optionally keeps only the nearest `bound`, and ORs
    /// `has_opposite`. Associative, so a node can merge the files it owns
    /// and a parent can merge the node-shards the same way.
    pub fn merge(shards: Vec<PageShard>, direction: Direction, bound: Option<usize>) -> PageShard {
        let mut cursors: Vec<Cursor> = Vec::new();
        let mut has_opposite = false;
        for shard in shards {
            cursors.extend(shard.cursors);
            has_opposite |= shard.has_opposite;
        }
        order_by_closeness(&mut cursors, direction);
        if let Some(bound) = bound {
            cursors.truncate(bound);
        }
        PageShard {
            cursors,
            has_opposite,
        }
    }

    /// Fold `other` into `self` in place — the incremental form of
    /// [`merge`](PageShard::merge) used to combine sources one at a time.
    /// Same per-step semantics as `merge(vec![take(self), other], …)`
    /// (re-order and re-bound after each fold), without the intermediate
    /// `Vec`.
    pub fn merge_into(&mut self, other: PageShard, direction: Direction, bound: Option<usize>) {
        self.cursors.extend(other.cursors);
        self.has_opposite |= other.has_opposite;
        order_by_closeness(&mut self.cursors, direction);
        if let Some(bound) = bound {
            self.cursors.truncate(bound);
        }
    }

    /// Map: evaluate one file's page candidates against the query.
    ///
    /// Intersects the filter with the window, tags each matching position
    /// with its [`Cursor`], and splits at `anchor` (exclusive): the candidates
    /// are the matches on `query.direction`'s side, ordered closest-to-anchor
    /// first and optionally truncated to `bound`; `has_opposite` records
    /// whether any match falls on the other side. `anchor == None` starts at
    /// the edge — every match is a candidate and there is no opposite side.
    pub fn evaluate(
        reader: &sfst::IndexReader<'_>,
        seq: u64,
        part: Part,
        query: &LogsQuery,
        anchor: Option<Cursor>,
        bound: Option<usize>,
    ) -> Result<PageShard, sfst::Error> {
        let filter = reader.compile_filter(&query.filter, query.query())?;
        let matched = reader.matched_positions(&filter, query.grid.range_ns())?;
        let timestamps = reader.load_timestamps()?;

        // Cursors for every match, ascending — within a file, position order
        // is cursor order (timestamps are chronological and `seq`/`part`
        // are constant). A matched position with no timestamp would mean the
        // file's chunks disagree (corrupt SFST); fail so the caller skips
        // this source rather than emitting a bogus epoch-0 cursor.
        //
        // Defensive — unreachable today: `matched_positions` clamps the match
        // set to `[lo, hi)` with `hi <= timestamps.len()` (the window is
        // resolved against this same timestamps chunk via `range_positions`),
        // so every matched position is in range. Kept as a guard against a
        // future `matched_positions` that stops clamping. (C-4 in
        // docs/sfsq-readability-refactors.md: a negative test would need a
        // mock reader, since no real/forged file can reach this branch.)
        let ascending: Vec<Cursor> = matched
            .into_iter()
            .map(|position| {
                let timestamp_ns = timestamps.at(position).ok_or_else(|| {
                    sfst::Error::CorruptIndex(format!(
                        "matched position {position} has no timestamp \
                         (file_seq={seq}, part={part:?})"
                    ))
                })?;
                Ok(Cursor {
                    timestamp_ns,
                    file_seq: seq,
                    part,
                    position,
                })
            })
            .collect::<Result<Vec<_>, sfst::Error>>()?;

        Ok(Self::from_cursors(
            ascending,
            query.direction,
            anchor,
            bound,
        ))
    }

    /// Build a shard from this source's cursors, already sorted ascending
    /// by [`Cursor`] order. Splits at `anchor` (exclusive), keeps the
    /// page side ordered closest-to-anchor first, and bounds it. Shared
    /// by the SFST path ([`evaluate`](Self::evaluate), whose cursors are
    /// ascending by position) and the WAL-tail row scan (whose cursors
    /// must be sorted first, since the tail isn't time-ordered).
    pub fn from_cursors(
        mut ascending: Vec<Cursor>,
        direction: Direction,
        anchor: Option<Cursor>,
        bound: Option<usize>,
    ) -> PageShard {
        debug_assert!(
            ascending.is_sorted(),
            "from_cursors requires ascending-by-cursor input"
        );
        // Split at the anchor (exclusive). Backward's page side is `< anchor`
        // (opposite `>= anchor`); forward's is `> anchor` (opposite `<= anchor`).
        let (mut cursors, has_opposite) = match (anchor, direction) {
            (None, _) => (std::mem::take(&mut ascending), false),
            (Some(a), Direction::Backward) => {
                let split = ascending.partition_point(|c| *c < a);
                let has_opposite = split < ascending.len();
                ascending.truncate(split);
                (ascending, has_opposite)
            }
            (Some(a), Direction::Forward) => {
                let split = ascending.partition_point(|c| *c <= a);
                let has_opposite = split > 0;
                (ascending.split_off(split), has_opposite)
            }
        };

        order_by_closeness(&mut cursors, direction);
        if let Some(bound) = bound {
            cursors.truncate(bound);
        }

        PageShard {
            cursors,
            has_opposite,
        }
    }
}

/// Order cursors closest-to-anchor first for `direction`: backward walks
/// toward older rows, so the largest (newest) cursors come first;
/// forward walks toward newer rows, so the smallest (oldest) come first.
fn order_by_closeness(cursors: &mut [Cursor], direction: Direction) {
    match direction {
        Direction::Backward => cursors.sort_unstable_by(|a, b| b.cmp(a)),
        Direction::Forward => cursors.sort_unstable(),
    }
}

/// The chosen page: cursors newest-first, plus the has-more flags a
/// consumer uses to gate infinite scroll in each direction.
#[derive(Default)]
struct SelectedPage {
    /// Cursors newest-first (`cursors[0]` is the newest).
    cursors: Vec<Cursor>,
    /// A newer row exists beyond the page (consumer "scroll up").
    has_newer: bool,
    /// An older row exists beyond the page (consumer "scroll down").
    has_older: bool,
}

/// Root: pick the page from the merged candidates.
///
/// The nearest `limit` cursors form the page; one more candidate beyond
/// them means there are more rows in `direction`, and `merged.has_opposite`
/// means more on the other side. The page is returned newest-first
/// regardless of direction.
fn finalize_page(merged: PageShard, direction: Direction, limit: usize) -> SelectedPage {
    let has_more_in_direction = merged.cursors.len() > limit;
    let mut cursors = merged.cursors;
    cursors.truncate(limit);
    // `cursors` is closest-to-anchor first: backward (toward older) is
    // already newest-first; forward (toward newer) is oldest-first, so
    // reverse it to present newest-first like the other direction.
    if direction == Direction::Forward {
        cursors.reverse();
    }
    let (has_newer, has_older) = match direction {
        Direction::Backward => (merged.has_opposite, has_more_in_direction),
        Direction::Forward => (has_more_in_direction, merged.has_opposite),
    };
    SelectedPage {
        cursors,
        has_newer,
        has_older,
    }
}

/// Which source (within one query) a cursor belongs to: its file plus the
/// [`Part`] discriminator. An `Indexed` and a `Tail` cursor of the same
/// `file_seq` route to different sources, so both fields are part of the
/// key.
#[derive(Debug, Clone, Copy, PartialEq, Eq, Hash)]
struct SourceKey {
    file_seq: u64,
    part: Part,
}

/// Identifies one materialized row: its [`SourceKey`] plus the row's
/// position within that source.
#[derive(Debug, Clone, Copy, PartialEq, Eq, Hash)]
struct RowKey {
    source: SourceKey,
    position: u32,
}

impl SourceKey {
    fn of(cursor: &Cursor) -> SourceKey {
        SourceKey {
            file_seq: cursor.file_seq,
            part: cursor.part,
        }
    }
}

impl RowKey {
    fn of(cursor: &Cursor) -> RowKey {
        RowKey {
            source: SourceKey::of(cursor),
            position: cursor.position,
        }
    }
}

/// Fetch: materialize the chosen cursors into rows.
///
/// Routes each cursor to its owning source by [`SourceKey`], batches
/// positions per source so each one decompresses once, and reassembles the
/// rows in the page's newest-first order. Locally the sources are the open
/// readers; a cross-node fetch would route each cursor to its owning node.
fn materialize(
    sfst_readers: &[(sfst::IndexReader<'_>, SourceKey)],
    tail_scans: &[(u64, WalScan)],
    selected: &SelectedPage,
) -> Result<Vec<(Cursor, sfst::MaterializedRow)>, sfst::Error> {
    // Route by `SourceKey`: an SFST reader (on-disk or chunk) for
    // `Part::Indexed`, the WAL row scanner (keyed by `file_seq`, one tail
    // per WAL) for `Part::Tail`.
    let sfst_by_key: HashMap<SourceKey, &sfst::IndexReader<'_>> = sfst_readers
        .iter()
        .map(|(reader, key)| (*key, reader))
        .collect();
    let tail_by_seq: HashMap<u64, &WalScan> =
        tail_scans.iter().map(|(seq, scan)| (*seq, scan)).collect();

    // Batch positions per source so each source decompresses once.
    let mut positions: HashMap<SourceKey, Vec<u32>> = HashMap::new();
    for cursor in &selected.cursors {
        positions
            .entry(SourceKey::of(cursor))
            .or_default()
            .push(cursor.position);
    }

    let mut row_by_key: HashMap<RowKey, sfst::MaterializedRow> = HashMap::new();
    for (source, pos) in &positions {
        if source.part == Part::Tail {
            if let Some(scan) = tail_by_seq.get(&source.file_seq) {
                for (p, row) in pos.iter().zip(scan.materialize_rows(pos)) {
                    row_by_key.insert(
                        RowKey {
                            source: *source,
                            position: *p,
                        },
                        row,
                    );
                }
            }
        } else if let Some(reader) = sfst_by_key.get(source) {
            for (p, row) in pos.iter().zip(reader.materialize_rows(pos)?) {
                row_by_key.insert(
                    RowKey {
                        source: *source,
                        position: *p,
                    },
                    row,
                );
            }
        }
    }

    let rows = selected
        .cursors
        .iter()
        .filter_map(|cursor| {
            row_by_key
                .remove(&RowKey::of(cursor))
                .map(|row| (*cursor, row))
        })
        .collect();
    Ok(rows)
}

/// A materialized page: rows newest-first plus the has-more flags.
#[derive(Default)]
pub(super) struct Page {
    pub(super) rows: Vec<(Cursor, sfst::MaterializedRow)>,
    pub(super) has_newer: bool,
    pub(super) has_older: bool,
}

/// Open the candidate files in time-priority order and materialize one
/// page, stopping as soon as the remaining files can't contribute.
///
/// Candidates are processed closest-to-anchor first (backward: newest
/// file first; forward: oldest first). Each file's bounded candidates fold
/// into a running merge; once the page is full *and* the next file is
/// entirely beyond the page boundary, the rest are skipped — never opened
/// or decoded.
///
/// `mapped` carries each source's bytes, resolved once per query by
/// [`run`](super::engine::run) (parallel to `sources`; `None` = tail or
/// failed map) — the same mappings the stats pass read, so an SFST
/// unlinked by retention between the passes is still served here.
/// Files that fail to parse/evaluate are
/// logged and skipped. Each opened file's cold suffix is released from the
/// page cache once the page is materialized.
///
/// The `anchor` (from a prior page's cursor) is only an exclusive
/// comparison boundary — it is never itself materialized. So the source it
/// once came from need not be in `sources`; only the *page rows'* sources
/// must be, and they always are, since every page cursor is produced by a
/// source evaluated in this same call. A stale anchor pointing at a now-
/// absent source (e.g. a WAL since sealed) is therefore harmless here; the
/// only cross-request artifact is the documented WAL→SFST cursor seam.
pub(super) fn paginate(
    sources: &[LogSource],
    mapped: &[Option<Mapped>],
    query: &LogsQuery,
    cancel: &tokio_util::sync::CancellationToken,
) -> Page {
    let (wal_tails, sfst_candidates) = partition_sources(sources, mapped);

    // limit + 1: one extra candidate past the page so finalize can set the
    // has-more flags.
    let page_bound = Some(query.limit.saturating_add(1));
    let anchor = query.anchor.map(Anchor::to_cursor);
    let mut merged = PageShard::default();

    // Seed the merge with the WAL tails *before* the SFSTs: the early-
    // termination below samples `merged.cursors[limit - 1]` as its
    // boundary, which must already include every tail cursor — adding tails
    // later can only push the boundary older, letting `beyond_boundary`
    // wrongly skip an SFST whose rows belong on the page.
    let tail_scans = scan_tails(&wal_tails, query, anchor, page_bound, &mut merged);

    // SFSTs (on-disk + in-memory chunks): order the pre-resolved
    // mappings into a stable `Vec` the readers can borrow, then open +
    // evaluate + fold them closest-to-anchor first with early termination.
    let mappings = sort_mapped_sfsts(sfst_candidates, query.direction);
    let (readers, reader_mapping) =
        open_and_evaluate_sfsts(&mappings, query, anchor, page_bound, &mut merged, cancel);

    let page = build_page(merged, &readers, &tail_scans, query);
    release_cold(readers, &reader_mapping, &mappings);
    page
}

/// Split sources by kind, pairing each SFST with its pre-resolved
/// mapping (an `Arc` bump). A candidate whose map failed (`None`) is
/// dropped here — it contributed nothing to the stats pass either.
/// Order *within* each kind is irrelevant — the
/// merge re-sorts by cursor; the ordering that matters (tails before
/// SFSTs) is enforced by [`paginate`]'s call order, not here.
fn partition_sources<'a>(
    sources: &'a [LogSource],
    mapped: &[Option<Mapped>],
) -> (Vec<&'a WalTail>, Vec<(&'a SfstCandidate, Mapped)>) {
    debug_assert_eq!(sources.len(), mapped.len());
    let mut wal_tails = Vec::new();
    let mut sfst_candidates = Vec::new();
    for (source, mapping) in sources.iter().zip(mapped) {
        match source {
            LogSource::Tail(t) => wal_tails.push(t),
            LogSource::Sfst(c) => {
                if let Some(m) = mapping {
                    sfst_candidates.push((c, m.clone()));
                }
            }
        }
    }
    (wal_tails, sfst_candidates)
}

/// Scan each WAL tail, fold its page shard into `merged`, and keep the
/// scans for the materialize step. A tail that fails to scan or evaluate
/// is logged and skipped.
fn scan_tails(
    wal_tails: &[&WalTail],
    query: &LogsQuery,
    anchor: Option<Cursor>,
    bound: Option<usize>,
    merged: &mut PageShard,
) -> Vec<(u64, WalScan)> {
    // `materialize` routes tail cursors by `file_seq` alone (`tail_by_seq`),
    // and tail cursors share one `(file_seq, Part::Tail, position)` space —
    // so each WAL must contribute at most one tail. The caller guarantees
    // this (a WAL with an un-indexable chunk is refused whole rather than
    // split into per-range tails; see C-5 in the readability backlog). Assert
    // it so a future caller that violates it fails loudly here instead of
    // silently dropping rows at materialize time.
    debug_assert!(
        {
            let mut seqs: Vec<u64> = wal_tails.iter().map(|t| t.file_seq).collect();
            seqs.sort_unstable();
            seqs.dedup();
            seqs.len() == wal_tails.len()
        },
        "WalTail file_seqs must be unique (one tail per WAL)"
    );

    let mut tail_scans: Vec<(u64, WalScan)> = Vec::new();
    for &tail in wal_tails {
        let scan = match WalScan::scan_range(&tail.path, tail.range) {
            Ok(scan) => scan,
            Err(e) => {
                tracing::warn!(
                    "sfsq: tail scan failed for {} [{}..{}]: {e}",
                    tail.path.display(),
                    tail.range.start(),
                    tail.range.end()
                );
                continue;
            }
        };
        match scan.page_shard(tail.file_seq, query, anchor, bound) {
            Ok(shard) => merged.merge_into(shard, query.direction, bound),
            Err(e) => {
                tracing::warn!(
                    "sfsq: tail page candidates failed (file_seq={}): {e}",
                    tail.file_seq
                );
                continue;
            }
        }
        tail_scans.push((tail.file_seq, scan));
    }
    tail_scans
}

/// Sort pre-mapped candidates closest-to-anchor first (backward: newest
/// file first; forward: oldest first) into the stable `Vec` the readers
/// borrow. The bytes were mapped once by [`run`](super::engine::run);
/// this only orders them.
fn sort_mapped_sfsts<'a>(
    mut candidates: Vec<(&'a SfstCandidate, Mapped)>,
    direction: Direction,
) -> Vec<(Mapped, &'a SfstCandidate)> {
    match direction {
        Direction::Backward => {
            candidates.sort_by_key(|(c, _)| std::cmp::Reverse(c.summary.max_timestamp_s));
        }
        Direction::Forward => candidates.sort_by_key(|(c, _)| c.summary.min_timestamp_s),
    }
    candidates
        .into_iter()
        .map(|(candidate, mapping)| (mapping, candidate))
        .collect()
}

/// Open and evaluate each mapped SFST closest-to-anchor first, folding its
/// page shard into `merged`. Stops once the page is full *and* the next
/// file is entirely beyond the page boundary — later files (time-sorted)
/// can't contribute, so they're never opened. Returns the opened readers
/// (which borrow `mappings`) and their mapping indices, for materialize and
/// cold-release. A file that fails to parse/evaluate is logged and skipped.
///
/// Polls `cancel` before each source: page work is bounded, so this is
/// belt-and-suspenders, but it keeps a cancelled query from opening
/// more files. The cancelled caller discards the partial page.
fn open_and_evaluate_sfsts<'a>(
    mappings: &'a [(Mapped, &SfstCandidate)],
    query: &LogsQuery,
    anchor: Option<Cursor>,
    bound: Option<usize>,
    merged: &mut PageShard,
    cancel: &tokio_util::sync::CancellationToken,
) -> (Vec<(sfst::IndexReader<'a>, SourceKey)>, Vec<usize>) {
    let mut readers: Vec<(sfst::IndexReader<'a>, SourceKey)> = Vec::new();
    let mut reader_mapping: Vec<usize> = Vec::new();
    for (index, (mapping, candidate)) in mappings.iter().enumerate() {
        if cancel.is_cancelled() {
            break;
        }
        let reader = match sfst::IndexReader::open(mapping.bytes()) {
            Ok(reader) => reader,
            Err(e) => {
                tracing::warn!("sfsq: failed to parse {}: {e}", candidate.source.describe());
                continue;
            }
        };
        match PageShard::evaluate(
            &reader,
            candidate.file_seq,
            candidate.part,
            query,
            anchor,
            bound,
        ) {
            Ok(shard) => merged.merge_into(shard, query.direction, bound),
            Err(e) => {
                tracing::warn!(
                    "sfsq: page candidates failed for {}: {e}",
                    candidate.source.describe()
                );
                continue;
            }
        }
        readers.push((
            reader,
            SourceKey {
                file_seq: candidate.file_seq,
                part: candidate.part,
            },
        ));
        reader_mapping.push(index);

        if query.limit > 0 && merged.cursors.len() > query.limit {
            let boundary = merged.cursors[query.limit - 1];
            if mappings.get(index + 1).is_some_and(|(_, next)| {
                let summary = &next.summary;
                beyond_boundary(
                    query.direction,
                    boundary,
                    summary.min_timestamp_s,
                    summary.max_timestamp_s,
                )
            }) {
                break;
            }
        }
    }
    (readers, reader_mapping)
}

/// Finalize the merged shard into a page and materialize its rows. A
/// materialize failure collapses to an empty page rather than reporting
/// has-more flags with no rows behind them.
fn build_page(
    merged: PageShard,
    readers: &[(sfst::IndexReader<'_>, SourceKey)],
    tail_scans: &[(u64, WalScan)],
    query: &LogsQuery,
) -> Page {
    let selected = finalize_page(merged, query.direction, query.limit);
    match materialize(readers, tail_scans, &selected) {
        Ok(rows) => Page {
            rows,
            has_newer: selected.has_newer,
            has_older: selected.has_older,
        },
        Err(e) => {
            tracing::warn!("sfsq: materialize failed: {e}");
            Page::default()
        }
    }
}

/// Release each opened file's cold suffix (mid/high field chunks + stream
/// batches) from the page cache, keeping the hot prefix resident. In-memory
/// chunks have no file pages to drop.
fn release_cold(
    readers: Vec<(sfst::IndexReader<'_>, SourceKey)>,
    reader_mapping: &[usize],
    mappings: &[(Mapped, &SfstCandidate)],
) {
    let cold: Vec<(usize, (usize, usize))> = readers
        .iter()
        .zip(reader_mapping)
        .filter_map(|((reader, _), &index)| reader.cold_region().map(|region| (index, region)))
        .collect();
    // Free the readers before the advise loop. Not load-bearing — the
    // borrows of `mappings` are shared, and `release_cold_region` re-faults
    // identical bytes — but it keeps the lifetime story tidy.
    drop(readers);
    for (index, region) in cold {
        if let Mapped::File(m) = &mappings[index].0 {
            mmap::release_cold_region(m, region);
        }
    }
}

/// Whether a candidate with second-granular range `[min_ts_s, max_ts_s]`
/// lies entirely beyond the page boundary — so it (and, since candidates
/// are processed in time-priority order, every one after it) cannot
/// contribute a cursor nearer the anchor. `boundary` is the page's
/// farthest-from-anchor cursor (the L-th).
///
/// Conservative across the second→nanosecond gap: a file is skipped only
/// when its *entire* second-range is past the boundary, so a file that
/// could still hold a contributing cursor is never skipped.
fn beyond_boundary(direction: Direction, boundary: Cursor, min_ts_s: u32, max_ts_s: u32) -> bool {
    match direction {
        // Backward: the file's newest possible cursor is `< (max_ts_s + 1)·s`.
        // If that's at or below the boundary, no cursor can sit nearer the
        // anchor (a larger cursor) than the boundary.
        Direction::Backward => (i64::from(max_ts_s) + 1) * NS_PER_S <= boundary.timestamp_ns,
        // Forward: the file's oldest possible cursor is `>= min_ts_s·s`. If
        // that's beyond the boundary, no cursor can sit nearer the anchor (a
        // smaller cursor) than the boundary.
        Direction::Forward => i64::from(min_ts_s) * NS_PER_S > boundary.timestamp_ns,
    }
}

#[cfg(test)]
mod tests;
