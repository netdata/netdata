# appsgo.plugin: C collection with the Go runtime

This Linux-only proof of concept demonstrates modernizing `apps.plugin` without rewriting its entire collection engine in Go. It builds a separate `appsgo.plugin` executable using `plugin/agent`, `CollectorV2`, `metrix`, YAML chart templates, dynamic configuration, and a Go process-list Function.

## Build and run without installation

From the repository root, on Linux with the Go version required by `src/go/go.mod` and a C compiler:

```sh
cd src/go
CGO_ENABLED=1 go build -o /tmp/appsgo.plugin ./cmd/appsplugin
/tmp/appsgo.plugin -d -c "$PWD/plugin/apps/config"
```

Run the second command in a terminal for the standalone demo: the standard runtime automatically enables its job when attached to a terminal. With redirected IO it follows the Agent's dynamic-configuration enable protocol. `-d` enables diagnostic logging; it does not bypass that protocol.

The build uses libc and the C math library; it does not require a Netdata C build, libnetdata, or sources outside this plugin for the native backend. The shared Go runtime is reused from the enclosing Go module. Source installation is opt-in as described below; distribution packages do not include the POC.

Run with the permissions needed for the processes being observed. Ordinary users can inspect their own processes; Linux permissions may restrict another user's IO, file descriptors, command line, or PSS. Unavailable optional observations remain missing. Native fixture tests also run on macOS; the live executable supports Linux only. A build with cgo disabled cannot collect processes.

## Install from source on Linux

From the repository root, with the normal Netdata source-build dependencies:

```sh
sudo ./netdata-installer.sh --enable-plugin-appsgo
```

This builds and installs Netdata plus `appsgo.plugin`. The installer checks for
the Go version required by `src/go/go.mod` (currently 1.27.0 or newer) and attempts
to provision it if missing. A C compiler is also required. The appsgo build is
independent of `--disable-plugin-go`, which controls `go.d.plugin`.
Use `--dont-start-it` to install without starting/restarting the Agent.
`--disable-plugin-appsgo` disables building it; it does not uninstall an existing
copy. The default is disabled.

For an installation without a prefix, the installed files are:

- `/usr/libexec/netdata/plugins.d/appsgo.plugin`
- `/usr/lib/netdata/conf.d/appsgo.conf`
- `/usr/lib/netdata/conf.d/appsgo/processes.conf`

The installer applies its usual installation prefix to these paths. User
configuration overrides belong in `/etc/netdata/appsgo.conf` and
`/etc/netdata/appsgo/processes.conf` (also under the prefix, when used). For
example, use `sudo /etc/netdata/edit-config appsgo/processes.conf` to customize
groups without changing the stock file.

Netdata normally discovers the installed plugin automatically. If new plugins are
disabled on the test Agent, enable it in `netdata.conf` and restart Netdata:

```ini
[plugins]
    appsgo = yes
```

The stock YAML enables the canonical `processes` job. The original `apps.plugin`
can run alongside it; to collect only with the POC, set `apps = no` in the same
section. Charts use the POC's distinct identities and the process-list Function
is `appsgo:processes`.

