//! The [`RowIndex`] type — in-memory index built during Phase 1 (reading).
//!
//! Holds four data structures:
//!
//! 1. **KeyValueInterner** — assigns a [`KvSlot`] to each unique `key=value` string.
//! 2. **Vec\<RoaringBitmap\>** — indexed by [`KvSlot`]; each bitmap tracks which
//!    row positions contain that `key=value` pair (insertion order).
//! 3. **Vec\<Vec\<KvSlot\>\>** — row entries: for each row position, the list
//!    of key=value slots it contains (needed for stream-batch serialization in Phase 2).
//! 4. **Vec\<i64\>** — nanosecond timestamp per row position, used to build the
//!    time-sort remap and sparse histogram.

use bumpalo::Bump;
use roaring::RoaringBitmap;
use sfst::{
    DroppedAttributeCounts, Durations, Flags, Histogram, ObservedTimestamps, ParentSpanIds,
    SpanIds, TraceIds,
};

use super::kv_interner::{KeyValueInterner, KvSlot};

/// The output of Phase 1: everything the frame loop extracts from the WAL.
///
/// Bundles the four data structures described in the module doc into a single
/// value, making the Phase 1 → Phase 2 handoff explicit.
pub struct RowIndex<'a> {
    pub kv_interner: KeyValueInterner<'a>,
    /// One bitmap per key=value slot. `kv_bitmaps[slot.idx()]` tracks which row
    /// positions (insertion order) contain that `key=value` pair.
    pub kv_bitmaps: Vec<RoaringBitmap>,
    /// Per-row key=value slots: `row_entries[pos]` lists all key=value slots
    /// for that row's attributes. Used for stream-batch serialization in Phase 2.
    pub row_entries: Vec<Vec<KvSlot>>,
    /// Nanosecond timestamp per row position (insertion order).
    pub timestamps: Vec<i64>,
    /// Optional per-row columns in **insertion order**, parallel to `timestamps`.
    /// Each is **independently** optional — a caller fills whichever it has, in any
    /// combination (or none). `None` for a producer that only feeds rows via
    /// [`row`](RowIndex::row) and sets no columns. Phase 2 reorders each
    /// present column to chronological order and writes its chunk
    /// (`OBTS`/`TRCE`/`SPAN`/`FLAG`/`DRAC`, plus the traces-only `PSPN`/`DURN`) in
    /// the cold region; absent columns write no chunk and add no manifest entry.
    /// Every present column MUST have length equal to the row count
    /// ([`crate::IndexError::ColumnLengthMismatch`] at build).
    pub observed_timestamps: Option<ObservedTimestamps>,
    pub trace_ids: Option<TraceIds>,
    pub span_ids: Option<SpanIds>,
    pub flags: Option<Flags>,
    pub dropped_attribute_counts: Option<DroppedAttributeCounts>,
    /// Span `parent_span_id` column (traces signal; logs leave it `None`).
    pub parent_span_ids: Option<ParentSpanIds>,
    /// Span `duration` column (traces signal; logs leave it `None`).
    pub durations: Option<Durations>,
    /// Producer signal: build the `trace_id` index (`TIDX`) at seal. The index is
    /// built in Phase 2 from the chronological `trace_ids` column, so this MUST
    /// only be set together with `trace_ids`. The logs path leaves it `false` (its
    /// `trace_ids` are near-unique-free log correlation ids, not a trace key).
    pub build_trace_id_index: bool,
    /// The typed schema tree to persist as the v9 field descriptor
    /// (`Metadata.tree`). `Some` when a producer with typed flattening supplies
    /// it (the `ng-index` path) — structure + per-leaf `ValueKind`, leaf stats
    /// filled at build from the per-field cardinality/tier. `None` for a producer
    /// that feeds only raw `(ts, key=value)` rows and supplies no typed tree; the
    /// builder then synthesizes a flat `Str`-typed tree from the derived field
    /// table so every v9 file carries a valid descriptor.
    pub tree: Option<sfst::SchemaTree>,
}

impl<'a> RowIndex<'a> {
    pub fn new(arena: &'a Bump, cardinality_threshold: u32) -> Self {
        Self {
            kv_interner: KeyValueInterner::new(arena, cardinality_threshold),
            kv_bitmaps: Vec::new(),
            row_entries: Vec::new(),
            timestamps: Vec::new(),
            observed_timestamps: None,
            trace_ids: None,
            span_ids: None,
            flags: None,
            dropped_attribute_counts: None,
            parent_span_ids: None,
            durations: None,
            build_trace_id_index: false,
            tree: None,
        }
    }

