//! Reader for the split-FST index format.
//!
//! Opens an `.sfst` file (typically via mmap) and provides query methods
//! that follow the access pattern described in `sfst/FORMAT.md`:
//!
//! 1. Decode SUMR + META + PRIM eagerly on open (always needed).
//! 2. Look up low-card `key=value` pairs in the primary FST → bitmap.
//! 3. Load secondary chunks on demand (mid-card FST or high-card blob).
//! 4. Load per-stream log entries for attribute resolution.

use fst_index::FstIndex;

use crate::{
    BitmapValue, Bucket, FacetResult, FieldEntry, FieldTier, Filter, Grid, Histogram, IdRanges,
    KvId, Matcher, Metadata, Summary, Timeline, Timestamps,
};

/// A successfully opened split-FST index.
///
/// Holds the mmap'd data, the deserialized summary, and the primary
/// FST (both eagerly loaded on open since every query needs them).
/// [`Metadata`] is cached on the underlying [`crate::Reader`] and
/// surfaced via [`metadata`](Self::metadata).
pub struct IndexReader<'a> {
    sfst: crate::Reader<'a>,
    summary: Summary,
    primary: FstIndex<BitmapValue>,
}

impl<'a> IndexReader<'a> {
    /// Open a split-FST index from a byte slice (typically an mmap).
    ///
    /// Immediately deserializes the summary, metadata, and primary FST.
    /// Metadata stays cached on the underlying [`crate::Reader`].
    pub fn open(data: &'a [u8]) -> Result<Self, crate::Error> {
        let sfst = crate::Reader::open(data)?;
        let summary = sfst.summary()?;
        // Force the metadata cache so subsequent accessors are infallible.
        sfst.metadata()?;
        let primary = sfst.primary()?;
        Ok(Self {
            sfst,
            summary,
            primary,
        })
    }

    /// The cheap summary fields (timestamps, record count, opaque
    /// `part_key`/`content_meta` identity).
    pub fn summary(&self) -> &Summary {
        &self.summary
    }

    /// The heavy index metadata (histogram + id_ranges + field table).
    pub fn metadata(&self) -> &Metadata {
        self.sfst
            .metadata()
            .expect("metadata cached at IndexReader::open")
    }

    /// Total number of log entries in this index.
    pub fn total_logs(&self) -> u32 {
        self.summary.record_count
    }

    /// The ID ranges for the three cardinality tiers.
    pub fn id_ranges(&self) -> &IdRanges {
        &self.metadata().id_ranges
    }

    /// The sparse histogram for time-range estimation.
    pub fn histogram(&self) -> &Histogram {
        &self.metadata().histogram
    }

    /// The file's opaque content-plane metadata blob (the content plane decodes
    /// it; the substrate never interprets it).
    pub fn content_meta(&self) -> &[u8] {
        &self.summary.content_meta
    }

    // ── Field table ─────────────────────────────────────────────────

    /// The field table (carried inside [`Metadata`]).
    pub fn field_table(&self) -> &crate::FieldTable {
        &self.metadata().fields
    }

    /// Byte span of the cold suffix (mid/high field chunks + stream
    /// batches) a query releases from the page cache once done. See
    /// [`crate::Reader::cold_region`].
    pub fn cold_region(&self) -> Option<(usize, usize)> {
        self.sfst.cold_region()
    }

    // ── Primary FST lookups ─────────────────────────────────────────

    /// Look up a low-card `key=value` pair in the primary FST.
    pub fn primary_lookup(&self, key_value: &[u8]) -> Option<&BitmapValue> {
        self.primary.get(key_value)
    }

    /// Iterate over all entries in the primary FST.
    pub fn primary_for_each(&self, f: impl FnMut(&[u8], &BitmapValue)) {
        self.primary.for_each(f);
    }

    /// Prefix search on the primary FST.
    pub fn primary_prefix(&self, prefix: &[u8]) -> Vec<(Vec<u8>, &BitmapValue)> {
        self.primary.prefix_pairs(prefix)
    }

    // ── Secondary chunk loading ─────────────────────────────────────

    /// Load a mid-cardinality field's FST. `mid_index` is `0..num_mid`.
    pub fn load_mid_field(&self, mid_index: u16) -> Result<FstIndex<BitmapValue>, crate::Error> {
        self.sfst.mid_field(mid_index)
    }

    // ── Per-log timestamps ──────────────────────────────────────────

    /// Load the per-log nanosecond [`Timestamps`], chronologically ordered
    /// and parallel-indexed to the concatenation of the stream-batch
    /// chunks (see [`load_all_stream_entries`](Self::load_all_stream_entries)).
    pub fn load_timestamps(&self) -> Result<Timestamps, crate::Error> {
        Ok(Timestamps::new(self.sfst.timestamps()?))
    }

    // ── Stream-batch chunks ─────────────────────────────────────────

    /// Number of stream-batch chunks in this file. Derived from
    /// `summary.record_count` via [`crate::num_stream_batches`].
    pub fn num_stream_batches(&self) -> u8 {
        crate::num_stream_batches(self.summary.record_count)
    }

    /// Load one stream-batch chunk by index (`0..num_stream_batches`).
    ///
    /// Returns the attribute lists for the logs in that batch, in
    /// chronological order. Concatenating batches in order yields the
    /// full chronological log stream.
    pub fn load_stream_batch(&self, batch_index: u8) -> Result<crate::StreamBatch, crate::Error> {
        self.sfst.stream_batch(batch_index)
    }

