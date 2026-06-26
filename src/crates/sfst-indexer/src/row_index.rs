//! The [`RowIndex`] type — in-memory index built during Phase 1 (reading).
//!
//! Holds four data structures:
//!
//! 1. **KeyValueInterner** — assigns a [`KvSlot`] to each unique `key=value` string.
//! 2. **Vec\<RoaringBitmap\>** — indexed by [`KvSlot`]; each bitmap tracks which
//!    log positions contain that `key=value` pair (insertion order).
//! 3. **Vec\<Vec\<KvSlot\>\>** — log entries: for each log position, the list
//!    of key=value slots it contains (needed for per-stream serialization in Phase 2).
//! 4. **Vec\<i64\>** — nanosecond timestamp per log position, used to build the
//!    time-sort remap and sparse histogram.

use crate::IndexError;
use bumpalo::Bump;
use otel_logs_identity::ServiceStream;
use roaring::RoaringBitmap;
use sfst::Histogram;

use super::kv_interner::{KeyValueInterner, KvSlot};

/// The output of Phase 1: everything the frame loop extracts from the WAL.
///
/// Bundles the four data structures described in the module doc into a single
/// value, making the Phase 1 → Phase 2 handoff explicit.
pub struct RowIndex<'a> {
    pub kv_interner: KeyValueInterner<'a>,
    /// One bitmap per key=value slot. `kv_bitmaps[slot.idx()]` tracks which log
    /// positions (insertion order) contain that `key=value` pair.
    pub kv_bitmaps: Vec<RoaringBitmap>,
    /// Per-log key=value slots: `log_entries[log_pos]` lists all key=value slots
    /// for that log's attributes. Used for per-stream serialization in Phase 2.
    pub log_entries: Vec<Vec<KvSlot>>,
    /// Nanosecond timestamp per log position (insertion order).
    pub timestamps: Vec<i64>,
}

impl<'a> RowIndex<'a> {
    pub fn new(arena: &'a Bump, cardinality_threshold: u32) -> Self {
        Self {
            kv_interner: KeyValueInterner::new(arena, cardinality_threshold),
            kv_bitmaps: Vec::new(),
            log_entries: Vec::new(),
            timestamps: Vec::new(),
        }
    }

    /// Total number of logs processed so far.
    pub fn num_logs(&self) -> usize {
        self.log_entries.len()
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

    /// All interned strings, ordered by KvSlot.
    pub fn strings(&self) -> &[&str] {
        self.kv_interner.strings()
    }

    /// Ensure kv_bitmaps vec has an entry for the given key=value ID.
    pub fn ensure_bitmap(&mut self, kv_slot: KvSlot) {
        if kv_slot.idx() >= self.kv_bitmaps.len() {
            self.kv_bitmaps
                .resize_with(kv_slot.idx() + 1, RoaringBitmap::new);
        }
    }

    /// Extract the file's single `(service.namespace, service.name)` stream.
    ///
    /// Walks the interner once for `service.name=X` and `service.namespace=Y`
    /// entries. The ingestor partitions WAL files by `ns_hash` and rejects
    /// writes whose `(namespace, name)` doesn't match the canonical pair
    /// for that hash, so every WAL file should expose at most one of each.
    /// Missing values default to the empty string (the catch-all stream).
    ///
    /// Returns [`IndexError::MultipleStreams`] if more than one distinct
    /// value is found for either key — that means an `ns_hash` collision
    /// slipped past the ingestor's check and the file has no single
    /// stream identity to attach to the SFST.
    pub fn service_stream(&self) -> Result<ServiceStream, IndexError> {
        let mut namespaces: Vec<&str> = Vec::new();
        let mut names: Vec<&str> = Vec::new();

        for kv_pair in self.kv_interner.strings().iter() {
            if let Some(name) = kv_pair.strip_prefix("service.name=") {
                names.push(name);
            } else if let Some(namespace) = kv_pair.strip_prefix("service.namespace=") {
                namespaces.push(namespace);
            }
        }

        if namespaces.len() > 1 || names.len() > 1 {
            return Err(IndexError::MultipleStreams {
                namespaces: namespaces.into_iter().map(String::from).collect(),
                names: names.into_iter().map(String::from).collect(),
            });
        }

        Ok(ServiceStream::new(
            namespaces.first().copied().unwrap_or(""),
            names.first().copied().unwrap_or(""),
        ))
    }
}

/// The indexer is one consumer of the shared frame decode
/// (`wal_otap`'s frame decode): tokens are interner
/// slots, and each decoded row lands in the four Phase-1 structures.
impl<'a> wal_otap::KvSink for RowIndex<'a> {
    type Token = KvSlot;

    fn lookup_hash(&mut self, hash: u64) -> Option<KvSlot> {
        self.kv_interner.lookup_hash(hash)
    }

    fn intern(&mut self, hash: Option<u64>, kv: &str) -> KvSlot {
        match hash {
            Some(h) => self.kv_interner.intern_with_hash(h, kv),
            None => self.kv_interner.intern(kv),
        }
    }

    fn reserve_rows(&mut self, additional: usize) {
        self.log_entries.reserve(additional);
        self.timestamps.reserve(additional);
    }

    fn row(&mut self, ts_ns: i64, tokens: &[KvSlot]) {
        let log_pos = self.log_entries.len() as u32;
        self.timestamps.push(ts_ns);
        for &kv_slot in tokens {
            self.ensure_bitmap(kv_slot);
            self.kv_bitmaps[kv_slot.idx()].insert(log_pos);
        }
        self.log_entries.push(tokens.to_vec());
    }
}

// ---------------------------------------------------------------------------
// Time-sort remap and sparse histogram
// ---------------------------------------------------------------------------

/// Bidirectional mapping between insertion order and chronological order.
///
/// Built from the nanosecond timestamps collected during Phase 1. Used in
/// Phase 2 to translate all bitmaps and log entries from insertion order to
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

    /// Total number of log positions (universe size).
    pub fn len(&self) -> u32 {
        self.sorted_position.len() as u32
    }
}

/// Build a permutation table that maps insertion-order positions to
/// time-sorted positions.
///
/// During indexing, each log gets a position based on the order it was read
/// from the WAL (insertion order). But for time-range queries we need
/// positions to correspond to chronological order, so that a contiguous
/// range of positions like `[100..200]` maps to a contiguous time window.
///
/// # Example
///
/// Suppose we indexed 5 logs with these timestamps:
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
/// A bitmap that had bits `{0, 2}` (logs at 10:03 and 10:05 in insertion
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

#[cfg(test)]
mod tests;

/// Build a sparse histogram from chronologically sorted log timestamps.
///
/// Each entry records (second, running_count) — the cumulative number of
/// logs up to and including that second. One entry per second that has at
/// least one log.
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
