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
    "OBTS"      Vec<i64>  (per-row observed timestamps)       No (per-row column, v8)
    "TRCE"      16-byte arena (per-row trace ids)             No (per-row column, v8)
    "SPAN"      8-byte arena (per-row span ids)               No (per-row column, v8)
    "FLAG"      Vec<u32>  (per-row LogRecord.flags)           No (per-row column, v8)
    "DRAC"      Vec<u32>  (per-row dropped_attributes_count)  No (per-row column, v8)
    "TIDX"      TraceIdIndex  (trace_id fanout + sort permutation)  No (optional, after per-row columns)
    "MF{hi}{lo}" FstIndex<BitmapValue>  (mid-card field)      No (one per mid field)
    "HF{hi}{lo}" HighField  (high-card field, columnar SoA)   No (one per high field)
    "SB0{N}"    StreamBatch  (stream-batch N, fixed-width arena)  Yes (at least 1)

The per-row column chunks (`OBTS`/`TRCE`/`SPAN`/`FLAG`/`DRAC`) are
**independently optional** — a file carries any subset, or none — and live in
the **cold region after `PRIM`**, so a query decodes a column only on demand.
A column is present iff the `META` `ColumnsTable` lists it; readers consult the
manifest (not the chunk table) for presence + type. Each holds exactly one
value per row, in the same chronological order as `TIMS` and the stream batches.

The optional `TIDX` chunk is the **`trace_id` index**: a 256-entry first-byte
fanout (`fanout[256]`, cumulative count of indexed positions whose `trace_id`
first byte is `≤ b`) plus a `sort_perm` of row positions sorted by their 16-byte
`trace_id` (a stable sort, so a trace's spans stay chronological; the all-zero
"unset" id is skipped). It enables O(log) trace-by-id lookup over the
chronological `TRCE` column without scanning — all spans of one trace are a
contiguous run in `sort_perm`. It lives in the cold region after the per-row
columns, requires the `TRCE` column it indexes, and is detected via the TOC
(`Reader::has_trace_id_index`), not the `ColumnsTable`. Presence is independent
of any per-row column except its required `TRCE`.

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

    pub fn num_stream_batches(record_count: u32) -> u8 {
        (record_count / MIN_LOGS_PER_BATCH).clamp(1, MAX_STREAM_BATCHES as u32) as u8
    }

    pub fn stream_batch_size(record_count: u32) -> u32 {
        if record_count == 0 { 1 } else { record_count.div_ceil(num_stream_batches(record_count) as u32) }
    }

Properties:

- A file with `record_count == 0` carries exactly one (empty) `SB00`
  chunk so the TOC always has at least one stream-batch entry.