    /// Load and concatenate every stream-batch chunk into per-row `KvId`
    /// lists, in chronological order. Convenience for tooling and tests that
    /// want the materialized form rather than walking [`StreamBatch`](crate::StreamBatch) rows;
    /// it reconstructs the `Vec<Vec<KvId>>`, so the hot scan/materialize
    /// paths use [`StreamBatch`](crate::StreamBatch) directly instead.
    pub fn load_all_stream_entries(&self) -> Result<Vec<Vec<KvId>>, crate::Error> {
        let n = self.num_stream_batches();
        let mut out = Vec::with_capacity(self.summary.record_count as usize);
        for i in 0..n {
            let batch = self.sfst.stream_batch(i)?;
            for r in 0..batch.num_rows() {
                out.push(batch.row(r).collect());
            }
        }
        Ok(out)
    }

    // ── KvId resolution ───────────────────────────────────────────

    /// Determine which cardinality tier a [`KvId`] belongs to.
    pub fn kv_id_tier(&self, id: KvId) -> FieldTier {
        let ranges = self.id_ranges();
        if id.0 < ranges.low_end.0 {
            FieldTier::Low
        } else if id.0 < ranges.mid_end.0 {
            FieldTier::Mid
        } else {
            FieldTier::High
        }
    }

    /// Build a reverse lookup table: `KvId → key=value` string.
    ///
    /// Walks the primary FST and every secondary chunk, decompressing as
    /// it goes. Returns one entry per `key=value` pair in the file.
    pub fn build_string_table(
        &self,
        field_table: &[FieldEntry],
    ) -> Result<Vec<String>, crate::Error> {
        let total = self.metadata().id_ranges.high_end.0 as usize;
        let mut table = vec![String::new(); total];
        let mut kv_id = 0usize;

        // Low-card: iterate primary FST.
        self.primary.for_each(|key, _| {
            if kv_id < table.len() {
                table[kv_id] = String::from_utf8_lossy(key).into_owned();
            }
            kv_id += 1;
        });

        // Mid/high-card: iterate secondary chunks in field_table order,
        // tracking mid- and high-relative positions independently.
        let mut mid_index: u16 = 0;
        let mut high_index: u16 = 0;
        for field in field_table {
            match field.tier {
                FieldTier::Low => continue,
                FieldTier::Mid => {
                    let fst = self.sfst.mid_field(mid_index)?;
                    fst.for_each(|key, _| {
                        if kv_id < table.len() {
                            table[kv_id] = String::from_utf8_lossy(key).into_owned();
                        }
                        kv_id += 1;
                    });
                    mid_index += 1;
                }
                FieldTier::High => {
                    let hf = self.sfst.high_field(high_index)?;
                    for key in hf.keys() {
                        if kv_id < table.len() {
                            table[kv_id] = String::from_utf8_lossy(key).into_owned();
                        }
                        kv_id += 1;
                    }
                    high_index += 1;
                }
            }
        }

        Ok(table)
    }

    /// Materialize full log rows for the given positions.
    ///
    /// For each position: its timestamp plus every attribute, resolved
    /// to a `(key, value)` pair by splitting the stored `key=value`
    /// string on the first `=`. Returns exactly one row per position, in
    /// the order supplied. A position that can't be resolved (out of
    /// range, or its stream batch / timestamp is inconsistent) means the
    /// file is corrupt and yields [`Error::CorruptIndex`](crate::Error::CorruptIndex)
    /// — never a silent skip, which would misalign a caller that pairs
    /// positions with the returned rows by index.
    ///
    /// This decompresses the timestamps chunk, all stream batches, and
    /// the whole reverse string table **once**, regardless of how many
    /// positions are requested — so callers should batch a page's worth
    /// of positions into a single call rather than invoking per-row.
    pub fn materialize_rows(
        &self,
        positions: &[u32],
    ) -> Result<Vec<crate::MaterializedRow>, crate::Error> {
        let timestamps = self.load_timestamps()?;
        let strings = self.build_string_table(self.field_table())?;
        let total = self.summary.record_count;
        let batch_size = crate::stream_batch_size(total);

        // Decode only the batches the requested positions fall in (a page is
        // a handful of rows, usually in one or two batches), and read each
        // row's KvIds straight from the fixed-width batch.
        let mut batches: Vec<Option<crate::StreamBatch>> =
            (0..self.num_stream_batches()).map(|_| None).collect();
        for &pos in positions {
            let b = (pos / batch_size) as usize;
            if pos < total && batches.get(b).is_some_and(Option::is_none) {
                batches[b] = Some(self.sfst.stream_batch(b as u8)?);
            }
        }

        let mut rows = Vec::with_capacity(positions.len());
        for &pos in positions {
            // The caller selected these positions from this file's own
            // index, so each must resolve to a real row. A miss means the
            // file's chunks disagree (corrupt SFST); fail rather than skip —
            // a skip would shorten the result and misalign the caller's
            // position-to-row pairing, attaching rows to the wrong cursors.
            if pos >= total {
                return Err(crate::Error::CorruptIndex(format!(
                    "materialize: position {pos} >= record_count {total}"
                )));
            }
            let b = (pos / batch_size) as usize;
            let local = (pos % batch_size) as usize;
            let Some(batch) = batches.get(b).and_then(Option::as_ref) else {
                return Err(crate::Error::CorruptIndex(format!(
                    "materialize: position {pos} maps to missing stream batch {b}"
                )));
            };
            if local >= batch.num_rows() {
                return Err(crate::Error::CorruptIndex(format!(
                    "materialize: position {pos} row {local} >= batch {b} length {}",
                    batch.num_rows()
                )));
            }
            let timestamp_ns = timestamps.at(pos).ok_or_else(|| {
                crate::Error::CorruptIndex(format!("materialize: position {pos} has no timestamp"))
            })?;
            let fields = batch
                .row(local)
                .map(|kv| {
                    let s = strings.get(kv.0 as usize).map(String::as_str).unwrap_or("");
                    match s.split_once('=') {
                        Some((k, v)) => (k.to_string(), v.to_string()),
                        None => (s.to_string(), String::new()),
                    }
                })
                .collect();
            rows.push(crate::MaterializedRow {
                timestamp_ns,
                fields,
            });
        }
        Ok(rows)
    }

