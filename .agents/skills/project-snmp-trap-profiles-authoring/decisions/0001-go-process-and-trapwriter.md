# ADR-0001: Go Process Model, Journal Writer Backend, and TrapWriter Contract

**Status**: Accepted for SOW-0035 implementation after reviewer round 5; amended on 2026-05-26 to use the published Go journal SDK `go/v0.1.0`; amended on 2026-05-28 to use SDK `go/v0.3.0`; amended on 2026-05-31 to use SDK `go/v0.4.0`; amended on 2026-06-01 by SOW-0045 to route the trap writer hot path through reusable raw journal payload serialization; amended on 2026-06-10 to use SDK `go/v0.6.3`; amended on 2026-06-11 to use SDK `go/v0.6.4`, place direct journals under `${NETDATA_LOG_DIR}/traps/`, and classify startup environment failures as retryable HTTP-503 coded errors.
**Date**: 2026-05-25
**SOW**: SOW-0035 M1

## Context

The Netdata SNMP trap subsystem (design spec: `.agents/skills/project-snmp-trap-profiles-authoring/netdata.md`) needs a concrete implementation architecture decision for three interlocking concerns:

1. **Process model**: Where does the trap plugin live in the Netdata process tree?
2. **Journal writer backend**: How do we write per-job journal files at `${NETDATA_LOG_DIR}/traps/{job_name}/` in Go, compatible with SDK-backed `snmp:traps` Function queries and optional `journalctl --directory=...` validation?
3. **TrapWriter interface + TrapEntry shape**: What is the concrete Go contract between the trap pipeline and storage backends?

The implementation language is **Go** (user decision, 2026-05-25). The journal writer must produce real systemd journal binary-format files so end-to-end acceptance criteria (M4: `journalctl --directory=${NETDATA_LOG_DIR}/traps/test/ TRAP_CATEGORY=security`) passes.

## Decision Drivers

1. **Fit for purpose** — the architecture must blend with existing Netdata patterns. The go.d framework already owns job orchestration (DynCfg Add/Enable/Update/Disable/Remove), coded-error surfacing in the dashboard, and the V2 collector lifecycle.
2. **Minimize blast radius** — new process boundaries, CGo dependencies, or IPC bridges add failure modes, build complexity, and operational surface that must be continuously tested.
3. **Creation-time failure detection** — all job resources (bind, profile load, journal directory, writer init, retention) must be validated before DynCfg reports the job as started. This is a user-facing correctness contract per spec §5.
4. **Share nothing, share once** — the trap profile cache loads on first runnable job creation, is shared across all listeners, and is released when no runnable jobs remain. In-process sharing is trivial; cross-process sharing adds synchronization, IPC, and lifecycle coordination.
5. **journalctl compatibility** — the M4 acceptance criterion requires `journalctl --directory=...` to work. This means real systemd journal binary-format files (not plain text, not SQLite). The `journalctl` tool reads the binary format documented at https://systemd.io/JOURNAL_FILE_FORMAT/.
6. **Simplicity** — avoid over-engineering. Prefer standard go.d module code unless evidence justifies another boundary.

## Options Considered

### Option A: Standard in-process go.d module + SDK-backed Go journal writer (SELECTED)

- Trap plugin lives as a standard go.d collector module at `src/go/plugin/go.d/collector/snmp_traps/`
- Registered through the standard `collectorapi.Register(...)` path in the existing `collector/init.go` import registry as `snmp_traps`, mirroring the existing `snmp_topology` naming style. A scan found no existing go.d collector registration name containing a dot, so the module name must not use `snmp.traps`.
- Uses V2 collector interface (`collectorapi.CollectorV2`), mirroring the `ping/` collector pattern
- Job lifecycle managed by the existing go.d framework (`src/go/plugin/agent/jobmgr/dyncfg_collector_callbacks.go`)
- Journal writing via a thin adapter around `github.com/netdata/systemd-journal-sdk/go/journal` `go/v0.6.4`, keeping the local `TrapWriter` abstraction and delegating journal file format, active-file indexing, rotation, retention, and writer locking to the SDK.
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
| DynCfg job orchestration | `src/go/plugin/agent/jobmgr/dyncfg_collector_callbacks.go` (`collectorCallbacks.Start`) | `Start()` preflight + coded errors |
| codedError for HTTP-422 | `src/go/plugin/agent/jobmgr/dyncfg_collector_callbacks.go` (`codedError`) | `type codedError struct` with `DyncfgCode() int` |
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

