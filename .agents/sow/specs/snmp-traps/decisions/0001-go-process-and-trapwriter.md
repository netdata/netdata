# ADR-0001: Go Process Model, Journal Writer Backend, and TrapWriter Contract

**Status**: Accepted for SOW-0035 implementation after reviewer round 5; four reviewers completed, one stalled and produced no final findings
**Date**: 2026-05-25
**SOW**: SOW-0035 M1

## Context

The Netdata SNMP trap subsystem (design spec: `.agents/sow/specs/snmp-traps/netdata.md`) needs a concrete implementation architecture decision for three interlocking concerns:

1. **Process model**: Where does the trap plugin live in the Netdata process tree?
2. **Journal writer backend**: How do we write per-job journal files at `/var/cache/netdata/traps/{job_name}/` in Go, compatible with `journalctl --directory=...` queries?
3. **TrapWriter interface + TrapEntry shape**: What is the concrete Go contract between the trap pipeline and storage backends?

The implementation language is **Go** (user decision, 2026-05-25). The journal writer must produce real systemd journal binary-format files so end-to-end acceptance criteria (M4: `journalctl --directory=/var/cache/netdata/traps/test/ TRAP_CATEGORY=security`) passes.

## Decision Drivers

1. **Fit for purpose** — the architecture must blend with existing Netdata patterns. The go.d framework already owns job orchestration (DynCfg Add/Enable/Update/Disable/Remove), coded-error surfacing in the dashboard, and the V2 collector lifecycle.
2. **Minimize blast radius** — new process boundaries, CGo dependencies, or IPC bridges add failure modes, build complexity, and operational surface that must be continuously tested.
3. **Creation-time failure detection** — all job resources (bind, profile load, journal directory, writer init, retention) must be validated before DynCfg reports the job as started. This is a user-facing correctness contract per spec §5.
4. **Share nothing, share once** — the trap profile cache loads on first runnable job creation, is shared across all listeners, and is released when no runnable jobs remain. In-process sharing is trivial; cross-process sharing adds synchronization, IPC, and lifecycle coordination.
5. **journalctl compatibility** — the M4 acceptance criterion requires `journalctl --directory=...` to work. This means real systemd journal binary-format files (not plain text, not SQLite). The `journalctl` tool reads the binary format documented at https://systemd.io/JOURNAL_FILE_FORMAT/.
6. **Simplicity** — avoid over-engineering. Prefer standard go.d module code unless evidence justifies another boundary.

## Options Considered

### Option A: Standard in-process go.d module + Go-native write-only journal file writer (SELECTED)

- Trap plugin lives as a standard go.d collector module at `src/go/plugin/go.d/collector/snmp_traps/`
- Registered through the standard `collectorapi.Register(...)` path in the existing `collector/init.go` import registry as `snmp_traps`, mirroring the existing `snmp_topology` naming style. A scan found no existing go.d collector registration name containing a dot, so the module name must not use `snmp.traps`.
- Uses V2 collector interface (`collectorapi.CollectorV2`), mirroring the `ping/` collector pattern
- Job lifecycle managed by the existing go.d framework (`src/go/plugin/agent/jobmgr/dyncfg_collector_callbacks.go`)
- Journal writing via a **new Go-native minimal write-only journal file writer** implementing the systemd journal binary format subset needed for writing. It has no reader or cursor support, but it must maintain the write-side DATA/FIELD hash tables and ENTRY_ARRAY chains incrementally so the active file remains queryable by `journalctl`.
- Shared profile cache: in-process Go package-level state, loaded on first runnable job creation

### Option B: Separate Go process (external plugin) via PLUGINSD

- Trap plugin runs as a standalone Go binary using the PLUGINSD protocol (`src/plugins.d/README.md`)
- Communicates metrics via stdout `BEGIN/SET/END` lines
- Journal writing must happen in-process in the separate binary or via a second bridge
- Profile cache lives in the separate process; cross-process sharing with any future Go go.d enrichment code requires netipc

### Option C: Go in-process + CGo bridge to Rust journal-log-writer

- Trap plugin stays in go.d process
- Journal writing calls the existing Rust `journal-log-writer` crate via CGo FFI
- Requires CGo, Rust toolchain at build time, and FFI safety surface

### Option D: Go in-process + subprocess Rust journal writer bridge

- Trap plugin stays in go.d process
- Journal writing pipes entries via stdin/socket to a Rust helper binary that uses `journal-log-writer`
- Process management (start, health-check, restart) is the trap plugin's responsibility

## Evidence

### Existing patterns to mirror