    /// Total number of rows processed so far.
    pub fn num_rows(&self) -> usize {
        self.row_entries.len()
    }

    /// Build a permutation table mapping insertion-order positions to
    /// time-sorted positions.
    ///
    /// All bitmaps in Phase 2 are translated from insertion order to
    /// chronological order so contiguous position ranges = contiguous time.
    pub fn time_order(&self) -> TimeOrder {
        build_time_order(&self.timestamps)
    }

    /// Build a sparse histogram from the timestamps in chronological order.
    pub fn sparse_histogram(&self, time_order: &TimeOrder) -> Histogram {
        build_sparse_histogram(&self.timestamps, time_order)
    }

    /// Resolve a key=value slot to its string.
    pub fn resolve(&self, slot: KvSlot) -> &str {
        self.kv_interner.resolve(slot)
    }

    /// Get the roaring bitmap for a key=value slot.
    pub fn bitmap(&self, slot: KvSlot) -> &RoaringBitmap {
        &self.kv_bitmaps[slot.idx()]
    }

    /// Low-cardinality fields (< threshold), sorted by field name.
    pub fn low_fields(&self) -> Vec<(&str, &[KvSlot])> {
        self.kv_interner.low_fields()
    }

    /// Mid-cardinality fields ([threshold, 10*threshold)), sorted by field name.
    pub fn mid_fields(&self) -> Vec<(&str, &[KvSlot])> {
        self.kv_interner.mid_fields()
    }

    /// High-cardinality fields (>= 10*threshold), sorted by field name.
    pub fn high_fields(&self) -> Vec<(&str, &[KvSlot])> {
        self.kv_interner.high_fields()
    }

    /// Tier-aligned assignment of key=value IDs.
    pub fn tier_assignment(&self) -> [Vec<KvSlot>; 3] {
        self.kv_interner.tier_assignment()
    }

    /// Cardinality threshold for field classification.
    pub fn cardinality_threshold(&self) -> u32 {
        self.kv_interner.cardinality_threshold()
    }

    /// Ensure kv_bitmaps vec has an entry for the given key=value ID.
    pub fn ensure_bitmap(&mut self, kv_slot: KvSlot) {
        if kv_slot.idx() >= self.kv_bitmaps.len() {
            self.kv_bitmaps
                .resize_with(kv_slot.idx() + 1, RoaringBitmap::new);
        }
    }
}

/// Row ingestion: a producer interns each `key=value` pair to a [`KvSlot`]
/// and hands the slots back per row. Tokens are interner slots, and each
/// row lands in the four Phase-1 structures.
impl<'a> RowIndex<'a> {
    /// Hash-only fast-path lookup: returns the slot a `key=value` was
    /// interned under if the producer's `xxhash64` is unambiguous, letting
    /// the caller skip formatting the string. Returns `None` (forcing
    /// [`intern`](Self::intern)) for an unseen or collision-ambiguous hash —
    /// the interner answers `None` for any hash it has seen more than one
    /// distinct string for, so a hit is always collision-safe.
    pub fn lookup_hash(&mut self, hash: u64) -> Option<KvSlot> {
        self.kv_interner.lookup_hash(hash)
    }

    /// Intern a `key=value` pair, deduplicating by the **full string**.
    /// `hash` is the producer's pre-computed `xxhash64(kv)` when available
    /// (collision-safe: a hash hit re-checks the stored string), `None`
    /// otherwise.
    pub fn intern(&mut self, hash: Option<u64>, kv: &str) -> KvSlot {
        match hash {
            Some(h) => self.kv_interner.intern_with_hash(h, kv),
            None => self.kv_interner.intern(kv),
        }
    }

    /// One row: its timestamp and the interned slots of every
    /// `key=value` pair it carries (duplicates allowed — a multi-valued
    /// field emits one pair per value).
    pub fn row(&mut self, ts_ns: i64, tokens: &[KvSlot]) {
        let pos = self.row_entries.len() as u32;
        self.timestamps.push(ts_ns);
        for &kv_slot in tokens {
            self.ensure_bitmap(kv_slot);
            self.kv_bitmaps[kv_slot.idx()].insert(pos);
        }
        self.row_entries.push(tokens.to_vec());
    }
}

// ---------------------------------------------------------------------------
// Time-sort remap and sparse histogram
// ---------------------------------------------------------------------------