Historical note (2026-05-26): this sizing drove the original M1 risk decision.
It is superseded for implementation by the SDK dependency recorded above; the
trap package now owns only the adapter, field mapping, queueing, and validation
around the SDK, not the journal binary format itself.

### Why not CGo / subprocess

1. **CGo** adds a C toolchain dependency to `go.d.plugin`, which currently builds pure Go (no CGo). Cross-compilation becomes harder. The Rust crate's FFI surface would need C-compatible wrappers.
2. **Subprocess** requires the trap plugin to manage a child process (start, health-check, restart on crash, graceful shutdown ordering). This is a whole class of reliability bugs that don't exist with in-process code. The Rust binary also needs to be built and shipped alongside go.d.plugin.
3. **libsystemd `sd_journal_sendv()`** via CGo would write through journald, not directly to per-job directories. The `_HOSTNAME` field (source device hostname) would be controlled by journald, not the plugin — violating spec §11: "the trap plugin's journal writer writes directly to journal files (bypassing journald) and controls every field."

## Decision

**Option A is selected**: standard in-process go.d module + SDK-backed Go journal writer.

### 1. Process Model

The SNMP trap plugin lives as a standard go.d collector V2 module:

```
src/go/plugin/go.d/collector/snmp_traps/
    collector.go         # Main struct, init() registration, Config, New(), lifecycle
    init.go              # validateConfig(), one-shot initialization
    collect.go           # collect() — per-cycle metric emission
    listener.go          # Per-job UDP listener binding (M2)
    decode.go            # BER limit pre-scan + gosnmp trap parse + RFC 3584 (M2)
    profile.go           # Profile type + shared lazy loader (M3)
    resolver.go          # 2-tier varbind resolution + template rendering (M3)
    journal_writer.go    # SDK-backed journal adapter + retention mapping (M4)
    trapentry.go         # TrapEntry type definitions
    trapwriter.go        # TrapWriter interface definition
    *.go_test.go         # Table-driven tests
```

Collector consistency artifacts (`metadata.yaml`, health, README, taxonomy) remain owned by SOW-0039 unless an earlier SOW needs a minimal internal test fixture. SOW-0035 M2 ships the minimal `config_schema.json` and disabled stock config needed for DynCfg creation-time preflight and manual opt-in; SOW-0039 remains responsible for the full user-facing documentation and integration metadata bundle.

Registration in `src/go/plugin/go.d/collector/init.go`:

```go
_ "github.com/netdata/netdata/go/plugins/plugin/go.d/collector/snmp_traps"
```

### 2. Journal Writer Backend

The local package provides a small `JournalWriter` adapter in
`src/go/plugin/go.d/collector/snmp_traps/journal_writer.go`. It does **not**
implement the systemd journal binary format locally. The adapter delegates file
format, active-file indexing, writer locks, rotation, retention, and chain
validation/reopen behavior to `github.com/netdata/systemd-journal-sdk/go/journal`
`go/v0.6.4`.

The public local API remains:

