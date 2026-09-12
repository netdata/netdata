---
name: packaging-static-installer
description: Build, test, review or troubleshoot Netdata static makeself installers and packaging/makeself changes. Covers x86_64, aarch64, armv6l and armv7l builds, cache/image issues, artifact inspection and explicitly requested target deployment.
---

# Building a static Netdata binary

The static build produces a self-extracting installer (`netdata-<arch>-latest.gz.run`) for a compatible Linux system
matching the selected architecture, including supported 32-bit ARM targets. It installs under `/opt/netdata` without
requiring a native build toolchain on the target.

| Task | Read |
|---|---|
| Build an artifact | Pre-flight, orchestration, output and cache sections; cross-architecture/debug sections when applicable |
| Review or explain packaging | Affected source owners and matching sections; assess existing build evidence without executing operational examples |
| Diagnose a build failure | Common failures plus the failing job and its current dependencies |
| Inspect or extract an archive | Output artifacts and extraction guidance; extraction writes files and requires root |
| Deploy to a target | Deployment section, matching architecture and the actual installation/update policy requested |

Loading this skill does not authorize image pulls/removal, submodule changes, privileged host registration, builds,
extraction or target installation. An authorized build may involve those build prerequisites: inspect the concrete
requirements and apply existing authorization. Preserve unrelated submodule work and existing artifacts before any
operation that would replace them. Building an artifact alone does not authorize deploying it.

Timing, size and slowdown figures below are historical observations, not build or compatibility guarantees.

## TL;DR — x86_64 native

```bash
# 1. Pre-flight for an authorized build (inspect first; honor pinned reproduction inputs)
# Initialize only missing required submodules using Gotcha #1 below.
# For a normal build, refresh the image; retain the selected image for a pinned reproduction.
docker pull netdata/static-builder:v1     # see Gotcha #2

# 2. Build (~22-25 min cold, ~10-15 min on cache hit, on a 24-core host)
./packaging/makeself/build-static.sh x86_64

# 3. Output
ls -la artifacts/
# artifacts/netdata-x86_64-latest.gz.run        historically ~190 MB; matching compatible x86_64 Linux target
# artifacts/netdata-x86_64-vX.Y.Z-N-nightly.gz.run
# artifacts/netdata-latest.gz.run               (x86_64 only — alias of the above)
# artifacts/netdata-vX.Y.Z-N-nightly.gz.run     (x86_64 only — alias)
```

For debug builds: `./packaging/makeself/build-static.sh x86_64 debug`.

## What you actually run

The orchestrator is `packaging/makeself/build-static.sh`, which:

1. Translates the architecture name (`x86_64`, `aarch64`, `armv6l`, `armv7l`) to a docker `--platform` value via `packaging/makeself/uname2platform.sh`.
2. Sets per-arch tuning flags (`packaging/makeself/build-static.sh:27-56`):
   - `x86_64` → `-march=x86-64` baseline (distinct from the `Nehalem-v2` QEMU CPU choice), `GOAMD64=v1`.
   - `aarch64` → `-march=armv8-a`, Cortex-A53, `GOARM64=v8.0`.
   - `armv7l` → `-march=armv7-a`, Cortex-A7, `GOARM=7`.
   - `armv6l` → `-march=armv6zk -mtune=arm1176jzf-s`, ARM1176, `GOARM=6`.
3. Registers `binfmt`/QEMU on the container host using a privileged container when cross-arch emulation is needed.
   Existing registration or `SKIP_EMULATION` skips that operation.
4. Pulls `netdata/static-builder:v1` if missing locally; removes a mismatched-platform image first.
5. Bind-mounts `$(pwd)` into the container at `/netdata` and runs `/netdata/packaging/makeself/build.sh` inside it.

The container then runs `packaging/makeself/run-all-jobs.sh`, which executes `packaging/makeself/jobs/*.sh` in lexical order:

