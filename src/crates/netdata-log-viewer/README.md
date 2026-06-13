# Netdata Log Viewer Plugin

A Netdata external plugin for querying and visualizing systemd journal entries with histogram analysis and faceted search.

## Overview

This plugin provides a `systemd-journal` function that Netdata can call to query journal entries, compute histograms, and return faceted data for visualization in the Netdata dashboard.

## Architecture

```
┌─────────────┐
│   Netdata   │
│   Agent     │
└──────┬──────┘
       │ stdin/stdout
       │ (plugin protocol)
       ↓
┌──────────────────────────┐
│  otel-signal-viewer-plugin   │
│                          │
│  ┌───────────────┐       │
│  │ Journal       │       │
│  │ Handler       │       │
│  └───────────────┘       │
│         ↓                │
│  ┌───────────────┐       │
│  │ Shared State  │       │
│  │ (AppState)    │       │
│  └───────────────┘       │
│         ↓                │
│  ┌───────────────┐       │
│  │ histogram-    │       │
│  │ service       │       │
│  └───────────────┘       │
│         ↓                │
│  ┌───────────────┐       │
│  │ journal       │       │
│  │ (indexing)    │       │
│  └───────────────┘       │
└──────────────────────────┘
         ↓
    ┌─────────┐
    │ Jaeger  │ (tracing)
    └─────────┘
```

## Key Features

- **Fast histogram computation** using pre-built indexes
- **Faceted search** across journal fields (PRIORITY, HOSTNAME, etc.)
- **Caching** with memory + disk tiers for performance
- **Distributed tracing** via OpenTelemetry/Jaeger
- **Metrics tracking** for function call success/failure rates

## Quick Start

### Prerequisites

1. **Jaeger** (optional, for development visibility):
```bash
# NOTE: Port 4318 avoids conflict with Netdata's otel-plugin on 4317
docker run -d --name jaeger \
  -p 16686:16686 \
  -p 4318:4317 \
  jaegertracing/all-in-one:latest
```

### Building

```bash
cargo build --bin otel-signal-viewer-plugin --release
```

### Running

The plugin is designed to be spawned by Netdata:

```bash
# Netdata spawns the plugin automatically when configured
# Configure in /etc/netdata/netdata.conf:

[plugins]
    otel-signal-viewer-plugin = yes
```

For development/testing:

```bash
# Set log level
export RUST_LOG="debug"

# Run Netdata in foreground
sudo netdata -D

# View traces
open http://localhost:16686
```

## Development

Use this README for the current development loop and tracing workflow.

For plugin-specific configuration details, see
[`otel-signal-viewer-plugin/README.md`](./otel-signal-viewer-plugin/README.md).

### Development Workflow

1. **Make changes** to the plugin code
2. **Rebuild** (fast, seconds): `cargo build --bin otel-signal-viewer-plugin`
3. **Restart** Netdata: `sudo systemctl restart netdata`
4. **View traces** in Jaeger: http://localhost:16686
5. **Check logs**: `sudo journalctl -u netdata -f`

### Key Benefits of Current Architecture

✅ **Simple** - Single binary, single mode (production-only)
✅ **Fast iteration** - Rebuild only the plugin, not all of Netdata
✅ **Observable** - Rich tracing and logging via Jaeger + stderr
✅ **Production parity** - Develop with exact production setup
✅ **No mock infrastructure** - No TCP bridges or test harnesses needed

## Project Structure

```
netdata-log-viewer/
├── histogram-service/       # Core business logic (library)
├── otel-signal-viewer-plugin/   # Netdata plugin (binary)
├── types/                   # Shared request/response types
├── watcher-plugin/          # DEPRECATED - no longer needed
├── lv/                      # DEPRECATED - no longer needed
├── otel-signal-viewer-plugin/README.md  # Plugin-specific configuration guide
└── README.md                # This file
```

## Configuration

The plugin is configured at compile time with sensible defaults:

- **Journal path**: `/var/log/journal`
- **Cache directory**: `/mnt/ramfs/foyer-storage`
- **Memory cache**: 10,000 entries
- **Disk cache**: 64 MiB

To customize, edit `create_shared_state()` in `otel-signal-viewer-plugin/src/main.rs`.

## Observability

### Tracing (Jaeger)

View request traces at http://localhost:16686:
- Function call timelines
- Histogram computation duration
- Lock acquisition times
- Error traces

### Logging (Stderr)

Control log verbosity with `RUST_LOG`:
```bash
# Debug everything
export RUST_LOG="debug"

# Selective logging
export RUST_LOG="otel_signal_viewer=trace,histogram_service=debug,journal=info"
```

### Metrics (Netdata)

The plugin reports its own metrics:
- `otel_signal_viewer.journal_calls` - Successful/failed/cancelled function calls

## Function Interface

The plugin exposes the `systemd-journal` function:

**Request**:
```json
{
  "after": 1699000000,
  "before": 1699100000,
  "selections": {
    "PRIORITY": ["3", "4"],
    "_HOSTNAME": ["server1"]
  }
}
```

**Response**:
```json
{
  "status": 200,
  "facets": [...],
  "histogram": [...],
  "available_histograms": [...],
  "columns": {...},
  "data": [...]
}
```

## Dependencies

### Core
- `rt` - Netdata plugin runtime
- `histogram-service` - Histogram computation
- `journal` - Journal file indexing

### Tracing
- `tracing` - Structured logging
- `opentelemetry` - Distributed tracing
- `opentelemetry-otlp` - OTLP exporter for Jaeger

### Async Runtime
- `tokio` - Async runtime

## Performance

- **Index caching**: Avoids re-reading journal files
- **Parallel processing**: Uses Rayon for CPU-bound work
- **Lock-free where possible**: RwLock allows concurrent reads
- **Efficient filtering**: Pre-built indexes for fast queries

## Troubleshooting

### Plugin not starting

```bash
# Check logs
sudo tail -f /var/log/netdata/error.log

# Run manually
sudo -u netdata /path/to/otel-signal-viewer-plugin
```

### No traces in Jaeger

```bash
# Verify Jaeger is running
docker ps | grep jaeger

# Check connectivity
nc -zv localhost 4317
```

### Slow queries

Check Jaeger traces for:
- Lock contention on shared state
- Cache misses in IndexCache
- Large time ranges

## License

[Your license here]

## Contributing

[Contributing guidelines here]