/// Bidirectional mapping between insertion order and chronological order.
///
/// Built from the nanosecond timestamps collected during Phase 1. Used in
/// Phase 2 to translate all bitmaps and row entries from insertion order to
/// chronological order, so contiguous position ranges correspond to
/// contiguous time windows.
pub struct TimeOrder {
    /// `sorted_position[insertion_pos] = sorted_pos`
    sorted_position: Vec<u32>,
    /// `insertion_position[sorted_pos] = insertion_pos`
    insertion_position: Vec<u32>,
}

impl TimeOrder {
    /// Map an insertion-order position to its chronological position.
    #[inline]
    pub fn to_sorted(&self, insertion_pos: u32) -> u32 {
        self.sorted_position[insertion_pos as usize]
    }

    /// Map a chronological position back to its insertion-order position.
    #[inline]
    pub fn to_insertion(&self, sorted_pos: u32) -> u32 {
        self.insertion_position[sorted_pos as usize]
    }

    /// Iterate insertion-order positions in chronological order.
    pub fn iter_by_time(&self) -> impl Iterator<Item = u32> + '_ {
        self.insertion_position.iter().copied()
    }

    /// Total number of row positions (universe size).
    pub fn len(&self) -> u32 {
        self.sorted_position.len() as u32
    }
}

/// Build a permutation table that maps insertion-order positions to
/// time-sorted positions.
///
/// During indexing, each row gets a position based on the order it was read
/// from the WAL (insertion order). But for time-range queries we need
/// positions to correspond to chronological order, so that a contiguous
/// range of positions like `[100..200]` maps to a contiguous time window.
///
/// # Example
///
/// Suppose we indexed 5 rows with these timestamps:
///
/// ```text
///   insertion pos:  0     1     2     3     4
///   timestamp:     10:03  10:01  10:05  10:00  10:02
/// ```
///
/// After sorting by timestamp, the chronological order is:
///
/// ```text
///   sorted pos:     0      1      2      3      4
///   original pos:   3      1      4      0      2
///   timestamp:     10:00  10:01  10:02  10:03  10:05
/// ```
///
/// This gives us the remap table `remap[original] = sorted`:
///
/// ```text
///   remap[0] = 3   (10:03 is 4th chronologically)
///   remap[1] = 1   (10:01 is 2nd chronologically)
///   remap[2] = 4   (10:05 is 5th chronologically)
///   remap[3] = 0   (10:00 is 1st chronologically)
///   remap[4] = 2   (10:02 is 3rd chronologically)
/// ```
///
/// A bitmap that had bits `{0, 2}` (rows at 10:03 and 10:05 in insertion
/// order) becomes `{3, 4}` (positions 3 and 4 in chronological order —
/// the last two events).
///
fn build_time_order(timestamps: &[i64]) -> TimeOrder {
    let n = timestamps.len();

    let mut insertion_position: Vec<u32> = (0..n as u32).collect();
    insertion_position.sort_by_key(|&i| timestamps[i as usize]);

    let mut sorted_position = vec![0u32; n];
    for (sorted_pos, &original_pos) in insertion_position.iter().enumerate() {
        sorted_position[original_pos as usize] = sorted_pos as u32;
    }

    TimeOrder {
        sorted_position,
        insertion_position,
    }
}

/// Build a sparse histogram from chronologically sorted row timestamps.
///
/// Each entry records (second, running_count) — the cumulative number of
/// rows up to and including that second. One entry per second that has at
/// least one row.
fn build_sparse_histogram(timestamps: &[i64], time_order: &TimeOrder) -> Histogram {
    if timestamps.is_empty() {
        return Histogram {
            timestamps: Vec::new(),
            counts: Vec::new(),
        };
    }

    let mut hist_ts: Vec<u32> = Vec::new();
    let mut hist_counts: Vec<u32> = Vec::new();
    let mut prev_sec = u32::MAX;

    for (i, ins_pos) in time_order.iter_by_time().enumerate() {
        let sec = (timestamps[ins_pos as usize] / 1_000_000_000) as u32;
        if sec != prev_sec {
            if prev_sec != u32::MAX {
                hist_ts.push(prev_sec);
                hist_counts.push(i as u32);
            }
            prev_sec = sec;
        }
    }

    // Emit final bucket.
    hist_ts.push(prev_sec);
    hist_counts.push(timestamps.len() as u32);

    Histogram {
        timestamps: hist_ts,
        counts: hist_counts,
    }
}
