# appsgo.plugin: C collection with the Go runtime

This Linux-only proof of concept demonstrates modernizing `apps.plugin` without rewriting its entire collection engine in Go. It builds a separate `appsgo.plugin` executable using `plugin/agent`, `CollectorV2`, `metrix`, YAML chart templates, dynamic configuration, and a Go process-list Function.

## Build and run

From the repository root, on Linux with the Go version required by `src/go/go.mod` and a C compiler:

```sh
cd src/go
CGO_ENABLED=1 go build -o /tmp/appsgo.plugin ./cmd/appsplugin
/tmp/appsgo.plugin -d -c "$PWD/plugin/apps/config"
```

Run the second command in a terminal for the standalone demo: the standard runtime automatically enables its job when attached to a terminal. With redirected IO it follows the Agent's dynamic-configuration enable protocol. `-d` enables diagnostic logging; it does not bypass that protocol.

The build uses libc and the C math library; it does not require a Netdata C build, libnetdata, or sources outside this plugin for the native backend. The shared Go runtime is reused from the enclosing Go module. The executable is not installed or enabled by production packaging.

Run with the permissions needed for the processes being observed. Ordinary users can inspect their own processes; Linux permissions may restrict another user's IO, file descriptors, command line, or PSS. Unavailable optional observations remain missing. Native fixture tests also run on macOS; the live executable supports Linux only. A build with cgo disabled cannot collect processes.

## Audit and responsibility split

The original plugin's expensive path includes directory scans and parsing, but also state that determines what a sample means. Moving only `read()` into C would leave process-lifetime and rate-accounting decisions spread across both languages.

| Responsibility | Owner | Original sources |
|---|---|---|
| Procfs traversal, parsing, process identity, sample timestamps and counter history | C | `apps_os_linux.c`, `apps_incremental_collection.c`, `apps_pid.c` |
| Exited-child CPU/fault reconciliation | C | `apps_pid.c` |
| FD traversal, cached links, type classification and unique-resource grouping | C | `apps_os_linux.c`, `apps_pid_files.c` |
| Sampled PSS and group-change/exit refresh feedback | C | `apps_os_linux.c`, `apps_aggregations.c` |
| Ordered matching, interpreter naming, manager boundaries and process-tree grouping | Go | `apps_targets.c`, `apps_pid_match.c`, naming portions of `apps_pid.c` |
| Application/user/group sums, min/max, CPU normalization | Go | `apps_aggregations.c`, normalization from `apps_plugin.c` |
| Configuration, scheduling, lifecycle, metric publication, charts and Functions | Go and shared framework | Replaces the original main/config/output/Function layers |

The copied algorithms are adapted into `internal/native`; their provenance is documented there. The POC does not link or include the old collector. It removes Netdata-wide headers, process-global C state, C protocol output, and unrelated integrations from its native dependency closure. Each scanner owns its native state, including buffers, histories and caches, so configuration validation and a running job can coexist.

One collection has two batched native operations:

1. `Scan` reads and reconciles the process population, returning owned process observations to Go.
2. Go assigns application/user/group identities and aggregates ordinary metrics. `Finalize` receives those numeric assignments and returns deduplicated FD counts, records PSS refresh feedback, and cleans retired native state.

The API has O(processes) row/assignment traffic and O(groups) FD summaries. It does not transport O(open descriptors) records into Go or cross cgo once per procfs file. Unique group resources cannot be computed by summing each process's descriptor count. Descriptor identity follows the original readlink-name classification, not a claim that every path is a globally unique kernel file object.

Go owns completed snapshots for concurrent Function requests. A failed collection preserves the preceding snapshot with its original timestamp. Functions never borrow native buffers. Native calls are synchronous: cancellation is checked around collection and cannot interrupt a kernel read already in progress.

## Configuration

Both `config/appsgo.conf` and `config/appsgo/processes.conf` are YAML. The first controls plugin enablement; the second contains the canonical `processes` job. Its JSON schema is served through the existing dynamic-configuration framework.

