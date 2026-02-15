//! The [`WalIndex`] type — in-memory index built during Phase 1 (reading).
//!
//! Holds four data structures:
//!
//! 1. **KeyValueInterner** — assigns a [`KeyValueId`] to each unique `key=value` string.
//! 2. **Vec\<RoaringBitmap\>** — indexed by [`KeyValueId`]; each bitmap tracks which
//!    log positions contain that `key=value` pair (insertion order).
//! 3. **Vec\<Vec\<KeyValueId\>\>** — log entries: for each log position, the list
//!    of key=value IDs it contains (needed for per-stream serialization in Phase 2).
//! 4. **Vec\<i64\>** — nanosecond timestamp per log position, used to build the
//!    time-sort remap and sparse histogram.

use bumpalo::Bump;
use roaring::RoaringBitmap;

use crate::kv_interner::{KeyValueId, KeyValueInterner};

/// A stream identified by its service.name / service.namespace combination.
pub struct ServiceStream {
    pub namespace: String,
    pub name: String,
    /// Insertion-order log positions belonging to this stream.
    pub positions: Vec<u32>,
}

/// The output of Phase 1: everything the frame loop extracts from the WAL.
///
/// Bundles the four data structures described in the module doc into a single
/// value, making the Phase 1 → Phase 2 handoff explicit.
pub struct WalIndex<'a> {
    pub kv_interner: KeyValueInterner<'a>,
    /// One bitmap per key=value ID. `kv_bitmaps[id.idx()]` tracks which log
    /// positions (insertion order) contain that `key=value` pair.
    pub kv_bitmaps: Vec<RoaringBitmap>,
    /// Per-log key=value IDs: `log_entries[log_pos]` lists all key=value IDs
    /// for that log's attributes. Used for per-stream serialization in Phase 2.
    pub log_entries: Vec<Vec<KeyValueId>>,
    /// Nanosecond timestamp per log position (insertion order).
    pub timestamps: Vec<i64>,
}

impl<'a> WalIndex<'a> {
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
    pub fn sparse_histogram(&self, time_order: &TimeOrder) -> SparseHistogram {
        build_sparse_histogram(&self.timestamps, time_order)
    }

    /// Resolve a key=value ID to its string.
    pub fn resolve(&self, id: KeyValueId) -> &str {
        self.kv_interner.resolve(id)
    }

    /// Get the roaring bitmap for a key=value ID.
    pub fn bitmap(&self, id: KeyValueId) -> &RoaringBitmap {
        &self.kv_bitmaps[id.idx()]
    }

    /// Low-cardinality fields (< threshold), sorted by field name.
    pub fn low_fields(&self) -> Vec<(&str, &[KeyValueId])> {
        self.kv_interner.low_fields()
    }

    /// Mid-cardinality fields ([threshold, 10*threshold)), sorted by field name.
    pub fn mid_fields(&self) -> Vec<(&str, &[KeyValueId])> {
        self.kv_interner.mid_fields()
    }

    /// High-cardinality fields (>= 10*threshold), sorted by field name.
    pub fn high_fields(&self) -> Vec<(&str, &[KeyValueId])> {
        self.kv_interner.high_fields()
    }

    /// Tier-aligned assignment of key=value IDs.
    pub fn tier_assignment(&self) -> [Vec<KeyValueId>; 3] {
        self.kv_interner.tier_assignment()
    }

    /// Cardinality threshold for field classification.
    pub fn cardinality_threshold(&self) -> u32 {
        self.kv_interner.cardinality_threshold()
    }

    /// All interned strings, ordered by KeyValueId.
    pub fn strings(&self) -> &[&str] {
        self.kv_interner.strings()
    }

    /// Ensure kv_bitmaps vec has an entry for the given key=value ID.
    pub fn ensure_bitmap(&mut self, kv_id: KeyValueId) {
        if kv_id.idx() >= self.kv_bitmaps.len() {
            self.kv_bitmaps
                .resize_with(kv_id.idx() + 1, RoaringBitmap::new);
        }
    }

