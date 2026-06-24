//! On-disk schema for SFST log indexes.
//!
//! These are the typed payloads carried by an SFST file's named chunks.
//! Producers (the WAL indexer in the `sfst-indexer` crate) construct
//! them; consumers decode them via the typed accessors on
//! [`crate::Reader`]. The container layout and chunk encoding are
//! specified in `FORMAT.md`.

use serde::{Deserialize, Serialize};
use treight::Bitmap;

// ── SUMR ─────────────────────────────────────────────────────────

/// The cheap, content-agnostic per-file summary stored in the `SUMR` chunk —
/// time span, record count, and opaque content metadata — read by a registry
/// to pick this SFST as a query candidate without opening the heavy `META`
/// chunk.
///
/// This is the substrate's [`file_registry::FileSummary`]; the SFST stores it
/// verbatim and never interprets `content_meta`. A query keeps the file as a
/// candidate when its `[min_timestamp_s, max_timestamp_s]` span overlaps the
/// request window and its `id.part_key` matches (the partition key lives in the
/// `FileId`, not the summary); the file's
/// stream-batch geometry comes from `record_count` (see
/// [`stream_batch_size`](crate::stream_batch_size)). Kept in its own `SUMR`
/// chunk so a registry rebuilds on startup by faulting in only the header, TOC,
/// and SUMR — never decompressing `META`.
pub use file_registry::FileSummary as Summary;

// ── META ─────────────────────────────────────────────────────────

/// Heavy query-time metadata (the `META` chunk payload).
///
/// Holds the data a reader needs to bootstrap any query against the
/// file: the sparse timestamp histogram, the cardinality-tier id
/// ranges, and the field table. Readers that only need the cheap
/// summary fields (min/max timestamp, total log count, stream) should
/// decode [`Summary`] from the `SUMR` chunk instead.
#[derive(Debug, Clone, PartialEq, Eq, Serialize, Deserialize)]
pub struct Metadata {
    pub histogram: Histogram,
    pub id_ranges: IdRanges,
    pub fields: FieldTable,
}

/// Sparse timestamp histogram: one entry per second that has at least
/// one log record, paired with the cumulative log count up to and
/// including that second. Built from chronologically-sorted log
/// timestamps during indexing; used at query time for time-range
/// narrowing.
#[derive(Debug, Clone, PartialEq, Eq, Serialize, Deserialize)]
pub struct Histogram {
    /// Second-boundary timestamps as u32 seconds since Unix epoch.
    pub timestamps: Vec<u32>,
    /// Cumulative log count at each second boundary.
    pub counts: Vec<u32>,
}

/// Contiguous [`KvId`] ranges for the three cardinality tiers.
///
/// Ids are assigned sequentially: `0..low_end` for low-card,
/// `low_end..mid_end` for mid-card, `mid_end..high_end` for high-card.
/// The reader uses these ranges to decide which section (primary FST,
/// mid-card FST, or high-card sorted list) to consult for a given id.
#[derive(Debug, Clone, PartialEq, Eq, Serialize, Deserialize)]
pub struct IdRanges {
    pub low_end: KvId,
    pub mid_end: KvId,
    pub high_end: KvId,
}

// ── Field table (carried inside META) ────────────────────────────

/// One entry in the field table carried by [`Metadata::fields`].
///
/// The table is ordered low → mid → high, with each tier internally
/// sorted by field name. Readers walk it to count mid-card and
/// high-card fields, to look up a field's tier when resolving a
/// [`KvId`], and to discover which secondary chunks the file carries.
#[derive(Debug, Clone, PartialEq, Eq, Serialize, Deserialize)]
pub struct FieldEntry {
    pub name: String,
    pub cardinality: u32,
    pub tier: FieldTier,
}

/// Cardinality tier for a field. The cardinality threshold `T` and
/// its 10× cutoff (set by the producer; default
/// [`DEFAULT_CARDINALITY_THRESHOLD`]) define the boundaries: `< T` is
/// low, `[T, 10·T)` is mid, `≥ 10·T` is high.
#[derive(Debug, Clone, Copy, PartialEq, Eq, Serialize, Deserialize)]
pub enum FieldTier {
    Low,
    Mid,
    High,
}

/// Default cardinality threshold for [`FieldTier`] classification.
/// Public so every producer of field tables — the WAL indexer in
/// `sfst-indexer` and the WAL row scan in `sfsq` — classifies with the
/// same boundaries unless explicitly overridden.
pub const DEFAULT_CARDINALITY_THRESHOLD: u32 = 100;

