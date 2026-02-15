# Split-FST Index Format

This document describes the file format and indexing approach for the
split-FST log index. The index enables fast filtering of log entries by
arbitrary `key=value` attribute pairs, time ranges, and service streams.


## Concepts

A **stream** is identified by the `(service.namespace, service.name)` tuple
from OpenTelemetry resource attributes. Every log entry belongs to exactly
one stream. Streams partition the data so that queries scoped to a single
service only load relevant sections of the index.

Every `key=value` attribute pair is assigned to one of three **cardinality
tiers** based on the number of unique values for that field (the part
before `=`):

    Tier         Unique values        Storage
    ───────────  ───────────────────  ─────────────────────────────
    Low-card     < T                  Primary FST (global)
    Mid-card     [T, 10T)             Per-field FST (global)
    High-card    >= 10T               Per-field zstd blob (global)

where T is a configurable cardinality threshold (default 100).


## File Layout

    ┌───────────────────────────────────────────────┐
    │  META chunk                                   │
    │    total_logs: u32                            │
    │    histogram: [(u32 ts, u32 count), ...]      │
    │    id_ranges: { low: 0..A, mid: A..B,         │
    │                 high: B..C }                  │
    │    streams: [                                 │
    │      { namespace, name, log_count }, ...      │
    │    ]                                          │
    ├───────────────────────────────────────────────┤
    │  FLDS chunk                                   │
    │    field_table: [                             │
    │      { name, cardinality, tier }, ...         │
    │    ]                                          │
    │    Ordered low → mid → high, each sub-group   │
    │    sorted by field name. Loaded on demand.    │
    ├───────────────────────────────────────────────┤
    │  PRIM chunk (Primary FST)             ~2 MB   │
    │    Global FST covering all low-card           │
    │    key=value -> bitmap entries.               │
    │    zstd level 3.                              │
    ├───────────────────────────────────────────────┤
    │  Secondary chunks: MID-CARD FSTs      ~3 MB   │
    │    One FST per mid-card field.                │
    │    key=value -> bitmap.                       │
    │    zstd level 3.                              │
    ├───────────────────────────────────────────────┤
    │  Secondary chunks: HC BLOBS         ~165 MB   │
    │    One chunk per high-card field.             │
    │    [(key=value, bitmap), ...] encoded with    │
    │    bincode + zstd level 1.                    │
    ├───────────────────────────────────────────────┤
    │  Secondary chunks: LOG ENTRIES                │
    │    One chunk per stream, independently        │
    │    compressed: log_position -> [id, id, ...]  │
    │    where each id references a key=value pair  │
    │    via the tier-aligned ID space.             │
    │    bincode + zstd level 1.                    │
    └───────────────────────────────────────────────┘

All sections are independently addressable through a gix-chunk table
of contents at the start of the file. META and PRIM are always loaded
on open; all other sections are loaded selectively depending on the
query.


## Tier-Aligned ID Space

Every `key=value` pair in the index is assigned a global ID. The IDs are
assigned by iterating through the tiers in order, so the ID ranges are
contiguous per tier:

    IDs 0 ...... A-1       Low-card entries (primary FST iteration order)
    IDs A ...... B-1       Mid-card entries (per-field FST iteration order)
    IDs B ...... C-1       High-card entries (per-field chunk order)

The log entries section uses these IDs to reference key=value pairs without
duplicating strings. The reader resolves an ID to its string by checking
which range it falls in and looking up the corresponding FST or HC chunk.


## Bitmaps

All bitmaps are treight bitmaps stored in time-sorted position space.
During indexing, log entries are assigned positions in insertion order,
then remapped to chronological order. A contiguous range of positions
corresponds to a contiguous time window, which enables efficient
time-range filtering using the histogram.

Dense bitmaps (cardinality > universe/2) store their complement to save
space.


## Stream Identification

Streams are derived from OpenTelemetry resource attributes following the
semantic conventions specification. The identifying attributes for the
`service` and `service.namespace` entities define the stream key:

    stream_key = (service.namespace, service.name)

where `service.namespace` defaults to empty string if absent. The spec
requires `service.name` to be unique within a namespace, making this
pair a stable, low-cardinality identifier.