    // ── Query API ────────────────────────────────────────────────────

    /// Compile a [`Filter`] against this file into a [`BitmapFilter`]:
    /// resolve each field's selected values to a position bitmap once, plus
    /// their AND (the full scope). The statistics methods take the compiled
    /// filter, so a query that touches `matched`/`facets`/`timeline`
    /// resolves the filter once instead of per call.
    ///
    /// An empty filter compiles to the full range `0..record_count`. A field
    /// mentioned in the filter but absent from this file resolves to the
    /// empty set, collapsing the full scope to empty — single-file SFSTs
    /// with disjoint field sets fall out of the query naturally.
    pub fn compile_filter(
        &self,
        filter: &Filter,
        query: Option<&str>,
    ) -> Result<BitmapFilter, crate::Error> {
        let universe = self.summary.record_count;
        let mut per_field: Vec<(String, PosSet)> = Vec::new();
        for (field, values) in filter.iter() {
            per_field.push((field.clone(), self.field_values_or(field, values)?));
        }
        // full = AND of every field; an empty filter is the full range.
        let mut full: Option<PosSet> = None;
        for (_, field_set) in &per_field {
            full = Some(match full {
                None => field_set.clone(),
                Some(mut acc) => {
                    acc.and_assign(field_set);
                    acc
                }
            });
            if full.as_ref().is_some_and(|s| s.is_empty()) {
                full = Some(PosSet::empty(universe));
                break;
            }
        }
        let mut full = full.unwrap_or_else(|| PosSet::full(universe));

        // Field-less full-text query: the positions of logs carrying any
        // `key=value` matching the (unanchored) query regex. A global AND
        // term, so it folds into `full` and into every facet's scope.
        let query_set = match query {
            Some(src) => {
                let regex = crate::query::compile_query(src)?;
                let set = self.query_positions(&regex)?;
                full.and_assign(&set);
                Some(set)
            }
            None => None,
        };

        Ok(BitmapFilter {
            universe,
            per_field,
            full,
            query: query_set,
        })
    }

    /// Count logs matching `filter` whose timestamp falls in `window_ns`
    /// (`[start, end)`). The window-clip uses the same `partition_point`
    /// bounds as [`facets`](Self::facets), so all the windowed paths agree.
    pub fn matched_count(
        &self,
        filter: &BitmapFilter,
        window_ns: std::ops::Range<i64>,
    ) -> Result<u64, crate::Error> {
        let total = self.summary.record_count;
        let (lo, hi) = self.range_positions(window_ns)?;
        let mut set = filter.full().clone();
        set.and_assign(&PosSet::range(lo, hi, total));
        Ok(set.len())
    }

    /// Positions (ascending) of logs matching `filter` whose timestamp
    /// falls in `window_ns` (`[start, end)`). Same clip as
    /// [`matched_count`](Self::matched_count); returns the positions so a
    /// caller can build per-row cursors.
    pub fn matched_positions(
        &self,
        filter: &BitmapFilter,
        window_ns: std::ops::Range<i64>,
    ) -> Result<Vec<u32>, crate::Error> {
        let total = self.summary.record_count;
        let (lo, hi) = self.range_positions(window_ns)?;
        let mut set = filter.full().clone();
        set.and_assign(&PosSet::range(lo, hi, total));
        Ok(set.iter().collect())
    }

    /// Position range `[lo, hi)` whose timestamps fall in `window_ns`
    /// (`[start, end)`), via `partition_point` on the chronological
    /// timestamps. Clamps naturally when the window extends past the
    /// file's range; the windowed query paths use it to count directly on
    /// the on-disk bitmaps via [`treight::Bitmap::range_cardinality`].
    pub fn range_positions(
        &self,
        window_ns: std::ops::Range<i64>,
    ) -> Result<(u32, u32), crate::Error> {
        Ok(self.load_timestamps()?.window(window_ns))
    }

