# Netdata Static Binary Build

Netdata provides pre-built static binaries for Linux systems where native packages aren't available. The [installer](/packaging/installer/methods/kickstart.md) automatically uses these static builds when needed.

**Key Features**:

- Self-contained installation (no system dependencies required, no interference with system libraries)
- Installed under `/opt/netdata`

## Supported Architectures

| Architecture | Identifier | Notes                            |
|--------------|------------|----------------------------------|
| x86_64       | `x86_64`   | 64-bit Intel/AMD processors      |
| ARMv6        | `armv6l`   | Raspberry Pi 1, some older SBCs  |
| ARMv7        | `armv7l`   | Raspberry Pi 2/3, many SBCs      |
| AArch64      | `aarch64`  | ARM 64-bit (Pi 4, newer devices) |

---

## Build Process

### Requirements

| Requirement      | Purpose                                   |
|------------------|-------------------------------------------|
| Docker or Podman | Container environment for isolated builds |
| ~10GB disk space | For build artifacts and containers        |

### Preparation

Before building, ensure your repository is clean from previous builds.

→ [Perform a cleanup](/packaging/installer/methods/manual.md#perform-a-cleanup-in-your-netdata-repo)

### Building the Static Binary

Run the build script with your target [architecture identifier](#supported-architectures):

```bash
# For x86_64 (default)
./packaging/makeself/build-static.sh x86_64

# For ARM 64-bit (AArch64)
./packaging/makeself/build-static.sh aarch64

# For ARMv6
./packaging/makeself/build-static.sh armv6l

# For ARMv7
./packaging/makeself/build-static.sh armv7l
```

The script will automatically:

- Launch an Alpine Linux container
- Install necessary build dependencies
- Compile required third-party tools (bash, curl, etc.)
- Build Netdata with optimized settings
- Package everything into a self-extracting installer

When building for an architecture different from your host:

- The build process uses QEMU for emulation
- Build times will be significantly longer
- More disk space may be required
- Some features may have architecture-specific limitations

### Build Output

The process generates several installer files in the artifacts/ directory:

```
artifacts/
├── netdata-latest.gz.run                    # Latest version
├── netdata-v[VERSION]-[BUILD].gz.run        # Specific version
├── netdata-[ARCH]-latest.gz.run             # Architecture-specific latest
└── netdata-[ARCH]-v[VERSION]-[BUILD].gz.run # Architecture-specific version
```

Example output:

```
$ ls -l artifacts/
drwxrwxr-x   - user group     cache
.rwxrwxr-x 94M user group     netdata-latest.gz.run
.rwxrwxr-x 94M user group     netdata-v2.3.0-193-nightly.gz.run
.rwxrwxr-x 94M user group     netdata-x86_64-latest.gz.run
.rwxrwxr-x 94M user group     netdata-x86_64-v2.3.0-193-nightly.gz.run
```

## Advanced Build Options

### Debug Builds

For troubleshooting, you can create a debug-enabled build:

```bash
./packaging/makeself/build-static.sh x86_64 debug
```

Debug build characteristics:

- Larger file size
- Runtime performance impact
- Additional diagnostic information
- Disabled optimizations
- Enhanced tracing capabilities

## Troubleshooting

### Running with Valgrind

To diagnose memory issues or crashes:

```bash
PATH="/opt/netdata/bin:${PATH}" valgrind --undef-value-errors=no /opt/netdata/bin/srv/netdata -D
```

**Important notes**:

- Performance will be significantly reduced (~10x slower)
- Stop Valgrind with Ctrl+C
- The `--undef-value-errors=no` flag suppresses hundreds of false positives from the bundled libraries

### Crash Reporting

If Netdata crashes during development or testing:

1. Capture the complete Valgrind output
2. [Open a GitHub Issue](https://github.com/netdata/netdata/issues/new/choose)
3. Include:
    - Build details (architecture, version)
    - Complete stack trace
    - Steps to reproduce the issue