```go
// JournalWriter is a thin adapter over journal.Log.
// Files are readable by journalctl --directory=<JournalDirectory()>.
type JournalWriter struct { ... }

// NewJournalWriter creates a writer for the given directory.
// On creation, it eagerly opens the SDK log and validates the directory,
// writer lock, active file, machine ID, boot ID, rotation policy, and
// retention policy.
func NewJournalWriter(dir string, cfg JournalConfig) (*JournalWriter, error)

type JournalField struct {
    Name  string // Validated journal field name.
    Value []byte // Raw value bytes passed to the SDK journal writer.
}

// WriteEntry writes one journal entry. It is synchronous and is called only by
// the journal TrapWriter's single queue worker goroutine, not by the decode hot
// path. WriteEntry is not concurrency-safe.
func (w *JournalWriter) WriteEntry(fields []JournalField, realtimeUsec, monotonicUsec int64) error

// WriteRawEntry writes one journal entry from prebuilt KEY=value payloads.
// It is an internal hot-path API used by the journal TrapWriter's single worker
// after SOW-0045. The SDK consumes the payloads synchronously during AppendRaw.
func (w *JournalWriter) WriteRawEntry(payloads [][]byte, sanitizedFields int, realtimeUsec, monotonicUsec int64) error

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

**SDK configuration**:

- `journal.NewLog(dir, journal.LogConfig{...})` receives the configured
  per-job root `${NETDATA_LOG_DIR}/traps/{job_name}/`.
- The plugin checks that `${NETDATA_LOG_DIR}` already exists before calling the
  SDK. It creates only the Netdata-owned trap child tree.
- The SDK appends `<machine-id>/`; `JournalWriter.JournalDirectory()` returns
  the effective query directory for `journalctl --directory`.
- `LogOpenEager` is mandatory so active journal file creation/open and writer
  lock acquisition fail during job creation.
- `LogIdentityStrict` is mandatory. The adapter reads `/etc/machine-id` and
  `/proc/sys/kernel/random/boot_id`; missing or malformed values are
  creation-time failures.
- `RotationPolicy` maps from parsed `retention.rotation_size` and
  `retention.rotation_duration`.
- `RetentionPolicy` maps from parsed `retention.max_size` and
  `retention.max_duration`.
- The SDK owns journal format compliance and retention deletion behavior. The
  trap package validates this through SDK-backed `journalctl --directory`
  tests, not by maintaining local DATA/FIELD hash-table code.

The journal-direct TrapWriter owns the concurrency boundary: multiple endpoint receive loops may call `TrapWriter.Write()` concurrently, but they fan into one concurrency-safe bounded queue per job. A single worker goroutine drains that queue and is the only caller of `JournalWriter.WriteRawEntry()` in the hot path.

The serialization boundary is explicit:

```go
// serializeToJournalFields converts TrapEntry to journal fields per netdata.md §11.
func serializeToJournalFields(entry *TrapEntry) ([]JournalField, error)
```

This function remains the simple allocation-returning reference API and test contract for `TrapEntry` to journal naming, including `PRIORITY`, `SYSLOG_IDENTIFIER`, `ND_LOG_SOURCE`, `ND_NIDL_NODE`, `_HOSTNAME` fallback, `TRAP_TAG_<KEY_UPPERCASE>`, `TRAP_JSON`, and omission of empty optional enrichment fields. The hot path uses an equivalent reusable serializer owned by the single worker and passes raw `KEY=value` payloads to `WriteRawEntry()`. It returns an error for structurally invalid entries (for example nil entry or missing required fields). Unsupported `VarbindValue.Value` concrete types use the guarded fallback path in production and are rejected by tests.

**Config parsing**: `JournalConfig` receives parsed byte/duration values. The config/DynCfg layer parses human-readable operator values such as `10GB` and `1h`.

**Rotation**: triggered by file size reaching `RotateSize` or file age reaching `RotateDur`. `RotateSize=0` means auto-calculate from `MaxSize / 20` and clamp to 5MB-200MB; if `MaxSize=0`, use the 200MB upper clamp as the default rotation size. Oversized single entries are written to the current file and can make that file exceed `RotateSize`; the next append opens a new file. `RotateDur=0` disables time-based rotation; the user-facing default remains `1h`. On rotation, the current file is finalized (archived) and a new file is opened. If `MaxSize=0`, size-based retention is disabled but rotation still occurs; files accumulate until an age cap or external cleanup removes them.

**Retention**: the adapter calls `Log.EnforceRetention()` on periodic sweeps so SDK retention is honored even during low-volume periods with no rotations.

The default writer queue capacity is 10,000 entries per job unless implementation tests prove a different default is safer. Queue-full and permanent-writer-failed errors are both drop-and-continue conditions for the caller: increment the per-job `journal_write_failed` dimension, discard that entry, and continue receiving traps. There is no disk spillover in the MVP.

The default flush cadence is time-only: 1s on a ticker, plus `Flush()`/`Close()` (the original count-based 1,000-entry trigger was later removed for throughput). `Flush()` is synchronous and concurrency-safe: it creates a barrier, waits until all entries accepted before that barrier have been written, and calls `fdatasync()`/`Sync()` before returning; entries accepted after the barrier may be flushed by a later cadence. `Close()` is concurrency-safe and idempotent. On first call, it stops new acceptance, drains the queue, finalizes the active journal file, best-effort sets file state to `archived`, syncs, and closes. If drain, finalization, or sync fails, `Close()` returns that terminal error, records the writer as permanently failed, and subsequent `Close()` calls return the stored terminal error; after `Close()` returns, `Write()` returns a closed-writer error.

The journal-direct backend is Linux-only for SOW-0035 because it depends on the Linux boot ID path and `journalctl` semantics. On non-Linux platforms, job creation must fail with a clear coded unsupported-backend error instead of starting and failing at runtime.

The Linux-only guard is checked before resource acquisition in module/job initialization. If the trap module is built on a non-Linux platform, trap job creation returns HTTP-422 unsupported backend before profile loading, socket binding, or journal directory creation.

### 3. TrapWriter Interface

Per spec §19, proposed:

```go
// TrapWriter is the contract between the trap pipeline and storage backends.
// Each job has exactly one TrapWriter. The journal-direct backend writes to
// ${NETDATA_LOG_DIR}/traps/{job_name}/; the OTLP backend (SOW-0038) implements
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
    ReportTypeTrap         ReportType = "trap"
    ReportTypeDedupSummary ReportType = "deduplication_summary"
    ReportTypeDecodeError  ReportType = "decode_error"
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