    /// Compute per-field value counts for the UI's facet sidebar.
    ///
    /// For each facet field, the filter is evaluated **with that field's
    /// own selections removed** — so selecting `level=error` doesn't
    /// reduce the `level` facet to a single bar. Facets not present in
    /// the filter share a single evaluation of the full filter.
    ///
    /// Counts are restricted to `window_ns` (`[start, end)`), the same
    /// request window the histogram and matched-count use — so the
    /// sidebar reflects only the logs in view, not the whole file.
    ///
    /// Returns [`crate::Error::UnknownField`] for fields not in this
    /// file, or [`crate::Error::HighCardFacet`] for high-cardinality
    /// facets (where exact counts would require scanning stream batches).
    ///
    /// Note the deliberate asymmetry with
    /// [`timeline`](Self::timeline): `facets` *errors* on an absent
    /// field (a facet is requested per-field, so absence is a caller
    /// mistake), whereas `timeline` routes an absent field's logs to
    /// `unset`. Callers querying a heterogeneous set of files should
    /// pre-filter the field list to those present in each file.
    pub fn facets<S: AsRef<str>>(
        &self,
        fields: &[S],
        filter: &BitmapFilter,
        window_ns: std::ops::Range<i64>,
    ) -> Result<Vec<FacetResult>, crate::Error> {
        let total = self.summary.record_count;
        let (lo, hi) = self.range_positions(window_ns)?;
        let range_set = PosSet::range(lo, hi, total);

        // Full filtered set in the window; reused by every facet whose
        // field is not itself in the filter (cheap when the filter is
        // empty — it's just the window range).
        let mut full_set = filter.full().clone();
        full_set.and_assign(&range_set);

        let mut results = Vec::with_capacity(fields.len());
        for field in fields {
            let field = field.as_ref();
            // A field's facet excludes its own selection; the count is
            // scoped by the *rest* of the filter. When nothing else
            // constrains it, the scope is exactly the window range, so we
            // count each value's positions in `[lo, hi)` directly on the
            // on-disk bitmap — no per-value intersection (the hot path for
            // unfiltered / single-field-filtered queries).
            let values = if filter.is_unconstrained(field) {
                self.value_counts_in_range(field, lo, hi)?
            } else if filter.contains_field(field) {
                let mut scope = filter.without(field);
                scope.and_assign(&range_set);
                self.value_counts_under(field, &scope)?
            } else {
                self.value_counts_under(field, &full_set)?
            };
            results.push(FacetResult {
                field: field.to_string(),
                values,
            });
        }
        Ok(results)
    }

    /// Compute a 2D time × value-of-`field` count grid for chart rendering.
    ///
    /// `grid` is caller-supplied. A grid that extends past the file's
    /// actual log range produces zero counts in the outer buckets
    /// (handled naturally by `partition_point`). `field`'s own
    /// selections are excluded from the filter (same reason as in
    /// [`facets`](Self::facets)).
    ///
    /// A field that isn't present in this file is treated as "every
    /// matching log lacks it": the result has no dimensions and all
    /// matching logs land in `unset`. This keeps the histogram total
    /// equal to the matched count in a multi-file query where some
    /// files carry the field and others don't.
    ///
    /// Errors:
    /// - [`crate::Error::InvalidBucketWidth`] if `grid.bucket_width_ns <= 0`.
    /// - [`crate::Error::HighCardFacet`] if `field` is high-cardinality.
    pub fn timeline(
        &self,
        field: &str,
        filter: &BitmapFilter,
        grid: Grid,
    ) -> Result<Timeline, crate::Error> {
        if grid.bucket_width_ns <= 0 {
            return Err(crate::Error::InvalidBucketWidth(grid.bucket_width_ns));
        }

        // The field's own selection is excluded from its histogram (same
        // reason as in `facets`). When nothing *else* constrains it
        // (`fast`), each value's per-bucket count is read directly from the
        // on-disk bitmap via `range_cardinality` — no per-value scope op.
        let fast = filter.is_unconstrained(field);
        let filter_set = if filter.contains_field(field) {
            filter.without(field)
        } else {
            filter.full().clone()
        };

        // Per-bucket position ranges `[pos_lo, pos_hi)`, clamped naturally:
        // a grid extending past the file's range yields empty outer buckets.
        let bucket_ranges = self.load_timestamps()?.bucket_ranges(grid);

        // Enumerate the field's values into dimensions + per-bucket counts
        // (dimension-major; transposed below). An absent field leaves both
        // empty, so every matching log lands in `unset`. `has_field`
        // accumulates the union of every value's positions: a log carrying
        // several values of the field (multi-valued, e.g. flattened scalar
        // arrays) appears in several dimension bitmaps but only once in the
        // union — which is what makes `unset` exact below.
        let prefix = format!("{field}=");
        let prefix_len = prefix.len();
        let mut dimensions: Vec<String> = Vec::new();
        let mut dim_counts: Vec<Vec<u64>> = Vec::new();
        let mut has_field = PosSet::empty(self.summary.record_count);
        match self.locate_field(field) {
            None => {}
            Some(FieldLocation::Low) => {
                for (kv_bytes, bv) in self.primary.prefix_pairs(prefix.as_bytes()) {
                    dimensions.push(String::from_utf8_lossy(&kv_bytes[prefix_len..]).into_owned());
                    dim_counts.push(bucket_counts(bv, fast, &filter_set, &bucket_ranges));
                    has_field.or_assign(&PosSet::from_value(bv));
                }
            }
            Some(FieldLocation::Mid(idx)) => {
                let chunk = self.sfst.mid_field(idx)?;
                chunk.for_each(|kv_bytes, bv| {
                    dimensions.push(String::from_utf8_lossy(&kv_bytes[prefix_len..]).into_owned());
                    dim_counts.push(bucket_counts(bv, fast, &filter_set, &bucket_ranges));
                    has_field.or_assign(&PosSet::from_value(bv));
                });
            }
            Some(FieldLocation::High(_)) => {
                return Err(crate::Error::HighCardFacet(field.to_string()));
            }
        }
        if !fast {
            has_field.and_assign(&filter_set);
        }

        // Transpose dimension-major counts into bucket-major and derive
        // `unset`. `bucket_total` comes from `filter_set` — in the fast path
        // that's the full range, so its cardinality over the bucket equals
        // the bucket's log count. `unset` counts the matching logs with *no*
        // value for the field: `bucket_total − |has_field ∩ filter|` over
        // the bucket. Subtracting the per-dimension sum instead would be
        // wrong for multi-valued fields (a log carrying two values counts
        // in two dimensions, and the inflated sum eats into `unset`).
        let mut buckets = Vec::with_capacity(grid.num_buckets);
        for (bucket_i, &(pos_lo, pos_hi)) in bucket_ranges.iter().enumerate() {
            let counts: Vec<u64> = dim_counts.iter().map(|dc| dc[bucket_i]).collect();
            let bucket_total = filter_set.range_cardinality(pos_lo, pos_hi);
            let with_field = has_field.range_cardinality(pos_lo, pos_hi);
            debug_assert!(with_field <= bucket_total);
            buckets.push(Bucket {
                counts,
                unset: bucket_total.saturating_sub(with_field),
            });
        }

        Ok(Timeline {
            grid,
            dimensions,
            buckets,
        })
    }