A root source installation grants `cap_dac_read_search,cap_sys_ptrace` when
supported on the host, with ownership `root:netdata` and mode `0750`. If
capabilities are unavailable (including the installer's container path), the
installer warns and leaves it executable with ordinary Netdata-user visibility.
There is no setuid fallback. A non-root install also has only that user's access.
For a terminal diagnostic after installation:

```sh
sudo -u netdata /usr/libexec/netdata/plugins.d/appsgo.plugin -d
```

### Direct CMake build or staged install

For an existing Netdata installation, the equivalent CMake option is
`ENABLE_PLUGIN_APPSGO`, default `OFF`:

```sh
cmake -S . -B build -DCMAKE_INSTALL_PREFIX=/ -DENABLE_PLUGIN_APPSGO=ON
cmake --build build --target appsgo-plugin -j2
sudo cmake --install build --component plugin-appsgo
```

The component installs only this executable and its stock YAML. Use the prefix
matching the test Agent; configuration paths are embedded at build time. For a
staged install, replace the last command with
`DESTDIR=/tmp/appsgo-stage cmake --install build --component plugin-appsgo`.
CMake installation does not grant capabilities. To give a direct host install
the same access as the source installer, with an existing `netdata` group:

```sh
sudo chown root:netdata /usr/libexec/netdata/plugins.d/appsgo.plugin
sudo chmod 0750 /usr/libexec/netdata/plugins.d/appsgo.plugin
sudo setcap cap_dac_read_search,cap_sys_ptrace+ep /usr/libexec/netdata/plugins.d/appsgo.plugin
```

The target uses `CGO_ENABLED=1` and CMake's selected C compiler. Existing pure-Go
targets retain `CGO_ENABLED=0`. Native sources/headers and embedded charts/schema
are build dependencies. The POC currently supports native Linux builds only;
CMake rejects other platforms and cross-compilation when this option is enabled.

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
- `groups`: ordered rules. The first matching group wins. Match clauses within one group are alternatives; populated fields inside one clause must all match. Expressions use the existing `pkg/matcher` syntax. Matching is against collected command names and space-rendered command lines, before display-name processing. Native argv boundaries remain intact for interpreter/script naming, including paths containing spaces.

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

System charts show state counts for readable processes; processes whose required identity files are inaccessible cannot be attributed or counted. Collector charts show file-read attempts, readlinks and read errors per scan; these diagnostics are not syscall counters. CPU/IO/fault/switch rates are already derived in C and published as gauges; Go and the Agent do not differentiate them again. Initial or reset counter baselines have no rate sample. If any member lacks a group measurement, that aggregate remains missing rather than being presented as a complete total.

PSS requires readable `smaps_rollup` files; this POC has no full-`smaps` fallback. PSS is sampled, not an instantaneous measurement for every process each interval. Its age is visible; estimated memory scales current RSS by the last measured PSS/RSS ratio. Membership changes and exits trigger refresh feedback. CPU normalization retains the original preference for live-process accounting over exited-child estimates; fault normalization retains the original CPU-based heuristic. If the global CPU observation is unavailable, valid process rates are preserved without normalization.

The `appsgo:processes` Function shows process-instance identity, PID/PPID, command, application, UID/GID, state, CPU, RSS/PSS and sample age, IO, threads and uptime. It supports an application selector and identifies the completed snapshot's time and age. Raw command lines are used for matching but are not returned by this POC Function or used as metric labels.

## Deliberate POC boundary

- Linux only; no FreeBSD, macOS or Windows collection backends.
- No eBPF shared-memory input, cgroup/container enrichment, or NetIPC lookup service.
- No legacy `apps_groups.conf` parser or chart/Function compatibility promise.
- No changes to the original production collector or its alerts; source installation of the POC is an explicit opt-in.
- No claim of production readiness or performance equivalence; the measurements below show additional runtime cost.

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

## Performance evidence

The C boundary avoids per-file cgo calls and retains FD caching, but the complete
POC currently costs more than the original executable. These development-machine
measurements are evidence of that tradeoff, not production thresholds.

The live comparison uses Linux/aarch64 in Docker, 200 idle worker processes with
20 additional `/dev/zero` descriptors each, root permissions, a one-second
interval, and FD/PSS/child/guest/user/group collection enabled. Both executables
observe the same workers sequentially; each run has six seconds of warmup and
12 seconds of measurement. Three runs alternate the original and hybrid order.
CPU comes from `/proc/PID/stat`, RSS from `/proc/PID/status`, and read/write-call
counts from `/proc/PID/io`. Output is continuously drained.

At commit `ad573aa70a` on 2026-09-25 (median of the three runs):

| Measurement | Original C | Hybrid POC |
|---|---:|---:|
| CPU time, ms/s | 15.8 | 28.2 |
| Resident memory, MiB | 8.3 | 30.1 |
| Read calls/s | 1652.4 | 2502.5 |
| Write calls/s | 2.0 | 4.2 |
| Protocol output, KiB/s | 5.3 | 22.7 |
| Reported collection duration, ms | unavailable | 23.5 |

The original is `apps.plugin v2.11.0-305-nightly` from image
`netdata/netdata@sha256:2dd6963cb15637748985871016af3c52d1a0cc67ea03a5b7fcfe51600e481e9f`;
it is not built from this POC's base revision. Original stock grouping and the POC
example grouping differ, as do chart count and protocol volume. PSS scheduling
also differs. Consequently this compares complete demo executables, not an
isolated C-versus-Go language cost. Host-global CPU observations include the VM;
this is not a cgroup isolation test. All timing is subject to development VM noise.

The original was invoked with:

```sh
apps.plugin 1 with-files with-childs with-guest with-detailed-uptime --pss 10
```

The hybrid used the example YAML configuration and the documented headless job
enablement. Original C allocation counts and collection wall time are not exposed
by this installed baseline; no equivalent numbers are inferred from CPU use.

Reproduce the checked-in native and full-pipeline fixture benchmarks with:

```sh
cd src/go
go test -run '^$' -bench 'Benchmark(Pipeline|ScanWarm)' -benchtime=1s -count=6 -benchmem \
  ./plugin/apps/collector/processes ./plugin/apps/internal/native
```

The same revision, using Go 1.27.1 on Linux/aarch64, produced these ranges over
six benchmark runs:

| Fixture path | Time per cycle | Go bytes per cycle | Go allocations per cycle |
|---|---:|---:|---:|
| Native scan, snapshot copy and finalization | 3.91–4.20 ms | 92,632–92,633 | 605 |
| Full collector/chart pipeline | 4.30–4.40 ms | 347,489–349,504 | 2,423–2,424 |

Both fixtures use ordinary cached files with static counters, not live procfs.
The native fixture uses a controlled clock; the full pipeline uses real time,
including periodic cache refresh. Their timings cannot be subtracted to isolate
framework overhead or compared directly with the live collection duration above.
Process/FD churn is covered by correctness fixtures, not a separate throughput
measurement.

The full-pipeline benchmark includes real native reads, grouping, metric commit,
chart planning and protocol emission to a reusable memory buffer. Its 200
processes share 20 distinct FD targets and produce 44 charts. It excludes the
scheduler and actual stdout transport. Native and full-pipeline `B/op` and
`allocs/op` count only Go allocations, not C/libc allocations. Live RSS includes
both runtimes. A profile identifies command parsing/copies, metric label writes
and committed-series cloning as allocation contributors. The original has no
corresponding Go benchmark at the base revision.

The source also explains two intentional acquisition costs: command lines are
read each cycle so exec changes can affect grouping, and a second stat read
checks PID incarnation before publishing a row. These contribute to the hybrid's
higher read-call count. Production tuning should measure these costs separately
from the Go framework and retain the identity and missing-data guarantees.
