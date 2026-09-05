# ADR-0001: Go Process Model, Journal Writer Backend, and Output Writer Contract

**Status**: Accepted for SOW-0035 implementation after reviewer round 5; amended through 2026-08-12 to use the
published Go journal SDK `go/v0.8.1`, route the journal hot path through one reusable raw-payload serializer, place direct
journals under `${NETDATA_LOG_DIR}/traps/`, classify startup environment failures as retryable HTTP-503 coded errors,
remove profile hot reload, and give output components explicit internal package and lifecycle ownership.
**Date**: 2026-05-25
**SOW**: SOW-0035 M1

## Context

The Netdata SNMP trap subsystem (design spec: `.agents/skills/collectors-snmp-trap-profiles/netdata.md`) needs a concrete implementation architecture decision for three interlocking concerns:

1. **Process model**: Where does the trap plugin live in the Netdata process tree?
2. **Journal writer backend**: How do we write per-job journal files at `${NETDATA_LOG_DIR}/traps/{job_name}/` in Go, compatible with SDK-backed `snmp:traps` Function queries and optional `journalctl --directory=...` validation?
3. **Output writer interface + TrapEntry shape**: What is the concrete Go contract between the trap pipeline and storage backends?

The implementation language is **Go** (user decision, 2026-05-25). The journal writer must produce real systemd journal binary-format files so end-to-end acceptance criteria (M4: `journalctl --directory=${NETDATA_LOG_DIR}/traps/test/ TRAP_CATEGORY=security`) passes.

## Decision Drivers

1. **Fit for purpose** — the architecture must blend with existing Netdata patterns. The go.d framework already owns job orchestration (DynCfg Add/Enable/Update/Disable/Remove), coded-error surfacing in the dashboard, and the V2 collector lifecycle.
2. **Minimize blast radius** — new process boundaries, CGo dependencies, or IPC bridges add failure modes, build complexity, and operational surface that must be continuously tested.
3. **Creation-time failure detection** — all job resources (bind, eager profile-catalog surfaces, journal directory, writer
   init, retention) must be validated before DynCfg reports the job as started. Lazy stock vendor YAML is not a job
   resource; a malformed lazy file fails the first matching lookup. This is a user-facing correctness contract per spec
   §5.
4. **Share nothing, share once** — the trap profile catalog creates one immutable configuration epoch on first runnable
   job creation, shares that epoch across all listeners created by the same plugin registration, and releases it when no
   runnable jobs remain. In-process sharing is trivial; cross-process sharing adds synchronization, IPC, and lifecycle
   coordination.
5. **journalctl compatibility** — the M4 acceptance criterion requires `journalctl --directory=...` to work. This means real systemd journal binary-format files (not plain text, not SQLite). The `journalctl` tool reads the binary format documented at https://systemd.io/JOURNAL_FILE_FORMAT/.
6. **Simplicity** — avoid over-engineering. Prefer standard go.d module code unless evidence justifies another boundary.

## Options Considered

### Option A: Standard in-process go.d module + SDK-backed Go journal writer (SELECTED)

- Trap plugin lives as a standard go.d collector module at `src/go/plugin/go.d/collector/snmp_traps/`
- Registered through the standard `collectorapi.Register(...)` path in the existing `collector/init.go` import registry as `snmp_traps`, mirroring the existing `snmp_topology` naming style. A scan found no existing go.d collector registration name containing a dot, so the module name must not use `snmp.traps`.
- Uses V2 collector interface (`collectorapi.CollectorV2`), mirroring the `ping/` collector pattern
- Job lifecycle managed by the existing go.d framework (`src/go/plugin/agent/jobmgr/dyncfg_collector_callbacks.go`)
- Journal writing via a thin adapter around `github.com/netdata/systemd-journal-sdk/go/journal` `go/v0.8.1`, keeping the internal `output.Writer` abstraction and delegating journal file format, active-file indexing, rotation, retention, and writer locking to the SDK.
- Shared profile catalog: an in-process manager owned by the plugin registration, loaded on first runnable job creation

