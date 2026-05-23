# AE_FIELDS reference

Verified field map for the agent-events journal namespace.
Every claim here is traceable to producer source at
`<repo>/src/daemon/status-file.c` (the schema is
`STATUS_FILE_VERSION = 28`, `src/daemon/status-file.h:14`).

The .local draft `agent-events-journals.md` was found to have
14 high-severity divergences (wrong enums, missing fields,
misattributed semantics). This file supersedes it.

## How journal field names are formed

Producer JSON path -> journal field name:

1. Producer emits a JSON path (e.g. `agent.profile[0]`).
2. log2journal applies `--prefix 'AE_'` (literal prepend; the
   prefix is NOT transliterated --
   `src/collectors/log2journal/log2journal-help.c:108-109`).
3. log2journal walks the JSON tree. For nested objects, it
   joins parent + child with `_`. For arrays, it appends
   `_<index>` (`log2journal-json.c:477-511`).
4. Per-character transliteration applies a 256-entry map
   (`log2journal.c:8-61`): lowercase -> uppercase, digits and
   uppercase pass through, everything else (including `.`,
   `-`, `:`, `/`, `@`, `_`, `[`, `]`) maps to `_`. Consecutive
   underscores are collapsed
   (`log2journal-json.c:395-396`).

So:
- `agent.id` -> `AE_AGENT_ID`
- `agent.profile[0]` -> `AE_AGENT_PROFILE_0`
- `host.boot.id` -> `AE_HOST_BOOT_ID`
- `@timestamp` -> `AE__TIMESTAMP` (note: double underscore
  because `@` -> `_`)
- `fatal.errno` -> `AE_FATAL_ERRNO` (NOT from a top-level
  `fatal_errno`; the path is nested)

## Top-level POST-time fields (always present)

Added by `post_status_file()` at `status-file.c:967-976`. These
sit at the JSON top level, so they have NO nested-path
prefix in the journal field name.

| JSON path | Journal field | Type | Always? | Triage meaning |
|---|---|---|---|---|
| `exit_cause` | `AE_EXIT_CAUSE` | string | yes | Human-readable label for why this session ended. The first thing to look at. See enum below. |
| `message` | `AE_MESSAGE` | string | yes | One-line summary; subject of FTS search. |
| `priority` | `AE_PRIORITY` | int | yes | Syslog priority (lower = more severe). |
| `version_saved` | `AE_VERSION_SAVED` | uint | yes | The status file's own version (typically 28). |
| `agent_version_now` | `AE_AGENT_VERSION_NOW` | string | yes | Posting agent's version (the agent that did the POST = next session, NOT the one that crashed). |
| `agent_pid_now` | `AE_AGENT_PID_NOW` | uint | yes | PID of the agent that POSTed. |
| `host_memory_critical` | `AE_HOST_MEMORY_CRITICAL` | bool | yes | Was the host under memory pressure at POST time? |
| `host_memory_free_percent` | `AE_HOST_MEMORY_FREE_PERCENT` | uint | yes | % free RAM at POST time. |
| `agent_health` | `AE_AGENT_HEALTH` | string | yes | Health classification across restart history. See enum below. |
| `@timestamp` | `AE__TIMESTAMP` | RFC3339 | yes | When the captured session ended. |
| `version` | `AE_VERSION` | uint | yes | The schema version (`STATUS_FILE_VERSION`). Use to slice by schema. |

These fields are **always present** on every record and are
the safest scoping anchors. `AE_EXIT_CAUSE`, `AE_AGENT_HEALTH`,
`AE_VERSION` are all good index-friendly facets.

## `agent.*` fields (`dsf_json_agent`, `status-file.c:99-156`)

The previous (crashed) session's agent state.

