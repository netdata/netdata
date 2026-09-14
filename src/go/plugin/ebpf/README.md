# eBPF shared-framework POC

This experimental executable runs one cachestat collector through the shared Go Agent, CollectorV2, `metrix`,
`charttpl`/`chartengine`, and single-instance DynCfg. It reuses the existing C/libbpf backend. The old Go and C eBPF
plugins remain available alongside it.

The live POC passed on Docker Desktop's Linux/arm64 VM on 2026-09-14. This is evidence that the architecture works
for this collector, not a production migration or a claim of parity across all kernels and collectors.

## Scope

- Separate `ebpf-poc.plugin` executable, with code in `src/go/cmd/ebpfpocplugin` and `src/go/plugin/ebpf`.
- One canonical `cachestat` job per process; DynCfg ID `ebpf-poc:collector:cachestat`.
- Four cumulative kernel-function event counters, rendered as rates on one chart, context
  `ebpf_poc.page_cache_events`.
- File configuration and the existing DynCfg schema, test, enable, update, get and disable operations.
- CO-RE base object, per-CPU global counters, no per-PID collection.
- No Functions, shared-memory publication, apps/cgroups integration, old chart identities, derived hit ratio,
  legacy-object fallback, native/static packages or complete Netdata Agent build validation.

All POC chart and config identities are experimental. An independent plugin executable retains a separate native
dependency and privilege boundary while using the same Go framework as the other plugins.

## Install from source on Linux

On this POC branch, run the usual source installer from the repository root:

```sh
sudo ./netdata-installer.sh
```

No POC-specific build flag or separate Go build is needed. On 64-bit Linux, with eBPF enabled and Go 1.27 or newer, CMake
builds `ebpf-poc.plugin` alongside the existing eBPF plugins. It uses the bundled libbpf and CO-RE skeletons and embeds
the configured installation paths. The source installer sets `root:netdata` ownership (or the configured Netdata
group) and mode `4750`, matching the other eBPF plugins.

For the default installation, the new files are:

- `/usr/libexec/netdata/plugins.d/ebpf-poc.plugin`
- `/usr/lib/netdata/conf.d/ebpf-poc.conf`
- `/usr/lib/netdata/conf.d/ebpf-poc/cachestat.conf`

Custom installation prefixes are supported. Netdata discovers the executable and its stock cachestat job with the
usual default plugin settings. If automatic discovery of new plugins is disabled, set `ebpf-poc = yes` in the
`[plugins]` section of `netdata.conf`. Successful collection publishes context `ebpf_poc.page_cache_events`.

To run the installed collector directly for debugging:

```sh
sudo /usr/libexec/netdata/plugins.d/ebpf-poc.plugin -d -m cachestat
```

This POC requires Linux kernel BTF at `/sys/kernel/btf/vmlinux` and the supported cachestat probe targets. It has no
legacy-object fallback. Native package builds (`BUILD_FOR_PACKAGING`) and static installers (`STATIC_BUILD`) exclude
the POC; this integration is limited to source installations on the experimental branch.

## Build and run with Docker

From this worktree's repository root:

```sh
docker build --progress plain \
  -t netdata-ebpf-framework-poc:local \
  -f src/go/plugin/ebpf/poc/Dockerfile .

docker run --rm --name netdata-ebpf-framework-poc-smoke \
  --privileged --network none netdata-ebpf-framework-poc:local
```

The image builds the POC and runs its focused Go tests, then tests and builds the original Go eBPF plugin as a
baseline check. It does not build the Netdata Agent. It downloads the CO-RE bundle using the version and SHA256
pinned by `packaging/cmake/Modules/NetdataEBPFCORE.cmake`. The archive's skeleton headers are in `includes/`;
`CGO_CFLAGS` must point there. Go build and module caches use BuildKit cache mounts.

The Dockerfile-specific ignore file restricts the context to the two Go source trees. No repository credentials,
SOW memory or host filesystem mounts are needed. The smoke container uses its own temporary config, state and file
workload. Privileged execution is needed to load BPF programs and inspect them with `bpftool`.

Docker containers share the VM kernel: these are **Linux VM counters**, not macOS host counters or container-only
counters. Run the smoke test while no other BPF loader is changing the VM's program set; its program-set comparison
uses that short-lived controlled-environment assumption. It never unloads programs by ID. Only the POC's own handles
are closed. The container is removed on exit; the build image remains available.

To inspect protocol output manually inside the same image:

```sh
docker run --rm -it --name netdata-ebpf-framework-poc-manual \
  --privileged --network none netdata-ebpf-framework-poc:local bash

# Inside the container:
mkdir -p /tmp/ebpf-poc-varlib
NETDATA_LIB_DIR=/tmp/ebpf-poc-varlib \
  /usr/local/bin/ebpf-poc.plugin -d -c /src/src/go/plugin/ebpf/config -m cachestat
```

The terminal run automatically enables discovered jobs. The automated smoke uses pipes, receives the accepted single
configuration and enables it through DynCfg, as a Netdata daemon would. `SIGINT` stops a manual run; `QUIT` on stdin
stops a protocol run. No installed Netdata daemon is required.

## What the smoke test proves

