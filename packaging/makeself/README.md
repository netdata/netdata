# Netdata Static Binary Build

We publish pre-built static builds of Netdata for Linux systems.

These builds are:

- Self-contained  
- Installed under `/opt/netdata`  
- Available for:  
  - `x86_64` (64-bit x86)  
  - `armv7l` (ARMv7)  
  - `aarch64` (AArch64)  
  - `ppc64le` (POWER8+)

> **Tip**  
> If your platform doesn’t have native Netdata packages, the installer will automatically use a static build.

---

## Enforce Static Build Only

To force the installer to use a static build (and fail if unavailable), add:  

```bash
--static-only
```

---

## Requirements

| Requirement | Purpose     |
|-------------|-------------|
| Docker or Podman | Required to build static binaries using containers |

---

## How to Build a Static Binary Package

Before building, ensure your repository is clean from previous builds.

→ [Perform a cleanup](/packaging/installer/methods/manual.md#perform-a-cleanup-in-your-netdata-repo)

---

### 1. Build for `x86_64` Architecture

In your Netdata repo root:

```bash
./packaging/makeself/build-static.sh x86_64
```

---

### 2. What Happens Automatically

The script will:

1. Start an Alpine Linux Docker container  
2. Install required packages  
3. Download and compile third-party tools (like `bash`, `curl`)  
4. Compile Netdata itself  

---

### 3. Output

The result is a single file:

```
netdata-vX.X.X-gGITHASH-x86_64-DATE-TIME.run
```

This is your static Netdata binary installer.

---

## Build for Other Architectures

Replace `x86_64` with:

| Architecture | Identifier  |
|--------------|-------------|
| ARMv7        | `armv7l`   |
| AArch64      | `aarch64`  |
| POWER8+      | `ppc64le`  |

Example for ARMv7:

```bash
./packaging/makeself/build-static.sh armv7l
```

---

## Build Static Binary With Debug Info

Debug builds:

- Are larger  
- Are slower  
- Disable some optimizations  
- Enable tracing/debugging features  

---

### Command

```bash
cd /path/to/netdata.git
./packaging/makeself/build-static.sh x86_64 debug
```

---

## Debugging a Static Build with Valgrind

To run Netdata under `valgrind`:

```bash
PATH="/opt/netdata/bin:${PATH}" valgrind --undef-value-errors=no /opt/netdata/bin/srv/netdata -D
```

---

### Notes:

- Expect Netdata to run ~10x slower under `valgrind`.  
- To stop it: Press `Ctrl+C`.

---

### Why `--undef-value-errors=no`?

Without it, you'll see hundreds of harmless warnings about uninitialized values.

This happens because Netdata’s static binary includes its own built-in libraries — `valgrind` can't filter these like system libraries.

---

## If Netdata Crashes

- Valgrind will print a stack trace.  
- Please open a GitHub issue and share the output.