//! OTAP frame processing — the Arrow/OTAP-specific part of Phase 1.
//!
//! Decodes a WAL frame into Arrow record batches, extracts timestamps and
//! key=value attributes, and feeds them into the [`WalIndex`].

use arrow::array::*;
use arrow::record_batch::RecordBatch;

use crate::KeyValueId;
use crate::arrow_columns::{AttrsMap, DictUtf8};
use crate::otap_frame::OtapFrame;
use crate::wal_index::WalIndex;

/// Decode and process a single WAL frame, updating all four data structures
/// in the [`WalIndex`].
///
/// For each log row in the frame:
/// 1. Records its timestamp.
/// 2. Interns all resource/scope/log attributes as `key=value` pairs.
/// 3. Updates forward bitmaps (`kv_id → {log_positions}`).
/// 4. Appends to the log entries (`log_position → [kv_ids]`).
///
/// Returns the number of log rows processed.
pub(crate) fn process_frame(
    wal_index: &mut WalIndex,
    wal_frame: &wal::WalFrame,
) -> Result<usize, Box<dyn std::error::Error>> {
    let otap_frame = OtapFrame::decode(wal_frame.data)?;
    let Some(logs_batch) = otap_frame.logs.as_ref() else {
        return Ok(0);
    };
    let global_log_offset = wal_index.num_logs();
    let ingestion_ns = wal_frame.timestamp_ns.0;

    collect_timestamps(logs_batch, ingestion_ns, &mut wal_index.timestamps);

    // Build key=value lookup tables for each attribute level.
    let resource_attrs = AttrsMap::build(
        otap_frame.resource_attrs.as_ref(),
        &mut wal_index.kv_interner,
    );
    let scope_attrs = AttrsMap::build(otap_frame.scope_attrs.as_ref(), &mut wal_index.kv_interner);
    let log_attrs = AttrsMap::build(otap_frame.log_attrs.as_ref(), &mut wal_index.kv_interner);

    // Each log row carries parent_id columns that link it to its
    // resource, scope, and log-level attribute rows.
    let resource_id_col = resolve_column::<UInt16Array>(logs_batch, &["resource", "id"]);
    let scope_id_col = resolve_column::<UInt16Array>(logs_batch, &["scope", "id"]);
    let log_id_col = resolve_column::<UInt16Array>(logs_batch, &["id"]);

    // scope.name and scope.version live as columns in the logs batch's
    // scope struct, not as rows in ScopeAttrs.
    let scope_name_col = resolve_scope_utf8(logs_batch, "name");
    let scope_version_col = resolve_scope_utf8(logs_batch, "version");

    wal_index.log_entries.reserve(logs_batch.num_rows());
    let mut scope_buf = String::new();

    for row in 0..logs_batch.num_rows() {
        let log_pos = (global_log_offset + row) as u32;
        let mut log_kv_ids: Vec<KeyValueId> = Vec::new();

        for (id_col, attrs) in [
            (resource_id_col, &resource_attrs),
            (scope_id_col, &scope_attrs),
            (log_id_col, &log_attrs),
        ] {
            let Some(col) = id_col else { continue };

            if col.is_null(row) {
                continue;
            }

            for &kv_id in attrs.get(col.value(row)) {
                wal_index.ensure_bitmap(kv_id);
                wal_index.kv_bitmaps[kv_id.idx()].insert(log_pos);
                log_kv_ids.push(kv_id);
            }
        }

        // Intern scope.name and scope.version from the logs batch columns.
        for (col, prefix) in [
            (&scope_name_col, "scope.name="),
            (&scope_version_col, "scope.version="),
        ] {
            if let Some(val) = col.as_ref().and_then(|c| c.value(row)) {
                if !val.is_empty() {
                    scope_buf.clear();
                    scope_buf.push_str(prefix);
                    scope_buf.push_str(val);

                    let kv_id = wal_index.kv_interner.intern(&scope_buf);
                    wal_index.ensure_bitmap(kv_id);
                    wal_index.kv_bitmaps[kv_id.idx()].insert(log_pos);
                    log_kv_ids.push(kv_id);
                }
            }
        }

        wal_index.log_entries.push(log_kv_ids);
    }

    Ok(logs_batch.num_rows())
}

/// Resolve a DictUtf8 column from the `scope` struct in the logs batch.
fn resolve_scope_utf8<'a>(batch: &'a RecordBatch, field: &str) -> Option<DictUtf8<'a>> {
    let scope_struct = batch
        .column_by_name("scope")?
        .as_any()
        .downcast_ref::<StructArray>()?;
    let col = scope_struct.column_by_name(field)?;
    DictUtf8::try_from(col.as_ref())
}

/// Resolve a column by path, navigating through nested StructArrays.
fn resolve_column<'a, T: 'static>(batch: &'a RecordBatch, path: &[&str]) -> Option<&'a T> {
    let mut col = batch.column_by_name(path.first()?)?;

    for &segment in &path[1..] {
        col = col
            .as_any()
            .downcast_ref::<StructArray>()?
            .column_by_name(segment)?;
    }

    col.as_any().downcast_ref::<T>()
}

/// Append the timestamp of each log row to `timestamps`.
///
/// Three-tier fallback following the OpenTelemetry Log Data Model:
///
/// 1. `time_unix_nano` — the event timestamp, if present and non-zero.
/// 2. `observed_time_unix_nano` — when the collector observed the event.
/// 3. `ingestion_ns` — the WAL frame's ingestion timestamp, with +1ns per
///    subsequent row to preserve intra-frame ordering.
///
/// A value of 0 in either OTAP column means "unknown or missing" per the
/// proto spec.
fn collect_timestamps(logs_rb: &RecordBatch, ingestion_ns: u64, timestamps: &mut Vec<i64>) {
    fn ts_value(col: Option<&TimestampNanosecondArray>, row: usize) -> Option<i64> {
        col.and_then(|c| {
            if c.is_null(row) {
                None
            } else {
                Some(c.value(row))
            }
        })
    }

    let time_col = logs_rb
        .column_by_name("time_unix_nano")
        .and_then(|c| c.as_any().downcast_ref::<TimestampNanosecondArray>());
    let observed_col = logs_rb
        .column_by_name("observed_time_unix_nano")
        .and_then(|c| c.as_any().downcast_ref::<TimestampNanosecondArray>());

    for row in 0..logs_rb.num_rows() {
        let ts = ts_value(time_col, row)
            .filter(|&v| v != 0)
            .or_else(|| ts_value(observed_col, row))
            .filter(|&v| v != 0)
            .unwrap_or(ingestion_ns as i64 + row as i64);

        timestamps.push(ts);
    }
}