`poc/smoke.py` starts the actual compiled executable and drives the real pluginsd protocol. Its backend is never
replaced with a fixture. It checks:

1. Discovery publishes a single config; schema and preflight test work without loading BPF programs.
2. Enable loads four native programs and emits a framework chart with incremental dimensions.
3. A bounded file write/read workload is followed by increasing real kernel counters.
4. DynCfg test while running leaves the incumbent program set unchanged.
5. Update from a one-second to two-second interval replaces the native program set, updates the chart interval,
   and is reflected by DynCfg get.
6. Disable unloads the owned programs; enable attaches a new set; QUIT unloads the final set and exits successfully.

On failure it prints recent protocol output and plugin errors, then stops only the child process it launched. A
nonzero exit or timeout fails the smoke test. No global cache eviction or changes to unrelated services occur.

## Observed evidence

Environment: Linux `6.12.76-linuxkit`, `aarch64`, Go 1.27.1, Debian trixie native libraries, repository-pinned CO-RE
bundle `v1.7.0.2`.

- Native macOS focused tests with `-race` and `go vet`: passed (native eBPF is unavailable on macOS).
- Linux/arm64 tagged tests with `-race`, without skeleton headers: passed; this checks the legacy-only build path.
- Linux/arm64 tagged tests with `-race`, with skeleton headers: passed; `go vet` and plugin build passed.
- Original eBPF module tests and original tagged executable build on Linux: passed. Its full test suite was also
  attempted on macOS, where existing config tests fail because they require a Linux kernel.
- Live CO-RE smoke: passed, four programs per active job and none of the owned programs remaining after cleanup.
- Independent read-only implementation review: no POC blockers found.
- Source installation: full Linux CMake configuration, the POC target build and installation under a custom prefix
  passed without building the Agent. The source installer's permission loop set `root:netdata` mode `4750`; the installed
  executable passed the live smoke as the `netdata` user using its compiled-in stock config paths.
- CMake rebuilds the POC after chart, shared schema and native backend edits. Configuring native packaging, static
  builds or disabled eBPF omits the POC target and install rules.

Example first live run, before and after the synthetic workload:

| Kernel event dimension | Initial total | Later total |
|---|---:|---:|
| accessed | 0 | 3 |
| buffer_dirty | 0 | 16401 |
| added | 0 | 0 |
| account_dirtied | 0 | 16391 |

These totals include activity from the whole VM and are not deterministic benchmark values. The test requires a real
increase, not particular counts or activity in every dimension. A kernel symbol's availability alone does not prove
that every modern kernel path calls it; this POC does not establish the old derived cache-hit semantics.

## Lifecycle and ownership

`Init` validates config. `Check` verifies that CO-RE was compiled in, BTF is readable and supported symbol names exist;
it does **not** run the kernel verifier or prove that attachment will succeed. That makes it safe for a config test
or replacement candidate overlapping an incumbent. Kernel privilege/verifier/attachment errors appear on collection.

The first `Collect` creates, prepares, loads, configures and attaches the native runtime. Subsequent collections reuse
that handle and write one snapshot into the framework-owned metric cycle. A failed snapshot returns an error so the
framework publishes a gap and applies its retry policy. It does not substitute zeros for a failed observation.

`Cleanup` closes the native runtime idempotently. The existing job runtime serializes collection and retires the job
before cleanup, so no new worker, singleton, global registry or framework extension is needed. The native adapter
closes partial handles on initialization failure. It checks cancellation between C calls; it cannot interrupt a C
call already in progress. The POC has no background loop and does not exercise `CollectorV2Runner`.

## Handoff and limits

- The C backend is imported directly from
  `src/collectors/ebpf.plugin/ebpfgo.plugin/libbpfloader` via a repository-local `go.mod` replacement. The original
  module already depends on `src/go`; the Go module graph therefore has a cycle, while the package import graph
  remains acyclic. This works in the tested builds and avoids copying or relocating the original implementation.
  A production migration should choose the permanent shared backend location; this POC keeps the old tree intact.
- The imported package compiles native code for the other existing eBPF modules too. Only cachestat is instantiated
  and only four cachestat programs are loaded by this executable.
- New collectors with shared memory, Functions, continuous event draining or different resource ownership need their
  own design. In particular, the old startup-only publisher election has not been ported or validated here.
- Stop/reload recreates maps and resets counters. Seamless state transfer, failed-attachment status presentation,
  cross-process singleton enforcement and privilege reduction are not established by this experiment.
- Neither full legacy-object runtime behavior nor the complete kernel/architecture/build matrix was tested.
- No throughput, CPU/RSS or per-event performance comparison was made. Do not infer performance parity from this POC.
- Native/static packages, integration metadata, alerts and replacement/removal of old collectors require a separate
  migration scope. Source installation on this branch includes the experiment for review and takeover.

Focused portable checks, from `src/go`:

```sh
go test -race -count=1 ./plugin/ebpf/... ./cmd/ebpfpocplugin
go vet ./plugin/ebpf/... ./cmd/ebpfpocplugin
```

Portable unit tests inject the native boundary and exercise known counter values, failure gaps, retry, cancellation,
target selection, config decoding, template materialization and actual JobV2 wire output. Linux smoke supplies the
complementary proof that the production native boundary, kernel and process protocol work together.
