---
name: project-build-static-binary
description: Build a static, self-extracting Netdata installer (`netdata-<arch>-latest.gz.run`) from this checkout for x86_64, aarch64, armv6l, or armv7l. Use when the user asks to build, produce, package, or test a static binary, makeself installer, `.gz.run` artifact, or "static install" of Netdata; when verifying a PR by deploying it to a Linux machine without a native build toolchain; when reproducing a CI static-builder issue locally. Covers the docker-based build flow under `packaging/makeself/`, mandatory pre-flight checks (submodule init, fresh `netdata/static-builder:v1` image), the 18 ordered jobs the build runs, output artifact layout, the `artifacts/cache/` reuse model, cross-arch QEMU caveats, debug builds, common failures with their fixes, and how to copy/verify the artifact on a target host.
type: project
---

# Building a static Netdata binary

The static build produces a self-extracting installer (`netdata-<arch>-latest.gz.run`) that runs on any 64-bit Linux without a native toolchain or system libraries. The output installs under `/opt/netdata`. Use it to test a PR on a target system that lacks build tools (RHEL, old distros, embedded), to ship a prebuilt to a customer host, or to reproduce CI behavior locally.

This skill captures the full local build flow plus the gotchas that block first-time builds. Read top to bottom on first use; the [Common failures](#common-failures) section exists because every one of them has burned a build in this repo.

## TL;DR — x86_64 native

```bash
# 1. Pre-flight (mandatory)
docker pull netdata/static-builder:v1     # refresh the cached image — see Gotcha #2
git submodule update --init --recursive   # populate vendored sources — see Gotcha #1

# 2. Build (~22-25 min cold, ~10-15 min on cache hit, on a 24-core host)
./packaging/makeself/build-static.sh x86_64

# 3. Output
ls -la artifacts/
# artifacts/netdata-x86_64-latest.gz.run        ~190 MB, ready to copy to any 64-bit Linux
# artifacts/netdata-x86_64-vX.Y.Z-N-nightly.gz.run
# artifacts/netdata-latest.gz.run               (x86_64 only — alias of the above)
# artifacts/netdata-vX.Y.Z-N-nightly.gz.run     (x86_64 only — alias)
```

For debug builds: `./packaging/makeself/build-static.sh x86_64 debug`.

## What you actually run

The orchestrator is `packaging/makeself/build-static.sh`, which:

1. Translates the architecture name (`x86_64`, `aarch64`, `armv6l`, `armv7l`) to a docker `--platform` value via `packaging/makeself/uname2platform.sh`.
2. Sets per-arch tuning flags (`packaging/makeself/build-static.sh:27-56`):
   - `x86_64` → `-march=x86-64` baseline (`x86-64-v2` equivalent), Nehalem-v2 QEMU CPU, `GOAMD64=v1`.
   - `aarch64` → `-march=armv8-a`, Cortex-A53, `GOARM64=v8.0`.
   - `armv7l` → `-march=armv7-a`, Cortex-A7, `GOARM=7`.
   - `armv6l` → `-march=armv6zk -mtune=arm1176jzf-s`, ARM1176, `GOARM=6`.
3. Installs `binfmt`/QEMU emulation if cross-arch and not already registered.
4. Pulls `netdata/static-builder:v1` if missing locally; removes a mismatched-platform image first.
5. Bind-mounts `$(pwd)` into the container at `/netdata` and runs `/netdata/packaging/makeself/build.sh` inside it.

The container then runs `packaging/makeself/run-all-jobs.sh`, which executes `packaging/makeself/jobs/*.sh` in lexical order:

| # | Job | What it does |
|---|-----|--------------|
| 00 | `prepare-destination` | Lays out `/opt/netdata/{bin,usr,sbin,...}` symlinks |
| 10 | `libucontext.install` | Builds bundled libucontext (musl context-switch fallback) |
| 11 | `openssl.install` | Builds OpenSSL 3.6.0 statically |
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
| 82 | `cpu-arch-check` | Verifies the produced binaries match the target arch baseline |
| 89 | `buildinfo.install` | Writes `/opt/netdata/share/netdata/buildinfo.txt` |
| 90 | `prepare-archive-source` | Copies post-installer scripts into the install tree |
| 91 | `copy-ca-certificates` | Bundles a CA bundle |
| 98 | `create-archive` | Runs `makeself.sh --gzip --complevel 9 --notemp --needroot` to build the `.gz.run` |
| 99 | `copy-archives` | Renames the archive to `netdata-<arch>-<version>.gz.run` and copies aliases |

Source files: `packaging/makeself/build-static.sh`, `packaging/makeself/build.sh`, `packaging/makeself/run-all-jobs.sh`, `packaging/makeself/functions.sh`, `packaging/makeself/jobs/`.

## Pre-flight (DO NOT SKIP)

These two checks save you a wasted 20+ min build.

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

A fresh `git clone` does **not** populate submodules. A worktree created from a fork that hasn't initialized submodules does not either. Always run before your first build in a working tree:

```bash
git submodule update --init --recursive
```

Verify with `git submodule status` — every line must start with a space (the leading `-` means uninitialized).

### Gotcha #2: refresh the cached docker image

The build pulls `netdata/static-builder:v1` only if no local image exists. If you have an old cached image (the project published a v1 in 2023 based on Alpine 3.18, then refreshed it in 2026 with Alpine 3.23 + coreutils), the build fails inside `packaging/makeself/functions.sh:84` with:

```
sha256sum: unrecognized option: c
SHA256 verification of tar file libnetfilter_acct-1.0.3.tar.bz2 failed (rc=1)
expected: <hash>, got <same-hash>
```

Why: `functions.sh:84` runs `sha256sum --c --status` (long-option form). BusyBox's sha256sum (Alpine without `coreutils`) does not accept long options, so it fails even though the actual hash matches. The fresh image installs `coreutils`, which symlinks `/usr/bin/sha256sum` to GNU coreutils and accepts the long form.

Fix: always re-pull before a build session:

```bash
docker pull netdata/static-builder:v1
```

The pull is a no-op if your cache is already up to date.

## Output artifacts

Job `99-copy-archives.sh` writes to `artifacts/` (gitignored, host-side, owned by your user):

```
artifacts/
├── netdata-x86_64-latest.gz.run            # symlink/alias to the latest
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

`artifacts/cache/<arch>/` holds the compiled third-party deps (openssl, curl, bash, libunwind, libnetfilter_acct, ioping) keyed by their pinned source versions in `packaging/makeself/bundled-packages.version`. The fetch logic is in `packaging/makeself/functions.sh` (`fetch()` + `cache_key()`).

Implications:

- First build: ~22-25 min on a 24-core host (most time = third-party compile + Rust crate compile + LTO link of the C plugins).
- Cache hit: ~10-15 min (skips the third-party deps; only the netdata sources rebuild).
- The cache survives `git checkout` and `git clean -fd` (it's under the gitignored `artifacts/`).
- Bumping a version in `bundled-packages.version` invalidates that one entry — the rest still reuse.
- To force a clean build: `rm -rf artifacts/`.

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

Sets `NETDATA_BUILD_WITH_DEBUG=1` (`packaging/makeself/build.sh:9-22`), which disables optimization and includes debug symbols. Result: larger archive (~2× size), slower runtime, useful for valgrind/gdb. The `README.md` in `packaging/makeself/` documents valgrind invocation.

## Common failures

| Symptom | Job | Cause | Fix |
|---------|-----|-------|-----|
| `Cannot find source file: vendored/lib/access.c` | 70 (CMake configure) | Submodules not initialized | `git submodule update --init --recursive` |
| `sha256sum: unrecognized option: c` then `expected: X, got X` | 11 / 20 / 30 / 40 / 50 | Stale `static-builder:v1` (Alpine 3.18, no coreutils) | `docker pull netdata/static-builder:v1` |
| `No cached copy of build directory for X found, fetching sources instead.` (every run) | any third-party | `artifacts/cache/` removed or arch dir missing | Normal on first build; persists for the next run |
| `Could not find a usable OCI runtime` | n/a | Neither docker nor podman in `$PATH` | Install one |
| `Waiting for netdata on localhost:19999 ...` hangs forever | 81 (runtime check) | Built binary segfaulted at startup | Read end of `81-netdata-runtime-check.sh` log; reproduce locally with the install path |
| `not statically linked` warning | 80 (static check) | A new dep introduced a dynamic link | Audit `ldd` of the built binary; check `CMakeLists.txt` for `target_link_libraries` adding a shared lib |
| OOM kill mid-Rust compile under QEMU | 70 | QEMU + Rust LTO is memory-hungry | Add swap; reduce `-j` via `PROCESSORS` env |

The build script exits with `Build failed.` on any job failure (`packaging/makeself/build.sh:44-52`). If `DEBUG_BUILD_INFRA=1` is set in the environment, the container drops to a `bash` shell instead of exiting — useful when you want to inspect the in-progress install tree.

## Watching a long-running build

Background launch + log tail pattern:

```bash
LOG=/tmp/netdata-build-$(date +%s).log
nohup ./packaging/makeself/build-static.sh x86_64 > "$LOG" 2>&1 &
echo $! > /tmp/netdata-build.pid

# Progress
grep -E '^ --- running' "$LOG"          # job-level milestones
tail -f "$LOG"                          # streaming output
docker stats --no-stream                # CPU/RAM of the running container
```

Indicators the build is alive (output buffering can stall the log for minutes during heavy compile):

- `ps -eo pid,pcpu,comm --sort=-pcpu | head` shows `rustc`, `cc1`, `lto1-ltrans` near 50-70% each.
- `docker stats` shows the static-builder container at hundreds of % CPU.
- The container's working set in `docker stats` keeps changing.

## Deploying to a target

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

Inspecting what's inside without installing:

```bash
sh artifacts/netdata-x86_64-latest.gz.run --info     # makeself metadata
sh artifacts/netdata-x86_64-latest.gz.run --list     # full file manifest
sh artifacts/netdata-x86_64-latest.gz.run --target /tmp/nd-extract --noexec --keep
```

## How to extend this skill

When you discover a new failure mode, an arch-specific quirk, or a workflow that's worth preserving, add a how-to under `how-tos/` and link it from `how-tos/INDEX.md`. Keep `SKILL.md` tight; push detail into how-tos. The catalog is live — every assistant who solves a non-trivial build problem must commit the recipe before moving on.

## Source-of-truth pointers

- `packaging/makeself/README.md` — high-level user-facing doc (architectures, valgrind notes).
- `packaging/makeself/build-static.sh` — host-side launcher, arch matrix, docker invocation.
- `packaging/makeself/build.sh` — in-container entry; debug-flag parsing.
- `packaging/makeself/run-all-jobs.sh` — job runner.
- `packaging/makeself/functions.sh` — `fetch()`, `cache()`, `cache_key()`, `progress()`, `run()` helpers.
- `packaging/makeself/jobs/*.sh` — the 18 ordered build steps.
- `packaging/makeself/bundled-packages.version` — pinned versions of openssl, curl, bash, libunwind, libnetfilter_acct, ioping.
- `packaging/makeself/install-alpine-packages.sh` — the package list the static-builder docker image is built from (used when refreshing the image, not on every build).
- `packaging/makeself/uname2platform.sh` — arch → docker `--platform` translation.
- `packaging/makeself/makeself.sh`, `makeself-header.sh` — vendored makeself archive builder.
