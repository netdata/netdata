//! Query-time row scan over a WAL file — the second evaluator.
//!
//! [`WalScan`] answers a [`LogsQuery`] over WAL-resident data without
//! building an index: it decodes the file's frames into rows once
//! ([`scan`](WalScan::scan)) and computes the statistics with plain
//! per-row loops ([`evaluate`](WalScan::evaluate)). It exists for data
//! the indexer hasn't reached yet — the sub-chunk *tail* of an active
//! WAL file, whose bounded size is what
//! makes re-scanning per query affordable.
//!
//! # Semantic equality with the SFST engine
//!
//! This is a deliberate second implementation of the query semantics,
//! and it must agree with [`LogsShard::evaluate`] over an SFST built
//! from the same frames — the shared guarantees:
//!
//! - the *rows* are identical by construction: both consumers receive
//!   them from `wal_otap`'s one frame decode;
//! - filter patterns anchor identically: both compile through
//!   [`sfst::compile_pattern`] / [`sfst::compile_query`];
//! - field tables classify identically: same distinct-pair cardinality
//!   rule, same [`sfst::DEFAULT_CARDINALITY_THRESHOLD`].
//!
//! What this module re-implements — and what the equivalence tests must
//! hold against the SFST engine — is the *evaluation*: OR-within-field /
//! AND-across-fields matching, the own-selection exclusion for facets
//! and the histogram, per-row value dedup (a row carrying one pair twice
//! counts once; a row carrying two values of a field counts in both),
//! the histogram's exact `unset`, and lexicographic value ordering.
//!
//! [`LogsShard::evaluate`]: super::aggregate::LogsShard::evaluate

use std::collections::HashMap;
use std::path::Path;

use sfst::{FacetResult, FieldEntry, FieldTable, FieldTier, Grid, Matcher, Timeline};

use super::aggregate::{LogsShard, eligible_facet_fields};
use super::cursor::{Cursor, Part};
use super::page::PageShard;
use super::query::LogsQuery;

/// A scan failure: the WAL file couldn't be read or a frame couldn't
/// be decoded — exactly [`wal_otap::ReadError`], the shared
/// read-and-decode error. Scanning is all-or-nothing — a torn or
/// corrupt frame fails the scan rather than silently truncating the
/// row set (the caller decides whether to degrade, mirroring the
/// per-file policy in [`run`](super::run)).
pub use wal_otap::ReadError as WalScanError;

/// One distinct `key=value` pair seen during the scan.
struct Pair {
    /// The full `key=value` string, exactly as interned by the indexer.
    kv: String,
    /// Byte offset of the `=` separator: field is `kv[..eq]`, value is
    /// `kv[eq + 1..]`. Same field-extraction rule as the indexer's
    /// interner: first `=`, or the whole string when none (cannot
    /// arise — wal-otap's decode always emits the separator; handled
    /// defensively).
    eq: usize,
}

impl Pair {
    fn field(&self) -> &str {
        &self.kv[..self.eq]
    }

    fn value(&self) -> &str {
        if self.eq < self.kv.len() {
            &self.kv[self.eq + 1..]
        } else {
            ""
        }
    }
}

/// One decoded log row: timestamp plus its pair tokens in stream order
/// (duplicates preserved, exactly as the indexer stores them).
struct Row {
    ts_ns: i64,
    tokens: Vec<u32>,
}

/// The decoded rows of one WAL file, ready to evaluate queries against.
///
/// Build once with [`scan`](Self::scan), evaluate any number of queries
/// with [`evaluate`](Self::evaluate). The pair table is deduplicated by
/// full string (`u32` tokens), so per-row storage and all evaluation
/// loops work on small integers.
pub struct WalScan {
    pairs: Vec<Pair>,
    /// Full `key=value` string → token. Retained for exact-matcher
    /// resolution at evaluation time.
    dedup: HashMap<String, u32>,
    rows: Vec<Row>,
    /// Field name → tokens of its distinct values. Insertion order is
    /// scan order; evaluation sorts where ordering matters.
    field_tokens: HashMap<String, Vec<u32>>,
    /// The field table, built with the same cardinality rule the
    /// indexer applies (distinct pairs per field vs. the threshold).
    fields: FieldTable,
}

