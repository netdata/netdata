# Netdata Plugins - Multi-Call Binary

This is a multi-call binary that combines `log-viewer-plugin` and `otel-plugin` into a single executable.

## Overview

Following the same pattern as `journal-tools`, this binary reduces disk usage by combining two Netdata plugins that share significant infrastructure (journal-core, OpenTelemetry, etc.).

## Binary Size Comparison

### Release Build (`opt-level = "z"`, `lto = true`, `strip = true`)

**Standalone binaries:**
- `log-viewer-plugin`: 5.3 MB
- `otel-plugin`: 5.5 MB
- **Total: 10.8 MB**

**Multi-call binary:**
- `netdata-plugins`: 7.6 MB
- Symlinks: 0 bytes
- **Total: 7.6 MB**

**Savings: ~30% disk usage reduction (3.2 MB saved)**

## Usage

### Building

```bash
# Build multi-call binary
cargo build --release --bin netdata-plugins

# Build standalone binaries (still supported)
cargo build --release -p log-viewer-plugin -p otel-plugin
```

### Creating Symlinks

```bash
cd target/release

# Create symlinks for the plugins
ln -sf netdata-plugins log-viewer-plugin
ln -sf netdata-plugins otel-plugin

# Optional: create aliases
ln -sf netdata-plugins log-viewer
ln -sf netdata-plugins otel
```

### Using the Plugins

Once symlinks are created, use them as normal:

```bash
# Via log-viewer-plugin symlink
./log-viewer-plugin

# Via otel-plugin symlink
./otel-plugin

# Via aliases
./log-viewer
./otel
```

## Registered Plugins

- `log-viewer-plugin` - Netdata plugin for journal log viewing with histograms and facets
- `otel-plugin` - OpenTelemetry gRPC receiver that converts OTLP logs/metrics to Netdata
- Aliases:
  - `log-viewer` → `log-viewer-plugin`
  - `otel` → `otel-plugin`

## How It Works

The `netdata-plugins` binary uses the `multicall` library to:

1. Check `argv[0]` (how it was invoked)
2. Extract the binary name from the path
3. Look up the registered plugin
4. Call the appropriate library function
5. Return the exit code

## Architecture

Both plugins were converted to hybrid library+binary crates:

### log-viewer-plugin
- **Library**: `src/lib.rs` exports `pub fn run(args: Vec<String>) -> i32`
- **Binary**: `src/main.rs` calls the library function
- **Runtime**: Long-lived async service using `PluginRuntime` and Tokio
- **Dependencies**: journal-function, foyer (hybrid cache), rt (PluginRuntime)

### otel-plugin
- **Library**: `src/lib.rs` exports `pub fn run(args: Vec<String>) -> i32`
- **Binary**: `src/main.rs` calls the library function
- **Runtime**: Long-lived async gRPC server using Tokio and Tonic
- **Dependencies**: journal-log-writer, flatten_otel, OpenTelemetry stack

## Advantages

1. **Disk Usage**: One binary instead of two (~30% savings)
2. **Memory**: Shared code pages when both plugins run simultaneously
3. **Maintenance**: Single binary to distribute
4. **Flexibility**: Can still build standalone binaries if needed
5. **Deployment**: Easier package management

## Technical Details

### Async Runtime Handling

Both plugins are async and create their own Tokio runtimes internally:

```rust
pub fn run(args: Vec<String>) -> i32 {
    let runtime = tokio::runtime::Runtime::new().unwrap();
    runtime.block_on(async_run(args))
}
```

This keeps the multi-call interface synchronous while supporting async plugins.

### Long-Running Services

Unlike CLI tools (`journalctl`, `journal-sql`) which run and exit, both plugins are long-running daemon services:

- **log-viewer-plugin**: Runs a Netdata plugin that responds to function calls
- **otel-plugin**: Runs a gRPC server listening for OTLP traffic

The multi-call pattern works perfectly for both CLI tools and daemons.

## Files

```
netdata-plugins/
├── Cargo.toml          # Multi-call binary manifest
├── src/
│   └── main.rs         # Dispatch logic using multicall library
└── README.md           # This file

netdata-log-viewer/log-viewer-plugin/
├── Cargo.toml          # Now has [lib] and [[bin]] sections
├── src/
│   ├── lib.rs          # Library with run() function
│   ├── main.rs         # Thin wrapper that calls lib
│   ├── catalog.rs
│   ├── charts.rs
│   └── tracing_config.rs

netdata-otel/otel-plugin/
├── Cargo.toml          # Now has [lib] and [[bin]] sections
├── src/
│   ├── lib.rs          # Library with run() function
│   ├── main.rs         # Thin wrapper that calls lib
│   ├── chart_config.rs
│   ├── logs_service.rs
│   ├── metrics_service.rs
│   └── ...
```

## Testing

```bash
# Build
cargo build --bin netdata-plugins

# Create symlinks
cd target/debug
ln -sf netdata-plugins log-viewer-plugin
ln -sf netdata-plugins otel-plugin

# Test dispatch
./netdata-plugins  # Should show error with available tools
./log-viewer-plugin  # Should start log viewer plugin
./otel-plugin  # Should start OTEL plugin
```

## Deployment Example

```bash
#!/bin/bash
# install-netdata-plugins.sh

DEST=/usr/local/bin
BINARY=netdata-plugins

# Install main binary
install -m 755 target/release/$BINARY $DEST/

# Create symlinks
cd $DEST
ln -sf $BINARY log-viewer-plugin
ln -sf $BINARY otel-plugin
ln -sf $BINARY log-viewer
ln -sf $BINARY otel

echo "Installed netdata-plugins to $DEST"
echo "Available commands: log-viewer-plugin, otel-plugin, log-viewer, otel"
```

## Comparison with journal-tools

Both multi-call binaries follow the same pattern:

| Aspect | journal-tools | netdata-plugins |
|--------|--------------|----------------|
| **Tools** | journalctl, journal-sql | log-viewer-plugin, otel-plugin |
| **Type** | CLI tools (run and exit) | Daemon services (long-running) |
| **Savings** | ~50% (33 MB → 16.5 MB estimated) | ~30% (10.8 MB → 7.6 MB) |
| **Shared deps** | journal-core, DataFusion | journal-core, OpenTelemetry |
| **Async** | Yes (internal) | Yes (internal) |
| **Use case** | Interactive queries | Background services |

## Future Extensions

To add more plugins to this multi-call binary:

1. Convert the plugin to a library (add `src/lib.rs` with `pub fn run()`)
2. Add dependency in `netdata-plugins/Cargo.toml`
3. Register plugin in `netdata-plugins/src/main.rs`
4. Create symlink during deployment

## References

- `multicall` library: `../multicall/`
- `journal-tools` example: `../journal-tools/`
- Multi-call pattern analysis: `/home/vk/mo/multicall.pdf`
