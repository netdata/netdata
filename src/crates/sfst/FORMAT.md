# SFST File Format

SFST is the on-disk format for one log-index file. Each file holds the
indexed contents of one log stream (one
`(service.namespace, service.name)` pair) and is built from one WAL
file by the `sfst-indexer` crate. The container is chunk-based with
a `chunk-file` table of contents; every chunk has a 4-byte id naming
its role.

This document defines the on-disk shape — the bytes, the chunk ids,
and the schema of each chunk's payload. Producers (the indexer) and
consumers (the query reader) sit on top of this format and are out of
scope here.

---

## File Layout

    ┌───────────────────────────────────────────────┐
    │  Header                       (12 bytes)      │
    ├───────────────────────────────────────────────┤
    │  TOC                          (chunk-file)    │
    │    12 × (num_chunks + 1) bytes                │
    ├───────────────────────────────────────────────┤
    │  Chunk bodies                                 │
    │    Concatenated in TOC order. Each body is    │
    │    <payload bytes> <crc32 u32 LE>.            │
    └───────────────────────────────────────────────┘

The TOC immediately follows the header. Chunk bodies follow the TOC in
the same order their entries appear in it. Readers look chunks up by
id through the TOC and must not assume a positional layout.

Since v5 every chunk body carries a trailing crc32 (`crc32fast`,
i.e. IEEE) computed over the payload bytes — the stored (compressed)
form, excluding the 4-byte CRC itself. The TOC's per-chunk span covers
`payload_len + 4`. Readers verify the CRC on every chunk access and
reject a mismatching chunk with `Error::CorruptIndex`. This framing
(header + TOC + per-chunk CRC) is the shared
`chunk_file::container` format, also used by the otel catalog
(magic `NCAT`).

---

## Header

    Offset  Size  Field        Encoding
    ──────  ────  ───────────  ────────────────────────
    0       4     magic        ASCII "SFST"
    4       4     version      u32 little-endian
    8       4     num_chunks   u32 little-endian

A reader rejects:

- any other magic with `Error::InvalidMagic`,
- any other version with `Error::UnsupportedVersion`,
- a file shorter than 12 bytes with `Error::FileTooShort`,
- a `num_chunks` value that exceeds the file body's plausible
  maximum (each TOC entry is at least 12 bytes) with `Error::Toc` —
  defense-in-depth against a corrupted header.

`num_chunks` is the number of chunk bodies between the TOC and EOF.
The TOC carries one entry per chunk plus a trailing sentinel.

---

## Table of Contents

A `chunk_file::Toc`. Each entry is 12 bytes:

    Bytes  Field
    ─────  ─────────────────────────────
    0..4   chunk id ([u8; 4])
    4..12  body offset within file (u64 LE)

The TOC has `num_chunks + 1` entries. The final entry is a sentinel
(`id = [0; 4]`) whose offset is EOF; chunk sizes are computed as the
delta between an entry's offset and the next entry's offset.

Chunk ids are 4 bytes. A producer must not emit the same id twice.

---

## Chunk Ids

Every chunk in the file uses one of the ids below. Singleton ids
identify a single chunk; indexed ids encode the chunk's position
within its tier in the trailing bytes.

    Id          Payload                                       Required?
    ──────────  ────────────────────────────────────────────  ──────────
    "SUMR"      Summary                                       No (always emitted)
    "META"      Metadata                                      No (always emitted)
    "TIMS"      Vec<i64>  (per-log nanosecond timestamps)     Yes
    "PRIM"      FstIndex<BitmapValue>                         Yes
    "MF{hi}{lo}" FstIndex<BitmapValue>  (mid-card field)      No (one per mid field)
    "HF{hi}{lo}" HighField  (high-card field, columnar SoA)   No (one per high field)
    "SB0{N}"    StreamBatch  (stream-batch N, fixed-width arena)  Yes (at least 1)