    // ── Query helpers (private) ──────────────────────────────────────

    /// Locate a field by name and return its tier + tier-relative chunk
    /// index. Returns `None` if the field is absent from this file.
    fn locate_field(&self, field_name: &str) -> Option<FieldLocation> {
        let mut mid_idx = 0u16;
        let mut high_idx = 0u16;
        for field in self.metadata().fields.iter() {
            if field.name == field_name {
                return Some(match field.tier {
                    FieldTier::Low => FieldLocation::Low,
                    FieldTier::Mid => FieldLocation::Mid(mid_idx),
                    FieldTier::High => FieldLocation::High(high_idx),
                });
            }
            match field.tier {
                FieldTier::Mid => mid_idx += 1,
                FieldTier::High => high_idx += 1,
                _ => {}
            }
        }
        None
    }

    /// Compute the on-disk `KvId` for the `local`-th value of high-card
    /// chunk `high_idx`. High-card KvIds are `mid_end + (cumulative
    /// high-card cardinalities before this field) + local`.
    fn high_kv_id(&self, high_idx: u16, local: usize) -> KvId {
        let id_ranges = &self.metadata().id_ranges;
        let mut kv = id_ranges.mid_end.0;
        let mut current = 0u16;
        for field in self.metadata().fields.iter() {
            if let FieldTier::High = field.tier {
                if current == high_idx {
                    return KvId(kv + local as u32);
                }
                kv += field.cardinality;
                current += 1;
            }
        }
        panic!("high_kv_id: high_idx {high_idx} out of range");
    }

    /// Position set matching `field` against `matchers` (OR within field).
    /// Exact matchers resolve by direct lookup; pattern matchers compile
    /// full-value-anchored and test the field's distinct values. Returns
    /// the empty set if the field is absent from this file.
    ///
    /// All-exact selections (every query without a regex) take the same
    /// lookups they always have; patterns add an enumeration pass over the
    /// field's values only when present.
    ///
    /// A malformed pattern is a hard failure ([`crate::Error::InvalidPattern`]),
    /// not "matches nothing" — validate patterns at the request boundary.
    fn field_values_or(&self, field: &str, matchers: &[Matcher]) -> Result<PosSet, crate::Error> {
        let total = self.summary.record_count;
        let location = match self.locate_field(field) {
            Some(loc) => loc,
            None => return Ok(PosSet::empty(total)),
        };

        // Split into exact values (resolved by lookup) and compiled patterns
        // (full-value anchored, tested against the field's distinct values).
        let mut exacts: Vec<&str> = Vec::new();
        let mut patterns: Vec<regex::bytes::Regex> = Vec::new();
        for matcher in matchers {
            match matcher {
                Matcher::Exact(value) => exacts.push(value),
                Matcher::Pattern(src) => patterns.push(crate::query::compile_pattern(src)?),
            }
        }

        // Whether a `field=value` key's value-part matches any pattern. The
        // value is the bytes after `field=`, matched directly — keys are UTF-8
        // by construction, so there's no `str::from_utf8` validation to pay.
        let prefix_len = field.len() + 1;
        let value_matches = |kv_bytes: &[u8]| -> bool {
            let value = &kv_bytes[prefix_len..];
            patterns.iter().any(|regex| regex.is_match(value))
        };

        match location {
            FieldLocation::Low => {
                let mut result = PosSet::empty(total);
                for value in &exacts {
                    let kv = format!("{field}={value}");
                    if let Some(bv) = self.primary.get(kv.as_bytes()) {
                        result.or_assign(&PosSet::from_value(bv));
                    }
                }
                if !patterns.is_empty() {
                    let prefix = format!("{field}=");
                    self.primary
                        .prefix_for_each(prefix.as_bytes(), |kv_bytes, bv| {
                            if value_matches(kv_bytes) {
                                result.or_assign(&PosSet::from_value(bv));
                            }
                        });
                }
                Ok(result)
            }
            FieldLocation::Mid(idx) => {
                let chunk = self.sfst.mid_field(idx)?;
                let mut result = PosSet::empty(total);
                for value in &exacts {
                    let kv = format!("{field}={value}");
                    if let Some(bv) = chunk.get(kv.as_bytes()) {
                        result.or_assign(&PosSet::from_value(bv));
                    }
                }
                if !patterns.is_empty() {
                    chunk.for_each(|kv_bytes, bv| {
                        if value_matches(kv_bytes) {
                            result.or_assign(&PosSet::from_value(bv));
                        }
                    });
                }
                Ok(result)
            }
            FieldLocation::High(idx) => {
                // High-card values are addressed by KvId; the set is built by
                // scanning the SB batches indicated by the union of the matched
                // values' batch masks. Exact values are found by binary search
                // over the sorted key dictionary, patterns by a linear scan of
                // it. Matched positions are ascending (batch start increases,
                // position within increases), so they feed `from_sorted`.
                let hf = self.sfst.high_field(idx)?;
                // Matched KvIds for this field fall in the contiguous range
                // [base, base + cardinality).
                let mut targets = KvIdSet::new(self.high_kv_id(idx, 0).0, hf.len() as u32);
                let mut combined_mask: u8 = 0;
                for value in &exacts {
                    let kv = format!("{field}={value}");
                    if let Ok(local) = hf.binary_search(kv.as_bytes()) {
                        targets.insert(self.high_kv_id(idx, local));
                        combined_mask |= hf.masks[local];
                    }
                }
                if !patterns.is_empty() {
                    for (local, key) in hf.keys().enumerate() {
                        if value_matches(key) {
                            targets.insert(self.high_kv_id(idx, local));
                            combined_mask |= hf.masks[local];
                        }
                    }
                }
                if targets.is_empty() {
                    return Ok(PosSet::empty(total));
                }
                let batch_size = crate::stream_batch_size(total);
                let num_batches = crate::num_stream_batches(total);
                let mut positions: Vec<u32> = Vec::new();
                for b in 0..num_batches {
                    if (combined_mask >> b) & 1 == 0 {
                        continue;
                    }
                    let batch_start = u32::from(b) * batch_size;
                    let batch = self.sfst.stream_batch(b)?;
                    for i in 0..batch.num_rows() {
                        if batch.row(i).any(|id| targets.contains(id)) {
                            positions.push(batch_start + i as u32);
                        }
                    }
                }
                Ok(PosSet::from_sorted(positions, total))
            }
        }
    }