```yaml
jobs:
  - name: processes
    update_every: 1
    collect_fds: true
    collect_pss: true
    groups:
      - name: postgres
        match:
          - comm: 'glob:postgres*'
      - name: web
        match:
          - cmdline: 'regexp:gunicorn|uvicorn'
```

- `proc_path`: absolute procfs mount; defaults to `/proc`, prefixed by `NETDATA_HOST_PREFIX` when set.
- `collect_fds`: collect descriptors and limits; enabled by default.
- `collect_pss`: sample proportional memory; enabled by default.
- `groups`: ordered rules. The first matching group wins. Match clauses within one group are alternatives; populated fields inside one clause must all match. Expressions use the existing `pkg/matcher` syntax. Matching is against collected command names and rendered command lines, before display-name processing.

Unmatched processes use process-tree groups, with manager boundaries and best-effort interpreter/script naming. User and group views use numeric UID/GID identities. There is one canonical scan job; multiple host scans are not created for each grouping axis. Group names are display metadata; a stable byte encoding identifies each named group independently of native numeric assignment IDs.

## Measurements and Functions

Each application, user and group gets the same metric families:

| Family | Measurements |
|---|---|
| CPU | User, system, guest and reconciled exited-child components; 100% is one core |
| Memory | RSS, virtual, shared and swap bytes; sampled PSS, estimated memory, oldest PSS sample age |
| Faults | Minor/major faults and exited-child components per second |
| IO | Physical/logical bytes per second and read/write calls per second |
| Scheduling | Voluntary/involuntary context switches per second |
| Processes | Process/thread counts and newest/oldest process uptime |
| File descriptors | Unique resources by type and maximum per-process soft-limit utilization |

System charts show process-state counts. Collector charts show reads, readlinks and read errors per scan. CPU/IO/fault/switch rates are already derived in C and published as gauges; Go and the Agent do not differentiate them again. Initial or reset counter baselines have no rate sample. If any member lacks a group measurement, that aggregate remains missing rather than being presented as a complete total.

PSS is sampled, not an instantaneous measurement for every process each interval. Its age is visible; estimated memory scales current RSS by the last measured PSS/RSS ratio. Membership changes and exits trigger refresh feedback. CPU normalization retains the original preference for live-process accounting over exited-child estimates; fault normalization retains the original CPU-based heuristic. If the global CPU observation is unavailable, valid process rates are preserved without normalization.

The `appsgo:processes` Function shows process-instance identity, PID/PPID, command, application, UID/GID, state, CPU, RSS/PSS and sample age, IO, threads and uptime. It supports an application selector and identifies the completed snapshot's time and age. Raw command lines are used for matching but are not returned by this POC Function or used as metric labels.

## Deliberate POC boundary

- Linux only; no FreeBSD, macOS or Windows collection backends.
- No eBPF shared-memory input, cgroup/container enrichment, or NetIPC lookup service.
- No legacy `apps_groups.conf` parser or chart/Function compatibility promise.
- No changes to the original production collector, its packaging, or its alerts.
- No claim of production readiness or performance equivalence before matched-workload measurement.

## Validation

```sh
cd src/go
go test -count=1 ./plugin/apps/... ./cmd/appsplugin
go vet ./plugin/apps/... ./cmd/appsplugin
go test -race -count=1 ./plugin/apps/...
go test -asan -count=1 ./plugin/apps/internal/native  # Linux with a supported compiler
```

Tests cover real synthetic-procfs parsing and lifecycle transitions, missing measurements, PID reuse, exited children, shared FD deduplication, PSS aging, Go matching/grouping, configuration serialization, chart materialization, snapshot publication and Function output. Go race detection validates Go synchronization; native sanitizers validate the C memory path.

Performance evaluation must separate backend scanning from Go grouping, chart materialization and protocol output. Compare equal process populations, enabled features, permissions and sampling intervals, with cold and warm caches and process/FD churn. A grouping-only benchmark is not evidence that the complete hybrid matches the original executable.