impl WalScan {
    /// Decode every frame of the WAL file at `path` into rows.
    ///
    /// Reads the whole file. To scan only the durable, not-yet-indexed
    /// tail of an active file, use [`scan_range`](Self::scan_range).
    pub fn scan(path: &Path) -> Result<WalScan, WalScanError> {
        let mut sink = ScanSink::default();
        wal_otap::decode_file(path, &mut sink)?;
        Ok(sink.finish())
    }

    /// Decode the frames in `range` into rows — the active-WAL tail. The
    /// range runs from a chunk-end frame boundary to the durable bound
    /// (`valid_up_to`); see [`wal::FrameRange`] / [`wal::Reader::open_range`]
    /// for the soundness checks. An empty range yields a zero-row scan.
    pub fn scan_range(path: &Path, range: wal::FrameRange) -> Result<WalScan, WalScanError> {
        let mut sink = ScanSink::default();
        wal_otap::decode_range(path, range, &mut sink)?;
        Ok(sink.finish())
    }

    /// Number of decoded log rows.
    pub fn num_rows(&self) -> usize {
        self.rows.len()
    }

    /// Page candidates for the tail — the row-scan counterpart of
    /// [`PageShard::evaluate`]. Every row matching the filter within the
    /// window becomes a [`Cursor`] tagged [`Part::Tail`] (the tail sorts
    /// after all chunks of the same `seq`) with `position` the row's
    /// **insertion index** — stable while the row stays in the tail.
    /// Unlike an SFST the tail isn't
    /// time-ordered, so the cursors are sorted explicitly before the
    /// shared split/order/bound.
    pub fn page_shard(
        &self,
        seq: u64,
        query: &LogsQuery,
        anchor: Option<Cursor>,
        bound: Option<usize>,
    ) -> Result<PageShard, sfst::Error> {
        let compiled = self.compile(query)?;
        let window = query.grid.range_ns();
        let mut conjuncts = RowMatch::default();
        let mut cursors: Vec<Cursor> = Vec::new();
        for (i, row) in self.rows.iter().enumerate() {
            compiled.match_row_into(row, &mut conjuncts);
            if conjuncts.full && window.contains(&row.ts_ns) {
                cursors.push(Cursor {
                    timestamp_ns: row.ts_ns,
                    file_seq: seq,
                    part: Part::Tail,
                    position: i as u32,
                });
            }
        }
        cursors.sort_unstable();
        Ok(PageShard::from_cursors(
            cursors,
            query.direction,
            anchor,
            bound,
        ))
    }

    /// Materialize tail rows by insertion index — the row-scan
    /// counterpart of `IndexReader::materialize_rows`. Each row's pairs
    /// are emitted in stored (stream) order, keys not deduplicated, to
    /// match the SFST path.
    pub fn materialize_rows(&self, positions: &[u32]) -> Vec<sfst::MaterializedRow> {
        positions
            .iter()
            .map(|&pos| {
                // Positions are insertion indices this scan produced via
                // `page_shard`, so they are always in range. Index
                // directly (not `get`): a silent drop would misalign the
                // caller's `zip(positions, rows)` and corrupt the page —
                // fail fast instead.
                let row = &self.rows[pos as usize];
                sfst::MaterializedRow {
                    timestamp_ns: row.ts_ns,
                    fields: row
                        .tokens
                        .iter()
                        .map(|&t| {
                            let pair = &self.pairs[t as usize];
                            (pair.field().to_string(), pair.value().to_string())
                        })
                        .collect(),
                }
            })
            .collect()
    }