    /// Positions of logs carrying any `key=value` pair whose **whole string**
    /// matches `query` (unanchored — a "contains" full-text search; the
    /// `key=` part lets a pattern scope to a subset of fields).
    ///
    /// Scans every distinct key across all tiers: the primary FST (all
    /// low-card fields), each mid-card chunk, and each high-card key
    /// dictionary. Low/mid matches OR their posting bitmaps directly;
    /// high-card matches are gathered by `KvId` and resolved through the
    /// stream batches (as in [`field_values_or`](Self::field_values_or)).
    /// This is a full distinct-key scan — the inherent cost of field-less
    /// full text without a token index.
    fn query_positions(&self, query: &regex::bytes::Regex) -> Result<PosSet, crate::Error> {
        let total = self.summary.record_count;
        let mut result = PosSet::empty(total);

        // Low-card: the primary FST holds every low-card field's keys. The
        // query matches the raw `key=value` bytes directly — keys are UTF-8 by
        // construction, so there's no `str::from_utf8` validation to pay.
        self.primary.for_each(|kv_bytes, bv| {
            if query.is_match(kv_bytes) {
                result.or_assign(&PosSet::from_value(bv));
            }
        });

        // Mid-card chunks (OR bitmaps) and high-card dictionaries (gather
        // KvId targets for one stream-batch pass).
        let mut mid_idx = 0u16;
        let mut high_idx = 0u16;
        // Matched high-card KvIds span the whole high-card range
        // [mid_end, high_end) (this scan accumulates across all high fields).
        let id_ranges = &self.metadata().id_ranges;
        let mut targets = KvIdSet::new(
            id_ranges.mid_end.0,
            id_ranges.high_end.0 - id_ranges.mid_end.0,
        );
        let mut combined_mask: u8 = 0;
        for field in self.metadata().fields.iter() {
            match field.tier {
                FieldTier::Low => {}
                FieldTier::Mid => {
                    let chunk = self.sfst.mid_field(mid_idx)?;
                    chunk.for_each(|kv_bytes, bv| {
                        if query.is_match(kv_bytes) {
                            result.or_assign(&PosSet::from_value(bv));
                        }
                    });
                    mid_idx += 1;
                }
                FieldTier::High => {
                    let hf = self.sfst.high_field(high_idx)?;
                    for (local, key) in hf.keys().enumerate() {
                        if query.is_match(key) {
                            targets.insert(self.high_kv_id(high_idx, local));
                            combined_mask |= hf.masks[local];
                        }
                    }
                    high_idx += 1;
                }
            }
        }

        if !targets.is_empty() {
            let batch_size = crate::stream_batch_size(total);
            let num_batches = crate::num_stream_batches(total);
            let mut positions: Vec<u32> = Vec::new();
            for b in 0..num_batches {
                if (combined_mask >> b) & 1 == 0 {
                    continue;
                }
                let batch_start = u32::from(b) * batch_size;
                let batch = self.sfst.stream_batch(b)?;
                for i in 0..batch.num_rows() {
                    if batch.row(i).any(|id| targets.contains(id)) {
                        positions.push(batch_start + i as u32);
                    }
                }
            }
            result.or_assign(&PosSet::from_sorted(positions, total));
        }

        Ok(result)
    }

    /// Per-value `(value, count)` pairs for `field` restricted to `scope`.
    /// Walks the field's chunk once, intersecting each value's set with
    /// `scope`. Errors with [`crate::Error::UnknownField`] /
    /// [`crate::Error::HighCardFacet`] as appropriate.
    fn value_counts_under(
        &self,
        field: &str,
        scope: &PosSet,
    ) -> Result<Vec<(String, u32)>, crate::Error> {
        let location = self
            .locate_field(field)
            .ok_or_else(|| crate::Error::UnknownField(field.to_string()))?;
        let prefix = format!("{field}=");
        let prefix_len = prefix.len();
        let mut results = Vec::new();

        match location {
            FieldLocation::Low => {
                for (kv_bytes, bv) in self.primary.prefix_pairs(prefix.as_bytes()) {
                    let value = String::from_utf8_lossy(&kv_bytes[prefix_len..]).into_owned();
                    let mut set = PosSet::from_value(bv);
                    set.and_assign(scope);
                    let count = set.len() as u32;
                    if count > 0 {
                        results.push((value, count));
                    }
                }
            }
            FieldLocation::Mid(idx) => {
                let chunk = self.sfst.mid_field(idx)?;
                chunk.for_each(|kv_bytes, bv| {
                    let value = String::from_utf8_lossy(&kv_bytes[prefix_len..]).into_owned();
                    let mut set = PosSet::from_value(bv);
                    set.and_assign(scope);
                    let count = set.len() as u32;
                    if count > 0 {
                        results.push((value, count));
                    }
                });
            }
            FieldLocation::High(_) => {
                return Err(crate::Error::HighCardFacet(field.to_string()));
            }
        }

        Ok(results)
    }

