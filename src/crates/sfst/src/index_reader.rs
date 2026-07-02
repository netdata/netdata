//! The query-plane reader for the split-FST index format.
//!
//! Opens an `.sfst` file (typically via mmap) and provides query methods
//! that follow the access pattern described in `sfst/FORMAT.md`:
//!
//! 1. Decode SUMR + META + PRIM eagerly on open (always needed).
//! 2. Look up low-card `key=value` pairs in the primary FST → bitmap.
//! 3. Load secondary chunks on demand (mid-card FST or high-card blob).
//! 4. Load per-stream log entries for attribute resolution.

use std::collections::{HashMap, HashSet};

use crate::PrefixMap;
use crate::reader::ChunkReader;

use crate::{
    BitmapValue, Bucket, FacetResult, FieldEntry, FieldTier, Filter, Grid, Histogram, IdRanges,
    KvId, Matcher, Metadata, SpanId, Summary, Timeline, Timestamps, TraceId,
};

/// A successfully opened split-FST index.
///
/// Holds the mmap'd data, the deserialized summary, and the primary
/// FST (both eagerly loaded on open since every query needs them).
/// [`Metadata`] is cached on the underlying chunk reader and
/// surfaced via [`metadata`](Self::metadata).
pub struct IndexReader<'a> {
    sfst: ChunkReader<'a>,
    summary: Summary,
    primary: PrefixMap<BitmapValue>,
}

/// One materialized span of a trace reconstructed by [`IndexReader::trace_by_id`]:
/// its ids, timing, per-row scalars, and attribute facets (`name`, `kind`,
/// `status_code`, `attributes.*`) as flat `(key, value)` pairs.
#[derive(Debug, Clone, PartialEq, Eq)]
pub struct TraceSpan {
    pub span_id: SpanId,
    pub parent_span_id: SpanId,
    pub start_ns: i64,
    pub duration_ns: i64,
    /// W3C trace flags (the low byte carries the sampled bit).
    pub flags: u32,
    pub dropped_attributes_count: u32,
    pub fields: Vec<(String, String)>,
}

/// A trace reconstructed by [`IndexReader::trace_by_id`]: the trace's spans plus the
/// in-memory parent/child tree over them.
///
/// The tree is a flat adjacency (no owned nesting, no recursion): walk from
/// [`roots`](Self::roots) via [`children`](Self::children). Because a trace's spans
/// can scatter across files/time (and this reader sees only one file), a span whose
/// `parent_span_id` is unset OR absent from this set is a **root** — so a partial
/// trace still forms a forest rather than dropping spans.
///
/// Normal traces are a forest. **Every span is reachable from some root** even for
/// pathological input: a parent cycle (or a cyclic component with no external entry)
/// has no natural root, so the earliest still-unreached span is promoted to a root
/// until all spans are reachable. In that case [`children`](Self::children) still
/// contains the cycle's edges, so a walker MUST guard against revisiting a node.
#[derive(Debug, Clone, PartialEq, Eq)]
pub struct Trace {
    /// The trace's spans, sorted by `start_ns` (ties broken by `span_id`) — the
    /// clock-skew-safe display order. Duplicate `span_id`s (resends) are collapsed
    /// to the first seen.
    pub spans: Vec<TraceSpan>,
    /// Indices into [`spans`](Self::spans) of the root spans (unset/missing/self parent).
    pub roots: Vec<usize>,
    /// `span_id` → indices into [`spans`](Self::spans) of its direct children.
    pub children: HashMap<SpanId, Vec<usize>>,
}

impl<'a> IndexReader<'a> {
    /// Open a split-FST index from a byte slice (typically an mmap).
    ///
    /// Immediately deserializes the summary, metadata, and primary FST.
    /// Metadata stays cached on the underlying chunk reader.
    pub fn open(data: &'a [u8]) -> Result<Self, crate::Error> {
        let sfst = ChunkReader::open(data)?;
        let summary = sfst.summary()?;
        // Force the metadata + derived-field-table caches so subsequent
        // accessors are infallible.
        sfst.metadata()?;
        sfst.fields()?;
        let primary = sfst.primary()?;
        Ok(Self {
            sfst,
            summary,
            primary,
        })
    }

