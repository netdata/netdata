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
/// ranges, the field table, and the per-row columns manifest. Readers
/// that only need the cheap summary fields (min/max timestamp, total log
/// count, stream) should decode [`Summary`] from the `SUMR` chunk instead.
#[derive(Debug, Clone, PartialEq, Eq, Serialize, Deserialize)]
pub struct Metadata {
    pub histogram: Histogram,
    pub id_ranges: IdRanges,
    pub fields: FieldTable,
    /// Manifest of the per-row column chunks this file carries
    /// (`OBTS`/`TRCE`/`SPAN`/`FLAG`/`DRAC`), with each column's type. Empty when
    /// the file has no per-row columns. The authoritative source for column
    /// presence + type — readers consult it instead of probing for chunks. See
    /// [`ColumnsTable`].
    pub columns: ColumnsTable,
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

// ── Per-row columns (OBTS / TRCE / SPAN / FLAG / DRAC) ───────────

/// On-disk type of a per-row column — its structural encoding, enough for a
/// reader to decode the column chunk and validate it against the typed accessor.
#[derive(Debug, Clone, Copy, PartialEq, Eq, Serialize, Deserialize)]
pub enum ColumnType {
    /// One `i64` per row (e.g. `observed_ts`).
    I64,
    /// One `u32` per row (e.g. `flags`, `dropped_attributes_count`).
    U32,
    /// A fixed-stride byte arena, `n` bytes per row (e.g. `trace_id` = 16,
    /// `span_id` = 8).
    FixedBytes(u8),
}

/// One per-row column the file carries: its name and on-disk type.
#[derive(Debug, Clone, PartialEq, Eq, Serialize, Deserialize)]
pub struct ColumnEntry {
    pub name: String,
    pub ty: ColumnType,
}

/// Manifest of the per-row columns a file carries (the `columns` field of
/// [`Metadata`]). Lists **present** columns only — membership is presence — each
/// with its [`ColumnType`]. The authoritative source for which per-row column
/// chunks (`OBTS`/`TRCE`/`SPAN`/`FLAG`/`DRAC`) exist and how to decode them; mirrors
/// [`FieldTable`](crate::FieldTable) for facets. Empty ⇒ no per-row columns.
#[derive(Debug, Clone, Default, PartialEq, Eq, Serialize, Deserialize)]
#[serde(transparent)]
pub struct ColumnsTable(pub Vec<ColumnEntry>);

impl ColumnsTable {
    /// Number of per-row columns.
    pub fn len(&self) -> usize {
        self.0.len()
    }

    /// Whether the file carries no per-row columns.
    pub fn is_empty(&self) -> bool {
        self.0.is_empty()
    }

    /// The type of the column named `name`, or `None` if absent. O(n) scan over a
    /// handful of columns — same tradeoff as [`FieldTable::get`](crate::FieldTable).
    pub fn get(&self, name: &str) -> Option<ColumnType> {
        self.0.iter().find(|c| c.name == name).map(|c| c.ty)
    }

    /// Whether this table carries a column named `name`.
    pub fn contains(&self, name: &str) -> bool {
        self.get(name).is_some()
    }