The rows are listed in the order the canonical producer emits chunk
bodies. This order is **not** part of the format contract — readers
resolve chunks through the TOC and must not assume positions (see
above) — but the producer groups the chunks a query's statistics phase
always reads (`SUMR`, `META`, `TIMS`, `PRIM`) into a hot prefix, ahead
of the mid/high field chunks (read only when a field is filtered or
faceted) and the stream batches (read only when materializing rows).
`PRIM` sits last in the prefix, next to the structurally-identical
mid/high field FSTs. This keeps the hot prefix resident in the page
cache while the cold remainder can be advised away as a single
contiguous span.

Indexed ids:

- `"MF{hi}{lo}"` is the 4 bytes `[b'M', b'F', hi, lo]` where
  `index = (hi << 8) | lo`. The chunk holds the FST for the
  mid-cardinality field at that position in the file's tier-sorted
  field list.
- `"HF{hi}{lo}"` is the analogous 4 bytes `[b'H', b'F', hi, lo]` for
  the high-cardinality field at that position. Payload is a
  struct-of-arrays — *not* an FST, because FSTs compress poorly at
  high cardinality. See [§ `HF{i}`](#hfi--high-card-field-columnar)
  for the schema.
- `"SB0{N}"` is the 4 bytes `[b'S', b'B', b'0', b'0' + N]` for
  `N` in `0..MAX_STREAM_BATCHES` (currently 8). The trailing byte is
  an ASCII digit so the ids are human-readable when dumping the TOC.

Indices start at 0 and are contiguous within each tier. A producer
emitting `M` mid-card chunks uses ids `MF{0}` through `MF{M-1}`;
similarly for `HF{i}` and `SB{i}`.

`PRIM`, `TIMS`, and at least one `SB{i}` are required. The public
writer (`sfst::StreamWriter`) enforces the full canonical shape — all
four named chunks plus the declared secondary counts, in order — and
fails with `Error::WriterMisuse` / `Error::InvalidStreamBatchCount`
on any deviation. SUMR/META are technically optional at the container
level (a raw `chunk_file` container without them still parses), but
every produced file carries all of them.

---

## Stream-batch partitioning

The number of `SB{i}` chunks in a file is derived from the file's
total log count via a fixed rule — it is **not** stored anywhere in
the file. Both writer and reader compute it identically:

    pub const MIN_LOGS_PER_BATCH: u32 = 1024;
    pub const MAX_STREAM_BATCHES: u8 = 8;

    pub fn num_stream_batches(total_logs: u32) -> u8 {
        (total_logs / MIN_LOGS_PER_BATCH).clamp(1, MAX_STREAM_BATCHES as u32) as u8
    }

    pub fn stream_batch_size(total_logs: u32) -> u32 {
        if total_logs == 0 { 1 } else { total_logs.div_ceil(num_stream_batches(total_logs) as u32) }
    }

Properties:

- A file with `total_logs == 0` carries exactly one (empty) `SB00`
  chunk so the TOC always has at least one stream-batch entry.
- Files with `total_logs ≤ MIN_LOGS_PER_BATCH` (1024) use one batch.
- Files above that scale linearly until they hit `MAX_STREAM_BATCHES`
  (8), where the rule clamps. The 8-batch ceiling exists so the
  per-value batch-membership mask in each `HF{i}` chunk fits in a
  single `u8` (one bit per batch — see
  [§ `HF{i}`](#hfi--high-card-field-columnar)).

The writer partitions chronologically-sorted log positions into
`stream_batch_size(total_logs)`-sized contiguous slices and emits one
`SB{i}` chunk per slice. The reader, given a chronological position
`p`, finds its batch as `p / stream_batch_size(total_logs)`.

---

## Chunk Payloads

All payloads are `bincode + zstd` (see [§ Encoding](#encoding)). The
decoded type of each chunk is given below.

### `SUMR` — Summary

The cheap recovery summary. Decodes to:

    pub struct Summary {
        pub min_timestamp_s: u32,
        pub max_timestamp_s: u32,
        pub total_logs:      u32,
        pub stream:          ServiceStream,
    }

    pub struct ServiceStream {
        pub namespace: String,
        pub name:      String,
    }

`min_timestamp_s` and `max_timestamp_s` are the earliest and latest
log seconds (Unix epoch) in the file. `total_logs` drives the
stream-batch partitioning (see above). `stream` carries the file's
single `(service.namespace, service.name)` identity.

### `META` — Metadata

Heavy query-time metadata:

    pub struct Metadata {
        pub histogram: Histogram,
        pub id_ranges: IdRanges,
        pub fields:    Vec<FieldEntry>,
    }

    pub struct Histogram {
        pub timestamps: Vec<u32>,   // second-boundary timestamps
        pub counts:     Vec<u32>,   // cumulative log count at each boundary
    }

    pub struct IdRanges {
        pub low_end:  KvId,
        pub mid_end:  KvId,
        pub high_end: KvId,
    }

    pub struct FieldEntry {
        pub name:        String,
        pub cardinality: u32,
        pub tier:        FieldTier,    // Low | Mid | High
    }

`histogram` supports time-range narrowing: binary-search `timestamps`
for the position bounds covering a window, then clip bitmaps to that
range. `id_ranges` tells a reader which cardinality tier a `KvId`
belongs to (see [§ Tier-Aligned IDs](#tier-aligned-ids)). `fields`
is ordered low → mid → high (each tier internally sorted by field
name) and yields the count of mid-card and high-card fields, which
in turn determines how many `MF{i}` and `HF{i}` chunks are present.

### `PRIM` — Primary FST

`fst_index::FstIndex<BitmapValue>` containing every low-cardinality
`key=value` entry across all low-card fields.

    pub struct BitmapValue {
        pub desc: treight::Bitmap,
        pub data: Vec<u8>,
    }

The bitmap records the time-sorted log positions where the
`key=value` pair appears. Always present.

### `MF{i}` — Mid-card field FSTs

One chunk per mid-cardinality field, in the order those fields appear
in `Metadata::fields`. Same payload schema as `PRIM` — an
`FstIndex<BitmapValue>` whose keys are full `key=value` strings.

### `HF{i}` — High-card field columnar (string arena)

One chunk per high-cardinality field. Payload is a string arena with
parallel columns:

    pub struct HighField {
        keys_blob: Vec<u8>,   // all "key=value" keys concatenated, sorted
        key_lens:  Vec<u32>,  // per-key byte length, parallel to the keys
        masks:     Vec<u8>,   // batch-membership bitmask, parallel to the keys
        // offsets: in-memory only (#[serde(skip)]) — see below
    }

The keys are the field's `key=value` strings, sorted lexicographically
and stored as one contiguous `keys_blob`; key `i` is
`keys_blob[offset(i)..offset(i+1)]`, where offsets are the prefix-sum of
`key_lens`. `masks[i]` is a bitmask over the file's stream batches: bit
`b` is set iff key `i` appears in stream batch `b`. The reader uses the
mask to skip stream batches the value isn't in when materialising
matching positions — without this index, every high-card filter would
have to scan every `SB{i}` chunk.

Why an arena rather than `Vec<String>`: deserializing one `keys_blob`
byte buffer is a single allocation, versus one heap `String` per key —
which, on a full high-card scan, dominated allocation. On disk it stores
**lengths, not offsets**: lengths are small varints (≈1 B/key), whereas
raw `u32` offsets would varint-inflate to ≈5 B/key (≈+18% on the
high-card chunks, which are ~76% of the file). The columnar layout (the
length and mask columns each contiguous) also compresses marginally
tighter under zstd than an interleaved form.

The `offsets` array (`key_lens.len() + 1` prefix sums) is **not
serialized** (`#[serde(skip)]`); the reader rebuilds it from `key_lens`
on load (`HighField::rebuild_offsets`) for O(1) key access. The write
side (`HighField::for_write`) leaves it unbuilt — that value exists only
to be packed, not read.

### `TIMS` — Per-log timestamps

`Vec<i64>` of nanosecond timestamps in chronological order,
parallel-indexed to the concatenation of every
[`SB{i}`](#sbi--stream-batch-N) chunk:
`timestamps[i]` is the nanosecond timestamp of the log whose attribute
list lives at global position `i` in the concatenated stream.

Per-log timestamps follow the OTel hierarchy: `time_unix_nano` →
`observed_time_unix_nano` → `ingestion_ns + row_offset` (the indexer
synthesizes a fallback if both OTel timestamps are absent so that
every log has a well-defined timestamp).

Required: the writer's ordered API makes it impossible to omit this
chunk. Downstream tooling (display, sub-second filtering,
time-of-event citation) relies on it.

### `SB{i}` — Stream-batch N

The per-log attribute lists for one chronological partition, as a
fixed-width arena (`StreamBatch`):

    StreamBatch {
        kv_bytes: Vec<u8>,    // each KvId as 4 little-endian bytes, rows concatenated
        row_lens: Vec<u32>,   // KvId count per row
        // row_offsets: in-memory only (#[serde(skip)]) — prefix-sum of row_lens
    }

Row `local_pos`'s `KvId`s are `kv_bytes[4*off(p) .. 4*off(p+1)]`, where
`off` is the prefix-sum of `row_lens` (in `KvId` units) and
`local_pos = global_pos - i * stream_batch_size(total_logs)`.
Concatenating every `SB{i}` chunk in id order recovers the full
chronological log stream.

`KvId`s are stored **fixed-width little-endian, not varint**: the scan
reads them straight from `kv_bytes` (`chunks_exact(4)` → `from_le_bytes`)
with no per-id deserialization — the dominant decode cost — and it's
*smaller* on disk than varint (high-card `KvId`s already cost ~4 bytes as
varints, and the regular 4-byte stride compresses tighter under zstd). The
column layout mirrors `HF{i}`: lengths and values each contiguous,
`row_offsets` rebuilt on load.

Each `KvId` references a `key=value` pair via the tier-aligned id space
below. The reader walks an `SB{i}` chunk to materialize a log's attributes
after time-range filtering has selected positions.

---

## Tier-Aligned IDs

A `KvId` is a `u32` identifying a `key=value` pair within the file:

    pub struct KvId(pub u32);

IDs are assigned during writing by walking the tiers in order:

    KvId 0          .. low_end       Low-card pairs   (primary FST iteration order)
    KvId low_end    .. mid_end       Mid-card pairs   (per-field FST iteration order)
    KvId mid_end    .. high_end      High-card pairs  (per-field chunk order)

`low_end`, `mid_end`, `high_end` are carried in `Metadata::id_ranges`.

The `SB{i}` chunks store `KvId`s, not strings; resolving a `KvId` back
to its `key=value` requires checking which range it falls in and
looking up the corresponding entry in `PRIM`, an `MF{i}` chunk, or an
`HF{i}` chunk.

Tier assignment is stable for a given input: fields are sorted by
name within each tier, and entries within each per-field structure
follow the underlying serialized order. Two indexers given the same
WAL produce identical id assignments.

---

## Encoding

All chunk payloads go through:

    serialized = bincode::serde::encode(value, bincode::config::standard())
    payload    = zstd::encode(serialized, level)

The packing is internal to the crate: producers hand typed payloads
to `sfst::StreamWriter`, which packs FST chunks (PRIM, `MF{i}`) at an
elevated zstd level and everything else at the default level. The
format itself does not fix the level — zstd frames are
self-describing, so readers decode any level.

Container-level integrity is the per-chunk crc32 trailer (see
[§ File Layout](#file-layout)): every chunk access verifies the
stored bytes before they reach the zstd decoder. TOC corruption is
caught indirectly — a corrupt offset resolves the wrong span, whose
CRC then fails to match.

---

## Format Version

The current version is **5**.

### v5 changelog (from v4)

- **Per-chunk crc32 trailers.** Every chunk body is now
  `<payload> <crc32 u32 LE>`, with the CRC computed over the stored
  (compressed) payload bytes via `crc32fast`. The TOC accounts for the
  4 extra bytes per chunk. Readers verify the CRC on every chunk
  access; a mismatch surfaces as `Error::CorruptIndex` and the query
  layer degrades that file to an empty shard instead of serving
  corrupted rows. SFSTs are uploaded to remote storage that is never
  garbage-collected, so files must be integrity-checked before any
  permanent objects accumulate. The framing is now produced/parsed by
  the shared `chunk_file::container` helper, whose TOC stores offsets
  little-endian as this document specifies. (The earlier in-branch v5
  draft framed the file through the external `gix-chunk` crate, which
  writes offsets big-endian — contradicting this spec; no such file was
  ever shipped.)

v4 files cannot be read by a v5 reader and vice versa
(`Error::UnsupportedVersion` on the version field). No migration tool
exists.

### v4 changelog (from v3)

- **`SB{i}` payload reshape — `Vec<Vec<KvId>>` → fixed-width arena.** v3
  stored each stream batch as `Vec<Vec<KvId>>` (varint `KvId`s, one inner
  `Vec` per row); decoding it deserialized every `KvId` one at a time —
  the dominant *cycle* cost on a full high-card scan. v4 stores a
  `StreamBatch { kv_bytes: Vec<u8>, row_lens: Vec<u32> }` arena where each
  `KvId` is 4 little-endian bytes, so the scan reads ids straight from
  `kv_bytes` with no per-id deserialization — see
  [§ `SB{i}`](#sbi--stream-batch-n). Fixed-width is also ~10% *smaller* on
  disk than varint here (high-card `KvId`s already cost ~4 bytes as
  varints, and the regular stride compresses tighter).

v3 files cannot be read by a v4 reader and vice versa
(`Error::UnsupportedVersion` on the version field). No migration tool
exists.

### v3 changelog (from v2)

- **`HF{i}` payload reshape — `Vec<String>` keys → string arena.** v2
  stored a high-card field's keys as `HighField { keys: Vec<String>,
  masks: Vec<u8> }`; deserializing it allocated one heap `String` per
  key, which dominated allocation on a full high-card scan. v3 replaces
  the keys column with a string arena (`keys_blob: Vec<u8>` +
  `key_lens: Vec<u32>`) — see [§ `HF{i}`](#hfi--high-card-field-columnar-string-arena).
  On disk this is size-neutral (slightly smaller; lengths are stored,
  not offsets); on decode it collapses N allocations to ~2. The write
  side now packs an owned `HighField` (`for_write`) directly, so the v2
  `HighFieldRef<'a>` borrowed view is gone.

v2 files cannot be read by a v3 reader and vice versa
(`Error::UnsupportedVersion` on the version field). No migration tool
exists.

### v2 changelog (from v1)

- **`STRM` → `SB{i}` stream-batch chunks.** v1 stored every log's
  attribute list in a single `STRM` chunk; v2 partitions logs
  chronologically into 1–8 `SB{i}` chunks (`SB00`..`SB07`), one per
  partition. Partition count and size are derived from
  `Summary::total_logs` via `num_stream_batches` /
  `stream_batch_size` — see
  [§ Stream-batch partitioning](#stream-batch-partitioning).
  This enables a high-card filter to materialise only the batches
  its values appear in, rather than scanning the whole stream.
- **`HF{i}` payload reshape.** v1 stored each high-card field as
  `Vec<(String, BitmapValue)>`. v2 replaces it with a columnar
  `HighField { keys: Vec<String>, masks: Vec<u8> }`. `BitmapValue`
  is gone from the high-card path; `masks` is a per-value batch-
  membership bitmask that tells a reader which `SB{i}` chunk(s)
  contain the value. The change requires the batching above to
  exist, so the two are bumped together.

v1 files cannot be read by a v2 reader and vice versa
(`Error::UnsupportedVersion` on the version field). No migration tool
exists — v1 files were never deployed beyond development.

### When to bump the version

A bump is required for any change that breaks the on-disk contract:

- adding a required chunk id,
- removing an existing chunk id,
- changing any chunk's payload schema in a non-backwards-compatible way,
- changing the stream-batch partitioning rule (which would change
  every reader's view of which `SB{i}` chunk a position lives in),
- changing how the TOC is laid out.

Adding a new optional chunk id, or extending an existing payload with
a field whose default decodes from absent bytes, does not require a
bump.