    /// Evaluate `query` into a [`LogsShard`] — the row-scan counterpart
    /// of [`LogsShard::evaluate`] over an SFST candidate, with the same
    /// degrade-gracefully shape: an unresolvable filter contributes the
    /// field table but no counts; a high-cardinality (or invalid-grid)
    /// histogram contributes no timeline.
    ///
    /// [`LogsShard::evaluate`]: super::aggregate::LogsShard::evaluate
    pub fn evaluate(&self, query: &LogsQuery) -> LogsShard {
        let fields = self.fields.clone();

        // Resolve the filter to per-field token sets once (the row-scan
        // analogue of `IndexReader::compile_filter`). A malformed
        // pattern degrades exactly like the SFST path: field table only.
        let compiled = match self.compile(query) {
            Ok(compiled) => compiled,
            Err(e) => {
                tracing::warn!("wal_scan: compile filter failed: {e}");
                return LogsShard {
                    matched: 0,
                    facets: Vec::new(),
                    timeline: None,
                    fields,
                };
            }
        };

        let grid = query.grid;
        let window = grid.range_ns();

        // Facet fields usable here: present and not high-card — the same
        // per-file rule `LogsShard::evaluate` applies before calling the
        // reader.
        let facet_fields = eligible_facet_fields(&query.facet_fields, &fields);
        let mut facets = FacetAcc::new(&facet_fields);

        let mut timeline = TimelineAcc::new(&query.histogram_field, grid, &fields, self);

        let mut matched: u64 = 0;
        let mut scratch: Vec<u32> = Vec::new();
        let mut conjuncts = RowMatch::default();
        for row in &self.rows {
            // Which filter conjuncts this row satisfies, plus the
            // full-text query term. `full` is their AND — identical to
            // the bitmap path's `filter.full()` membership.
            compiled.match_row_into(row, &mut conjuncts);
            let in_window = window.contains(&row.ts_ns);

            if in_window && conjuncts.full {
                matched += 1;
            }

            // Distinct tokens of this row (bitmap semantics: a row
            // carrying the same pair twice counts once per value).
            scratch.clear();
            for &t in &row.tokens {
                if !scratch.contains(&t) {
                    scratch.push(t);
                }
            }

            if in_window {
                facets.accumulate(&compiled, &conjuncts, &scratch, self);
            }
            timeline.accumulate(row.ts_ns, &compiled, &conjuncts, &scratch);
        }

        LogsShard {
            matched,
            facets: facets.finish(self),
            timeline: timeline.finish(),
            fields,
        }
    }

    /// Resolve the query's filter and full-text term against the pair
    /// table (the row-scan analogue of `compile_filter`).
    fn compile(&self, query: &LogsQuery) -> Result<CompiledFilter, sfst::Error> {
        let mut per_field: Vec<(String, TokenSet)> = Vec::new();
        for (field, matchers) in query.filter.iter() {
            per_field.push((field.clone(), self.field_matches(field, matchers)?));
        }

        let query_set = match query.query() {
            Some(src) => {
                let regex = sfst::compile_query(src)?;
                let mut set = TokenSet::new(self.pairs.len());
                for (t, pair) in self.pairs.iter().enumerate() {
                    if regex.is_match(pair.kv.as_bytes()) {
                        set.insert(t as u32);
                    }
                }
                Some(set)
            }
            None => None,
        };

        Ok(CompiledFilter {
            per_field,
            query: query_set,
        })
    }

    /// The tokens of `field` whose value matches any of `matchers` —
    /// the row-scan analogue of `field_values_or`: exact values resolve
    /// by full-string lookup, patterns anchor via
    /// [`sfst::compile_pattern`] and test the value bytes of each of the
    /// field's distinct values. An absent field yields the empty set
    /// (so its conjunct matches nothing).
    fn field_matches(&self, field: &str, matchers: &[Matcher]) -> Result<TokenSet, sfst::Error> {
        let mut set = TokenSet::new(self.pairs.len());

        // Absent field → empty set, before any pattern compiles — the
        // same gate order as the SFST path, whose `field_values_or`
        // returns empty on a `locate_field` miss without touching the
        // matchers (so a malformed pattern on an absent field is not an
        // error there, and must not be one here).
        //
        // The gate also makes the exact-match lookup below sound. Field
        // names in `field_tokens` never contain `=` (extraction splits
        // on the first one), so for any field that passes the gate,
        // `"{field}={value}"` re-splits at the intended boundary.
        // Without the gate, a requested field `a=b` with exact value
        // `c` would concatenate to `a=b=c` and false-match that stored
        // pair — whose field is `a` — where the SFST path matches
        // nothing.
        if !self.field_tokens.contains_key(field) {
            return Ok(set);
        }

        let mut patterns: Vec<regex::bytes::Regex> = Vec::new();
        for matcher in matchers {
            match matcher {
                Matcher::Exact(value) => {
                    let kv = format!("{field}={value}");
                    if let Some(&t) = self.dedup.get(&kv) {
                        set.insert(t);
                    }
                }
                Matcher::Pattern(src) => patterns.push(sfst::compile_pattern(src)?),
            }
        }

        if !patterns.is_empty() {
            if let Some(tokens) = self.field_tokens.get(field) {
                for &t in tokens {
                    let value = self.pairs[t as usize].value();
                    if patterns.iter().any(|re| re.is_match(value.as_bytes())) {
                        set.insert(t);
                    }
                }
            }
        }

        Ok(set)
    }
}