- Files with `record_count ≤ MIN_LOGS_PER_BATCH` (1024) use one batch.
- Files above that scale linearly until they hit `MAX_STREAM_BATCHES`
  (8), where the rule clamps. The 8-batch ceiling exists so the
  per-value batch-membership mask in each `HF{i}` chunk fits in a
  single `u8` (one bit per batch — see
  [§ `HF{i}`](#hfi--high-card-field-columnar)).

The writer partitions chronologically-sorted log positions into
`stream_batch_size(record_count)`-sized contiguous slices and emits one
`SB{i}` chunk per slice. The reader, given a chronological position
`p`, finds its batch as `p / stream_batch_size(record_count)`.

---

## Chunk Payloads

All payloads are `bincode + zstd` (see [§ Encoding](#encoding)). The
decoded type of each chunk is given below.

### `SUMR` — Summary

The cheap recovery summary. Decodes to `file_registry::FileSummary`
(re-exported as `sfst::Summary`):

    pub struct FileSummary {
        pub min_timestamp_s: u32,
        pub max_timestamp_s: u32,
        pub record_count:    u32,
        pub content_meta:    Vec<u8>,
    }

`min_timestamp_s` and `max_timestamp_s` are the earliest and latest
record seconds (Unix epoch) in the file. `record_count` drives the
stream-batch partitioning (see above). `content_meta` is an opaque
content-plane identity blob, stored verbatim — the content plane (e.g.
`otel-logs-identity`) is the sole encoder/decoder. The substrate is
content-agnostic: it holds these neutral fields and never decodes the
identity. The file's opaque **partition key is NOT stored in the
summary** — it is the single source of truth in the filename (`FileId`);
candidate filtering reads it from there.

#### Partition-key authority

The partition key in the filename is **authoritative**: every reader
(WAL, SFST, catalog, query planner) routes a file by `id.part_key`
parsed from its name, and the key is never re-derived from the file's
contents or cross-checked against them at read time. The producer chain
guarantees the filename matches the contents — the WAL writer stamps the
`FileId` from the same partition key it routes a frame to, and the SFST
indexer inherits that `FileId` verbatim — so for self-produced files the
label and the content always agree by construction. The trade-off is
explicit: a file whose name is altered out-of-band (manual rename, disk
corruption) would be routed by its (now wrong) label, not its contents.
This is accepted because filenames are internal and never rewritten
externally; there is intentionally no runtime content-vs-label guard.

### `META` — Metadata

Heavy query-time metadata:

    pub struct Metadata {
        pub histogram: Histogram,
        pub id_ranges: IdRanges,
        pub tree:      SchemaTree,         // typed field descriptor (v9; replaces `fields`)
        pub columns:   Vec<ColumnEntry>,   // per-row columns manifest (v8)
    }

    // The per-row columns manifest: one entry per present column (membership =
    // presence), the authoritative source of column presence + type. Empty when
    // the file carries no per-row columns.
    pub struct ColumnEntry {
        pub name: String,        // "observed_ts" | "trace_id" | "span_id" | "flags"
                                 //   | "dropped_attributes_count"
        pub ty:   ColumnType,    // I64 | U32 | FixedBytes(n)
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

    // The typed, array-collapsed schema tree — the on-disk field descriptor
    // (v9; replaces the flat `Vec<FieldEntry>`). An arena of nodes; node 0 is the
    // root. A non-root node always has a smaller id than its children.
    pub struct SchemaTree { nodes: Vec<SchemaNode> }

    pub struct SchemaNode {
        pub kind: ValueKind,            // Null|Bool|Int|Double|Str|Bytes|EmptyKvlist
                                        //   |EmptyArray|Kvlist|Array
        pub edge: Option<SchemaEdge>,   // None only at the root
        pub leaf: Option<LeafStats>,    // Some on leaf nodes only (the path's stats)
    }
    pub struct SchemaEdge { pub parent: NodeId, pub step: Step }  // Step = Field(String)|ArrayElem
    pub struct LeafStats  { pub cardinality: u32, pub tier: FieldTier }  // Low|Mid|High

    pub struct FieldEntry {              // retained type; the *derived* flat view
        pub name:        String,
        pub cardinality: u32,
        pub tier:        FieldTier,      // Low | Mid | High
    }

`histogram` supports time-range narrowing: binary-search `timestamps`
for the position bounds covering a window, then clip bitmaps to that
range. `id_ranges` tells a reader which cardinality tier a `KvId`
belongs to (see [§ Tier-Aligned IDs](#tier-aligned-ids)).

`tree` is the typed field descriptor: each leaf node is a typed `(path, kind)`
column (a path collapses array indices to `[]`); interior `Kvlist`/`Array` nodes
give structure; leaf nodes carry the path's `cardinality` + `tier`. The flat
**`FieldTable`** the tier machinery and legacy consumers use is **derived** from
the tree at read time (`SchemaTree::derive_field_table`): one `FieldEntry` per
distinct leaf path, ordered low → mid → high then by name (byte-identical to the
`Vec<FieldEntry>` a pre-v9 file stored), which yields the count of mid-card and
high-card fields and thus how many `MF{i}`/`HF{i}` chunks are present. A
polymorphic path (multiple leaf kinds) collapses to one `FieldEntry`; its single
coalesced **scalar** type (for typed/UI display) comes from
`SchemaTree::derive_scalar_kinds` (drop `Null`; empty containers contribute no
scalar; `Int ⊔ Double = Double`; any other scalar mix → `Str`). A producer with
no typed tree (raw `(ts, key=value)` rows) emits a flat `Str`-typed tree
(`SchemaTree::flat`) so every v9 file has a valid descriptor. Per-occurrence type
is not stored anywhere — the tree records the *set* of kinds per path, not which
kind each stored value was.

On decode the reader validates the tree's structural invariants (non-empty arena;
node 0 is the root with no edge; every non-root `parent` is a strictly smaller id,
which also forbids cycles) via `SchemaTree::validate`. A file violating them is
rejected as `CorruptIndex` (the query layer's skip-the-file path) rather than
panicking the unchecked path walk or hanging on a cyclic edge.

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
in the derived field table (`SchemaTree::derive_field_table`). Same payload
schema as `PRIM` — an `FstIndex<BitmapValue>` whose keys are full `key=value`
strings.

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
`local_pos = global_pos - i * stream_batch_size(record_count)`.
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

The current version is **9**.

### Additive within v9: the `TIDX` trace_id index

The optional `TIDX` chunk (see [§ Chunk Ids](#chunk-ids)) is **additive and
TOC-indexed**: it changes no existing chunk's bincode layout, a file without it
simply omits it, and a reader that does not know it reads the rest unchanged.
Its presence therefore needs **no version bump** — the same treatment the v8
per-row column chunks received. A `TIDX` produced by this build pairs with the
`TRCE` column in the same file.

### v9 changelog (from v8)

- **`META` replaces the flat field table with a typed schema tree.** `Metadata`
  swaps `fields: Vec<FieldEntry>` for `tree: SchemaTree` — the typed,
  array-collapsed schema tree is now the on-disk field descriptor (per-leaf
  `ValueKind` + parent/child structure + per-leaf `cardinality`/`tier`). The flat
  `FieldTable` is *derived* from the tree at read time
  (`SchemaTree::derive_field_table`), byte-identical to what a v8 file stored, so
  the tier machinery and legacy consumers are unchanged. Coalesced scalar field
  types come from `SchemaTree::derive_scalar_kinds`.
- **Storage chunks are unchanged from v8.** `PRIM`/`MF{i}`/`HF{i}`/`SB{i}`/`TIMS`
  and the per-row column chunks keep their v8 encoding — only the `META` payload
  changed. A v9 SFST built from a given WAL has byte-identical non-`META` chunks
  to the v8 SFST from the same WAL.
- Incompatible `META` bincode layout, so older files are rejected on open
  (`Container::open` version check).

### v8 changelog (from v7)

- **`META` gains a per-row columns manifest.** `Metadata` adds
  `columns: Vec<ColumnEntry>` — the authoritative list of which per-row column
  chunks the file carries and their types. Empty for files with no per-row
  columns. Incompatible `META` bincode layout, so older files are rejected on
  open (`Container::open` version check).
- **New optional per-row column chunks** `OBTS`/`TRCE`/`SPAN`/`FLAG`/`DRAC`,
  in the **cold region after `PRIM`**. Each is independently optional, holds one
  value per row in chronological order, and is stored — not FST-indexed — so
  near-unique identifiers don't explode the facet index. Producers that write no
  per-row columns (the production logs indexer) emit none of these chunks and an
  empty manifest.

### v7 changelog (from v6)

- **`SUMR` drops `part_key`.** The partition key is no longer stored in the
  summary — it is the single source of truth in the filename (`FileId`),
  propagated from the WAL file the SFST is built from. `FileSummary` is now
  `{ min_timestamp_s, max_timestamp_s, record_count, content_meta }`. Candidate
  filtering reads `id.part_key` (from the filename) instead of `summary.part_key`.
  The bincode layout is incompatible, so a v6 file is rejected at the version
  check (`Error::UnsupportedVersion`).

v6 files cannot be read by a v7 reader and vice versa
(`Error::UnsupportedVersion`). No migration tool exists.

### v6 changelog (from v5)

- **Content-agnostic `SUMR`.** The summary payload changed from the typed
  `Summary { total_logs: u32, stream: ServiceStream }` to
  `file_registry::FileSummary { record_count: u32, part_key: u64,
  content_meta: Vec<u8> }`. The SFST layer no longer knows about
  `(service.namespace, service.name)`: it stores an opaque `part_key`
  (compared for candidate selection) and an opaque `content_meta` blob
  (stored verbatim; decoded only by the content plane). The bincode byte
  layouts are incompatible, so a v5 file is rejected at the version check
  (`Error::UnsupportedVersion`) rather than failing later inside the bincode
  decode. No other chunk changed.

v5 files cannot be read by a v6 reader and vice versa
(`Error::UnsupportedVersion` on the version field). No migration tool
exists.

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
  `Summary::record_count` via `num_stream_batches` /
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