### Option B: Separate Go process (external plugin) via PLUGINSD

- Trap plugin runs as a standalone Go binary using the PLUGINSD protocol (`src/plugins.d/README.md`)
- Communicates metrics via stdout `BEGIN/SET/END` lines
- Journal writing must happen in-process in the separate binary or via a second bridge
- Profile catalog lives in the separate process; cross-process sharing with any future Go go.d enrichment code requires netipc

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
    collector.go                 # Framework adapter, authority selection, metrics, lifecycle wiring
    config.go / init.go          # Public config DTOs and validation/normalization adapter
    internal/model/              # Shared semantic trap model
    internal/output/             # Writer contract, coordinator, typed outcomes, shared projection
    internal/output/journal/     # Retention, SDK adapter, raw serializer, queue worker
    internal/output/otlp/        # OTLP policy, preflight, queue/batch worker, protobuf projection
    internal/receiver/           # UDP receive loops, SNMP decode/admission, v3 state, INFORM responses
    internal/snmptrapsfunc/      # Logs Function implementation
    pipeline.go / enrich.go      # Post-acceptance enrichment, rendering, and pipeline behavior
```

Collector consistency artifacts (`metadata.yaml`, health, README, taxonomy) remain owned by SOW-0039 unless an earlier SOW needs a minimal internal test fixture. SOW-0035 M2 ships the minimal `config_schema.json` and disabled stock config needed for DynCfg creation-time preflight and manual opt-in; SOW-0039 remains responsible for the full user-facing documentation and integration metadata bundle.

Registration in `src/go/plugin/go.d/collector/init.go`:

```go
_ "github.com/netdata/netdata/go/plugins/plugin/go.d/collector/snmp_traps"
```

#### Final ownership outcome

The initial layout above was an implementation starting point. The completed ownership split keeps the selected
in-process model and writer contract, but moves all per-job lifecycle and packet orchestration into
`internal/jobruntime`. The root `collector.go` is now the framework adapter; `config.go` / `init.go` own public DTOs and
normalization; `register.go` / `services.go` compose plugin-scoped services; and `internal/jobruntime` owns acquired
resources, rollback, cleanup, synchronous packet handling, and metric collection. The former root `pipeline.go`,
`decode_error.go`, `enrich.go`, and compatibility bridge files were removed rather than retained as forwarding layers.

### 2. Journal Writer Backend

`internal/output/journal` owns the journal backend. It does **not** implement the systemd journal binary format locally;
it delegates format, active-file indexing, writer locks, rotation, retention, and chain validation/reopen behavior to
`github.com/netdata/systemd-journal-sdk/go/journal` `go/v0.8.1`.

The backend has an explicit two-phase lifecycle:

- `journal.Prepare(dir, cfg, host, opts)` eagerly opens the SDK log and proves the directory, lock, active file, host
  identity, rotation policy, and retention policy without starting a goroutine.
- `Writer.Start()` starts the one queue worker after every job resource has passed preflight.
- `Writer.Write()`, `Flush()`, `Close()`, and `BinaryEncodedFields()` expose only the runtime behavior needed
  by the job runtime and output-owner tests. Closing a prepared-but-not-started writer is safe.
- The package owns parsed `Retention` and SDK-facing `Config`; the root package retains only the human-readable config DTO
  and maps it into these normalized types.

**SDK configuration**:

- `journal.NewLog(dir, journal.LogConfig{...})` receives the configured
  per-job root `${NETDATA_LOG_DIR}/traps/{job_name}/`.
- The plugin checks that `${NETDATA_LOG_DIR}` already exists before calling the
  SDK. It creates only the Netdata-owned trap child tree.
- The SDK appends `<machine-id>/`; `journalctl --directory` can query the configured per-job root recursively.
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

The journal backend owns the concurrency boundary: multiple endpoint receive loops call `Writer.Write()` concurrently and
fan into one bounded queue per job. One worker owns one reusable serializer and is the only caller of SDK `AppendRaw()`.
There is no allocation-returning reference serializer or field-slice write API. Tests decode and assert the actual ordered
raw `KEY=value` payloads emitted by the production serializer.

The serializer owns journal naming including `PRIORITY`, `SYSLOG_IDENTIFIER`, `ND_LOG_SOURCE`, `ND_NIDL_NODE`,
`_HOSTNAME` fallback, `TRAP_TAG_<KEY_UPPERCASE>`, indexed `TRAP_VAR_*` fields, `TRAP_JSON`, sensitive-varbind omission,
and empty optional-field omission. Structurally invalid entries fail in the worker. Shared label ordering and canonical
varbind projection live in `internal/output` and are reused by OTLP without adding an allocation to journal enqueue.

**Rotation**: triggered by file size reaching `RotateSize` or file age reaching `RotateDur`. `RotateSize=0` means auto-calculate from `MaxSize / 20` and clamp to 5MB-200MB; if `MaxSize=0`, use the 200MB upper clamp as the default rotation size. Oversized single entries are written to the current file and can make that file exceed `RotateSize`; the next append opens a new file. `RotateDur=0` disables time-based rotation and is the user-facing default. On rotation, the current file is finalized (archived) and a new file is opened. If `MaxSize=0`, size-based retention is disabled but rotation still occurs; files accumulate until an age cap or external cleanup removes them.

**Retention**: the adapter calls `Log.EnforceRetention()` on periodic sweeps so SDK retention is honored even during low-volume periods with no rotations.

The default writer queue capacity is 10,000 entries per job unless implementation tests prove a different default is safer. Queue-full and permanent-writer-failed errors are both drop-and-continue conditions for the caller: increment the per-job `journal_write_failed` dimension, discard that entry, and continue receiving traps. There is no disk spillover in the MVP.

The default flush cadence is time-only: 1s on a ticker, plus `Flush()`/`Close()` (the original count-based 1,000-entry trigger was later removed for throughput). `Flush()` is synchronous and concurrency-safe: it creates a barrier, waits until all entries accepted before that barrier have been written, and calls `fdatasync()`/`Sync()` before returning; entries accepted after the barrier may be flushed by a later cadence. `Close()` is concurrency-safe and idempotent. On first call, it stops new acceptance, drains the queue, finalizes the active journal file, best-effort sets file state to `archived`, syncs, and closes. If drain, finalization, or sync fails, `Close()` returns that terminal error, records the writer as permanently failed, and subsequent `Close()` calls return the stored terminal error; after `Close()` returns, `Write()` returns a closed-writer error.

The journal-direct backend is Linux-only for SOW-0035 because it depends on the Linux boot ID path and `journalctl` semantics. On non-Linux platforms, job creation must fail with a clear coded unsupported-backend error instead of starting and failing at runtime.

The Linux-only guard is checked before resource acquisition in module/job initialization. If the trap module is built on a non-Linux platform, trap job creation returns HTTP-422 unsupported backend before profile loading, socket binding, or journal directory creation.

### 3. Output Writer Interface

Per spec §19, proposed:

```go
// Writer is the contract between the trap pipeline and storage backends.
// Each job has exactly one Writer. The journal-direct backend writes to
// ${NETDATA_LOG_DIR}/traps/{job_name}/; the OTLP backend (SOW-0038) implements
// the same interface with protobuf serialization.
type Writer interface {
    Write(entry *TrapEntry) error   // Fast accept into backend-owned queue or return drop-worthy error
    Flush() error                   // Durability boundary
    Close() error                   // Idempotent close
}
```

Semantics:

- **Write() does not perform blocking disk or network I/O on the decode hot path.** It returns after the backend has accepted ownership of the immutable entry into a bounded internal queue. If the queue is full or the backend is in a permanent failed state, `Write()` returns an error; the caller increments `journal_write_failed` and drops the trap while the hot path continues.
- **Every coordinator `Write()` invocation consumes the entry.** The caller must not mutate maps, slices, or strings
  reachable from the entry after the call, regardless of the returned authoritative-backend error. This preserves fanout:
  the secondary backend may have accepted the same entry even when the primary backend returns an error.
- **An individual backend retains only successfully enqueued entries.** A backend that returns an error from its own
  `Write()` has not retained that entry. Reusing the entry after a coordinator call remains invalid.
- **Flush()** creates a queue barrier, waits for all entries accepted before that barrier to be written, and calls `Sync()`/`fdatasync()` on the underlying journal writer, forcing all buffered data needed for `journalctl` visibility and shutdown durability to disk.
- **Close()** is concurrency-safe and idempotent. On first call, it drains the queue, finalizes the active journal file, best-effort sets file state to archived, syncs, and closes. Subsequent calls return nil after a successful first close, or the stored terminal error after a failed first close.
- **Backend-internal batching** is the writer's responsibility. The interface does not expose batching.
- **CWE-117** is owned by the journal serializer and raw SDK append path, not the interface.

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
// The canonical serializer renders any other concrete type through a guarded
// string fallback; tests pin the fallback behavior.
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

`VarbindValue.Value` is not serialized with raw `encoding/json` defaults. The implementation must use a single canonical
helper that preserves ordered `Varbinds`, renders `[]byte` deterministically as hexadecimal, and uses a guarded string
fallback for unsupported concrete Go types. Every backend must use this canonical rendering path for
`VarbindValue.Value` instead of raw `encoding/json` or backend-local type switches.

All maps attached to `TrapEntry` (`Labels`, `SummaryCounts.ByTrap`) must be immutable after `output.Writer.Write()` is
called. Dedup summary builders must deep-clone `ByTrap` before attaching it to a `TrapEntry`. If a future implementation
needs `sync.Pool` reuse, it must deep-copy maps/slices before the `Write()` boundary or prove by race tests that reused
objects are not reachable by any backend.

`DisplayHint` is reserved in the profile schema and is intentionally absent from the first TrapEntry struct. When the renderer starts using MIB DISPLAY-HINT metadata, the extractor, profile format, and TrapEntry shape must be changed together.

Constants for the closed sets:

- **Category**: `state_change`, `config_change`, `security`, `auth`, `license`, `mobility`, `diagnostic`, `unknown`
- **Severity**: `emerg`, `alert`, `crit`, `err`, `warning`, `notice`, `info`, `debug`

### 5. Shared Profile Catalog Lifecycle

`internal/catalog.Manager` is **plugin-registration-owned in-process state**, not package-global state and not per-job
state. The plugin factory constructs one manager with explicit operator and stock paths; every collector created by that
factory acquires its own exact-once `Lease`:

```go
manager := catalog.NewManager(catalog.Paths{UserDirs: userDirs, StockDir: stockDir})
lease, err := manager.Acquire()
epoch := lease.Epoch()
defer lease.Close()
```

Lifecycle:

- The first `Manager.Acquire()` during job creation eagerly loads and validates all operator profiles, exactly one stock
  manifest, every entry's required SHA-256 syntax, and the manifest-to-filesystem inventory. It does not read stock body
  bytes. This acquisition occurs in `Collector.Init()`; `Collector.Check()` is a no-op. It starts one configuration epoch
  and returns a lease that owns one reference.
- Every subsequent job created by the same plugin registration receives a lease for that epoch. Each collector stores its
  lease and epoch explicitly; packet-path code never reads a package-global current index.
- Every job cleanup closes its own lease. `Lease.Close()` is idempotent; the final release drops the epoch so GC can
  reclaim it and the next acquisition observes the current on-disk configuration.
- Profile files are not watched or reloaded while jobs hold leases. After changing a profile, restart the Agent or
  recreate all `snmp_traps` jobs so the final release drops the old epoch.
- Agents with no trap jobs never acquire a lease, so they never pay the catalog memory cost.
- Stock profile bodies load lazily through exact trap-OID and metric-rule routes or through a deterministic candidate-file
  set for a MIB-qualified trap name. Candidate hydration is followed by one exact name match. Enabling metrics does not
  parse the complete stock pack.
- Lazy hydration reads and decompresses a file once, verifies the epoch's required SHA-256, and parses the same bytes. The
  digest binds the manifest and body within one epoch; it is not an authenticity signature.
- Failed initial eager loads leave the manager empty and retry on the next acquisition. A failed lazy stock load is
  coalesced and negatively cached per file only for the current epoch.
- The manager mutex covers build/acquire/refcount/release transitions. Lazy body hydration uses per-file `sync.Once`, so
  same-file requests coalesce while unrelated files load independently. A full body bundle is validated before traps,
  metric rules, and charts are published atomically.

### 6. Creation-time Failure Detection

All job resources are validated synchronously in the go.d framework's job `Start()` callback before the job is reported as running:

| Resource | Validation | Failure code |
|---|---|---|
| Job name | `^[a-zA-Z0-9][a-zA-Z0-9_-]*$`, max 64 chars, no path separators/dots | HTTP-422 |
| Endpoint list | At least one endpoint, protocol supported (udp), address/port parseable | HTTP-422 |
| Endpoint bind | `net.ListenUDP()` on every configured endpoint; all-or-nothing cleanup on partial failure | HTTP-503 retryable |
| Eager profile catalog | `Manager.Acquire()` validates operator profiles, one stock manifest, and its complete inventory | HTTP-422 |
| Netdata log directory | `os.Stat("${NETDATA_LOG_DIR}")`; parent must already exist and be a directory | HTTP-503 retryable |
| Journal directory | SDK creates/opens `${NETDATA_LOG_DIR}/traps/{job_name}/`; failure is all-or-nothing cleanup | HTTP-503 retryable |
| Journal writer | `journal.Prepare(dir, cfg, host, opts)` validates directory and retention config | HTTP-503 retryable for environment failures; HTTP-422 for invalid retention config |

These errors must flow through DynCfg as coded errors with the resource-specific code above. Non-retryable configuration
and eager profile-catalog errors use HTTP-422; retryable startup/environment errors use HTTP-503 and implement
`DyncfgRetryable() bool` so file-configured jobs can retry after the transient condition clears. Lazy stock-profile errors
are runtime lookup failures and do not pass through DynCfg.

- `src/go/plugin/agent/jobmgr/dyncfg_collector_callbacks.go` (`collectorCallbacks.Start`) wraps `createCollectorJob()` failures as `codedError{code: 400}`, hiding any inner 422.
- `src/go/plugin/agent/jobmgr/dyncfg_collector_callbacks.go` (`collectorCallbacks.Start`) schedules retry and returns a plain error for `AutoDetection(ctx)` failures.
- `src/go/plugin/agent/jobmgr/dyncfg_collector_callbacks.go` (`collectorCallbacks.Update`) returns plain errors for both `createCollectorJob()` and `AutoDetection(ctx)` failures.
- `src/go/plugin/framework/dyncfg/handler.go:506-509` honors `CodedError` for `CmdEnable`, but `CmdUpdate` callback failures currently send HTTP 200 at `handler.go:683`. Update payload parse/validation failures already return HTTP 400 before the callback path.

**Framework change needed** (small jobmgr + DynCfg handler edit, scoped to SOW-0035 M2):

1. Preserve an inner `CodedError` from `createCollectorJob()` instead of replacing it with hardcoded HTTP 400.
2. If `AutoDetection(ctx)` returns a `CodedError`, call `Cleanup()` and return that error. Schedule a retry only when the error also implements `DyncfgRetryable() bool` and returns `true`; plain non-coded `AutoDetection(ctx)` errors keep the existing retry behavior for other collectors.
3. Make `Update()` mirror `Start()` for both `createCollectorJob()` and `AutoDetection(ctx)` coded errors.
4. Make the `CmdUpdate` callback error path at `src/go/plugin/framework/dyncfg/handler.go:683` honor `CodedError` response codes like `CmdEnable` does, instead of always sending HTTP 200 for `cb.Start()` / `cb.Update()` failure. Preserve the existing `ErrNonDisruptiveUpdate` rollback path at `handler.go:667-677` as HTTP 200 because the old config remains effective and runtime state did not change; trap creation-time failures must not use `ErrNonDisruptiveUpdate`.

Before changing shared DynCfg behavior, M2 must run a same-failure scan (`rg 'CodedError|codedError|MarkNonDisruptiveUpdate' src/go/plugin`) and add handler/jobmgr tests proving existing plain-error retry behavior remains unchanged while coded trap creation failures surface their HTTP status.

The trap plugin must still preflight all resources before it reports successful startup. `AutoDetection(ctx)` should be a
no-op for traps unless a future SOW proves a cheap consistency check is needed; bind, eager profile-catalog, journal, writer,
and retention failures must be creation-time coded errors, not retry-loop events. Lazy stock-profile errors remain first
matching lookup failures.

**Partial resource cleanup**: if endpoint 3 of 5 fails to bind after endpoints 1-2 succeeded, the previously bound endpoints are closed before returning the error. The job never enters the running state.

`createCollectorJob()` failure is not followed by framework `Cleanup()` because no job object is returned. The trap job factory must therefore own rollback for every partial resource acquired during creation: release profile-catalog leases, close bound sockets, close or remove partially-created writer state, and leave the journal directory in a valid empty-or-reusable state before returning the coded error.

On `Update()`, current jobmgr behavior stops the old running job before creating the replacement. If replacement creation fails, the trap job factory can only roll back partial resources from the failed new job; it cannot restore the stopped old job. This is shared framework behavior. M2 tests must capture the resulting failed status and coded response so operators see the apply-time failure clearly.

## Risks and Mitigations

| Risk | Mitigation |
|---|---|
| Go journal writer has format bugs undetected by `journalctl` | M4 end-to-end test replays a pcap through the full pipeline and queries with `journalctl --directory=...`; write tests against the journal file format spec |
| SDK-backed adapter fails creation-time preflight | Use `LogOpenEager` + `LogIdentityStrict` and wrap errors as coded DynCfg job-creation failures |
| Active journal file is not queryable until rotation | Not acceptable for the MVP; validate SDK-backed active files with `journalctl --directory=...` before `Close()` |
| Shared profile catalog lease leak leaves an epoch allocated | Every collector stores and closes its exact lease in `Cleanup()` and on partial initialization failure; test idempotent close, final release, retry, and concurrent acquisition |
| Framework coded-error change breaks other collectors | Preserve existing behavior for plain errors; only `CodedError` suppresses retry and controls HTTP code; add Start and Update tests |
| Direct journal writer cannot sustain target trap volume | Run `go test -benchmem` / throughput benchmarks for queue enqueue and the SDK-backed raw serializer/drain path; if allocation or throughput misses the tens-of-thousands/sec target, reopen batching or backend design |
| SDK dependency API drifts | Pin to `github.com/netdata/systemd-journal-sdk/go v0.8.1`; review API changes before updating |
| SDK chain handling has an upstream defect | Keep `internal/output.Writer` and the internal journal adapter boundary narrow so the SDK can be updated or replaced without changing ingestion semantics |

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
- Shared profile catalog has explicit ownership (plugin-scoped manager + per-collector exact lease)
- Job lifecycle is the well-understood go.d pattern operators and maintainers already know
- `internal/output.Writer` isolates backend concerns; backends remain swappable
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
- go.d codedError: `src/go/plugin/agent/jobmgr/dyncfg_collector_callbacks.go` (`codedError`)
- go.d V2 collector pattern: `src/go/plugin/go.d/collector/ping/collector.go`
- SNMP profile loader multipath pattern: `src/go/plugin/go.d/collector/snmp/ddsnmp/load.go:270-286`
- Design spec: `.agents/skills/collectors-snmp-trap-profiles/netdata.md` §5, §11, §19