// ---------------------------------------------------------------------------
// Scan sink
// ---------------------------------------------------------------------------

/// The [`wal_otap::KvSink`] that accumulates a [`WalScan`].
///
/// Tokens are dense indexes into the pair table, deduplicated by full
/// `key=value` string. No hash index is kept: `lookup_hash` always
/// answers `None` — the always-safe escape hatch the trait documents —
/// so every pair is formatted and deduped by string. Collision safety
/// is therefore structural rather than maintained.
#[derive(Default)]
struct ScanSink {
    pairs: Vec<Pair>,
    dedup: HashMap<String, u32>,
    rows: Vec<Row>,
}

impl wal_otap::KvSink for ScanSink {
    type Token = u32;

    fn lookup_hash(&mut self, _hash: u64) -> Option<u32> {
        None
    }

    fn intern(&mut self, _hash: Option<u64>, kv: &str) -> u32 {
        if let Some(&t) = self.dedup.get(kv) {
            return t;
        }
        let t = self.pairs.len() as u32;
        self.pairs.push(Pair {
            kv: kv.to_string(),
            eq: kv.find('=').unwrap_or(kv.len()),
        });
        self.dedup.insert(kv.to_string(), t);
        t
    }

    fn reserve_rows(&mut self, additional: usize) {
        self.rows.reserve(additional);
    }

    fn row(&mut self, ts_ns: i64, tokens: &[u32]) {
        self.rows.push(Row {
            ts_ns,
            tokens: tokens.to_vec(),
        });
    }
}

impl ScanSink {
    /// Derive the per-field token lists and the field table, completing
    /// the scan. Cardinality is the number of distinct pairs per field
    /// and tiers use the indexer's default threshold — the same rule
    /// `KeyValueInterner` applies, so an SFST built from these frames
    /// carries an identical table.
    fn finish(self) -> WalScan {
        let mut field_tokens: HashMap<String, Vec<u32>> = HashMap::new();
        for (t, pair) in self.pairs.iter().enumerate() {
            field_tokens
                .entry(pair.field().to_string())
                .or_default()
                .push(t as u32);
        }

        let threshold = sfst::DEFAULT_CARDINALITY_THRESHOLD as usize;
        let tier_of = |cardinality: usize| -> FieldTier {
            if cardinality < threshold {
                FieldTier::Low
            } else if cardinality < threshold * 10 {
                FieldTier::Mid
            } else {
                FieldTier::High
            }
        };

        // Table order matches the indexer's: low → mid → high, each tier
        // sorted by field name.
        let mut by_tier: [Vec<FieldEntry>; 3] = Default::default();
        for (name, tokens) in &field_tokens {
            let tier = tier_of(tokens.len());
            by_tier[match tier {
                FieldTier::Low => 0,
                FieldTier::Mid => 1,
                FieldTier::High => 2,
            }]
            .push(FieldEntry {
                name: name.clone(),
                cardinality: tokens.len() as u32,
                tier,
            });
        }
        for tier in &mut by_tier {
            tier.sort_by(|a, b| a.name.cmp(&b.name));
        }
        let fields: FieldTable = by_tier.into_iter().flatten().collect();

        WalScan {
            pairs: self.pairs,
            dedup: self.dedup,
            rows: self.rows,
            field_tokens,
            fields,
        }
    }
}

// ---------------------------------------------------------------------------
// Compiled filter
// ---------------------------------------------------------------------------

/// A dense bitset over pair tokens.
struct TokenSet {
    bits: Vec<u64>,
}

