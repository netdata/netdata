//! WAL writer configuration: the rotation thresholds and the per-file codec
//! flags.
//!
//! One [`Config`] is captured at `Writer::new` and cloned into each partition
//! stream (`writer.rs`); it is never mutated afterwards. The settings are
//! producer-side only: `crc_enabled`/`compression_enabled` are stamped into
//! each new file's header flags at creation, and the reader verifies frames
//! by those per-file flags (`reader.rs`), never by any config — files
//! written under different settings coexist in one directory, and a change
//! affects only files created afterwards.
//!
//! Producers: the otel-ingestor resolves one per tenant from
//! `bridge::config::WalConfig` (its `rotation` policy resolved per tenant;
//! `logs_service.rs`/`trace_service.rs` `resolve_wal_config`, time rotation
//! always on); the dev/bench `ng-ingest` disables rotation via
//! `one_file_config`; tests use `Default` or hand-set limits. `Default` is
//! the standalone default — the production path overrides every field from
//! the plugin's per-signal tuning and per-tenant rotation policy
//! (`bridge::config`). No serde: operator YAML is parsed upstream there;
//! this type is code-constructed.
use std::time::Duration;

use file_registry::ByteSize;

/// When to rotate a WAL file and start a new one. Thresholds are evaluated
/// before each `write_frame` against the current active file
/// (`should_rotate_with` in `writer.rs`) and re-checked with zero incoming
/// entries by the idle sweep (`Writer::rotate_expired`), so a file may
/// overshoot a limit by one frame until the next write or sweep seals it.
/// No validation: callers construct the fields directly.
#[derive(Debug, Clone)]
pub struct RotationConfig {
    /// Rotate before the incoming frame when the file's accumulated record
    /// count plus that frame's `entry_count` would exceed this limit — a
    /// strict `>`, so a file holds at most exactly this many log records.
    /// Unit: log records (`FrameMeta::entry_count`), not frames. A single
    /// frame above the limit is still admitted to the fresh file (the check
    /// sees the old file); `0` rotates before every nonempty frame.
    /// Default 100_000.
    pub max_entries: usize,
    /// Size cap per file, inclusive of the 4 KiB header page (accounting
    /// starts at `format::HEADER_SIZE`). Checked with `>=` before appending
    /// the incoming frame: the frame that crosses the limit is kept, and the
    /// next write (or the idle sweep) seals the file, so a file can exceed
    /// the cap by up to one frame. Default 256 MiB.
    pub max_file_size: ByteSize,
    /// Time cap per file; `None` disables time-based rotation. Age is
    /// measured on the caller's monotonic `ingestion_ns` — from the file's
    /// first frame to the candidate frame (write path) or the sweep clock
    /// (idle path) — and fires at `>=`; not the header's wall-clock
    /// `created_at`. The idle sweep applies the same threshold with zero
    /// incoming entries, sealing idle files at this age. Production always
    /// passes `Some`. Default 1 h.
    pub max_duration: Option<Duration>,
}

impl Default for RotationConfig {
    fn default() -> Self {
        Self {
            max_entries: 100_000,
            max_file_size: ByteSize(256 * 1024 * 1024),
            max_duration: Some(Duration::from_secs(3600)),
        }
    }
}

/// Configuration for one WAL writer: the rotation policy plus the codec
/// flags stamped into every file it creates. Passed complete to
/// `Writer::new` and cloned per stream — no defaults are applied at
/// construction, and it is effectively immutable for the writer's lifetime.
#[derive(Debug, Clone)]
pub struct Config {
    pub rotation: RotationConfig,
    /// Compute a CRC32 per frame (over the frame's header fields and the
    /// stored payload) and stamp `FLAG_CRC_ENABLED` into new files' headers;
    /// when off the checksum field is written as 0 and the flag omitted.
    /// Verification follows the per-file header flag (`reader.rs`), so
    /// toggling this only changes files created afterwards. Default true.
    pub crc_enabled: bool,
    /// LZ4-block-compress each frame's payload (the `compression_lz4()`
    /// accessor in `writer.rs`); when off, payloads are stored raw and new
    /// files' headers carry `COMPRESSION_NONE` (absent flag means LZ4 — the
    /// format default). Decompression follows the per-file flag, not this
    /// setting. Default true.
    pub compression_enabled: bool,
}

impl Default for Config {
    fn default() -> Self {
        Self {
            rotation: RotationConfig::default(),
            crc_enabled: true,
            compression_enabled: true,
        }
    }
}