| # | Job | What it does |
|---|-----|--------------|
| 00 | `prepare-destination` | Lays out `/opt/netdata/{bin,usr,sbin,...}` symlinks |
| 10 | `libucontext.install` | Builds bundled libucontext (musl context-switch fallback) |
| 11 | `openssl.install` | Builds OpenSSL statically; version comes from `bundled-packages.version` |
| 20 | `libnetfilter_acct.install` | Builds libnetfilter_acct statically |
| 20 | `libunwind.install` | Builds libunwind statically |
| 30 | `curl.install` | Builds curl + libcurl statically |
| 40 | `bash.install` | Builds bash statically |
| 50 | `ioping.install` | Builds ioping statically |
| 70 | `netdata-git.install` | Builds Netdata itself (Rust crates + C/CMake + Go plugins) and installs into `/opt/netdata` |
| 71 | `install-type` | Stamps `.install-type` so the post-installer knows it's a static install |
| 72 | `conf-fixup` | Strips machine-specific configuration and sample files |
| 80 | `netdata-static-check` | Verifies key binaries are statically linked |
| 81 | `netdata-runtime-check` | Boots `/opt/netdata/bin/netdata`, waits for `localhost:19999`, checks `/api/v1/info` |
| 82 | `cpu-arch-check` | Checks ELF class/machine for Netdata and go.d; does not prove the instruction-set baseline |
| 89 | `buildinfo.install` | Writes `/opt/netdata/share/netdata/buildinfo.txt` |
| 90 | `prepare-archive-source` | Copies post-installer scripts into the install tree |
| 91 | `copy-ca-certificates` | Bundles a CA bundle |
| 98 | `create-archive` | Runs `makeself.sh --gzip --complevel 9 --notemp --needroot` to build the `.gz.run` |
| 99 | `copy-archives` | Renames the archive to `netdata-<arch>-<version>.gz.run` and copies aliases |

Source files: `packaging/makeself/build-static.sh`, `packaging/makeself/build.sh`, `packaging/makeself/run-all-jobs.sh`, `packaging/makeself/functions.sh`, `packaging/makeself/jobs/`.

## Pre-flight (DO NOT SKIP)

For an authorized build, establish these prerequisites before spending build time. The launcher bind-mounts the
checkout writable and replaces generated artifacts; preserve existing results that must survive the build.

### Gotcha #1: submodules must be initialized

The build configures with CMake against vendored sources at:

- `src/aclk/aclk-schemas/`
- `src/collectors/debugfs.plugin/libsensors/vendored/`

If either is empty, CMake aborts after ~2 min with:

```
Cannot find source file: vendored/lib/access.c
No SOURCES given to target: vendored_libsensors
ABORTED  Failed to configure Netdata sources.
```

A plain fresh clone or linked worktree may have uninitialized submodules. Inspect `git submodule status` and local
submodule changes first. Initialize missing required sources for the authorized build; do not reset modified or
divergent submodules merely to make the status clean. For each required path listed above whose status begins
with `-`, set `missing_required_submodule` to that exact path in the same shell invocation as the command below.
Repeat that assignment and command once per selected path; do not assume shell variables persist across separate tool calls:

```bash
git submodule update --init -- "${missing_required_submodule:?set one uninitialized required submodule path}"
```

The path argument confines initialization to the selected missing module. Do not use an unscoped or recursive update:
that can move already initialized modules to recorded commits, including unrelated or deliberately divergent modules.

Verify with `git submodule status`: `-` means uninitialized, `+` differs from the recorded commit, and `U` is
conflicted.
A leading space confirms the recorded commit, not the absence of local edits. Investigate differences before proceeding.

### Gotcha #2: refresh the cached docker image

The launcher pulls the target-platform image only when it is missing; it removes a cached image with a mismatched
platform first. A previously observed stale image failed checksum verification with:

```
sha256sum: unrecognized option: c
SHA256 verification of tar file libnetfilter_acct-1.0.3.tar.bz2 failed (rc=1)
expected: <hash>, got <same-hash>
```

`functions.sh` uses `sha256sum --c --status`; an implementation lacking that option can fail despite matching bytes.
This symptom is evidence to inspect the image/tool, not proof of a specific Alpine version. For a normal build session,
refresh the intended platform's image and record its identity; a deliberately pinned reproduction should retain its
selected image. The quick-start pull above targets native x86_64; use `--platform` from `uname2platform.sh` for other
targets.
A pull can replace the cached tag; current registry contents are not established by this skill.

## Output artifacts

Job `99-copy-archives.sh` writes to `artifacts/` (gitignored, host-side, owned by your user):

```
artifacts/
├── netdata-x86_64-latest.gz.run            # copied alias of the versioned archive
├── netdata-x86_64-v2.10.0-171-nightly.gz.run
├── netdata-latest.gz.run                   # x86_64 only — generic alias
├── netdata-v2.10.0-171-nightly.gz.run      # x86_64 only — generic alias
└── cache/                                  # build cache, see below
```