    /// Per-value `(value, count)` pairs for `field`, counting each value's
    /// set positions in `[lo, hi)` directly on the on-disk bitmap (no
    /// per-value intersection). Used when no other field constrains the
    /// scope, so the count is exactly the value's positions in the window
    /// range. Errors as [`value_counts_under`](Self::value_counts_under).
    fn value_counts_in_range(
        &self,
        field: &str,
        lo: u32,
        hi: u32,
    ) -> Result<Vec<(String, u32)>, crate::Error> {
        let location = self
            .locate_field(field)
            .ok_or_else(|| crate::Error::UnknownField(field.to_string()))?;
        let prefix = format!("{field}=");
        let prefix_len = prefix.len();
        let mut results = Vec::new();

        match location {
            FieldLocation::Low => {
                for (kv_bytes, bv) in self.primary.prefix_pairs(prefix.as_bytes()) {
                    let count = bv.desc.range_cardinality(&bv.data, lo..hi) as u32;
                    if count > 0 {
                        let value = String::from_utf8_lossy(&kv_bytes[prefix_len..]).into_owned();
                        results.push((value, count));
                    }
                }
            }
            FieldLocation::Mid(idx) => {
                let chunk = self.sfst.mid_field(idx)?;
                chunk.for_each(|kv_bytes, bv| {
                    let count = bv.desc.range_cardinality(&bv.data, lo..hi) as u32;
                    if count > 0 {
                        let value = String::from_utf8_lossy(&kv_bytes[prefix_len..]).into_owned();
                        results.push((value, count));
                    }
                });
            }
            FieldLocation::High(_) => {
                return Err(crate::Error::HighCardFacet(field.to_string()));
            }
        }

        Ok(results)
    }
}

/// Per-bucket set-position counts for one value's on-disk bitmap.
///
/// Fast path (`fast` — nothing else constrains the scope): count directly
/// on the [`treight::Bitmap`] via `range_cardinality`. Slow path: intersect
/// the value with `scope`, then count each bucket on the result.
///
/// TODO(perf): this calls `range_cardinality` once per bucket, re-walking the
/// tree from the root ~num_buckets times. The buckets are a contiguous sorted
/// partition, so a single-pass `treight::Bitmap::range_counts(data, &edges)`
/// that tallies all buckets in one traversal (skip full sub-trees into their
/// bucket, descend only at the ~N boundaries) would collapse the histogram's
/// dominant `skip_subtree` cost (≈40% on a wide-OR + faceted query) from
/// `(dims+1) × buckets` walks to `dims+1`. Helps `timeline` only — facets'
/// per-value `len()` is already a single walk. Needs a treight addition.
fn bucket_counts(
    bv: &crate::BitmapValue,
    fast: bool,
    scope: &PosSet,
    bucket_ranges: &[(u32, u32)],
) -> Vec<u64> {
    if fast {
        bucket_ranges
            .iter()
            .map(|&(lo, hi)| bv.desc.range_cardinality(&bv.data, lo..hi))
            .collect()
    } else {
        let mut set = PosSet::from_value(bv);
        set.and_assign(scope);
        bucket_ranges
            .iter()
            .map(|&(lo, hi)| set.range_cardinality(lo, hi))
            .collect()
    }
}

/// Tier + tier-relative chunk index for a single field. Private; used by
/// the query helpers ([`IndexReader::facets`], [`IndexReader::timeline`]).
enum FieldLocation {
    Low,
    Mid(u16),
    High(u16),
}

/// A [`Filter`] compiled against one file: each filter
/// field's value-OR bitmap, plus their AND (the full scope). Built by
/// [`IndexReader::compile_filter`] so the statistics methods resolve the
/// filter once. Opaque — the scope helpers below are crate-internal.
#[derive(Debug)]
pub struct BitmapFilter {
    universe: u32,
    /// Each filter field paired with the OR of its selected values'
    /// position bitmaps (empty if the field is absent from this file).
    per_field: Vec<(String, PosSet)>,
    /// AND of every field's bitmap **and** the full-text query set — the
    /// full filter scope. The full range when the filter is empty.
    full: PosSet,
    /// Field-less full-text query positions (logs carrying a `key=value`
    /// matching the query regex), or `None` when no query was given. A
    /// global AND term: already folded into `full`, and AND'd into every
    /// facet's scope by [`without`](Self::without).
    query: Option<PosSet>,
}

impl BitmapFilter {
    /// The full filter scope (AND of every field and the query; full range
    /// if empty).
    fn full(&self) -> &PosSet {
        &self.full
    }

    /// Whether `field` has its own selection in the filter.
    fn contains_field(&self, field: &str) -> bool {
        self.per_field.iter().any(|(name, _)| name == field)
    }

    /// Whether no constraint *other than* `field`'s own selection applies —
    /// then `field`'s own facet/histogram can count directly over the window
    /// (the fast path), with no scope to intersect. A full-text query is a
    /// global constraint, so its presence rules the fast path out. Purely
    /// structural; builds no bitmap.
    fn is_unconstrained(&self, field: &str) -> bool {
        self.query.is_none() && !self.per_field.iter().any(|(name, _)| name != field)
    }