    /// The cheap summary fields (timestamps, record count, opaque
    /// `content_meta` identity). The partition key is not here — it lives in
    /// the file's `FileId` (filename), the single source of truth.
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

    /// The field table — the flat view derived from the schema tree
    /// (`metadata().tree`), cached on the underlying chunk reader (forced
    /// at [`open`](Self::open)).
    pub fn field_table(&self) -> &crate::FieldTable {
        self.sfst
            .fields()
            .expect("field table derived + cached at IndexReader::open")
    }

    /// The typed schema tree (the on-disk field descriptor).
    pub fn tree(&self) -> &crate::SchemaTree {
        &self.metadata().tree
    }

    /// Byte span of the cold suffix (optional per-row columns + optional
    /// `trace_id` index + mid/high field chunks + stream batches) a query
    /// releases from the page cache once done. See
    /// the chunk reader's cold-region rule.
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
    /// want the materialized `Vec<Vec<KvId>>` rather than walking
    /// [`StreamBatch`](crate::StreamBatch) rows.
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

    // ── Per-row columns ───────────────────────────────────────────
    //
    // Each column is independently optional per file (declared in the META
    // manifest); an accessor errors when its column is absent. All columns
    // are chronologically ordered, parallel-indexed to `load_timestamps`.

    /// The per-row columns this file carries (the META manifest).
    pub fn columns_table(&self) -> &crate::ColumnsTable {
        self.sfst
            .columns_table()
            .expect("metadata cached at IndexReader::open")
    }

    /// Whether the file carries any per-row column chunk.
    pub fn has_per_row_columns(&self) -> bool {
        !self.columns_table().is_empty()
    }

    /// The per-row observed-timestamps column (`OBTS`).
    pub fn observed_timestamps(&self) -> Result<crate::ObservedTimestamps, crate::Error> {
        self.sfst.observed_timestamps()
    }

    /// The per-row trace-ids column (`TRCE`).
    pub fn trace_ids(&self) -> Result<crate::TraceIds, crate::Error> {
        self.sfst.trace_ids()
    }

    /// The per-row span-ids column (`SPAN`).
    pub fn span_ids(&self) -> Result<crate::SpanIds, crate::Error> {
        self.sfst.span_ids()
    }

    /// The per-row flags column (`FLAG`).
    pub fn flags(&self) -> Result<crate::Flags, crate::Error> {
        self.sfst.flags()
    }

    /// The per-row dropped-attributes-count column (`DRAC`).
    pub fn dropped_attribute_counts(&self) -> Result<crate::DroppedAttributeCounts, crate::Error> {
        self.sfst.dropped_attribute_counts()
    }

    /// The per-row parent-span-ids column (`PSPN`, traces signal).
    pub fn parent_span_ids(&self) -> Result<crate::ParentSpanIds, crate::Error> {
        self.sfst.parent_span_ids()
    }

    /// The per-row span-duration column (`DURN`, traces signal).
    pub fn durations(&self) -> Result<crate::Durations, crate::Error> {
        self.sfst.durations()
    }

    /// Whether the file carries the optional `trace_id` index (`TIDX`).
    pub fn has_trace_id_index(&self) -> bool {
        self.sfst.has_trace_id_index()
    }