type DecodeErrorInfo struct {
    Kind          string `json:"kind"`
    Error         string `json:"error"`
    PacketSize    int    `json:"packet_size"`
    PacketSHA256  string `json:"packet_sha256"`
    SourceUDPPort int    `json:"source_udp_port,omitempty"`
    Listener      string `json:"listener,omitempty"`
    SnmpVersion   string `json:"snmp_version,omitempty"`
    EngineID      string `json:"engine_id,omitempty"`
}

type TrapEntry struct {
    JobName               string     // Which job produced this entry
    ReportType            ReportType // trap / deduplication_summary / decode_error
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
    DecodeError           *DecodeErrorInfo  // Only when ReportType = decode_error
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
| Endpoint bind | `net.ListenUDP()` on every configured endpoint; all-or-nothing cleanup on partial failure | HTTP-503 retryable |
| Profile cache | `AcquireProfileCache()` returns error on load/parse failure | HTTP-422 |
| Netdata log directory | `os.Stat("${NETDATA_LOG_DIR}")`; parent must already exist and be a directory | HTTP-503 retryable |
| Journal directory | SDK creates/opens `${NETDATA_LOG_DIR}/traps/{job_name}/`; failure is all-or-nothing cleanup | HTTP-503 retryable |
| Journal writer | `NewJournalWriter(dir, cfg)` validates directory and retention config | HTTP-503 retryable for environment failures; HTTP-422 for invalid retention config |

These errors must flow through DynCfg as coded errors with the resource-specific code above. Non-retryable configuration/profile errors use HTTP-422; retryable startup/environment errors use HTTP-503 and implement `DyncfgRetryable() bool` so file-configured jobs can retry after the transient condition clears.

- `src/go/plugin/agent/jobmgr/dyncfg_collector_callbacks.go` (`collectorCallbacks.Start`) wraps `createCollectorJob()` failures as `codedError{code: 400}`, hiding any inner 422.
- `src/go/plugin/agent/jobmgr/dyncfg_collector_callbacks.go` (`collectorCallbacks.Start`) schedules retry and returns a plain error for `AutoDetection(ctx)` failures.
- `src/go/plugin/agent/jobmgr/dyncfg_collector_callbacks.go` (`collectorCallbacks.Update`) returns plain errors for both `createCollectorJob()` and `AutoDetection(ctx)` failures.
- `src/go/plugin/framework/dyncfg/handler.go:506-509` honors `CodedError` for `CmdEnable`, but `CmdUpdate` callback failures currently send HTTP 200 at `handler.go:683`. Update payload parse/validation failures already return HTTP 400 before the callback path.

**Framework change needed** (small jobmgr + DynCfg handler edit, scoped to SOW-0035 M2):

1. Preserve an inner `CodedError` from `createCollectorJob()` instead of replacing it with hardcoded HTTP 400.
2. If `AutoDetection(ctx)` returns a `CodedError`, call `Cleanup()` and return that error. Schedule a retry only when the error also implements `DyncfgRetryable() bool` and returns `true`; plain non-coded `AutoDetection(ctx)` errors keep the existing retry behavior for other collectors.
3. Make `Update()` mirror `Start()` for both `createCollectorJob()` and `AutoDetection()` coded errors.
4. Make the `CmdUpdate` callback error path at `src/go/plugin/framework/dyncfg/handler.go:683` honor `CodedError` response codes like `CmdEnable` does, instead of always sending HTTP 200 for `cb.Start()` / `cb.Update()` failure. Preserve the existing `ErrNonDisruptiveUpdate` rollback path at `handler.go:667-677` as HTTP 200 because the old config remains effective and runtime state did not change; trap creation-time failures must not use `ErrNonDisruptiveUpdate`.

Before changing shared DynCfg behavior, M2 must run a same-failure scan (`rg 'CodedError|codedError|MarkNonDisruptiveUpdate' src/go/plugin`) and add handler/jobmgr tests proving existing plain-error retry behavior remains unchanged while coded trap creation failures surface their HTTP status.

The trap plugin must still preflight all resources before it reports successful startup. `AutoDetection(ctx)` should be a no-op for traps unless a future SOW proves a cheap consistency check is needed; bind/profile/journal/writer/retention failures must be creation-time coded errors, not retry-loop events.

**Partial resource cleanup**: if endpoint 3 of 5 fails to bind after endpoints 1-2 succeeded, the previously bound endpoints are closed before returning the error. The job never enters the running state.

`createCollectorJob()` failure is not followed by framework `Cleanup()` because no job object is returned. The trap job factory must therefore own rollback for every partial resource acquired during creation: release profile-cache references, close bound sockets, close or remove partially-created writer state, and leave the journal directory in a valid empty-or-reusable state before returning the coded error.

On `Update()`, current jobmgr behavior stops the old running job before creating the replacement. If replacement creation fails, the trap job factory can only roll back partial resources from the failed new job; it cannot restore the stopped old job. This is shared framework behavior. M2 tests must capture the resulting failed status and coded response so operators see the apply-time failure clearly.

## Risks and Mitigations

| Risk | Mitigation |
|---|---|
| Go journal writer has format bugs undetected by `journalctl` | M4 end-to-end test replays a pcap through the full pipeline and queries with `journalctl --directory=...`; write tests against the journal file format spec |
| SDK-backed adapter fails creation-time preflight | Use `LogOpenEager` + `LogIdentityStrict` and wrap errors as coded DynCfg job-creation failures |
| Active journal file is not queryable until rotation | Not acceptable for the MVP; validate SDK-backed active files with `journalctl --directory=...` before `Close()` |
| Shared profile cache refcount leak leaves memory allocated | `ReleaseProfileCache()` is called in `Cleanup()` which the framework guarantees on shutdown; add refcount-leak and underflow detection tests |
| Framework coded-error change breaks other collectors | Preserve existing behavior for plain errors; only `CodedError` suppresses retry and controls HTTP code; add Start and Update tests |
| Direct journal writer cannot sustain target trap volume | M4 must include `go test -benchmem` / throughput benchmarks for `TrapWriter.Write()`, queue drain, and SDK-backed journal `WriteEntry()`; if allocation or throughput misses the tens-of-thousands/sec target, reopen batching or backend design before accepting M4 |
| SDK dependency API drifts | Pin to `github.com/netdata/systemd-journal-sdk/go v0.6.4`; re-vendor through the module tag and review API changes before updating |
| SDK chain handling has an upstream defect | Keep `TrapWriter` and local adapter boundaries narrow so the SDK can be updated or replaced without changing ingestion semantics |

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
- TrapWriter interface isolates the journal backend concern; backend is swappable
- All creation-time failures surface as coded DynCfg errors

### Negative

- The trap module now depends on the external `github.com/netdata/systemd-journal-sdk/go` module and its transitive compression libraries
- SDK API or behavior changes need explicit re-vendoring/review before dependency updates
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
- Design spec: `.agents/skills/project-snmp-trap-profiles-authoring/netdata.md` §5, §11, §19