    /// The column names, in table order.
    pub fn names(&self) -> impl Iterator<Item = &str> {
        self.0.iter().map(|c| c.name.as_str())
    }
}

impl std::ops::Deref for ColumnsTable {
    type Target = [ColumnEntry];
    fn deref(&self) -> &Self::Target {
        &self.0
    }
}

impl From<Vec<ColumnEntry>> for ColumnsTable {
    fn from(entries: Vec<ColumnEntry>) -> Self {
        Self(entries)
    }
}

impl FromIterator<ColumnEntry> for ColumnsTable {
    fn from_iter<I: IntoIterator<Item = ColumnEntry>>(iter: I) -> Self {
        Self(iter.into_iter().collect())
    }
}

/// Generate a scalar per-row column newtype over `Vec<$elem>` ($elem is `Copy`):
/// one value per row, stored in the **same chronological order** as the `TIMS`
/// chunk and the stream batches (entry `i` is global row `i`). Optional — written
/// only when the file carries that column. Stored, never FST-indexed.
macro_rules! scalar_column {
    ($(#[$m:meta])* $ty:ident, $elem:ty, $name:expr, $coltype:expr) => {
        $(#[$m])*
        #[derive(Debug, Clone, Default, PartialEq, Eq, Serialize, Deserialize)]
        #[serde(transparent)]
        pub struct $ty(pub Vec<$elem>);

        impl $ty {
            /// The column's manifest name + on-disk type (see [`ColumnsTable`]).
            pub const NAME: &'static str = $name;
            pub const COLUMN_TYPE: ColumnType = $coltype;

            pub fn len(&self) -> usize {
                self.0.len()
            }
            pub fn is_empty(&self) -> bool {
                self.0.is_empty()
            }
            /// A copy reordered by `positions` (insertion-order indices in the
            /// desired output order) — the build-time insertion→chronological remap.
            pub fn reordered(&self, positions: impl Iterator<Item = usize>) -> Self {
                $ty(positions.map(|i| self.0[i]).collect())
            }
        }
    };
}

scalar_column!(
    /// Per-row observed timestamps (`OBTS` chunk). The record's *resolved* timestamp
    /// lives in `TIMS`; this carries the raw `observed_time_unix_nano` for losslessness.
    ObservedTimestamps, i64, "observed_ts", ColumnType::I64
);
scalar_column!(
    /// Per-row OTLP `LogRecord.flags` (`FLAG` chunk) — a `fixed32`; the W3C trace
    /// flags occupy the low byte, the remaining 24 bits are reserved per the spec.
    Flags, u32, "flags", ColumnType::U32
);
scalar_column!(
    /// Per-row OTLP `LogRecord.dropped_attributes_count` (`DRAC` chunk).
    DroppedAttributeCounts, u32, "dropped_attributes_count", ColumnType::U32
);

/// Per-row W3C trace ids (`TRCE` chunk): a **fixed-stride 16-byte arena** — row `i`
/// is `bytes[i*16 .. (i+1)*16]`, in chronological row order. An all-zero id is the
/// OTLP/W3C "unset/invalid" sentinel, so an absent id is 16 zero bytes. Fixed width
/// (vs `Vec<Vec<u8>>`) avoids a heap allocation per row, drops the per-element length
/// prefix, and compresses tighter — the same layout as [`StreamBatch`]/[`HighField`].
#[derive(Debug, Clone, Default, PartialEq, Eq, Serialize, Deserialize)]
pub struct TraceIds {
    /// 16 bytes per row, concatenated. `serde_bytes` decodes in one bulk copy
    /// instead of serde's per-byte `Vec<u8>` seq path; wire-identical under bincode.
    #[serde(with = "serde_bytes")]
    bytes: Vec<u8>,
}

/// Per-row W3C span ids (`SPAN` chunk): a fixed-stride **8-byte** arena. See
/// [`TraceIds`] for the layout and the all-zero "unset" sentinel.
#[derive(Debug, Clone, Default, PartialEq, Eq, Serialize, Deserialize)]
pub struct SpanIds {
    #[serde(with = "serde_bytes")]
    bytes: Vec<u8>,
}

/// Generate the shared fixed-stride-arena API for the id column newtypes.
macro_rules! id_arena {
    ($ty:ty, $width:expr, $name:expr) => {
        impl $ty {
            /// Bytes per id (the OTLP/W3C fixed width).
            pub const WIDTH: usize = $width;

            /// The column's manifest name + on-disk type (see [`ColumnsTable`]).
            pub const NAME: &'static str = $name;
            pub const COLUMN_TYPE: ColumnType = ColumnType::FixedBytes($width as u8);

            /// An empty arena sized for `rows` ids.
            pub fn with_capacity(rows: usize) -> Self {
                Self { bytes: Vec::with_capacity(rows * Self::WIDTH) }
            }

            /// Append one id, normalized to exactly `WIDTH` bytes: a shorter or
            /// empty id is zero-padded, a longer one truncated. Callers that want
            /// to reject/flag malformed lengths must do so before calling (see
            /// `ng-ingest`); this is the storage backstop.
            pub fn push(&mut self, id: &[u8]) {
                let start = self.bytes.len();
                self.bytes.resize(start + Self::WIDTH, 0);
                let n = id.len().min(Self::WIDTH);
                self.bytes[start..start + n].copy_from_slice(&id[..n]);
            }

            /// Number of ids.
            pub fn len(&self) -> usize {
                self.bytes.len() / Self::WIDTH
            }

            /// Whether the arena is empty.
            pub fn is_empty(&self) -> bool {
                self.bytes.is_empty()
            }

            /// Whether the backing buffer is a whole number of `WIDTH`-byte ids.
            /// A decoded chunk that fails this is malformed (truncated/corrupt) —
            /// the reader rejects it rather than letting `get`/`iter` drop or
            /// straddle bytes.
            pub fn well_formed(&self) -> bool {
                self.bytes.len() % Self::WIDTH == 0
            }

            /// The `i`-th id (`WIDTH` bytes).
            pub fn get(&self, i: usize) -> &[u8] {
                &self.bytes[i * Self::WIDTH..(i + 1) * Self::WIDTH]
            }

            /// Iterate ids in stored order.
            pub fn iter(&self) -> impl Iterator<Item = &[u8]> {
                self.bytes.chunks_exact(Self::WIDTH)
            }

            /// A copy reordered by `positions` (insertion-order indices in the
            /// desired output order) — the build-time insertion→chronological remap.
            pub fn reordered(&self, positions: impl Iterator<Item = usize>) -> Self {
                let mut out = Self::with_capacity(self.len());
                for i in positions {
                    out.push(self.get(i));
                }
                out
            }
        }
    };
}

id_arena!(TraceIds, 16, "trace_id");
id_arena!(SpanIds, 8, "span_id");

#[cfg(test)]
mod tests;