impl FieldEntry {
    /// High-cardinality — rejected by `facets`/`timeline`, so not
    /// offerable as a facet or histogram dimension.
    pub fn is_high_card(&self) -> bool {
        matches!(self.tier, FieldTier::High)
    }
}

/// A file's field table: its fields with cardinality and tier, ordered
/// low → mid → high then by name (see [`FieldEntry`]).
///
/// Readers walk it to count tiers, resolve a field's tier by name, and
/// discover which secondary chunks a file carries; query layers walk it
/// to pick facet / histogram fields. Serializes transparently as its
/// inner `Vec<FieldEntry>`, and derefs to `[FieldEntry]` so slice
/// operations (`iter`, indexing, `len`) work directly.
#[derive(Debug, Clone, Default, PartialEq, Eq, Serialize, Deserialize)]
#[serde(transparent)]
pub struct FieldTable(Vec<FieldEntry>);

impl FieldTable {
    /// The entry for `name`, or `None` if absent from this table.
    ///
    /// O(n) linear scan. The table is tier-ordered (not globally sorted
    /// by name), and holds only a handful of fields per file, so a scan
    /// is the right tradeoff; add a side index only if tables ever grow.
    pub fn get(&self, name: &str) -> Option<&FieldEntry> {
        self.0.iter().find(|f| f.name == name)
    }

    /// Whether this table carries a field named `name`.
    pub fn contains(&self, name: &str) -> bool {
        self.get(name).is_some()
    }

    /// The field names, in table order.
    pub fn names(&self) -> impl Iterator<Item = &str> {
        self.0.iter().map(|f| f.name.as_str())
    }
}

impl std::ops::Deref for FieldTable {
    type Target = [FieldEntry];
    fn deref(&self) -> &Self::Target {
        &self.0
    }
}

impl From<Vec<FieldEntry>> for FieldTable {
    fn from(entries: Vec<FieldEntry>) -> Self {
        Self(entries)
    }
}

impl FromIterator<FieldEntry> for FieldTable {
    fn from_iter<I: IntoIterator<Item = FieldEntry>>(iter: I) -> Self {
        Self(iter.into_iter().collect())
    }
}

// ── PRIM / secondary chunks ──────────────────────────────────────

/// Value type for FST entries in the primary chunk and mid-card field
/// chunks, and for the pairs inside high-card field chunks.
///
/// Carries a [`treight::Bitmap`] over time-sorted log positions where
/// the `key=value` pair appears. `desc` is the bitmap metadata; `data`
/// holds the encoded payload bytes.
#[derive(Debug, Clone, Serialize, Deserialize)]
pub struct BitmapValue {
    pub desc: Bitmap,
    /// `serde_bytes` decodes this as one bulk copy rather than serde's
    /// generic per-byte `Vec<u8>` seq path; wire-identical under bincode.
    #[serde(with = "serde_bytes")]
    pub data: Vec<u8>,
}

// ── High-card field chunk (struct-of-arrays) ─────────────────────

/// Body of a high-card field chunk (the `HF{i}` chunks).
///
/// The `key=value` strings are stored as a **string arena**: one
/// contiguous `keys_blob` byte buffer plus the parallel
/// `key_lens` (per-key byte length) and
/// [`masks`](Self::masks) (per-key stream-batch bitmask, bit `b` set iff
/// the value appears in batch `b` — see [`crate::num_stream_batches`]).
/// Keys are sorted lexicographically.
///
/// The arena is what makes decode cheap: deserializing one `Vec<u8>` blob
/// is a single allocation, versus one heap `String` per key (which, on a
/// full high-card scan, dominated allocation). On disk it stores **lengths,
/// not offsets** — lengths are small varints (≈1 B/key, like the old
/// per-string length prefix), whereas raw `u32` offsets would varint-inflate
/// to ≈5 B/key. The columnar layout (lengths and masks each contiguous) also
/// compresses marginally tighter under zstd than the old interleaved form.
/// In memory, `offsets` is the prefix-sum of `key_lens`
/// (rebuilt on load, not serialized) so key access is O(1).
#[derive(Debug, Clone, Serialize, Deserialize)]
pub struct HighField {
    /// All `field=value` keys concatenated, in sorted order. `serde_bytes`
    /// decodes this in one bulk copy instead of serde's per-byte `Vec<u8>`
    /// seq path (the dominant high-card scan cost); wire-identical under
    /// bincode, so no format change.
    #[serde(with = "serde_bytes")]
    keys_blob: Vec<u8>,
    /// Per-key byte length, parallel to the keys. Varint-compact on disk;
    /// prefix-summed into `offsets` in memory.
    key_lens: Vec<u32>,
    /// Per-key stream-batch bitmask, parallel to the keys.
    #[serde(with = "serde_bytes")]
    pub masks: Vec<u8>,
    /// Prefix-sum of `key_lens` (`len() + 1` entries): key `i` is
    /// `keys_blob[offsets[i]..offsets[i + 1]]`. Rebuilt on load via
    /// [`rebuild_offsets`](Self::rebuild_offsets); never serialized.
    #[serde(skip)]
    offsets: Vec<u32>,
}