    /// Derive streams by cross-joining service.namespace and service.name bitmaps.
    ///
    /// Scans the interner for `service.name=X` and `service.namespace=Y` entries,
    /// then AND-s their bitmaps to find which (namespace, name) combinations have
    /// logs. Logs not covered by any service.name bitmap go to a catch-all stream.
    pub fn service_streams(&self) -> Vec<ServiceStream> {
        // Collect all service.name and service.namespace key=value pairs.
        let mut namespace_entries: Vec<(&str, KeyValueId)> = Vec::new();
        let mut name_entries: Vec<(&str, KeyValueId)> = Vec::new();

        for (kv_id, kv_pair) in self.kv_interner.strings().iter().enumerate() {
            if let Some(name) = kv_pair.strip_prefix("service.name=") {
                name_entries.push((name, KeyValueId::from(kv_id)));
            } else if let Some(namespace) = kv_pair.strip_prefix("service.namespace=") {
                namespace_entries.push((namespace, KeyValueId::from(kv_id)));
            }
        }

        let mut streams: Vec<ServiceStream> = Vec::new();
        let mut covered = RoaringBitmap::new();

        if namespace_entries.is_empty() {
            // No namespaces: each service.name defines a stream directly.
            for (name, kv_id) in name_entries {
                let bm = self.bitmap(kv_id);

                if !bm.is_empty() {
                    streams.push(ServiceStream {
                        namespace: String::new(),
                        name: name.to_string(),
                        positions: bm.iter().collect(),
                    });

                    covered |= bm;
                }
            }
        } else {
            // Cross-join: AND each (namespace, name) pair to find logs that
            // belong to both. Each non-empty intersection becomes a stream.
            for &(namespace, namespace_kv_id) in &namespace_entries {
                let namespace_bm = self.bitmap(namespace_kv_id);

                for &(name, name_kv_id) in &name_entries {
                    let name_bm = self.bitmap(name_kv_id);

                    let intersection = namespace_bm & name_bm;

                    if !intersection.is_empty() {
                        streams.push(ServiceStream {
                            namespace: namespace.to_string(),
                            name: name.to_string(),
                            positions: intersection.iter().collect(),
                        });

                        covered |= &intersection;
                    }
                }
            }

            // Logs with a service.name but no matching namespace get their
            // own namespace-less stream.
            for &(name, name_kv_id) in &name_entries {
                let name_bm = self.bitmap(name_kv_id);

                let uncovered_in_name = name_bm - &covered;
                if !uncovered_in_name.is_empty() {
                    streams.push(ServiceStream {
                        namespace: String::new(),
                        name: name.to_string(),
                        positions: uncovered_in_name.iter().collect(),
                    });

                    covered |= &uncovered_in_name;
                }
            }
        }

        // Catch-all: logs with no service.name at all.
        let universe_size = self.num_logs() as u32;
        let all: RoaringBitmap = (0..universe_size).collect();
        let uncovered = all - &covered;
        if !uncovered.is_empty() {
            streams.push(ServiceStream {
                namespace: String::new(),
                name: String::new(),
                positions: uncovered.iter().collect(),
            });
        }

        streams
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

#[derive(Debug, Clone, serde::Serialize, serde::Deserialize)]
pub struct SparseHistogram {
    /// Second-boundary timestamps as u32 seconds since Unix epoch.
    pub timestamps: Vec<u32>,
    /// Cumulative log count at each second boundary.
    pub counts: Vec<u32>,
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

/// Build a sparse histogram from chronologically sorted log timestamps.
///
/// Each entry records (second, running_count) — the cumulative number of
/// logs up to and including that second. One entry per second that has at
/// least one log.
fn build_sparse_histogram(timestamps: &[i64], time_order: &TimeOrder) -> SparseHistogram {
    if timestamps.is_empty() {
        return SparseHistogram {
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

    SparseHistogram {
        timestamps: hist_ts,
        counts: hist_counts,
    }
}
