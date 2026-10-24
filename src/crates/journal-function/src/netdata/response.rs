//! Renders the logs table into the row arrays the Netdata logs UI consumes:
//! one JSON array per table row - `[µs timestamp, {"severity": ...}, field,
//! ...]`.
//!
//! Input is the transformed table from netdata/builder.rs - cells keep their
//! raw value beside the display string (`CellValue::with_display` in
//! `journal-engine/src/logs/table.rs`),
//! so the timestamp renders from the raw µs value while fields render their
//! display strings - plus the column schema from netdata/columns.rs
//! ([`crate::netdata::columns::generate_column_schema`]), which selects and orders the
//! rendered columns.
//!
//! Sole caller (grep-verified): [`crate::netdata::builder::build_ui_response`]
//! (its table-rendering step); `otel-legacy-logs/src/handler.rs`
//! (`LegacyLogsHandler::on_call`) puts the result
//! into the `data` field of [`crate::netdata::types::JournalResponse`].

use super::columns::ColumnSchema;
use super::severity::Severity;
use journal_engine::Table;
use serde_json::json;
use std::collections::HashMap;

/// Renders one table into the logs UI's row arrays, to be read against the
/// schema serialized by [`crate::netdata::columns::columns_to_sorted_json`].
///
/// Row shape:
/// ```json
/// [
///   [1717027200000000, {"severity": "critical"}, "disk full", "error"],
///   [1717027200000001, {"severity": "normal"}, "all good", "info"]
/// ]
/// ```
///
/// Each row carries, in order:
/// 1. Timestamp - the raw value of the table's `timestamp` column (matched
///    by column name, index-0 fallback), parsed as u64 microseconds; 0 when
///    missing or unparseable
/// 2. The rowOptions object `{"severity": ...}` - from the PRIORITY column's
///    raw value ([`Severity::from_priority`]; missing
///    or invalid PRIORITY renders "normal")
/// 3. One display value per remaining schema column (`timestamp`/`rowOptions`
///    excluded), in index order - a column renders when visible or present
///    in the table; `null` when the cell has no display value (column absent
///    from the table, or the entry lacks the field)
///
/// # Arguments
///
/// * `table` - The transformed table (cells carry raw + display values)
/// * `column_schema` - The column schema; selects and orders the rendered
///   columns
///
/// # Returns
///
/// One JSON array per table row, ready for the `data` field of
/// [`crate::netdata::types::JournalResponse`].
///
/// # Example
///
/// ```ignore
/// use journal_function::netdata::columns::generate_column_schema;
/// use journal_function::netdata::response::table_to_netdata_response;
///
/// // `table` and `field_names` come from the pipeline in netdata/builder.rs
/// let column_schema = generate_column_schema(&field_names);
/// let ui_rows = table_to_netdata_response(&table, &column_schema);
/// // JournalResponse.data = serde_json::json!(ui_rows)
/// ```
pub fn table_to_netdata_response(
    table: &Table,
    column_schema: &HashMap<String, ColumnSchema>,
) -> Vec<serde_json::Value> {
    let mut rows = Vec::with_capacity(table.row_count());

    // Build mapping: table column name → table column index
    let col_map: HashMap<&str, usize> = table
        .columns()
        .iter()
        .map(|col| (col.name.as_str(), col.index))
        .collect();

    // Timestamp column index: matched by name, index 0 as fallback
    let timestamp_idx = col_map.get("timestamp").copied().unwrap_or(0);

    // Get PRIORITY column index (optional, for severity calculation)
    let priority_idx = col_map.get("PRIORITY").copied();

    // Field columns to render: every schema column except the special
    // timestamp/rowOptions ones (sorted by index below).
    let mut schema_cols: Vec<_> = column_schema
        .values()
        .filter(|col| col.key != "timestamp" && col.key != "rowOptions")
        .filter(|col| {
            // Keep visible columns, plus hidden ones that exist in the table
            col.visible || col_map.contains_key(col.key.as_str())
        })
        .collect();
    schema_cols.sort_by_key(|col| col.index);

    for table_row in table.rows() {
        let mut ui_row = Vec::with_capacity(2 + schema_cols.len());

        // Element 0: timestamp as u64 µs (0 when missing or unparseable)
        let timestamp = table_row
            .get(timestamp_idx)
            .and_then(|cell| cell.raw.as_ref())
            .and_then(|s| s.parse::<u64>().ok())
            .unwrap_or(0);
        ui_row.push(json!(timestamp));

        // Element 1: rowOptions object - severity from the raw PRIORITY value
        let priority_value = priority_idx
            .and_then(|idx| table_row.get(idx))
            .and_then(|cell| cell.raw.as_deref());
        let severity = Severity::from_priority(priority_value);
        ui_row.push(json!({"severity": severity}));

        // Elements 2+: one display value per rendered column, in index order
        for schema_col in &schema_cols {
            if let Some(&table_idx) = col_map.get(schema_col.key.as_str()) {
                // The cell's display value (the transformed string)
                let value = table_row
                    .get(table_idx)
                    .and_then(|cell| cell.display.clone());
                ui_row.push(json!(value));
            } else {
                // Schema column absent from the table: render null
                ui_row.push(json!(null));
            }
        }

        rows.push(json!(ui_row));
    }

    rows
}