impl HighField {
    /// Build the **write form** — ready to serialize — from sorted
    /// `field=value` keys and their parallel masks. `keys` must be
    /// lexicographically sorted and the same length as `masks`.
    ///
    /// `offsets` (the in-memory key index) is intentionally **not** built:
    /// this value exists to be packed, not read. Key access
    /// ([`key`](Self::key), [`keys`](Self::keys),
    /// [`binary_search`](Self::binary_search)) is only valid after a load,
    /// where the reader calls `rebuild_offsets` (crate-internal);
    /// calling it on a write-form value panics (debug-asserted).
    pub fn for_write<S: AsRef<str>>(keys: &[S], masks: Vec<u8>) -> Self {
        let total_bytes: usize = keys.iter().map(|key| key.as_ref().len()).sum();
        let mut keys_blob = Vec::with_capacity(total_bytes);
        let mut key_lens = Vec::with_capacity(keys.len());
        for key in keys {
            let bytes = key.as_ref().as_bytes();
            keys_blob.extend_from_slice(bytes);
            key_lens.push(bytes.len() as u32);
        }
        Self {
            keys_blob,
            key_lens,
            masks,
            offsets: Vec::new(),
        }
    }

    /// Recompute `offsets` from `key_lens`. Called after deserialize (where
    /// `offsets` is skipped and so arrives empty).
    pub(crate) fn rebuild_offsets(&mut self) {
        self.offsets.clear();
        self.offsets.reserve(self.key_lens.len() + 1);
        let mut acc = 0u32;
        self.offsets.push(0);
        for &len in &self.key_lens {
            acc += len;
            self.offsets.push(acc);
        }
    }

    /// Number of keys.
    pub fn len(&self) -> usize {
        self.key_lens.len()
    }

    /// Whether the chunk has no keys.
    pub fn is_empty(&self) -> bool {
        self.key_lens.is_empty()
    }

    /// The `i`-th `field=value` key as bytes (keys are valid UTF-8). Only
    /// valid once `offsets` is built — on load, or via
    /// `rebuild_offsets` (crate-internal); never on a
    /// [`for_write`](Self::for_write) value.
    pub fn key(&self, i: usize) -> &[u8] {
        debug_assert_eq!(
            self.offsets.len(),
            self.key_lens.len() + 1,
            "HighField offsets not built — call rebuild_offsets() after deserialize",
        );
        &self.keys_blob[self.offsets[i] as usize..self.offsets[i + 1] as usize]
    }

    /// Iterate keys (`field=value` bytes) in sorted order.
    pub fn keys(&self) -> impl Iterator<Item = &[u8]> + '_ {
        (0..self.len()).map(move |i| self.key(i))
    }

    /// Binary-search the sorted keys for an exact match — `Ok(index)` or
    /// `Err(insert_pos)`, mirroring [`slice::binary_search`]. Byte order
    /// matches the lexicographic order keys are stored in.
    pub fn binary_search(&self, key: &[u8]) -> Result<usize, usize> {
        let mut lo = 0usize;
        let mut hi = self.len();
        while lo < hi {
            let mid = lo + (hi - lo) / 2;
            match self.key(mid).cmp(key) {
                std::cmp::Ordering::Less => lo = mid + 1,
                std::cmp::Ordering::Greater => hi = mid,
                std::cmp::Ordering::Equal => return Ok(mid),
            }
        }
        Err(lo)
    }
}

impl PartialEq for HighField {
    /// Compares the persisted columns; `offsets` is a derived cache.
    fn eq(&self, other: &Self) -> bool {
        self.keys_blob == other.keys_blob
            && self.key_lens == other.key_lens
            && self.masks == other.masks
    }
}

impl Eq for HighField {}

// ── Stream-log-entries chunk ─────────────────────────────────────