For non-x86_64 builds, only the two `netdata-<arch>-*` files are produced (`packaging/makeself/jobs/99-copy-archives.sh:25-30`).

Verify a build:

```bash
ls -la artifacts/
sha256sum artifacts/netdata-x86_64-latest.gz.run

# What's inside (read-only inspection, does not run the installer)
sh artifacts/netdata-x86_64-latest.gz.run --info
sh artifacts/netdata-x86_64-latest.gz.run --list | head
```

Each `.gz.run` is a `makeself` archive: a shell prefix that extracts the gzipped tar embedded after it. Run it as root on the target to install.

## Build cache

`artifacts/cache/<arch>/` holds the compiled third-party deps (openssl, curl, bash, libunwind, libnetfilter_acct,
ioping) keyed by their pinned source versions in `packaging/makeself/bundled-packages.version`. The fetch logic is in
`packaging/makeself/functions.sh` (`cache_path()`, `fetch()`, `fetch_git()`, `store_cache()`).

Implications:

- First build: ~22-25 min on a 24-core host (most time = third-party compile + Rust crate compile + LTO link of the C plugins).
- Cache hit: ~10-15 min (skips the third-party deps; only the netdata sources rebuild).
- The cache survives `git checkout` and `git clean -fd` (it's under the gitignored `artifacts/`).
- Bumping a version in `bundled-packages.version` invalidates that one entry — the rest still reuse.
- Cache entries are version-derived directories under `<arch>/<package>/`; image contents and compiler flags are not
  part of that key. For a cold rebuild, preserve the exact cache being invalidated outside the active cache path, or
  build in an isolated checkout. Do not delete all of `artifacts/`: it also holds installers and other architectures.
  Any deletion still needs the existing task authorization.

## Cross-architecture builds (aarch64, armv7l, armv6l)

```bash
./packaging/makeself/build-static.sh aarch64
./packaging/makeself/build-static.sh armv7l
./packaging/makeself/build-static.sh armv6l
```

The script auto-installs QEMU binfmt handlers via `tonistiigi/binfmt:master` if not already registered (`packaging/makeself/build-static.sh:60-62`). Cross-arch builds:

- Run all C/Rust/Go compilation under QEMU emulation — expect 4-8× slowdown vs native.
- Are CPU-bound, not network-bound — the source download is one-time.
- Can fail in ways native builds do not (e.g. Rust LTO under QEMU has historically OOMed; libbpf BPF skeleton generation has hit qemu syscall edge cases). When you see a failure that isn't on x86_64 native, suspect QEMU first.

`SKIP_EMULATION=1` is set automatically when the host already has a `binfmt_misc` entry for the target arch (e.g. on a CI runner with persistent qemu).

## Debug builds

```bash
./packaging/makeself/build-static.sh x86_64 debug
```

Sets `NETDATA_BUILD_WITH_DEBUG=1` (`packaging/makeself/build.sh:9-22`), which selects reduced C optimization
(`-O1 -ggdb`) and internal checks in the Netdata build job. Historically the archive was about twice the size, with
slower runtime useful for valgrind/gdb. The `README.md` in `packaging/makeself/` documents valgrind invocation.

## Common failures

| Symptom | Job | Cause | Fix |
|---------|-----|-------|-----|
| `Cannot find source file: vendored/lib/access.c` | 70 (CMake configure) | Submodules not initialized | Initialize only the missing required path under Gotcha #1 |
| `sha256sum: unrecognized option: c` then `expected: X, got X` | 11 / 20 / 30 / 40 / 50 | Image checksum utility lacks the required option; stale image is one observed cause | For a normal build, refresh the intended platform image under Gotcha #2; retain pinned reproduction inputs |
| `No cached copy of build directory for X found, fetching sources instead.` (every run) | any third-party | `artifacts/cache/` removed or arch dir missing | Normal on first build; persists for the next run |
| `Could not find a usable OCI runtime` | n/a | Neither docker nor podman in `$PATH` | Install one |
| Runtime check times out waiting for localhost:19999 | 81 | Agent did not become reachable within the bounded wait; cause is not yet established | Inspect the job log and `netdata.log`, then diagnose startup |
| `not statically linked` warning | 80 (static check) | A new dep introduced a dynamic link | Audit `ldd` of the built binary; check `CMakeLists.txt` for `target_link_libraries` adding a shared lib |
| OOM kill mid-Rust compile under QEMU | 70 | QEMU + Rust LTO is memory-hungry | Use a suitably provisioned native builder or investigate actual job parallelism; the launcher exposes no `PROCESSORS` knob |

The build script exits with `Build failed.` on any job failure (`packaging/makeself/build.sh:44-52`). For an
interactive TTY launch, `DEBUG_BUILD_INFRA=1` is forwarded and opens a `bash` shell on failure; the non-TTY
branch does not forward it. Use an interactive launch when that diagnostic shell is needed.

## Watching a long-running build

Prefer the execution tool's tracked session for a long build. If using a shell background job, give it a fresh task
directory and capture its PID immediately:

```bash
BUILD_RUN="$(mktemp -d "${TMPDIR:-/tmp}/netdata-build.XXXXXX")" || exit $?
LOG="$BUILD_RUN/build.log"
nohup ./packaging/makeself/build-static.sh x86_64 > "$LOG" 2>&1 &
BUILD_PID=$!
printf '%s\n' "$BUILD_PID" > "$BUILD_RUN/build.pid"

# Progress
grep -E '^ --- running' "$LOG"          # job-level milestones
tail -f "$LOG"                          # streaming output
docker stats --no-stream                # CPU/RAM of the running container
```

Before stopping it, verify the recorded PID still belongs to this task; stopping the launcher does not prove its
container stopped. Inspect the task container identity separately and never terminate by a broad process name.

Indicators the build is alive (output buffering can stall the log for minutes during heavy compile):

- `ps -eo pid,pcpu,comm --sort=-pcpu | head` shows `rustc`, `cc1`, `lto1-ltrans` near 50-70% each.
- `docker stats` shows the static-builder container at hundreds of % CPU.
- The container's working set in `docker stats` keeps changing.

## Deploying to a target

Use this only for the requested target installation, with a matching architecture and the requested update policy.

```bash
# Copy
scp artifacts/netdata-x86_64-latest.gz.run target-host:/tmp/

# Install (on the target, as root)
ssh target-host
sudo sh /tmp/netdata-x86_64-latest.gz.run -- --auto-update
# installer flags after the bare `--`; common: --dont-start-it, --disable-telemetry,
# --claim-token <T> --claim-rooms <R>, --no-updates
```

The installer always installs into `/opt/netdata` (hard-coded; `--target` would change it but the in-archive paths assume `/opt/netdata`, do not override).

Read-only archive metadata/listing:

```bash
sh artifacts/netdata-x86_64-latest.gz.run --info     # makeself metadata
sh artifacts/netdata-x86_64-latest.gz.run --list     # full file manifest
```

For requested extraction, use a fresh directory. `--noexec` skips the installer but still writes files and does not
bypass the archive's root requirement. `--target` here controls extraction, not a supported alternate install prefix.

```bash
EXTRACT_DIR="$(mktemp -d "${TMPDIR:-/tmp}/netdata-extract.XXXXXX")" || exit $?
sudo sh artifacts/netdata-x86_64-latest.gz.run --target "$EXTRACT_DIR" --noexec --keep
```

Keep the extracted files for inspection; do not reuse an existing destination or remove unrelated files.

## How to extend this skill

Capture timing and authorization follow `AGENTS.md#knowledge-capture`. Authorized recipes for build failures,
architecture-specific quirks, and reusable workflows belong in `how-tos/`, with an entry in `./how-tos/INDEX.md`.
Keep `SKILL.md` focused on the workflow and route detailed recipes through the catalog.

## Source-of-truth pointers

- `packaging/makeself/README.md` — high-level user-facing doc (architectures, valgrind notes).
- `packaging/makeself/build-static.sh` — host-side launcher, arch matrix, docker invocation.
- `packaging/makeself/build.sh` — in-container entry; debug-flag parsing.
- `packaging/makeself/run-all-jobs.sh` — job runner.
- `packaging/makeself/functions.sh` — `cache_path()`, `fetch()`, `fetch_git()`, `store_cache()`, `progress()`, `run()` helpers.
- `packaging/makeself/jobs/*.sh` — the ordered build steps.
- `packaging/makeself/bundled-packages.version` — pinned versions of openssl, curl, bash, libunwind, libnetfilter_acct, ioping.
- `packaging/makeself/install-alpine-packages.sh` — the package list the static-builder docker image is built from (used when refreshing the image, not on every build).
- `packaging/makeself/uname2platform.sh` — arch → docker `--platform` translation.
- `packaging/makeself/makeself.sh`, `makeself-header.sh` — vendored makeself archive builder.