    /// The `trace_id` index. Positions it yields index the chronological
    /// [`trace_ids`](Self::trace_ids) column.
    pub fn trace_id_index(&self) -> Result<crate::TraceIdIndex, crate::Error> {
        self.sfst.trace_id_index()
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

        // Mid/high-card: iterate secondary chunks in field_table order, each at
        // its tier-relative chunk index.
        for (field, ti) in field_table_tiered(field_table) {
            match field.tier {
                FieldTier::Low => continue,
                FieldTier::Mid => {
                    let fst = self.sfst.mid_field(ti)?;
                    fst.for_each(|key, _| {
                        if kv_id < table.len() {
                            table[kv_id] = String::from_utf8_lossy(key).into_owned();
                        }
                        kv_id += 1;
                    });
                }
                FieldTier::High => {
                    let hf = self.sfst.high_field(ti)?;
                    for key in hf.keys() {
                        if kv_id < table.len() {
                            table[kv_id] = String::from_utf8_lossy(key).into_owned();
                        }
                        kv_id += 1;
                    }
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

    /// Reconstruct one trace by its `trace_id`: look up the trace's span row
    /// positions via the `TIDX` index, materialize each span (ids + timing +
    /// attribute facets), and rebuild the parent/child tree in memory.
    ///
    /// Returns an empty [`Trace`] if the id is absent. Errors if the file carries no
    /// `trace_id` index (not a traces file).
    ///
    /// The tree build is **iterative** and defensive (a trace's spans can be partial
    /// or malformed): spans are sorted by start time (clock-skew-safe), duplicate
    /// `span_id`s are collapsed, and a span whose parent is unset, self-referential,
    /// or absent from this file becomes a root — yielding a forest, never a dropped
    /// span or an unbounded recursion.
    pub fn trace_by_id(&self, trace_id: TraceId) -> Result<Trace, crate::Error> {
        let index = self.sfst.trace_id_index()?;
        let trace_ids = self.sfst.trace_ids()?;
        let positions = index.positions(trace_id, &trace_ids);
        if positions.is_empty() {
            return Ok(Trace {
                spans: Vec::new(),
                roots: Vec::new(),
                children: HashMap::new(),
            });
        }

        let span_ids = self.sfst.span_ids()?;
        let parents = self.sfst.parent_span_ids()?;
        let durations = self.sfst.durations()?;
        let flags = self.sfst.flags()?;
        let dropped = self.sfst.dropped_attribute_counts()?;
        // `materialize_rows` returns one row per position and already validated each
        // `pos < record_count`; every per-row column has `record_count` entries (a
        // build-time invariant, CRC-guarded), so indexing them at `pos` is in-bounds.
        let rows = self.materialize_rows(positions)?;

        // Assemble spans, collapsing duplicate span_ids (resends) to the first seen.
        // All positions share `trace_id`, so dedup by span_id == dedup by
        // (trace_id, span_id).
        let mut seen: HashSet<SpanId> = HashSet::new();
        let mut spans: Vec<TraceSpan> = Vec::with_capacity(positions.len());
        for (i, &pos) in positions.iter().enumerate() {
            let span_id = span_ids.get(pos as usize);
            // Dedup real span ids (resends) to the first seen, but NEVER collapse
            // UNSET (all-zero) ids: those are distinct spans that merely lack a valid
            // span_id (malformed/absent), not one span sent twice.
            if !span_id.is_unset() && !seen.insert(span_id) {
                continue;
            }
            spans.push(TraceSpan {
                span_id,
                parent_span_id: parents.get(pos as usize),
                start_ns: rows[i].timestamp_ns,
                duration_ns: durations.0[pos as usize],
                flags: flags.0[pos as usize],
                dropped_attributes_count: dropped.0[pos as usize],
                fields: rows[i].fields.clone(),
            });
        }

        // Clock-skew-safe display order; span_id breaks ties deterministically.
        spans.sort_by(|a, b| {
            a.start_ns
                .cmp(&b.start_ns)
                .then_with(|| a.span_id.cmp(&b.span_id))
        });

        // Iterative tree build over the sorted spans: parent present in this set →
        // edge; otherwise (unset / missing / self) → root.
        // Exclude UNSET span ids: a parent lookup below guards `!is_unset()` first, so
        // UNSET is never queried here — indexing it would only cause a meaningless
        // last-writer-wins overwrite when several spans lack a valid span_id.
        let idx_of: HashMap<SpanId, usize> = spans
            .iter()
            .enumerate()
            .filter(|(_, s)| !s.span_id.is_unset())
            .map(|(i, s)| (s.span_id, i))
            .collect();
        let mut children: HashMap<SpanId, Vec<usize>> = HashMap::new();
        let mut roots: Vec<usize> = Vec::new();
        for (i, s) in spans.iter().enumerate() {
            let has_parent = !s.parent_span_id.is_unset()
                && s.parent_span_id != s.span_id
                && idx_of.contains_key(&s.parent_span_id);
            if has_parent {
                children.entry(s.parent_span_id).or_default().push(i);
            } else {
                roots.push(i);
            }
        }

        // Reachability guard: guarantee every span is reachable from some root, so a
        // root-first walk visits all of them. Spans with an unset/missing/self parent
        // are already roots; but a pathological parent *cycle* (or a cyclic component
        // with no external entry) would leave its members reachable from no root even
        // though they are present in `spans`. Promote the earliest unreached span to a
        // root until everything is reachable. O(V+E): each node is marked once (the
        // `reachable` guard), each edge is followed once, and `cursor` scans the span
        // list monotonically across all rounds.
        let mut reachable = vec![false; spans.len()];
        let mut stack: Vec<usize> = roots.clone();
        let mut cursor = 0usize;
        loop {
            while let Some(i) = stack.pop() {
                if reachable[i] {
                    continue;
                }
                reachable[i] = true;
                if let Some(kids) = children.get(&spans[i].span_id) {
                    stack.extend(kids.iter().copied().filter(|&c| !reachable[c]));
                }
            }
            while cursor < spans.len() && reachable[cursor] {
                cursor += 1;
            }
            if cursor == spans.len() {
                break;
            }
            roots.push(cursor); // earliest (start-sorted) unreached span
            stack.push(cursor);
        }

        Ok(Trace {
            spans,
            roots,
            children,
        })
    }

    /// Resolve a **single** field's values at `positions`, decoding only that
    /// field's chunk. Thin wrapper over [`materialize_fields`](Self::materialize_fields).
    pub fn materialize_field(
        &self,
        field: &str,
        positions: &[u32],
    ) -> Result<Vec<Vec<String>>, crate::Error> {
        Ok(self
            .materialize_fields(&[field], positions)?
            .into_iter()
            .next()
            .expect("one field in → one column out"))
    }

    /// Resolve several fields' values at `positions`, decoding only those
    /// fields' chunks — never the whole reverse string table.
    ///
    /// Returns one column per input field (same order); each column has one
    /// entry per input position (same order). An entry is that field's value(s)
    /// at the position: empty when the field is absent there, more than one for
    /// a multi-valued (array-collapsed `[]`) field. This is the column-direct
    /// counterpart to [`materialize_rows`](Self::materialize_rows): cost scales
    /// with the projected fields, not the file's whole attribute space.
    ///
    /// Low/mid fields scatter from their own per-value position bitmaps;
    /// high-card fields share a **single** stream-batch scan (the SB chunks are
    /// read once regardless of how many high-card fields are projected). Each
    /// field's `KvId` range comes from `META` cardinalities
    /// (`high_kv_id`); no chunk is decoded just to locate
    /// it. An absent field yields an all-empty column.
    pub fn materialize_fields(
        &self,
        fields: &[&str],
        positions: &[u32],
    ) -> Result<Vec<Vec<Vec<String>>>, crate::Error> {
        let mut out: Vec<Vec<Vec<String>>> = fields
            .iter()
            .map(|_| vec![Vec::new(); positions.len()])
            .collect();

        // Matched positions are a set, so each maps to exactly one output slot.
        let slot: std::collections::HashMap<u32, usize> =
            positions.iter().enumerate().map(|(i, &p)| (p, i)).collect();

        // High-card fields are resolved together in one stream-batch scan; this
        // collects their disjoint KvId ranges + value arenas, tagged with the
        // output column they fill.
        struct HighScan {
            out_idx: usize,
            base: u32,
            span: u32,
            values: Vec<String>,
        }
        let mut highs: Vec<HighScan> = Vec::new();

        for (fp, &field) in fields.iter().enumerate() {
            let prefix_len = field.len() + 1; // strip "field=" → the value bytes
            match self.locate_field(field) {
                // Absent → out[fp] stays all-empty.
                None => {}
                Some(FieldLocation::Low) => {
                    let prefix = format!("{field}=");
                    for (kv_bytes, bv) in self.primary.prefix_pairs(prefix.as_bytes()) {
                        let value = String::from_utf8_lossy(&kv_bytes[prefix_len..]).into_owned();
                        for p in PosSet::from_value(bv).iter() {
                            if let Some(&i) = slot.get(&p) {
                                out[fp][i].push(value.clone());
                            }
                        }
                    }
                }
                Some(FieldLocation::Mid(idx)) => {
                    // Every key in a mid chunk belongs to this one field.
                    let chunk = self.sfst.mid_field(idx)?;
                    chunk.for_each(|kv_bytes, bv| {
                        let value = String::from_utf8_lossy(&kv_bytes[prefix_len..]).into_owned();
                        for p in PosSet::from_value(bv).iter() {
                            if let Some(&i) = slot.get(&p) {
                                out[fp][i].push(value.clone());
                            }
                        }
                    });
                }
                Some(FieldLocation::High(idx)) => {
                    let hf = self.sfst.high_field(idx)?;
                    highs.push(HighScan {
                        out_idx: fp,
                        base: self.high_kv_id(idx, 0).0,
                        span: hf.len() as u32,
                        values: hf
                            .keys()
                            .map(|k| String::from_utf8_lossy(&k[prefix_len..]).into_owned())
                            .collect(),
                    });
                }
            }
        }

        // One stream-batch pass resolving every projected high-card field.
        if !highs.is_empty() {
            let total = self.summary.record_count;
            let batch_size = crate::stream_batch_size(total);
            let mut loaded: Vec<Option<crate::StreamBatch>> =
                (0..self.num_stream_batches()).map(|_| None).collect();

            for (i, &p) in positions.iter().enumerate() {
                if p >= total {
                    continue;
                }
                let b = (p / batch_size) as usize;
                if loaded[b].is_none() {
                    loaded[b] = Some(self.sfst.stream_batch(b as u8)?);
                }
                let batch = loaded[b].as_ref().unwrap();
                let local = (p % batch_size) as usize;
                for id in batch.row(local) {
                    // Ranges are disjoint, so the first containing field wins.
                    for h in &highs {
                        if id.0 >= h.base && id.0 < h.base + h.span {
                            out[h.out_idx][i].push(h.values[(id.0 - h.base) as usize].clone());
                            break;
                        }
                    }
                }
            }
        }

        Ok(out)
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
    /// `unset`.
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

    /// Per-bucket matched counts for `grid` — the field-less timeline backing
    /// `GROUP BY date_bin(timestamp)`. `totals[i]` is the number of logs
    /// matching `filter` whose timestamp falls in bucket `i`; buckets past the
    /// file's time range are zero (the grid clamps naturally). This is the
    /// time-axis analogue of [`matched_count`](Self::matched_count) — one bucket
    /// edge resolution, then a `range_cardinality` per bucket.
    pub fn timeline_totals(
        &self,
        filter: &BitmapFilter,
        grid: Grid,
    ) -> Result<Vec<u64>, crate::Error> {
        let full = filter.full();
        Ok(self
            .load_timestamps()?
            .bucket_ranges(grid)
            .into_iter()
            .map(|(lo, hi)| full.range_cardinality(lo, hi))
            .collect())
    }

    // ── Query helpers (private) ──────────────────────────────────────

    /// Locate a field by name and return its tier + tier-relative chunk
    /// index. Returns `None` if the field is absent from this file.
    fn locate_field(&self, field_name: &str) -> Option<FieldLocation> {
        field_table_tiered(self.field_table())
            .find(|(field, _)| field.name == field_name)
            .map(|(field, ti)| match field.tier {
                FieldTier::Low => FieldLocation::Low,
                FieldTier::Mid => FieldLocation::Mid(ti),
                FieldTier::High => FieldLocation::High(ti),
            })
    }

    /// Compute the on-disk `KvId` for the `local`-th value of high-card
    /// chunk `high_idx`. High-card KvIds are `mid_end + (cumulative
    /// high-card cardinalities before this field) + local`.
    fn high_kv_id(&self, high_idx: u16, local: usize) -> KvId {
        let id_ranges = &self.metadata().id_ranges;
        let mut kv = id_ranges.mid_end.0;
        let mut current = 0u16;
        for field in self.field_table().iter() {
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
                // [base, base + cardinality); `base` is fixed for the field, so
                // resolve it once rather than per matched value.
                let base = self.high_kv_id(idx, 0).0;
                let mut targets = KvIdSet::new(base, hf.len() as u32);
                let mut combined_mask: u8 = 0;
                for value in &exacts {
                    let kv = format!("{field}={value}");
                    if let Ok(local) = hf.binary_search(kv.as_bytes()) {
                        targets.insert(KvId(base + local as u32));
                        combined_mask |= hf.masks[local];
                    }
                }
                if !patterns.is_empty() {
                    for (local, key) in hf.keys().enumerate() {
                        if value_matches(key) {
                            targets.insert(KvId(base + local as u32));
                            combined_mask |= hf.masks[local];
                        }
                    }
                }
                self.scan_high_positions(&targets, combined_mask, total)
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
        // Matched high-card KvIds span the whole high-card range
        // [mid_end, high_end) (this scan accumulates across all high fields).
        let id_ranges = &self.metadata().id_ranges;
        let mut targets = KvIdSet::new(
            id_ranges.mid_end.0,
            id_ranges.high_end.0 - id_ranges.mid_end.0,
        );
        let mut combined_mask: u8 = 0;
        for (field, ti) in field_table_tiered(self.field_table()) {
            match field.tier {
                FieldTier::Low => {}
                FieldTier::Mid => {
                    let chunk = self.sfst.mid_field(ti)?;
                    chunk.for_each(|kv_bytes, bv| {
                        if query.is_match(kv_bytes) {
                            result.or_assign(&PosSet::from_value(bv));
                        }
                    });
                }
                FieldTier::High => {
                    let hf = self.sfst.high_field(ti)?;
                    let base = self.high_kv_id(ti, 0).0;
                    for (local, key) in hf.keys().enumerate() {
                        if query.is_match(key) {
                            targets.insert(KvId(base + local as u32));
                            combined_mask |= hf.masks[local];
                        }
                    }
                }
            }
        }

        if !targets.is_empty() {
            result.or_assign(&self.scan_high_positions(&targets, combined_mask, total)?);
        }

        Ok(result)
    }

    /// Positions of rows containing any KvId in `targets`, scanning only the
    /// stream batches selected by `mask` (bit `b` set ⇒ batch `b` may hold a
    /// target — the OR of the matched values' per-value batch masks). Empty
    /// `targets` yields an empty set. Shared by the high-card paths of
    /// [`field_values_or`](Self::field_values_or) and
    /// [`query_positions`](Self::query_positions).
    fn scan_high_positions(
        &self,
        targets: &KvIdSet,
        mask: u8,
        total: u32,
    ) -> Result<PosSet, crate::Error> {
        if targets.is_empty() {
            return Ok(PosSet::empty(total));
        }
        let batch_size = crate::stream_batch_size(total);
        let num_batches = crate::num_stream_batches(total);
        let mut positions: Vec<u32> = Vec::new();
        for b in 0..num_batches {
            if (mask >> b) & 1 == 0 {
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
    /// Invariant: the writer builds every value bitmap at the same
    /// `universe_size` (`== record_count`), which is what lets the resulting set
    /// combine with `range`/`full`/other values via `and`/`or` — those require
    /// matching universes (a mismatch is only a debug assert, so it would be
    /// silently wrong in release).
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
        debug_assert!(
            kvid.0 >= self.base && kvid.0 - self.base < self.width,
            "KvId {} outside [{}, {})",
            kvid.0,
            self.base,
            self.base + self.width
        );
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

/// Pair each field with its **tier-relative** index (0-based within its own
/// tier). The mid/high positions are exactly the chunk indices the reader loads,
/// so this centralizes the running-index bookkeeping the tier-dispatch sites
/// would otherwise each repeat.
fn field_table_tiered(fields: &[FieldEntry]) -> impl Iterator<Item = (&FieldEntry, u16)> {
    let (mut low, mut mid, mut high) = (0u16, 0u16, 0u16);
    fields.iter().map(move |field| {
        let idx = match field.tier {
            FieldTier::Low => {
                let i = low;
                low += 1;
                i
            }
            FieldTier::Mid => {
                let i = mid;
                mid += 1;
                i
            }
            FieldTier::High => {
                let i = high;
                high += 1;
                i
            }
        };
        (field, idx)
    })
}

#[cfg(test)]
mod tests;