| Pattern | Source | Lines |
|---|---|---|
| go.d V2 collector registration + lifecycle | `src/go/plugin/go.d/collector/ping/collector.go:25-34` | `collectorapi.Register("ping", ...)` + `CreateV2` |
| DynCfg job orchestration | `src/go/plugin/agent/jobmgr/dyncfg_collector_callbacks.go:85-121` | `Start()` preflight + coded errors |
| codedError for HTTP-422 | `src/go/plugin/agent/jobmgr/dyncfg_collector_callbacks.go:140-147` | `type codedError struct` with `Code() int` |
| SNMP profile multipath+dedup loader | `src/go/plugin/go.d/collector/snmp/ddsnmp/load.go:270-286` | `multipath.MultiPath` + filename dedup |
| chart templates (V2) | `src/go/plugin/go.d/collector/ping/charts.yaml` | YAML-driven chart definitions |

### Rust journal-log-writer crate size

The Rust `journal-log-writer` crate at `src/crates/journal-log-writer/` is a thin orchestration layer:

| Crate | Lines | Purpose |
|---|---|---|
| `journal-log-writer` | 1,353 | Public API, `Log` struct, rotation/retention orchestration |
| `journal-core` | 6,689 | Binary journal file format — mmap, headers, objects, hash tables, offset arrays, writer, reader, cursor, field remapping |
| `journal-registry` | 1,594 | File chain management, naming convention, origin tracking |
| `journal-common` | 802 | Boot ID, machine ID, monotonic clocks, microseconds |
| **Total** | **10,438** | Full read/write/query/cursor support |

Counts are source lines under each crate's `src/` tree; test files are excluded.

A **write-only** Go implementation needs only a fraction of this:
- Journal file header written with an explicit `header_size`; the implementation must not assume a magic fixed header byte count when opening/recovering existing files
- Sequential object writing (DATA, FIELD, ENTRY objects — variable-length, tagged)
- DATA/FIELD hash table maintenance and ENTRY_ARRAY chaining during writes, so active files are queryable before rotation
- Boot ID injection (`_BOOT_ID` on every entry)
- Monotonic timestamp guards (clamp to non-decreasing)
- File rotation (size + duration thresholds)
- Retention (delete oldest files exceeding size or age caps)
- No reader, no cursor, no field remapping (trap field names are already systemd-compatible), and no general-purpose query implementation

Reference write-path evidence from the existing Rust implementation is roughly 5,000 lines across `journal-core/src/file/{writer.rs,object.rs,file.rs,hash.rs,offset_array.rs}` and `journal-log-writer/src/log/{mod.rs,chain.rs}` before Go simplification. A realistic Go-native write-only backend estimate is therefore **~4,000-5,500 lines**, not the earlier optimistic 1,500-2,000. This remains testable and reviewable, but M4 must treat the writer as the highest-risk component.

### Why not CGo / subprocess

1. **CGo** adds a C toolchain dependency to `go.d.plugin`, which currently builds pure Go (no CGo). Cross-compilation becomes harder. The Rust crate's FFI surface would need C-compatible wrappers.
2. **Subprocess** requires the trap plugin to manage a child process (start, health-check, restart on crash, graceful shutdown ordering). This is a whole class of reliability bugs that don't exist with in-process code. The Rust binary also needs to be built and shipped alongside go.d.plugin.
3. **libsystemd `sd_journal_sendv()`** via CGo would write through journald, not directly to per-job directories. The `_HOSTNAME` field (source device hostname) would be controlled by journald, not the plugin — violating spec §11: "the trap plugin's journal writer writes directly to journal files (bypassing journald) and controls every field."

## Decision

**Option A is selected**: standard in-process go.d module + Go-native write-only journal file writer.

### 1. Process Model

The SNMP trap plugin lives as a standard go.d collector V2 module:

```
src/go/plugin/go.d/collector/snmp_traps/
    collector.go         # Main struct, init() registration, Config, New(), lifecycle
    init.go              # validateConfig(), one-shot initialization
    collect.go           # collect() — per-cycle metric emission
    listener.go          # Per-job UDP listener + BER decode + RFC 3584 (M2)
    profile.go           # Profile type + shared lazy loader (M3)
    resolver.go          # 2-tier varbind resolution + template rendering (M3)
    journal.go           # Go-native journal file writer + retention (M4)
    trapwriter.go        # TrapEntry type + TrapWriter interface definition
    *.go_test.go         # Table-driven tests
```

Collector consistency artifacts (`metadata.yaml`, `config_schema.json`, stock config, health, README, taxonomy) remain owned by SOW-0039 unless an earlier SOW needs a minimal internal test fixture. Any earlier artifact must still be consistent with the final SOW-0039 bundle plan.

Registration in `src/go/plugin/go.d/collector/init.go`:

```go
_ "github.com/netdata/netdata/go/plugins/plugin/go.d/collector/snmp_traps"
```