impl TokenSet {
    fn new(num_tokens: usize) -> Self {
        Self {
            bits: vec![0u64; num_tokens.div_ceil(64)],
        }
    }

    fn insert(&mut self, t: u32) {
        self.bits[(t / 64) as usize] |= 1u64 << (t % 64);
    }

    fn contains(&self, t: u32) -> bool {
        (self.bits[(t / 64) as usize] >> (t % 64)) & 1 == 1
    }
}

/// The query's filter resolved against one [`WalScan`]'s pair table —
/// the row-scan analogue of `BitmapFilter`: one matched-token set per
/// filter field, plus the full-text query's set.
struct CompiledFilter {
    per_field: Vec<(String, TokenSet)>,
    query: Option<TokenSet>,
}

/// Which parts of the filter one row satisfies. Reused across rows
/// (the `fields` buffer is refilled in place by
/// [`CompiledFilter::match_row_into`]).
#[derive(Default)]
struct RowMatch {
    /// Per filter-field conjunct, parallel to `CompiledFilter::per_field`.
    fields: Vec<bool>,
    /// The full-text query term (`true` when no query was given).
    query: bool,
    /// AND of every conjunct and the query — the full filter.
    full: bool,
}

impl CompiledFilter {
    fn match_row_into(&self, row: &Row, m: &mut RowMatch) {
        m.fields.clear();
        m.fields.extend(
            self.per_field
                .iter()
                .map(|(_, set)| row.tokens.iter().any(|&t| set.contains(t))),
        );
        m.query = match &self.query {
            Some(set) => row.tokens.iter().any(|&t| set.contains(t)),
            None => true,
        };
        m.full = m.query && m.fields.iter().all(|&ok| ok);
    }

    /// Whether the row satisfies every conjunct *except* `field`'s own
    /// selection (plus the query) — the row-scan analogue of
    /// `BitmapFilter::without`, used to scope a facet or histogram so a
    /// field's own selection doesn't collapse its breakdown.
    fn matches_without(&self, m: &RowMatch, field: &str) -> bool {
        m.query
            && self
                .per_field
                .iter()
                .zip(&m.fields)
                .all(|((name, _), &ok)| ok || name == field)
    }
}

// ---------------------------------------------------------------------------
// Facet accumulation
// ---------------------------------------------------------------------------

/// Per-facet-field counters over the row loop.
struct FacetAcc<'q> {
    /// `(field, token → count)` per eligible facet field, in request
    /// order (the order `IndexReader::facets` emits results).
    per_field: Vec<(&'q str, HashMap<u32, u32>)>,
}

impl<'q> FacetAcc<'q> {
    fn new(facet_fields: &'q [String]) -> Self {
        Self {
            per_field: facet_fields
                .iter()
                .map(|f| (f.as_str(), HashMap::new()))
                .collect(),
        }
    }

    /// Count this row's distinct values of each facet field whose scope
    /// (the filter minus the field's own selection, plus the query) the
    /// row satisfies. The caller has already applied the window clip.
    fn accumulate(
        &mut self,
        compiled: &CompiledFilter,
        conjuncts: &RowMatch,
        distinct_tokens: &[u32],
        scan: &WalScan,
    ) {
        for (field, counts) in &mut self.per_field {
            // Scope check is per facet field; recomputing it from the
            // precomputed conjunct booleans is O(filter fields).
            if !compiled.matches_without(conjuncts, field) {
                continue;
            }
            for &t in distinct_tokens {
                if scan.pairs[t as usize].field() == *field {
                    *counts.entry(t).or_insert(0) += 1;
                }
            }
        }
    }

    fn finish(self, scan: &WalScan) -> Vec<FacetResult> {
        self.per_field
            .into_iter()
            .map(|(field, counts)| {
                let mut values: Vec<(String, u32)> = counts
                    .into_iter()
                    .map(|(t, count)| (scan.pairs[t as usize].value().to_string(), count))
                    .collect();
                // Lexicographic by value — FST iteration order in the
                // SFST path (`field=` prefix is shared, so ordering by
                // value equals ordering by full key).
                values.sort_by(|a, b| a.0.cmp(&b.0));
                FacetResult {
                    field: field.to_string(),
                    values,
                }
            })
            .collect()
    }
}

