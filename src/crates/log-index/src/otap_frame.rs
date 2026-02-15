//! Decodes a decompressed WAL frame payload into Arrow RecordBatches.
//!
//! # OTAP Arrow encoding
//!
//! The OpenTelemetry Arrow Protocol (OTAP) encodes telemetry data as a sequence
//! of Arrow IPC streams, one per "batch type" (e.g., Logs, LogAttrs, etc.).
//!
//! A single WAL frame payload contains multiple batch types concatenated
//! together. Each batch type is prefixed with a 1-byte tag identifying the
//! [`ArrowPayloadType`] and a 4-byte little-endian length:
//!
//! ```text
//! ┌──────────┬────────────┬──────────────────────────┐
//! │ tag (u8) │ len (u32)  │ Arrow IPC stream (bytes) │
//! └──────────┴────────────┴──────────────────────────┘
//!  ... repeated for each batch type in the frame ...
//! ```
//!
//! The Arrow IPC stream is the standard Arrow streaming format: a schema
//! message followed by one or more record batch messages. We read only the
//! first record batch from each stream (OTAP sends exactly one per stream).
//!
//! # Batch types relevant to logs
//!
//! For log signals, a frame typically contains four batch types:
//!
//! - **Logs** (`ArrowPayloadType::Logs`): The main log records. Each row is one
//!   log entry with fields like `time_unix_nano`, `severity_number`, etc.
//!   Contains `resource.id` and `scope.id` columns (UInt16) that reference
//!   rows in the corresponding attrs batches.
//!
//! - **ResourceAttrs** (`ArrowPayloadType::ResourceAttrs`): Resource-level
//!   attributes (e.g., `service.name`). Linked to Logs via `parent_id` →
//!   `resource.id`.
//!
//! - **ScopeAttrs** (`ArrowPayloadType::ScopeAttrs`): Instrumentation scope
//!   attributes. Linked to Logs via `parent_id` → `scope.id`.
//!
//! - **LogAttrs** (`ArrowPayloadType::LogAttrs`): Per-log attributes (e.g.,
//!   `log.body.*`). Linked to Logs via `parent_id` → `id`.
//!
//! # ID linkage
//!
//! The relationship between Logs and attribute batches uses UInt16 IDs:
//!
//! ```text
//! Logs.id            ──→  LogAttrs.parent_id
//! Logs.resource.id   ──→  ResourceAttrs.parent_id
//! Logs.scope.id      ──→  ScopeAttrs.parent_id
//! ```
//!
//! Multiple log rows can share the same `resource.id` or `scope.id`, meaning
//! resource/scope attributes are shared across logs in the same frame.

use std::io::Cursor;

use arrow::ipc::reader::StreamReader;
use arrow::record_batch::RecordBatch;
use otap_df_pdata::proto::opentelemetry::arrow::v1::ArrowPayloadType;

/// The decoded Arrow RecordBatches from a single WAL frame.
///
/// Each field corresponds to one of the OTAP batch types. Fields are `None`
/// if the batch type was not present in the frame (e.g., a frame with no
/// log attributes will have `log_attrs: None`).
pub(crate) struct OtapFrame {
    /// Resource-level attributes. Columns: `parent_id` (UInt16), `key`
    /// (Dict-encoded Utf8), `type` (UInt8), and value columns (`str`, `int`,
    /// `double`, `bool`, `bytes`).
    pub resource_attrs: Option<RecordBatch>,

    /// Instrumentation scope attributes. Same schema as `resource_attrs`.
    pub scope_attrs: Option<RecordBatch>,

    /// Per-log attributes. Same schema as `resource_attrs`.
    pub log_attrs: Option<RecordBatch>,

    /// The main log records. Columns include `id` (UInt16), `time_unix_nano`
    /// (TimestampNanosecond), `severity_number`, `severity_text`, and nested
    /// structs `resource { id }` and `scope { id, name, version }`.
    pub logs: Option<RecordBatch>,
}

impl OtapFrame {
    /// Decode a decompressed WAL frame payload into Arrow RecordBatches.
    ///
    /// The input `data` is the raw bytes after WAL-level decompression (e.g.,
    /// LZ4). It contains a sequence of tagged Arrow IPC streams as described
    /// in the module documentation.
    ///
    /// Batch types not relevant to logs (e.g., spans, metrics) are silently
    /// ignored.
    pub fn decode(data: &[u8]) -> Result<Self, Box<dyn std::error::Error>> {
        let mut frame = Self {
            logs: None,
            log_attrs: None,
            resource_attrs: None,
            scope_attrs: None,
        };

        let mut pos = 0;
        while pos < data.len() {
            // Each sub-stream is prefixed with a 1-byte tag and a 4-byte LE length.
            if pos + 5 > data.len() {
                return Err("truncated tag+length".into());
            }
            let tag = data[pos] as i32;
            pos += 1;
            let len = u32::from_le_bytes(data[pos..pos + 4].try_into().unwrap()) as usize;
            pos += 4;
            if pos + len > data.len() {
                return Err("truncated IPC stream".into());
            }

            // Map the tag to an ArrowPayloadType enum variant. Unknown tags
            // (e.g., from a newer protocol version) are rejected.
            let pt = ArrowPayloadType::try_from(tag).map_err(|_| format!("unknown tag: {tag}"))?;

            // Parse the Arrow IPC stream. StreamReader handles the schema
            // message and yields RecordBatches. OTAP sends exactly one batch
            // per stream, so we only read the first.
            let mut reader = StreamReader::try_new(Cursor::new(&data[pos..pos + len]), None)?;
            pos += len;

            let Some(batch) = reader.next() else {
                continue;
            };
            let batch = batch?;
            match pt {
                ArrowPayloadType::ResourceAttrs => frame.resource_attrs = Some(batch),
                ArrowPayloadType::ScopeAttrs => frame.scope_attrs = Some(batch),
                ArrowPayloadType::LogAttrs => frame.log_attrs = Some(batch),
                ArrowPayloadType::Logs => frame.logs = Some(batch),
                _ => {}
            }
        }

        Ok(frame)
    }
}