### 2. Journal Writer Backend

A new Go package `src/go/plugin/go.d/collector/snmp_traps/journal.go` implements a **write-only** systemd journal binary-format writer. The public API:

```go
// JournalWriter produces write-only journal files in a directory.
// Files are readable by journalctl --directory=<path>.
// file_id, seqnum_id, per-entry seqnum, and crash-recovery state are internal.
type JournalWriter struct { ... }

// NewJournalWriter creates a writer for the given directory.
// On creation, it validates the directory exists and is writable.
func NewJournalWriter(dir string, cfg JournalConfig) (*JournalWriter, error)

type JournalField struct {
    Name  string // Validated journal field name.
    Value []byte // Raw value bytes; writer applies binary encoding when needed.
}

// WriteEntry writes one journal entry. It is synchronous and is called only by
// the journal TrapWriter's single queue worker goroutine, not by the decode hot
// path. WriteEntry is not concurrency-safe.
// Entry sequence numbers are managed internally, starting at 1 per new file and
// incrementing by 1 per successful entry write.
func (w *JournalWriter) WriteEntry(fields []JournalField, realtimeUsec, monotonicUsec int64) error

// Sync flushes pending writes to disk.
func (w *JournalWriter) Sync() error

// Close finalizes the active journal file and releases resources.
// Idempotent.
func (w *JournalWriter) Close() error

// JournalConfig controls rotation and retention.
type JournalConfig struct {
    MaxSize     uint64        // Total bytes cap; 0 = disabled
    MaxDuration time.Duration // Maximum age; 0 = disabled
    RotateSize  uint64        // Per-file rotation size; 0 = auto
    RotateDur   time.Duration // Per-file rotation duration; 0 = disabled; user-facing default is 1h
}
```