| JSON path | Journal field | Type | Version-gating | Triage meaning |
|---|---|---|---|---|
| `agent.id` | `AE_AGENT_ID` | UUID | always | Netdata machine GUID (per-install, persistent). The "agent identity". DIFFERENT from `host.id`. |
| `agent.since` | `AE_AGENT_SINCE` | RFC3339 | v>=24 | When this install was first registered. |
| `agent.ephemeral_id` | `AE_AGENT_EPHEMERAL_ID` | UUID | always | Unique per Netdata invocation (changes on every restart). Use to group multiple events from the same crashed session. |
| `agent.version` | `AE_AGENT_VERSION` | string | always | The version of the *crashed* session. **Slice on this for regression-spotting.** |
| `agent.uptime` | `AE_AGENT_UPTIME` | int seconds | always | Duration the crashed session ran. Short uptime + crash = startup bug. |
| `agent.node_id` | `AE_AGENT_NODE_ID` | UUID | always | Cloud node UUID (empty when agent isn't claimed). |
| `agent.claim_id` | `AE_AGENT_CLAIM_ID` | UUID | always | Cloud claim UUID (empty when not claimed). Presence -> cloud-connected agent. |
| `agent.restarts` | `AE_AGENT_RESTARTS` | uint | always | Total restart count for this install. High value + recent crash = agent loop. |
| `agent.crashes` | `AE_AGENT_CRASHES` | uint | v>=24 | Total crash count. |
| `agent.pid` | `AE_AGENT_PID` | uint | v>=27 | PID of the crashed session. |
| `agent.posts` | `AE_AGENT_POSTS` | uint | v>=22 | Total POSTs from this install. |
| `agent.aclk` | `AE_AGENT_ACLK` | enum | v>=22 | Cloud connection state. See enum. |
| `agent.profile[N]` | `AE_AGENT_PROFILE_0..N` | enum array | always | Bitmap rendered as array. Slice by parent vs child vs iot. See enum. |
| `agent.status` | `AE_AGENT_STATUS` | enum | always | DAEMON_STATUS at the time of capture. See enum. |
| `agent.exit_reason[N]` | `AE_AGENT_EXIT_REASON_0..N` | enum array | always | EXIT_REASON bitmap rendered as array. Empty array = no specific reason. See enum. |
| `agent.install_type` | `AE_AGENT_INSTALL_TYPE` | string | always | `kickstart`, `binpkg`, `static`, etc. **Slice on this for "is this a packaging issue?"** |
| `agent.db_mode` | `AE_AGENT_DB_MODE` | string | v>=14 | dbengine memory mode. |
| `agent.db_tiers` | `AE_AGENT_DB_TIERS` | uint | v>=14 | Number of dbengine tiers. |
| `agent.kubernetes` | `AE_AGENT_KUBERNETES` | bool | v>=14 | Kubernetes deployment? Slice on this for k8s-specific issues. |
| `agent.sentry_available` | `AE_AGENT_SENTRY_AVAILABLE` | bool | v>=16 | Is Sentry enabled? |
| `agent.reliability` | `AE_AGENT_RELIABILITY` | int | always | Signed reliability counter (positive = healthy run streak; negative = crash streak). `<= -2` -> `crash-loop`. |
| `agent.stack_traces` | `AE_AGENT_STACK_TRACES` | string | always | Backtrace backend name (`libbacktrace`, `none`). |
| `agent.timings.init` | `AE_AGENT_TIMINGS_INIT` | int seconds | always | How long startup took. Long init + crash = startup bug. |
| `agent.timings.exit` | `AE_AGENT_TIMINGS_EXIT` | int seconds | always | How long shutdown took. |

## `metrics.*` fields (`dsf_json_metrics`, `:158-193`)

Snapshot of the database at the time of capture. Useful for
"big-database crashes" investigations.

| JSON path | Journal field |
|---|---|
| `metrics.nodes.total` | `AE_METRICS_NODES_TOTAL` |
| `metrics.nodes.receiving` | `AE_METRICS_NODES_RECEIVING` |
| `metrics.nodes.sending` | `AE_METRICS_NODES_SENDING` |
| `metrics.nodes.archived` | `AE_METRICS_NODES_ARCHIVED` |
| `metrics.metrics.collected` | `AE_METRICS_METRICS_COLLECTED` |
| `metrics.metrics.available` | `AE_METRICS_METRICS_AVAILABLE` |
| `metrics.instances.collected` | `AE_METRICS_INSTANCES_COLLECTED` |
| `metrics.instances.available` | `AE_METRICS_INSTANCES_AVAILABLE` |
| `metrics.contexts.collected` | `AE_METRICS_CONTEXTS_COLLECTED` |
| `metrics.contexts.available` | `AE_METRICS_CONTEXTS_AVAILABLE` |

## `host.*` fields (`dsf_json_host`, `:195-253`)

Host-level info, mostly stable across crashes on the same
host.

| JSON path | Journal field | Triage meaning |
|---|---|---|
| `host.id` | `AE_HOST_ID` | OS-level `/etc/machine-id`. **Different from `AE_AGENT_ID`** (Netdata's own identifier). Use to spot multiple agents on the same host. |
| `host.architecture` | `AE_HOST_ARCHITECTURE` | `x86_64`, `aarch64`, `armv7l`, ... **Slice for arch-specific bugs.** |
| `host.virtualization` | `AE_HOST_VIRTUALIZATION` | `none`, `kvm`, `vmware`, `lxc`, `docker`, ... |
| `host.container` | `AE_HOST_CONTAINER` | `none`, `docker`, `kubernetes`, ... |
| `host.uptime` | `AE_HOST_UPTIME` | **MISLEADING NAME.** Stores boottime EPOCH (`status-file.c:202` writes `ds->boottime` from `now_boottime_sec()`). NOT a duration. Compute uptime via `now - AE_HOST_UPTIME`. |
| `host.timezone` | `AE_HOST_TIMEZONE` | string, v>=20 |
| `host.cloud_provider` | `AE_HOST_CLOUD_PROVIDER` | `aws`, `gcp`, `azure`, ..., v>=20 |
| `host.cloud_instance` | `AE_HOST_CLOUD_INSTANCE` | EC2 instance type etc., v>=20 |
| `host.cloud_region` | `AE_HOST_CLOUD_REGION` | v>=20 |
| `host.system_cpus` | `AE_HOST_SYSTEM_CPUS` | uint. **Slice for "low-cpu environment" bugs.** |
| `host.boot.id` | `AE_HOST_BOOT_ID` | UUID, changes on every host boot. |
| `host.memory.total` | `AE_HOST_MEMORY_TOTAL` | bytes, only when `OS_SYSTEM_MEMORY_OK` |
| `host.memory.free` | `AE_HOST_MEMORY_FREE` | bytes |
| `host.memory.netdata` | `AE_HOST_MEMORY_NETDATA` | bytes used by netdata, v>=21 |
| `host.memory.oom_protection` | `AE_HOST_MEMORY_OOM_PROTECTION` | uint, v>=21 |
| `host.disk.db.total` | `AE_HOST_DISK_DB_TOTAL` | bytes available to dbengine |
| `host.disk.db.free` | `AE_HOST_DISK_DB_FREE` | bytes free |
| `host.disk.db.inodes_total` | `AE_HOST_DISK_DB_INODES_TOTAL` | uint |
| `host.disk.db.inodes_free` | `AE_HOST_DISK_DB_INODES_FREE` | uint |
| `host.disk.db.read_only` | `AE_HOST_DISK_DB_READ_ONLY` | bool. True + crash -> "disk read-only" cause. |
| `host.disk.netdata.dbengine` | `AE_HOST_DISK_NETDATA_DBENGINE` | bytes used by dbengine files. |
| `host.disk.netdata.sqlite` | `AE_HOST_DISK_NETDATA_SQLITE` | bytes used by SQLite files. |
| `host.disk.netdata.other` | `AE_HOST_DISK_NETDATA_OTHER` | bytes used by other files. |
| `host.disk.netdata.last_updated` | `AE_HOST_DISK_NETDATA_LAST_UPDATED` | RFC3339. |

## `os.*` fields (`dsf_json_os`, `:255-266`)

| JSON path | Journal field | Triage meaning |
|---|---|---|
| `os.type` | `AE_OS_TYPE` | enum: `unknown`, `linux`, `freebsd`, `macos`, `windows`. |
| `os.kernel` | `AE_OS_KERNEL` | Kernel version string. |
| `os.name` | `AE_OS_NAME` | Distro name (e.g. `Ubuntu`, `CentOS Stream`). |
| `os.version` | `AE_OS_VERSION` | Distro version. |
| `os.family` | `AE_OS_FAMILY` | `os_id` (e.g. `ubuntu`). **Slice for distro-specific issues.** |
| `os.platform` | `AE_OS_PLATFORM` | `os_id_like` (parent distro family, e.g. `debian`). NOT a rewrite of `AE_OS_FAMILY` -- they're independent producer fields. |

## `hw.*` fields (`dsf_json_hw`, `:268-319`)

DMI / SMBIOS data. Useful for hardware-specific bug
investigation. Privacy-sensitive serials and asset_tags are
**commented out at the producer side** (`status-file.c:275-276,
:294-295, :304-305`) and never reach the journal.

| JSON path | Journal field | Notes |
|---|---|---|
| `hw.sys.vendor` | `AE_HW_SYS_VENDOR` | BIOS / system vendor. |
| `hw.sys.uuid` | `AE_HW_SYS_UUID` | System UUID. |
| `hw.product.name` | `AE_HW_PRODUCT_NAME` | Product name (e.g. `MacBookPro18,3`). |
| `hw.product.version` | `AE_HW_PRODUCT_VERSION` | |
| `hw.product.sku` | `AE_HW_PRODUCT_SKU` | |
| `hw.product.family` | `AE_HW_PRODUCT_FAMILY` | |
| `hw.board.name` | `AE_HW_BOARD_NAME` | |
| `hw.board.version` | `AE_HW_BOARD_VERSION` | |
| `hw.board.vendor` | `AE_HW_BOARD_VENDOR` | |
| `hw.chassis.type` | `AE_HW_CHASSIS_TYPE` | Numeric (e.g. `6` = desktop, `9` = laptop). |
| `hw.chassis.vendor` | `AE_HW_CHASSIS_VENDOR` | |
| `hw.chassis.version` | `AE_HW_CHASSIS_VERSION` | |
| `hw.bios.date` | `AE_HW_BIOS_DATE` | |
| `hw.bios.release` | `AE_HW_BIOS_RELEASE` | |
| `hw.bios.version` | `AE_HW_BIOS_VERSION` | |
| `hw.bios.vendor` | `AE_HW_BIOS_VENDOR` | |

## `product.*` fields (`dsf_json_product`, `:321-329`)

| JSON path | Journal field |
|---|---|
| `product.vendor` | `AE_PRODUCT_VENDOR` |
| `product.name` | `AE_PRODUCT_NAME` |
| `product.type` | `AE_PRODUCT_TYPE` |

## `fatal.*` fields (`dsf_json_fatal`, `:331-367`)

Present on crashes and deliberate fatal conditions. Empty on
graceful exits.

| JSON path | Journal field | Version-gating | Triage meaning |
|---|---|---|---|
| `fatal.line` | `AE_FATAL_LINE` | always | Source line of the panic. Combine with FILENAME and FUNCTION for de-dup. |
| `fatal.filename` | `AE_FATAL_FILENAME` | always | Source file. **Slice on this for "this file is buggy".** |
| `fatal.function` | `AE_FATAL_FUNCTION` | always | Function name (with demangled symbol). **Slice on this for "this function is buggy".** |
| `fatal.message` | `AE_FATAL_MESSAGE` | always | Panic message. Subject of FTS. |
| `fatal.errno` | `AE_FATAL_ERRNO` | always | errno string at panic. |
| `fatal.thread` | `AE_FATAL_THREAD` | always | Worker thread name (e.g. `CTXLOAD`, `STREAM:63`). |
| `fatal.thread_id` | `AE_FATAL_THREAD_ID` | always | POSIX TID. |
| `fatal.stack_trace` | `AE_FATAL_STACK_TRACE` | always | Backtrace. Real addresses preserved (anonymization is dedup-only, `status-file-dedup.c:26-36`). |
| `fatal.signal_code` | `AE_FATAL_SIGNAL_CODE` | v>=16 | `SIGNAL/SI_CODE` formatted (e.g. `SIGSEGV/SEGV_MAPERR`). Empty -> not a signal crash. **Primary signal-crash predicate.** See enum. |
| `fatal.sentry` | `AE_FATAL_SENTRY` | v>=17 | Was a Sentry submission attempted? |
| `fatal.fault_address` | `AE_FATAL_FAULT_ADDRESS` | v>=18 | Hex address of the fault. Empty when `signal_code == 0`. |
| `fatal.worker_job_id` | `AE_FATAL_WORKER_JOB_ID` | v>=23 | Worker job ID at panic. |

## Enum reference

### `AE_AGENT_STATUS` (DAEMON_STATUS)

Source: `src/daemon/status-file.c:23-33`.

| Value | Meaning for triage |
|---|---|
| `none` | No prior status (very first session). |
| `initializing` | Crashed during startup -> startup bug. Combine with `agent.timings.init` for context. |
| `running` | Crashed during normal operation -> the most "interesting" class. |
| `exiting` | Crashed during shutdown -> shutdown-path bug. |
| `exited` | Graceful exit (no crash). |

### `AE_AGENT_ACLK` (CLOUD_STATUS)

Source: `src/claim/cloud-status.c:5-15`.

| Value | Meaning for triage |
|---|---|
| `available` | Default; not yet attempted. |
| `online` | Connected to Cloud (ACLK up). |
| `indirect` | Connected via parent. |
| `banned` | Cloud rejected (claim issue). |
| `offline` | Disconnected (network or shutdown). |

(The .local draft listed a `disabled` value -- it does NOT
exist in the producer source.)

### `AE_AGENT_HEALTH`

Source: `src/daemon/status-file.c:929-952`. Computed by the
**agent** (not the ingestion server) at POST time across
restart history. Used to isolate crash classes.

| Value | Meaning for triage |
|---|---|
| `healthy-first` | First run, no prior crashes. Boring (filter out). |
| `healthy-repeated` | Multiple healthy runs in a row. |
| `healthy-loop` | Reliability >= 2 consecutive healthy runs. |
| `healthy-recovered` | Was unhealthy, now healthy. |
| `crash-first` | First crash ever on this install. Interesting -- new bug? |
| `crash-entered` | Single crash, then recovered. |
| `crash-loop` | Reliability <= -2 (repeated crashes). **Highest-priority class.** |
| `crash-repeated` | Two or more crashes. |

To find ALL crashes: `(AE_AGENT_HEALTH in crash-first, crash-loop, crash-repeated, crash-entered)`.

### `AE_AGENT_PROFILE_*` (ND_PROFILE bitmap)

Source: `src/daemon/config/netdata-conf-profile.c:7-15`.

| Value | Meaning |
|---|---|
| `standalone` | Single-node deployment. |
| `parent` | Streaming parent. **Slice for "parent-only" bugs.** |
| `child` | Streaming child. **Slice for "child-only" bugs.** |
| `iot` | IoT / lightweight profile. |

(The .local draft listed `dopple` and `store-child` -- they do
NOT exist; `iot` was missing.)

### `AE_AGENT_EXIT_REASON_*` (EXIT_REASON bitmap)

Source: `src/libnetdata/exit/exit_initiated.c:7-38`. The
EXIT_REASON bitmap renders as a JSON array. Empty bitmap ->
empty array (no `none` element).

20 distinct strings:

| Value | Meaning |
|---|---|
| `signal-segmentation-fault` | SIGSEGV received. |
| `signal-bus-error` | SIGBUS received. |
| `signal-floating-point-exception` | SIGFPE received. |
| `signal-illegal-instruction` | SIGILL received. |
| `signal-abort` | SIGABRT received (assertion / abort()). |
| `signal-bad-system-call` | SIGSYS received. |
| `signal-cpu-time-limit-exceeded` | SIGXCPU received. |
| `signal-file-size-limit-exceeded` | SIGXFSZ received. |
| `signal-quit` | SIGQUIT received. |
| `signal-terminate` | SIGTERM received (graceful kill). |
| `signal-interrupt` | SIGINT received (Ctrl-C). |
| `out-of-memory` | OOM panic. |
| `already-running` | Another instance held the listen socket. |
| `fatal` | Generic fatal() call. |
| `api-quit` | API endpoint requested exit. |
| `cmd-exit` | Explicit `netdata --exit` invocation. |
| `service-stop` | Service manager (systemd) sent stop. |
| `system-shutdown` | Host shutting down. |
| `update` | Replaced by a new version. |
| `shutdown-timeout` | Shutdown took too long. |

(The .local draft was significantly wrong here -- listed
~10 invented values like `exit-called`, `exit-and-update`,
`cannot-allocate`, `oom`, `assertion-failed`, none of which
exist in producer source.)

### `AE_EXIT_CAUSE` (top-level)

Source: `src/daemon/status-file.c:1097-1286`. Computed by the
**agent**, NOT the ingestion server. The most useful field for
classifying records.

26 distinct strings:

**Initial / no prior state (1):**

| Value | Meaning |
|---|---|
| `no last status` | First-ever start; no prior status file readable. |

**Prior was EXITED (graceful) (7):**

| Value | Meaning |
|---|---|
| `exit no reason` | Prior exited cleanly with no reason recorded. |
| `deadly signal and exit` | Got a deadly signal but exited normally. |
| `fatal and exit` | Hit a fatal but managed to exit. |
| `exit on system shutdown` | Host shutting down; agent stopped gracefully. |
| `exit to update` | Stopped to allow an update. |
| `exit and updated` | Stopped and was replaced by a new version. |
| `exit instructed` | `netdata --exit` or service stop. |

**Prior was INITIALIZING (8):**

| Value | Meaning |
|---|---|
| `abnormal power off` | Power loss during startup. |
| `deadly signal on start` | Signal during startup. |
| `out of memory` | OOM during startup. (.local draft says `cannot allocate` -- wrong.) |
| `already running` | Listen socket conflict at init. |
| `disk read-only` | Filesystem read-only at init. |
| `disk full` | Disk full at init. |
| `disk almost full` | Disk near capacity at init. |
| `fatal on start` | fatal() during startup. |
| `killed hard on start` | SIGKILL/SIGTERM during startup. |

**Prior was EXITING (5):**

| Value | Meaning |
|---|---|
| `deadly signal on exit` | Signal during shutdown. |
| `exit timeout` | Shutdown didn't complete in time. |
| `fatal on exit` | fatal() during shutdown. |
| `killed hard on shutdown` | SIGKILL during shutdown (host shutdown). |
| `killed hard on update` | SIGKILL during hot update. |
| `killed hard on exit` | SIGKILL during exit. |

**Prior was RUNNING (6):**

| Value | Meaning |
|---|---|
| `abnormal power off` | Power loss during normal operation. |
| `out of memory` | OOM during normal operation. |
| `deadly signal` | Signal received during normal operation. |
| `killed fatal` | SIGKILL after a fatal. |
| `killed hard low ram` | OOM-killed (RAM pressure). |
| `killed hard` | SIGKILL/SIGTERM from outside. |

### `AE_OS_TYPE` (DAEMON_OS_TYPE)

Source: `src/daemon/status-file.c:35-45`.

`unknown`, `linux`, `freebsd`, `macos`, `windows`.

### `AE_FATAL_SIGNAL_CODE`

Format: `SIGNAL/SI_CODE` (e.g. `SIGSEGV/SEGV_MAPERR`). Sources:
`src/libnetdata/signals/signal-code.c:12-53` (signal name map),
`:97-184` (per-signal SI_CODE map).

Most relevant for crash triage:

| Value | Meaning |
|---|---|
| `SIGSEGV/SEGV_MAPERR` | Invalid memory map (NULL pointer, freed memory). |
| `SIGSEGV/SEGV_ACCERR` | Access violation (write to read-only page). |
| `SIGSEGV/SEGV_BNDERR` | Address bound check fault. |
| `SIGSEGV/SEGV_PKUERR` | Protection key fault. |
| `SIGBUS/BUS_ADRALN` | Alignment error. |
| `SIGBUS/BUS_ADRERR` | Non-existent physical address. |
| `SIGBUS/BUS_OBJERR` | Object-specific bus error. |
| `SIGFPE/FPE_INTDIV` | Integer divide by zero. |
| `SIGFPE/FPE_INTOVF` | Integer overflow. |
| `SIGFPE/FPE_FLTDIV` | Float divide by zero. |
| `SIGABRT/SI_TKILL` | abort() / assertion failure (typical SI_CODE for abort()). |
| `SIGTRAP/TRAP_BRKPT` | Breakpoint trap. |
| `SIGTRAP/TRAP_TRACE` | Trace trap. |

Empty `AE_FATAL_SIGNAL_CODE` -> not a signal crash (it's a
deliberate fatal or a graceful exit).

(The .local draft had `SIGABRT/ABRT` which is wrong: `ABRT`
is not a valid SI_CODE token. And `SIGTRAP/TRAP Trace` should
be `SIGTRAP/TRAP_TRACE`. And `SIGVTALRM/VTALRM` does not
exist as a per-signal SI_CODE.)

## Index-friendly facets (high-value)

These fields are the **first-pass slicers** for queries.
Always include at least 1-2 of these in `selections` before
falling back to FTS:

- `AE_AGENT_VERSION` -- regression / fix-detection.
- `AE_AGENT_HEALTH` -- crash class.
- `AE_EXIT_CAUSE` -- exit class.
- `AE_FATAL_SIGNAL_CODE` -- signal type.
- `AE_FATAL_FUNCTION` -- localize to a function.
- `AE_FATAL_FILENAME` -- localize to a file.
- `AE_HOST_ARCHITECTURE` -- arch-specific bugs.
- `AE_OS_FAMILY` -- distro-specific bugs.
- `AE_AGENT_PROFILE_0` (and `_1`, `_2`) -- parent / child / iot.
- `AE_AGENT_KUBERNETES` -- k8s-specific.
- `AE_AGENT_INSTALL_TYPE` -- packaging-specific.

## Privacy-sensitive fields

Treat these as identifying. The `redact-events.sh` opt-in
filter masks them when sharing:

- `AE_AGENT_ID` (machine GUID).
- `AE_HOST_ID` (OS machine-id).
- `AE_AGENT_NODE_ID`, `AE_AGENT_CLAIM_ID` (Cloud identifiers).
- `AE_HOST_BOOT_ID`, `AE_AGENT_EPHEMERAL_ID`.
- `AE_HW_SYS_UUID`.
- DMI fields (`AE_HW_*`) when correlated with serial-equivalent
  identifiers.

(Privacy-sensitive serials and asset_tags are already
commented out at the producer side and never reach the
journal -- `status-file.c:275-276, :294-295, :304-305`.)

## What is NOT in the journal

Several producer fields are intentionally redacted at the
producer side (commented out in `dsf_json_hw`):

- `hw.sys.serial`, `hw.sys.asset_tag`
- `hw.board.serial`, `hw.board.asset_tag`
- `hw.chassis.serial`, `hw.chassis.asset_tag`

There is no `agent.happiness` field in the producer source at
any version. The `.local` draft mentioned it -- the field has
never existed.

## Stack trace addresses are NOT anonymized in the journal

`status-file-dedup.c:26-36` zeroes out hex addresses ONLY when
computing the dedup hash. The journal-emitted
`AE_FATAL_STACK_TRACE` retains real addresses. Useful for
bug investigation; sensitive when sharing externally.