// ---------------------------------------------------------------------------
// Timeline accumulation
// ---------------------------------------------------------------------------

/// The histogram accumulator over the row loop — the row-scan analogue
/// of `IndexReader::timeline`, including its exact-`unset` rule.
struct TimelineAcc<'q> {
    /// `None` when the field is high-card here or the grid is invalid —
    /// the cases where the SFST path errors and `evaluate` degrades the
    /// shard's timeline to `None`.
    state: Option<TimelineState<'q>>,
}

struct TimelineState<'q> {
    field: &'q str,
    grid: Grid,
    /// Dimension labels: every distinct value of the field in the file
    /// (not just matching rows), lexicographic — FST enumeration order.
    dimensions: Vec<String>,
    /// token → dimension index.
    dim_of: HashMap<u32, usize>,
    /// `counts[dim][bucket]`, transposed at finish.
    dim_counts: Vec<Vec<u64>>,
    /// Rows satisfying the scope (filter minus own selection), per bucket.
    bucket_total: Vec<u64>,
    /// Of those, rows carrying at least one value of the field.
    with_field: Vec<u64>,
}

impl<'q> TimelineAcc<'q> {
    fn new(field: &'q str, grid: Grid, fields: &FieldTable, scan: &WalScan) -> Self {
        // Mirror the SFST error paths that `evaluate` turns into a
        // `None` timeline: an invalid bucket width, or a field that is
        // high-cardinality in this file.
        if grid.bucket_width_ns <= 0 {
            tracing::warn!("wal_scan: timeline failed: invalid bucket width");
            return Self { state: None };
        }
        if fields.get(field).is_some_and(|f| f.is_high_card()) {
            tracing::warn!("wal_scan: timeline failed: high-cardinality field {field}");
            return Self { state: None };
        }

        // Dimensions: the field's distinct values across the whole
        // file, sorted lexicographically. An absent field yields no
        // dimensions — every matching row lands in `unset`.
        let mut tokens: Vec<u32> = scan.field_tokens.get(field).cloned().unwrap_or_default();
        tokens.sort_by(|&a, &b| {
            scan.pairs[a as usize]
                .value()
                .cmp(scan.pairs[b as usize].value())
        });
        let dimensions: Vec<String> = tokens
            .iter()
            .map(|&t| scan.pairs[t as usize].value().to_string())
            .collect();
        let dim_of: HashMap<u32, usize> = tokens.iter().enumerate().map(|(i, &t)| (t, i)).collect();

        Self {
            state: Some(TimelineState {
                field,
                grid,
                dim_counts: vec![vec![0; grid.num_buckets]; dimensions.len()],
                bucket_total: vec![0; grid.num_buckets],
                with_field: vec![0; grid.num_buckets],
                dimensions,
                dim_of,
            }),
        }
    }

    fn accumulate(
        &mut self,
        ts_ns: i64,
        compiled: &CompiledFilter,
        conjuncts: &RowMatch,
        distinct_tokens: &[u32],
    ) {
        let Some(state) = &mut self.state else { return };
        // Range check first: it's a pair of comparisons, while the scope
        // check iterates the filter's conjuncts.
        let grid = state.grid;
        if !grid.range_ns().contains(&ts_ns) {
            return;
        }
        if !compiled.matches_without(conjuncts, state.field) {
            return;
        }
        let bucket = ((ts_ns - grid.bucket_start_ns) / grid.bucket_width_ns) as usize;

        state.bucket_total[bucket] += 1;
        let mut has_field = false;
        for &t in distinct_tokens {
            if let Some(&dim) = state.dim_of.get(&t) {
                state.dim_counts[dim][bucket] += 1;
                has_field = true;
            }
        }
        if has_field {
            state.with_field[bucket] += 1;
        }
    }

    fn finish(self) -> Option<Timeline> {
        let state = self.state?;
        let buckets = (0..state.grid.num_buckets)
            .map(|b| sfst::Bucket {
                counts: state.dim_counts.iter().map(|dc| dc[b]).collect(),
                unset: state.bucket_total[b] - state.with_field[b],
            })
            .collect();
        Some(Timeline {
            grid: state.grid,
            dimensions: state.dimensions,
            buckets,
        })
    }
}

#[cfg(test)]
mod tests;