/// Tier-aligned identifier for a `key=value` pair within one SFST.
///
/// Assigned during writing in FST iteration order across the three
/// cardinality tiers; the stream-log-entries chunk stores sequences of
/// these instead of duplicating the strings.
///
/// Not to be confused with [`file_registry::FileId`], which identifies
/// an SFST file on disk.
#[derive(Debug, Clone, Copy, PartialEq, Eq, Hash, Serialize, Deserialize)]
pub struct KvId(pub u32);

impl KvId {
    /// The id as a `usize`, for indexing parallel tables.
    #[inline]
    pub fn idx(self) -> usize {
        self.0 as usize
    }
}

/// Body of a stream-batch chunk (the `SB{i}` chunks): the per-log attribute
/// lists for one chronological partition, as a fixed-width arena.
///
/// `kv_bytes` holds every row's `KvId`s concatenated, each as **4
/// little-endian bytes**; `row_lens[i]` is the number of `KvId`s in row
/// `i`. Row `i`'s ids are `kv_bytes[4*off(i) .. 4*off(i+1)]`, where `off`
/// is the prefix-sum of `row_lens` (held in `row_offsets`, in `KvId`
/// units, rebuilt on load).
///
/// Fixed-width LE (vs varint `KvId`s) lets the high-card scan read ids
/// straight from the byte buffer — no per-id deserialization, the dominant
/// decode cost — and it's *smaller* on disk: high-card `KvId`s are large
/// enough that varint already spends ~4 bytes, and a regular 4-byte stride
/// compresses tighter under zstd than ragged varints.
#[derive(Debug, Clone, Serialize, Deserialize)]
pub struct StreamBatch {
    /// `serde_bytes` decodes this in one bulk copy instead of serde's
    /// per-byte `Vec<u8>` seq path; wire-identical under bincode.
    #[serde(with = "serde_bytes")]
    kv_bytes: Vec<u8>,
    row_lens: Vec<u32>,
    /// Prefix-sum of `row_lens` (`rows + 1` entries, in `KvId` units);
    /// rebuilt on load via [`rebuild_offsets`](Self::rebuild_offsets), never
    /// serialized.
    #[serde(skip)]
    row_offsets: Vec<u32>,
}

impl StreamBatch {
    /// Build the **write form** from per-row `KvId` lists, ready to
    /// serialize. `row_offsets` is left unbuilt (this value is for packing,
    /// not reads) — mirrors `HighField::for_write`.
    pub fn for_write(rows: &[Vec<KvId>]) -> Self {
        let total_ids: usize = rows.iter().map(Vec::len).sum();
        let mut kv_bytes = Vec::with_capacity(total_ids * 4);
        let mut row_lens = Vec::with_capacity(rows.len());
        for row in rows {
            row_lens.push(row.len() as u32);
            for kv in row {
                kv_bytes.extend_from_slice(&kv.0.to_le_bytes());
            }
        }
        Self {
            kv_bytes,
            row_lens,
            row_offsets: Vec::new(),
        }
    }

    /// Recompute `row_offsets` from `row_lens`. Called after deserialize.
    pub(crate) fn rebuild_offsets(&mut self) {
        self.row_offsets.clear();
        self.row_offsets.reserve(self.row_lens.len() + 1);
        let mut acc = 0u32;
        self.row_offsets.push(0);
        for &len in &self.row_lens {
            acc += len;
            self.row_offsets.push(acc);
        }
    }

    /// Number of log rows in this batch.
    pub fn num_rows(&self) -> usize {
        self.row_lens.len()
    }

    /// Whether the batch has no rows.
    pub fn is_empty(&self) -> bool {
        self.row_lens.is_empty()
    }

    /// The `KvId`s of row `i`, read from the fixed-width little-endian
    /// bytes. Only valid once `row_offsets` is built (on load).
    pub fn row(&self, i: usize) -> impl Iterator<Item = KvId> + '_ {
        debug_assert_eq!(
            self.row_offsets.len(),
            self.row_lens.len() + 1,
            "StreamBatch row_offsets not built — call rebuild_offsets() after deserialize",
        );
        let start = self.row_offsets[i] as usize * 4;
        let end = self.row_offsets[i + 1] as usize * 4;
        self.kv_bytes[start..end]
            .chunks_exact(4)
            .map(|bytes| KvId(u32::from_le_bytes(bytes.try_into().unwrap())))
    }
}

impl PartialEq for StreamBatch {
    /// Compares the persisted columns; `row_offsets` is a derived cache.
    fn eq(&self, other: &Self) -> bool {
        self.kv_bytes == other.kv_bytes && self.row_lens == other.row_lens
    }
}

impl Eq for StreamBatch {}

#[cfg(test)]
mod tests;