If the number of unique streams exceeds a hard cap (e.g. 65536), the
smallest streams by log count are merged into a catch-all stream.
Streams below a minimum size (e.g. 0.1% of total logs) may also be
merged to avoid tiny partitions that compress poorly.


## Query Flow

A typical query like `service.name=api AND sha256=abc123` proceeds as:

    1. Load META + PRIMARY FST                      (always, cached, ~2 MB)

    2. Primary FST lookup: service.name=api
       Result: bitmap of all matching log positions.
       If the query also specifies service.namespace, AND both bitmaps
       to identify the exact stream.

    3. Time range narrowing (if applicable):
       Histogram lookup -> position range -> AND with current bitmap

    4. Load the HC chunk for the sha256 field      (on demand, one field)
       Scan for sha256=abc123 -> bitmap
       AND with current bitmap -> final positions

    5. Load the matching stream's log entries section
       For each final position, retrieve attribute IDs
       Resolve IDs to key=value strings via FST/chunk lookups


## Indexing Approach

The indexing pipeline transforms a WAL file into the format above. The
process has two phases: reading (collecting data in memory) and writing
(building the file sections).


### Phase 1: Reading

Process each WAL frame sequentially. For each log entry, four pieces
of information are recorded:

    Data structure              Purpose
    ─────────────────────────── ───────────────────────────────────
    KeyValueInterner            Assigns a KeyValueId to each unique
                                key=value string encountered.

    Vec<RoaringBitmap>          Indexed by KeyValueId. Each bitmap
                                tracks which log positions contain
                                that key=value pair (insertion order).

    Vec<i64>                    Timestamp per log position, used
                                later for time-sort remapping.

    Vec<Vec<KeyValueId>>        Log entries: for each log position,
                                the list of KeyValueIds it contains.
                                Built in the same loop that populates
                                the bitmaps.

Stream identification is not done during reading. The `service.name`
and `service.namespace` attributes are interned as regular key=value
pairs with their own bitmaps. Streams are derived from these bitmaps
during the writing phase by cross-joining them.

The reading loop in pseudocode:

    for each frame in WAL:
        for each log row:
            timestamps.push(row.timestamp)

            for each key=value attribute on this row:
                aid = interner.intern(key=value)
                bitmaps[aid].insert(log_pos)
                log_entries[log_pos].push(kv_id)

            log_pos += 1


### Phase 2: Writing

After reading completes, the writing phase transforms in-memory data
into the file format.

Step 1 — Time-sort remap:

    Build remap[insertion_pos] = sorted_pos from timestamps.
    All bitmaps and stream assignments are translated from insertion
    order to time-sorted order using this table.

Step 2 — Histogram:

    Single pass over sorted timestamps, emitting (second, running_count)
    for each second boundary.

Step 3 — Cardinality classification:

    Group interner entries by field name (prefix before '=').
    Count unique values per field. Classify into low / mid / high.

Step 4 — Primary FST:

    Build a single FST containing low-card key=value entries with
    treight bitmaps. Compress with zstd level 3.

Step 5 — Mid-card FSTs:

    For each mid-card field, build a per-field FST with
    key=value -> treight bitmap entries.
    Compress with zstd level 3.

Step 6 — HC chunks:

    For each high-card field, serialize all key=value + bitmap entries
    with bincode, compress with zstd level 1.

Step 7 — Tier-aligned ID assignment:

    Iterate through the tiers in write order (primary FST, then each
    mid-card FST, then each HC chunk), assigning sequential IDs
    0, 1, 2, ... to each key=value pair encountered.

    Build a translation table: interner_id -> file_id.

Step 8 — Stream derivation:

    Scan the interner for service.name=X and service.namespace=Y
    entries. Cross-join their bitmaps to find which (namespace, name)
    combinations have logs. Logs not covered by any service.name
    bitmap go to a catch-all stream.

Step 9 — Log entries (per stream):

    For each stream:
      Collect all log positions belonging to this stream.
      For each position, translate its log_entries[pos] KeyValueIds
      to file IDs using the translation table.
      Serialize and compress independently with zstd.

Step 10 — Field table:

    Build the field table: for each field across all tiers, record
    its name, cardinality, and tier. Write as the FLDS chunk.

Step 11 — Write file:

    Assemble all sections with a table of contents and write to disk.