**Format compliance**: the writer produces files conforming to the systemd Journal File Format specification (https://systemd.io/JOURNAL_FILE_FORMAT/). Each file contains:

- A valid file header with magic bytes, compatible/incompatible feature flags, `header_size`, and tail object offsets
- Sequentially appended DATA objects (field key=value pairs, uncompressed in the first implementation; compression flags remain off unless M4 validation proves compression is needed for practical trap payload sizes)
- Sequentially appended FIELD objects (field names)
- Sequentially appended ENTRY objects (arrays of entry items referencing DATA objects)
- Incrementally maintained DATA and FIELD hash tables, plus ENTRY_ARRAY chains, so both active and rotated files are queryable by `journalctl --directory=...`
- Boot ID read once at writer creation from `/proc/sys/kernel/random/boot_id`, cached, stored in entry headers/tail metadata, and exposed as `_BOOT_ID` on every journal entry
- Machine ID read once at writer creation from `/etc/machine-id`, validated as a 128-bit machine identifier, stored in the file header, and exposed to `journalctl` as `_MACHINE_ID` for the entries in that file
- Timestamp guards (`ReceivedRealtimeUsec` never decreases per writer; `ReceivedMonotonicUsec` is captured by the recv hot path via `CLOCK_MONOTONIC`, not re-sourced by the writer). Before writing journal binary timestamp fields, `JournalWriter` validates both timestamp inputs are non-negative and casts them to the journal format's unsigned representation; negative timestamps are structural serialization errors.
- DATA hash table floor: 4096 buckets; FIELD hash table floor: 512 buckets. New files may implement the Rust writer's `with_optimized_buckets` behavior by deriving larger initial DATA buckets from `RotateSize` and previous utilization; otherwise the floor values are acceptable for MVP if M4 benchmarks pass. Crash recovery must read actual hash table sizes from the file header instead of assuming defaults.
- Keyed DATA/FIELD hash-table lookups use SipHash-2-4 with the file's 16-byte `file_id` when `HEADER_INCOMPATIBLE_KEYED_HASH` is set in the file header. The `file_id` is generated as a random UUID for new files and read from the header during recovery.
- Non-keyed DATA/FIELD hash-table lookups use Jenkins lookup3/hash64. The implementation must replicate the systemd/Rust 32-bit half ordering in `journal-core/src/file/hash.rs`; a generic non-systemd hash is invalid.
- ENTRY `xor_hash` always uses Jenkins lookup3/hash64 regardless of the keyed-hash flag, matching `journal-core/src/file/writer.rs`.
- Header state starts `online`, transitions to `archived` on rotation/close, and tracks `seqnum_id`, `tail_entry_boot_id`, tail object offset, tail entry offset, entry count, and arena size consistently after appends
- `seqnum_id` is generated per journal file as a random UUID. `entry_seqnum` is the separate per-entry incrementing integer: it starts at 1 for new files and increments by 1 per entry; `head_entry_seqnum` and `tail_entry_seqnum` must stay consistent with the first and last written entries.
- `_BOOT_ID` and `tail_entry_boot_id` use the current boot ID for the entries written in that file
- The implementation uses regular Linux file I/O (`write` plus positioned writes/reads via `golang.org/x/sys/unix` or equivalent, and `fdatasync`) rather than mmap. This keeps all random-access header/hash/entry-array updates as explicit offset writes, avoids mmap lifetime and page-fault failure modes in `go.d.plugin`, and keeps the first implementation free of CGo.
- Header/tail updates must leave the file recoverable after unclean shutdown. M4 tests must include `journalctl --directory=...` against an active file before `Close()` and after an intentionally interrupted writer process.
- First implementation writes uncompressed DATA objects and leaves compression flags unset. Compression is a future optimization gate, not an implicit file-format behavior.

If boot ID or machine ID cannot be read or parsed at writer creation, `NewJournalWriter()` fails with a coded creation-time error. The job must not enter the running state with placeholder IDs.

The Go implementation is a regular-I/O rewrite, not a mechanical port of the Rust mmap writer. ENTRY_ARRAY management must use explicit `pread`/`pwrite` offset updates. Initial ENTRY_ARRAY capacity is 4096 entries per array node, doubling on overflow, matching the Rust writer's strategy. The writer should also include a bounded recent-DATA cache (the Rust writer uses 4096 slots) so repeated field/value pairs do not perform full hash-table work on every entry.

Crash recovery algorithm for existing active files:

1. Read and validate magic bytes plus enough fixed header bytes to obtain `header_size`.
2. Reject/rename corrupt files whose header size, arena size, offsets, or object bounds are impossible, then start a fresh file.
3. Scan objects from the first arena offset through the last fully valid object, validating object type and size bounds.
4. Truncate any partially-written tail object.
5. Rebuild DATA/FIELD hash tables and ENTRY_ARRAY chains from valid objects, recalculate counts/tail offsets/arena size, and write a consistent `online` header before appending.
6. M4 must test this by interrupting the writer mid-stream and validating with both `journalctl --directory=...` and `journalctl --verify`.

The journal-direct TrapWriter owns the concurrency boundary: multiple endpoint receive loops may call `TrapWriter.Write()` concurrently, but they fan into one concurrency-safe bounded queue per job. A single worker goroutine drains that queue and is the only caller of `JournalWriter.WriteEntry()`.

The serialization boundary is explicit:

```go
// serializeToJournalFields converts TrapEntry to journal fields per netdata.md §11.
func serializeToJournalFields(entry *TrapEntry) ([]JournalField, error)
```

This function is called only by the journal TrapWriter's single queue worker goroutine, immediately before `JournalWriter.WriteEntry()`. It owns `TrapEntry` to journal naming, including `PRIORITY`, `SYSLOG_IDENTIFIER`, `ND_LOG_SOURCE`, `ND_NIDL_NODE`, `_HOSTNAME` fallback, `TRAP_TAG_<KEY_UPPERCASE>`, `TRAP_JSON`, and omission of empty optional enrichment fields. It returns an error for structurally invalid entries (for example nil entry or missing required fields). Unsupported `VarbindValue.Value` concrete types use the guarded fallback path in production and are rejected by tests.

**Config parsing**: `JournalConfig` receives parsed byte/duration values. The config/DynCfg layer parses human-readable operator values such as `10GB` and `1h`.

**Rotation**: triggered by file size reaching `RotateSize` or file age reaching `RotateDur`. `RotateSize=0` means auto-calculate from `MaxSize / 20` and clamp to 5MB-200MB; if `MaxSize=0`, use the 200MB upper clamp as the default rotation size. Oversized single entries are written to the current file and can make that file exceed `RotateSize`; the next append opens a new file. `RotateDur=0` disables time-based rotation; the user-facing default remains `1h`. On rotation, the current file is finalized (archived) and a new file is opened. If `MaxSize=0`, size-based retention is disabled but rotation still occurs; files accumulate until an age cap or external cleanup removes them.

**Retention**: after each rotation and on a periodic retention sweep, the writer scans the directory for journal files and deletes the oldest files until total size ≤ `MaxSize` and oldest file age ≤ `MaxDuration` (both independent, inclusive thresholds). The periodic sweep is required so `MaxDuration` is honored even during low-volume periods with no rotations.

The default writer queue capacity is 10,000 entries per job unless implementation tests prove a different default is safer. Queue-full and permanent-writer-failed errors are both drop-and-continue conditions for the caller: increment the per-job `journal_write_failed` dimension, discard that entry, and continue receiving traps. There is no disk spillover in the MVP.

The default flush cadence is 1s or 1,000 accepted entries, whichever comes first, plus `Flush()`/`Close()`. `Flush()` is synchronous and concurrency-safe: it creates a barrier, waits until all entries accepted before that barrier have been written, and calls `fdatasync()`/`Sync()` before returning; entries accepted after the barrier may be flushed by a later cadence. `Close()` is concurrency-safe and idempotent. On first call, it stops new acceptance, drains the queue, finalizes the active journal file, best-effort sets file state to `archived`, syncs, and closes. If drain, finalization, or sync fails, `Close()` returns that terminal error, records the writer as permanently failed, and subsequent `Close()` calls return the stored terminal error; after `Close()` returns, `Write()` returns a closed-writer error.

The journal-direct backend is Linux-only for SOW-0035 because it depends on the Linux boot ID path and `journalctl` semantics. On non-Linux platforms, job creation must fail with a clear coded unsupported-backend error instead of starting and failing at runtime.

The Linux-only guard is checked before resource acquisition in module/job initialization. If the trap module is built on a non-Linux platform, trap job creation returns HTTP-422 unsupported backend before profile loading, socket binding, or journal directory creation.

### 3. TrapWriter Interface

Per spec §19, proposed:

```go
// TrapWriter is the contract between the trap pipeline and storage backends.
// Each job has exactly one TrapWriter. The journal-direct backend writes to
// /var/cache/netdata/traps/{job_name}/; the OTLP backend (SOW-0038) implements
// the same interface with protobuf serialization.
type TrapWriter interface {
    Write(entry *TrapEntry) error   // Fast accept into backend-owned queue or return drop-worthy error
    Flush() error                   // Durability boundary
    Close() error                   // Idempotent close
}
```

Semantics:

- **Write() does not perform blocking disk or network I/O on the decode hot path.** It returns after the backend has accepted ownership of the immutable entry into a bounded internal queue. If the queue is full or the backend is in a permanent failed state, `Write()` returns an error; the caller increments `journal_write_failed` and drops the trap while the hot path continues.
- **Entry ownership transfers to the writer on successful `Write()`.** The caller must not mutate maps, slices, or strings reachable from the entry after `Write()` returns nil. The writer must treat entries as immutable. Reusing a `TrapEntry`, `Labels` map, `SummaryCounts.ByTrap` map, or `Varbinds` backing array after a successful `Write()` is a correctness bug unless the implementation first deep-copies the reused data.
- **Entry ownership does not transfer on failed `Write()`.** The caller still discards the entry and allocates/obtains a fresh one for the next trap; writers must not retain references on an error return.
- **Flush()** creates a queue barrier, waits for all entries accepted before that barrier to be written, and calls `Sync()`/`fdatasync()` on the underlying journal writer, forcing all buffered data needed for `journalctl` visibility and shutdown durability to disk.
- **Close()** is concurrency-safe and idempotent. On first call, it drains the queue, finalizes the active journal file, best-effort sets file state to archived, syncs, and closes. Subsequent calls return nil after a successful first close, or the stored terminal error after a failed first close.
- **Backend-internal batching** is the writer's responsibility. The interface does not expose batching.
- **CWE-117** is owned by the journal writer backend (`JournalWriter.WriteEntry()`), not the interface.

### 4. TrapEntry Shape

Per spec §19, proposed in Go:

```go
type ReportType string
const (
    ReportTypeTrap               ReportType = "trap"
    ReportTypeDedupSummary       ReportType = "deduplication_summary"
    ReportTypeDecodeErrorSummary ReportType = "decode_error_summary" // reserved until decode-summary payload is specified
)

type PduType string
const (
    PduTypeTrap   PduType = "trap"
    PduTypeInform PduType = "inform"
)

type SnmpVersion string
const (
    SnmpVersionV1  SnmpVersion = "v1"
    SnmpVersionV2c SnmpVersion = "v2c"
    SnmpVersionV3  SnmpVersion = "v3"
)

type Category string
type Severity string
type ASN1Type string

// Allowed VarbindValue.Value concrete types:
// string, int64, uint64, float64, bool, net.IP, []byte, nil.
// The canonical serializer rejects any other concrete type in tests and
// renders a guarded string fallback in production while incrementing a
// decode/serialization error counter.
// TimeTicks, DateAndTime, Bits, Opaque, and vendor extensions must be normalized
// by the decoder to one of these concrete types before serialization.

type VarbindValue struct {
    Name  string   `json:"name,omitempty"` // MIB symbolic name (empty if unknown)
    OID   string   `json:"oid"`            // Numeric OID
    Type  ASN1Type `json:"type"`           // ASN.1 type name from a closed parser-owned set
    Value any      `json:"value"`          // Decoded value; serialized only through canonical helper
    Enum  string   `json:"enum,omitempty"` // Enum label if applicable
}

type DedupSummary struct {
    TotalSuppressed int64            `json:"total_suppressed"`
    PeriodSec       int64            `json:"period_sec"`
    Fingerprints    int64            `json:"fingerprints"`
    ByTrap          map[string]int64 `json:"by_trap"` // Numeric OID → count; MESSAGE renderer resolves names.
}

type TrapEntry struct {
    JobName               string     // Which job produced this entry
    ReportType            ReportType // trap / deduplication_summary / decode_error_summary
    ReceivedRealtimeUsec  int64      // Wall-clock receive timestamp from recv path
    ReceivedMonotonicUsec int64      // CLOCK_MONOTONIC receive timestamp from recv path
    TrapOID               string     // Numeric OID (e.g. "1.3.6.1.4.1.9.9.315.0.1")
    TrapName              string     // MIB-qualified name
    Category              Category   // One of 8 canonical slugs
    Severity              Severity   // One of 8 canonical slugs
    Message               string     // May contain arbitrary bytes; writer applies CWE-117
    SourceIP              string     // Identified source per RFC 3584 cascade
    SourceUDPPeer         string     // Transport peer from recvfrom()
    DeviceHostname        string     // sysName enrichment; _HOSTNAME falls back to SourceIP when empty
    DeviceVendor          string     // Vendor slug from PEN; omitted from output when empty
    PduType               PduType    // trap / inform
    SnmpVersion           SnmpVersion       // v1 / v2c / v3
    SourceVnodeID         string            // Source device Netdata vnode identity
    TopologyInterface     string            // Omitted from output when empty
    TopologyNeighbors     string            // Omitted from output when empty
    Labels                map[string]string // Nil means no labels; lowercase keys
    Varbinds              []VarbindValue    // Ordered varbind values from PDU
    SummaryCounts         *DedupSummary     // Only when ReportType = deduplication_summary
}
```

`VarbindValue.Value` is not serialized with raw `encoding/json` defaults. The implementation must use a single canonical helper that preserves ordered `Varbinds`, renders `[]byte` as deterministic hex/base64 according to the decoder decision, and rejects unsupported concrete Go types in tests. Every backend, including future OTLP, must use this canonical rendering path for `VarbindValue.Value` instead of raw `encoding/json` or backend-local type switches.

All maps attached to `TrapEntry` (`Labels`, `SummaryCounts.ByTrap`) must be immutable after `TrapWriter.Write()` succeeds. Dedup summary builders must deep-clone `ByTrap` before attaching it to a `TrapEntry`. If a future implementation needs `sync.Pool` reuse, it must either deep-copy maps/slices at the `Write()` boundary or prove by race tests that reused objects are not reachable by the writer.

`DisplayHint` is reserved in the profile schema and is intentionally absent from the first TrapEntry struct. When the renderer starts using MIB DISPLAY-HINT metadata, the extractor, profile format, and TrapEntry shape must be changed together.

Constants for the closed sets:

- **Category**: `state_change`, `config_change`, `security`, `auth`, `license`, `mobility`, `diagnostic`, `unknown`
- **Severity**: `emerg`, `alert`, `crit`, `err`, `warning`, `notice`, `info`, `debug`

### 5. Shared Profile Cache Lifecycle

The profile cache is **plugin-wide in-process state**, not per-job:

```go
// In profile.go — package-level state protected by sync.Mutex.
// SOW-0035 has one active generation; the per-generation holder map prevents
// the SOW-0037 hot-reload path from leaking references when old jobs release
// an index after a newer generation has become active.
type profileCacheGeneration struct {
    index   *ProfileIndex
    refs    int
    retired bool
}

var (
    profileCacheMu       sync.Mutex
    activeProfileGen     uint64
    nextProfileGen       uint64
    profileGenerations   map[uint64]*profileCacheGeneration
)

// AcquireProfileCache loads profiles on first call, increments refcount.
// Returns error if load/validation fails — caller surfaces via HTTP-422.
func AcquireProfileCache() (*ProfileIndex, uint64, error) { ... }

// ReleaseProfileCache decrements refcount; drops the index when refs == 0.
func ReleaseProfileCache(generation uint64) { ... }
```

Lifecycle:
- First `AcquireProfileCache()` call during job creation loads profiles from multipath (user dirs first, then stock), creates a new active generation, increments that generation's holder count, and returns both `*ProfileIndex` and generation ID.
- Every subsequent job creation increments the active generation's holder count and receives the same `*ProfileIndex`.
- Every job removal calls `ReleaseProfileCache(generation)`. Release decrements the holder count for the exact generation returned by acquire, even if that generation is no longer active. When a generation's holder count reaches 0, that generation is deleted and GC can reclaim its index.
- Agents with no trap jobs never call `AcquireProfileCache()`, so they never pay the profile memory cost
- Failed profile loads do not poison future attempts. The implementation must not use `sync.Once`; it must retry loading on the next job creation after a failed attempt.
- The mutex covers the full acquire-check-load-increment and release-decrement-delete sequence. No goroutine may observe a partially-initialized cache. A failed load creates no generation and leaves `profileGenerations` unchanged.
- The mutex also covers the entire release and underflow-recovery sequence. Jobs store the generation returned by `AcquireProfileCache()` and pass it to `ReleaseProfileCache(generation)`. Unknown-generation releases are programmer bugs; production recovery logs the error and ignores the release after tests have asserted the path. Underflow is a programmer bug; production recovery logs the error, deletes that generation, and if it was active clears `activeProfileGen` so the next acquire reloads cleanly.
- SOW-0035 does not implement hot reload, but the lifecycle deliberately reserves generation retirement: a later reload marks the old active generation retired, installs a new active generation, and lets old jobs keep using the immutable old `*ProfileIndex` until their exact-generation releases delete it.
- Tests need a package-private reset helper (for example `resetProfileCacheForTest`) so package-level state does not leak between test cases.

### 6. Creation-time Failure Detection

All job resources are validated synchronously in the go.d framework's job `Start()` callback before the job is reported as running:

| Resource | Validation | Failure code |
|---|---|---|
| Job name | `^[a-zA-Z0-9][a-zA-Z0-9_-]*$`, max 64 chars, no path separators/dots | HTTP-422 |
| Endpoint list | At least one endpoint, protocol supported (udp), address/port parseable | HTTP-422 |
| Endpoint bind | `net.ListenUDP()` on every configured endpoint; all-or-nothing cleanup on partial failure | HTTP-422 |
| Profile cache | `AcquireProfileCache()` returns error on load/parse failure | HTTP-422 |
| Journal directory | `os.MkdirAll()` + writability check on `/var/cache/netdata/traps/{job_name}/` | HTTP-422 |
| Journal writer | `NewJournalWriter(dir, cfg)` validates directory and retention config | HTTP-422 |

These errors must flow through DynCfg as `CodedError{code: 422}`. The current framework does **not** do this for every path:

- `src/go/plugin/agent/jobmgr/dyncfg_collector_callbacks.go:88-90` wraps `Start()` `createCollectorJob()` failures as `codedError{code: 400}`, hiding any inner 422.
- `src/go/plugin/agent/jobmgr/dyncfg_collector_callbacks.go:93-96` schedules retry and returns a plain error for `Start()` `AutoDetection()` failures.
- `src/go/plugin/agent/jobmgr/dyncfg_collector_callbacks.go:108-116` returns plain errors for both `Update()` `createCollectorJob()` and `AutoDetection()` failures.
- `src/go/plugin/framework/dyncfg/handler.go:506-509` honors `CodedError` for `CmdEnable`, but `CmdUpdate` callback failures currently send HTTP 200 at `handler.go:683`. Update payload parse/validation failures already return HTTP 400 before the callback path.

**Framework change needed** (small jobmgr + DynCfg handler edit, scoped to SOW-0035 M2):

1. Preserve an inner `CodedError` from `createCollectorJob()` instead of replacing it with hardcoded HTTP 400.
2. If `AutoDetection()` returns a `CodedError`, call `Cleanup()` and return that error without scheduling a retry; plain non-coded `AutoDetection()` errors keep the existing retry behavior for other collectors.
3. Make `Update()` mirror `Start()` for both `createCollectorJob()` and `AutoDetection()` coded errors.
4. Make the `CmdUpdate` callback error path at `src/go/plugin/framework/dyncfg/handler.go:683` honor `CodedError` response codes like `CmdEnable` does, instead of always sending HTTP 200 for `cb.Start()` / `cb.Update()` failure. Preserve the existing `ErrNonDisruptiveUpdate` rollback path at `handler.go:667-677` as HTTP 200 because the old config remains effective and runtime state did not change; trap creation-time failures must not use `ErrNonDisruptiveUpdate`.

Before changing shared DynCfg behavior, M2 must run a same-failure scan (`rg 'CodedError|codedError|MarkNonDisruptiveUpdate' src/go/plugin`) and add handler/jobmgr tests proving existing plain-error retry behavior remains unchanged while coded trap creation failures surface their HTTP status.

The trap plugin must still preflight all resources before it reports successful startup. `AutoDetection()` should be a no-op for traps unless a future SOW proves a cheap consistency check is needed; bind/profile/journal/writer/retention failures must be creation-time coded errors, not retry-loop events.

**Partial resource cleanup**: if endpoint 3 of 5 fails to bind after endpoints 1-2 succeeded, the previously bound endpoints are closed before returning the error. The job never enters the running state.

`createCollectorJob()` failure is not followed by framework `Cleanup()` because no job object is returned. The trap job factory must therefore own rollback for every partial resource acquired during creation: release profile-cache references, close bound sockets, close or remove partially-created writer state, and leave the journal directory in a valid empty-or-reusable state before returning the coded error.

On `Update()`, current jobmgr behavior stops the old running job before creating the replacement. If replacement creation fails, the trap job factory can only roll back partial resources from the failed new job; it cannot restore the stopped old job. This is shared framework behavior. M2 tests must capture the resulting failed status and coded response so operators see the apply-time failure clearly.

## Risks and Mitigations

| Risk | Mitigation |
|---|---|
| Go journal writer has format bugs undetected by `journalctl` | M4 end-to-end test replays a pcap through the full pipeline and queries with `journalctl --directory=...`; write tests against the journal file format spec |
| Go journal writer is larger than estimated (4K lines → 5.5K+ lines) | The backend is behind the `TrapWriter` interface; if Go-native exceeds the M4 midpoint budget, reopen the backend decision before adding a subprocess bridge |
| Active journal file is not queryable until rotation | Not acceptable for the MVP; maintain DATA/FIELD hash tables and ENTRY_ARRAY chains incrementally and test `journalctl --directory=...` before `Close()` |
| Shared profile cache refcount leak leaves memory allocated | `ReleaseProfileCache()` is called in `Cleanup()` which the framework guarantees on shutdown; add refcount-leak and underflow detection tests |
| Framework coded-error change breaks other collectors | Preserve existing behavior for plain errors; only `CodedError` suppresses retry and controls HTTP code; add Start and Update tests |
| Direct journal writer cannot sustain target trap volume | M4 must include `go test -benchmem` / throughput benchmarks for `TrapWriter.Write()`, queue drain, and journal `WriteEntry()`; if allocation or pwrite throughput misses the tens-of-thousands/sec target, reopen batching or backend design before accepting M4 |
| Journal hash/header assumptions drift from the Rust implementation | Port the keyed SipHash/Jenkins behavior and header-size handling from `journal-core`; validate active, rotated, and crash-interrupted files with `journalctl --directory=...` |
| Crash recovery corrupts active file after interrupted write | Implement the recovery scan/truncate/rebuild algorithm above; validate with `journalctl --directory=...` and `journalctl --verify` after an interrupted writer |

## Validation Requirements

- [ ] ADR reviewed by all 5 external reviewers (glm, kimi, mimo, minimax, qwen) — consensus
- [ ] ADR reviewed by coordinating assistant
- [ ] Spec §5, §13, §19 updated to reflect decisions
- [ ] Spec §13 "Open Questions" item 1 marked resolved
- [ ] `audit.sh` passes
- [ ] `git diff --check` passes

## Consequences

### Positive

- Single process, no IPC, no CGo, no child process management
- Shared profile cache is trivial (Go package-level state + refcount)
- Job lifecycle is the well-understood go.d pattern operators and maintainers already know
- TrapWriter interface isolates the journal format concern; backend is swappable
- All creation-time failures surface as coded DynCfg errors

### Negative

- Go journal writer must be written from scratch (~4K-5.5K lines estimate) rather than reusing the proven Rust crate
- The journal binary format has edge cases (optional compression flags, hash table sizing, tag/type byte semantics) that must be tested thoroughly
- Framework coded-error change touches shared go.d infrastructure (small edit, needs cross-module test validation)
- Regular I/O means `journalctl` may observe new low-volume entries up to the 1s flush cadence later. This is acceptable for the MVP; `Flush()` and `Close()` remain explicit durability/visibility boundaries.

### Neutral

- The trap module adds a new collector to the go.d import registry (`collector/init.go`), which increases the `go.d.plugin` binary size slightly for *all* users — but only users who create trap jobs pay the profile memory cost (lazy load)

## References

- Systemd Journal File Format: https://systemd.io/JOURNAL_FILE_FORMAT/
- Existing Rust journal-log-writer: `src/crates/journal-log-writer/src/log/mod.rs`
- Existing NetFlow journal retention: `src/crates/netflow-plugin/src/plugin_config/types/journal.rs`
- go.d DynCfg callbacks: `src/go/plugin/agent/jobmgr/dyncfg_collector_callbacks.go`
- go.d codedError: `src/go/plugin/agent/jobmgr/dyncfg_collector_callbacks.go:140-147`
- go.d V2 collector pattern: `src/go/plugin/go.d/collector/ping/collector.go`
- SNMP profile loader multipath pattern: `src/go/plugin/go.d/collector/snmp/ddsnmp/load.go:270-286`
- Design spec: `.agents/sow/specs/snmp-traps/netdata.md` §5, §11, §19