    /// The filter scope with `field`'s own selection excluded — the AND of
    /// the *other* fields and the query. For a facet/histogram on a field
    /// that is itself filtered and has siblings (otherwise it's
    /// [`is_unconstrained`]).
    ///
    /// [`is_unconstrained`]: Self::is_unconstrained
    fn without(&self, field: &str) -> PosSet {
        let mut acc: Option<PosSet> = None;
        for (name, field_set) in &self.per_field {
            if name == field {
                continue;
            }
            acc = Some(match acc {
                None => field_set.clone(),
                Some(mut a) => {
                    a.and_assign(field_set);
                    a
                }
            });
        }
        let mut acc = acc.unwrap_or_else(|| PosSet::full(self.universe));
        if let Some(query_set) = &self.query {
            acc.and_assign(query_set);
        }
        acc
    }
}

/// A set of log positions within one file, backed by a native
/// [`treight::Bitmap`] (a `Copy` descriptor plus its external tree bytes).
/// The whole query path operates on these — on-disk value bitmaps are used
/// as-is, and unions/intersections stay native (no Roaring round-trip). The
/// universe (position upper bound) is the file's `record_count`; every set
/// built for a file shares it so boolean ops line up.
#[derive(Clone, Debug)]
struct PosSet {
    bitmap: treight::Bitmap,
    data: Vec<u8>,
}

impl PosSet {
    /// The empty set over `universe` positions.
    fn empty(universe: u32) -> Self {
        Self {
            bitmap: treight::Bitmap::empty(universe),
            data: Vec::new(),
        }
    }

    /// Every position `0..universe`. `full` is an inverted-empty
    /// descriptor, so it needs no tree bytes.
    fn full(universe: u32) -> Self {
        Self {
            bitmap: treight::Bitmap::full(universe),
            data: Vec::new(),
        }
    }

    /// The contiguous half-open range `[lo, hi)` over `universe` positions.
    fn range(lo: u32, hi: u32, universe: u32) -> Self {
        let mut data = Vec::new();
        let bitmap = treight::Bitmap::from_range(lo..hi, universe, &mut data);
        Self { bitmap, data }
    }

    /// The positions set in an on-disk value bitmap. The descriptor is
    /// `Copy`; only the tree bytes are cloned.
    ///
    /// Invariant: every value bitmap in a file is built with the same
    /// `universe_size` (`== record_count`; see the writer's `remap_one_bitmap`),
    /// which is what lets the resulting set combine with `range`/`full`/other
    /// values via `and`/`or` — those require matching universes (a mismatch
    /// is only a debug assert, so it would be silently wrong in release).
    fn from_value(bv: &BitmapValue) -> Self {
        Self {
            bitmap: bv.desc,
            data: bv.data.clone(),
        }
    }

    /// A set built from positions in **ascending** order (high-card scan).
    fn from_sorted(positions: Vec<u32>, universe: u32) -> Self {
        let mut data = Vec::new();
        let bitmap = treight::Bitmap::from_sorted_iter(positions.into_iter(), universe, &mut data);
        Self { bitmap, data }
    }

    /// In-place union (`self |= other`).
    fn or_assign(&mut self, other: &Self) {
        let mut out = Vec::new();
        self.bitmap = self
            .bitmap
            .or(&self.data, &other.bitmap, &other.data, &mut out);
        self.data = out;
    }

    /// In-place intersection (`self &= other`).
    fn and_assign(&mut self, other: &Self) {
        let mut out = Vec::new();
        self.bitmap = self
            .bitmap
            .and(&self.data, &other.bitmap, &other.data, &mut out);
        self.data = out;
    }

    /// Whether the set is empty.
    fn is_empty(&self) -> bool {
        self.bitmap.is_empty(&self.data)
    }

    /// Total number of set positions.
    fn len(&self) -> u64 {
        self.bitmap.len(&self.data)
    }

    /// Number of set positions in `[lo, hi)`.
    fn range_cardinality(&self, lo: u32, hi: u32) -> u64 {
        self.bitmap.range_cardinality(&self.data, lo..hi)
    }

    /// Set positions in ascending order.
    fn iter(&self) -> impl Iterator<Item = u32> + '_ {
        self.bitmap.iter(&self.data)
    }
}

/// Membership oracle for the high-card stream-batch scan — "is this `KvId`
/// one of the matched values?", probed once per `KvId` per scanned row.
///
/// A dense bitset over the contiguous `KvId` range `[base, base + width)`
/// the matched values fall in — one field's range (`field_values_or`) or
/// the whole high-card range (`query_positions`). Membership is a direct
/// bit test (no hashing), and at 1 bit/value the set stays cache-resident.
struct KvIdSet {
    bits: Vec<u64>,
    base: u32,
    width: u32,
}

impl KvIdSet {
    fn new(base: u32, width: u32) -> Self {
        Self {
            bits: vec![0u64; width.div_ceil(64) as usize],
            base,
            width,
        }
    }

    /// Record a matched value; `kvid` must lie in `[base, base + width)`.
    fn insert(&mut self, kvid: KvId) {
        let i = (kvid.0 - self.base) as usize;
        self.bits[i / 64] |= 1u64 << (i % 64);
    }

    fn contains(&self, kvid: KvId) -> bool {
        let k = kvid.0;
        if k < self.base || k - self.base >= self.width {
            return false;
        }
        let i = (k - self.base) as usize;
        (self.bits[i / 64] >> (i % 64)) & 1 == 1
    }

    fn is_empty(&self) -> bool {
        self.bits.iter().all(|&word| word == 0)
    }
}
